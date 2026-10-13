package settings

import (
	"image"
	"math"

	"github.com/oops1/headless-gui/v3/widget"
)

func (p *Page) passes(x, y int) bool {
	if p.Chrome == nil {
		return false
	}
	_, owns := p.Chrome(x, y)
	return owns
}

func (it *item) interactive() bool {
	switch it.kind {
	case kindCheck, kindRadio, kindCard, kindDropdown, kindSlider, kindButton, kindLink:
		return !it.disabled
	case kindFormat:
		return it.change != nil
	}
	return false
}

func (it *item) hitRect() image.Rectangle {
	if it.kind == kindSlider {
		_, x1, _ := sliderSpan(it.r)
		return image.Rect(it.r.Min.X, it.r.Min.Y, x1+sliderThumb, it.r.Max.Y)
	}
	return it.box
}

func (p *Page) hitAt(x, y int) hit {
	if !in(p.Bounds(), x, y) {
		return noHit
	}
	p.ensureLayout()
	g := p.geo
	switch {
	case in(g.back, x, y):
		return hit{kind: hitBack}
	case in(g.nav, x, y):
		for s := Section(0); s < sectionCount; s++ {
			if in(navRowRect(s), x, y) {
				return hit{kind: hitNav, index: int(s)}
			}
		}
	case in(g.view, x, y):
		if _, thumb, ok := p.barRects(); ok && p.bar.shown && in(thumb.Inset(-6), x, y) {
			return hit{kind: hitBar}
		}
		off := image.Pt(0, -p.scroll)
		for i := range p.items {
			it := &p.items[i]
			if it.interactive() && in(it.hitRect().Add(off), x, y) {
				return hit{kind: hitItem, index: i}
			}
		}
	}
	return noHit
}

