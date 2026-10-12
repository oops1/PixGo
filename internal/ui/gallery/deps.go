package gallery

import (
	"image"
	"time"

	"github.com/oops1/headless-gui/v3/widget"

	model "github.com/oops1/PixGo/internal/gallery"
	"github.com/oops1/PixGo/internal/ui/theme"
)

type ViewMode uint8

const (
	ViewGrid ViewMode = iota
	ViewList
)

type Source interface {
	Location() model.Location
	Items() []model.Item
	Loading() bool
	Failed() bool
	TruncatedText() string
	Sidebar() []model.Entry
	Formats() []string
	Format() string
	ViewMode() ViewMode
	ThumbSize() int
	Selected() int
	Status() []string
	CanBack() bool
	CanForward() bool
	CanUp() bool
	Favorite(path string) bool
	Thumb(path string, side int, visible bool) (*image.RGBA, bool)
}

type Actions struct {
	Navigate       func(model.Location)
	Back           func()
	Forward        func()
	Up             func()
	Select         func(i int)
	Open           func(i int)
	OpenNewWindow  func(i int)
	Reveal         func(i int)
	ToggleFavorite func(i int)
	Delete         func(i int)
	SetFormat      func(format string)
	SetViewMode    func(ViewMode)
	SetThumbSize   func(px int)
	Address        func(text string) error
	Refresh        func()
	Close          func()
	Visible        func(first, last int)
	ShowMenu       func(items []widget.MenuItem, x, y int)
}

type Deps struct {
	Source  Source
	Actions Actions
	Theme   func() *theme.Theme
	After   func(d time.Duration, fn func()) (stop func())
	Now     func() time.Time
}
