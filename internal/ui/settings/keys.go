package settings

import (
	"image"

	"github.com/oops1/headless-gui/v3/widget"
)

func (p *Page) groupLeader(g int) int {
	first := -1
	for i := range p.items {
		it := &p.items[i]
		if it.group != g || !it.focusable() {
			continue
		}
		if first < 0 {
			first = i
		}
		if it.on {
			return i
		}
	}
	return first
}

func (p *Page) stops() []string {
	out := []string{focusBack, focusNav}
	for i := range p.items {
		it := &p.items[i]
		if !it.focusable() || it.id == "" {
			continue
		}
		if it.grouped() && p.groupLeader(it.group) != i {
			continue
		}
		out = append(out, it.id)
	}
	return out
}

func (p *Page) focusIndex(stops []string) int {
	for i, s := range stops {
		if s == p.focus {
			return i
		}
	}
	if i := p.indexOf(p.focus); i >= 0 {
		g := p.items[i].group
		if g != 0 {
			if l := p.groupLeader(g); l >= 0 {
				for k, s := range stops {
					if s == p.items[l].id {
						return k
					}
				}
			}
		}
	}
	return -1
}

func (p *Page) setFocus(id string) {
	if id == p.focus && p.keyNav {
		return
	}
	p.invalidateFocus()
	p.focus = id
	p.keyNav = true
	p.invalidateFocus()
	if i := p.indexOf(id); i >= 0 {
		p.reveal(p.items[i].r)
	}
}

func (p *Page) invalidateFocus() {
	switch p.focus {
	case "":
	case focusBack:
		p.invalidateRect(p.geo.back.Inset(-4))
	case focusNav:
		p.invalidateRect(p.geo.nav)
	default:
		if i := p.indexOf(p.focus); i >= 0 {
			p.invalidateRect(p.items[i].r.Add(image.Pt(0, -p.scroll)).Inset(-6))
		}
	}
}

func (p *Page) moveFocus(dir int) {
	stops := p.stops()
	cur := p.focusIndex(stops)
	var next int
	switch {
	case cur < 0 && dir > 0:
		next = 0
	case cur < 0:
		next = len(stops) - 1
	default:
		next = (cur + dir + len(stops)) % len(stops)
	}
	p.setFocus(stops[next])
}

func (p *Page) moveContentFocus(dir int) {
	stops := p.stops()
	cur := p.focusIndex(stops)
	next := cur + dir
	if next < 2 || next >= len(stops) {
		return
	}
	p.setFocus(stops[next])
}

func (p *Page) HandleKey(e widget.KeyEvent) bool {
	if !e.Pressed || p.deps.Source == nil {
		return false
	}
	p.ensureLayout()
	ctrl, alt, shift := e.Mod&widget.ModCtrl != 0, e.Mod&widget.ModAlt != 0, e.Mod&widget.ModShift != 0
	switch {
	case alt && !ctrl:
		return p.altKey(e)
	case ctrl && !alt:
		return p.ctrlKey(e, shift)
	case ctrl || alt || e.Code == widget.KeyUnknown:
		return false
	}
	switch e.Code {
	case widget.KeyEscape, widget.KeyBackspace:
		if !e.Repeat {
			p.back()
		}
		return true
	case widget.KeyTab:
		if shift {
			p.moveFocus(-1)
		} else {
			p.moveFocus(1)
		}
		return true
	}
	if p.focusKey(e, shift) {
		p.keyNav = true
		return true
	}
	return p.scrollKey(e)
}

func (p *Page) altKey(e widget.KeyEvent) bool {
	if e.Code == widget.KeyLeft {
		if !e.Repeat {
			p.back()
		}
		return true
	}
	if e.Code == widget.KeyDown {
		if i := p.indexOf(p.focus); i >= 0 && p.items[i].kind == kindDropdown {
			p.openDropdown(i)
			return true
		}
	}
	return false
}

func (p *Page) ctrlKey(e widget.KeyEvent, shift bool) bool {
	switch e.Code {
	case widget.KeyTab:
		if shift {
			p.stepSection(-1)
		} else {
			p.stepSection(1)
		}
	case widget.KeyPageDown:
		p.stepSection(1)
	case widget.KeyPageUp:
		p.stepSection(-1)
	default:
		return false
	}
	return true
}

func (p *Page) stepSection(dir int) {
	n := int(sectionCount)
	p.SetSection(Section((int(p.sec) + dir + n) % n))
	p.keyNav = true
	if p.focus == "" {
		p.focus = focusNav
	}
	p.invalidate()
}

func (p *Page) activateKey(e widget.KeyEvent) bool {
	return e.Code == widget.KeySpace || e.Code == widget.KeyEnter
}

