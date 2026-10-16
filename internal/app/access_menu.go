package app

import (
	"image"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/i18n"
)

const defaultMenuPadding = 2

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
	if !m.IsOpen() {
		return widget.AccessInfo{Role: widget.RolePanel}
	}
	return widget.AccessInfo{Role: widget.RoleGroup, Name: i18n.T("access.menu"), Bounds: m.OverlayBounds()}
}

func menuItemRole(it widget.MenuItem) widget.AccessRole {
	switch {
	case it.Checkable && it.RadioGroup != "":
		return widget.RoleRadioButton
	case it.Checkable:
		return widget.RoleCheckBox
	}
	return widget.RoleButton
}

func (m *contextMenu) AccessChildren() []widget.AccessInfo {
	if !m.IsOpen() {
		return nil
	}
	area := m.OverlayBounds()
	pad := m.PaddingY
	if pad <= 0 {
		pad = defaultMenuPadding
	}
	y := area.Min.Y + pad
	var out []widget.AccessInfo
	for _, it := range m.Items() {
		if it.Separator {
			y += m.SeparatorH
			continue
		}
		info := widget.AccessInfo{
			Role:        menuItemRole(it),
			Name:        it.Text,
			Description: it.Shortcut,
			Bounds:      image.Rect(area.Min.X, y, area.Max.X, y+m.ItemHeight),
		}
		if it.Checkable && it.Checked {
			info.States = append(info.States, widget.StateChecked)
			if info.Role == widget.RoleRadioButton {
				info.States = append(info.States, widget.StateSelected)
			}
		}
		if it.Disabled {
			info.States = append(info.States, widget.StateDisabled)
		}
		out = append(out, info)
		y += m.ItemHeight
	}
	return out
}