func (p *Page) hitRectOf(h hit) image.Rectangle {
	switch h.kind {
	case hitBack:
		return p.geo.back.Inset(-4)
	case hitNav:
		return navRowRect(Section(h.index)).Inset(-4)
	case hitBar:
		return p.barArea()
	case hitItem:
		if h.index >= 0 && h.index < len(p.items) {
			return p.items[h.index].r.Add(image.Pt(0, -p.scroll)).Inset(-6)
		}
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

func (p *Page) Cursor(x, y int) widget.Cursor {
	if p.Chrome != nil {
		if cur, owns := p.Chrome(x, y); owns {
			return cur
		}
	}
	if h := p.hitAt(x, y); h.kind == hitItem {
		if k := p.items[h.index].kind; k == kindCard || k == kindLink {
			return widget.CursorHand
		}
	}
	return widget.CursorArrow
}

func (p *Page) WantsCapture(e widget.MouseEvent) bool {
	if e.Button != widget.MouseLeft || !e.Pressed || p.passes(e.X, e.Y) {
		return false
	}
	return in(p.Bounds(), e.X, e.Y)
}

func (p *Page) OnMouseMove(x, y int) {
	if widget.CursorIsNowhere(x, y) {
		p.setHover(noHit)
		return
	}
	if p.drag != nil {
		if p.drag.bar {
			p.dragBar(y)
		} else {
			p.dragSlider(x)
		}
		return
	}
	if p.passes(x, y) {
		p.setHover(noHit)
		return
	}
	p.ensureLayout()
	if in(p.barArea(), x, y) {
		p.showBar()
	}
	p.setHover(p.hitAt(x, y))
}

func (p *Page) OnMouseButton(e widget.MouseEvent) bool {
	if p.drag == nil && p.pressed.kind == hitNone && p.passes(e.X, e.Y) {
		return false
	}
	switch e.Button {
	case widget.MouseWheelUp:
		if e.Pressed {
			p.wheelEvent(e.X, e.Y, -1)
		}
		return true
	case widget.MouseWheelDown:
		if e.Pressed {
			p.wheelEvent(e.X, e.Y, 1)
		}
		return true
	case widget.MouseButtonX1:
		if e.Pressed {
			p.back()
		}
		return true
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
	if !in(p.geo.view, x, y) {
		return false
	}
	return p.wheel(dy / wheelNotchPixels)
}

func (p *Page) wheelEvent(x, y, dir int) {
	p.ensureLayout()
	if in(p.geo.view, x, y) {
		p.wheel(float64(dir))
	}
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
	p.keyNav = false
	p.setPressed(h)
	switch h.kind {
	case hitBar:
		if _, thumb, ok := p.barRects(); ok {
			p.drag = &dragState{bar: true, grab: e.Y - thumb.Min.Y}
			p.invalidateRect(p.barArea())
		}
	case hitItem:
		it := &p.items[h.index]
		if it.id != "" {
			p.focus = it.id
		}
		if it.kind == kindSlider {
			p.drag = &dragState{id: it.id}
			p.dragSlider(e.X)
		}
	case hitBack:
		p.focus = focusBack
	case hitNav:
		p.focus = focusNav
	}
	return true
}

func (p *Page) releaseLeft(h hit) bool {
	pressed := p.pressed
	p.setPressed(noHit)
	if d := p.drag; d != nil {
		p.drag = nil
		if d.bar {
			p.showBar()
		}
		p.invalidate()
		return true
	}
	if pressed.kind == hitNone || pressed != h {
		return true
	}
	p.activate(h)
	return true
}

func (p *Page) activate(h hit) {
	switch h.kind {
	case hitBack:
		p.back()
	case hitNav:
		p.SetSection(Section(h.index))
	case hitItem:
		p.activateItem(h.index)
	}
}

func (p *Page) activateItem(i int) {
	if i < 0 || i >= len(p.items) {
		return
	}
	it := &p.items[i]
	switch it.kind {
	case kindCheck, kindFormat:
		if it.change != nil {
			p.update(it.change)
		}
	case kindRadio, kindCard:
		if !it.on && it.change != nil {
			p.update(it.change)
		}
	case kindDropdown:
		p.openDropdown(i)
	case kindButton, kindLink:
		if it.run != nil {
			it.run()
		}
	}
}

func (p *Page) openDropdown(i int) {
	f := p.deps.Actions.ShowMenu
	it := &p.items[i]
	if f == nil || len(it.choices) == 0 {
		return
	}
	items := make([]widget.MenuItem, 0, len(it.choices))
	for _, ch := range it.choices {
		change := ch.change
		items = append(items, widget.MenuItem{Text: ch.text, Checkable: true, Checked: ch.on, OnClick: func() { p.update(change) }})
	}
	r := it.r.Add(image.Pt(0, -p.scroll))
	f(items, r.Min.X, r.Max.Y+2)
}

func (p *Page) itemByID(id string) *item {
	if i := p.indexOf(id); i >= 0 {
		return &p.items[i]
	}
	return nil
}

func sliderValueAt(it *item, x int) int {
	x0, x1, _ := sliderSpan(it.r)
	span := it.hi - it.lo
	if x1 <= x0 || span <= 0 {
		return it.lo
	}
	f := math.Min(math.Max(float64(x-x0)/float64(x1-x0), 0), 1)
	step := max(it.step, 1)
	steps := int(math.Round(f * float64(span) / float64(step)))
	return min(it.lo+steps*step, it.hi)
}

func (p *Page) dragSlider(x int) {
	if p.drag == nil {
		return
	}
	it := p.itemByID(p.drag.id)
	if it == nil || it.slide == nil {
		return
	}
	if v := sliderValueAt(it, x); v != it.value {
		p.update(it.slide(v))
	}
}

func (p *Page) stepSlider(it *item, delta int) {
	if it.slide == nil {
		return
	}
	v := min(max(it.value+delta*max(it.step, 1), it.lo), it.hi)
	if v != it.value {
		p.update(it.slide(v))
	}
}

func (p *Page) setSliderValue(it *item, v int) {
	if it.slide != nil && v != it.value {
		p.update(it.slide(v))
	}
}
