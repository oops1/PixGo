package app

import (
	"fmt"
	"image"
	"image/color"
	stdgif "image/gif"
	"image/png"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/engine"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/gif"
	pngcodec "github.com/oops1/PixGo/internal/codec/png"
	"github.com/oops1/PixGo/internal/settings"
	"github.com/oops1/PixGo/internal/ui/view"
)

type harness struct {
	t      *testing.T
	c      *Controller
	canvas *view.Canvas
	mu     sync.Mutex
	queue  []func()
	timers []*fakeTimer
	now    time.Time
	events int
}

type fakeTimer struct {
	at      time.Time
	fn      func()
	stopped bool
}

func newHarness(t *testing.T, mutate func(*settings.Settings)) *harness {
	t.Helper()
	st := settings.Defaults()
	if mutate != nil {
		mutate(&st)
	}
	h := &harness{t: t, now: time.Unix(5000, 0)}
	eng := engine.New(400, 300, 30)
	h.canvas = view.New(eng)
	eng.SetRootFullCanvas(h.canvas)
	reg := codec.NewRegistry()
	reg.RegisterDecoder(pngcodec.Decoder{})
	reg.RegisterDecoder(gif.Decoder{})
	h.c = NewController(Env{
		Registry: reg,
		Settings: settings.NewMemory(st),
		Post: func(f func()) {
			h.mu.Lock()
			h.queue = append(h.queue, f)
			h.mu.Unlock()
		},
		After: func(d time.Duration, fn func()) func() {
			tm := &fakeTimer{at: h.now.Add(d), fn: fn}
			h.timers = append(h.timers, tm)
			return func() { tm.stopped = true }
		},
		Invalidate: func() {},
		Now:        func() time.Time { return h.now },
	}, h.canvas)
	h.c.OnChange = func() { h.events++ }
	t.Cleanup(h.c.Close)
	eng.Flush()
	eng.RenderOnce()
	return h
}

func (h *harness) pump(until func() bool) {
	h.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		h.mu.Lock()
		q := h.queue
		h.queue = nil
		h.mu.Unlock()
		for _, f := range q {
			f()
		}
		if until() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	h.t.Fatalf("condition not reached; status=%d err=%v path=%s", h.c.Status(), h.c.Err(), h.c.Path())
}

func (h *harness) advance(d time.Duration) {
	h.now = h.now.Add(d)
	for _, tm := range append([]*fakeTimer(nil), h.timers...) {
		if !tm.stopped && !tm.at.After(h.now) {
			tm.stopped = true
			tm.fn()
		}
	}
}

func (h *harness) pendingTimers() int {
	n := 0
	for _, tm := range h.timers {
		if !tm.stopped {
			n++
		}
	}
	return n
}

func writePNG(t *testing.T, path string, w, hgt int, c color.NRGBA) {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, hgt))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, 255
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

func makeFolder(t *testing.T, n int) (string, []string) {
	t.Helper()
	dir := t.TempDir()
	var paths []string
	for i := 0; i < n; i++ {
		p := filepath.Join(dir, fmt.Sprintf("img%02d.png", i+1))
		writePNG(t, p, 40+i*10, 30+i*5, color.NRGBA{uint8(40 * i), 100, 200, 255})
		paths = append(paths, p)
	}
	return dir, paths
}

func TestOpenShowsImageAndTitle(t *testing.T) {
	h := newHarness(t, nil)
	_, paths := makeFolder(t, 3)
	if err := h.c.Open(paths[1]); err != nil {
		t.Fatal(err)
	}
	if i, n := h.c.Position(); i != 2 || n != 3 {
		t.Fatalf("position %d/%d, want 2/3", i, n)
	}
	h.pump(func() bool { return h.c.Status() == StatusReady })
	if !h.canvas.HasImage() {
		t.Fatal("canvas has no image")
	}
	if got, want := h.c.Title(), "img02.png — 50 × 35"; got != want {
		t.Errorf("title = %q, want %q", got, want)
	}
}

