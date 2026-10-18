package imaging

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/colorm"
	"github.com/oops1/PixGo/internal/par"
)

const (
	defaultViewW = 1920
	defaultViewH = 1080

	MinLoaderWorkers = 2
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
	path     string
	prio     Priority
	livePrio atomic.Uint32
	seq      uint64
	vw, vh   int
	lim      codec.Limits
	ctx      context.Context
	cancel   context.CancelFunc
}

func (j *job) setPrio(p Priority) {
	j.prio = p
	j.livePrio.Store(uint32(p))
}

func (j *job) currentPrio() Priority { return Priority(j.livePrio.Load()) }

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

	viewW      atomic.Int32
	viewH      atomic.Int32
	tileBudget atomic.Int64
	explicit   atomic.Int32
	color      atomic.Pointer[colorm.Manager]

	mu      sync.Mutex
	cond    *sync.Cond
	queue   []*job
	seq     uint64
	quit    bool
	target  int
	running int
	active  map[*job]struct{}

	heavy atomic.Pointer[string]

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
	ctx, stop := context.WithCancel(context.Background())
	l := &Loader{
		reg:    reg,
		cache:  cache,
		post:   post,
		limits: limits,
		ctx:    ctx,
		stop:   stop,
		slots:  map[string]*slot{},
		active: map[*job]struct{}{},
	}
	l.viewW.Store(defaultViewW)
	l.viewH.Store(defaultViewH)
	l.tileBudget.Store(DefaultTileBudget)
	l.cond = sync.NewCond(&l.mu)
	l.SetWorkers(workers)
	return l
}

func autoWorkers() int { return max(MinLoaderWorkers, par.Count()/2) }

func (l *Loader) Workers() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.target
}

func (l *Loader) SetWorkers(n int) {
	l.explicit.Store(int32(max(n, 0)))
	l.mu.Lock()
	if l.quit {
		l.mu.Unlock()
		return
	}
	l.target = autoWorkers()
	if n > 0 {
		l.target = min(n, par.MaxThreads)
	}
	for l.running < l.target {
		l.running++
		l.wg.Add(1)
		go l.worker()
	}
	l.mu.Unlock()
	l.cond.Broadcast()
}

func (l *Loader) tileWorkerCount() int {
	if n := int(l.explicit.Load()); n > 0 {
		return min(max(n/2, 1), MaxTileWorkers)
	}
	return 0
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
	j := &job{path: r.Path, seq: l.seq, vw: r.VectorW, vh: r.VectorH, lim: lim, ctx: ctx, cancel: cancel}
	j.setPrio(r.Prio)
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
	l.releaseHeavyUnless(reqs)
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
		switch {
		case s.job != nil && s.job.prio != r.Prio:
			l.seq++
			s.job.setPrio(r.Prio)
			s.job.seq = l.seq
		case s.job == nil && (s.entry.State == StateQueued || s.entry.State == StateProbed):
			s.job = l.newJobLocked(r, lim)
		}
	}
	l.mu.Unlock()
	l.cond.Broadcast()
	l.cache.Kick()
}

func (l *Loader) Entry(path string) (Entry, bool) {
	s := l.slots[path]
	if s == nil {
		return Entry{}, false
	}
	if s.entry.State == StateReady && (s.entry.Img.Released() || s.entry.Img.ColorStamp != l.colorStamp()) {
		l.requeue(s)
	}
	return s.entry, true
}

func (l *Loader) requeue(s *slot) {
	if l.closed.Load() {
		return
	}
	lim := l.currentLimits()
	l.mu.Lock()
	if s.job != nil {
		l.cancelLocked(s.job)
	}
	s.job = l.newJobLocked(s.req, lim)
	l.mu.Unlock()
	s.entry = Entry{Path: s.entry.Path, State: StateQueued}
	l.cond.Broadcast()
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
		l.finished(j)
	}
}

func (l *Loader) Pending() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.queue) + len(l.active)
}

func (l *Loader) finished(j *job) {
	l.mu.Lock()
	delete(l.active, j)
	l.mu.Unlock()
	l.cond.Broadcast()
}

func (l *Loader) currentRunningLocked() bool {
	for j := range l.active {
		if j.prio == PrioCurrent {
			return true
		}
	}
	return false
}

func (l *Loader) next() *job {
	l.mu.Lock()
	defer l.mu.Unlock()
	for {
		if l.quit || l.running > l.target {
			l.running--
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
			if j.prio != PrioCurrent && l.currentRunningLocked() {
				l.cond.Wait()
				continue
			}
			l.queue[best] = l.queue[n-1]
			l.queue[n-1] = nil
			l.queue = l.queue[:n-1]
			l.active[j] = struct{}{}
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
	case StatePreview:
		if s.entry.State != StateQueued && s.entry.State != StateProbed && s.entry.State != StatePreview {
			return
		}
		s.entry.State = StatePreview
		s.entry.Info = info
		s.entry.Img = img
	case StateReady:
		if img.Released() {
			l.requeue(s)
			return
		}
		s.entry.State = StateReady
		s.entry.Info = img.Info
		s.entry.Img = img
		s.entry.Err = nil
		s.job = nil
	case stateSkipped:
		if s.entry.State != StateQueued && s.entry.State != StateProbed {
			return
		}
		if s.req.Prio == PrioCurrent {
			l.requeue(s)
			return
		}
		s.job = nil
		return
	case StateFailed:
		s.entry.State = StateFailed
		s.entry.Err = err
		s.job = nil
	default:
		return
	}
	l.notify(j.path)
}