func (p *Page) focusKey(e widget.KeyEvent, shift bool) bool {
	switch p.focus {
	case focusBack:
		if p.activateKey(e) {
			if !e.Repeat {
				p.back()
			}
			return true
		}
		return false
	case focusNav:
		return p.navKey(e)
	}
	i := p.indexOf(p.focus)
	if i < 0 {
		return false
	}
	it := &p.items[i]
	switch it.kind {
	case kindCheck, kindFormat:
		return p.toggleKey(i, e)
	case kindRadio, kindCard:
		return p.groupKey(i, e)
	case kindDropdown:
		return p.dropdownKey(i, e)
	case kindSlider:
		return p.sliderKey(it, e)
	case kindButton, kindLink:
		if p.activateKey(e) {
			if !e.Repeat {
				p.activateItem(i)
			}
			return true
		}
		return p.verticalKey(e)
	}
	return false
}

func (p *Page) verticalKey(e widget.KeyEvent) bool {
	switch e.Code {
	case widget.KeyUp:
		p.moveContentFocus(-1)
	case widget.KeyDown:
		p.moveContentFocus(1)
	default:
		return false
	}
	return true
}

func (p *Page) toggleKey(i int, e widget.KeyEvent) bool {
	if p.activateKey(e) {
		if !e.Repeat {
			p.activateItem(i)
		}
		return true
	}
	return p.verticalKey(e)
}

func (p *Page) navKey(e widget.KeyEvent) bool {
	n := int(sectionCount)
	switch e.Code {
	case widget.KeyUp, widget.KeyLeft:
		p.SetSection(Section((int(p.sec) + n - 1) % n))
	case widget.KeyDown, widget.KeyRight:
		p.SetSection(Section((int(p.sec) + 1) % n))
	case widget.KeyHome:
		p.SetSection(0)
	case widget.KeyEnd:
		p.SetSection(sectionCount - 1)
	default:
		return false
	}
	p.focus = focusNav
	return true
}

func (p *Page) groupKey(i int, e widget.KeyEvent) bool {
	it := &p.items[i]
	dir := 0
	switch e.Code {
	case widget.KeyUp, widget.KeyLeft:
		dir = -1
	case widget.KeyDown, widget.KeyRight:
		dir = 1
	case widget.KeySpace, widget.KeyEnter:
		if !e.Repeat && !it.on {
			p.activateItem(i)
		}
		return true
	default:
		return false
	}
	var members []int
	for k := range p.items {
		if p.items[k].group == it.group && p.items[k].focusable() {
			members = append(members, k)
		}
	}
	pos := 0
	for k, m := range members {
		if m == i {
			pos = k
		}
	}
	next := members[(pos+dir+len(members))%len(members)]
	id := p.items[next].id
	p.activateItem(next)
	p.focus = id
	return true
}

func (p *Page) dropdownKey(i int, e widget.KeyEvent) bool {
	it := &p.items[i]
	switch e.Code {
	case widget.KeySpace, widget.KeyEnter:
		if !e.Repeat {
			p.openDropdown(i)
		}
		return true
	case widget.KeyUp, widget.KeyDown:
		cur := 0
		for k, ch := range it.choices {
			if ch.on {
				cur = k
			}
		}
		next := cur + 1
		if e.Code == widget.KeyUp {
			next = cur - 1
		}
		if next >= 0 && next < len(it.choices) && it.choices[next].change != nil {
			p.update(it.choices[next].change)
		}
		return true
	}
	return false
}

func (p *Page) sliderKey(it *item, e widget.KeyEvent) bool {
	switch e.Code {
	case widget.KeyLeft, widget.KeyDown:
		p.stepSlider(it, -1)
	case widget.KeyRight, widget.KeyUp:
		p.stepSlider(it, 1)
	case widget.KeyPageDown:
		p.stepSlider(it, -10)
	case widget.KeyPageUp:
		p.stepSlider(it, 10)
	case widget.KeyHome:
		p.setSliderValue(it, it.lo)
	case widget.KeyEnd:
		p.setSliderValue(it, it.hi)
	default:
		return false
	}
	return true
}

func (p *Page) scrollKey(e widget.KeyEvent) bool {
	page := max(p.geo.view.Dy()-dim.pageStepKeep, 40)
	switch e.Code {
	case widget.KeyPageDown:
		p.scrollBy(page)
	case widget.KeyPageUp:
		p.scrollBy(-page)
	case widget.KeyHome:
		p.setScroll(0)
	case widget.KeyEnd:
		p.setScroll(p.maxScroll())
	case widget.KeyDown:
		p.scrollBy(dim.scrollKeyStep)
	case widget.KeyUp:
		p.scrollBy(-dim.scrollKeyStep)
	default:
		return false
	}
	return true
}
