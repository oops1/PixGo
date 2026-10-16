package gfx

import (
	"image"
	"image/color"
	"math"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/ui/theme"
)

const RingWidth = 2

type Ring struct {
	Color    color.RGBA
	Inner    color.RGBA
	Contrast bool
}

func RingOf(t *theme.Theme) Ring {
	return Ring{Color: t.Focus, Inner: t.Canvas, Contrast: t.Name == "contrast"}
}

func (g Ring) gap() int {
	if g.Contrast {
		return 1
	}
	return 0
}

func (g Ring) Rect(c widget.DrawContext, r image.Rectangle, radius float64) {
	if k := widget.ContextScale(c); k != 1 {
		g.rectHi(c, PhysRect(r, k), r, radius, k)
		return
	}
	gap := g.gap()
	if gap > 0 {
		StrokeRound(c, r.Inset(-gap), radius+float64(gap), 1, g.Inner)
	}
	StrokeRound(c, r.Inset(-RingWidth-gap), radius+float64(RingWidth+gap), RingWidth, g.Color)
}

func (g Ring) Circle(c widget.DrawContext, cx, cy, radius float64) {
	gap := float64(g.gap())
	if gap > 0 {
		StrokeCircle(c, cx, cy, radius+gap, 1, g.Inner)
	}
	StrokeCircle(c, cx, cy, radius+RingWidth+gap, RingWidth, g.Color)
}

func (g Ring) RectF(c widget.DrawContext, r RectF, radius float64) {
	if k := widget.ContextScale(c); k != 1 {
		if p := r.physical(k); !p.Empty() {
			g.rectHi(c, p, r.Bounds(), radius, k)
		}
		return
	}
	gap := float64(g.gap())
	if gap > 0 {
		StrokeRoundF(c, r.Grow(gap), radius+gap, 1, g.Inner)
	}
	StrokeRoundF(c, r.Grow(RingWidth+gap), radius+RingWidth+gap, RingWidth, g.Color)
}

func (g Ring) rectHi(c widget.DrawContext, p image.Rectangle, logical image.Rectangle, radius, k float64) {
	wp := int(physWidth(RingWidth, k))
	gap := g.gap()
	reach := wp + gap
	area := logical.Inset(-int(math.Ceil(float64(reach)/k)) - 1)
	if gap > 0 {
		fillHi(c, area, k, strokeRoundCoverage(p.Inset(-gap), radius*k+float64(gap), 1), g.Inner)
	}
	fillHi(c, area, k, strokeRoundCoverage(p.Inset(-reach), radius*k+float64(reach), float64(wp)), g.Color)
}
