package imaging

import (
	"image"
	"math/rand"
	"testing"
)

func randomNRGBA(rng *rand.Rand, w, h int, translucent bool) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	rng.Read(img.Pix)
	if !translucent {
		for i := 3; i < len(img.Pix); i += 4 {
			img.Pix[i] = 255
		}
		return img
	}
	for i := 3; i < len(img.Pix); i += 4 {
		switch rng.Intn(4) {
		case 0:
			img.Pix[i] = 0
		case 1:
			img.Pix[i] = 255
		}
	}
	return img
}

func refHalve(src *image.NRGBA) *image.NRGBA {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	nw, nh := (w+1)/2, (h+1)/2
	out := image.NewNRGBA(image.Rect(0, 0, nw, nh))
	for y := 0; y < nh; y++ {
		for x := 0; x < nw; x++ {
			var sa, sr, sg, sb int
			for _, d := range [][2]int{{0, 0}, {1, 0}, {0, 1}, {1, 1}} {
				px, py := min(2*x+d[0], w-1), min(2*y+d[1], h-1)
				o := src.PixOffset(px, py)
				a := int(src.Pix[o+3])
				sa += a
				sr += a * int(src.Pix[o])
				sg += a * int(src.Pix[o+1])
				sb += a * int(src.Pix[o+2])
			}
			o := out.PixOffset(x, y)
			if sa == 0 {
				continue
			}
			out.Pix[o] = uint8((sr + sa/2) / sa)
			out.Pix[o+1] = uint8((sg + sa/2) / sa)
			out.Pix[o+2] = uint8((sb + sa/2) / sa)
			out.Pix[o+3] = uint8((sa + 2) / 4)
		}
	}
	return out
}

func TestBuildPyramidLevelSizes(t *testing.T) {
	cases := []struct {
		w, h, minSide int
		want          [][2]int
	}{
		{6000, 4000, 0, [][2]int{{6000, 4000}, {3000, 2000}, {1500, 1000}, {750, 500}, {375, 250}}},
		{1001, 513, 0, [][2]int{{1001, 513}, {501, 257}}},
		{255, 100, 0, [][2]int{{255, 100}}},
		{512, 512, 256, [][2]int{{512, 512}, {256, 256}}},
		{513, 513, 256, [][2]int{{513, 513}, {257, 257}}},
		{100, 90, 40, [][2]int{{100, 90}, {50, 45}}},
		{64, 64, 1, [][2]int{{64, 64}, {32, 32}, {16, 16}, {8, 8}, {4, 4}, {2, 2}, {1, 1}}},
		{1, 1, 1, [][2]int{{1, 1}}},
		{3, 1, 1, [][2]int{{3, 1}, {2, 1}, {1, 1}}},
	}
	for _, c := range cases {
		img := image.NewNRGBA(image.Rect(0, 0, c.w, c.h))
		lv := BuildPyramid(img, c.minSide)
		if len(lv) != len(c.want) {
			t.Fatalf("%dx%d min %d: got %d levels, want %d", c.w, c.h, c.minSide, len(lv), len(c.want))
		}
		for i, l := range lv {
			b := l.Bounds()
			if b.Dx() != c.want[i][0] || b.Dy() != c.want[i][1] {
				t.Fatalf("%dx%d min %d level %d: got %v, want %v", c.w, c.h, c.minSide, i, b.Size(), c.want[i])
			}
		}
		if lv[0] != img {
			t.Fatalf("level 0 must be the source image")
		}
	}
	if BuildPyramid(nil, 0) != nil {
		t.Fatal("nil in, nil out")
	}
}

func TestBuildPyramidMatchesReference(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for _, translucent := range []bool{false, true} {
		for _, dims := range [][2]int{{1000, 777}, {513, 1025}, {301, 3}, {3, 301}, {257, 257}, {2048, 2048}} {
			img := randomNRGBA(rng, dims[0], dims[1], translucent)
			lv := BuildPyramid(img, 8)
			if len(lv) < 3 {
				t.Fatalf("%v: expected several levels, got %d", dims, len(lv))
			}
			for i := 1; i < len(lv); i++ {
				want := refHalve(lv[i-1])
				if lv[i].Bounds() != want.Bounds() {
					t.Fatalf("%v level %d: bounds %v want %v", dims, i, lv[i].Bounds(), want.Bounds())
				}
				for j := range want.Pix {
					d := int(lv[i].Pix[j]) - int(want.Pix[j])
					if d < -1 || d > 1 {
						t.Fatalf("translucent=%v %v level %d: byte %d got %d want %d", translucent, dims, i, j, lv[i].Pix[j], want.Pix[j])
					}
				}
			}
		}
	}
}

func TestBuildPyramidOpaqueExactAverage(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	copy(img.Pix, []uint8{
		10, 20, 30, 255, 11, 21, 31, 255,
		12, 22, 32, 255, 14, 24, 34, 255,
	})
	lv := BuildPyramid(img, 1)
	if len(lv) != 2 {
		t.Fatalf("levels %d", len(lv))
	}
	want := []uint8{12, 22, 32, 255}
	for i, w := range want {
		if lv[1].Pix[i] != w {
			t.Fatalf("channel %d: got %d want %d", i, lv[1].Pix[i], w)
		}
	}
}

func TestBuildPyramidTransparentDoesNotBleed(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	copy(img.Pix, []uint8{
		255, 0, 0, 0, 0, 0, 255, 255,
		255, 0, 0, 0, 0, 0, 255, 255,
	})
	lv := BuildPyramid(img, 1)
	got := lv[1].Pix[:4]
	if got[0] != 0 || got[1] != 0 || got[2] != 255 {
		t.Fatalf("transparent red bled into the average: %v", got)
	}
	if got[3] != 128 {
		t.Fatalf("alpha: got %d want 128", got[3])
	}
}

func TestBuildPyramidSubImageSource(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	big := randomNRGBA(rng, 40, 30, false)
	sub := big.SubImage(image.Rect(5, 4, 35, 26)).(*image.NRGBA)
	lv := BuildPyramid(sub, 1)
	flat := image.NewNRGBA(image.Rect(0, 0, 30, 22))
	for y := 0; y < 22; y++ {
		copy(flat.Pix[y*flat.Stride:y*flat.Stride+120], sub.Pix[y*sub.Stride:y*sub.Stride+120])
	}
	ref := BuildPyramid(flat, 1)
	if len(lv) != len(ref) {
		t.Fatalf("levels %d vs %d", len(lv), len(ref))
	}
	for i := 1; i < len(lv); i++ {
		for j := range lv[i].Pix {
			if lv[i].Pix[j] != ref[i].Pix[j] {
				t.Fatalf("level %d byte %d differs", i, j)
			}
		}
	}
}

func BenchmarkBuildPyramid6000x4000(b *testing.B) {
	rng := rand.New(rand.NewSource(3))
	img := randomNRGBA(rng, 6000, 4000, false)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		BuildPyramid(img, 0)
	}
}

func BenchmarkBuildPyramidTranslucent6000x4000(b *testing.B) {
	rng := rand.New(rand.NewSource(3))
	img := randomNRGBA(rng, 6000, 4000, true)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		BuildPyramid(img, 0)
	}
}
