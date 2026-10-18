package gallery

import (
	"strconv"
	"strings"

	"github.com/oops1/headless-gui/v3/widget"
)

const (
	idBack      = "back"
	idForward   = "forward"
	idUp        = "up"
	idAddress   = "address"
	idFilter    = "filter"
	idRefresh   = "refresh"
	idViewList  = "view.list"
	idViewGrid  = "view.grid"
	idThumbs    = "thumbs"
	idOpen      = "open"
	idMore      = "more"
	idPlacePref = "place."
	idItemPref  = "item."
)

var controlIDs = map[string]struct {
	hit  hit
	stop stop
}{
	idBack:     {hit{kind: hitBack}, stopBack},
	idForward:  {hit{kind: hitForward}, stopForward},
	idUp:       {hit{kind: hitUp}, stopUp},
	idAddress:  {hit{kind: hitAddress}, stopAddress},
	idFilter:   {hit{kind: hitFilter}, stopFilter},
	idRefresh:  {hit{kind: hitRefresh}, stopRefresh},
	idViewList: {hit{kind: hitViewList}, stopViews},
	idViewGrid: {hit{kind: hitViewGrid}, stopViews},
	idThumbs:   {hit{kind: hitThumbs}, stopThumbs},
	idOpen:     {hit{kind: hitOpen}, stopOpen},
	idMore:     {hit{kind: hitMore}, stopMore},
}

func placeID(index int) string { return idPlacePref + strconv.Itoa(index) }

func itemID(index int) string { return idItemPref + strconv.Itoa(index) }

func indexOfID(id, prefix string) (int, bool) {
	rest, ok := strings.CutPrefix(id, prefix)
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(rest)
	return n, err == nil && n >= 0 && strconv.Itoa(n) == rest
}

func (p *Page) AccessAction(id string, action widget.AccessActionKind) bool {
	if p.src() == nil {
		return false
	}
	p.ensureLayout()
	if c, ok := controlIDs[id]; ok {
		return p.controlAction(id, c.hit, c.stop, action)
	}
	if n, ok := indexOfID(id, idPlacePref); ok {
		return p.placeAction(n, action)
	}
	if n, ok := indexOfID(id, idItemPref); ok {
		return p.itemAction(n, action)
	}
	return false
}

func (p *Page) controlEnabled(id string) bool {
	s := p.snap
	switch id {
	case idBack:
		return s.canBack
	case idForward:
		return s.canForward
	case idUp:
		return s.canUp
	case idAddress:
		return !p.editing
	case idOpen, idMore:
		return p.Selected() >= 0
	}
	return true
}

func (p *Page) controlAction(id string, h hit, s stop, action widget.AccessActionKind) bool {
	if !p.controlEnabled(id) {
		return false
	}
	switch action {
	case widget.AccessFocus:
		if id == idAddress {
			p.startEdit()
			return p.editing
		}
		p.setFocus(s)
		return true
	case widget.AccessSelect:
		if h.kind != hitViewList && h.kind != hitViewGrid {
			return false
		}
	case widget.AccessInvoke:
	default:
		return false
	}
	p.clearKeyNav()
	p.endEditFor(id)
	p.focus = s
	p.activate(h)
	return true
}

func (p *Page) placeAction(index int, action widget.AccessActionKind) bool {
	if !p.placeVisible(index) {
		return false
	}
	switch action {
	case widget.AccessFocus:
		p.navCursor = index
		p.setFocus(stopNav)
		return true
	case widget.AccessInvoke, widget.AccessSelect:
		p.clearKeyNav()
		p.endEdit()
		p.focus = stopNav
		p.navCursor = index
		p.navigateRow(index)
		return true
	}
	return false
}

func (p *Page) placeVisible(index int) bool {
	for _, r := range p.rows {
		if r.index == index && !r.entry.Header && !r.entry.Disabled {
			return !p.navRowRect(index).Empty()
		}
	}
	return false
}

func (p *Page) itemVisible(i int) bool {
	if i < 0 || i >= len(p.snap.items) {
		return false
	}
	first, last := p.grid.Visible(p.scroll)
	return i >= first && i < last && !p.tileRect(i).Intersect(p.geo.grid).Empty()
}

func (p *Page) itemAction(i int, action widget.AccessActionKind) bool {
	if !p.itemVisible(i) {
		return false
	}
	switch action {
	case widget.AccessFocus:
		if i != p.Selected() {
			p.selectIndex(i)
		}
		p.setFocus(stopGrid)
		return true
	case widget.AccessSelect:
		p.clearKeyNav()
		p.endEdit()
		p.focus = stopGrid
		if i != p.Selected() {
			p.selectIndex(i)
		}
		return true
	case widget.AccessInvoke:
		p.clearKeyNav()
		p.endEdit()
		p.focus = stopGrid
		if i != p.Selected() {
			p.selectIndex(i)
		}
		p.openIndex(i)
		return true
	}
	return false
}

func (p *Page) endEditFor(id string) {
	if id != idAddress && id != idRefresh {
		p.endEdit()
	}
}
