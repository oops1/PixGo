package imaging

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/colorm"
)

type colorCodec struct {
	mu      sync.Mutex
	decodes int
	options []codec.Options
	deepTR  bool
}

func (c *colorCodec) ID() codec.ID           { return "col" }
func (c *colorCodec) Name() string           { return "Col" }
func (c *colorCodec) Extensions() []string   { return []string{".col"} }
func (c *colorCodec) Sniff(head []byte) bool { return bytes.HasPrefix(head, []byte("COL ")) }

func (c *colorCodec) kind(r io.ReaderAt, size int64) string {
	b := make([]byte, size)
	r.ReadAt(b, 0)
	return string(bytes.TrimSpace(bytes.TrimPrefix(b, []byte("COL "))))
}

func (c *colorCodec) Probe(ctx context.Context, r io.ReaderAt, size int64, lim codec.Limits) (codec.Info, error) {
	return codec.Info{Format: "col", FormatName: "Col", Width: 64, Height: 48, Frames: 1, BitDepth: 10}, nil
}

func colorOf(kind string) codec.ColorInfo {
	switch kind {
	case "p3":
		return codec.ColorInfo{CICP: &codec.CICP{Primaries: 12, Transfer: 13, FullRange: true}}
	case "srgb":
		return codec.ColorInfo{CICP: &codec.CICP{Primaries: 1, Transfer: 13, FullRange: true}}
	case "pq":
		return codec.ColorInfo{CICP: &codec.CICP{Primaries: 9, Transfer: 16, FullRange: true}}
	}
	return codec.ColorInfo{}
}

func testPix(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			copy(img.Pix[img.PixOffset(x, y):], []uint8{uint8(40 + x*3), uint8(20 + y*4), uint8(90 + x), 255})
		}
	}
	return img
}

func testDeep(w, h int, v uint16) *image.NRGBA64 {
	img := image.NewNRGBA64(image.Rect(0, 0, w, h))
	for i := 0; i < w*h; i++ {
		for ch := 0; ch < 4; ch++ {
			x := v
			if ch == 3 {
				x = 0xffff
			}
			img.Pix[8*i+2*ch], img.Pix[8*i+2*ch+1] = byte(x>>8), byte(x)
		}
	}
	return img
}

func (c *colorCodec) Decode(ctx context.Context, r io.ReaderAt, size int64, o codec.Options) (*codec.Image, error) {
	kind := c.kind(r, size)
	c.mu.Lock()
	c.decodes++
	c.options = append(c.options, o)
	c.mu.Unlock()
	img := &codec.Image{Info: codec.Info{Format: "col", Width: 64, Height: 48, Frames: 1}, Color: colorOf(kind)}
	if o.Deep && kind == "pq" {
		img.Frames = []codec.Frame{{Deep: testDeep(64, 48, 38054)}}
		return img, nil
	}
	if o.Deep && kind == "ink" {
		img.Frames = []codec.Frame{{CMYK: testInk(64, 48)}}
		return img, nil
	}
	img.Frames = []codec.Frame{{Pix: testPix(64, 48)}}
	return img, nil
}

type colorTiles struct {
	info  codec.Info
	color codec.ColorInfo
	deep  bool
}

func (t *colorTiles) Info() codec.Info { return t.info }
func (t *colorTiles) MinShift() int    { return 0 }
func (t *colorTiles) Close() error     { return nil }
func (t *colorTiles) Color() codec.ColorInfo {
	return t.color
}

func (t *colorTiles) Overview(ctx context.Context, shift int) (*image.NRGBA, error) {
	w, h := codec.LevelSize(t.info.Width, t.info.Height, shift)
	return testPix(w, h), nil
}

func (t *colorTiles) Region(ctx context.Context, shift int, r image.Rectangle) (*image.NRGBA, error) {
	return testPix(r.Dx(), r.Dy()), nil
}

type deepTiles struct{ colorTiles }

func (t *deepTiles) Deep() bool { return true }

func (t *deepTiles) OverviewDeep(ctx context.Context, shift int) (*image.NRGBA64, error) {
	w, h := codec.LevelSize(t.info.Width, t.info.Height, shift)
	return testDeep(w, h, 38054), nil
}

func (t *deepTiles) RegionDeep(ctx context.Context, shift int, r image.Rectangle) (*image.NRGBA64, error) {
	return testDeep(r.Dx(), r.Dy(), 38054), nil
}

func (c *colorCodec) OpenTiled(ctx context.Context, r io.ReaderAt, size int64, lim codec.Limits) (codec.TileReader, error) {
	kind := c.kind(r, size)
	base := colorTiles{info: codec.Info{Format: "col", Width: 64, Height: 48}, color: colorOf(kind)}
	if kind == "pq" {
		return &deepTiles{base}, nil
	}
	return &base, nil
}

