package window

import (
	"image"
	"time"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/commands"
	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/ui/gfx"
)

const (
	tipDelay = 600 * time.Millisecond
	tipGap   = 8
	tipPadX  = 10
)

var tipFont = gfx.UI(9)

type tooltip struct {
	zone ZoneKind
	text string
	stop func()
}

func (r *Root) tipText(k ZoneKind) string {
	switch k {
	case ZoneMin:
		return i18n.T("window.minimize")
	case ZoneMax:
		if r.Layout().Opt.Maximized {
			return i18n.T("window.restore")
		}
		return i18n.T("window.maximize")
	case ZoneClose:
		return i18n.T("window.close")
	case ZoneToolZoomLabel:
		return i18n.T("zoom.label")
	case ZoneSlidePlay:
		return r.slidePlayTip()
	case ZoneSlideSettings:
		return i18n.T("slideshow.settings")
	case ZoneDrag, ZoneNone, ZoneSlideBar:
		return ""
	}
	cmd := r.command(k)
	if cmd == nil {
		return ""
	}
	label := cmd.Label()
	if (k == ZoneToolFullscreen || k == ZoneExit || k == ZoneSlideExit) && r.host.Fullscreen() {
		label = i18n.T("cmd.fullscreenExit")
	}
	if k == ZoneFavorite && r.host.Favorite() {
		label = i18n.T("cmd.favorite.remove")
	}
	if sc := cmd.Shortcut(); sc != "" {
		return label + " (" + sc + ")"
	}
	return label
}

func (r *Root) slidePlayTip() string {
	v := r.host.Slideshow()
	id, key := "slideshow.start", "cmd.slideshow"
	switch {
	case v.Active && v.Playing:
		id, key = "slideshow.toggle", "cmd.slideshowPause"
	case v.Active:
		id, key = "slideshow.toggle", "cmd.slideshowResume"
	}
	text := i18n.T(key)
	if reg := r.host.Registry(); reg != nil {
		if cmd := reg.Get(commands.ID(id)); cmd != nil {
			if sc := cmd.Shortcut(); sc != "" {
				return text + " (" + sc + ")"
			}
		}
	}
	return text
}

func (r *Root) scheduleTip(k ZoneKind) {
	r.hideTip()
	if r.After == nil {
		return
	}
	text := r.tipText(k)
	if text == "" {
		return
	}
	r.tip.zone = k
	r.tip.stop = r.After(tipDelay, func() {
		r.tip.stop = nil
		r.tip.text = text
		r.eng.InvalidateRect(r.tipRect(nil))
	})
}

func (r *Root) hideTip() {
	if r.tip.stop != nil {
		r.tip.stop()
		r.tip.stop = nil
	}
	if r.tip.text != "" {
		rect := r.tipRect(nil)
		r.tip.text = ""
		r.eng.InvalidateRect(rect)
	}
	r.tip.zone = ZoneNone
}

func (r *Root) tipRect(c widget.DrawContext) image.Rectangle {
	l := r.fullOverlayLayout()
	anchor := l.Rect(r.tip.zone)
	if anchor.Empty() || r.tip.text == "" && c == nil {
		return image.Rect(0, 0, 0, 0)
	}
	h := tipH()
	w := min(textWidth(tipFont, r.tip.text)+2*tipPadX, max(0, l.W-8))
	cx := (anchor.Min.X + anchor.Max.X) / 2
	x0 := max(4, min(cx-w/2, l.W-w-4))
	var y0 int
	if anchor.Max.Y <= TitleHeight {
		y0 = anchor.Max.Y + tipGap
	} else {
		top := anchor.Min.Y
		if r.tip.zone >= ZoneToolGallery && r.tip.zone <= ZoneToolFullscreen || r.tip.zone >= ZoneSlidePlay && r.tip.zone <= ZoneSlideBar {
			top = l.Toolbar.Min.Y
		}
		y0 = top - tipGap - h
	}
	y0 = max(0, min(y0, l.H-h-4))
	return image.Rect(x0, y0, x0+w, y0+h)
}

func (r *Root) drawTip(c widget.DrawContext) {
	if r.tip.text == "" {
		return
	}
	t := r.Theme
	rect := r.tipRect(c)
	gfx.FillRound(c, rect, 6, t.Menu)
	gfx.StrokeRound(c, rect, 6, 1, t.MenuBorder)
	baseline := rect.Min.Y + (rect.Dy()+tipFont.Ascent()-3)/2 - textOpticalRise(tipFont)
	tipFont.DrawFit(c, r.tip.text, rect.Min.X+tipPadX, baseline, rect.Dx()-2*tipPadX, t.Text)
}
