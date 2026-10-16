package imaging

import (
	"image"
	"math"
	"math/rand"
	"testing"
)

func refUpsample(coarse *image.NRGBA, factor uint, x, y int, c int) float64 {
	cb := coarse.Rect
	f := float64(uint(1) << factor)
	px := (float64(x)+0.5)/f - 0.5
	py := (float64(y)+0.5)/f - 0.5
	x0, y0 := math.Floor(px), math.Floor(py)
	tx, ty := px-x0, py-y0
	fetch := func(ix, iy int) (float64, float64) {
		ix = min(max(ix, 0), cb.Dx()-1)
		iy = min(max(iy, 0), cb.Dy()-1)
		o := coarse.PixOffset(cb.Min.X+ix, cb.Min.Y+iy)
		a := float64(coarse.Pix[o+3]) / 255
		return float64(coarse.Pix[o+c]) * a, a
	}
	c00, a00 := fetch(int(x0), int(y0))
	c01, a01 := fetch(int(x0)+1, int(y0))
	c10, a10 := fetch(int(x0), int(y0)+1)
	c11, a11 := fetch(int(x0)+1, int(y0)+1)
	mix := func(v00, v01, v10, v11 float64) float64 {
		return (v00*(1-tx)+v01*tx)*(1-ty) + (v10*(1-tx)+v11*tx)*ty
	}
	if c == 3 {
		return mix(a00, a01, a10, a11) * 255
	}
	a := mix(a00, a01, a10, a11)
	if a == 0 {
		return 0
	}
	return mix(c00, c01, c10, c11) / a
}

func TestFillFromCoarseMatchesBilinearReference(t *testing.T) {
	rng := rand.New(rand.NewSource(5))
	for _, translucent := range []bool{false, true} {
		coarse := randomNRGBA(rng, 37, 23, translucent)
		for _, factor := range []uint{1, 2, 3, 4} {
			w, h := 37<<factor+9, 23<<factor+9
			region := image.Rect(3, 2, min(w, 3+90), min(h, 2+70))
			dst := image.NewNRGBA(image.Rect(0, 0, region.Dx()+4, region.Dy()+4))
			org := image.Pt(region.Min.X-2, region.Min.Y-2)
			fillFromCoarse(dst, org, region, coarse, factor, !translucent)
			for y := region.Min.Y; y < region.Max.Y; y++ {
				for x := region.Min.X; x < region.Max.X; x++ {
					o := dst.PixOffset(x-org.X, y-org.Y)
					for c := 0; c < 4; c++ {
						want := refUpsample(coarse, factor, x, y, c)
						got := float64(dst.Pix[o+c])
						tol := 2.5
						if translucent && c < 3 {
							tol = 6
						}
						if math.Abs(got-want) > tol {
							t.Fatalf("translucent=%v factor=%d (%d,%d) c=%d: got %v want %.2f", translucent, factor, x, y, c, got, want)
						}
					}
				}
			}
			if dst.Pix[0] != 0 || dst.Pix[3] != 0 {
				t.Fatal("pixels outside the region must stay untouched")
			}
		}
	}
}

func TestFillFromCoarseClampsAtTheBorders(t *testing.T) {
	coarse := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	vals := [4][4]uint8{{10, 20, 30, 255}, {200, 20, 30, 255}, {10, 220, 30, 255}, {200, 220, 130, 255}}
	for i, v := range vals {
		copy(coarse.Pix[(i/2)*coarse.Stride+(i%2)*4:], v[:])
	}
	dst := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	fillFromCoarse(dst, image.Point{}, image.Rect(0, 0, 8, 8), coarse, 2, true)
	if got := [4]uint8(dst.Pix[0:4]); got != vals[0] {
		t.Fatalf("top left %v, want the clamped corner %v", got, vals[0])
	}
	last := dst.PixOffset(7, 7)
	if got := [4]uint8(dst.Pix[last : last+4]); got != vals[3] {
		t.Fatalf("bottom right %v, want %v", got, vals[3])
	}
	for i := 3; i < len(dst.Pix); i += 4 {
		if dst.Pix[i] != 255 {
			t.Fatal("opaque source must give opaque placeholder")
		}
	}
}

func TestFillFromCoarseIgnoresEmptyInput(t *testing.T) {
	dst := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	fillFromCoarse(dst, image.Point{}, image.Rectangle{}, image.NewNRGBA(image.Rect(0, 0, 2, 2)), 1, true)
	fillFromCoarse(dst, image.Point{}, image.Rect(0, 0, 4, 4), nil, 1, true)
	fillFromCoarse(dst, image.Point{}, image.Rect(0, 0, 4, 4), image.NewNRGBA(image.Rectangle{}), 1, true)
	for _, v := range dst.Pix {
		if v != 0 {
			t.Fatal("nothing may be written")
		}
	}
}

func TestFillFromCoarseTransparentStaysTransparent(t *testing.T) {
	coarse := image.NewNRGBA(image.Rect(0, 0, 6, 6))
	dst := image.NewNRGBA(image.Rect(0, 0, 12, 12))
	for i := range dst.Pix {
		dst.Pix[i] = 99
	}
	fillFromCoarse(dst, image.Point{}, image.Rect(0, 0, 12, 12), coarse, 1, false)
	for _, v := range dst.Pix {
		if v != 0 {
			t.Fatal("fully transparent source must produce fully transparent placeholder")
		}
	}
}

func BenchmarkFillFromCoarse(b *testing.B) {
	rng := rand.New(rand.NewSource(6))
	coarse := randomNRGBA(rng, 1024, 768, false)
	dst := image.NewNRGBA(image.Rect(0, 0, TileSide, TileSide))
	region := image.Rect(3000, 2000, 3000+TileSide, 2000+TileSide)
	b.SetBytes(TileSide * TileSide * 4)
	for i := 0; i < b.N; i++ {
		fillFromCoarse(dst, region.Min, region, coarse, 2, true)
	}
}
