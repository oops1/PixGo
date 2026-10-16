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
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/render"
)

type bigCodec struct {
	mu       sync.Mutex
	calls    map[string][]codec.Options
	readers  map[string][]*fakeReader
	opens    map[string]int
	gates    map[string]chan struct{}
	cancels  map[string]chan struct{}
	probeLog []string
}

func newBigCodec() *bigCodec {
	return &bigCodec{
		calls:   map[string][]codec.Options{},
		readers: map[string][]*fakeReader{},
		opens:   map[string]int{},
		gates:   map[string]chan struct{}{},
		cancels: map[string]chan struct{}{},
	}
}

func (c *bigCodec) ID() codec.ID           { return "big" }
func (c *bigCodec) Name() string           { return "Big" }
func (c *bigCodec) Extensions() []string   { return []string{".big"} }
func (c *bigCodec) Sniff(head []byte) bool { return bytes.HasPrefix(head, []byte("BIG ")) }

type bigSpec struct {
	name  string
	w, h  int
	flags map[string]bool
}

func (c *bigCodec) parse(r io.ReaderAt, size int64) bigSpec {
	buf := make([]byte, size)
	r.ReadAt(buf, 0)
	fields := strings.Fields(strings.TrimPrefix(string(buf), "BIG "))
	s := bigSpec{name: fields[0], flags: map[string]bool{}}
	dims := strings.SplitN(fields[1], "x", 2)
	s.w, _ = strconv.Atoi(dims[0])
	s.h, _ = strconv.Atoi(dims[1])
	for _, f := range fields[2:] {
		s.flags[f] = true
	}
	return s
}

func (c *bigCodec) Probe(ctx context.Context, r io.ReaderAt, size int64, lim codec.Limits) (codec.Info, error) {
	s := c.parse(r, size)
	info := codec.Info{Format: "big", FormatName: "Big", Width: s.w, Height: s.h, Frames: 1, Orientation: codec.OrientNormal}
	px := int64(s.w) * int64(s.h)
	switch {
	case px > lim.MaxTiledPixels:
		return codec.Info{}, codec.ErrTooLarge
	case px > lim.MaxPixels:
		return info, codec.ErrTooLarge
	}
	return info, nil
}

func (c *bigCodec) gateFor(name string) (chan struct{}, chan struct{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.gates[name], c.cancels[name]
}

func (c *bigCodec) setGate(name string) chan struct{} {
	c.mu.Lock()
	defer c.mu.Unlock()
	g := make(chan struct{})
	c.gates[name] = g
	c.cancels[name] = make(chan struct{})
	return g
}

func (c *bigCodec) Decode(ctx context.Context, r io.ReaderAt, size int64, o codec.Options) (*codec.Image, error) {
	s := c.parse(r, size)
	c.mu.Lock()
	c.calls[s.name] = append(c.calls[s.name], o)
	c.mu.Unlock()
	if gate, cancelled := c.gateFor(s.name); gate != nil && o.Scale <= 1 {
		select {
		case <-gate:
		case <-ctx.Done():
			select {
			case <-cancelled:
			default:
				close(cancelled)
			}
			return nil, ctx.Err()
		}
	}
	if s.flags["decfail"] || (s.flags["scalefail"] && o.Scale > 1) {
		return nil, errBoom
	}
	info := codec.Info{Format: "big", FormatName: "Big", Width: s.w, Height: s.h, Frames: 1, Orientation: codec.OrientNormal}
	scale := 1
	if s.flags["scale"] && o.Scale > 1 {
		scale = o.Scale
	}
	w, h := s.w, s.h
	if scale > 1 {
		shift := 0
		for 1<<uint(shift) < scale {
			shift++
		}
		w, h = codec.LevelSize(s.w, s.h, shift)
		if s.flags["scalebad"] {
			w++
		}
	}
	pix := image.NewNRGBA(image.Rect(0, 0, w, h))
	return &codec.Image{Info: info, Frames: []codec.Frame{{Pix: pix}}, Scale: scale}, nil
}

func (c *bigCodec) OpenTiled(ctx context.Context, r io.ReaderAt, size int64, lim codec.Limits) (codec.TileReader, error) {
	s := c.parse(r, size)
	c.mu.Lock()
	c.opens[s.name]++
	c.mu.Unlock()
	if s.flags["opentilefail"] {
		return nil, codec.ErrUnsupported
	}
	if s.flags["slowopen"] {
		gate, cancelled := c.gateFor(s.name)
		select {
		case <-gate:
		case <-ctx.Done():
			select {
			case <-cancelled:
			default:
				close(cancelled)
			}
			return nil, ctx.Err()
		}
	}
	rd := newFakeReader(picture{w: s.w, h: s.h, minShift: boolToInt(s.flags["minshift1"])})
	rd.opened.Add(1)
	if s.flags["ovfail"] {
		rd.overviewErr = errBoom
	}
	c.mu.Lock()
	c.readers[s.name] = append(c.readers[s.name], rd)
	c.mu.Unlock()
	if s.flags["ovwrong"] {
		return &wrongOverview{fakeReader: rd}, nil
	}
	return rd, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

type wrongOverview struct{ *fakeReader }

func (w *wrongOverview) Overview(ctx context.Context, shift int) (*image.NRGBA, error) {
	return image.NewNRGBA(image.Rect(0, 0, 10, 10)), nil
}

func (c *bigCodec) optionsOf(name string) []codec.Options {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]codec.Options(nil), c.calls[name]...)
}

