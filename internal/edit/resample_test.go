package edit

import (
	"context"
	"errors"
	"image"
	"image/color"
	"runtime"
	"testing"
)

func resample(t *testing.T, src *image.NRGBA, w, h int) *image.NRGBA {
	t.Helper()
	out, err := Resample(context.Background(), src, w, h)
	if err != nil {
		t.Fatal(err)
	}
	if out.Bounds() != image.Rect(0, 0, w, h) {
		t.Fatalf("bounds %v, want %dx%d", out.Bounds(), w, h)
	}
	return out
}

func TestSolidColorsStayExact(t *testing.T) {
	colors := []color.NRGBA{
		{R: 0, G: 0, B: 0, A: 255},
		{R: 255, G: 255, B: 255, A: 255},
		{R: 12, G: 200, B: 77, A: 255},
		{R: 12, G: 200, B: 77, A: 128},
		{R: 255, G: 1, B: 0, A: 3},
		{R: 9, G: 9, B: 9, A: 254},
	}
	sizes := [][4]int{{50, 40, 7, 5}, {50, 40, 100, 80}, {50, 40, 49, 41}, {3, 3, 1, 1}, {1, 1, 9, 9}, {2000, 30, 11, 30}, {64, 64, 640, 3}}
	for _, c := range colors {
		for _, s := range sizes {
			out := resample(t, solid(s[0], s[1], c), s[2], s[3])
			for y := 0; y < s[3]; y++ {
				for x := 0; x < s[2]; x++ {
					if got := out.NRGBAAt(x, y); got != c {
						t.Fatalf("%v %v: pixel %d,%d = %v", c, s, x, y, got)
					}
				}
			}
		}
	}
}

func TestTransparentRegionsStayTransparent(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 200, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 200; x++ {
			if x >= 100 {
				src.SetNRGBA(x, y, color.NRGBA{R: 255, A: 255})
			} else {
				src.SetNRGBA(x, y, color.NRGBA{R: 10, G: 250, B: 10, A: 0})
			}
		}
	}
	for _, sz := range [][2]int{{100, 50}, {40, 20}, {400, 200}, {25, 100}} {
		out := resample(t, src, sz[0], sz[1])
		for y := 0; y < sz[1]; y++ {
			for x := 0; x < sz[0]; x++ {
				p := out.NRGBAAt(x, y)
				left := float64(x+1)/float64(sz[0]) <= 0.4
				right := float64(x)/float64(sz[0]) >= 0.6
				switch {
				case left && p.A != 0:
					t.Fatalf("%v: alpha %d deep in the transparent area at %d,%d", sz, p.A, x, y)
				case left && (p.R|p.G|p.B) != 0:
					t.Fatalf("%v: colour %v in a transparent pixel", sz, p)
				case right && p != (color.NRGBA{R: 255, A: 255}):
					t.Fatalf("%v: %v deep in the opaque area at %d,%d", sz, p, x, y)
				case p.A > 0 && (p.G > 2 || p.B > 2 || p.R < 250):
					t.Fatalf("%v: edge pixel %v at %d,%d is not pure red: the transparent colour leaked", sz, p, x, y)
				}
			}
		}
	}
}

func TestCheckerboardHasNoMoire(t *testing.T) {
	src := checker(512, 512)
	for _, f := range []int{2, 3, 4, 5, 7, 8, 16, 32} {
		out := resample(t, src, 512/f, 512/f)
		worst := 0
		for y := 3; y < out.Bounds().Dy()-3; y++ {
			for x := 3; x < out.Bounds().Dx()-3; x++ {
				v := int(out.NRGBAAt(x, y).R)
				if d := v - 128; d < 0 {
					worst = max(worst, -d)
				} else {
					worst = max(worst, d)
				}
			}
		}
		t.Logf("checkerboard /%d: worst deviation from mid grey %d", f, worst)
		if worst > 4 {
			t.Fatalf("checkerboard shrunk by %d: deviation %d, moire", f, worst)
		}
	}
}

