package app

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/settings"
)

type bigFake struct {
	mu      sync.Mutex
	scale   bool
	decodes []codec.Options
	gate    chan struct{}
	readers []*bigReader
}

type bigReader struct {
	info   codec.Info
	closed atomic.Int32
	owner  *bigFake
}

func (f *bigFake) ID() codec.ID           { return "bigfake" }
func (f *bigFake) Name() string           { return "Big fake" }
func (f *bigFake) Extensions() []string   { return []string{".bigf"} }
func (f *bigFake) Sniff(head []byte) bool { return bytes.HasPrefix(head, []byte("BIGF ")) }

func (f *bigFake) dims(r io.ReaderAt, size int64) (int, int) {
	buf := make([]byte, size)
	r.ReadAt(buf, 0)
	parts := strings.SplitN(strings.TrimSpace(strings.TrimPrefix(string(buf), "BIGF ")), "x", 2)
	w, _ := strconv.Atoi(parts[0])
	h, _ := strconv.Atoi(parts[1])
	return w, h
}

func (f *bigFake) infoOf(w, h int) codec.Info {
	return codec.Info{Format: "bigfake", FormatName: "Big fake", Width: w, Height: h, Frames: 1, Orientation: codec.OrientNormal}
}

func (f *bigFake) Probe(ctx context.Context, r io.ReaderAt, size int64, lim codec.Limits) (codec.Info, error) {
	w, h := f.dims(r, size)
	info := f.infoOf(w, h)
	px := int64(w) * int64(h)
	switch {
	case px > lim.MaxTiledPixels:
		return codec.Info{}, codec.ErrTooLarge
	case px > lim.MaxPixels:
		return info, codec.ErrTooLarge
	}
	return info, nil
}

func opaque(img *image.NRGBA) *image.NRGBA {
	for i := 3; i < len(img.Pix); i += 4 {
		img.Pix[i] = 255
	}
	return img
}

func (f *bigFake) Decode(ctx context.Context, r io.ReaderAt, size int64, o codec.Options) (*codec.Image, error) {
	w, h := f.dims(r, size)
	f.mu.Lock()
	f.decodes = append(f.decodes, o)
	gate := f.gate
	f.mu.Unlock()
	scale := 1
	if f.scale && o.Scale > 1 {
		scale = o.Scale
	} else if gate != nil {
		select {
		case <-gate:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	dw, dh := w/scale, h/scale
	return &codec.Image{Info: f.infoOf(w, h), Frames: []codec.Frame{{Pix: opaque(image.NewNRGBA(image.Rect(0, 0, dw, dh)))}}, Scale: scale}, nil
}

func (f *bigFake) OpenTiled(ctx context.Context, r io.ReaderAt, size int64, lim codec.Limits) (codec.TileReader, error) {
	w, h := f.dims(r, size)
	rd := &bigReader{info: f.infoOf(w, h), owner: f}
	f.mu.Lock()
	f.readers = append(f.readers, rd)
	f.mu.Unlock()
	return rd, nil
}

func (f *bigFake) openReaders() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, r := range f.readers {
		if r.closed.Load() == 0 {
			n++
		}
	}
	return n
}

func (f *bigFake) decodeCalls() []codec.Options {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]codec.Options(nil), f.decodes...)
}

func (r *bigReader) Info() codec.Info { return r.info }
func (r *bigReader) MinShift() int    { return 0 }
func (r *bigReader) Close() error {
	r.closed.Add(1)
	return nil
}

func (r *bigReader) Overview(ctx context.Context, shift int) (*image.NRGBA, error) {
	w, h := codec.LevelSize(r.info.Width, r.info.Height, shift)
	return opaque(image.NewNRGBA(image.Rect(0, 0, w, h))), nil
}

func (r *bigReader) Region(ctx context.Context, shift int, rc image.Rectangle) (*image.NRGBA, error) {
	return opaque(image.NewNRGBA(image.Rect(0, 0, rc.Dx(), rc.Dy()))), nil
}

