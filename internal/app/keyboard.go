package app

import (
	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/commands"
	"github.com/oops1/PixGo/internal/ui/window"
)

const (
	CmdPanLeft     commands.ID = "view.panLeft"
	CmdPanRight    commands.ID = "view.panRight"
	CmdPanUp       commands.ID = "view.panUp"
	CmdPanDown     commands.ID = "view.panDown"
	CmdContextMenu commands.ID = "app.contextMenu"
)

func (a *App) registerKeyboardCommands() {
	pan := func(id commands.ID, title string, key commands.Key, right, down int) {
		a.Commands.Add(commands.Command{
			ID:      id,
			Title:   title,
			Keys:    []commands.Chord{commands.C(commands.Ctrl, key)},
			Enabled: a.Canvas.CanPan,
			Run:     func() { a.Canvas.PanSteps(right, down) },
		})
	}
	pan(CmdPanLeft, "cmd.panLeft", commands.KeyLeft, -1, 0)
	pan(CmdPanRight, "cmd.panRight", commands.KeyRight, 1, 0)
	pan(CmdPanUp, "cmd.panUp", commands.KeyUp, 0, -1)
	pan(CmdPanDown, "cmd.panDown", commands.KeyDown, 0, 1)
	a.Commands.Add(commands.Command{
		ID:      CmdContextMenu,
		Title:   "cmd.more",
		Keys:    []commands.Chord{commands.C(commands.Shift, commands.KeyF10), commands.C(0, commands.Key(widget.KeyMenu))},
		Enabled: func() bool { return a.page == window.PageViewer },
		Run:     a.showMenuFromKeyboard,
	})
}

func (a *App) showMenuFromKeyboard() {
	a.showMoreMenu(a.Root.Layout().Rect(window.ZoneMore))
}
