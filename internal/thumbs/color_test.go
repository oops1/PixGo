package thumbs

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"io"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/colorm"
)

type colorCodec struct {
	deepSeen bool
}

func (c *colorCodec) ID() codec.ID           { return "col" }
func (c *colorCodec) Name() string           { return "Col" }
func (c *colorCodec) Extensions() []string   { return []string{".col"} }
func (c *colorCodec) Sniff(head []byte) bool { return bytes.HasPrefix(head, []byte("COL ")) }

func (c *colorCodec) Probe(ctx context.Context, r io.ReaderAt, size int64, lim codec.Limits) (codec.Info, error) {
	return codec.Info{Format: "col", Width: 40, Height: 30, Frames: 1}, nil
}

func (c *colorCodec) Decode(ctx context.Context, r io.ReaderAt, size int64, o codec.Options) (*codec.Image, error) {
	c.deepSeen = c.deepSeen || o.Deep
	img := &codec.Image{Info: codec.Info{Format: "col", Width: 40, Height: 30, Frames: 1}}
	if o.Deep {
		d := image.NewNRGBA64(image.Rect(0, 0, 40, 30))
		for i := 0; i < 40*30; i++ {
			v := uint16(38054)
			for ch := 0; ch < 4; ch++ {
				x := v
				if ch == 3 {
					x = 0xffff
				}
				d.Pix[8*i+2*ch], d.Pix[8*i+2*ch+1] = byte(x>>8), byte(x)
			}
		}
		img.Color = codec.ColorInfo{CICP: &codec.CICP{Primaries: 9, Transfer: 16, FullRange: true}}
		img.Frames = []codec.Frame{{Deep: d}}
		return img, nil
	}
	img.Frames = []codec.Frame{{Pix: solid(40, 30, color.NRGBA{R: 40, G: 120, B: 200, A: 255})}}
	return img, nil
}

func TestThumbnailsOfDeepHDRAreToneMapped(t *testing.T) {
	reg := codec.NewRegistry()
	cc := &colorCodec{}
	reg.RegisterDecoder(cc)
	m := colorm.NewManager(nil)
	m.Configure(colorm.Config{Enabled: true})
	h := newHarness(t, reg, Options{Color: m})
	p := h.write("a.col", []byte("COL x"))
	th := h.get(p, 64)
	if th.Err != nil || th.Img == nil {
		t.Fatalf("%+v", th)
	}
	if !cc.deepSeen {
		t.Fatal("thumbnails must ask for Deep when colour management is on")
	}
	if px := th.Img.Pix; px[0] < 250 || px[1] < 250 || px[2] < 250 {
		t.Fatalf("203 nit PQ grey became %v", px[:4])
	}
}

func TestThumbnailKeysSeparateColorManagedFiles(t *testing.T) {
	k := Key{Path: "a", Size: 1, Side: 64}
	c := k
	c.Color = "cm1"
	if k.File() == c.File() || k.ck() == c.ck() {
		t.Fatal("colour-managed thumbnails must not reuse unmanaged cache entries")
	}
	if (Key{Path: "a", Size: 1, Side: 64}).File() != k.File() {
		t.Fatal("unmanaged names must stay as they were in 1.1")
	}
	var off *colorm.Manager
	if off.Tag() != "" {
		t.Fatal("no manager, no tag")
	}
	m := colorm.NewManager(nil)
	if m.Tag() != "" {
		t.Fatal("disabled manager must not tag")
	}
	m.Configure(colorm.Config{Enabled: true})
	if m.Tag() != "cm1" {
		t.Fatalf("tag %q", m.Tag())
	}
}

type inkCodec struct{ colorCodec }

func (c *inkCodec) Decode(ctx context.Context, r io.ReaderAt, size int64, o codec.Options) (*codec.Image, error) {
	img := &codec.Image{Info: codec.Info{Format: "col", Width: 40, Height: 30, Frames: 1}}
	if o.Deep {
		ink := image.NewCMYK(image.Rect(0, 0, 40, 30))
		for i := 0; i < 40*30; i++ {
			copy(ink.Pix[4*i:], []byte{0, 255, 255, 0})
		}
		img.Frames = []codec.Frame{{CMYK: ink}}
		return img, nil
	}
	img.Frames = []codec.Frame{{Pix: solid(40, 30, color.NRGBA{R: 255, A: 255})}}
	return img, nil
}

func TestThumbnailsOfInkFramesAreConverted(t *testing.T) {
	reg := codec.NewRegistry()
	reg.RegisterDecoder(&inkCodec{})
	m := colorm.NewManager(nil)
	m.Configure(colorm.Config{Enabled: true})
	h := newHarness(t, reg, Options{Color: m})
	p := h.write("a.col", []byte("COL x"))
	th := h.get(p, 64)
	if th.Err != nil || th.Img == nil {
		t.Fatalf("%+v", th)
	}
	if px := th.Img.Pix; px[0] < 250 || px[1] > 5 || px[2] > 5 || px[3] != 255 {
		t.Fatalf("magenta+yellow ink became %v", px[:4])
	}
}