func TestStepNavigatesAndPreloadsNeighbours(t *testing.T) {
	h := newHarness(t, nil)
	_, paths := makeFolder(t, 5)
	h.c.Open(paths[2])
	h.pump(func() bool { return h.c.Status() == StatusReady })
	for _, p := range []string{paths[1], paths[3]} {
		p := p
		h.pump(func() bool {
			e, ok := h.c.loader.Entry(p)
			return ok && e.State == 2
		})
	}
	if !h.c.Step(1) {
		t.Fatal("step to next refused")
	}
	if h.c.Name() != "img04.png" {
		t.Fatalf("current = %s", h.c.Name())
	}
	h.pump(func() bool { return h.c.Status() == StatusReady && h.c.Title() == "img04.png — 70 × 45" })
	if h.c.Step(-1) && h.c.Step(-1) && h.c.Step(-1) && h.c.Step(-1) {
		t.Fatal("stepped past the first image without looping")
	}
	if h.c.Name() != "img01.png" {
		t.Fatalf("expected to stop at the first image, at %s", h.c.Name())
	}
	if !h.c.Last() || h.c.Name() != "img05.png" {
		t.Fatalf("Last() -> %s", h.c.Name())
	}
}

func TestLoopSettingWraps(t *testing.T) {
	h := newHarness(t, func(s *settings.Settings) { s.LoopFolder = true })
	_, paths := makeFolder(t, 3)
	h.c.Open(paths[2])
	h.pump(func() bool { return h.c.Status() == StatusReady })
	if !h.c.Step(1) || h.c.Name() != "img01.png" {
		t.Fatalf("loop did not wrap: %s", h.c.Name())
	}
}

func TestCorruptFileReportsFailure(t *testing.T) {
	h := newHarness(t, nil)
	dir, paths := makeFolder(t, 2)
	bad := filepath.Join(dir, "img03.png")
	os.WriteFile(bad, append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 40)...), 0o644)
	h.c.Open(paths[1])
	h.pump(func() bool { return h.c.Status() == StatusReady })
	h.c.Step(1)
	h.pump(func() bool { return h.c.Status() == StatusFailed })
	if h.c.Err() == nil {
		t.Error("no error for a corrupt file")
	}
	if h.canvas.HasImage() {
		t.Error("canvas still shows an image after a failed load")
	}
	if !h.c.Step(-1) {
		t.Fatal("cannot leave a failed image")
	}
	h.pump(func() bool { return h.c.Status() == StatusReady })
	if !h.canvas.HasImage() {
		t.Error("image not restored after leaving the corrupt file")
	}
}

func TestInitialZoomFromSettings(t *testing.T) {
	h := newHarness(t, func(s *settings.Settings) { s.InitialZoom = settings.ZoomActual })
	_, paths := makeFolder(t, 1)
	h.c.Open(paths[0])
	h.pump(func() bool { return h.c.Status() == StatusReady })
	if got := h.canvas.ZoomPercent(); got != 100 {
		t.Errorf("zoom = %d%%, want 100%%", got)
	}
}

func TestEmptyFolder(t *testing.T) {
	h := newHarness(t, nil)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o644)
	if err := h.c.Open(dir); err != ErrNothingToShow {
		t.Fatalf("err = %v, want ErrNothingToShow", err)
	}
	if h.c.Status() != StatusEmpty {
		t.Errorf("status = %d", h.c.Status())
	}
}

func TestFavoriteToggle(t *testing.T) {
	h := newHarness(t, nil)
	_, paths := makeFolder(t, 1)
	h.c.Open(paths[0])
	if h.c.IsFavorite() {
		t.Fatal("favorite before toggle")
	}
	if !h.c.ToggleFavorite() || !h.c.IsFavorite() {
		t.Fatal("toggle did not add")
	}
	if h.c.ToggleFavorite() || h.c.IsFavorite() {
		t.Fatal("second toggle did not remove")
	}
}

