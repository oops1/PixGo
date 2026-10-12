package thumbs

import (
	"image"
	"image/color"
	"math"
	"math/rand"
	"testing"
)

func TestFitSize(t *testing.T) {
	cases := []struct {
		name         string
		w, h, side   int
		wantW, wantH int
	}{
		{"landscape 6000x4000", 6000, 4000, 256, 256, 171},
		{"portrait 4000x6000", 4000, 6000, 256, 171, 256},
		{"square", 3000, 3000, 256, 256, 256},
		{"tiny stays unscaled", 100, 50, 256, 100, 50},
		{"exactly the side", 256, 100, 256, 256, 100},
		{"1x1", 1, 1, 256, 1, 1},
		{"1xN tall", 1, 5000, 256, 1, 256},
		{"Nx1 wide", 5000, 1, 256, 256, 1},
		{"just above the side", 257, 256, 256, 256, 255},
		{"96 px", 1920, 1080, 96, 96, 54},
		{"384 px", 1080, 1920, 384, 216, 384},
		{"zero width", 0, 10, 256, 0, 0},
		{"zero side", 10, 10, 0, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w, h := fitSize(tc.w, tc.h, tc.side)
			if w != tc.wantW || h != tc.wantH {
				t.Fatalf("fitSize(%d,%d,%d) = %dx%d, want %dx%d", tc.w, tc.h, tc.side, w, h, tc.wantW, tc.wantH)
			}
		})
	}
}

func TestScaleHint(t *testing.T) {
	cases := []struct {
		longest, side, want int
	}{
		{6000, 256, 8},
		{6000, 1000, 4},
		{6000, 2000, 2},
		{6000, 3000, 2},
		{6000, 3001, 1},
		{512, 256, 2},
		{511, 256, 2},
		{510, 256, 1},
		{255, 256, 1},
		{2048, 256, 8},
		{2047, 256, 8},
		{2040, 256, 4},
		{100, 0, 1},
	}
	for _, tc := range cases {
		if got := scaleHint(tc.longest, tc.side); got != tc.want {
			t.Errorf("scaleHint(%d,%d) = %d, want %d", tc.longest, tc.side, got, tc.want)
		}
	}
}

func TestSizes(t *testing.T) {
	want := []Size{96, 128, 160, 192, 256, 384}
	got := Sizes()
	if len(got) != len(want) {
		t.Fatalf("Sizes() = %v", got)
	}
	for i := range want {
		if got[i] != want[i] || got[i].Px() != int(want[i]) {
			t.Fatalf("Sizes()[%d] = %v", i, got[i])
		}
	}
	nearest := map[int]Size{1: 96, 96: 96, 97: 128, 190: 192, 256: 256, 257: 384, 5000: 384}
	for px, want := range nearest {
		if got := Nearest(px); got != want {
			t.Errorf("Nearest(%d) = %d, want %d", px, got, want)
		}
	}
}

func TestResizeAreaDimensions(t *testing.T) {
	cases := []struct {
		name         string
		sw, sh       int
		dw, dh       int
		wantW, wantH int
	}{
		{"6000x4000 to 256", 600, 400, 256, 171, 256, 171},
		{"portrait", 400, 600, 171, 256, 171, 256},
		{"identity", 50, 40, 50, 40, 50, 40},
		{"1x1", 1, 1, 1, 1, 1, 1},
		{"1xN to 1xM", 1, 1000, 1, 17, 1, 17},
		{"Nx1 to Mx1", 1000, 1, 17, 1, 17, 1},
		{"horizontal only", 100, 10, 25, 10, 25, 10},
		{"vertical only", 10, 100, 10, 25, 10, 25},
		{"odd ratio", 97, 61, 31, 19, 31, 19},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := solid(tc.sw, tc.sh, color.NRGBA{R: 10, G: 20, B: 30, A: 255})
			out := resizeArea(src, tc.dw, tc.dh)
			if out.Bounds().Dx() != tc.wantW || out.Bounds().Dy() != tc.wantH {
				t.Fatalf("got %v", out.Bounds())
			}
			if out.Bounds().Min != (image.Point{}) {
				t.Fatalf("origin %v", out.Bounds().Min)
			}
		})
	}
}

func TestResizeAreaDegenerate(t *testing.T) {
	if got := resizeArea(nil, 4, 4); !got.Bounds().Empty() {
		t.Fatal("nil source must give an empty image")
	}
	if got := resizeArea(solid(4, 4, color.NRGBA{A: 255}), 0, 4); !got.Bounds().Empty() {
		t.Fatal("zero target must give an empty image")
	}
	if got := resizeArea(image.NewNRGBA(image.Rectangle{}), 4, 4); !got.Bounds().Empty() {
		t.Fatal("empty source must give an empty image")
	}
}

