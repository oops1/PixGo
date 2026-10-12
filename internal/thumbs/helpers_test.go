package thumbs

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	stdjpeg "image/jpeg"
	stdpng "image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/jpeg"
	"github.com/oops1/PixGo/internal/codec/png"
)

type uiQueue struct {
	mu sync.Mutex
	q  []func()
}

func (u *uiQueue) post(fn func()) {
	u.mu.Lock()
	u.q = append(u.q, fn)
	u.mu.Unlock()
}

func (u *uiQueue) drain() int {
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

type fakeClock struct {
	ns atomic.Int64
}

func newFakeClock() *fakeClock {
	c := &fakeClock{}
	c.ns.Store(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC).UnixNano())
	return c
}

func (c *fakeClock) now() time.Time { return time.Unix(0, c.ns.Load()) }

func (c *fakeClock) advance(d time.Duration) { c.ns.Add(int64(d)) }

type harness struct {
	t     *testing.T
	svc   *Service
	ui    *uiQueue
	clock *fakeClock
	dir   string
}

func newHarness(t *testing.T, reg *codec.Registry, opt Options) *harness {
	t.Helper()
	h := &harness{t: t, ui: &uiQueue{}, clock: newFakeClock(), dir: t.TempDir()}
	if opt.Workers == 0 {
		opt.Workers = 2
	}
	opt.now = h.clock.now
	h.svc = New(reg, h.ui.post, opt)
	t.Cleanup(h.svc.Close)
	return h
}

func (h *harness) waitUntil(cond func() bool) {
	h.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		h.ui.drain()
		if cond() {
			return
		}
		if time.Now().After(deadline) {
			h.t.Fatal("condition not reached in time")
		}
		time.Sleep(time.Millisecond)
	}
}

func (h *harness) get(path string, side int) Thumb {
	h.t.Helper()
	if t, ok := h.svc.Request(path, side, true); ok {
		return t
	}
	var got Thumb
	h.waitUntil(func() bool {
		t, ok := h.svc.Request(path, side, true)
		got = t
		return ok
	})
	return got
}

func (h *harness) write(name string, data []byte) string {
	h.t.Helper()
	p := filepath.Join(h.dir, name)
	if err := os.WriteFile(p, data, 0o600); err != nil {
		h.t.Fatal(err)
	}
	return p
}

func (h *harness) touch(path string, d time.Duration) {
	h.t.Helper()
	st, err := os.Stat(path)
	if err != nil {
		h.t.Fatal(err)
	}
	mt := st.ModTime().Add(d)
	if err := os.Chtimes(path, mt, mt); err != nil {
		h.t.Fatal(err)
	}
	h.clock.advance(3 * statTTL)
}

func solid(w, h int, c color.NRGBA) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, c.A
	}
	return img
}

func jpegBytes(t testing.TB, img image.Image, quality int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := stdjpeg.Encode(&buf, img, &stdjpeg.Options{Quality: quality}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

var (
	colRed   = color.NRGBA{R: 230, G: 20, B: 20, A: 255}
	colGreen = color.NRGBA{R: 20, G: 200, B: 20, A: 255}
	colBlue  = color.NRGBA{R: 20, G: 20, B: 230, A: 255}
	colWhite = color.NRGBA{R: 240, G: 240, B: 240, A: 255}
)

func quadrantImage(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var c color.NRGBA
			switch {
			case x < w/2 && y < h/2:
				c = colRed
			case x >= w/2 && y < h/2:
				c = colGreen
			case x < w/2:
				c = colBlue
			default:
				c = colWhite
			}
			img.SetNRGBA(x, y, c)
		}
	}
	return img
}

