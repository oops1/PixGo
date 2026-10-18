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
	b := image.Rect(l.View.Min.X, l.Toolbar.Min.Y-toastGap-toastH()-4, l.View.Max.X, l.Toolbar.Min.Y-toastGap+4)
	return b.Intersect(l.View)
}

func (r *Root) toastRect(l Layout) image.Rectangle {
	area := r.toastArea()
	h := toastH()
	w := textWidth(toastFont, r.toastText) + 2*toastPadX
	w = min(w, max(80, l.View.Dx()-32))
	cx := (l.View.Min.X + l.View.Max.X) / 2
	return image.Rect(cx-w/2, area.Max.Y-4-h, cx-w/2+w, area.Max.Y-4)
}

func (r *Root) drawToast(c widget.DrawContext, l Layout) {
	if r.toastText == "" {
		return
	}
	t := r.Theme
	rect := r.toastRect(l)
	gfx.FillRound(c, rect, toastRadius, t.Menu)
	gfx.StrokeRound(c, rect, toastRadius, 1, t.MenuBorder)
	baseline := rect.Min.Y + (rect.Dy()+toastFont.Ascent()-4)/2 - textOpticalRise(toastFont)
	toastFont.DrawFit(c, r.toastText, rect.Min.X+toastPadX, baseline, rect.Dx()-2*toastPadX, t.Text)
}

func (r *Root) ToastText() string { return r.toastText }
