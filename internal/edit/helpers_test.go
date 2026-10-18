package edit

import (
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/oops1/PixGo/internal/par"
)

func solid(w, h int, c color.NRGBA) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, c.A
	}
	return img
}

func gradient(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			fx, fy := float64(x)/float64(w), float64(y)/float64(h)
			r := 127.5 + 127.5*math.Sin(6.28*fx*1.5)
			g := 127.5 + 127.5*math.Cos(6.28*fy*2)
			b := 255 * (fx + fy) / 2
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: 255})
		}
	}
	return img
}

func noisy(w, h int, seed uint32, alpha bool) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	state := seed
	next := func() uint8 {
		state = state*1664525 + 1013904223
		return uint8(state >> 24)
	}
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2] = next(), next(), next()
		img.Pix[i+3] = 255
		if alpha {
			img.Pix[i+3] = next()
		}
	}
	return img
}

func checker(w, h int) *image.NRGBA {
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

func sum(img *image.NRGBA) string {
	h := sha256.New()
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		o := img.PixOffset(b.Min.X, y)
		h.Write(img.Pix[o : o+b.Dx()*4])
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

func maxDiff(a, b *image.NRGBA) int {
	if a.Bounds().Size() != b.Bounds().Size() {
		return 1 << 20
	}
	m := 0
	for y := 0; y < a.Bounds().Dy(); y++ {
		for x := 0; x < a.Bounds().Dx(); x++ {
			pa, pb := a.NRGBAAt(x, y), b.NRGBAAt(x, y)
			for _, d := range [4]int{int(pa.R) - int(pb.R), int(pa.G) - int(pb.G), int(pa.B) - int(pb.B), int(pa.A) - int(pb.A)} {
				if d < 0 {
					d = -d
				}
				m = max(m, d)
			}
		}
	}
	return m
}

func psnr(a, b *image.NRGBA) float64 {
	var se float64
	n := 0
	for i := 0; i < len(a.Pix); i += 4 {
		for c := 0; c < 3; c++ {
			d := float64(a.Pix[i+c]) - float64(b.Pix[i+c])
			se += d * d
			n++
		}
	}
	if se == 0 {
		return 99
	}
	return 10 * math.Log10(255*255/(se/float64(n)))
}

func withWorkers(t *testing.T, n int) {
	t.Helper()
	par.Set(n)
	t.Cleanup(func() { par.Set(0) })
}