func tiffWithOrientation(bigEndian bool, orientation uint16, thumb []byte) []byte {
	var order binary.AppendByteOrder = binary.LittleEndian
	out := []byte{'I', 'I'}
	if bigEndian {
		order = binary.BigEndian
		out = []byte{'M', 'M'}
	}
	out = order.AppendUint16(out, 42)
	out = order.AppendUint32(out, 8)
	entries := 0
	if orientation != 0 {
		entries = 1
	}
	out = order.AppendUint16(out, uint16(entries))
	if orientation != 0 {
		out = order.AppendUint16(out, 0x0112)
		out = order.AppendUint16(out, 3)
		out = order.AppendUint32(out, 1)
		out = order.AppendUint16(out, orientation)
		out = order.AppendUint16(out, 0)
	}
	if thumb == nil {
		return order.AppendUint32(out, 0)
	}
	out = order.AppendUint32(out, uint32(len(out)+4))
	ifdLen := 2 + 3*12 + 4
	dataStart := len(out) + ifdLen
	out = order.AppendUint16(out, 3)
	out = order.AppendUint16(out, 0x0103)
	out = order.AppendUint16(out, 3)
	out = order.AppendUint32(out, 1)
	out = order.AppendUint16(out, 6)
	out = order.AppendUint16(out, 0)
	out = order.AppendUint16(out, 0x0201)
	out = order.AppendUint16(out, 4)
	out = order.AppendUint32(out, 1)
	out = order.AppendUint32(out, uint32(dataStart))
	out = order.AppendUint16(out, 0x0202)
	out = order.AppendUint16(out, 4)
	out = order.AppendUint32(out, 1)
	out = order.AppendUint32(out, uint32(len(thumb)))
	out = order.AppendUint32(out, 0)
	return append(out, thumb...)
}

func injectEXIF(jpegData, tiff []byte) []byte {
	payload := append([]byte("Exif\x00\x00"), tiff...)
	seg := []byte{0xff, 0xe1, byte((len(payload) + 2) >> 8), byte(len(payload) + 2)}
	seg = append(seg, payload...)
	out := make([]byte, 0, len(jpegData)+len(seg))
	out = append(out, jpegData[:2]...)
	out = append(out, seg...)
	return append(out, jpegData[2:]...)
}

type countingJPEG struct {
	jpeg.Decoder
	probes  atomic.Int32
	decodes atomic.Int32
	opts    atomic.Pointer[codec.Options]
	idOver  codec.ID
}

func (c *countingJPEG) ID() codec.ID {
	if c.idOver != "" {
		return c.idOver
	}
	return codec.JPEG
}

func (c *countingJPEG) Probe(ctx context.Context, r io.ReaderAt, size int64, lim codec.Limits) (codec.Info, error) {
	c.probes.Add(1)
	info, err := c.Decoder.Probe(ctx, r, size, lim)
	if c.idOver != "" {
		info.Format = c.idOver
	}
	return info, err
}

func (c *countingJPEG) Decode(ctx context.Context, r io.ReaderAt, size int64, o codec.Options) (*codec.Image, error) {
	c.decodes.Add(1)
	c.opts.Store(&o)
	return c.Decoder.Decode(ctx, r, size, o)
}

func jpegRegistry() (*codec.Registry, *countingJPEG) {
	reg := codec.NewRegistry()
	cj := &countingJPEG{}
	reg.RegisterDecoder(cj)
	reg.RegisterDecoder(png.Decoder{})
	return reg, cj
}

type fakeCodec struct {
	mu         sync.Mutex
	started    []string
	probes     map[string]int
	decodes    map[string]int
	opts       map[string]codec.Options
	gates      map[string]chan struct{}
	cancelled  map[string]chan struct{}
	hang       map[string]chan struct{}
	panics     map[string]bool
	width      int
	height     int
	honorScale bool
	orient     codec.Orientation
	order      []string
}

