package convert

import (
	"image"
	"image/color"
	"math/rand"
	"sync/atomic"
	"testing"
)

func randomBytes(rng *rand.Rand, n int) []byte {
	buf := make([]byte, n)
	rng.Read(buf)
	return buf
}

func requireMatchesModel(t *testing.T, name string, src image.Image, got *image.NRGBA) {
	t.Helper()
	bounds := src.Bounds()
	if got.Bounds() != image.Rect(0, 0, bounds.Dx(), bounds.Dy()) {
		t.Fatalf("%s: bounds %v, want origin-based size of %v", name, got.Bounds(), bounds)
	}
	for y := 0; y < bounds.Dy(); y++ {
		for x := 0; x < bounds.Dx(); x++ {
			want := color.NRGBAModel.Convert(src.At(bounds.Min.X+x, bounds.Min.Y+y)).(color.NRGBA)
			if have := got.NRGBAAt(x, y); have != want {
				t.Fatalf("%s: pixel (%d,%d) = %v, want %v", name, x, y, have, want)
			}
		}
	}
}

func randomYCbCr(rng *rand.Rand, ratio image.YCbCrSubsampleRatio, w, h int) *image.YCbCr {
	img := image.NewYCbCr(image.Rect(0, 0, w, h), ratio)
	copy(img.Y, randomBytes(rng, len(img.Y)))
	copy(img.Cb, randomBytes(rng, len(img.Cb)))
	copy(img.Cr, randomBytes(rng, len(img.Cr)))
	return img
}

func TestYCbCrMatchesStdlibExhaustively(t *testing.T) {
	img := image.NewYCbCr(image.Rect(0, 0, 256, 256), image.YCbCrSubsampleRatio444)
	for y := 0; y < 256; y++ {
		for x := 0; x < 256; x++ {
			img.Y[y*img.YStride+x] = uint8(x)
			img.Cr[y*img.CStride+x] = uint8(y)
		}
	}
	for cb := 0; cb < 256; cb++ {
		for i := range img.Cb {
			img.Cb[i] = uint8(cb)
		}
		got := NRGBA(img)
		for y := 0; y < 256; y++ {
			for x := 0; x < 256; x++ {
				r, g, b := color.YCbCrToRGB(uint8(x), uint8(cb), uint8(y))
				if have := got.NRGBAAt(x, y); have != (color.NRGBA{R: r, G: g, B: b, A: 0xff}) {
					t.Fatalf("y=%d cb=%d cr=%d: got %v want %d,%d,%d", x, cb, y, have, r, g, b)
				}
			}
		}
	}
}

func TestYCbCrAllSubsampleRatios(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	ratios := map[string]image.YCbCrSubsampleRatio{
		"444": image.YCbCrSubsampleRatio444,
		"422": image.YCbCrSubsampleRatio422,
		"420": image.YCbCrSubsampleRatio420,
		"440": image.YCbCrSubsampleRatio440,
		"411": image.YCbCrSubsampleRatio411,
		"410": image.YCbCrSubsampleRatio410,
	}
	for name, ratio := range ratios {
		for _, size := range [][2]int{{1, 1}, {2, 2}, {3, 5}, {37, 23}, {64, 64}} {
			img := randomYCbCr(rng, ratio, size[0], size[1])
			requireMatchesModel(t, name, img, NRGBA(img))
		}
		img := randomYCbCr(rng, ratio, 41, 33)
		sub := img.SubImage(image.Rect(3, 5, 30, 28)).(*image.YCbCr)
		requireMatchesModel(t, name+" subimage", sub, NRGBA(sub))
		odd := img.SubImage(image.Rect(1, 1, 40, 32)).(*image.YCbCr)
		requireMatchesModel(t, name+" odd subimage", odd, NRGBA(odd))
	}
}

func TestYCbCrParallelPath(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	for _, ratio := range []image.YCbCrSubsampleRatio{image.YCbCrSubsampleRatio420, image.YCbCrSubsampleRatio444} {
		img := randomYCbCr(rng, ratio, 1203, 1001)
		requireMatchesModel(t, "large", img, NRGBA(img))
	}
}

func TestGray(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	img := image.NewGray(image.Rect(0, 0, 33, 17))
	copy(img.Pix, randomBytes(rng, len(img.Pix)))
	requireMatchesModel(t, "gray", img, NRGBA(img))
	sub := img.SubImage(image.Rect(2, 3, 20, 15)).(*image.Gray)
	requireMatchesModel(t, "gray sub", sub, NRGBA(sub))
}

