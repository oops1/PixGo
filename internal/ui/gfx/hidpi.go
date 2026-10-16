package gfx

import (
	"image"
	"image/color"
	"math"

	"github.com/oops1/headless-gui/v3/widget"
)

const maskSamples = 4

func physEdge(v int, k float64) int { return int(math.Round(float64(v) * k)) }

func PhysRect(r image.Rectangle, k float64) image.Rectangle {
	return image.Rect(physEdge(r.Min.X, k), physEdge(r.Min.Y, k), physEdge(r.Max.X, k), physEdge(r.Max.Y, k))
}

func physWidth(width, k float64) float64 {
	w := width * k
	if width >= 1 {
		return math.Max(1, math.Round(w))
	}
	return w
}

func setPremul(img *image.RGBA, x, y int, premul color.RGBA, cov float64) {
	o := img.PixOffset(x, y)
	px := img.Pix[o : o+4 : o+4]
	if cov >= 0.999 {
		px[0], px[1], px[2], px[3] = premul.R, premul.G, premul.B, premul.A
		return
	}
	s := scalePremul(premul, cov)
	px[0], px[1], px[2], px[3] = s.R, s.G, s.B, s.A
}

func fillHi(c widget.DrawContext, bounds image.Rectangle, k float64, coverage func(px, py int) float64, premul color.RGBA) {
	clip := c.Clip().Intersect(bounds)
	if clip.Empty() {
		return
	}
	p := PhysRect(clip, k)
	if p.Empty() {
		return
	}
	img := image.NewRGBA(image.Rect(0, 0, p.Dx(), p.Dy()))
	for y := 0; y < p.Dy(); y++ {
		for x := 0; x < p.Dx(); x++ {
			if cov := coverage(p.Min.X+x, p.Min.Y+y); cov > 0.001 {
				setPremul(img, x, y, premul, cov)
			}
		}
	}
	c.DrawImageScaled(img, clip.Min.X, clip.Min.Y, clip.Dx(), clip.Dy())
}

func distanceCoverage(d Distance, k float64) func(px, py int) float64 {
	return func(px, py int) float64 {
		return clamp01(0.5 - d((float64(px)+.5)/k, (float64(py)+.5)/k)*k)
	}
}

func strokeCoverage(d Distance, k, width float64) func(px, py int) float64 {
	wp := physWidth(width, k)
	half := wp / 2
	return func(px, py int) float64 {
		v := d((float64(px)+.5)/k, (float64(py)+.5)/k) * k
		return clamp01(0.5 - (math.Abs(v+half) - half))
	}
}

func roundCoverage(p image.Rectangle, radius float64) func(px, py int) float64 {
	cx := float64(p.Min.X+p.Max.X) / 2
	cy := float64(p.Min.Y+p.Max.Y) / 2
	hw := float64(p.Dx()) / 2
	hh := float64(p.Dy()) / 2
	radius = math.Min(radius, math.Min(hw, hh))
	iw, ih := hw-radius, hh-radius
	return func(px, py int) float64 {
		qx := math.Abs(float64(px)+.5-cx) - iw
		qy := math.Abs(float64(py)+.5-cy) - ih
		if qx <= -1 && qy <= -1 {
			return 1
		}
		d := math.Hypot(math.Max(qx, 0), math.Max(qy, 0)) + math.Min(math.Max(qx, qy), 0) - radius
		return clamp01(0.5 - d)
	}
}

func strokeRoundCoverage(p image.Rectangle, radius, wp float64) func(px, py int) float64 {
	cx := float64(p.Min.X+p.Max.X) / 2
	cy := float64(p.Min.Y+p.Max.Y) / 2
	hw := float64(p.Dx()) / 2
	hh := float64(p.Dy()) / 2
	radius = math.Min(radius, math.Min(hw, hh))
	iw, ih := hw-radius, hh-radius
	half := wp / 2
	return func(px, py int) float64 {
		qx := math.Abs(float64(px)+.5-cx) - iw
		qy := math.Abs(float64(py)+.5-cy) - ih
		if qx <= -wp-1 && qy <= -wp-1 {
			return 0
		}
		d := math.Hypot(math.Max(qx, 0), math.Max(qy, 0)) + math.Min(math.Max(qx, qy), 0) - radius
		return clamp01(0.5 - (math.Abs(d+half) - half))
	}
}

func fillRoundHi(c widget.DrawContext, r image.Rectangle, radius float64, k float64, premul color.RGBA) {
	fillHi(c, r, k, roundCoverage(PhysRect(r, k), radius*k), premul)
}

