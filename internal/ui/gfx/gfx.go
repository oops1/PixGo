package gfx

import (
	"image"
	"image/color"
	"math"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/fonts"
)

func R(x, y, w, h int) image.Rectangle { return image.Rect(x, y, x+max(0, w), y+max(0, h)) }

func RGB(v uint32) color.RGBA { return color.RGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 255} }

func Blend(a, b color.RGBA, f float64) color.RGBA {
	m := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*f + .5) }
	return color.RGBA{m(a.R, b.R), m(a.G, b.G), m(a.B, b.B), 255}
}

func Alpha(col color.RGBA, a float64) color.RGBA {
	a = math.Max(0, math.Min(1, a))
	m := func(v uint8) uint8 { return uint8(float64(v)*a + .5) }
	return color.RGBA{m(col.R), m(col.G), m(col.B), uint8(a*255 + .5)}
}

func Clip(c widget.DrawContext, r image.Rectangle) func() {
	old := c.Clip()
	c.SetClip(old.Intersect(r))
	return func() { c.SetClip(old) }
}

func Visible(c widget.DrawContext, r image.Rectangle) bool { return c.Clip().Overlaps(r) }

func Line(c widget.DrawContext, x1, y1, x2, y2 int, col color.RGBA) {
	if aa, ok := c.(widget.AAShapes); ok {
		aa.DrawLineAA(x1, y1, x2, y2, 1, col)
		return
	}
	if y1 == y2 {
		c.DrawHLine(min(x1, x2), y1, abs(x2-x1)+1, col)
	} else if x1 == x2 {
		c.DrawVLine(x1, min(y1, y2), abs(y2-y1)+1, col)
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

type Font struct {
	Name string
	Pt   float64
}

func UI(pt float64) Font { return Font{fonts.UI, pt} }

func UISemi(pt float64) Font { return Font{fonts.UISemiBold, pt} }

func UILight(pt float64) Font { return Font{fonts.UILight, pt} }

func (f Font) Size() float64 { return f.Pt }

func (f Font) Ascent() int { return fonts.MetricsOf(f.Name, f.Size(), 96).Ascent }

func (f Font) Measure(c widget.DrawContext, s string) int {
	if c == nil {
		return widget.MeasureUITextFont(s, f.Size(), f.Name)
	}
	return c.MeasureTextFont(s, f.Size(), f.Name)
}

func (f Font) Draw(c widget.DrawContext, s string, x, baseline int, col color.RGBA) {
	c.DrawTextFont(s, x, baseline-f.Ascent(), f.Size(), f.Name, col)
}

func (f Font) Fit(c widget.DrawContext, s string, w int) string {
	if f.Measure(c, s) <= w {
		return s
	}
	r := []rune(s)
	lo, hi := 0, len(r)
	for lo < hi {
		m := (lo + hi + 1) / 2
		if f.Measure(c, string(r[:m])+"…") <= w {
			lo = m
		} else {
			hi = m - 1
		}
	}
	return string(r[:lo]) + "…"
}

func (f Font) DrawFit(c widget.DrawContext, s string, x, baseline, w int, col color.RGBA) {
	s = f.Fit(c, s, w)
	restore := Clip(c, image.Rect(x, baseline-f.Ascent()-4, x+w, baseline+f.Ascent()))
	f.Draw(c, s, x, baseline, col)
	restore()
}

func DrawChildren(c widget.DrawContext, w widget.Widget) {
	for _, ch := range w.Children() {
		if widget.IsWidgetVisible(ch) && !widget.SkipSubtree(c, ch) {
			ch.Draw(c)
		}
	}
}
