package imaging

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/oops1/PixGo/internal/codec"
)

var errNotRegularFile = errors.New("imaging: not a regular file")

type Request struct {
	Path    string
	Prio    Priority
	VectorW int
	VectorH int
}

type Entry struct {
	Path  string
	State State
	Info  codec.Info
	Img   *Decoded
	Err   error
}

type job struct {
	path   string
	prio   Priority
	seq    uint64
	vw, vh int
	lim    codec.Limits
	ctx    context.Context
	cancel context.CancelFunc
}

type slot struct {
	entry Entry
	req   Request
	job   *job
}

type subscriber struct {
	id uint64
	fn func(path string)
}

type Loader struct {
	reg    *codec.Registry
	cache  *Cache
	post   func(func())
	limits func() codec.Limits

	ctx    context.Context
	stop   context.CancelFunc
	closed atomic.Bool
	wg     sync.WaitGroup

	mu    sync.Mutex
	cond  *sync.Cond
	queue []*job
	seq   uint64
	quit  bool

	slots map[string]*slot
	subs  []subscriber
	subID uint64
}

func NewLoader(reg *codec.Registry, cache *Cache, post func(func()), workers int, limits func() codec.Limits) *Loader {
	if reg == nil {
		reg = codec.Default
	}
	if cache == nil {
		cache = NewCache(0)
	}
	if workers <= 0 {
		workers = max(2, runtime.NumCPU()/2)
	}
	ctx, stop := context.WithCancel(context.Background())
	l := &Loader{
		reg:    reg,
		cache:  cache,
		post:   post,
		limits: limits,
		ctx:    ctx,
		stop:   stop,
		slots:  map[string]*slot{},
	}
	l.cond = sync.NewCond(&l.mu)
	l.wg.Add(workers)
	for i := 0; i < workers; i++ {
		go l.worker()
	}
	return l
}

func (l *Loader) currentLimits() codec.Limits {
	if l.limits == nil {
		return codec.DefaultLimits()
	}
	return l.limits()
}

func (l *Loader) newJobLocked(r Request, lim codec.Limits) *job {
	ctx, cancel := context.WithCancel(l.ctx)
	l.seq++
	j := &job{path: r.Path, prio: r.Prio, seq: l.seq, vw: r.VectorW, vh: r.VectorH, lim: lim, ctx: ctx, cancel: cancel}
	l.queue = append(l.queue, j)
	return j
}

func (l *Loader) cancelLocked(j *job) {
	j.cancel()
	for i, q := range l.queue {
		if q == j {
			l.queue[i] = l.queue[len(l.queue)-1]
			l.queue[len(l.queue)-1] = nil
			l.queue = l.queue[:len(l.queue)-1]
			return
		}
	}
}

func (l *Loader) Want(reqs []Request) {
	if l.closed.Load() {
		return
	}
	wanted := make(map[string]Request, len(reqs))
	order := make([]string, 0, len(reqs))
	for _, r := range reqs {
		if r.Path == "" {
			continue
		}
		old, seen := wanted[r.Path]
		if !seen {
			order = append(order, r.Path)
		}
		if !seen || r.Prio < old.Prio {
			wanted[r.Path] = r
		}
	}
	lim := l.currentLimits()
	l.mu.Lock()
	for path, s := range l.slots {
		if _, keep := wanted[path]; keep {
			continue
		}
		if s.job != nil {
			l.cancelLocked(s.job)
		}
		delete(l.slots, path)
	}
	for _, path := range order {
		r := wanted[path]
		s := l.slots[path]
		if s == nil {
			s = &slot{entry: Entry{Path: path, State: StateQueued}, req: r}
			s.job = l.newJobLocked(r, lim)
			l.slots[path] = s
			continue
		}
		s.req.Prio = r.Prio
		if s.job != nil && s.job.prio != r.Prio {
			l.seq++
			s.job.prio = r.Prio
			s.job.seq = l.seq
		}
	}
	l.mu.Unlock()
	l.cond.Broadcast()
}

func (l *Loader) Entry(path string) (Entry, bool) {
	s := l.slots[path]
	if s == nil {
		return Entry{}, false
	}
	return s.entry, true
}

func (l *Loader) Subscribe(fn func(path string)) (unsubscribe func()) {
	l.subID++
	id := l.subID
	l.subs = append(l.subs, subscriber{id: id, fn: fn})
	return func() {
		for i, s := range l.subs {
			if s.id == id {
				l.subs = append(l.subs[:i:i], l.subs[i+1:]...)
				return
			}
		}
	}
}

func (l *Loader) notify(path string) {
	if len(l.subs) == 0 {
		return
	}
	subs := append([]subscriber(nil), l.subs...)
	for _, s := range subs {
		s.fn(path)
	}
}

