package imaging

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
)

func numbered(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{uint8(x), uint8(y), uint8(x + y), 255})
		}
	}
	return img
}

func TestOrientMatchesDefinition(t *testing.T) {
	for _, dims := range [][2]int{{3, 2}, {5, 7}, {1, 4}, {64, 33}} {
		w, h := dims[0], dims[1]
		src := numbered(w, h)
		for o := codec.OrientNormal; o <= codec.OrientRotate270; o++ {
			got := Orient(src, o)
			dw, dh := o.Size(w, h)
			if got.Bounds().Dx() != dw || got.Bounds().Dy() != dh {
				t.Fatalf("%dx%d o=%d: size %v want %dx%d", w, h, o, got.Bounds(), dw, dh)
			}
			for v := 0; v < dh; v++ {
				for u := 0; u < dw; u++ {
					x, y := o.ToSource(float64(u)+.5, float64(v)+.5, float64(w), float64(h))
					want := src.NRGBAAt(int(x), int(y))
					if g := got.NRGBAAt(u, v); g != want {
						t.Fatalf("%dx%d o=%d (%d,%d): got %v want %v", w, h, o, u, v, g, want)
					}
				}
			}
		}
	}
}

func TestOrientNormalReturnsSameImage(t *testing.T) {
	src := numbered(4, 4)
	if Orient(src, codec.OrientNormal) != src {
		t.Error("normal orientation copied the image")
	}
}

func BenchmarkOrientRotate90(b *testing.B) {
	src := image.NewNRGBA(image.Rect(0, 0, 6000, 4000))
	b.SetBytes(int64(len(src.Pix)))
	for i := 0; i < b.N; i++ {
		Orient(src, codec.OrientRotate90)
	}
}