func (c *bigCodec) openCount(name string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.opens[name]
}

func (c *bigCodec) readersOf(name string) []*fakeReader {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]*fakeReader(nil), c.readers[name]...)
}

func (c *bigCodec) openReaders() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, rs := range c.readers {
		for _, r := range rs {
			if r.opened.Load() > r.closed.Load() {
				n++
			}
		}
	}
	return n
}

type noTile struct{ codec.Decoder }

type bigRig struct {
	t      *testing.T
	dir    string
	big    *bigCodec
	ui     *uiQueue
	cache  *Cache
	loader *Loader
	lim    codec.Limits
	states map[string][]State

	filesMu sync.Mutex
	files   map[*sourceFile]bool
}

func (r *bigRig) openFiles() int {
	r.filesMu.Lock()
	defer r.filesMu.Unlock()
	n := 0
	for _, open := range r.files {
		if open {
			n++
		}
	}
	return n
}

func newBigRig(t *testing.T, workers int, lim codec.Limits, tiled bool) *bigRig {
	t.Helper()
	reg := codec.NewRegistry()
	big := newBigCodec()
	if tiled {
		reg.RegisterDecoder(big)
	} else {
		reg.RegisterDecoder(noTile{big})
	}
	r := &bigRig{t: t, dir: t.TempDir(), big: big, ui: &uiQueue{}, cache: NewCache(1 << 40), lim: lim, states: map[string][]State{}}
	r.loader = NewLoader(reg, r.cache, r.ui.Post, workers, func() codec.Limits { return r.lim })
	r.loader.Subscribe(func(p string) {
		if e, ok := r.loader.Entry(p); ok {
			r.states[filepath.Base(p)] = append(r.states[filepath.Base(p)], e.State)
		}
	})
	r.files = map[*sourceFile]bool{}
	opened := func(f *sourceFile) {
		r.filesMu.Lock()
		r.files[f] = true
		r.filesMu.Unlock()
	}
	closed := func(f *sourceFile) {
		r.filesMu.Lock()
		r.files[f] = false
		r.filesMu.Unlock()
	}
	openedHook.Store(&opened)
	closedHook.Store(&closed)
	t.Cleanup(func() {
		r.loader.Close()
		r.cache.Clear()
		openedHook.Store(nil)
		closedHook.Store(nil)
	})
	return r
}

func (r *bigRig) file(name string, w, h int, flags ...string) string {
	r.t.Helper()
	p := filepath.Join(r.dir, name+".big")
	body := fmt.Sprintf("BIG %s %dx%d %s", name, w, h, strings.Join(flags, " "))
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		r.t.Fatal(err)
	}
	return p
}

