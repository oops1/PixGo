package settings

import (
	"image"
	"image/color"
	"math"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/ui/gfx"
)

type frect struct{ x0, y0, x1, y1 float64 }

func fr(r image.Rectangle) frect {
	return frect{float64(r.Min.X), float64(r.Min.Y), float64(r.Max.X), float64(r.Max.Y)}
}

func (r frect) grow(d float64) frect { return frect{r.x0 - d, r.y0 - d, r.x1 + d, r.y1 + d} }

func (r frect) bounds() image.Rectangle {
	return image.Rect(int(math.Floor(r.x0))-1, int(math.Floor(r.y0))-1, int(math.Ceil(r.x1))+1, int(math.Ceil(r.y1))+1)
}

func (r frect) distance(radius float64) gfx.Distance {
	cx, cy := (r.x0+r.x1)/2, (r.y0+r.y1)/2
	hw, hh := (r.x1-r.x0)/2, (r.y1-r.y0)/2
	radius = math.Max(0, math.Min(radius, math.Min(hw, hh)))
	return func(x, y float64) float64 {
		qx := math.Abs(x-cx) - (hw - radius)
		qy := math.Abs(y-cy) - (hh - radius)
		return math.Hypot(math.Max(qx, 0), math.Max(qy, 0)) + math.Min(math.Max(qx, qy), 0) - radius
	}
}

func (r frect) gfx() gfx.RectF { return gfx.RectF{X0: r.x0, Y0: r.y0, X1: r.x1, Y1: r.y1} }

func fillRound(c widget.DrawContext, r frect, radius float64, col color.RGBA) {
	gfx.FillRoundF(c, r.gfx(), radius, col)
}

func strokeRound(c widget.DrawContext, r frect, radius, width float64, col color.RGBA) {
	gfx.StrokeRoundF(c, r.gfx(), radius, width, col)
}

func fillDisc(c widget.DrawContext, cx, cy, radius float64, col color.RGBA) {
	gfx.FillCircle(c, cx, cy, radius, col)
}

func strokeDisc(c widget.DrawContext, cx, cy, radius, width float64, col color.RGBA) {
	gfx.StrokeCircle(c, cx, cy, radius, width, col)
}

func in(r image.Rectangle, x, y int) bool { return image.Pt(x, y).In(r) }