func TestGray16(t *testing.T) {
	rng := rand.New(rand.NewSource(4))
	img := image.NewGray16(image.Rect(0, 0, 33, 17))
	copy(img.Pix, randomBytes(rng, len(img.Pix)))
	requireMatchesModel(t, "gray16", img, NRGBA(img))
	sub := img.SubImage(image.Rect(2, 3, 20, 15)).(*image.Gray16)
	requireMatchesModel(t, "gray16 sub", sub, NRGBA(sub))
}

func TestCMYK(t *testing.T) {
	rng := rand.New(rand.NewSource(5))
	img := image.NewCMYK(image.Rect(0, 0, 33, 17))
	copy(img.Pix, randomBytes(rng, len(img.Pix)))
	requireMatchesModel(t, "cmyk", img, NRGBA(img))
	sub := img.SubImage(image.Rect(2, 3, 20, 15)).(*image.CMYK)
	requireMatchesModel(t, "cmyk sub", sub, NRGBA(sub))
}

func TestPaletted(t *testing.T) {
	rng := rand.New(rand.NewSource(6))
	palette := make(color.Palette, 0, 40)
	for i := 0; i < 40; i++ {
		palette = append(palette, color.NRGBA{R: uint8(rng.Intn(256)), G: uint8(rng.Intn(256)), B: uint8(rng.Intn(256)), A: uint8(rng.Intn(256))})
	}
	palette = append(palette, color.RGBA{}, color.Gray{Y: 7})
	img := image.NewPaletted(image.Rect(0, 0, 33, 17), palette)
	for i := range img.Pix {
		img.Pix[i] = uint8(rng.Intn(len(palette)))
	}
	requireMatchesModel(t, "paletted", img, NRGBA(img))
	sub := img.SubImage(image.Rect(2, 3, 20, 15)).(*image.Paletted)
	requireMatchesModel(t, "paletted sub", sub, NRGBA(sub))
}

func TestRGBAUnpremultiply(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	img := image.NewRGBA(image.Rect(0, 0, 61, 29))
	copy(img.Pix, randomBytes(rng, len(img.Pix)))
	requireMatchesModel(t, "rgba random bytes", img, NRGBA(img))

	valid := image.NewRGBA(image.Rect(0, 0, 256, 256))
	for a := 0; a < 256; a++ {
		for v := 0; v <= a; v++ {
			i := valid.PixOffset(v, a)
			valid.Pix[i], valid.Pix[i+1], valid.Pix[i+2], valid.Pix[i+3] = uint8(v), uint8(v/2), uint8(a), uint8(a)
		}
	}
	requireMatchesModel(t, "rgba valid", valid, NRGBA(valid))
	sub := img.SubImage(image.Rect(2, 3, 20, 15)).(*image.RGBA)
	requireMatchesModel(t, "rgba sub", sub, NRGBA(sub))
}

func TestRGBAParallelPath(t *testing.T) {
	rng := rand.New(rand.NewSource(8))
	img := image.NewRGBA(image.Rect(0, 0, 1100, 1000))
	copy(img.Pix, randomBytes(rng, len(img.Pix)))
	requireMatchesModel(t, "rgba large", img, NRGBA(img))
}

func TestNRGBAReturnedAsIs(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	if NRGBA(img) != img {
		t.Fatal("origin-based NRGBA must be returned unchanged")
	}
}

func TestNRGBAWithOffsetIsCopied(t *testing.T) {
	rng := rand.New(rand.NewSource(9))
	img := image.NewNRGBA(image.Rect(0, 0, 30, 20))
	copy(img.Pix, randomBytes(rng, len(img.Pix)))
	sub := img.SubImage(image.Rect(4, 3, 25, 18)).(*image.NRGBA)
	got := NRGBA(sub)
	if got == sub {
		t.Fatal("sub image with offset must be copied to an origin-based image")
	}
	requireMatchesModel(t, "nrgba sub", sub, got)
}

func TestNRGBA64(t *testing.T) {
	rng := rand.New(rand.NewSource(10))
	img := image.NewNRGBA64(image.Rect(0, 0, 33, 17))
	copy(img.Pix, randomBytes(rng, len(img.Pix)))
	requireHighBytes := func(name string, src *image.NRGBA64) {
		t.Helper()
		got := NRGBA(src)
		bounds := src.Bounds()
		for y := 0; y < bounds.Dy(); y++ {
			for x := 0; x < bounds.Dx(); x++ {
				c := src.NRGBA64At(bounds.Min.X+x, bounds.Min.Y+y)
				want := color.NRGBA{R: uint8(c.R >> 8), G: uint8(c.G >> 8), B: uint8(c.B >> 8), A: uint8(c.A >> 8)}
				if have := got.NRGBAAt(x, y); have != want {
					t.Fatalf("%s: pixel (%d,%d) = %v, want %v", name, x, y, have, want)
				}
			}
		}
	}
	requireHighBytes("nrgba64", img)
	requireHighBytes("nrgba64 sub", img.SubImage(image.Rect(2, 3, 20, 15)).(*image.NRGBA64))
}

