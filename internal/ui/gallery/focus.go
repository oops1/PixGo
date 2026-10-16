package gallery

import (
	"image"
	"math"
	"slices"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/ui/gfx"
	"github.com/oops1/PixGo/internal/ui/theme"
)

type stop uint8

const (
	stopGrid stop = iota
	stopBack
	stopForward
	stopUp
	stopAddress
	stopFilter
	stopRefresh
	stopViews
	stopThumbs
	stopNav
	stopOpen
	stopMore
)

var stopOrder = []stop{
	stopBack, stopForward, stopUp, stopAddress, stopFilter, stopRefresh, stopViews, stopThumbs,
	stopNav, stopGrid, stopOpen, stopMore,
}

const filterPlateInset = 3

func (p *Page) KeyboardFocus() (stop, bool) { return p.focus, p.keyNav }

func (p *Page) stops() []stop {
	s := p.snap
	selected := p.Selected() >= 0
	var out []stop
	for _, st := range stopOrder {
		on := true
		switch st {
		case stopBack:
			on = s.canBack
		case stopForward:
			on = s.canForward
		case stopUp:
			on = s.canUp
		case stopNav:
			on = len(p.navSelectable()) > 0
		case stopGrid:
			on = len(s.items) > 0
		case stopOpen, stopMore:
			on = selected
		}
		if on {
			out = append(out, st)
		}
	}
	return out
}

func (p *Page) moveFocus(dir int) {
	p.ensureLayout()
	stops := p.stops()
	if len(stops) == 0 {
		return
	}
	cur := slices.Index(stops, p.focus)
	if cur < 0 {
		cur = slices.Index(stops, stopGrid)
	}
	if cur < 0 {
		cur = 0
	}
	p.setFocus(stops[(cur+dir+len(stops))%len(stops)])
}

func (p *Page) setFocus(s stop) {
	p.invalidateFocus()
	p.focus = s
	p.keyNav = true
	switch s {
	case stopNav:
		p.navCursor = p.navCurrent()
		p.revealNav(p.navCursor)
	case stopGrid:
		if p.Selected() < 0 && len(p.snap.items) > 0 {
			p.selectIndex(0)
		}
		if i := p.Selected(); i >= 0 {
			p.setScroll(p.grid.Reveal(i, p.scroll))
		}
	}
	p.invalidateFocus()
}

func (p *Page) clearKeyNav() {
	if !p.keyNav {
		return
	}
	p.invalidateFocus()
	p.keyNav = false
}

func (p *Page) focusFromHit(h hit) {
	var s stop
	switch h.kind {
	case hitBack:
		s = stopBack
	case hitForward:
		s = stopForward
	case hitUp:
		s = stopUp
	case hitAddress:
		s = stopAddress
	case hitFilter:
		s = stopFilter
	case hitRefresh:
		s = stopRefresh
	case hitViewList, hitViewGrid:
		s = stopViews
	case hitThumbs:
		s = stopThumbs
	case hitNav, hitNavBar:
		s = stopNav
		if h.kind == hitNav {
			p.navCursor = h.index
		}
	case hitOpen:
		s = stopOpen
	case hitMore:
		s = stopMore
	default:
		s = stopGrid
	}
	p.focus = s
}

func (p *Page) navSelectable() []int {
	var out []int
	for _, r := range p.rows {
		if !r.entry.Header && !r.entry.Disabled {
			out = append(out, r.index)
		}
	}
	return out
}

func (p *Page) navCurrent() int {
	rows := p.navSelectable()
	if len(rows) == 0 {
		return -1
	}
	if slices.Contains(rows, p.navCursor) {
		return p.navCursor
	}
	for _, r := range p.rows {
		if r.entry.Selected && !r.entry.Header && !r.entry.Disabled {
			return r.index
		}
	}
	return rows[0]
}

func (p *Page) moveNav(dir int) {
	rows := p.navSelectable()
	if len(rows) == 0 {
		return
	}
	cur := slices.Index(rows, p.navCurrent())
	p.setNavCursor(rows[min(max(cur+dir, 0), len(rows)-1)])
}

func (p *Page) setNavCursor(index int) {
	if index == p.navCursor {
		return
	}
	p.invalidateFocus()
	p.navCursor = index
	p.revealNav(index)
	p.invalidateFocus()
}

func (p *Page) revealNav(index int) {
	for _, r := range p.rows {
		if r.index != index {
			continue
		}
		top := int(math.Floor(r.center-navPillHalf+.5)) - navLeft
		if top < navLeft {
			top = 0
		}
		bottom := top + 2*navPillHalf + 2*navLeft
		view := p.geo.sidebar.Dy()
		switch {
		case top < p.sideY:
			p.sideY = top
		case bottom > p.sideY+view:
			p.sideY = bottom - view
		}
		p.clampSide()
		p.invalidateRect(p.geo.sidebar)
		return
	}
}

