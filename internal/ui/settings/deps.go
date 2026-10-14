package settings

import (
	"time"

	"github.com/oops1/headless-gui/v3/widget"

	data "github.com/oops1/PixGo/internal/settings"
	"github.com/oops1/PixGo/internal/ui/theme"
)

type Language struct {
	ID   string
	Name string
}

type Format struct {
	ID         string
	Name       string
	Extensions []string
	Note       string
	Optional   bool
	Available  bool
	Enabled    bool
}

type Component struct {
	Name    string
	License string
	URL     string
}

type About struct {
	Version    string
	Revision   string
	Engine     string
	Go         string
	Platform   string
	Repository string
	License    string
	Components []Component
}

type Source interface {
	Values() data.Settings
	Languages() []Language
	Formats() []Format
	About() About
	ThumbCacheBytes() int64
	LastUpdateCheck() time.Time
}

type Actions struct {
	Update          func(change func(*data.Settings))
	Back            func()
	ClearThumbCache func()
	OpenDefaultApps func()
	CheckUpdates    func()
	OpenURL         func(url string)
	ShowMenu        func(items []widget.MenuItem, x, y int)
}

type Deps struct {
	Source  Source
	Actions Actions
	Theme   func() *theme.Theme
	After   func(d time.Duration, fn func()) (stop func())
	Now     func() time.Time
}