type colorRig struct {
	t      *testing.T
	dir    string
	codec  *colorCodec
	ui     *uiQueue
	loader *Loader
	mgr    *colorm.Manager
}

func newColorRig(t *testing.T, enabled bool, tiled bool) *colorRig {
	t.Helper()
	reg := codec.NewRegistry()
	cc := &colorCodec{}
	reg.RegisterDecoder(cc)
	r := &colorRig{t: t, dir: t.TempDir(), codec: cc, ui: &uiQueue{}}
	lim := func() codec.Limits {
		l := codec.DefaultLimits()
		if tiled {
			l.TiledAbove = 100
		}
		return l
	}
	r.loader = NewLoader(reg, NewCache(1<<30), r.ui.Post, 2, lim)
	t.Cleanup(r.loader.Close)
	r.mgr = colorm.NewManager(nil)
	r.mgr.Configure(colorm.Config{Enabled: enabled})
	r.loader.SetColor(r.mgr)
	return r
}

func (r *colorRig) load(kind string) Entry {
	r.t.Helper()
	p := filepath.Join(r.dir, kind+".col")
	if _, err := os.Stat(p); err != nil {
		if err := os.WriteFile(p, []byte("COL "+kind), 0o644); err != nil {
			r.t.Fatal(err)
		}
	}
	r.loader.Want([]Request{{Path: p, Prio: PrioCurrent}})
	rr := &rig{t: r.t, ui: r.ui, loader: r.loader}
	return rr.waitState(p, StateReady)
}

func TestLoaderWithoutManagerKeepsEightBitAsIs(t *testing.T) {
	r := newColorRig(t, false, false)
	r.loader.SetColor(nil)
	e := r.load("p3")
	if r.codec.options[0].Deep {
		t.Fatal("Deep requested without colour management")
	}
	if !bytes.Equal(e.Img.Levels[0].Pix, testPix(64, 48).Pix) {
		t.Fatal("pixels changed")
	}
}

func TestLoaderDisabledManagerKeepsEightBitAsIs(t *testing.T) {
	r := newColorRig(t, false, false)
	e := r.load("p3")
	if r.codec.options[0].Deep || e.Img.Color.Managed {
		t.Fatal("disabled colour management must behave as 1.1")
	}
	if !bytes.Equal(e.Img.Levels[0].Pix, testPix(64, 48).Pix) {
		t.Fatal("pixels changed")
	}
}

func TestLoaderConvertsTaggedEightBit(t *testing.T) {
	r := newColorRig(t, true, false)
	e := r.load("p3")
	want := testPix(64, 48)
	p3, _ := colorm.FromCICP(codec.CICP{Primaries: 12, Transfer: 13})
	tf, err := colorm.NewTransform(p3, colorm.SRGB(), colorm.Options{})
	if err != nil {
		t.Fatal(err)
	}
	tf.Apply(want)
	if !bytes.Equal(e.Img.Levels[0].Pix, want.Pix) || bytes.Equal(want.Pix, testPix(64, 48).Pix) {
		t.Fatal("Display P3 picture was not converted")
	}
	rep := e.Img.Color
	if !rep.Managed || rep.Origin != colorm.OriginCICP || rep.Profile != "Display P3 / sRGB" || !rep.Converted {
		t.Fatalf("report %+v", rep)
	}
	if !r.codec.options[0].Deep {
		t.Fatal("the loader must ask decoders for Deep when colour management is on")
	}
}

func TestLoaderTreatsPlainSRGBCICPAsIdentity(t *testing.T) {
	r := newColorRig(t, true, false)
	e := r.load("srgb")
	if !bytes.Equal(e.Img.Levels[0].Pix, testPix(64, 48).Pix) {
		t.Fatal("sRGB CICP changed the pixels")
	}
	if e.Img.Color.Converted {
		t.Fatalf("%+v", e.Img.Color)
	}
}

func TestLoaderConvertsDeepHDRAndDropsDeep(t *testing.T) {
	r := newColorRig(t, true, false)
	e := r.load("pq")
	f := e.Img.Frames[0]
	if f.Deep != nil || f.Pix == nil {
		t.Fatalf("deep frame left behind: %+v", f)
	}
	rep := e.Img.Color
	if rep.HDR != "PQ" || !rep.ToneMapped || !rep.Deep {
		t.Fatalf("report %+v", rep)
	}
	if px := e.Img.Levels[0].Pix; px[0] < 250 || px[1] < 250 || px[3] != 255 {
		t.Fatalf("203 nit PQ grey became %v", px[:4])
	}
}