func (p *Page) navRowRect(index int) image.Rectangle {
	for _, r := range p.rows {
		if r.index == index {
			return r.rect(p.geo.sidebar.Min.Y-p.sideY, p.geo.sidebar.Max.X).Intersect(p.geo.sidebar)
		}
	}
	return image.Rectangle{}
}

func (p *Page) viewPillOf(mode ViewMode) image.Rectangle {
	if mode == ViewList {
		return p.viewPill(p.geo.viewList)
	}
	return p.viewPill(p.geo.viewGrid)
}

func (p *Page) focusRect(s stop) image.Rectangle {
	g := p.geo
	switch s {
	case stopBack:
		return g.back
	case stopForward:
		return g.forward
	case stopUp:
		return g.up
	case stopAddress:
		return g.address
	case stopFilter:
		return g.filter.Inset(filterPlateInset)
	case stopRefresh:
		return g.refresh.Inset(filterPlateInset)
	case stopViews:
		return p.viewPillOf(p.snap.mode)
	case stopThumbs:
		return g.thumbs
	case stopNav:
		return p.navRowRect(p.navCurrent())
	case stopOpen:
		return g.open
	case stopMore:
		return g.more
	}
	if i := p.Selected(); i >= 0 && i < len(p.snap.items) {
		return p.tileRect(i).Intersect(g.grid)
	}
	return image.Rectangle{}
}

func (p *Page) invalidateFocus() {
	if p.eng == nil {
		return
	}
	p.invalidateRect(p.focusRect(p.focus).Inset(-gfx.RingWidth - 3))
}

func (p *Page) hitOfStop(s stop) hit {
	switch s {
	case stopBack:
		return hit{kind: hitBack}
	case stopForward:
		return hit{kind: hitForward}
	case stopUp:
		return hit{kind: hitUp}
	case stopAddress:
		return hit{kind: hitAddress}
	case stopFilter:
		return hit{kind: hitFilter}
	case stopRefresh:
		return hit{kind: hitRefresh}
	case stopThumbs:
		return hit{kind: hitThumbs}
	case stopNav:
		return hit{kind: hitNav, index: p.navCurrent()}
	case stopOpen:
		return hit{kind: hitOpen}
	case stopMore:
		return hit{kind: hitMore}
	}
	return noHit
}

func isActivate(e widget.KeyEvent) bool {
	return e.Code == widget.KeyEnter || e.Code == widget.KeySpace
}

func (p *Page) controlKey(e widget.KeyEvent) bool {
	switch p.focus {
	case stopViews:
		return p.viewsKey(e)
	case stopNav:
		return p.navKey(e)
	case stopGrid:
		return false
	}
	if isActivate(e) {
		if !e.Repeat {
			p.ensureLayout()
			p.activate(p.hitOfStop(p.focus))
		}
		return true
	}
	return false
}

func (p *Page) viewsKey(e widget.KeyEvent) bool {
	switch e.Code {
	case widget.KeyLeft, widget.KeyUp:
		p.setMode(ViewList)
	case widget.KeyRight, widget.KeyDown:
		p.setMode(ViewGrid)
	case widget.KeyEnter, widget.KeySpace:
	default:
		return false
	}
	return true
}

func (p *Page) navKey(e widget.KeyEvent) bool {
	switch e.Code {
	case widget.KeyUp:
		p.moveNav(-1)
	case widget.KeyDown:
		p.moveNav(1)
	case widget.KeyHome:
		p.moveNav(-len(p.rows))
	case widget.KeyEnd:
		p.moveNav(len(p.rows))
	case widget.KeyEnter, widget.KeySpace:
		if !e.Repeat {
			p.ensureLayout()
			p.activate(p.hitOfStop(stopNav))
		}
	default:
		return false
	}
	return true
}

func (p *Page) drawFocus(c widget.DrawContext, t *theme.Theme) {
	if !p.keyNav || p.editing {
		return
	}
	r := p.focusRect(p.focus)
	if r.Empty() {
		return
	}
	ring := gfx.RingOf(t)
	g := p.geo
	switch p.focus {
	case stopBack, stopForward, stopUp, stopThumbs, stopViews, stopFilter, stopRefresh:
		ring.Rect(c, r, navPillRadius)
	case stopAddress:
		ring.Rect(c, r, controlRadius)
	case stopOpen, stopMore:
		ring.Rect(c, r, buttonRadius)
	case stopNav:
		restore := gfx.Clip(c, g.sidebar)
		ring.Rect(c, r, 5)
		restore()
	default:
		restore := gfx.Clip(c, g.grid)
		radius := float64(cellRadius)
		if p.snap.mode == ViewList {
			radius = photoRadius
		}
		ring.Rect(c, r, radius)
		restore()
	}
}
