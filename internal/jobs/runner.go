package jobs

import (
	"context"
	"fmt"
	"runtime/debug"
	"strings"
	"sync"
)

type PanicError struct {
	Key   string
	Value any
	Stack []byte
}

func (e *PanicError) Error() string {
	return fmt.Sprintf("jobs: panic in %q: %v", e.Key, e.Value)
}

type job struct {
	key    string
	gen    uint64
	cancel context.CancelFunc
	dead   bool
}

type Runner struct {
	post   func(func())
	mu     sync.Mutex
	jobs   map[string]*job
	gens   map[string]uint64
	root   context.Context
	stop   context.CancelFunc
	wg     sync.WaitGroup
	closed bool
}

func NewRunner(post func(func())) *Runner {
	if post == nil {
		post = func(fn func()) { fn() }
	}
	root, stop := context.WithCancel(context.Background())
	return &Runner{
		post: post,
		jobs: map[string]*job{},
		gens: map[string]uint64{},
		root: root,
		stop: stop,
	}
}

func (r *Runner) Go(key string, gen uint64, work func(ctx context.Context) (any, error), done func(any, error)) {
	r.mu.Lock()
	if r.closed || work == nil {
		r.mu.Unlock()
		return
	}
	if current, ok := r.gens[key]; ok && current != gen {
		r.mu.Unlock()
		return
	}
	if previous := r.jobs[key]; previous != nil {
		kill(previous)
	}
	ctx, cancel := context.WithCancel(r.root)
	j := &job{key: key, gen: gen, cancel: cancel}
	r.jobs[key] = j
	r.wg.Add(1)
	r.mu.Unlock()
	go r.run(ctx, j, work, done)
}

func (r *Runner) Cancel(key string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.forget(key)
}

func (r *Runner) CancelPrefix(prefix string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for key := range r.jobs {
		if strings.HasPrefix(key, prefix) {
			r.forget(key)
		}
	}
	for key := range r.gens {
		if strings.HasPrefix(key, prefix) {
			delete(r.gens, key)
		}
	}
}

func (r *Runner) SetGeneration(key string, gen uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}
	r.gens[key] = gen
	if j := r.jobs[key]; j != nil && j.gen != gen {
		kill(j)
		delete(r.jobs, key)
	}
}

func (r *Runner) Pending() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.jobs)
}

func (r *Runner) Close() {
	r.mu.Lock()
	r.closed = true
	for key, j := range r.jobs {
		kill(j)
		delete(r.jobs, key)
	}
	clear(r.gens)
	r.stop()
	r.mu.Unlock()
	r.wg.Wait()
}

func (r *Runner) run(ctx context.Context, j *job, work func(context.Context) (any, error), done func(any, error)) {
	defer r.wg.Done()
	defer j.cancel()
	value, err := call(ctx, j.key, work)
	if done == nil {
		r.release(j)
		return
	}
	r.mu.Lock()
	dead := j.dead
	r.mu.Unlock()
	if dead {
		return
	}
	r.post(func() {
		if r.release(j) {
			done(value, err)
		}
	})
}

func (r *Runner) release(j *job) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.jobs[j.key] == j {
		delete(r.jobs, j.key)
	}
	return !j.dead
}

func (r *Runner) forget(key string) {
	if j := r.jobs[key]; j != nil {
		kill(j)
		delete(r.jobs, key)
	}
	delete(r.gens, key)
}

func kill(j *job) {
	j.dead = true
	j.cancel()
}

func call(ctx context.Context, key string, work func(context.Context) (any, error)) (value any, err error) {
	defer func() {
		if p := recover(); p != nil {
			value, err = nil, &PanicError{Key: key, Value: p, Stack: debug.Stack()}
		}
	}()
	return work(ctx)
}