func TestLoaderReconvertsWhenColorSettingsChange(t *testing.T) {
	r := newColorRig(t, true, false)
	first := r.load("p3")
	r.mgr.Configure(colorm.Config{Enabled: false})
	r.loader.ColorChanged()
	second := r.load("p3")
	if r.codec.decodes != 2 {
		t.Fatalf("a stale colour stamp must force a new decode, got %d decodes", r.codec.decodes)
	}
	if bytes.Equal(first.Img.Levels[0].Pix, second.Img.Levels[0].Pix) || second.Img.Color.Managed {
		t.Fatal("the second load must be unmanaged")
	}
	r.load("p3")
	if r.codec.decodes != 2 {
		t.Fatalf("an unchanged stamp must hit the cache, got %d decodes", r.codec.decodes)
	}
}

func TestTiledOverviewAndTilesAreConverted(t *testing.T) {
	r := newColorRig(t, true, true)
	e := r.load("p3")
	if e.Img.Detail == nil && e.Img.Shift != 0 {
		t.Fatal("expected a tiled picture")
	}
	want := testPix(64, 48)
	p3, _ := colorm.FromCICP(codec.CICP{Primaries: 12, Transfer: 13})
	tf, _ := colorm.NewTransform(p3, colorm.SRGB(), colorm.Options{})
	tf.Apply(want)
	if !bytes.Equal(e.Img.Levels[0].Pix, want.Pix) {
		t.Fatal("overview not converted")
	}
	if e.Img.Color.Profile != "Display P3 / sRGB" {
		t.Fatalf("report %+v", e.Img.Color)
	}
}

func TestWrappedTileReaderConvertsDeepTiles(t *testing.T) {
	m := colorm.NewManager(nil)
	m.Configure(colorm.Config{Enabled: true})
	base := &deepTiles{colorTiles{info: codec.Info{Width: 64, Height: 48}, color: colorOf("pq")}}
	wrapped, rep := m.WrapTiles(base)
	if !rep.Deep || rep.HDR != "PQ" {
		t.Fatalf("%+v", rep)
	}
	tile, err := wrapped.Region(context.Background(), 0, image.Rect(0, 0, 16, 16))
	if err != nil || tile.Pix[0] < 250 {
		t.Fatalf("%v %v", err, tile.Pix[:4])
	}
	ov, err := wrapped.Overview(context.Background(), 1)
	if err != nil || ov.Rect.Dx() != 32 || ov.Pix[0] < 250 {
		t.Fatalf("%v", err)
	}
	off := colorm.NewManager(nil)
	same, rep2 := off.WrapTiles(base)
	if same != codec.TileReader(base) || rep2.Managed {
		t.Fatal("a disabled manager must hand the reader back untouched")
	}
}

func testInk(w, h int) *image.CMYK {
	img := image.NewCMYK(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			copy(img.Pix[img.PixOffset(x, y):], []uint8{uint8(x * 4), uint8(y * 5), uint8(x + y), uint8(x * y % 90)})
		}
	}
	return img
}

func TestLoaderConvertsInkFramesAndDropsThem(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		r := newColorRig(t, enabled, false)
		if !enabled {
			r.loader.SetColor(nil)
		}
		e := r.load("ink")
		if enabled && !r.codec.options[0].Deep {
			t.Fatal("Deep not requested")
		}
		if !enabled {
			continue
		}
		f := e.Img.Frames[0]
		if f.CMYK != nil || f.Pix == nil {
			t.Fatalf("ink frame left behind: %+v", f)
		}
		src := testInk(64, 48)
		for _, p := range [][2]int{{0, 0}, {10, 20}, {63, 47}} {
			c := src.CMYKAt(p[0], p[1])
			r8, g8, b8 := color.CMYKToRGB(c.C, c.M, c.Y, c.K)
			if got := f.Pix.NRGBAAt(p[0], p[1]); got != (color.NRGBA{R: r8, G: g8, B: b8, A: 255}) {
				t.Fatalf("(%d,%d) = %v", p[0], p[1], got)
			}
		}
		if len(e.Img.Levels) == 0 || e.Img.Levels[0] != f.Pix {
			t.Fatal("pyramid not built from the converted frame")
		}
	}
}

func TestInkJPEGReservesRoomForTheConvertedCopy(t *testing.T) {
	cmyk := codec.Info{Format: codec.JPEG, Width: 100, Height: 50, BitDepth: 32}
	if got := deepExtraBytes(cmyk); got != 100*50*4 {
		t.Fatalf("CMYK JPEG reserve %d", got)
	}
	rgb := codec.Info{Format: codec.JPEG, Width: 100, Height: 50, BitDepth: 24}
	if got := deepExtraBytes(rgb); got != 0 {
		t.Fatalf("RGB JPEG reserve %d", got)
	}
	png := codec.Info{Format: codec.PNG, Width: 100, Height: 50, BitDepth: 32}
	if got := deepExtraBytes(png); got != 0 {
		t.Fatalf("RGBA PNG reserve %d", got)
	}
}