func writeBig(t *testing.T, dir, name string, w, h int) string {
	t.Helper()
	p := filepath.Join(dir, name+".bigf")
	if err := os.WriteFile(p, []byte(fmt.Sprintf("BIGF %dx%d", w, h)), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestControllerOpensHugeFileThroughTiles(t *testing.T) {
	fake := &bigFake{}
	h := newHarnessWith(t, func(s *settings.Settings) { s.MaxPixels = 100; s.Preload = 0 }, fake)
	dir := t.TempDir()
	p := writeBig(t, dir, "huge", 20000, 15000)
	if err := h.c.Open(p); err != nil {
		t.Fatal(err)
	}
	h.pump(func() bool { return h.c.Status() == StatusReady })
	d := h.c.Shown()
	if d == nil || d.Shift != 3 || d.Detail == nil || d.Reduced {
		t.Fatalf("shown %+v", d)
	}
	if h.c.Decoded() != nil {
		t.Fatal("a tiled picture is not a full resolution frame, copy and save must not use it")
	}
	f := h.canvas.Frame()
	if f.ImgW != 20000 || f.ImgH != 15000 {
		t.Fatalf("canvas frame %+v", f)
	}
	if got, want := h.c.Title(), "huge.bigf — 20000 × 15000"; got != want {
		t.Fatalf("title %q, want %q", got, want)
	}
	if len(fake.decodeCalls()) != 0 {
		t.Fatal("a file above the pixel limit must not be decoded as a whole")
	}
	h.canvas.Actual()
	h.canvas.SetBackground(h.canvas.Background)
	if h.canvas.ZoomPercent() != 100 {
		t.Fatalf("zoom %d", h.canvas.ZoomPercent())
	}
}

func TestControllerShownTiledImageIsPinnedUntilReplaced(t *testing.T) {
	fake := &bigFake{}
	h := newHarnessWith(t, func(s *settings.Settings) { s.MaxPixels = 100; s.Preload = 0; s.CacheMB = 256 }, fake)
	dir := t.TempDir()
	a := writeBig(t, dir, "a", 20000, 15000)
	b := writeBig(t, dir, "b", 20000, 15000)
	h.c.cache.SetBudget(1)
	if err := h.c.Open(a); err != nil {
		t.Fatal(err)
	}
	h.pump(func() bool { return h.c.Status() == StatusReady })
	first := h.c.Shown()
	h.c.cache.SetBudget(1)
	if first.Released() {
		t.Fatal("the displayed image must survive budget pressure")
	}
	h.c.Step(1)
	h.pump(func() bool { return h.c.Status() == StatusReady && h.c.Name() == "b.bigf" })
	_ = b
	if !first.Released() {
		t.Fatal("the image that was left must be released once nothing shows it")
	}
	if h.c.Shown().Released() {
		t.Fatal("the new image must stay open")
	}
	if fake.openReaders() != 1 {
		t.Fatalf("open sessions %d, want 1", fake.openReaders())
	}
	h.c.Close()
	if fake.openReaders() != 0 {
		t.Fatalf("Close must release every session, %d left", fake.openReaders())
	}
}

func TestControllerRemovedFileReleasesItsSession(t *testing.T) {
	fake := &bigFake{}
	h := newHarnessWith(t, func(s *settings.Settings) { s.MaxPixels = 100; s.Preload = 0 }, fake)
	dir := t.TempDir()
	a := writeBig(t, dir, "a", 20000, 15000)
	writeBig(t, dir, "b", 20000, 15000)
	h.open(a)
	h.pump(func() bool { return h.c.Status() == StatusReady })
	if err := os.Remove(a); err != nil {
		t.Fatalf("the open session must not block deleting the file: %v", err)
	}
	h.c.Removed(a)
	h.pump(func() bool { return h.c.Status() == StatusReady && h.c.Name() == "b.bigf" })
	if fake.openReaders() != 1 {
		t.Fatalf("open sessions %d, want only the new one", fake.openReaders())
	}
}

func TestControllerPreviewThenFinalKeepsTheView(t *testing.T) {
	fake := &bigFake{scale: true, gate: make(chan struct{})}
	h := newHarnessWith(t, func(s *settings.Settings) { s.MaxPixels = 100; s.Preload = 0 }, fake)
	dir := t.TempDir()
	p := writeBig(t, dir, "photo", 2400, 1800)
	if err := h.c.Open(p); err != nil {
		t.Fatal(err)
	}
	h.pump(func() bool { return h.c.Shown() != nil && h.c.Shown().Reduced })
	if h.c.Status() != StatusLoading {
		t.Fatalf("the loading state must last until the final image, got %d", h.c.Status())
	}
	if !h.canvas.HasImage() {
		t.Fatal("the preview must be on screen")
	}
	if h.c.Decoded() != nil {
		t.Fatal("a preview is not a full resolution frame")
	}
	if calls := fake.decodeCalls(); len(calls) < 1 || calls[0].Scale != 4 {
		t.Fatalf("the preview must be decoded at 1/4 for a 400x300 view of a 2400x1800 picture: %+v", calls)
	}
	f := h.canvas.Frame()
	if f.ImgW != 2400 || f.ImgH != 1800 {
		t.Fatalf("preview frame %+v", f)
	}
	h.canvas.Actual()
	h.canvas.View().CenterU, h.canvas.View().CenterV = 700, 500
	viewBefore := *h.canvas.View()
	preview := h.c.Shown()
	close(fake.gate)
	h.pump(func() bool { return h.c.Status() == StatusReady })
	final := h.c.Shown()
	if final == preview || final.Reduced || final.Shift != 0 {
		t.Fatalf("final %+v", final)
	}
	if *h.canvas.View() != viewBefore {
		t.Fatalf("view changed when the final image replaced the preview: %+v -> %+v", viewBefore, *h.canvas.View())
	}
	if h.c.Decoded() != final {
		t.Fatal("the final image is a full resolution frame")
	}
	if h.canvas.Frame().ImgW != 2400 {
		t.Fatalf("final frame %+v", h.canvas.Frame())
	}
}

func TestControllerPreviewIsNotPinnedButFinalIs(t *testing.T) {
	fake := &bigFake{scale: true, gate: make(chan struct{})}
	h := newHarnessWith(t, func(s *settings.Settings) { s.MaxPixels = 100; s.Preload = 0 }, fake)
	dir := t.TempDir()
	p := writeBig(t, dir, "photo", 2400, 1800)
	h.c.Open(p)
	h.pump(func() bool { return h.c.Shown() != nil && h.c.Shown().Reduced })
	if h.c.pinned {
		t.Fatal("a preview is not in the cache and must not be pinned")
	}
	close(fake.gate)
	h.pump(func() bool { return h.c.Status() == StatusReady })
	if !h.c.pinned {
		t.Fatal("the final image must be pinned")
	}
}

func TestControllerViewportHintComesFromTheCanvas(t *testing.T) {
	fake := &bigFake{scale: true}
	h := newHarnessWith(t, func(s *settings.Settings) { s.MaxPixels = 100; s.Preload = 0 }, fake)
	dir := t.TempDir()
	p := writeBig(t, dir, "photo", 6000, 3000)
	h.open(p)
	h.pump(func() bool { return h.c.Status() == StatusReady })
	calls := fake.decodeCalls()
	if len(calls) != 2 || calls[0].Scale != 8 {
		t.Fatalf("a 400x300 canvas needs only 1/8 of a 6000x3000 picture: %+v", calls)
	}
}

func TestControllerLimitsFollowSettings(t *testing.T) {
	h := newHarness(t, func(s *settings.Settings) { s.MaxPixels = 80; s.CacheMB = 512 })
	l := h.c.limits()
	if l.MaxPixels != 80_000_000 {
		t.Fatalf("MaxPixels %d", l.MaxPixels)
	}
	if want := int64(512<<20) / 16; l.TiledAbove != want {
		t.Fatalf("TiledAbove %d, want %d", l.TiledAbove, want)
	}
	if l.MaxTiledPixels != codec.DefaultLimits().MaxTiledPixels {
		t.Fatalf("MaxTiledPixels %d", l.MaxTiledPixels)
	}
	h.c.env.Settings.Update(func(s *settings.Settings) { s.CacheMB = 8192; s.MaxPixels = 20 })
	l = h.c.limits()
	if l.TiledAbove != 20_000_000 {
		t.Fatalf("TiledAbove must never exceed MaxPixels: %d", l.TiledAbove)
	}
	h.c.env.Settings.Update(func(s *settings.Settings) { s.MaxPixels = 1000 })
	l = h.c.limits()
	if l.MaxPixels != 1_000_000_000 || l.MaxTiledPixels < l.MaxPixels {
		t.Fatalf("tiled limit %d must cover MaxPixels %d", l.MaxTiledPixels, l.MaxPixels)
	}
}

func TestTileBudgetFollowsCache(t *testing.T) {
	cases := map[int64]int64{
		64 << 20:   32 << 20,
		256 << 20:  64 << 20,
		384 << 20:  96 << 20,
		512 << 20:  96 << 20,
		4096 << 20: 96 << 20,
	}
	for cache, want := range cases {
		if got := tileBudgetFor(cache); got != want {
			t.Errorf("cache %d MB: tile budget %d MB, want %d MB", cache>>20, got>>20, want>>20)
		}
	}
}

func TestControllerOrdinaryImagesStayUntouched(t *testing.T) {
	h := newHarness(t, nil)
	_, paths := makeFolder(t, 2)
	h.open(paths[0])
	h.pump(func() bool { return h.c.Status() == StatusReady })
	d := h.c.Shown()
	if d == nil || d.Shift != 0 || d.Detail != nil || d.Reduced {
		t.Fatalf("shown %+v", d)
	}
	if h.c.Decoded() != d {
		t.Fatal("Decoded must return an ordinary image as before")
	}
}