func TestFineStripesShrinkToGrey(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 600, 600))
	for y := 0; y < 600; y++ {
		for x := 0; x < 600; x++ {
			v := uint8(0)
			if (x/2)%2 == 0 {
				v = 255
			}
			src.SetNRGBA(x, y, color.NRGBA{R: v, G: v, B: v, A: 255})
		}
	}
	out := resample(t, src, 100, 100)
	worst := 0
	for y := 2; y < 98; y++ {
		for x := 2; x < 98; x++ {
			d := int(out.NRGBAAt(x, y).R) - 128
			worst = max(worst, max(d, -d))
		}
	}
	t.Logf("2px stripes /6: worst deviation %d", worst)
	if worst > 12 {
		t.Fatalf("stripes leave a %d level beat pattern", worst)
	}
}

func centroid(img *image.NRGBA, row int) float64 {
	var total, weighted float64
	for x := 0; x < img.Bounds().Dx(); x++ {
		v := float64(img.NRGBAAt(x, row).R)
		total += v
		weighted += v * (float64(x) + 0.5)
	}
	return weighted / total
}

func TestThinLineKeepsItsPosition(t *testing.T) {
	for _, tc := range []struct {
		w, lineX, tw int
	}{
		{100, 40, 50}, {100, 41, 25}, {101, 50, 33}, {64, 10, 128}, {64, 33, 200}, {300, 123, 20}, {1000, 777, 100},
	} {
		src := solid(tc.w, 8, color.NRGBA{A: 255})
		for y := 0; y < 8; y++ {
			src.SetNRGBA(tc.lineX, y, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
		}
		out := resample(t, src, tc.tw, 8)
		want := (float64(tc.lineX) + 0.5) * float64(tc.tw) / float64(tc.w)
		got := centroid(out, 4)
		t.Logf("line at %d of %d -> %d: centroid %.3f, ideal %.3f", tc.lineX, tc.w, tc.tw, got, want)
		if d := got - want; d > 0.15 || d < -0.15 {
			t.Fatalf("line moved: centroid %.3f, want %.3f", got, want)
		}
	}
}

func TestUpThenDownReturnsToTheSource(t *testing.T) {
	src := gradient(160, 120)
	up := resample(t, src, 320, 240)
	back := resample(t, up, 160, 120)
	diff, db := maxDiff(src, back), psnr(src, back)
	t.Logf("gradient 2x up then 2x down: max difference %d, PSNR %.1f dB", diff, db)
	if diff > 6 || db < 40 {
		t.Fatalf("round trip drifted: max %d, PSNR %.1f", diff, db)
	}
	detail := noisy(96, 96, 7, false)
	back = resample(t, resample(t, detail, 192, 192), 96, 96)
	db = psnr(detail, back)
	t.Logf("noise 2x up then 2x down: PSNR %.1f dB", db)
	if db < 20 {
		t.Fatalf("noise round trip PSNR %.1f", db)
	}
}

func TestIdentitySizeCopies(t *testing.T) {
	src := noisy(33, 21, 3, true)
	out := resample(t, src, 33, 21)
	if maxDiff(src, out) != 0 {
		t.Fatal("same size must copy")
	}
	out.Pix[0]++
	if src.Pix[0] == out.Pix[0] {
		t.Fatal("the copy shares memory with the source")
	}
}

func TestSubImageSource(t *testing.T) {
	big := noisy(120, 90, 5, true)
	sub := big.SubImage(image.Rect(17, 11, 83, 61)).(*image.NRGBA)
	flat := image.NewNRGBA(image.Rect(0, 0, 66, 50))
	for y := 0; y < 50; y++ {
		copy(flat.Pix[y*flat.Stride:y*flat.Stride+66*4], big.Pix[big.PixOffset(17, 11+y):])
	}
	if maxDiff(resample(t, sub, 30, 20), resample(t, flat, 30, 20)) != 0 {
		t.Fatal("a sub-image resamples differently from its copy")
	}
}

func TestWorkerCountDoesNotChangeTheOutput(t *testing.T) {
	src := noisy(257, 191, 11, true)
	withWorkers(t, 1)
	a := resample(t, src, 90, 301)
	withWorkers(t, 8)
	b := resample(t, src, 90, 301)
	if maxDiff(a, b) != 0 {
		t.Fatal("output depends on the number of workers")
	}
}

func TestBadArguments(t *testing.T) {
	src := solid(4, 4, color.NRGBA{A: 255})
	for _, sz := range [][2]int{{0, 4}, {4, 0}, {-1, 5}, {1 << 20, 1 << 20}} {
		if _, err := Resample(context.Background(), src, sz[0], sz[1]); !errors.Is(err, ErrSize) {
			t.Fatalf("%v: err %v", sz, err)
		}
	}
	if _, err := Resample(context.Background(), nil, 2, 2); !errors.Is(err, ErrEmptyPicture) {
		t.Fatal("nil source")
	}
	if _, err := Resample(context.Background(), image.NewNRGBA(image.Rectangle{}), 2, 2); !errors.Is(err, ErrEmptyPicture) {
		t.Fatal("empty source")
	}
}

func TestCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Resample(ctx, noisy(64, 64, 1, false), 32, 32); !errors.Is(err, context.Canceled) {
		t.Fatalf("err %v", err)
	}
}

