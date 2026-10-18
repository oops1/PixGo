package thumbs

import (
	"context"
	"errors"
	"image"
	"math"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/colorm"
	"github.com/oops1/PixGo/internal/fsutil"
	"github.com/oops1/PixGo/internal/par"
)

type Size int

const (
	Size96  Size = 96
	Size128 Size = 128
	Size160 Size = 160
	Size192 Size = 192
	Size256 Size = 256
	Size384 Size = 384
)

func Sizes() []Size {
	return []Size{Size96, Size128, Size160, Size192, Size256, Size384}
}

func (s Size) Px() int { return int(s) }

func Nearest(px int) Size {
	for _, s := range Sizes() {
		if int(s) >= px {
			return s
		}
	}
	return Size384
}

var (
	ErrTimeout    = errors.New("thumbs: time budget exceeded")
	ErrBadSide    = errors.New("thumbs: invalid thumbnail size")
	ErrNotRegular = errors.New("thumbs: not a regular file")
)

const (
	defaultMemoryEntries       = 512
	defaultDiskLimit     int64 = 256 << 20
	maxSide                    = 4096
	statTTL                    = 2 * time.Second
	recentWindow               = 2 * time.Second
	fileBudget                 = 5 * time.Second
	pruneEvery                 = 30 * time.Second
	statCacheMax               = 16384
	rankOther                  = math.MaxInt
)

type Key struct {
	Path    string
	Size    int64
	ModTime time.Time
	Side    int
	Color   string
}

func (k Key) ck() ckey {
	return ckey{path: k.Path, size: k.Size, mtime: k.ModTime.UnixNano(), side: k.Side, color: k.Color}
}

type Thumb struct {
	Img         *image.RGBA
	SrcW, SrcH  int
	Orientation codec.Orientation
	Err         error
}

type Options struct {
	Workers        int
	MemoryEntries  int
	DiskDir        string
	DiskLimitBytes int64
	Limits         func() codec.Limits
	Color          *colorm.Manager

	now    func() time.Time
	budget time.Duration
}

func DefaultDiskDir() string {
	return filepath.Join(fsutil.CacheDir(), "thumbs")
}

func DefaultOptions() Options {
	return Options{
		Workers:        defaultWorkers(),
		MemoryEntries:  defaultMemoryEntries,
		DiskDir:        DefaultDiskDir(),
		DiskLimitBytes: defaultDiskLimit,
	}
}

func defaultWorkers() int {
	return par.Share(4)
}

func (s *Service) Workers() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.target
}

func (s *Service) SetWorkers(n int) {
	if n <= 0 {
		n = defaultWorkers()
	}
	n = min(n, par.MaxThreads)
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.target = n
	s.maxAbandoned.Store(int32(max(2, 2*n)))
	for s.running < n {
		s.running++
		s.wg.Add(1)
		go s.worker()
	}
	s.mu.Unlock()
	s.cond.Broadcast()
}

type jobKey struct {
	path string
	side int
}

type job struct {
	jk      jobKey
	key     Key
	lim     codec.Limits
	seq     uint64
	rank    int
	lastReq time.Time
	ctx     context.Context
	cancel  context.CancelFunc
	dead    bool
	done    atomic.Bool
}

type statEntry struct {
	size  int64
	mtime time.Time
	err   error
	at    time.Time
}

type subscriber struct {
	id uint64
	fn func(path string)
}

type Service struct {
	reg    *codec.Registry
	post   func(func())
	limits func() codec.Limits
	disk   *diskCache
	opt    Options

	now          func() time.Time
	budget       time.Duration
	maxAbandoned atomic.Int32
	abandoned    atomic.Int32
	lastPrune    atomic.Int64

	ctx  context.Context
	stop context.CancelFunc
	wg   sync.WaitGroup

	mu      sync.Mutex
	cond    *sync.Cond
	closed  bool
	cache   *lru
	stats   map[string]statEntry
	pending map[jobKey]*job
	queue   []*job
	seq     uint64
	target  int
	running int
	vis     map[string]int
	subs    []subscriber
	subID   uint64
}

