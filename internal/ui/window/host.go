package window

import (
	"image"
	"time"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/commands"
	"github.com/oops1/PixGo/internal/i18n"
)

type ContentState uint8

const (
	StateEmpty ContentState = iota
	StateLoading
	StateReady
	StateFailed
)

type Page uint8

const (
	PageViewer Page = iota
	PageGallery
	PageSettings
)

type SlideshowView struct {
	Active   bool
	Playing  bool
	Index    int
	Total    int
	Progress float64
	Interval time.Duration
}

func (v SlideshowView) CounterText() string {
	return i18n.T("slideshow.counter", v.Index, v.Total)
}

func (v SlideshowView) digits() int {
	n, d := max(v.Total, v.Index), 1
	for ; n >= 10; n /= 10 {
		d++
	}
	return d
}

type Host interface {
	Page() Page
	HeaderHoles() []image.Rectangle
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
	Slideshow() SlideshowView
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
