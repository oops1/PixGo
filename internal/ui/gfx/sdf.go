package gfx

import (
	"image"
	"image/color"
	"math"

	"github.com/oops1/headless-gui/v3/widget"
)

type Distance func(x, y float64) float64

func RoundRectDistance(r image.Rectangle, radius float64) Distance {
	cx := float64(r.Min.X+r.Max.X) / 2
	cy := float64(r.Min.Y+r.Max.Y) / 2
	hw := float64(r.Dx()) / 2
	hh := float64(r.Dy()) / 2
	radius = math.Min(radius, math.Min(hw, hh))
	return func(x, y float64) float64 {
		qx := math.Abs(x-cx) - (hw - radius)
		qy := math.Abs(y-cy) - (hh - radius)
		return math.Hypot(math.Max(qx, 0), math.Max(qy, 0)) + math.Min(math.Max(qx, qy), 0) - radius
	}
}

func CircleDistance(cx, cy, radius float64) Distance {
	return func(x, y float64) float64 { return math.Hypot(x-cx, y-cy) - radius }
}

func scalePremul(c color.RGBA, f float64) color.RGBA {
	m := func(v uint8) uint8 { return uint8(float64(v)*f + .5) }
	return color.RGBA{m(c.R), m(c.G), m(c.B), m(c.A)}
}

func clamp01(v float64) float64 { return math.Max(0, math.Min(1, v)) }

func fillCoverage(c widget.DrawContext, bounds image.Rectangle, coverage func(x, y int) float64, premul color.RGBA) {
	clip := c.Clip().Intersect(bounds)
	if clip.Empty() {
		return
	}
	for y := clip.Min.Y; y < clip.Max.Y; y++ {
		runStart := -1
		flush := func(end int) {
			if runStart >= 0 {
				c.FillRectAlpha(runStart, y, end-runStart, 1, premul)
				runStart = -1
			}
		}
		for x := clip.Min.X; x < clip.Max.X; x++ {
			cov := coverage(x, y)
			switch {
			case cov >= 0.999:
				if runStart < 0 {
					runStart = x
				}
			case cov <= 0.001:
				flush(x)
			default:
				flush(x)
				c.FillRectAlpha(x, y, 1, 1, scalePremul(premul, cov))
			}
		}
		flush(clip.Max.X)
	}
}

func FillDistance(c widget.DrawContext, bounds image.Rectangle, d Distance, premul color.RGBA) {
	if k := widget.ContextScale(c); k != 1 {
		fillDistanceHi(c, bounds, d, k, premul)
		return
	}
	fillCoverage(c, bounds, func(x, y int) float64 {
		return clamp01(0.5 - d(float64(x)+.5, float64(y)+.5))
	}, premul)
}

func StrokeDistance(c widget.DrawContext, bounds image.Rectangle, d Distance, width float64, premul color.RGBA) {
	if k := widget.ContextScale(c); k != 1 {
		strokeDistanceHi(c, bounds, d, width, k, premul)
		return
	}
	half := width / 2
	fillCoverage(c, bounds, func(x, y int) float64 {
		v := d(float64(x)+.5, float64(y)+.5)
		return clamp01(0.5 - (math.Abs(v+half) - half))
	}, premul)
}

func FillRound(c widget.DrawContext, r image.Rectangle, radius float64, premul color.RGBA) {
	if k := widget.ContextScale(c); k != 1 {
		fillRoundHi(c, r, radius, k, premul)
		return
	}
	FillDistance(c, r, RoundRectDistance(r, radius), premul)
}

func StrokeRound(c widget.DrawContext, r image.Rectangle, radius, width float64, premul color.RGBA) {
	if k := widget.ContextScale(c); k != 1 {
		strokeRoundHi(c, r, radius, width, k, premul)
		return
	}
	StrokeDistance(c, r, RoundRectDistance(r, radius), width, premul)
}

func FillCircle(c widget.DrawContext, cx, cy, radius float64, premul color.RGBA) {
	b := image.Rect(int(math.Floor(cx-radius))-1, int(math.Floor(cy-radius))-1, int(math.Ceil(cx+radius))+1, int(math.Ceil(cy+radius))+1)
	FillDistance(c, b, CircleDistance(cx, cy, radius), premul)
}

func StrokeCircle(c widget.DrawContext, cx, cy, radius, width float64, premul color.RGBA) {
	b := image.Rect(int(math.Floor(cx-radius))-1, int(math.Floor(cy-radius))-1, int(math.Ceil(cx+radius))+1, int(math.Ceil(cy+radius))+1)
	StrokeDistance(c, b, CircleDistance(cx, cy, radius), width, premul)
}

func Premultiply(c color.RGBA) color.RGBA {
	if c.A == 255 {
		return c
	}
	f := float64(c.A) / 255
	m := func(v uint8) uint8 { return uint8(float64(v)*f + .5) }
	return color.RGBA{m(c.R), m(c.G), m(c.B), c.A}
}