func New(reg *codec.Registry, post func(func()), opt Options) *Service {
	if reg == nil {
		reg = codec.Default
	}
	if post == nil {
		post = func(fn func()) { fn() }
	}
	if opt.Workers <= 0 {
		opt.Workers = defaultWorkers()
	}
	if opt.MemoryEntries <= 0 {
		opt.MemoryEntries = defaultMemoryEntries
	}
	if opt.DiskLimitBytes <= 0 {
		opt.DiskLimitBytes = defaultDiskLimit
	}
	limits := opt.Limits
	if limits == nil {
		limits = codec.DefaultLimits
	}
	clock := opt.now
	if clock == nil {
		clock = time.Now
	}
	budget := opt.budget
	if budget <= 0 {
		budget = fileBudget
	}
	ctx, stop := context.WithCancel(context.Background())
	s := &Service{
		reg:     reg,
		post:    post,
		limits:  limits,
		opt:     opt,
		now:     clock,
		budget:  budget,
		ctx:     ctx,
		stop:    stop,
		cache:   newLRU(opt.MemoryEntries),
		stats:   map[string]statEntry{},
		pending: map[jobKey]*job{},
		vis:     map[string]int{},
	}
	s.cond = sync.NewCond(&s.mu)
	if opt.DiskDir != "" {
		s.disk = &diskCache{dir: opt.DiskDir}
	}
	s.SetWorkers(opt.Workers)
	if s.disk != nil {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.maybePrune(true)
		}()
	}
	return s
}

func (s *Service) stat(path string, now time.Time) (statEntry, error) {
	s.mu.Lock()
	if e, ok := s.stats[path]; ok && now.Sub(e.at) < statTTL {
		s.mu.Unlock()
		return e, e.err
	}
	s.mu.Unlock()
	e := statEntry{at: now}
	fi, err := os.Stat(path)
	switch {
	case err != nil:
		e.err = err
	case !fi.Mode().IsRegular():
		e.err = ErrNotRegular
	default:
		e.size, e.mtime = fi.Size(), fi.ModTime().UTC()
	}
	s.mu.Lock()
	if len(s.stats) >= statCacheMax {
		for p, old := range s.stats {
			if now.Sub(old.at) >= statTTL {
				delete(s.stats, p)
			}
		}
		if len(s.stats) >= statCacheMax {
			clear(s.stats)
		}
	}
	s.stats[path] = e
	s.mu.Unlock()
	return e, e.err
}

func (s *Service) Request(path string, side int, visible bool) (Thumb, bool) {
	if side <= 0 || side > maxSide {
		return Thumb{Err: ErrBadSide}, true
	}
	now := s.now()
	st, err := s.stat(path, now)
	if err != nil {
		return Thumb{Err: err}, true
	}
	key := Key{Path: path, Size: st.size, ModTime: st.mtime, Side: side, Color: s.opt.Color.Tag()}
	ck := key.ck()
	jk := jobKey{path: path, side: side}

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return Thumb{}, false
	}
	if visible {
		s.markVisibleLocked(path)
	}
	if t, ok := s.cache.get(ck); ok {
		s.mu.Unlock()
		return t, true
	}
	if j := s.pending[jk]; j != nil && j.key.ck() == ck {
		s.touchLocked(j, now)
		s.mu.Unlock()
		return Thumb{}, false
	}
	s.mu.Unlock()

	lim := s.limits()

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return Thumb{}, false
	}
	if t, ok := s.cache.get(ck); ok {
		return t, true
	}
	if j := s.pending[jk]; j != nil {
		if j.key.ck() == ck {
			s.touchLocked(j, now)
			return Thumb{}, false
		}
		s.cancelLocked(j)
		s.compactLocked()
	}
	s.enqueueLocked(key, lim, now)
	return Thumb{}, false
}

func (s *Service) markVisibleLocked(path string) {
	if _, ok := s.vis[path]; !ok {
		s.vis[path] = len(s.vis)
	}
}

func (s *Service) touchLocked(j *job, now time.Time) {
	j.lastReq = now
	if r, ok := s.vis[j.jk.path]; ok && r < j.rank {
		j.rank = r
	}
}

func (s *Service) enqueueLocked(key Key, lim codec.Limits, now time.Time) {
	ctx, cancel := context.WithCancel(s.ctx)
	s.seq++
	rank := rankOther
	if r, ok := s.vis[key.Path]; ok {
		rank = r
	}
	j := &job{
		jk:      jobKey{path: key.Path, side: key.Side},
		key:     key,
		lim:     lim,
		seq:     s.seq,
		rank:    rank,
		lastReq: now,
		ctx:     ctx,
		cancel:  cancel,
	}
	s.pending[j.jk] = j
	s.queue = append(s.queue, j)
	s.cond.Signal()
}

func (s *Service) cancelLocked(j *job) {
	j.dead = true
	j.cancel()
	if s.pending[j.jk] == j {
		delete(s.pending, j.jk)
	}
}

