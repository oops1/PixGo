package gallery

import (
	"image"
	"time"

	"github.com/oops1/headless-gui/v3/widget"

	model "github.com/oops1/PixGo/internal/gallery"
)

const doubleClickWindow = 500 * time.Millisecond

func (p *Page) passes(x, y int) bool {
	if p.Chrome == nil {
		return false
	}
	_, owns := p.Chrome(x, y)
	return owns
}

func (p *Page) Cursor(x, y int) widget.Cursor {
	if p.Chrome != nil {
		if cur, owns := p.Chrome(x, y); owns {
			return cur
		}
	}
	if p.hitAt(x, y).kind == hitAddress {
		return widget.CursorIBeam
	}
	return widget.CursorArrow
}

func (p *Page) WantsCapture(e widget.MouseEvent) bool {
	if e.Button != widget.MouseLeft || !e.Pressed || p.passes(e.X, e.Y) {
		return false
	}
	return in(p.Bounds(), e.X, e.Y)
}

func (p *Page) hitRectOf(h hit) image.Rectangle {
	g := p.geo
	switch h.kind {
	case hitBack:
		return g.back
	case hitForward:
		return g.forward
	case hitUp:
		return g.up
	case hitAddress:
		return g.address
	case hitFilter:
		return g.filter
	case hitRefresh:
		return g.refresh
	case hitViewList, hitViewGrid:
		return g.views
	case hitThumbs:
		return g.thumbs
	case hitOpen:
		return g.open.Inset(-2)
	case hitMore:
		return g.more.Inset(-2)
	case hitTile:
		return p.tileRect(h.index).Inset(-2)
	case hitNav:
		for _, r := range p.rows {
			if r.index == h.index {
				return r.rect(g.sidebar.Min.Y-p.sideY, g.sidebar.Max.X)
			}
		}
	case hitBar, hitTrack:
		return p.gridBarArea()
	case hitNavBar:
		return p.navBarArea()
	}
	return image.Rectangle{}
}

func (p *Page) setHover(h hit) {
	if h == p.hover {
		return
	}
	old := p.hover
	p.hover = h
	p.invalidateRect(p.hitRectOf(old))
	p.invalidateRect(p.hitRectOf(h))
}

func (p *Page) setPressed(h hit) {
	if h == p.pressed {
		return
	}
	old := p.pressed
	p.pressed = h
	p.invalidateRect(p.hitRectOf(old))
	p.invalidateRect(p.hitRectOf(h))
}

func (p *Page) OnMouseMove(x, y int) {
	if widget.CursorIsNowhere(x, y) {
		p.setHover(noHit)
		return
	}
	if p.gridBar.drag != nil {
		p.dragGridBar(y)
		return
	}
	if p.navBar.drag != nil {
		p.dragNavBar(y)
		return
	}
	if p.passes(x, y) {
		p.setHover(noHit)
		return
	}
	p.ensureLayout()
	if in(p.gridBarArea(), x, y) {
		p.showGridBar()
	}
	if in(p.navBarArea(), x, y) {
		p.showNavBar()
	}
	p.setHover(p.hitAt(x, y))
}

func (p *Page) OnMouseButton(e widget.MouseEvent) bool {
	if p.gridBar.drag == nil && p.navBar.drag == nil && p.pressed.kind == hitNone && p.passes(e.X, e.Y) {
		return false
	}
	switch e.Button {
	case widget.MouseWheelUp:
		if e.Pressed {
			p.wheelEvent(e.X, e.Y, -1, e.Mod)
		}
		return true
	case widget.MouseWheelDown:
		if e.Pressed {
			p.wheelEvent(e.X, e.Y, 1, e.Mod)
		}
		return true
	case widget.MouseButtonX1:
		if e.Pressed {
			p.act(p.deps.Actions.Back, p.snap.canBack)
		}
		return true
	case widget.MouseButtonX2:
		if e.Pressed {
			p.act(p.deps.Actions.Forward, p.snap.canForward)
		}
		return true
	case widget.MouseRight:
		return p.rightButton(e)
	case widget.MouseLeft:
		return p.leftButton(e)
	}
	return false
}

func (p *Page) OnMouseWheelPixelsMod(x, y int, dx, dy float64, mod widget.KeyMod) bool {
	if dy == 0 || !in(p.Bounds(), x, y) || p.passes(x, y) {
		return false
	}
	p.ensureLayout()
	if mod&widget.ModCtrl != 0 {
		return p.stepThumbs(dy)
	}
	return p.wheel(x, y, dy/wheelNotchPixels)
}

func (p *Page) wheelEvent(x, y, dir int, mod widget.KeyMod) {
	if mod&widget.ModCtrl != 0 {
		p.stepThumbs(float64(dir))
		return
	}
	p.wheel(x, y, float64(dir))
}

func (p *Page) stepThumbs(dir float64) bool {
	f := p.deps.Actions.SetThumbSize
	if f == nil || dir == 0 {
		return false
	}
	step := 1
	if dir > 0 {
		step = -1
	}
	next := model.StepThumbSize(p.snap.side, step)
	if next != p.snap.side {
		f(next)
		p.resync()
		p.invalidate()
	}
	return true
}

func (p *Page) act(f func(), enabled bool) {
	if f != nil && enabled {
		f()
		p.resync()
		p.invalidate()
	}
}

