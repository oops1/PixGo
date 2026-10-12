package thumbs

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
)

func exifStoredFor(o codec.Orientation, x, y, w, h int) (col, row int) {
	switch o {
	case 2:
		return w - 1 - x, y
	case 3:
		return w - 1 - x, h - 1 - y
	case 4:
		return x, h - 1 - y
	case 5:
		return y, x
	case 6:
		return y, h - 1 - x
	case 7:
		return w - 1 - y, h - 1 - x
	case 8:
		return w - 1 - y, x
	}
	return x, y
}

func numbered(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: uint8(x*16 + y), A: 255})
		}
	}
	return img
}

func TestOrientMatchesEXIFDefinition(t *testing.T) {
	const w, h = 5, 3
	for o := codec.OrientNormal; o <= codec.OrientRotate270; o++ {
		src := numbered(w, h)
		out := orient(src, o)
		dw, dh := w, h
		if o >= 5 {
			dw, dh = h, w
		}
		if out.Bounds().Dx() != dw || out.Bounds().Dy() != dh {
			t.Fatalf("orientation %d: got %v, want %dx%d", o, out.Bounds(), dw, dh)
		}
		for y := 0; y < dh; y++ {
			for x := 0; x < dw; x++ {
				col, row := exifStoredFor(o, x, y, w, h)
				want := src.RGBAAt(col, row)
				if got := out.RGBAAt(x, y); got != want {
					t.Fatalf("orientation %d: pixel (%d,%d) = %v, want stored (%d,%d) = %v", o, x, y, got, col, row, want)
				}
			}
		}
	}
}

func TestOrientNormalReturnsSameImage(t *testing.T) {
	src := numbered(4, 4)
	if orient(src, codec.OrientNormal) != src {
		t.Fatal("normal orientation must not copy")
	}
	if orient(src, codec.Orientation(0)) != src || orient(src, codec.Orientation(99)) != src {
		t.Fatal("invalid orientation must be treated as normal")
	}
	if orient(nil, codec.OrientRotate90) != nil {
		t.Fatal("nil in, nil out")
	}
}

func TestOrientRoundTrips(t *testing.T) {
	src := numbered(7, 4)
	pairs := [][2]codec.Orientation{
		{codec.OrientRotate90, codec.OrientRotate270},
		{codec.OrientFlipH, codec.OrientFlipH},
		{codec.OrientTranspose, codec.OrientTranspose},
		{codec.OrientRotate180, codec.OrientRotate180},
	}
	for _, p := range pairs {
		back := orient(orient(src, p[0]), p[1])
		if back.Bounds() != src.Bounds() {
			t.Fatalf("%v: bounds %v", p, back.Bounds())
		}
		for i := range src.Pix {
			if back.Pix[i] != src.Pix[i] {
				t.Fatalf("%v: pixel data differs", p)
			}
		}
	}
}

func TestOrientHandlesSubImage(t *testing.T) {
	big := numbered(10, 10)
	sub := big.SubImage(image.Rect(2, 3, 6, 5)).(*image.RGBA)
	out := orient(sub, codec.OrientRotate90)
	if out.Bounds().Dx() != 2 || out.Bounds().Dy() != 4 {
		t.Fatalf("got %v", out.Bounds())
	}
	for y := 0; y < 4; y++ {
		for x := 0; x < 2; x++ {
			col, row := exifStoredFor(6, x, y, 4, 2)
			if got, want := out.RGBAAt(x, y), big.RGBAAt(2+col, 3+row); got != want {
				t.Fatalf("pixel (%d,%d) = %v, want %v", x, y, got, want)
			}
		}
	}
}

func TestOrientedAndStoredSize(t *testing.T) {
	for o := codec.OrientNormal; o <= codec.OrientRotate270; o++ {
		w, h := orientedSize(o, 30, 20)
		sw, sh := storedSize(o, w, h)
		if sw != 30 || sh != 20 {
			t.Errorf("orientation %d: round trip gave %dx%d", o, sw, sh)
		}
		if o >= 5 && (w != 20 || h != 30) {
			t.Errorf("orientation %d: oriented size %dx%d", o, w, h)
		}
	}
}
