package gallery

import (
	"image"
	"math"
)

func barGeometry(vp image.Rectangle, content, scroll, maxScroll int, wide bool) (track, thumb image.Rectangle, ok bool) {
	if maxScroll <= 0 || vp.Dy() <= 2*barInset || content <= 0 {
		return track, thumb, false
	}
	right := vp.Max.X - barRightInset
	track = image.Rect(right-barWide, vp.Min.Y+barInset, right, vp.Max.Y-barInset)
	h := min(max(24, track.Dy()*vp.Dy()/content), track.Dy())
	top := track.Min.Y + (track.Dy()-h)*scroll/maxScroll
	width := barThin
	if wide {
		width = barWide
	}
	return track, image.Rect(right-width, top, right, top+h), true
}

func barAreaOf(vp image.Rectangle) image.Rectangle {
	return image.Rect(vp.Max.X-barRightInset-barWide-2, vp.Min.Y, vp.Max.X, vp.Max.Y)
}

func barOffset(track, thumb image.Rectangle, grab, y, maxScroll int) (int, bool) {
	span := track.Dy() - thumb.Dy()
	if span <= 0 {
		return 0, false
	}
	pos := min(max(y-grab-track.Min.Y, 0), span)
	return int(math.Round(float64(pos) / float64(span) * float64(maxScroll))), true
}

func (p *Page) gridBarRects() (track, thumb image.Rectangle, ok bool) {
	wide := p.gridBar.drag != nil || p.hover.kind == hitBar
	return barGeometry(p.geo.grid, p.grid.ContentHeight(), p.scroll, p.grid.MaxScroll(), wide)
}

func (p *Page) navMax() int { return max(navContentHeight(p.rows)-p.geo.sidebar.Dy(), 0) }

func (p *Page) navBarRects() (track, thumb image.Rectangle, ok bool) {
	wide := p.navBar.drag != nil || p.hover.kind == hitNavBar
	return barGeometry(p.geo.sidebar, navContentHeight(p.rows), p.sideY, p.navMax(), wide)
}

func (p *Page) gridBarArea() image.Rectangle { return barAreaOf(p.geo.grid) }

func (p *Page) navBarArea() image.Rectangle { return barAreaOf(p.geo.sidebar) }

func (p *Page) setScrollRaw(v int) {
	v = p.grid.ClampScroll(v)
	if v == p.scroll {
		return
	}
	p.scroll = v
}

func (p *Page) setScroll(v int) bool {
	old := p.scroll
	p.setScrollRaw(v)
	if p.scroll == old {
		return false
	}
	p.invalidateRect(p.geo.grid)
	p.syncVisible()
	p.showGridBar()
	return true
}

func (p *Page) ScrollTo(index int) {
	if p.src() == nil {
		return
	}
	p.resync()
	if index < 0 || index >= p.grid.Count {
		return
	}
	p.reveal = index
	p.applyReveal()
}

func (p *Page) applyReveal() {
	if p.reveal < 0 || p.Bounds().Empty() || p.reveal >= p.grid.Count {
		return
	}
	i := p.reveal
	p.reveal = -1
	p.setScroll(p.grid.Reveal(i, p.scroll))
}

func (p *Page) clampSide() {
	p.sideY = min(max(p.sideY, 0), p.navMax())
}

func (p *Page) scrollSide(dy int) bool {
	old := p.sideY
	p.sideY += dy
	p.clampSide()
	if p.sideY == old {
		return false
	}
	p.invalidateRect(p.geo.sidebar)
	p.showNavBar()
	return true
}

func (p *Page) seekNav() {
	if !p.navSeek || p.Bounds().Empty() {
		return
	}
	p.navSeek = false
	view := p.geo.sidebar.Dy()
	for _, r := range p.rows {
		if !r.entry.Selected {
			continue
		}
		top := int(math.Floor(r.center-navPillHalf+.5)) - navLeft
		if top < navLeft {
			top = 0
		}
		bottom := top + 2*navPillHalf + 2*navLeft
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

func (p *Page) wheelStep() float64 {
	if p.snap.mode == ViewList {
		return float64(p.m.listRow) / 2
	}
	return float64(p.grid.CellH+p.grid.GapY) / 3
}

func (p *Page) wheel(x, y int, notches float64) bool {
	p.ensureLayout()
	if in(p.geo.sidebar, x, y) {
		p.sideFrac += notches * p.wheelStep()
		d := int(p.sideFrac)
		p.sideFrac -= float64(d)
		if d != 0 {
			p.scrollSide(d)
		}
		return true
	}
	if !in(p.geo.grid, x, y) {
		return false
	}
	p.wheelFrac += notches * p.wheelStep()
	d := int(p.wheelFrac)
	p.wheelFrac -= float64(d)
	if d != 0 {
		p.setScroll(p.scroll + d)
	}
	return true
}

func (p *Page) syncVisible() {
	if p.deps.Actions.Visible == nil || p.grid.Count == 0 {
		return
	}
	first, last := p.grid.Visible(p.scroll)
	if first == p.lastFirst && last == p.lastLast {
		return
	}
	p.lastFirst, p.lastLast = first, last
	p.deps.Actions.Visible(first, last)
}

func (p *Page) holdBar(b *scrollBar, has bool, area image.Rectangle) {
	if !has {
		return
	}
	if b.stop != nil {
		b.stop()
	}
	b.shown = true
	p.invalidateRect(area)
	b.stop = p.after(barHideDelay, func() {
		b.stop = nil
		if b.drag != nil {
			return
		}
		b.shown = false
		p.invalidateRect(area)
	})
}

func (p *Page) showGridBar() {
	_, _, ok := p.gridBarRects()
	p.holdBar(&p.gridBar, ok, p.gridBarArea())
}

func (p *Page) showNavBar() {
	_, _, ok := p.navBarRects()
	p.holdBar(&p.navBar, ok, p.navBarArea())
}

func (p *Page) dragGridBar(y int) {
	track, thumb, ok := p.gridBarRects()
	if !ok || p.gridBar.drag == nil {
		return
	}
	if v, ok := barOffset(track, thumb, p.gridBar.drag.grab, y, p.grid.MaxScroll()); ok {
		p.setScroll(v)
	}
}

func (p *Page) dragNavBar(y int) {
	track, thumb, ok := p.navBarRects()
	if !ok || p.navBar.drag == nil {
		return
	}
	if v, ok := barOffset(track, thumb, p.navBar.drag.grab, y, p.navMax()); ok {
		p.scrollSide(v - p.sideY)
	}
}
