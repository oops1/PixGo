package reduce

import (
	"image"
	"math/rand"
	"testing"
)

func randomImage(rng *rand.Rand, w, h int, mode string) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	rng.Read(img.Pix)
	for i := 3; i < len(img.Pix); i += 4 {
		switch mode {
		case "opaque":
			img.Pix[i] = 255
		case "mixed":
			switch rng.Intn(4) {
			case 0:
				img.Pix[i] = 0
			case 1:
				img.Pix[i] = 255
			}
		}
	}
	return img
}

func referenceHalve(src *image.NRGBA) *image.NRGBA {
	w, h := src.Rect.Dx(), src.Rect.Dy()
	nw, nh := (w+1)/2, (h+1)/2
	out := image.NewNRGBA(image.Rect(0, 0, nw, nh))
	for y := 0; y < nh; y++ {
		for x := 0; x < nw; x++ {
			var alpha, red, green, blue, count int
			opaque := true
			for _, d := range [][2]int{{0, 0}, {1, 0}, {0, 1}, {1, 1}} {
				px, py := min(2*x+d[0], w-1), min(2*y+d[1], h-1)
				c := src.NRGBAAt(src.Rect.Min.X+px, src.Rect.Min.Y+py)
				a := int(c.A)
				opaque = opaque && a == 255
				alpha += a
				red += a * int(c.R)
				green += a * int(c.G)
				blue += a * int(c.B)
				count++
			}
			o := out.PixOffset(x, y)
			if opaque {
				sum := func(channel int) uint8 {
					total := 0
					for _, d := range [][2]int{{0, 0}, {1, 0}, {0, 1}, {1, 1}} {
						px, py := min(2*x+d[0], w-1), min(2*y+d[1], h-1)
						c := src.NRGBAAt(src.Rect.Min.X+px, src.Rect.Min.Y+py)
						total += int([]uint8{c.R, c.G, c.B}[channel])
					}
					return uint8((total + 2) >> 2)
				}
				out.Pix[o], out.Pix[o+1], out.Pix[o+2], out.Pix[o+3] = sum(0), sum(1), sum(2), 255
				continue
			}
			if alpha == 0 {
				continue
			}
			out.Pix[o] = uint8((red + alpha/2) / alpha)
			out.Pix[o+1] = uint8((green + alpha/2) / alpha)
			out.Pix[o+2] = uint8((blue + alpha/2) / alpha)
			out.Pix[o+3] = uint8((alpha + 2) >> 2)
		}
	}
	return out
}

func equal(a, b *image.NRGBA) bool {
	if a.Rect != b.Rect {
		return false
	}
	for y := 0; y < a.Rect.Dy(); y++ {
		ra := a.Pix[y*a.Stride : y*a.Stride+a.Rect.Dx()*4]
		rb := b.Pix[y*b.Stride : y*b.Stride+b.Rect.Dx()*4]
		for i := range ra {
			if ra[i] != rb[i] {
				return false
			}
		}
	}
	return true
}

func TestHalveMatchesTheReferenceRule(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for _, mode := range []string{"opaque", "mixed", "random"} {
		for _, size := range [][2]int{{1, 1}, {2, 2}, {3, 1}, {1, 3}, {5, 7}, {16, 16}, {17, 33}, {100, 3}, {257, 129}} {
			src := randomImage(rng, size[0], size[1], mode)
			if got, want := Halve(src), referenceHalve(src); !equal(got, want) {
				t.Fatalf("%s %v: differs from the reference", mode, size)
			}
		}
	}
}

func TestHalveKnownValues(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	copy(src.Pix, []uint8{10, 20, 30, 255, 11, 21, 31, 255, 12, 22, 32, 255, 14, 24, 34, 255})
	got := Halve(src)
	for i, want := range []uint8{12, 22, 32, 255} {
		if got.Pix[i] != want {
			t.Fatalf("channel %d: %d, want %d", i, got.Pix[i], want)
		}
	}
	src = image.NewNRGBA(image.Rect(0, 0, 2, 2))
	copy(src.Pix, []uint8{255, 0, 0, 0, 0, 0, 255, 255, 255, 0, 0, 0, 0, 0, 255, 255})
	got = Halve(src)
	if got.Pix[0] != 0 || got.Pix[1] != 0 || got.Pix[2] != 255 || got.Pix[3] != 128 {
		t.Fatalf("transparent pixels bled into the average: %v", got.Pix[:4])
	}
	src = image.NewNRGBA(image.Rect(0, 0, 2, 2))
	if got := Halve(src); got.Pix[0]|got.Pix[1]|got.Pix[2]|got.Pix[3] != 0 {
		t.Fatalf("fully transparent block: %v", got.Pix[:4])
	}
}

