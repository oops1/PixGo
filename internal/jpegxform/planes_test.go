package jpegxform

import (
	"math"
	"testing"
)

type plane struct {
	w, h int
	pix  []float64
}

var idctBasis = func() [8][8]float64 {
	var t [8][8]float64
	for x := 0; x < 8; x++ {
		for u := 0; u < 8; u++ {
			c := 0.5
			if u == 0 {
				c = 0.5 / math.Sqrt2
			}
			t[x][u] = c * math.Cos(float64(2*x+1)*float64(u)*math.Pi/16)
		}
	}
	return t
}()

func dequantised(h *header, ci int) []int {
	c := h.fr.comps[ci]
	var natural [64]int
	for i := 0; i < 64; i++ {
		natural[zigzag[i]] = int(h.quant[c.quant].values[i])
	}
	return natural[:]
}

func planeOf(h *header, g grid, ci int) plane {
	p := plane{w: g.wide * 8, h: g.high * 8}
	p.pix = make([]float64, p.w*p.h)
	q := dequantised(h, ci)
	for by := 0; by < g.high; by++ {
		for bx := 0; bx < g.wide; bx++ {
			blk := g.block(bx, by)
			for y := 0; y < 8; y++ {
				for x := 0; x < 8; x++ {
					sum := 0.0
					for v := 0; v < 8; v++ {
						for u := 0; u < 8; u++ {
							f := float64(int(blk[v*8+u]) * q[v*8+u])
							if f != 0 {
								sum += idctBasis[y][v] * idctBasis[x][u] * f
							}
						}
					}
					p.pix[(by*8+y)*p.w+bx*8+x] = sum
				}
			}
		}
	}
	return p
}

func (p plane) crop(w, h int) plane {
	out := plane{w: w, h: h, pix: make([]float64, w*h)}
	for y := 0; y < h; y++ {
		copy(out.pix[y*w:(y+1)*w], p.pix[y*p.w:y*p.w+w])
	}
	return out
}

func (p plane) apply(op Op) plane {
	o := op.Orientation()
	ow, oh := o.Size(p.w, p.h)
	out := plane{w: ow, h: oh, pix: make([]float64, ow*oh)}
	for y := 0; y < oh; y++ {
		for x := 0; x < ow; x++ {
			sx, sy := o.ToSource(float64(x)+.5, float64(y)+.5, float64(p.w), float64(p.h))
			out.pix[y*ow+x] = p.pix[int(math.Floor(sy))*p.w+int(math.Floor(sx))]
		}
	}
	return out
}

func maxPlaneDiff(a, b plane) float64 {
	if a.w != b.w || a.h != b.h {
		return math.Inf(1)
	}
	worst := 0.0
	for i := range a.pix {
		worst = max(worst, math.Abs(a.pix[i]-b.pix[i]))
	}
	return worst
}

func checkPlanes(t *testing.T, in, out []byte, op Op, srcMCUCropW, srcMCUCropH int) {
	t.Helper()
	hin, gin := parseCoefficients(t, in)
	hout, gout := parseCoefficients(t, out)
	for ci := range hin.fr.comps {
		c := hin.fr.comps[ci]
		cw := srcMCUCropW * c.h * 8
		ch := srcMCUCropH * c.v * 8
		src := planeOf(hin, gin[ci], ci).crop(cw, ch)
		want := src.apply(op)
		got := planeOf(hout, gout[ci], ci)
		if d := maxPlaneDiff(got, want); d > 1e-6 {
			t.Fatalf("component %d: the planes differ by %g (got %dx%d, want %dx%d)", ci, d, got.w, got.h, want.w, want.h)
		}
	}
}
