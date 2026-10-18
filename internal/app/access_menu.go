package app

import (
	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/i18n"
)

type contextMenu struct {
	*widget.PopupMenu
	focus func(widget.Widget)
}

func newContextMenu(focus func(widget.Widget)) *contextMenu {
	return &contextMenu{PopupMenu: widget.NewPopupMenu(), focus: focus}
}

func (m *contextMenu) Show(x, y int) {
	m.PopupMenu.Show(x, y)
	if m.focus != nil {
		m.focus(m)
	}
}

func (m *contextMenu) AccessInfo() widget.AccessInfo {
	info := m.PopupMenu.AccessInfo()
	info.Name = i18n.T("access.menu")
	return info
}