func writeGIF(t *testing.T, path string, frames int, delayCS int, loop int) {
	t.Helper()
	pal := []color.Color{color.Black, color.White, color.RGBA{255, 0, 0, 255}}
	g := &stdgif.GIF{LoopCount: loop}
	for i := 0; i < frames; i++ {
		im := image.NewPaletted(image.Rect(0, 0, 8, 8), pal)
		for p := range im.Pix {
			im.Pix[p] = uint8(i % 3)
		}
		g.Image = append(g.Image, im)
		g.Delay = append(g.Delay, delayCS)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := stdgif.EncodeAll(f, g); err != nil {
		t.Fatal(err)
	}
}

func TestAnimationAdvancesAndRespectsFrameRateCap(t *testing.T) {
	h := newHarness(t, func(s *settings.Settings) { s.AnimationFPS = 10 })
	dir := t.TempDir()
	p := filepath.Join(dir, "anim.gif")
	writeGIF(t, p, 3, 2, 0)
	h.c.Open(p)
	h.pump(func() bool { return h.c.Status() == StatusReady })
	if h.pendingTimers() != 1 {
		t.Fatalf("pending timers = %d, want 1", h.pendingTimers())
	}
	h.advance(50 * time.Millisecond)
	if h.c.anim.frame != 0 {
		t.Errorf("frame advanced after 50 ms although the cap is 100 ms")
	}
	h.advance(60 * time.Millisecond)
	if h.c.anim.frame != 1 {
		t.Errorf("frame = %d after 110 ms, want 1", h.c.anim.frame)
	}
	h.advance(100 * time.Millisecond)
	h.advance(100 * time.Millisecond)
	if h.c.anim.frame != 0 {
		t.Errorf("animation did not wrap to frame 0, at %d", h.c.anim.frame)
	}
}

func TestAnimationPausesWhenInactive(t *testing.T) {
	h := newHarness(t, nil)
	dir := t.TempDir()
	p := filepath.Join(dir, "anim.gif")
	writeGIF(t, p, 3, 10, 0)
	h.c.Open(p)
	h.pump(func() bool { return h.c.Status() == StatusReady })
	h.c.SetActive(false)
	if h.pendingTimers() != 0 {
		t.Fatalf("timers pending while inactive: %d", h.pendingTimers())
	}
	h.advance(10 * time.Second)
	if h.c.anim.frame != 0 {
		t.Errorf("animation ran while inactive")
	}
	h.c.SetActive(true)
	if h.pendingTimers() != 1 {
		t.Errorf("animation did not resume: timers=%d", h.pendingTimers())
	}
}

func TestNavigatingAwayStopsAnimation(t *testing.T) {
	h := newHarness(t, nil)
	dir := t.TempDir()
	writeGIF(t, filepath.Join(dir, "a.gif"), 3, 10, 0)
	writePNG(t, filepath.Join(dir, "b.png"), 20, 20, color.NRGBA{1, 2, 3, 255})
	h.c.Open(filepath.Join(dir, "a.gif"))
	h.pump(func() bool { return h.c.Status() == StatusReady })
	h.c.Step(1)
	h.pump(func() bool { return h.c.Status() == StatusReady && h.c.Name() == "b.png" })
	if h.pendingTimers() != 0 {
		t.Errorf("animation timer survived navigation")
	}
}

func TestRescanPicksUpNewFile(t *testing.T) {
	h := newHarness(t, nil)
	dir, paths := makeFolder(t, 2)
	h.c.Open(paths[0])
	h.pump(func() bool { return h.c.Status() == StatusReady })
	writePNG(t, filepath.Join(dir, "img99.png"), 10, 10, color.NRGBA{9, 9, 9, 255})
	h.c.RescanAsync()
	h.pump(func() bool { _, n := h.c.Position(); return n == 3 })
	if h.c.Name() != "img01.png" {
		t.Errorf("rescan changed the current image to %s", h.c.Name())
	}
}

func TestOpenMissingPathShowsFailure(t *testing.T) {
	h := newHarness(t, nil)
	missing := filepath.Join(t.TempDir(), "gone.png")
	if err := h.c.Open(missing); err == nil {
		t.Fatal("expected an error for a missing file")
	}
	if h.c.Status() != StatusFailed || h.c.Err() == nil {
		t.Errorf("status=%d err=%v", h.c.Status(), h.c.Err())
	}
	if h.c.Name() != "gone.png" {
		t.Errorf("name = %q", h.c.Name())
	}
	if h.canvas.HasImage() {
		t.Error("canvas shows an image for a missing file")
	}
}

func TestOpenUnsupportedFileShowsFailure(t *testing.T) {
	h := newHarness(t, nil)
	p := filepath.Join(t.TempDir(), "notes.txt")
	os.WriteFile(p, []byte("not an image at all"), 0o644)
	if err := h.c.Open(p); err != nil {
		t.Fatalf("open: %v", err)
	}
	h.pump(func() bool { return h.c.Status() == StatusFailed })
	if h.c.Err() == nil {
		t.Error("no error for an unsupported file")
	}
}