func TestRGBA64(t *testing.T) {
	rng := rand.New(rand.NewSource(11))
	img := image.NewRGBA64(image.Rect(0, 0, 33, 17))
	copy(img.Pix, randomBytes(rng, len(img.Pix)))
	requireMatchesModel(t, "rgba64 random", img, NRGBA(img))
	for i := 0; i+8 <= len(img.Pix); i += 8 {
		if i%16 == 0 {
			img.Pix[i+6], img.Pix[i+7] = 0xff, 0xff
		} else if i%24 == 0 {
			img.Pix[i+6], img.Pix[i+7] = 0, 0
		}
	}
	requireMatchesModel(t, "rgba64 mixed", img, NRGBA(img))
	sub := img.SubImage(image.Rect(2, 3, 20, 15)).(*image.RGBA64)
	requireMatchesModel(t, "rgba64 sub", sub, NRGBA(sub))
}

func TestGenericFallback(t *testing.T) {
	rng := rand.New(rand.NewSource(12))
	img := image.NewAlpha(image.Rect(2, 3, 40, 30))
	copy(img.Pix, randomBytes(rng, len(img.Pix)))
	requireMatchesModel(t, "alpha", img, NRGBA(img))
	empty := image.NewGray(image.Rectangle{})
	if got := NRGBA(empty); got.Bounds().Dx() != 0 || got.Bounds().Dy() != 0 {
		t.Fatalf("empty source produced %v", got.Bounds())
	}
	if got := NRGBA(nil); got == nil {
		t.Fatal("nil source must produce an empty image")
	}
}

func TestBandsCoverEveryRowOnce(t *testing.T) {
	for _, size := range [][2]int{{10, 10}, {2000, 1}, {1500, 900}, {1, 50000}, {4000, 4000}} {
		counts := make([]int32, size[1])
		Bands(size[0], size[1], func(y0, y1 int) {
			for y := y0; y < y1; y++ {
				atomic.AddInt32(&counts[y], 1)
			}
		})
		for y, c := range counts {
			if c != 1 {
				t.Fatalf("%v: row %d visited %d times", size, y, c)
			}
		}
	}
}

func TestBandsRethrowWorkerPanic(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("panic in a worker must reach the caller")
		}
	}()
	Bands(4000, 4000, func(y0, y1 int) {
		if y0 > 0 {
			panic("boom")
		}
	})
}

func TestOverBackground(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 3, 1))
	src.SetNRGBA(0, 0, color.NRGBA{R: 200, G: 100, B: 50, A: 255})
	src.SetNRGBA(1, 0, color.NRGBA{R: 200, G: 100, B: 50, A: 0})
	src.SetNRGBA(2, 0, color.NRGBA{R: 0, G: 0, B: 0, A: 128})
	got := OverBackground(src, color.RGBA{R: 255, G: 255, B: 255, A: 255})
	want := []color.RGBA{{200, 100, 50, 255}, {255, 255, 255, 255}, {127, 127, 127, 255}}
	for x, w := range want {
		if have := got.RGBAAt(x, 0); have != w {
			t.Fatalf("pixel %d = %v, want %v", x, have, w)
		}
	}
}

func TestAlphaAllZeroAndSetOpaque(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 5, 5))
	img.Pix[0] = 9
	if !AlphaAllZero(img) {
		t.Fatal("all alpha bytes are zero")
	}
	SetOpaque(img)
	if AlphaAllZero(img) || img.NRGBAAt(4, 4).A != 0xff || img.Pix[0] != 9 {
		t.Fatalf("SetOpaque changed colour data or missed alpha: %v", img.NRGBAAt(4, 4))
	}
}

func BenchmarkYCbCr420Large(b *testing.B) {
	rng := rand.New(rand.NewSource(13))
	img := randomYCbCr(rng, image.YCbCrSubsampleRatio420, 6000, 4000)
	b.SetBytes(int64(6000 * 4000 * 4))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		NRGBA(img)
	}
}