func TestHalveRepeatsTheLastRowAndColumn(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 3, 3))
	for i := 0; i < 9; i++ {
		src.Pix[i*4], src.Pix[i*4+1], src.Pix[i*4+2], src.Pix[i*4+3] = uint8(i*20), 0, 0, 255
	}
	got := Halve(src)
	if got.Rect.Dx() != 2 || got.Rect.Dy() != 2 {
		t.Fatalf("size %v", got.Rect)
	}
	want := []uint8{(0 + 20 + 60 + 80 + 2) >> 2, (40 + 40 + 100 + 100 + 2) >> 2, (120 + 140 + 120 + 140 + 2) >> 2, (160 + 160 + 160 + 160 + 2) >> 2}
	for i, w := range want {
		if got.Pix[i*4] != w {
			t.Fatalf("pixel %d: %d, want %d", i, got.Pix[i*4], w)
		}
	}
}

func TestHalveOfSubImagesAndEmptyImages(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	big := randomImage(rng, 40, 30, "mixed")
	sub := big.SubImage(image.Rect(5, 4, 36, 27)).(*image.NRGBA)
	flat := image.NewNRGBA(image.Rect(0, 0, 31, 23))
	for y := 0; y < 23; y++ {
		copy(flat.Pix[y*flat.Stride:], sub.Pix[y*sub.Stride:y*sub.Stride+31*4])
	}
	if !equal(Halve(sub), Halve(flat)) {
		t.Fatal("a sub-image halves differently from its copy")
	}
	if got := Halve(image.NewNRGBA(image.Rectangle{})); got.Rect.Dx() != 0 || got.Rect.Dy() != 0 {
		t.Fatalf("empty: %v", got.Rect)
	}
	if got := Levels(image.NewNRGBA(image.Rectangle{}), 3); got.Rect.Dx() != 0 {
		t.Fatalf("empty levels: %v", got.Rect)
	}
}

func TestLevelsAreRepeatedHalvings(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	src := randomImage(rng, 203, 117, "mixed")
	if Levels(src, 0) != src || Levels(src, -2) != src {
		t.Fatal("level 0 must be the source itself")
	}
	level := src
	for n := 1; n <= 9; n++ {
		level = Halve(level)
		got := Levels(src, n)
		if !equal(got, level) {
			t.Fatalf("level %d differs", n)
		}
		wantW, wantH := (203+(1<<uint(n))-1)>>uint(n), (117+(1<<uint(n))-1)>>uint(n)
		if got.Rect.Dx() != wantW || got.Rect.Dy() != wantH {
			t.Fatalf("level %d is %v, want %dx%d", n, got.Rect, wantW, wantH)
		}
	}
}

func TestScratchMatchesLevelsAndIsReusable(t *testing.T) {
	rng := rand.New(rand.NewSource(4))
	var scratch Scratch
	for _, size := range [][2]int{{64, 64}, {13, 9}, {200, 150}, {7, 300}, {64, 64}, {1, 1}} {
		src := randomImage(rng, size[0], size[1], "mixed")
		for n := 0; n <= 4; n++ {
			if !equal(scratch.Levels(src, n), Levels(src, n)) {
				t.Fatalf("%v level %d differs", size, n)
			}
		}
	}
	if got := scratch.Levels(image.NewNRGBA(image.Rectangle{}), 2); got.Rect.Dx() != 0 {
		t.Fatalf("empty: %v", got.Rect)
	}
}

func BenchmarkHalve6000x4000(b *testing.B) {
	src := randomImage(rand.New(rand.NewSource(5)), 6000, 4000, "opaque")
	b.SetBytes(int64(len(src.Pix)))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		Halve(src)
	}
}

func BenchmarkHalveTranslucent6000x4000(b *testing.B) {
	src := randomImage(rand.New(rand.NewSource(5)), 6000, 4000, "mixed")
	b.SetBytes(int64(len(src.Pix)))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		Halve(src)
	}
}
