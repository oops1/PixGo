package window

import (
	"image"
	"time"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/ui/gfx"
)

const (
	toastDuration = 2500 * time.Millisecond
	toastPadX     = 18
	toastHeight   = 40
	toastGap      = 14
	toastRadius   = 8
)

var toastFont = gfx.UI(10)

func (r *Root) Toast(text string) {
	r.toastText = text
	if r.stopToast != nil {
		r.stopToast()
		r.stopToast = nil
	}
	r.eng.InvalidateRect(r.toastArea())
	if r.After != nil && text != "" {
		r.stopToast = r.After(toastDuration, func() {
			r.stopToast = nil
			r.toastText = ""
			r.eng.InvalidateRect(r.toastArea())
		})
	}
}

func (r *Root) toastArea() image.Rectangle {
	l := r.fullOverlayLayout()
	b := image.Rect(l.View.Min.X, l.Toolbar.Min.Y-toastGap-toastHeight-4, l.View.Max.X, l.Toolbar.Min.Y-toastGap+4)
	return b.Intersect(l.View)
}

func (r *Root) drawToast(c widget.DrawContext, l Layout) {
	if r.toastText == "" {
		return
	}
	t := r.Theme
	area := r.toastArea()
	w := toastFont.Measure(c, r.toastText) + 2*toastPadX
	w = min(w, max(80, l.View.Dx()-32))
	cx := (l.View.Min.X + l.View.Max.X) / 2
	rect := image.Rect(cx-w/2, area.Max.Y-4-toastHeight, cx-w/2+w, area.Max.Y-4)
	gfx.FillRound(c, rect, toastRadius, t.Menu)
	gfx.StrokeRound(c, rect, toastRadius, 1, t.MenuBorder)
	baseline := rect.Min.Y + (toastHeight+toastFont.Ascent()-4)/2
	toastFont.DrawFit(c, r.toastText, rect.Min.X+toastPadX, baseline, w-2*toastPadX, t.Text)
}