func newFakeCodec() *fakeCodec {
	return &fakeCodec{
		probes:    map[string]int{},
		decodes:   map[string]int{},
		opts:      map[string]codec.Options{},
		gates:     map[string]chan struct{}{},
		cancelled: map[string]chan struct{}{},
		hang:      map[string]chan struct{}{},
		panics:    map[string]bool{},
		width:     600,
		height:    400,
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
	f.mu.Lock()
	f.probes[name]++
	w, h, o := f.width, f.height, f.orient
	f.mu.Unlock()
	return codec.Info{Format: "fake", FormatName: "Fake", Width: w, Height: h, Frames: 1, Orientation: o}, nil
}

func (f *fakeCodec) Decode(ctx context.Context, r io.ReaderAt, size int64, o codec.Options) (*codec.Image, error) {
	name := f.nameOf(r, size)
	f.mu.Lock()
	f.started = append(f.started, name)
	f.decodes[name]++
	f.opts[name] = o
	gate := f.gates[name]
	hang := f.hang[name]
	canceled := f.cancelled[name]
	doPanic := f.panics[name]
	w, h, honor := f.width, f.height, f.honorScale
	f.mu.Unlock()
	if doPanic {
		panic("fake decoder exploded")
	}
	if hang != nil {
		<-hang
	}
	if gate != nil {
		select {
		case <-gate:
		case <-ctx.Done():
			if canceled != nil {
				close(canceled)
			}
			return nil, ctx.Err()
		}
	}
	scale := 1
	if honor && o.Scale > 1 {
		scale = o.Scale
		w, h = (w+scale-1)/scale, (h+scale-1)/scale
	}
	c := color.NRGBA{R: uint8(len(name) * 40), G: 120, B: 200, A: 255}
	f.mu.Lock()
	f.order = append(f.order, name)
	f.mu.Unlock()
	return &codec.Image{
		Info:   codec.Info{Format: "fake", Width: w, Height: h, Frames: 1, Orientation: f.orient},
		Frames: []codec.Frame{{Pix: solid(w, h, c)}},
		Scale:  scale,
	}, nil
}

func (f *fakeCodec) decodeCount(name string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.decodes[name]
}

func (f *fakeCodec) finished() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.order...)
}

func (f *fakeCodec) gate(name string) chan struct{} {
	f.mu.Lock()
	defer f.mu.Unlock()
	ch := make(chan struct{})
	f.gates[name] = ch
	return ch
}

func (f *fakeCodec) watchCancel(name string) chan struct{} {
	f.mu.Lock()
	defer f.mu.Unlock()
	ch := make(chan struct{})
	f.cancelled[name] = ch
	return ch
}

func (f *fakeCodec) hangOn(name string) chan struct{} {
	f.mu.Lock()
	defer f.mu.Unlock()
	ch := make(chan struct{})
	f.hang[name] = ch
	return ch
}

func (f *fakeCodec) waitStarted(t *testing.T, name string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		f.mu.Lock()
		seen := f.decodes[name] > 0
		f.mu.Unlock()
		if seen {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("decoder never started %q", name)
		}
		time.Sleep(time.Millisecond)
	}
}

func fakeRegistry() (*codec.Registry, *fakeCodec) {
	reg := codec.NewRegistry()
	f := newFakeCodec()
	reg.RegisterDecoder(f)
	return reg, f
}

func (h *harness) writeFake(names ...string) []string {
	h.t.Helper()
	paths := make([]string, len(names))
	for i, n := range names {
		paths[i] = h.write(fmt.Sprintf("%s.fake", n), []byte("FAKE "+n))
	}
	return paths
}

func pixelAt(img *image.RGBA, x, y int) color.RGBA {
	return img.RGBAAt(x, y)
}

func near(a, b uint8, tol int) bool {
	d := int(a) - int(b)
	if d < 0 {
		d = -d
	}
	return d <= tol
}

func colorNear(got color.RGBA, want color.NRGBA, tol int) bool {
	return near(got.R, want.R, tol) && near(got.G, want.G, tol) && near(got.B, want.B, tol) && got.A == 255
}

func pngBytes(t testing.TB, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := stdpng.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