func (s *Service) compactLocked() {
	kept := s.queue[:0]
	for _, j := range s.queue {
		if !j.dead {
			kept = append(kept, j)
		}
	}
	clear(s.queue[len(kept):])
	s.queue = kept
}

func (s *Service) SetVisible(paths []string) {
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.vis = make(map[string]int, len(paths))
	for _, p := range paths {
		if _, ok := s.vis[p]; !ok {
			s.vis[p] = len(s.vis)
		}
	}
	cancelled := false
	for _, j := range s.pending {
		if r, ok := s.vis[j.jk.path]; ok {
			j.rank = r
			continue
		}
		j.rank = rankOther
		if now.Sub(j.lastReq) >= recentWindow && !j.done.Load() {
			s.cancelLocked(j)
			cancelled = true
		}
	}
	if cancelled {
		s.compactLocked()
	}
}

func (s *Service) Subscribe(fn func(path string)) (unsubscribe func()) {
	if fn == nil {
		return func() {}
	}
	s.mu.Lock()
	s.subID++
	id := s.subID
	s.subs = append(s.subs, subscriber{id: id, fn: fn})
	s.mu.Unlock()
	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		for i, sub := range s.subs {
			if sub.id == id {
				s.subs = append(s.subs[:i:i], s.subs[i+1:]...)
				return
			}
		}
	}
}

func (s *Service) ClearDisk() error {
	if s.disk == nil {
		return nil
	}
	return s.disk.clear()
}

func (s *Service) DiskUsage() int64 {
	if s.disk == nil {
		return 0
	}
	return s.disk.usage()
}

func (s *Service) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		s.wg.Wait()
		return
	}
	s.closed = true
	for _, j := range s.pending {
		j.dead = true
		j.cancel()
	}
	clear(s.pending)
	clear(s.queue)
	s.queue = nil
	s.subs = nil
	s.mu.Unlock()
	s.stop()
	s.cond.Broadcast()
	s.wg.Wait()
}

func (s *Service) maybePrune(force bool) {
	if s.disk == nil {
		return
	}
	now := s.now()
	last := s.lastPrune.Load()
	if !force && now.UnixNano()-last < int64(pruneEvery) {
		return
	}
	if !s.lastPrune.CompareAndSwap(last, now.UnixNano()) {
		return
	}
	s.disk.prune(s.opt.DiskLimitBytes, now)
}

func (s *Service) worker() {
	defer s.wg.Done()
	for {
		j := s.next()
		if j == nil {
			return
		}
		s.run(j)
	}
}

func (s *Service) next() *job {
	s.mu.Lock()
	defer s.mu.Unlock()
	for {
		if s.closed || s.running > s.target {
			s.running--
			return nil
		}
		if j := s.popLocked(); j != nil {
			return j
		}
		s.cond.Wait()
	}
}

func (s *Service) popLocked() *job {
	best := -1
	for i, j := range s.queue {
		if j.dead {
			continue
		}
		if best < 0 || before(j, s.queue[best]) {
			best = i
		}
	}
	if best < 0 {
		clear(s.queue)
		s.queue = s.queue[:0]
		return nil
	}
	j := s.queue[best]
	last := len(s.queue) - 1
	s.queue[best] = s.queue[last]
	s.queue[last] = nil
	s.queue = s.queue[:last]
	return j
}

func before(a, b *job) bool {
	if a.rank != b.rank {
		return a.rank < b.rank
	}
	return a.seq < b.seq
}

func (s *Service) run(j *job) {
	ctx, cancel := context.WithTimeout(j.ctx, s.budget)
	t, fromRender := s.safeProduce(ctx, j)
	cancel()
	if j.ctx.Err() != nil {
		return
	}
	if t.Err != nil && errors.Is(t.Err, context.DeadlineExceeded) {
		t.Err = ErrTimeout
	}
	j.done.Store(true)
	s.post(func() { s.complete(j, t) })
	if fromRender && t.Err == nil && s.disk != nil {
		if s.disk.store(j.key, t) == nil {
			s.maybePrune(false)
		}
	}
}

func (s *Service) complete(j *job, t Thumb) {
	s.mu.Lock()
	if s.closed || s.pending[j.jk] != j {
		s.mu.Unlock()
		return
	}
	delete(s.pending, j.jk)
	s.cache.put(j.key.ck(), t)
	subs := append([]subscriber(nil), s.subs...)
	s.mu.Unlock()
	for _, sub := range subs {
		sub.fn(j.key.Path)
	}
}

func (s *Service) Pending() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.pending)
}