func TestExtremeAspectRatios(t *testing.T) {
	for _, tc := range [][4]int{{1, 500, 1, 7}, {500, 1, 9, 1}, {1, 1, 40, 40}, {7, 3, 700, 300}, {4000, 3, 5, 3}, {3, 4000, 3, 5}} {
		src := noisy(tc[0], tc[1], 9, true)
		out := resample(t, src, tc[2], tc[3])
		if out.Bounds().Dx() != tc[2] {
			t.Fatal("size")
		}
	}
}

func TestGoldenChecksums(t *testing.T) {
	withWorkers(t, 3)
	cases := []struct {
		name string
		src  *image.NRGBA
		w, h int
		want string
	}{
		{"opaque noise down 2.5x", noisy(200, 150, 1, false), 80, 60, "e60fd7ca7fb19300"},
		{"alpha noise down 3x", noisy(180, 120, 2, true), 60, 40, "e2d81924a0053129"},
		{"gradient up 1.7x", gradient(100, 70), 170, 119, "fc7277f3a83ee5d7"},
		{"alpha noise up 3x", noisy(31, 29, 4, true), 93, 87, "35c0aaab2936151a"},
		{"opaque noise down 12x box prefilter", noisy(600, 480, 5, false), 50, 40, "7c27f682e23520c3"},
		{"alpha noise down 40x", noisy(800, 640, 6, true), 20, 16, "fdc73f0fc076287c"},
		{"gradient mixed", gradient(257, 129), 300, 33, "e087712afc454094"},
	}
	for _, c := range cases {
		got := sum(resample(t, c.src, c.w, c.h))
		if c.want == "GOLDEN" {
			t.Errorf("%s: %s", c.name, got)
			continue
		}
		if got != c.want {
			t.Errorf("%s: checksum %s, want %s", c.name, got, c.want)
		}
	}
}

func TestResampleWorkingSetIsOneBandPerWorker(t *testing.T) {
	withWorkers(t, 4)
	src := noisy(3000, 2000, 1, true)
	resample(t, src, 100, 66)
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	out := resample(t, src, 1500, 1000)
	runtime.ReadMemStats(&after)
	used := int64(after.TotalAlloc - before.TotalAlloc)
	extra := used - int64(len(out.Pix))
	t.Logf("2x shrink of 3000x2000 allocated %d bytes beyond the %d byte result", extra, len(out.Pix))
	if extra > 24<<20 {
		t.Fatalf("working memory %d bytes, expected a few bands", extra)
	}
}
