package gallery

import (
	"strings"
	"unicode"

	"github.com/oops1/headless-gui/v3/widget"

	model "github.com/oops1/PixGo/internal/gallery"
)

var navKeys = map[widget.KeyCode]model.Nav{
	widget.KeyLeft:     model.NavLeft,
	widget.KeyRight:    model.NavRight,
	widget.KeyUp:       model.NavUp,
	widget.KeyDown:     model.NavDown,
	widget.KeyHome:     model.NavHome,
	widget.KeyEnd:      model.NavEnd,
	widget.KeyPageUp:   model.NavPageUp,
	widget.KeyPageDown: model.NavPageDown,
}

func (p *Page) HandleKey(e widget.KeyEvent) bool {
	if !e.Pressed || p.src() == nil {
		return false
	}
	if p.editing {
		return p.editKey(e)
	}
	ctrl, alt := e.Mod&widget.ModCtrl != 0, e.Mod&widget.ModAlt != 0
	if e.Code == widget.KeyUnknown {
		return !ctrl && !alt && p.focus == stopGrid && p.typeAhead(e.Rune)
	}
	switch {
	case alt && !ctrl:
		return p.altKey(e)
	case ctrl && !alt:
		return p.ctrlKey(e)
	case ctrl || alt:
		return false
	}
	if e.Code == widget.KeyTab {
		p.moveFocus(tabDirection(e))
		return true
	}
	return p.plainKey(e)
}

func tabDirection(e widget.KeyEvent) int {
	if e.Mod&widget.ModShift != 0 {
		return -1
	}
	return 1
}

func (p *Page) editKey(e widget.KeyEvent) bool {
	switch e.Code {
	case widget.KeyEscape:
		p.endEdit()
		return true
	case widget.KeyEnter:
		p.commitEdit()
		return true
	case widget.KeyTab:
		if e.Mod&(widget.ModCtrl|widget.ModAlt) == 0 {
			p.endEdit()
			p.moveFocus(tabDirection(e))
			return true
		}
	}
	return false
}

func (p *Page) altKey(e widget.KeyEvent) bool {
	a := p.deps.Actions
	switch e.Code {
	case widget.KeyLeft:
		p.act(a.Back, p.snap.canBack)
	case widget.KeyRight:
		p.act(a.Forward, p.snap.canForward)
	case widget.KeyUp:
		p.act(a.Up, p.snap.canUp)
	default:
		return false
	}
	return true
}

func (p *Page) ctrlKey(e widget.KeyEvent) bool {
	switch e.Code {
	case widget.KeyD:
		if !e.Repeat {
			p.toggleFavorite()
		}
	case widget.KeyR:
		if !e.Repeat && p.deps.Actions.Refresh != nil {
			p.deps.Actions.Refresh()
		}
	case widget.KeyL:
		p.FocusAddress()
	default:
		return false
	}
	return true
}

func (p *Page) plainKey(e widget.KeyEvent) bool {
	if p.focus == stopGrid {
		if p.gridKey(e) {
			return true
		}
	} else if p.controlKey(e) {
		return true
	}
	a := p.deps.Actions
	switch e.Code {
	case widget.KeyBackspace:
		p.act(a.Up, p.snap.canUp)
	case widget.KeyF5:
		if !e.Repeat && a.Refresh != nil {
			a.Refresh()
		}
	case widget.KeyF4:
		p.FocusAddress()
	case widget.KeyEscape:
		if a.Close != nil && !e.Repeat {
			a.Close()
		}
	default:
		return false
	}
	return true
}

func (p *Page) gridKey(e widget.KeyEvent) bool {
	if nav, ok := navKeys[e.Code]; ok {
		p.moveSelection(nav)
		return true
	}
	switch e.Code {
	case widget.KeyEnter:
		if i := p.Selected(); i >= 0 && !e.Repeat {
			p.openIndex(i)
		}
	case widget.KeyDelete:
		p.deleteSelected(e.Repeat)
	case widget.KeyMenu:
		p.menuFromKeyboard()
	default:
		return false
	}
	return true
}

func (p *Page) menuFromKeyboard() {
	i := p.Selected()
	if i < 0 {
		return
	}
	p.ensureLayout()
	cell := p.tileRect(i)
	p.showItemMenu(i, cell.Min.X+cell.Dx()/2, cell.Min.Y+cell.Dy()/2)
}

func (p *Page) deleteSelected(repeat bool) {
	i := p.Selected()
	if i < 0 || repeat || p.deps.Actions.Delete == nil {
		return
	}
	p.deps.Actions.Delete(i)
	p.resync()
	p.invalidate()
}

func (p *Page) toggleFavorite() {
	if i, it, ok := p.selectedItem(); ok && !it.Dir && p.deps.Actions.ToggleFavorite != nil {
		p.deps.Actions.ToggleFavorite(i)
		p.resync()
		p.invalidate()
	}
}

func (p *Page) moveSelection(nav model.Nav) {
	p.ensureLayout()
	cur := p.Selected()
	next := p.grid.Move(cur, nav)
	if next < 0 {
		return
	}
	if next != cur {
		p.selectIndex(next)
	}
	p.setScroll(p.grid.Reveal(next, p.scroll))
}

func (p *Page) typeAhead(r rune) bool {
	if r < 32 || len(p.snap.items) == 0 {
		return false
	}
	if p.stopType != nil {
		p.stopType()
	}
	p.typed = append(p.typed, unicode.ToLower(r))
	p.stopType = p.after(typeAheadReset, func() {
		p.typed = p.typed[:0]
		p.stopType = nil
	})
	prefix := string(p.typed)
	for i, it := range p.snap.items {
		if strings.HasPrefix(strings.ToLower(it.Name), prefix) {
			if i != p.Selected() {
				p.selectIndex(i)
			}
			p.ensureLayout()
			p.setScroll(p.grid.Reveal(i, p.scroll))
			break
		}
	}
	return true
}
