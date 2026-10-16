package infopanel

import (
	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/ui/gfx"
)

const (
	focusRadius = 4
	scrollKeep  = 8
)

func (p *Panel) FocusActive() bool { return p.active }

func (p *Panel) FocusEnter(dir int) {
	stops := p.hits()
	p.active = len(stops) > 0
	p.cur = 0
	if dir < 0 {
		p.cur = len(stops) - 1
	}
	p.revealFocus()
	p.invalidate()
}

func (p *Panel) FocusStep(dir int) bool {
	if !p.active {
		return false
	}
	next := p.cur + dir
	if next < 0 || next >= len(p.hits()) {
		p.active = false
		p.invalidate()
		return false
	}
	p.cur = next
	p.revealFocus()
	p.invalidate()
	return true
}

func (p *Panel) FocusLeave() {
	if !p.active {
		return
	}
	p.active = false
	p.invalidate()
}

func (p *Panel) focusedHit() (hit, bool) {
	stops := p.hits()
	if !p.active || p.cur < 0 || p.cur >= len(stops) {
		return hit{}, false
	}
	return stops[p.cur], true
}

func (p *Panel) revealFocus() {
	h, ok := p.focusedHit()
	if !ok || h.kind == hitClose {
		p.scroll = 0
		return
	}
	b := p.Bounds()
	switch {
	case h.rect.Min.Y < b.Min.Y+scrollKeep:
		p.scrollBy(h.rect.Min.Y - b.Min.Y - scrollKeep)
	case h.rect.Max.Y > b.Max.Y-scrollKeep:
		p.scrollBy(h.rect.Max.Y - b.Max.Y + scrollKeep)
	}
}

func (p *Panel) HandleKey(e widget.KeyEvent) bool {
	if !p.active {
		return false
	}
	if e.Mod&(widget.ModCtrl|widget.ModAlt) != 0 {
		return false
	}
	switch e.Code {
	case widget.KeyEnter, widget.KeySpace:
		if !e.Repeat {
			if h, ok := p.focusedHit(); ok {
				p.activate(h)
			}
		}
	case widget.KeyEscape:
		if !e.Repeat && p.OnClose != nil {
			p.OnClose()
		}
	case widget.KeyUp:
		p.scrollBy(-p.step())
	case widget.KeyDown:
		p.scrollBy(p.step())
	case widget.KeyPageUp:
		p.scrollBy(-p.Bounds().Dy())
	case widget.KeyPageDown:
		p.scrollBy(p.Bounds().Dy())
	default:
		return false
	}
	return true
}

func (p *Panel) drawFocus(c widget.DrawContext) {
	h, ok := p.focusedHit()
	if !ok || p.Theme == nil {
		return
	}
	if !h.rect.Overlaps(p.Bounds()) {
		return
	}
	gfx.RingOf(p.Theme).Rect(c, h.rect, focusRadius)
}
