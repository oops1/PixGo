package app

import (
	"image"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/commands"
	"github.com/oops1/PixGo/internal/i18n"
)

const (
	menuItemHeight = 32
	menuSepHeight  = 9
	menuPadding    = 12
)

type menuEntry struct {
	id  commands.ID
	sep bool
}

func (a *App) menuLayout() []menuEntry {
	return []menuEntry{
		{id: CmdOpen}, {id: CmdSaveAs}, {sep: true},
		{id: CmdCopy}, {id: CmdCopyImage}, {id: CmdCopyPath}, {id: CmdReveal}, {sep: true},
		{id: CmdRotateRight}, {id: CmdRotateLeft}, {id: CmdFlipH}, {id: CmdFlipV}, {sep: true},
		{id: CmdFavorite}, {id: CmdInfo}, {id: CmdFullscreen}, {sep: true},
		{id: CmdDelete}, {sep: true},
		{id: CmdCheckUpdates},
	}
}

func (a *App) menuItems() []widget.MenuItem {
	var items []widget.MenuItem
	for _, e := range a.menuLayout() {
		if e.sep {
			items = append(items, widget.MenuItem{Separator: true})
			continue
		}
		cmd := a.Commands.Get(e.id)
		if cmd == nil || cmd.Run == nil {
			continue
		}
		id := e.id
		label := cmd.Label()
		if id == CmdFavorite && a.Ctl.IsFavorite() {
			label = i18n.T("cmd.favorite.remove")
		}
		if id == CmdFullscreen && a.fullscreen {
			label = i18n.T("cmd.fullscreenExit")
		}
		items = append(items, widget.MenuItem{
			Text:     label,
			Shortcut: cmd.Shortcut(),
			Disabled: !cmd.IsEnabled(),
			OnClick:  func() { a.Commands.Run(id); a.Root.Refresh() },
		})
	}
	return trimSeparators(items)
}

func trimSeparators(items []widget.MenuItem) []widget.MenuItem {
	var out []widget.MenuItem
	for _, it := range items {
		if it.Separator && (len(out) == 0 || out[len(out)-1].Separator) {
			continue
		}
		out = append(out, it)
	}
	for len(out) > 0 && out[len(out)-1].Separator {
		out = out[:len(out)-1]
	}
	return out
}

func menuHeight(items []widget.MenuItem) int {
	h := 2 * menuPadding
	for _, it := range items {
		if it.Separator {
			h += menuSepHeight
		} else {
			h += menuItemHeight
		}
	}
	return h
}

func (a *App) showMenuAt(x, y int) {
	if a.menu == nil {
		return
	}
	a.menu.SetItems(a.menuItems())
	a.menu.Show(x, y)
}

func (a *App) showMoreMenu(anchor image.Rectangle) {
	items := a.menuItems()
	if a.menu == nil || len(items) == 0 {
		return
	}
	a.menu.SetItems(items)
	h := menuHeight(items)
	x := anchor.Min.X
	y := anchor.Max.Y + 4
	if anchor.Min.Y > a.Root.Bounds().Dy()/2 {
		y = max(0, anchor.Min.Y-4-h)
	}
	a.menu.Show(x, y)
}

func (a *App) Menu() widget.Widget {
	if a.menu == nil {
		return nil
	}
	return a.menu
}
