package imaging

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/oops1/PixGo/internal/codec"
)

var errBoom = errors.New("boom")

type fakeCodec struct {
	mu       sync.Mutex
	started  []string
	counts   map[string]int
	gates    map[string]chan struct{}
	canceled map[string]chan struct{}
	options  map[string]codec.Options
	startCh  chan string
}

func newFakeCodec() *fakeCodec {
	return &fakeCodec{
		counts:   map[string]int{},
		gates:    map[string]chan struct{}{},
		canceled: map[string]chan struct{}{},
		options:  map[string]codec.Options{},
		startCh:  make(chan string, 256),
	}
}

func (f *fakeCodec) ID() codec.ID           { return "fake" }
func (f *fakeCodec) Name() string           { return "Fake" }
func (f *fakeCodec) Extensions() []string   { return []string{".fake"} }
func (f *fakeCodec) Sniff(head []byte) bool { return bytes.HasPrefix(head, []byte("FAKE ")) }

func (f *fakeCodec) nameOf(r io.ReaderAt, size int64) string {
	buf := make([]byte, size)
	r.ReadAt(buf, 0)
	return strings.TrimSpace(strings.TrimPrefix(string(buf), "FAKE "))
}

func (f *fakeCodec) Probe(ctx context.Context, r io.ReaderAt, size int64, lim codec.Limits) (codec.Info, error) {
	name := f.nameOf(r, size)
	info := codec.Info{Format: "fake", FormatName: "Fake", Width: 600, Height: 400, Frames: 1, Orientation: codec.OrientNormal}
	switch {
	case strings.HasPrefix(name, "vec"):
		info.Vector = true
	case strings.HasPrefix(name, "anim"):
		info.Animated = true
		info.Frames = 3
	}
	return info, nil
}

func (f *fakeCodec) Decode(ctx context.Context, r io.ReaderAt, size int64, o codec.Options) (*codec.Image, error) {
	name := f.nameOf(r, size)
	f.mu.Lock()
	f.started = append(f.started, name)
	f.counts[name]++
	f.options[name] = o
	gate := f.gates[name]
	canceled := f.canceled[name]
	f.mu.Unlock()
	select {
	case f.startCh <- name:
	default:
	}
	if gate != nil {
		select {
		case <-gate:
		case <-ctx.Done():
			f.mu.Lock()
			if canceled != nil {
				select {
				case <-canceled:
				default:
					close(canceled)
				}
			}
			f.mu.Unlock()
			return nil, ctx.Err()
		}
	}
	switch {
	case strings.HasPrefix(name, "fail"):
		return nil, errBoom
	case strings.HasPrefix(name, "panic"):
		panic("decoder exploded")
	}
	pix := image.NewNRGBA(image.Rect(0, 0, 600, 400))
	for i := range pix.Pix {
		pix.Pix[i] = 255
	}
	info := codec.Info{Format: "fake", FormatName: "Fake", Width: 600, Height: 400, Frames: 1, Orientation: codec.OrientNormal}
	img := &codec.Image{Info: info, Frames: []codec.Frame{{Pix: pix}}}
	if strings.HasPrefix(name, "anim") {
		img.Info.Animated = true
		img.Info.Frames = 3
		for i := 0; i < 2; i++ {
			img.Frames = append(img.Frames, codec.Frame{Pix: image.NewNRGBA(image.Rect(0, 0, 600, 400)), Delay: 40 * time.Millisecond})
		}
	}
	return img, nil
}

func (f *fakeCodec) gate(name string) chan struct{} {
	f.mu.Lock()
	defer f.mu.Unlock()
	g := make(chan struct{})
	f.gates[name] = g
	f.canceled[name] = make(chan struct{})
	return g
}

func (f *fakeCodec) canceledCh(name string) chan struct{} {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.canceled[name]
}

func (f *fakeCodec) startedNames() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.started...)
}

func (f *fakeCodec) count(name string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.counts[name]
}

func (f *fakeCodec) optionsOf(name string) codec.Options {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.options[name]
}

type uiQueue struct {
	mu sync.Mutex
	q  []func()
}

