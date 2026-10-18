package convert

import (
	"image"
	"image/color"
	"math/rand"
	"testing"
)

func TestDeepOfNRGBA64IsTheSameImage(t *testing.T) {
	src := image.NewNRGBA64(image.Rect(0, 0, 4, 4))
	if Deep(src) != src {
		t.Fatal("copied")
	}
	shifted := image.NewNRGBA64(image.Rect(2, 2, 6, 6))
	shifted.SetNRGBA64(3, 3, color.NRGBA64{R: 1, G: 2, B: 3, A: 4})
	got := Deep(shifted)
	if got.Rect != image.Rect(0, 0, 4, 4) || got.NRGBA64At(1, 1) != (color.NRGBA64{R: 1, G: 2, B: 3, A: 4}) {
		t.Fatal("origin not normalised")
	}
}

func TestDeepOfRGBA64UnpremultipliesInPlace(t *testing.T) {
	src := image.NewRGBA64(image.Rect(0, 0, 3, 1))
	src.SetRGBA64(0, 0, color.RGBA64{R: 0x4000, G: 0x2000, B: 0x1000, A: 0x8000})
	src.SetRGBA64(1, 0, color.RGBA64{R: 0x1234, G: 0x2345, B: 0x3456, A: 0xffff})
	src.SetRGBA64(2, 0, color.RGBA64{})
	first := &src.Pix[0]
	got := Deep(src)
	if &got.Pix[0] != first {
		t.Fatal("pixels were copied")
	}
	if p := got.NRGBA64At(0, 0); p.R != 0x7fff || p.G != 0x3fff || p.B != 0x1fff || p.A != 0x8000 {
		t.Fatalf("%v", p)
	}
	if p := got.NRGBA64At(1, 0); p != (color.NRGBA64{R: 0x1234, G: 0x2345, B: 0x3456, A: 0xffff}) {
		t.Fatalf("%v", p)
	}
	if p := got.NRGBA64At(2, 0); p != (color.NRGBA64{}) {
		t.Fatalf("%v", p)
	}
}

func TestDeepWidensEightBitSamplesExactly(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	n := image.NewNRGBA(image.Rect(0, 0, 33, 7))
	rng.Read(n.Pix)
	got := Deep(n)
	for y := 0; y < 7; y++ {
		for x := 0; x < 33; x++ {
			c, d := n.NRGBAAt(x, y), got.NRGBA64At(x, y)
			if d.R != uint16(c.R)*257 || d.G != uint16(c.G)*257 || d.B != uint16(c.B)*257 || d.A != uint16(c.A)*257 {
				t.Fatalf("%v %v", c, d)
			}
		}
	}
	g := image.NewGray(image.Rect(0, 0, 5, 5))
	rng.Read(g.Pix)
	for y := 0; y < 5; y++ {
		for x := 0; x < 5; x++ {
			v := uint16(g.GrayAt(x, y).Y) * 257
			if d := Deep(g).NRGBA64At(x, y); d.R != v || d.G != v || d.B != v || d.A != 0xffff {
				t.Fatalf("gray %v", d)
			}
		}
	}
	if w := Widen(n); w.Rect != n.Rect {
		t.Fatal("Widen")
	}
}

func TestDeepOfGray16KeepsAllBits(t *testing.T) {
	g := image.NewGray16(image.Rect(0, 0, 9, 9))
	rand.New(rand.NewSource(2)).Read(g.Pix)
	out := Deep(g)
	for y := 0; y < 9; y++ {
		for x := 0; x < 9; x++ {
			v := g.Gray16At(x, y).Y
			if d := out.NRGBA64At(x, y); d.R != v || d.G != v || d.B != v || d.A != 0xffff {
				t.Fatalf("%v vs %d", d, v)
			}
		}
	}
}

func TestDeepOfOtherImagesGoesThroughTheColorModel(t *testing.T) {
	y := image.NewYCbCr(image.Rect(0, 0, 8, 8), image.YCbCrSubsampleRatio420)
	for i := range y.Y {
		y.Y[i] = 200
	}
	for i := range y.Cb {
		y.Cb[i], y.Cr[i] = 128, 128
	}
	d := Deep(y).NRGBA64At(3, 3)
	if d.A != 0xffff || d.R < 0xc000 || d.R > 0xcfff {
		t.Fatalf("%v", d)
	}
	if got := Deep(nil); !got.Rect.Empty() {
		t.Fatal("nil")
	}
}