func TestResizeAreaPreservesSolidColour(t *testing.T) {
	colours := []color.NRGBA{
		{R: 200, G: 100, B: 50, A: 255},
		{R: 0, G: 0, B: 0, A: 255},
		{R: 255, G: 255, B: 255, A: 255},
		{R: 255, G: 0, B: 0, A: 128},
		{R: 17, G: 99, B: 201, A: 7},
	}
	ratios := [][4]int{{64, 48, 7, 5}, {1000, 3, 333, 2}, {3, 1000, 2, 333}, {257, 257, 256, 256}, {90, 90, 30, 30}, {91, 7, 13, 1}}
	for _, c := range colours {
		for _, r := range ratios {
			out := resizeArea(solid(r[0], r[1], c), r[2], r[3])
			want := premul(c)
			for y := 0; y < r[3]; y++ {
				for x := 0; x < r[2]; x++ {
					got := out.RGBAAt(x, y)
					if got != want {
						t.Fatalf("%v %v: pixel (%d,%d) = %v, want %v", c, r, x, y, got, want)
					}
				}
			}
		}
	}
}

func premul(c color.NRGBA) color.RGBA {
	a := uint32(c.A)
	return color.RGBA{
		R: uint8((uint32(c.R)*a + 127) / 255),
		G: uint8((uint32(c.G)*a + 127) / 255),
		B: uint8((uint32(c.B)*a + 127) / 255),
		A: c.A,
	}
}

func TestResizeAreaIdentityPremultiplies(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 4, 1))
	src.SetNRGBA(0, 0, color.NRGBA{R: 255, G: 128, B: 0, A: 255})
	src.SetNRGBA(1, 0, color.NRGBA{R: 255, G: 128, B: 0, A: 128})
	src.SetNRGBA(2, 0, color.NRGBA{R: 255, G: 255, B: 255, A: 0})
	src.SetNRGBA(3, 0, color.NRGBA{R: 10, G: 20, B: 30, A: 1})
	out := resizeArea(src, 4, 1)
	for x := 0; x < 4; x++ {
		if got, want := out.RGBAAt(x, 0), premul(src.NRGBAAt(x, 0)); got != want {
			t.Errorf("pixel %d = %v, want %v", x, got, want)
		}
	}
}

func TestResizeAreaTransparentDoesNotBleed(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	src.SetNRGBA(0, 0, color.NRGBA{R: 255, G: 0, B: 0, A: 255})
	src.SetNRGBA(1, 0, color.NRGBA{R: 0, G: 255, B: 0, A: 0})
	out := resizeArea(src, 1, 1)
	got := out.RGBAAt(0, 0)
	if got.G != 0 {
		t.Fatalf("transparent green bled into the result: %v", got)
	}
	if !near(got.R, 128, 1) || !near(got.A, 128, 1) {
		t.Fatalf("got %v, want about {128 0 0 128}", got)
	}
}

func TestResizeAreaFullyTransparentStaysClear(t *testing.T) {
	src := solid(40, 30, color.NRGBA{R: 255, G: 255, B: 255, A: 0})
	out := resizeArea(src, 10, 8)
	for i, v := range out.Pix {
		if v != 0 {
			t.Fatalf("byte %d = %d, want 0", i, v)
		}
	}
}

func TestResizeAreaOutputIsValidPremultiplied(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	src := image.NewNRGBA(image.Rect(0, 0, 123, 77))
	for i := range src.Pix {
		src.Pix[i] = uint8(rng.Intn(256))
	}
	out := resizeArea(src, 40, 25)
	for i := 0; i < len(out.Pix); i += 4 {
		a := out.Pix[i+3]
		if out.Pix[i] > a || out.Pix[i+1] > a || out.Pix[i+2] > a {
			t.Fatalf("pixel %d is not premultiplied: %v", i/4, out.Pix[i:i+4])
		}
	}
}

func checkerboard(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			v := uint8(0)
			if (x+y)%2 == 0 {
				v = 255
			}
			img.SetNRGBA(x, y, color.NRGBA{R: v, G: v, B: v, A: 255})
		}
	}
	return img
}

func TestResizeAreaCheckerboardAverages(t *testing.T) {
	for _, factor := range []int{2, 4, 8} {
		src := checkerboard(64, 48)
		out := resizeArea(src, 64/factor, 48/factor)
		for y := 0; y < out.Bounds().Dy(); y++ {
			for x := 0; x < out.Bounds().Dx(); x++ {
				p := out.RGBAAt(x, y)
				if !near(p.R, 128, 1) || !near(p.G, 128, 1) || !near(p.B, 128, 1) || p.A != 255 {
					t.Fatalf("factor %d: pixel (%d,%d) = %v, want mid grey", factor, x, y, p)
				}
			}
		}
	}
}