func (l *Loader) Reload(path string) {
	l.cache.Remove(path)
	s := l.slots[path]
	if s == nil || l.closed.Load() {
		return
	}
	lim := l.currentLimits()
	l.mu.Lock()
	if s.job != nil {
		l.cancelLocked(s.job)
	}
	s.job = l.newJobLocked(s.req, lim)
	l.mu.Unlock()
	s.entry = Entry{Path: path, State: StateQueued}
	l.cond.Broadcast()
	l.post(func() {
		if !l.closed.Load() {
			l.notify(path)
		}
	})
}

func (l *Loader) Close() {
	if !l.closed.CompareAndSwap(false, true) {
		l.wg.Wait()
		return
	}
	l.stop()
	l.mu.Lock()
	l.quit = true
	l.queue = nil
	l.mu.Unlock()
	l.cond.Broadcast()
	l.wg.Wait()
}

func (l *Loader) worker() {
	defer l.wg.Done()
	for {
		j := l.next()
		if j == nil {
			return
		}
		l.run(j)
	}
}

func (l *Loader) next() *job {
	l.mu.Lock()
	defer l.mu.Unlock()
	for {
		if l.quit {
			return nil
		}
		if n := len(l.queue); n > 0 {
			best := 0
			for i, j := range l.queue {
				b := l.queue[best]
				if j.prio < b.prio || (j.prio == b.prio && j.seq < b.seq) {
					best = i
				}
			}
			j := l.queue[best]
			l.queue[best] = l.queue[n-1]
			l.queue[n-1] = nil
			l.queue = l.queue[:n-1]
			return j
		}
		l.cond.Wait()
	}
}

func (l *Loader) publish(j *job, st State, info codec.Info, img *Decoded, err error) {
	if j.ctx.Err() != nil {
		return
	}
	l.post(func() { l.apply(j, st, info, img, err) })
}

func (l *Loader) apply(j *job, st State, info codec.Info, img *Decoded, err error) {
	if l.closed.Load() {
		return
	}
	s := l.slots[j.path]
	if s == nil || s.job != j {
		return
	}
	switch st {
	case StateProbed:
		if s.entry.State != StateQueued {
			return
		}
		s.entry.State = StateProbed
		s.entry.Info = info
	case StateReady:
		s.entry.State = StateReady
		s.entry.Info = img.Info
		s.entry.Img = img
		s.entry.Err = nil
		s.job = nil
	case StateFailed:
		s.entry.State = StateFailed
		s.entry.Err = err
		s.job = nil
	default:
		return
	}
	l.notify(j.path)
}

func (l *Loader) run(j *job) {
	defer func() {
		if v := recover(); v != nil {
			l.publish(j, StateFailed, codec.Info{}, nil, fmt.Errorf("imaging: panic while loading %s: %v", j.path, v))
		}
	}()
	if j.ctx.Err() != nil {
		return
	}
	st, err := os.Stat(j.path)
	if err == nil && !st.Mode().IsRegular() {
		err = errNotRegularFile
	}
	if err != nil {
		l.publish(j, StateFailed, codec.Info{}, nil, err)
		return
	}
	key := Key{Path: j.path, Size: st.Size(), ModTime: st.ModTime()}
	if d, ok := l.cache.Get(key); ok {
		l.publish(j, StateReady, d.Info, d, nil)
		return
	}
	src, err := codec.OpenFile(j.path)
	if err != nil {
		l.publish(j, StateFailed, codec.Info{}, nil, err)
		return
	}
	defer src.Close()
	dec, info, err := l.reg.Probe(j.ctx, src, j.lim)
	if err != nil {
		l.publish(j, StateFailed, codec.Info{}, nil, err)
		return
	}
	l.publish(j, StateProbed, info, nil, nil)
	opts := codec.Options{Limits: j.lim, Frames: codec.FirstFrame}
	if info.Animated {
		opts.Frames = codec.AllFrames
	}
	if info.Vector {
		opts.VectorW, opts.VectorH = j.vw, j.vh
	}
	img, err := l.reg.Decode(j.ctx, dec, src, opts)
	if err != nil {
		l.publish(j, StateFailed, info, nil, err)
		return
	}
	if j.ctx.Err() != nil {
		return
	}
	decoded := img.Info
	if decoded.Format == "" {
		decoded = info
	}
	d := &Decoded{
		Key:     key,
		Info:    decoded,
		Levels:  BuildPyramid(img.Frames[0].Pix, 0),
		Frames:  img.Frames,
		Loop:    img.Loop,
		Preview: info.Preview,
	}
	l.cache.Put(d)
	l.publish(j, StateReady, d.Info, d, nil)
}