func strokeRoundHi(c widget.DrawContext, r image.Rectangle, radius, width float64, k float64, premul color.RGBA) {
	fillHi(c, r, k, strokeRoundCoverage(PhysRect(r, k), radius*k, physWidth(width, k)), premul)
}

func fillDistanceHi(c widget.DrawContext, bounds image.Rectangle, d Distance, k float64, premul color.RGBA) {
	fillHi(c, bounds, k, distanceCoverage(d, k), premul)
}

func strokeDistanceHi(c widget.DrawContext, bounds image.Rectangle, d Distance, width float64, k float64, premul color.RGBA) {
	fillHi(c, bounds, k, strokeCoverage(d, k, width), premul)
}

func MaskHi(s Shape, pw, ph int, k float64) []uint8 {
	n := maskSamples
	out := make([]uint8, pw*ph)
	step := 1 / (float64(n) * k)
	for j := 0; j < ph; j++ {
		for i := 0; i < pw; i++ {
			in := 0
			x0, y0 := float64(i)/k, float64(j)/k
			for sy := 0; sy < n; sy++ {
				for sx := 0; sx < n; sx++ {
					if s(x0+(float64(sx)+.5)*step, y0+(float64(sy)+.5)*step) {
						in++
					}
				}
			}
			out[j*pw+i] = uint8(in * 255 / (n * n))
		}
	}
	return out
}

func PaintMaskHi(c widget.DrawContext, m []uint8, x, y, w, h int, k float64, tint func(row, rows int) color.RGBA) {
	p := PhysRect(R(x, y, w, h), k)
	pw, ph := p.Dx(), p.Dy()
	if pw <= 0 || ph <= 0 || len(m) < pw*ph || !Visible(c, R(x, y, w, h)) {
		return
	}
	img := image.NewRGBA(image.Rect(0, 0, pw, ph))
	for j := 0; j < ph; j++ {
		col := tint(j, ph)
		for i := 0; i < pw; i++ {
			a := m[j*pw+i]
			if a == 0 {
				continue
			}
			if a == 255 && col.A == 255 {
				setPremul(img, i, j, col, 1)
				continue
			}
			setPremul(img, i, j, Alpha(col, float64(a)/255*float64(col.A)/255), 1)
		}
	}
	c.DrawImageScaled(img, x, y, w, h)
}

func HiPhysSize(x, y, w, h int, k float64) (int, int) {
	p := PhysRect(R(x, y, w, h), k)
	return p.Dx(), p.Dy()
}

type RectF struct{ X0, Y0, X1, Y1 float64 }

func (r RectF) Bounds() image.Rectangle {
	return image.Rect(int(math.Floor(r.X0))-1, int(math.Floor(r.Y0))-1, int(math.Ceil(r.X1))+1, int(math.Ceil(r.Y1))+1)
}

func (r RectF) Distance(radius float64) Distance {
	cx, cy := (r.X0+r.X1)/2, (r.Y0+r.Y1)/2
	hw, hh := (r.X1-r.X0)/2, (r.Y1-r.Y0)/2
	radius = math.Max(0, math.Min(radius, math.Min(hw, hh)))
	return func(x, y float64) float64 {
		qx := math.Abs(x-cx) - (hw - radius)
		qy := math.Abs(y-cy) - (hh - radius)
		return math.Hypot(math.Max(qx, 0), math.Max(qy, 0)) + math.Min(math.Max(qx, qy), 0) - radius
	}
}

func (r RectF) Grow(d float64) RectF {
	return RectF{r.X0 - d, r.Y0 - d, r.X1 + d, r.Y1 + d}
}

func (r RectF) physical(k float64) image.Rectangle {
	return image.Rect(int(math.Round(r.X0*k)), int(math.Round(r.Y0*k)), int(math.Round(r.X1*k)), int(math.Round(r.Y1*k)))
}

func FillRoundF(c widget.DrawContext, r RectF, radius float64, premul color.RGBA) {
	if k := widget.ContextScale(c); k != 1 {
		if p := r.physical(k); !p.Empty() {
			fillHi(c, r.Bounds(), k, roundCoverage(p, radius*k), premul)
		}
		return
	}
	FillDistance(c, r.Bounds(), r.Distance(radius), premul)
}

func StrokeRoundF(c widget.DrawContext, r RectF, radius, width float64, premul color.RGBA) {
	if k := widget.ContextScale(c); k != 1 {
		if p := r.physical(k); !p.Empty() {
			fillHi(c, r.Bounds(), k, strokeRoundCoverage(p, radius*k, physWidth(width, k)), premul)
		}
		return
	}
	StrokeDistance(c, r.Bounds(), r.Distance(radius), width, premul)
}