func (u *uiQueue) Post(fn func()) {
	u.mu.Lock()
	u.q = append(u.q, fn)
	u.mu.Unlock()
}

func (u *uiQueue) Drain() int {
	n := 0
	for {
		u.mu.Lock()
		if len(u.q) == 0 {
			u.mu.Unlock()
			return n
		}
		fn := u.q[0]
		u.q = u.q[1:]
		u.mu.Unlock()
		fn()
		n++
	}
}

type rig struct {
	t      *testing.T
	dir    string
	fake   *fakeCodec
	ui     *uiQueue
	cache  *Cache
	loader *Loader
}

func newRig(t *testing.T, workers int) *rig {
	t.Helper()
	reg := codec.NewRegistry()
	fake := newFakeCodec()
	reg.RegisterDecoder(fake)
	r := &rig{t: t, dir: t.TempDir(), fake: fake, ui: &uiQueue{}, cache: NewCache(1 << 30)}
	r.loader = NewLoader(reg, r.cache, r.ui.Post, workers, nil)
	t.Cleanup(r.loader.Close)
	return r
}

func (r *rig) file(name string) string {
	r.t.Helper()
	p := filepath.Join(r.dir, name+".fake")
	if err := os.WriteFile(p, []byte("FAKE "+name), 0o644); err != nil {
		r.t.Fatal(err)
	}
	return p
}

