package window

import (
	"image"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/commands"
)

type ContentState uint8

const (
	StateEmpty ContentState = iota
	StateLoading
	StateReady
	StateFailed
)

type Host interface {
	Registry() *commands.Registry
	Title() string
	ZoomText() string
	State() ContentState
	FailureText() string
	Favorite() bool
	InfoOpen() bool
	CanPrev() bool
	CanNext() bool
	Content() widget.Widget
	Panel() widget.Widget
	Menu() widget.Widget
	AutoHide() bool
	Fullscreen() bool
}

type Engine interface {
	Invalidate()
	InvalidateRect(r image.Rectangle)
	Post(fn func())
	SetFocus(w widget.Widget)
}

type WindowOps struct {
	Minimize       func()
	ToggleMaximize func()
	Maximized      func() bool
	Close          func()
	MoveBy         func(dx, dy int)
	BeginMove      func() bool
	BeginResize    func(edges int) bool
	SetFullscreen  func(on bool) bool
	Maximize       func(on bool)
	Raise          func()
}

type CursorHider interface {
	SetCursorOverride(c widget.Cursor)
	ClearCursorOverride()
}

const (
	EdgeTop    = 1
	EdgeBottom = 2
	EdgeLeft   = 4
	EdgeRight  = 8
)