func (p *Page) isDouble(e widget.MouseEvent, index int) bool {
	if e.Clicks >= 2 {
		return true
	}
	now := p.now()
	double := e.Clicks == 0 && p.lastTap.index == index && !p.lastTap.at.IsZero() && now.Sub(p.lastTap.at) < doubleClickWindow
	if double {
		p.lastTap = tap{index: -1}
	} else {
		p.lastTap = tap{index: index, at: now}
	}
	return double
}

func (p *Page) leftButton(e widget.MouseEvent) bool {
	p.ensureLayout()
	h := p.hitAt(e.X, e.Y)
	if e.Pressed {
		return p.pressLeft(e, h)
	}
	return p.releaseLeft(h)
}

func (p *Page) pressLeft(e widget.MouseEvent, h hit) bool {
	p.clearKeyNav()
	p.focusFromHit(h)
	if p.editing {
		switch h.kind {
		case hitAddress:
			return true
		case hitRefresh:
		default:
			p.endEdit()
		}
	}
	p.setPressed(h)
	switch h.kind {
	case hitTile:
		if h.index != p.Selected() {
			p.selectIndex(h.index)
		}
		if p.isDouble(e, h.index) {
			p.setPressed(noHit)
			p.openIndex(h.index)
		}
	case hitBlank:
		if p.Selected() >= 0 {
			p.selectIndex(-1)
		}
	case hitBar:
		if _, thumb, ok := p.gridBarRects(); ok {
			p.gridBar.drag = &barDrag{grab: e.Y - thumb.Min.Y}
			p.invalidateRect(p.gridBarArea())
		}
	case hitNavBar:
		if _, thumb, ok := p.navBarRects(); ok {
			p.navBar.drag = &barDrag{grab: e.Y - thumb.Min.Y}
			p.invalidateRect(p.navBarArea())
		}
	}
	return true
}

func (p *Page) releaseLeft(h hit) bool {
	pressed := p.pressed
	p.setPressed(noHit)
	if p.gridBar.drag != nil {
		p.gridBar.drag = nil
		p.showGridBar()
		p.invalidateRect(p.gridBarArea())
		return true
	}
	if p.navBar.drag != nil {
		p.navBar.drag = nil
		p.showNavBar()
		p.invalidateRect(p.navBarArea())
		return true
	}
	if pressed.kind == hitNone || pressed != h {
		return true
	}
	p.activate(h)
	return true
}

func (p *Page) activate(h hit) {
	a := p.deps.Actions
	g := p.geo
	switch h.kind {
	case hitBack:
		p.act(a.Back, p.snap.canBack)
	case hitForward:
		p.act(a.Forward, p.snap.canForward)
	case hitUp:
		p.act(a.Up, p.snap.canUp)
	case hitAddress:
		p.startEdit()
	case hitFilter:
		p.showFilterMenu(g.filter.Min.X, g.address.Max.Y)
	case hitRefresh:
		if p.editing {
			p.commitEdit()
		} else if a.Refresh != nil {
			a.Refresh()
		}
	case hitViewList:
		p.setMode(ViewList)
	case hitViewGrid:
		p.setMode(ViewGrid)
	case hitThumbs:
		p.showSizeMenu(g.thumbs.Min.X, g.thumbs.Max.Y)
	case hitNav:
		p.navigateRow(h.index)
	case hitOpen:
		if i := p.Selected(); i >= 0 {
			p.openIndex(i)
		}
	case hitMore:
		if i := p.Selected(); i >= 0 {
			p.showMoreMenu(i)
		}
	}
}

func (p *Page) setMode(m ViewMode) {
	if m == p.snap.mode || p.deps.Actions.SetViewMode == nil {
		return
	}
	p.deps.Actions.SetViewMode(m)
	p.resync()
	p.setScrollRaw(0)
	p.invalidate()
}

func (p *Page) navigateRow(index int) {
	for _, r := range p.rows {
		if r.index == index && !r.entry.Header && !r.entry.Disabled {
			if f := p.deps.Actions.Navigate; f != nil {
				f(r.entry.Loc)
				p.resync()
				p.invalidate()
			}
			return
		}
	}
}

func (p *Page) rightButton(e widget.MouseEvent) bool {
	if e.Pressed {
		return true
	}
	p.ensureLayout()
	h := p.hitAt(e.X, e.Y)
	if h.kind != hitTile {
		return true
	}
	if h.index != p.Selected() {
		p.selectIndex(h.index)
	}
	p.showItemMenu(h.index, e.X, e.Y)
	return true
}

func (p *Page) selectIndex(i int) {
	old := p.Selected()
	if f := p.deps.Actions.Select; f != nil {
		f(i)
	}
	p.snap.status = p.src().Status()
	p.invalidateSelection(old, p.Selected())
}

func (p *Page) invalidateSelection(old, cur int) {
	if old >= 0 {
		p.invalidateRect(p.tileRect(old).Inset(-3))
	}
	if cur >= 0 {
		p.invalidateRect(p.tileRect(cur).Inset(-3))
	}
	g := p.geo
	p.invalidateRect(image.Rect(g.status.Min.X, g.statusY-8, g.status.Max.X, g.status.Max.Y))
}

func (p *Page) openIndex(i int) {
	if f := p.deps.Actions.Open; f != nil && i >= 0 && i < len(p.snap.items) {
		f(i)
		p.resync()
		p.invalidate()
	}
}

func (p *Page) resync() {
	p.take()
	p.relayout()
	p.clampSide()
	p.seekNav()
	p.setScrollRaw(p.scroll)
	p.syncVisible()
}
