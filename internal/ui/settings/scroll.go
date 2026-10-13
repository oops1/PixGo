package settings

import (
	"image"
	"math"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/ui/gfx"
	"github.com/oops1/PixGo/internal/ui/theme"
)

func barGeometry(vp image.Rectangle, content, scroll, maxScroll int, wide bool) (track, thumb image.Rectangle, ok bool) {
	if maxScroll <= 0 || vp.Dy() <= 2*barInset || content <= 0 {
		return track, thumb, false
	}
	right := vp.Max.X - barRightInset
	track = image.Rect(right-dim.barWide, vp.Min.Y+barInset, right, vp.Max.Y-barInset)
	h := min(max(24, track.Dy()*vp.Dy()/content), track.Dy())
	top := track.Min.Y + (track.Dy()-h)*scroll/maxScroll
	width := dim.barThin
	if wide {
		width = dim.barWide
	}
	return track, image.Rect(right-width, top, right, top+h), true
}

func barOffset(track, thumb image.Rectangle, grab, y, maxScroll int) (int, bool) {
	span := track.Dy() - thumb.Dy()
	if span <= 0 {
		return 0, false
	}
	pos := min(max(y-grab-track.Min.Y, 0), span)
	return int(math.Round(float64(pos) / float64(span) * float64(maxScroll))), true
}

func (p *Page) maxScroll() int { return max(p.contentH-p.geo.view.Max.Y, 0) }

func (p *Page) barRects() (track, thumb image.Rectangle, ok bool) {
	wide := (p.drag != nil && p.drag.bar) || p.hover.kind == hitBar
	view := p.geo.view
	return barGeometry(view, p.contentH-view.Min.Y, p.scroll, p.maxScroll(), wide)
}

func (p *Page) barArea() image.Rectangle {
	v := p.geo.view
	return image.Rect(v.Max.X-barRightInset-dim.barWide-2, v.Min.Y, v.Max.X, v.Max.Y)
}

func (p *Page) setScroll(v int) bool {
	v = min(max(v, 0), p.maxScroll())
	if v == p.scroll {
		return false
	}
	p.scroll = v
	p.invalidateRect(p.geo.view)
	p.showBar()
	return true
}

func (p *Page) scrollBy(dy int) bool { return p.setScroll(p.scroll + dy) }

func (p *Page) showBar() {
	if _, _, ok := p.barRects(); !ok {
		return
	}
	if p.bar.stop != nil {
		p.bar.stop()
	}
	p.bar.shown = true
	area := p.barArea()
	p.invalidateRect(area)
	p.bar.stop = p.after(barHideDelay, func() {
		p.bar.stop = nil
		if p.drag != nil && p.drag.bar {
			return
		}
		p.bar.shown = false
		p.invalidateRect(area)
	})
}

func (p *Page) reveal(r image.Rectangle) {
	view := p.geo.view
	top, bottom := r.Min.Y-12, r.Max.Y+12
	switch {
	case top < view.Min.Y+p.scroll:
		p.setScroll(top - view.Min.Y)
	case bottom > view.Max.Y+p.scroll:
		p.setScroll(bottom - view.Max.Y)
	}
}

func (p *Page) wheel(notches float64) bool {
	p.wheelFrac += notches * dim.wheelStep
	d := int(p.wheelFrac)
	p.wheelFrac -= float64(d)
	if d != 0 {
		p.setScroll(p.scroll + d)
	}
	return true
}

func (p *Page) dragBar(y int) {
	track, thumb, ok := p.barRects()
	if !ok || p.drag == nil || !p.drag.bar {
		return
	}
	if v, ok := barOffset(track, thumb, p.drag.grab, y, p.maxScroll()); ok {
		p.setScroll(v)
	}
}

func (p *Page) drawBar(c widget.DrawContext, t *theme.Theme) {
	if !p.bar.shown {
		return
	}
	_, thumb, ok := p.barRects()
	if !ok {
		return
	}
	a := .32
	if (p.drag != nil && p.drag.bar) || p.hover.kind == hitBar {
		a = .6
	}
	gfx.FillRound(c, thumb, float64(thumb.Dx())/2, gfx.Alpha(t.ScrollThumb, a))
}