func (r *bigRig) waitFor(what string, cond func() bool) {
	r.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
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

func (r *bigRig) waitState(path string, st State) Entry {
	r.t.Helper()
	var e Entry
	r.waitFor("state "+strconv.Itoa(int(st))+" of "+filepath.Base(path), func() bool {
		var ok bool
		e, ok = r.loader.Entry(path)
		return ok && e.State == st
	})
	return e
}

func tiledLimits() codec.Limits {
	l := codec.DefaultLimits()
	l.TiledAbove = 8_000_000
	return l
}

func TestLoaderTiledPathForFileAboveMaxPixels(t *testing.T) {
	r := newBigRig(t, 2, tiledLimits(), true)
	p := r.file("huge", 20000, 15000)
	r.loader.Want([]Request{req(p, PrioCurrent)})
	e := r.waitState(p, StateReady)
	d := e.Img
	if d == nil || d.Reduced || d.Shift != 3 || d.Detail == nil {
		t.Fatalf("decoded: %+v", d)
	}
	if got := fmt.Sprint(r.states["huge.big"]); got != "[1 2]" {
		t.Fatalf("states %s, want Probed then Ready", got)
	}
	src := d.Source()
	if src.Width != 20000 || src.Height != 15000 || src.Shift != 3 || src.Detail == nil {
		t.Fatalf("source %d x %d shift %d", src.Width, src.Height, src.Shift)
	}
	if b := src.Levels[0].Bounds(); b.Dx() != 2500 || b.Dy() != 1875 {
		t.Fatalf("overview %v", b)
	}
	if len(d.Levels) < 2 || d.Levels[0] != d.Frames[0].Pix {
		t.Fatalf("levels %d", len(d.Levels))
	}
	if e.Info.Width != 20000 || e.Info.Height != 15000 {
		t.Fatalf("entry info %+v", e.Info)
	}
	if d.FullResolution() {
		t.Fatal("a tiled decode is not a full resolution frame")
	}
	if got := r.big.openCount("huge"); got != 1 {
		t.Fatalf("OpenTiled calls %d", got)
	}
	if calls := r.big.optionsOf("huge"); len(calls) != 0 {
		t.Fatalf("tiled path must not decode directly: %v", calls)
	}
	rd := r.big.readersOf("huge")[0]
	if ov := rd.overviews; len(ov) != 1 || ov[0] != 3 {
		t.Fatalf("overview calls %v", ov)
	}
	w := src.Detail.Window(0, image.Rect(1000, 1000, 1400, 1300))
	if w.Img == nil || w.Shift != 0 {
		t.Fatal("tile window not served")
	}
	if d.Bytes() < int64(len(d.Levels[0].Pix))+DefaultTileBudget {
		t.Fatalf("Bytes %d does not include the tile reserve", d.Bytes())
	}
	if cached, ok := r.cache.Get(d.Key); !ok || cached != d {
		t.Fatal("tiled decode must be cached")
	}
	if rd.closed.Load() != 0 || r.openFiles() != 1 {
		t.Fatalf("session must stay open while cached: reader closed %d, files open %d", rd.closed.Load(), r.openFiles())
	}
}

func TestLoaderTiledPathBecauseOfTiledAbove(t *testing.T) {
	r := newBigRig(t, 2, tiledLimits(), true)
	p := r.file("mid", 6000, 4000)
	r.loader.Want([]Request{req(p, PrioCurrent)})
	d := r.waitState(p, StateReady).Img
	if d.Shift != 1 || d.Detail == nil {
		t.Fatalf("shift %d detail %v", d.Shift, d.Detail)
	}
	if b := d.Levels[0].Bounds(); b.Dx() != 3000 || b.Dy() != 2000 {
		t.Fatalf("overview %v", b)
	}
	if len(r.big.optionsOf("mid")) != 0 {
		t.Fatal("no direct decode expected")
	}
}

func TestLoaderTiledPathAtFullResolutionIsAPlainImage(t *testing.T) {
	r := newBigRig(t, 2, tiledLimits(), true)
	lim := tiledLimits()
	lim.TiledAbove = 500_000
	r.lim = lim
	p := r.file("small", 2000, 1000)
	r.loader.Want([]Request{req(p, PrioCurrent)})
	d := r.waitState(p, StateReady).Img
	if d.Shift != 0 || d.Detail != nil || !d.FullResolution() {
		t.Fatalf("decoded: shift %d detail %v", d.Shift, d.Detail)
	}
	rd := r.big.readersOf("small")[0]
	if rd.closed.Load() != 1 {
		t.Fatalf("a session that is not needed must be closed at once: %d", rd.closed.Load())
	}
	if len(r.big.optionsOf("small")) != 0 {
		t.Fatal("overview only, no direct decode")
	}
	r.waitFor("source file closed", func() bool { return r.openFiles() == 0 })
}

func TestLoaderOversizeWithoutTiledDecoderFails(t *testing.T) {
	r := newBigRig(t, 2, tiledLimits(), false)
	p := r.file("huge", 20000, 15000)
	r.loader.Want([]Request{req(p, PrioCurrent)})
	e := r.waitState(p, StateFailed)
	if !errors.Is(e.Err, codec.ErrTooLarge) {
		t.Fatalf("err %v", e.Err)
	}
	if e.Info.Width != 20000 {
		t.Fatalf("failure should keep the probed info: %+v", e.Info)
	}
	if len(r.big.optionsOf("huge")) != 0 {
		t.Fatal("must not decode")
	}
}

func TestLoaderBeyondTiledLimitFails(t *testing.T) {
	r := newBigRig(t, 2, tiledLimits(), true)
	p := r.file("absurd", 40000, 40000)
	r.loader.Want([]Request{req(p, PrioCurrent)})
	e := r.waitState(p, StateFailed)
	if !errors.Is(e.Err, codec.ErrTooLarge) {
		t.Fatalf("err %v", e.Err)
	}
	if r.big.openCount("absurd") != 0 {
		t.Fatal("OpenTiled must not be tried beyond MaxTiledPixels")
	}
}

func TestLoaderTiledOpenFailureFallsBackToDirectDecode(t *testing.T) {
	r := newBigRig(t, 2, tiledLimits(), true)
	p := r.file("fallback", 3000, 3000, "opentilefail")
	r.loader.Want([]Request{req(p, PrioNeighbor)})
	d := r.waitState(p, StateReady).Img
	if d.Detail != nil || d.Shift != 0 || len(d.Levels) == 0 {
		t.Fatalf("decoded %+v", d)
	}
	if got := len(r.big.optionsOf("fallback")); got != 1 {
		t.Fatalf("direct decodes %d", got)
	}
	r.waitFor("source file closed", func() bool { return r.openFiles() == 0 })
}

func TestLoaderTiledOpenFailureOfOversizeFileFails(t *testing.T) {
	r := newBigRig(t, 2, tiledLimits(), true)
	p := r.file("nope", 20000, 15000, "opentilefail")
	r.loader.Want([]Request{req(p, PrioCurrent)})
	e := r.waitState(p, StateFailed)
	if !errors.Is(e.Err, codec.ErrUnsupported) {
		t.Fatalf("err %v", e.Err)
	}
}

func TestLoaderBrokenOverviewFailsAndClosesTheSession(t *testing.T) {
	r := newBigRig(t, 2, tiledLimits(), true)
	wrong := r.file("wrong", 20000, 15000, "ovwrong")
	broken := r.file("broken", 20000, 15000, "ovfail")
	r.loader.Want([]Request{req(wrong, PrioCurrent), req(broken, PrioNeighbor)})
	e := r.waitState(wrong, StateFailed)
	if !errors.Is(e.Err, codec.ErrCorrupt) {
		t.Fatalf("err %v", e.Err)
	}
	r.waitState(broken, StateFailed)
	r.waitFor("sessions closed", func() bool { return r.big.openReaders() == 0 })
	r.waitFor("source files closed", func() bool { return r.openFiles() == 0 })
	if r.cache.Len() != 0 {
		t.Fatal("failures must not be cached")
	}
}

func TestLoaderCancelledTiledOpenReleasesEverything(t *testing.T) {
	base := runtime.NumGoroutine()
	r := newBigRig(t, 2, tiledLimits(), true)
	r.big.setGate("slow")
	p := r.file("slow", 20000, 15000, "slowopen")
	r.loader.Want([]Request{req(p, PrioCurrent)})
	r.waitFor("OpenTiled call", func() bool { return r.big.openCount("slow") == 1 })
	r.loader.Want(nil)
	_, cancelled := r.big.gateFor("slow")
	select {
	case <-cancelled:
	case <-time.After(5 * time.Second):
		t.Fatal("OpenTiled did not observe the cancellation")
	}
	time.Sleep(20 * time.Millisecond)
	r.ui.Drain()
	if r.cache.Len() != 0 {
		t.Fatal("a cancelled load must not be cached")
	}
	if _, ok := r.loader.Entry(p); ok {
		t.Fatal("entry must be dropped")
	}
	r.waitFor("source file closed", func() bool { return r.openFiles() == 0 })
	if r.big.openReaders() != 0 {
		t.Fatal("session leaked")
	}
	r.loader.Close()
	if n := settledGoroutines(base); n > base {
		t.Fatalf("goroutines %d, before %d", n, base)
	}
}

func TestLoaderEvictionReleasesTiledSessions(t *testing.T) {
	r := newBigRig(t, 1, tiledLimits(), true)
	one := int64(2500*1875*4) * 2
	r.cache.SetBudget(one)
	a := r.file("a", 20000, 15000)
	b := r.file("b", 20000, 15000)
	r.loader.Want([]Request{req(a, PrioCurrent)})
	da := r.waitState(a, StateReady).Img
	r.loader.Want([]Request{req(b, PrioCurrent)})
	r.waitState(b, StateReady)
	if !da.Released() {
		t.Fatal("the evicted decode must be released")
	}
	rd := r.big.readersOf("a")[0]
	if rd.closed.Load() != 1 {
		t.Fatalf("evicted session closed %d times", rd.closed.Load())
	}
	if w := da.Detail.Window(0, image.Rect(0, 0, 100, 100)); w.Img != nil {
		t.Fatal("released detail must not serve windows")
	}
	if r.big.readersOf("b")[0].closed.Load() != 0 {
		t.Fatal("the new session must stay open")
	}
	if r.openFiles() != 1 {
		t.Fatalf("exactly the file of the live session must stay open, got %d", r.openFiles())
	}
}

func TestLoaderPinnedDecodeSurvivesBudgetPressure(t *testing.T) {
	r := newBigRig(t, 1, tiledLimits(), true)
	r.cache.SetBudget(1)
	a := r.file("a", 20000, 15000)
	b := r.file("b", 20000, 15000)
	r.loader.Want([]Request{req(a, PrioCurrent)})
	da := r.waitState(a, StateReady).Img
	r.cache.Pin(da.Key)
	r.loader.Want([]Request{req(b, PrioCurrent)})
	r.waitState(b, StateReady)
	if da.Released() {
		t.Fatal("pinned decode must not be released")
	}
	r.cache.Unpin(da.Key)
	if !da.Released() {
		t.Fatal("unpinned decode over budget must be released")
	}
}

func TestLoaderRequeuesWhenReadyEntryWasReleased(t *testing.T) {
	r := newBigRig(t, 1, tiledLimits(), true)
	a := r.file("a", 20000, 15000)
	r.loader.Want([]Request{req(a, PrioCurrent)})
	first := r.waitState(a, StateReady).Img
	r.cache.Remove(a)
	if !first.Released() {
		t.Fatal("Remove must release")
	}
	e, ok := r.loader.Entry(a)
	if !ok || e.State != StateQueued || e.Img != nil {
		t.Fatalf("a released ready entry must be queued again: %+v", e)
	}
	second := r.waitState(a, StateReady).Img
	if second == first || second.Released() {
		t.Fatal("expected a fresh decode")
	}
	if r.big.openCount("a") != 2 {
		t.Fatalf("opens %d", r.big.openCount("a"))
	}
}

func TestLoaderCacheHitReusesTiledSession(t *testing.T) {
	r := newBigRig(t, 1, tiledLimits(), true)
	a := r.file("a", 20000, 15000)
	r.loader.Want([]Request{req(a, PrioCurrent)})
	first := r.waitState(a, StateReady).Img
	r.loader.Want(nil)
	r.loader.Want([]Request{req(a, PrioCurrent)})
	second := r.waitState(a, StateReady).Img
	if second != first || r.big.openCount("a") != 1 {
		t.Fatalf("cache hit must reuse the open session (opens %d)", r.big.openCount("a"))
	}
}

func TestLoaderTiledFileCanBeDeletedWhileOpen(t *testing.T) {
	r := newBigRig(t, 1, tiledLimits(), true)
	a := r.file("a", 20000, 15000)
	r.loader.Want([]Request{req(a, PrioCurrent)})
	r.waitState(a, StateReady)
	if err := os.Remove(a); err != nil {
		t.Fatalf("an open tiled session must not block deleting the file: %v", err)
	}
}

func TestLoaderTiledPathSurvivesReload(t *testing.T) {
	r := newBigRig(t, 1, tiledLimits(), true)
	a := r.file("a", 20000, 15000)
	r.loader.Want([]Request{req(a, PrioCurrent)})
	first := r.waitState(a, StateReady).Img
	r.loader.Reload(a)
	if !first.Released() {
		t.Fatal("Reload must release the old session")
	}
	second := r.waitState(a, StateReady).Img
	if second == first {
		t.Fatal("expected a new decode")
	}
}

func previewLimits() codec.Limits {
	l := codec.DefaultLimits()
	l.TiledAbove = 0
	return l
}

func TestLoaderPreviewStageThenFinal(t *testing.T) {
	r := newBigRig(t, 2, previewLimits(), false)
	r.loader.SetViewportHint(800, 600)
	gate := r.big.setGate("photo")
	p := r.file("photo", 2400, 1800, "scale")
	r.loader.Want([]Request{req(p, PrioCurrent)})
	e := r.waitState(p, StatePreview)
	d := e.Img
	if d == nil || !d.Reduced || d.Shift != 1 || d.Detail != nil {
		t.Fatalf("preview decode: %+v", d)
	}
	src := d.Source()
	if src.Width != 2400 || src.Height != 1800 || src.Shift != 1 {
		t.Fatalf("preview source %d x %d shift %d", src.Width, src.Height, src.Shift)
	}
	if b := d.Levels[0].Bounds(); b.Dx() != 1200 || b.Dy() != 900 {
		t.Fatalf("preview level %v", b)
	}
	if e.Info.Width != 2400 || d.Info.Width != 2400 || d.Info.Height != 1800 {
		t.Fatalf("preview must carry the full size: %+v %+v", e.Info, d.Info)
	}
	if r.cache.Len() != 0 {
		t.Fatal("the preview must not be cached")
	}
	if d.FullResolution() {
		t.Fatal("preview is not full resolution")
	}
	close(gate)
	final := r.waitState(p, StateReady).Img
	if final.Reduced || final.Shift != 0 || final.Levels[0].Bounds().Dx() != 2400 {
		t.Fatalf("final decode: %+v", final)
	}
	if r.cache.Len() != 1 {
		t.Fatal("the final decode must be cached")
	}
	calls := r.big.optionsOf("photo")
	if len(calls) != 2 || calls[0].Scale != 2 || calls[1].Scale != 0 {
		t.Fatalf("decode options %+v", calls)
	}
	r.waitFor("source file closed", func() bool { return r.openFiles() == 0 })
	if got := fmt.Sprint(r.states["photo.big"]); got != "[1 4 2]" {
		t.Fatalf("states %s, want Probed, Preview, Ready", got)
	}
}

func TestLoaderPreviewScaleFollowsViewport(t *testing.T) {
	cases := []struct {
		vw, vh int
		scale  int
	}{
		{800, 600, 2},
		{500, 400, 4},
		{250, 200, 8},
		{100, 100, 8},
		{1600, 1200, 0},
		{2400, 1800, 0},
		{3840, 2160, 0},
	}
	for _, tc := range cases {
		r := newBigRig(t, 1, previewLimits(), false)
		r.loader.SetViewportHint(tc.vw, tc.vh)
		p := r.file("photo", 2400, 1800, "scale")
		r.loader.Want([]Request{req(p, PrioCurrent)})
		r.waitState(p, StateReady)
		calls := r.big.optionsOf("photo")
		if tc.scale == 0 {
			if len(calls) != 1 || calls[0].Scale != 0 {
				t.Fatalf("viewport %dx%d: expected a single plain decode, got %+v", tc.vw, tc.vh, calls)
			}
			continue
		}
		if len(calls) != 2 || calls[0].Scale != tc.scale {
			t.Fatalf("viewport %dx%d: expected preview scale %d, got %+v", tc.vw, tc.vh, tc.scale, calls)
		}
	}
}

func TestLoaderPreviewHonoursRotatedViewport(t *testing.T) {
	r := newBigRig(t, 1, previewLimits(), false)
	r.loader.SetViewportHint(600, 800)
	p := r.file("photo", 2400, 1800, "scale")
	r.loader.Want([]Request{req(p, PrioCurrent)})
	r.waitState(p, StateReady)
	calls := r.big.optionsOf("photo")
	if len(calls) != 2 || calls[0].Scale != 2 {
		t.Fatalf("a portrait viewport shows the landscape picture and its rotated copy at the same fit: %+v", calls)
	}
}

func TestLoaderPreviewSkippedForNeighboursAndSmallImages(t *testing.T) {
	r := newBigRig(t, 2, previewLimits(), false)
	r.loader.SetViewportHint(800, 600)
	near := r.file("near", 2400, 1800, "scale")
	small := r.file("small", 1500, 1000, "scale")
	pre := r.file("pre", 2400, 1800, "scale")
	r.loader.Want([]Request{req(near, PrioNeighbor), req(small, PrioCurrent), req(pre, PrioPrefetch)})
	for _, p := range []string{near, small, pre} {
		r.waitState(p, StateReady)
	}
	for _, name := range []string{"near", "small", "pre"} {
		calls := r.big.optionsOf(name)
		if len(calls) != 1 || calls[0].Scale != 0 {
			t.Fatalf("%s: %+v", name, calls)
		}
	}
	for _, name := range []string{"near.big", "small.big", "pre.big"} {
		if got := fmt.Sprint(r.states[name]); got != "[1 2]" {
			t.Fatalf("%s states %s", name, got)
		}
	}
}

func TestLoaderPreviewPromotedWhenNeighbourBecomesCurrent(t *testing.T) {
	r := newBigRig(t, 1, previewLimits(), false)
	r.loader.SetViewportHint(800, 600)
	block := r.file("block", 100, 100)
	gate := r.big.setGate("block")
	target := r.file("target", 2400, 1800, "scale")
	r.loader.Want([]Request{req(block, PrioCurrent), req(target, PrioNeighbor)})
	r.waitFor("blocker", func() bool { return len(r.big.optionsOf("block")) == 1 })
	r.loader.Want([]Request{req(block, PrioCurrent), req(target, PrioCurrent)})
	close(gate)
	r.waitState(target, StateReady)
	calls := r.big.optionsOf("target")
	if len(calls) != 2 || calls[0].Scale != 2 {
		t.Fatalf("a job promoted to current before it starts must get the preview: %+v", calls)
	}
}

func TestLoaderHintedDecodeThatCannotReduceIsFinal(t *testing.T) {
	r := newBigRig(t, 1, previewLimits(), false)
	r.loader.SetViewportHint(800, 600)
	p := r.file("png", 2400, 1800)
	r.loader.Want([]Request{req(p, PrioCurrent)})
	e := r.waitState(p, StateReady)
	calls := r.big.optionsOf("png")
	if len(calls) != 1 || calls[0].Scale != 2 {
		t.Fatalf("exactly one decode with the hint expected: %+v", calls)
	}
	if e.Img.Reduced || e.Img.Shift != 0 || e.Img.Levels[0].Bounds().Dx() != 2400 {
		t.Fatalf("decode: %+v", e.Img)
	}
	if got := fmt.Sprint(r.states["png.big"]); got != "[1 2]" {
		t.Fatalf("states %s, there must be no preview state", got)
	}
}

func TestLoaderPreviewWithWrongSizeIsSkipped(t *testing.T) {
	r := newBigRig(t, 1, previewLimits(), false)
	r.loader.SetViewportHint(800, 600)
	p := r.file("bad", 2400, 1800, "scale", "scalebad")
	r.loader.Want([]Request{req(p, PrioCurrent)})
	r.waitState(p, StateReady)
	if got := fmt.Sprint(r.states["bad.big"]); got != "[1 2]" {
		t.Fatalf("states %s", got)
	}
	if calls := r.big.optionsOf("bad"); len(calls) != 2 {
		t.Fatalf("%+v", calls)
	}
}

func TestLoaderPreviewFailureFallsBackToFullDecode(t *testing.T) {
	r := newBigRig(t, 1, previewLimits(), false)
	r.loader.SetViewportHint(800, 600)
	p := r.file("flaky", 2400, 1800, "scale", "scalefail")
	r.loader.Want([]Request{req(p, PrioCurrent)})
	e := r.waitState(p, StateReady)
	if calls := r.big.optionsOf("flaky"); len(calls) != 2 || e.Img.Shift != 0 {
		t.Fatalf("%+v", calls)
	}
	q := r.file("dead", 2400, 1800, "scale", "decfail")
	r.loader.Want([]Request{req(q, PrioCurrent)})
	f := r.waitState(q, StateFailed)
	if !errors.Is(f.Err, errBoom) {
		t.Fatalf("err %v", f.Err)
	}
}

func TestLoaderPreviewCancelledWhenFileChanges(t *testing.T) {
	base := runtime.NumGoroutine()
	r := newBigRig(t, 2, previewLimits(), false)
	r.loader.SetViewportHint(800, 600)
	r.big.setGate("slowfull")
	p := r.file("slowfull", 2400, 1800, "scale")
	other := r.file("other", 100, 100)
	r.loader.Want([]Request{req(p, PrioCurrent)})
	r.waitState(p, StatePreview)
	r.loader.Want([]Request{req(other, PrioCurrent)})
	_, cancelled := r.big.gateFor("slowfull")
	select {
	case <-cancelled:
	case <-time.After(5 * time.Second):
		t.Fatal("the full decode did not observe the cancellation")
	}
	r.waitState(other, StateReady)
	if _, ok := r.loader.Entry(p); ok {
		t.Fatal("entry of the abandoned file must be dropped")
	}
	if r.cache.Len() != 1 {
		t.Fatalf("cache %d", r.cache.Len())
	}
	r.loader.Close()
	if n := settledGoroutines(base); n > base {
		t.Fatalf("goroutines %d, before %d", n, base)
	}
}

func TestLoaderPreviewNeverRegressesReadyEntry(t *testing.T) {
	r := newBigRig(t, 1, previewLimits(), false)
	p := r.file("x", 10, 10)
	r.loader.Want([]Request{req(p, PrioCurrent)})
	e := r.waitState(p, StateReady)
	j := &job{path: p, ctx: r.loader.ctx}
	r.loader.slots[p].job = j
	r.loader.apply(j, StatePreview, e.Info, &Decoded{Reduced: true}, nil)
	if got, _ := r.loader.Entry(p); got.State != StateReady || got.Img != e.Img {
		t.Fatalf("a late preview replaced the final image: %+v", got)
	}
}

func TestLoaderViewportHintValidation(t *testing.T) {
	r := newBigRig(t, 1, previewLimits(), false)
	if r.loader.viewW.Load() != defaultViewW || r.loader.viewH.Load() != defaultViewH {
		t.Fatal("default hint must be 1920x1080")
	}
	r.loader.SetViewportHint(0, 100)
	r.loader.SetViewportHint(100, -5)
	if r.loader.viewW.Load() != defaultViewW {
		t.Fatal("invalid hints must be ignored")
	}
	r.loader.SetViewportHint(1000, 700)
	if r.loader.viewW.Load() != 1000 || r.loader.viewH.Load() != 700 {
		t.Fatal("hint not stored")
	}
	r.loader.SetTileBudget(-1)
	if r.loader.tileBudget.Load() != DefaultTileBudget {
		t.Fatal("invalid budget must be ignored")
	}
	r.loader.SetTileBudget(64 << 20)
	if r.loader.tileBudget.Load() != 64<<20 {
		t.Fatal("budget not stored")
	}
}

func TestLoaderTileBudgetAndWorkersReachTheManager(t *testing.T) {
	r := newBigRig(t, 4, tiledLimits(), true)
	r.loader.SetTileBudget(20 << 20)
	a := r.file("a", 20000, 15000)
	r.loader.Want([]Request{req(a, PrioCurrent)})
	d := r.waitState(a, StateReady).Img
	m := d.Detail.(*TileManager)
	if m.budget != 20<<20 || m.workers != 2 {
		t.Fatalf("budget %d workers %d", m.budget, m.workers)
	}
	if r.loader.tileWorkerCount() != 2 {
		t.Fatalf("tile workers %d", r.loader.tileWorkerCount())
	}
	r1 := newBigRig(t, 1, tiledLimits(), true)
	if r1.loader.tileWorkerCount() != 1 {
		t.Fatalf("a single decode thread still needs one tile worker, got %d", r1.loader.tileWorkerCount())
	}
	r8 := newBigRig(t, 16, tiledLimits(), true)
	if r8.loader.tileWorkerCount() != MaxTileWorkers {
		t.Fatalf("tile workers %d", r8.loader.tileWorkerCount())
	}
}

func TestLoaderChurnWithTiledFilesUnderRaceDetector(t *testing.T) {
	r := newBigRig(t, 3, tiledLimits(), true)
	r.cache.SetBudget(int64(2500*1875*4) * 3)
	var paths []string
	for i := 0; i < 8; i++ {
		paths = append(paths, r.file(fmt.Sprintf("img%d", i), 20000, 15000))
	}
	for round := 0; round < 30; round++ {
		var reqs []Request
		for i := 0; i < 3; i++ {
			reqs = append(reqs, req(paths[(round+i)%len(paths)], Priority(i)))
		}
		r.loader.Want(reqs)
		r.ui.Drain()
		for _, p := range reqs {
			if e, ok := r.loader.Entry(p.Path); ok && e.Img != nil && e.Img.Detail != nil {
				e.Img.Detail.Window(1, image.Rect(round*10, 0, round*10+800, 600))
			}
		}
		time.Sleep(time.Millisecond)
	}
	last := r.file("last", 20000, 15000)
	r.loader.Want([]Request{req(last, PrioCurrent)})
	r.waitState(last, StateReady)
}

func TestDecodedReleaseIsIdempotentAndSafeOnPlainImages(t *testing.T) {
	var nilDecoded *Decoded
	nilDecoded.Release()
	if nilDecoded.Released() || nilDecoded.FullResolution() {
		t.Fatal("nil decoded")
	}
	plain := &Decoded{Levels: []*image.NRGBA{image.NewNRGBA(image.Rect(0, 0, 3, 3))}}
	plain.Release()
	plain.Release()
	if !plain.Released() || plain.FullResolution() == false {
		t.Fatal("plain image: released flag only")
	}
	closer := &countingCloser{}
	d := &Decoded{Levels: plain.Levels, Detail: &closableDetail{closer: closer}}
	d.Release()
	d.Release()
	if closer.n.Load() != 1 {
		t.Fatalf("closed %d times", closer.n.Load())
	}
}

type closableDetail struct{ closer *countingCloser }

func (c *closableDetail) Close() error { return c.closer.Close() }

func (c *closableDetail) SetOnChange(func()) {}

func (c *closableDetail) Window(int, image.Rectangle) render.Window { return render.Window{} }