func TestResizeAreaBlockAverage(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	vals := []uint8{
		0, 100, 10, 20,
		200, 100, 30, 40,
		5, 5, 255, 255,
		5, 5, 255, 255,
	}
	for i, v := range vals {
		src.SetNRGBA(i%4, i/4, color.NRGBA{R: v, G: v, B: v, A: 255})
	}
	out := resizeArea(src, 2, 2)
	want := []uint8{100, 25, 5, 255}
	for i, w := range want {
		got := out.RGBAAt(i%2, i/2)
		if got.R != w || got.G != w || got.B != w || got.A != 255 {
			t.Errorf("block %d = %v, want %d", i, got, w)
		}
	}
}

func referenceArea(src *image.NRGBA, dw, dh int) *image.RGBA {
	sw, sh := src.Bounds().Dx(), src.Bounds().Dy()
	out := image.NewRGBA(image.Rect(0, 0, dw, dh))
	fx := float64(sw) / float64(dw)
	fy := float64(sh) / float64(dh)
	for y := 0; y < dh; y++ {
		for x := 0; x < dw; x++ {
			x0, x1 := float64(x)*fx, float64(x+1)*fx
			y0, y1 := float64(y)*fy, float64(y+1)*fy
			var r, g, b, a, total float64
			for sy := int(math.Floor(y0)); sy < int(math.Ceil(y1)) && sy < sh; sy++ {
				wy := math.Min(float64(sy+1), y1) - math.Max(float64(sy), y0)
				for sx := int(math.Floor(x0)); sx < int(math.Ceil(x1)) && sx < sw; sx++ {
					wx := math.Min(float64(sx+1), x1) - math.Max(float64(sx), x0)
					w := wx * wy
					p := src.NRGBAAt(sx, sy)
					al := float64(p.A) / 255
					r += w * float64(p.R) * al
					g += w * float64(p.G) * al
					b += w * float64(p.B) * al
					a += w * float64(p.A)
					total += w
				}
			}
			out.SetRGBA(x, y, color.RGBA{
				R: uint8(math.Round(r / total)),
				G: uint8(math.Round(g / total)),
				B: uint8(math.Round(b / total)),
				A: uint8(math.Round(a / total)),
			})
		}
	}
	return out
}

func TestResizeAreaMatchesReference(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	sizes := [][4]int{
		{100, 80, 33, 27},
		{97, 61, 31, 19},
		{64, 64, 16, 16},
		{200, 3, 50, 3},
		{3, 200, 3, 50},
		{51, 51, 50, 50},
		{128, 96, 127, 1},
		{1, 40, 1, 7},
	}
	for _, s := range sizes {
		src := image.NewNRGBA(image.Rect(0, 0, s[0], s[1]))
		for i := range src.Pix {
			src.Pix[i] = uint8(rng.Intn(256))
		}
		got := resizeArea(src, s[2], s[3])
		want := referenceArea(src, s[2], s[3])
		worst := 0
		for i := range got.Pix {
			d := int(got.Pix[i]) - int(want.Pix[i])
			if d < 0 {
				d = -d
			}
			worst = max(worst, d)
		}
		if worst > 1 {
			t.Errorf("%v: max deviation from the float reference is %d", s, worst)
		}
	}
}

func TestResizeAreaHonoursSubImageBounds(t *testing.T) {
	big := checkerboard(40, 40)
	for y := 0; y < 40; y++ {
		for x := 0; x < 40; x++ {
			if x >= 10 && x < 30 && y >= 10 && y < 30 {
				big.SetNRGBA(x, y, color.NRGBA{R: 200, G: 50, B: 50, A: 255})
			}
		}
	}
	sub := big.SubImage(image.Rect(10, 10, 30, 30)).(*image.NRGBA)
	out := resizeArea(sub, 5, 5)
	for y := 0; y < 5; y++ {
		for x := 0; x < 5; x++ {
			if p := out.RGBAAt(x, y); p != (color.RGBA{R: 200, G: 50, B: 50, A: 255}) {
				t.Fatalf("pixel (%d,%d) = %v", x, y, p)
			}
		}
	}
}

func TestResizeAreaUpscaleDoesNotCrash(t *testing.T) {
	src := checkerboard(3, 3)
	out := resizeArea(src, 7, 5)
	if out.Bounds().Dx() != 7 || out.Bounds().Dy() != 5 {
		t.Fatalf("got %v", out.Bounds())
	}
	for i := 3; i < len(out.Pix); i += 4 {
		if out.Pix[i] != 255 {
			t.Fatal("opaque source must stay opaque")
		}
	}
}

func TestIsOpaque(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 3, 2))
	for i := 3; i < len(img.Pix); i += 4 {
		img.Pix[i] = 255
	}
	if !isOpaque(img) {
		t.Fatal("expected opaque")
	}
	img.Pix[len(img.Pix)-1] = 254
	if isOpaque(img) {
		t.Fatal("expected translucent")
	}
}
