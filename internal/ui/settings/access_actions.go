package settings

import (
	"strconv"
	"strings"

	"github.com/oops1/headless-gui/v3/widget"
)

const (
	idBack     = "back"
	idNavPref  = "nav."
	idItemPref = "item."
)

func navID(s Section) string { return idNavPref + strconv.Itoa(int(s)) }

func itemID(it *item) string {
	if it.id == "" {
		return ""
	}
	return idItemPref + it.id
}

func (p *Page) AccessAction(id string, action widget.AccessActionKind) bool {
	if p.deps.Source == nil {
		return false
	}
	p.ensureLayout()
	switch {
	case id == idBack:
		return p.backAction(action)
	case strings.HasPrefix(id, idNavPref):
		n, err := strconv.Atoi(strings.TrimPrefix(id, idNavPref))
		if err != nil || n < 0 || n >= int(sectionCount) || strconv.Itoa(n) != strings.TrimPrefix(id, idNavPref) {
			return false
		}
		return p.navAction(Section(n), action)
	case strings.HasPrefix(id, idItemPref):
		if i := p.indexOf(strings.TrimPrefix(id, idItemPref)); i >= 0 && p.items[i].interactive() {
			return p.itemAction(i, action)
		}
	}
	return false
}

func (p *Page) backAction(action widget.AccessActionKind) bool {
	switch action {
	case widget.AccessFocus:
		p.setFocus(focusBack)
	case widget.AccessInvoke:
		p.keyNav = false
		p.focus = focusBack
		p.back()
	default:
		return false
	}
	return true
}

func (p *Page) navAction(s Section, action widget.AccessActionKind) bool {
	switch action {
	case widget.AccessFocus:
		if s != p.sec {
			p.SetSection(s)
		}
		p.setFocus(focusNav)
	case widget.AccessInvoke, widget.AccessSelect:
		p.keyNav = false
		p.focus = focusNav
		p.SetSection(s)
	default:
		return false
	}
	return true
}

func (p *Page) itemAction(i int, action widget.AccessActionKind) bool {
	it := &p.items[i]
	switch action {
	case widget.AccessFocus:
		p.setFocus(it.id)
		return true
	case widget.AccessToggle:
		if it.kind != kindCheck && it.kind != kindFormat {
			return false
		}
	case widget.AccessSelect:
		if it.kind != kindRadio && it.kind != kindCard {
			return false
		}
	case widget.AccessExpand:
		if it.kind != kindDropdown {
			return false
		}
	case widget.AccessInvoke:
		if it.kind == kindSlider {
			return false
		}
	default:
		return false
	}
	p.keyNav = false
	p.focus = it.id
	p.activateItem(i)
	return true
}