func (r *rig) waitFor(what string, cond func() bool) {
	r.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		r.ui.Drain()
		if cond() {
			return
		}
		if time.Now().After(deadline) {
			r.t.Fatalf("timeout waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

func (r *rig) waitState(path string, st State) Entry {
	r.t.Helper()
	var e Entry
	r.waitFor("state of "+filepath.Base(path), func() bool {
		var ok bool
		e, ok = r.loader.Entry(path)
		return ok && e.State == st
	})
	return e
}

func (r *rig) waitStarted(n int) {
	r.t.Helper()
	r.waitFor("decodes to start", func() bool { return len(r.fake.startedNames()) >= n })
}

func req(path string, prio Priority) Request { return Request{Path: path, Prio: prio} }

func sameNames(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestLoaderDecodesInPriorityOrder(t *testing.T) {
	r := newRig(t, 1)
	gate := r.fake.gate("blocker")
	blocker := r.file("blocker")
	r.loader.Want([]Request{req(blocker, PrioCurrent)})
	r.waitStarted(1)
	names := []string{"p1", "n1", "c1", "n2", "p2", "c2"}
	prios := []Priority{PrioPrefetch, PrioNeighbor, PrioCurrent, PrioNeighbor, PrioPrefetch, PrioCurrent}
	reqs := []Request{req(blocker, PrioCurrent)}
	paths := map[string]string{}
	for i, n := range names {
		paths[n] = r.file(n)
		reqs = append(reqs, req(paths[n], prios[i]))
	}
	r.loader.Want(reqs)
	close(gate)
	for _, n := range names {
		r.waitState(paths[n], StateReady)
	}
	want := []string{"blocker", "c1", "c2", "n1", "n2", "p1", "p2"}
	if got := r.fake.startedNames(); !sameNames(got, want) {
		t.Fatalf("decode order %v, want %v", got, want)
	}
}

func TestLoaderUpdatesPriorityOfKeptJobs(t *testing.T) {
	r := newRig(t, 1)
	gate := r.fake.gate("blocker")
	blocker, a, b := r.file("blocker"), r.file("a"), r.file("b")
	r.loader.Want([]Request{req(blocker, PrioCurrent)})
	r.waitStarted(1)
	r.loader.Want([]Request{req(blocker, PrioCurrent), req(a, PrioPrefetch), req(b, PrioPrefetch)})
	r.loader.Want([]Request{req(blocker, PrioCurrent), req(a, PrioPrefetch), req(b, PrioCurrent)})
	close(gate)
	r.waitState(a, StateReady)
	r.waitState(b, StateReady)
	want := []string{"blocker", "b", "a"}
	if got := r.fake.startedNames(); !sameNames(got, want) {
		t.Fatalf("decode order %v, want %v", got, want)
	}
}

func TestLoaderCancelsJobsLeavingWantedSet(t *testing.T) {
	r := newRig(t, 1)
	r.fake.gate("slow")
	slow, gone, keep := r.file("slow"), r.file("gone"), r.file("keep")
	var seen []string
	r.loader.Subscribe(func(p string) { seen = append(seen, filepath.Base(p)) })
	r.loader.Want([]Request{req(slow, PrioCurrent), req(gone, PrioNeighbor), req(keep, PrioPrefetch)})
	r.waitStarted(1)
	r.loader.Want([]Request{req(keep, PrioPrefetch)})
	select {
	case <-r.fake.canceledCh("slow"):
	case <-time.After(5 * time.Second):
		t.Fatal("decoder did not observe context cancellation")
	}
	r.waitState(keep, StateReady)
	if got, want := r.fake.startedNames(), []string{"slow", "keep"}; !sameNames(got, want) {
		t.Fatalf("started %v, want %v", got, want)
	}
	for _, p := range []string{slow, gone} {
		if _, ok := r.loader.Entry(p); ok {
			t.Fatalf("entry for %s must be dropped", filepath.Base(p))
		}
	}
	r.ui.Drain()
	for _, s := range seen {
		if s == slow || s == gone {
			t.Fatalf("subscriber notified about cancelled path %s", s)
		}
	}
	if r.cache.Len() != 1 {
		t.Fatalf("only keep may be cached, cache holds %d", r.cache.Len())
	}
}

func TestLoaderPublishesProbedBeforeReady(t *testing.T) {
	r := newRig(t, 2)
	gate := r.fake.gate("a")
	a := r.file("a")
	var states []State
	r.loader.Subscribe(func(p string) {
		e, _ := r.loader.Entry(p)
		states = append(states, e.State)
	})
	if _, ok := r.loader.Entry(a); ok {
		t.Fatal("entry before Want")
	}
	r.loader.Want([]Request{req(a, PrioCurrent)})
	if e, ok := r.loader.Entry(a); !ok || e.State != StateQueued {
		t.Fatalf("entry right after Want: %+v %v", e, ok)
	}
	e := r.waitState(a, StateProbed)
	if e.Info.Width != 600 || e.Info.Height != 400 || e.Img != nil {
		t.Fatalf("probed entry: %+v", e)
	}
	close(gate)
	e = r.waitState(a, StateReady)
	if e.Img == nil || e.Err != nil || len(e.Img.Levels) < 2 {
		t.Fatalf("ready entry: %+v", e)
	}
	if e.Img.Levels[0] != e.Img.Frames[0].Pix {
		t.Fatal("Levels[0] must be the very same image as Frames[0].Pix")
	}
	if e.Img.Key.Path != a || e.Img.Key.Size != int64(len("FAKE a")) {
		t.Fatalf("key %+v", e.Img.Key)
	}
	if len(states) != 2 || states[0] != StateProbed || states[1] != StateReady {
		t.Fatalf("notified states %v", states)
	}
	if _, ok := r.cache.Get(e.Img.Key); !ok {
		t.Fatal("decoded image must be cached")
	}
}

func TestLoaderCacheHitSkipsDecoding(t *testing.T) {
	r := newRig(t, 2)
	a := r.file("a")
	var states []State
	r.loader.Want([]Request{req(a, PrioCurrent)})
	first := r.waitState(a, StateReady)
	r.loader.Want(nil)
	if _, ok := r.loader.Entry(a); ok {
		t.Fatal("entry must be dropped when no longer wanted")
	}
	r.loader.Subscribe(func(p string) {
		e, _ := r.loader.Entry(p)
		states = append(states, e.State)
	})
	r.loader.Want([]Request{req(a, PrioCurrent)})
	e := r.waitState(a, StateReady)
	if r.fake.count("a") != 1 {
		t.Fatalf("decoded %d times", r.fake.count("a"))
	}
	if e.Img != first.Img {
		t.Fatal("cache hit must return the cached image")
	}
	if len(states) != 1 || states[0] != StateReady {
		t.Fatalf("a cache hit must publish Ready directly, got %v", states)
	}
}

func TestLoaderRedecodesWhenFileChanged(t *testing.T) {
	r := newRig(t, 2)
	a := r.file("a")
	r.loader.Want([]Request{req(a, PrioCurrent)})
	first := r.waitState(a, StateReady)
	r.loader.Want(nil)
	later := time.Now().Add(3 * time.Second)
	if err := os.Chtimes(a, later, later); err != nil {
		t.Fatal(err)
	}
	r.loader.Want([]Request{req(a, PrioCurrent)})
	second := r.waitState(a, StateReady)
	if r.fake.count("a") != 2 {
		t.Fatalf("decoded %d times, want 2", r.fake.count("a"))
	}
	if second.Img == first.Img || !second.Img.Key.ModTime.After(first.Img.Key.ModTime) {
		t.Fatalf("new entry must carry the new modification time")
	}
	if r.cache.Len() != 1 {
		t.Fatalf("stale version must leave the cache, len %d", r.cache.Len())
	}
}

func TestLoaderReportsFailures(t *testing.T) {
	r := newRig(t, 2)
	bad := r.file("fail1")
	junk := filepath.Join(r.dir, "junk.fake")
	os.WriteFile(junk, []byte("this is not an image"), 0o644)
	missing := filepath.Join(r.dir, "missing.fake")
	panicky := r.file("panic1")
	dir := filepath.Join(r.dir, "folder.fake")
	os.Mkdir(dir, 0o755)
	r.loader.Want([]Request{req(bad, PrioCurrent), req(junk, PrioCurrent), req(missing, PrioCurrent), req(panicky, PrioCurrent), req(dir, PrioCurrent)})
	for _, p := range []string{bad, junk, missing, panicky, dir} {
		r.waitState(p, StateFailed)
	}
	if e, _ := r.loader.Entry(bad); !errors.Is(e.Err, errBoom) || e.Info.Width != 600 {
		t.Fatalf("decode failure: %+v", e)
	}
	if e, _ := r.loader.Entry(junk); !errors.Is(e.Err, codec.ErrUnknownFormat) {
		t.Fatalf("unknown format: %v", e.Err)
	}
	if e, _ := r.loader.Entry(missing); !errors.Is(e.Err, os.ErrNotExist) {
		t.Fatalf("missing: %v", e.Err)
	}
	if e, _ := r.loader.Entry(panicky); !errors.Is(e.Err, codec.ErrCorrupt) {
		t.Fatalf("panic must be reported as corrupt: %v", e.Err)
	}
	if e, _ := r.loader.Entry(dir); e.Err == nil {
		t.Fatal("directory must fail")
	}
	if r.cache.Len() != 0 {
		t.Fatal("failures must not be cached")
	}
}

func TestLoaderLimitsAreApplied(t *testing.T) {
	reg := codec.NewRegistry()
	fake := newFakeCodec()
	reg.RegisterDecoder(fake)
	ui := &uiQueue{}
	calls := 0
	lim := func() codec.Limits {
		calls++
		l := codec.DefaultLimits()
		l.MaxFileBytes = 4
		return l
	}
	l := NewLoader(reg, NewCache(1<<30), ui.Post, 1, lim)
	defer l.Close()
	dir := t.TempDir()
	p := filepath.Join(dir, "a.fake")
	os.WriteFile(p, []byte("FAKE a"), 0o644)
	l.Want([]Request{req(p, PrioCurrent)})
	if calls == 0 {
		t.Fatal("limits must be sampled on the calling (UI) thread inside Want")
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		ui.Drain()
		if e, _ := l.Entry(p); e.State == StateFailed {
			if !errors.Is(e.Err, codec.ErrTooLarge) {
				t.Fatalf("err %v", e.Err)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("timeout")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestLoaderPassesDecodeOptions(t *testing.T) {
	r := newRig(t, 2)
	vec, anim, plain := r.file("vec1"), r.file("anim1"), r.file("plain")
	r.loader.Want([]Request{{Path: vec, VectorW: 800, VectorH: 600}, req(anim, PrioCurrent), {Path: plain, VectorW: 5, VectorH: 5}})
	for _, p := range []string{vec, anim, plain} {
		r.waitState(p, StateReady)
	}
	if o := r.fake.optionsOf("vec1"); o.VectorW != 800 || o.VectorH != 600 || o.Frames != codec.FirstFrame {
		t.Fatalf("vector options %+v", o)
	}
	if o := r.fake.optionsOf("anim1"); o.Frames != codec.AllFrames || o.VectorW != 0 {
		t.Fatalf("animated options %+v", o)
	}
	if o := r.fake.optionsOf("plain"); o.VectorW != 0 || o.VectorH != 0 || o.Frames != codec.FirstFrame {
		t.Fatalf("plain options %+v", o)
	}
	e, _ := r.loader.Entry(anim)
	if len(e.Img.Frames) != 3 || e.Img.Levels[0] != e.Img.Frames[0].Pix {
		t.Fatalf("animation frames %d", len(e.Img.Frames))
	}
}

func TestLoaderReload(t *testing.T) {
	r := newRig(t, 2)
	a, b := r.file("a"), r.file("b")
	var notified []string
	r.loader.Want([]Request{req(a, PrioCurrent)})
	first := r.waitState(a, StateReady)
	r.loader.Subscribe(func(p string) { notified = append(notified, filepath.Base(p)) })
	r.loader.Reload(a)
	if e, ok := r.loader.Entry(a); !ok || e.State != StateQueued || e.Img != nil {
		t.Fatalf("after Reload: %+v", e)
	}
	second := r.waitState(a, StateReady)
	if r.fake.count("a") != 2 || second.Img == first.Img {
		t.Fatalf("Reload must decode again: count %d", r.fake.count("a"))
	}
	if len(notified) == 0 {
		t.Fatal("subscribers must hear about the reload")
	}
	r.loader.Want([]Request{req(a, PrioCurrent), req(b, PrioNeighbor)})
	r.waitState(b, StateReady)
	r.loader.Want(nil)
	r.loader.Reload(b)
	r.ui.Drain()
	if _, ok := r.loader.Entry(b); ok {
		t.Fatal("Reload of an unwanted path must not create an entry")
	}
	if r.cache.Len() != 1 {
		t.Fatalf("cache must have dropped b and kept a: len %d", r.cache.Len())
	}
	time.Sleep(20 * time.Millisecond)
	if r.fake.count("b") != 1 {
		t.Fatalf("unwanted path must not be re-decoded, count %d", r.fake.count("b"))
	}
}

func TestLoaderReloadCancelsRunningJob(t *testing.T) {
	r := newRig(t, 2)
	gate := r.fake.gate("a")
	a := r.file("a")
	r.loader.Want([]Request{req(a, PrioCurrent)})
	r.waitStarted(1)
	r.loader.Reload(a)
	select {
	case <-r.fake.canceledCh("a"):
	case <-time.After(5 * time.Second):
		t.Fatal("running decode must be cancelled by Reload")
	}
	close(gate)
	r.waitState(a, StateReady)
	if r.fake.count("a") != 2 {
		t.Fatalf("count %d", r.fake.count("a"))
	}
}

func TestLoaderUnsubscribe(t *testing.T) {
	r := newRig(t, 1)
	a, b := r.file("a"), r.file("b")
	var first, second int
	unsub := r.loader.Subscribe(func(string) { first++ })
	r.loader.Subscribe(func(string) { second++ })
	r.loader.Want([]Request{req(a, PrioCurrent)})
	r.waitState(a, StateReady)
	if first == 0 || first != second {
		t.Fatalf("first %d second %d", first, second)
	}
	unsub()
	unsub()
	before := first
	r.loader.Want([]Request{req(a, PrioCurrent), req(b, PrioCurrent)})
	r.waitState(b, StateReady)
	if first != before || second <= before {
		t.Fatalf("unsubscribed callback still invoked: first %d (was %d) second %d", first, before, second)
	}
}

func TestLoaderDeduplicatesRequests(t *testing.T) {
	r := newRig(t, 1)
	a := r.file("a")
	r.loader.Want([]Request{req(a, PrioPrefetch), req(a, PrioCurrent), req(a, PrioNeighbor), {}})
	r.waitState(a, StateReady)
	if r.fake.count("a") != 1 {
		t.Fatalf("decoded %d times", r.fake.count("a"))
	}
}

func settledGoroutines(base int) int {
	n := runtime.NumGoroutine()
	for i := 0; i < 200 && n > base; i++ {
		time.Sleep(10 * time.Millisecond)
		n = runtime.NumGoroutine()
	}
	return n
}

func TestLoaderCloseDoesNotLeakGoroutines(t *testing.T) {
	base := runtime.NumGoroutine()
	reg := codec.NewRegistry()
	fake := newFakeCodec()
	reg.RegisterDecoder(fake)
	ui := &uiQueue{}
	l := NewLoader(reg, NewCache(1<<30), ui.Post, 4, nil)
	dir := t.TempDir()
	var reqs []Request
	for _, n := range []string{"a", "b", "c", "d", "e", "f"} {
		fake.gate(n)
		p := filepath.Join(dir, n+".fake")
		os.WriteFile(p, []byte("FAKE "+n), 0o644)
		reqs = append(reqs, req(p, PrioCurrent))
	}
	l.Want(reqs)
	for i := 0; i < 4; i++ {
		select {
		case <-fake.startCh:
		case <-time.After(5 * time.Second):
			t.Fatal("decodes did not start")
		}
	}
	if runtime.NumGoroutine() <= base {
		t.Fatal("workers should be running")
	}
	l.Close()
	l.Close()
	if n := settledGoroutines(base); n > base {
		buf := make([]byte, 1<<16)
		buf = buf[:runtime.Stack(buf, true)]
		t.Fatalf("goroutines: before %d, after Close %d\n%s", base, n, buf)
	}
	started := fake.startedNames()
	if len(started) != 4 {
		t.Fatalf("exactly four decodes were expected to start, got %v", started)
	}
	for _, n := range started {
		select {
		case <-fake.canceledCh(n):
		default:
			t.Fatalf("blocked decode %s did not observe cancellation on Close", n)
		}
	}
	l.Want([]Request{req(filepath.Join(dir, "a.fake"), PrioCurrent)})
	ui.Drain()
	if n := runtime.NumGoroutine(); n > base {
		t.Fatalf("Want after Close started work: %d goroutines", n)
	}
}

func TestLoaderCloseWithIdleWorkers(t *testing.T) {
	base := runtime.NumGoroutine()
	l := NewLoader(codec.NewRegistry(), NewCache(1), (&uiQueue{}).Post, 3, nil)
	l.Close()
	if n := settledGoroutines(base); n > base {
		t.Fatalf("goroutines: before %d, after %d", base, n)
	}
}

func TestLoaderDefaultWorkerCount(t *testing.T) {
	base := runtime.NumGoroutine()
	l := NewLoader(codec.NewRegistry(), NewCache(1), (&uiQueue{}).Post, 0, nil)
	if runtime.NumGoroutine()-base < 2 {
		t.Fatal("at least two workers expected by default")
	}
	l.Close()
	if n := settledGoroutines(base); n > base {
		t.Fatalf("goroutines: before %d, after %d", base, n)
	}
}

func TestLoaderChurnUnderRaceDetector(t *testing.T) {
	r := newRig(t, 4)
	var paths []string
	for i := 0; i < 30; i++ {
		paths = append(paths, r.file(fmt.Sprintf("img%02d", i)))
	}
	for round := 0; round < 40; round++ {
		var reqs []Request
		for i := 0; i < 8; i++ {
			reqs = append(reqs, req(paths[(round*3+i)%len(paths)], Priority(i%3)))
		}
		r.loader.Want(reqs)
		r.ui.Drain()
		if round%7 == 0 {
			r.loader.Reload(paths[(round*3)%len(paths)])
		}
		time.Sleep(time.Millisecond)
	}
	r.loader.Want([]Request{req(paths[0], PrioCurrent), req(paths[1], PrioNeighbor)})
	r.waitState(paths[0], StateReady)
	r.waitState(paths[1], StateReady)
}
