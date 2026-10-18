package infopanel

import (
	"strconv"

	"github.com/oops1/headless-gui/v3/widget"
)

const (
	idClose = "close"
	idCopy  = "copy"
	idLink  = "link"
)

func rowID(kind string, card, row int) string {
	return kind + "." + strconv.Itoa(card) + "." + strconv.Itoa(row)
}

func (p *Panel) hitIDs() []string {
	ids := []string{idClose}
	for ci, cp := range p.place().cards {
		for ri, rp := range cp.rows {
			if rp.row.Copy != "" {
				ids = append(ids, rowID(idCopy, ci, ri))
			}
			if rp.row.Link {
				ids = append(ids, rowID(idLink, ci, ri))
			}
		}
	}
	return ids
}

func (p *Panel) AccessAction(id string, action widget.AccessActionKind) bool {
	hs := p.hits()
	for i, hid := range p.hitIDs() {
		if hid != id || i >= len(hs) {
			continue
		}
		switch action {
		case widget.AccessInvoke:
			return p.activate(hs[i])
		case widget.AccessFocus:
			if p.OnFocus != nil {
				p.OnFocus()
			}
			p.active, p.cur = true, i
			p.revealFocus()
			p.invalidate()
			return true
		}
		return false
	}
	return false
}
