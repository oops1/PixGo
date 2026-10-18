package reduce

import (
	"image"
	"math/rand"
	"testing"
)

func randomDeep(rng *rand.Rand, w, h int, mode string) *image.NRGBA64 {
	img := image.NewNRGBA64(image.Rect(0, 0, w, h))
	rng.Read(img.Pix)
	for i := 6; i < len(img.Pix); i += 8 {
		switch mode {
		case "opaque":
			img.Pix[i], img.Pix[i+1] = 0xff, 0xff
		case "mixed":
			switch rng.Intn(4) {
			case 0:
				img.Pix[i], img.Pix[i+1] = 0, 0
			case 1:
				img.Pix[i], img.Pix[i+1] = 0xff, 0xff
			}
		}
	}
	return img
}

func referenceHalveDeep(src *image.NRGBA64) *image.NRGBA64 {
	w, h := src.Rect.Dx(), src.Rect.Dy()
	nw, nh := (w+1)/2, (h+1)/2
	out := image.NewNRGBA64(image.Rect(0, 0, nw, nh))
	for y := 0; y < nh; y++ {
		for x := 0; x < nw; x++ {
			var sums [3]uint64
			var alpha, total uint64
			opaque := true
			var colors [4][3]uint64
			var alphas [4]uint64
			for i, d := range [][2]int{{0, 0}, {1, 0}, {0, 1}, {1, 1}} {
				c := src.NRGBA64At(min(2*x+d[0], w-1), min(2*y+d[1], h-1))
				colors[i] = [3]uint64{uint64(c.R), uint64(c.G), uint64(c.B)}
				alphas[i] = uint64(c.A)
				opaque = opaque && c.A == 0xffff
				total += uint64(c.A)
			}
			var px [4]uint16
			switch {
			case opaque:
				for i := 0; i < 4; i++ {
					for c := 0; c < 3; c++ {
						sums[c] += colors[i][c]
					}
				}
				for c := 0; c < 3; c++ {
					px[c] = uint16((sums[c] + 2) / 4)
				}
				px[3] = 0xffff
			case total == 0:
			default:
				for i := 0; i < 4; i++ {
					for c := 0; c < 3; c++ {
						sums[c] += alphas[i] * colors[i][c]
					}
				}
				for c := 0; c < 3; c++ {
					px[c] = uint16((sums[c] + total/2) / total)
				}
				alpha = (total + 2) / 4
				px[3] = uint16(alpha)
			}
			o := out.PixOffset(x, y)
			for c := 0; c < 4; c++ {
				out.Pix[o+2*c], out.Pix[o+2*c+1] = byte(px[c]>>8), byte(px[c])
			}
		}
	}
	return out
}

func TestHalveDeepMatchesTheReference(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for _, mode := range []string{"opaque", "mixed", "random"} {
		for _, size := range [][2]int{{1, 1}, {2, 2}, {3, 5}, {16, 9}, {17, 1}, {1, 17}, {64, 63}} {
			src := randomDeep(rng, size[0], size[1], mode)
			got, want := HalveDeep(src), referenceHalveDeep(src)
			if got.Rect != want.Rect || string(got.Pix) != string(want.Pix) {
				t.Fatalf("%s %v: differs", mode, size)
			}
		}
	}
}

func TestDeepScratchLevelsEqualRepeatedHalving(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	var s ScratchDeep
	for _, mode := range []string{"opaque", "mixed"} {
		for _, size := range [][2]int{{37, 29}, {8, 8}, {100, 3}} {
			src := randomDeep(rng, size[0], size[1], mode)
			for n := 0; n <= 4; n++ {
				got := s.Levels(src, n)
				want := LevelsDeep(src, n)
				if got.Rect != want.Rect || string(got.Pix[:got.Rect.Dx()*got.Rect.Dy()*8]) != string(want.Pix) {
					t.Fatalf("%s %v level %d", mode, size, n)
				}
			}
		}
	}
}

func TestHalveDeepOfASubImageAndEmpty(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	src := randomDeep(rng, 20, 20, "mixed")
	view := src.SubImage(image.Rect(3, 5, 12, 14)).(*image.NRGBA64)
	copyOf := image.NewNRGBA64(image.Rect(0, 0, 9, 9))
	for y := 0; y < 9; y++ {
		copy(copyOf.Pix[y*copyOf.Stride:], src.Pix[src.PixOffset(3, 5+y):src.PixOffset(3, 5+y)+72])
	}
	if got, want := HalveDeep(view), HalveDeep(copyOf); string(got.Pix) != string(want.Pix) {
		t.Fatal("sub image")
	}
	if got := HalveDeep(image.NewNRGBA64(image.Rectangle{})); !got.Rect.Empty() {
		t.Fatal("empty")
	}
}
