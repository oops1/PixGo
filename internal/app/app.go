package app

import (
	"errors"
	"image/color"
	"io/fs"
	"sync"
	"time"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/commands"
	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/platform"
	"github.com/oops1/PixGo/internal/render"
	"github.com/oops1/PixGo/internal/settings"
	"github.com/oops1/PixGo/internal/ui/infopanel"
	"github.com/oops1/PixGo/internal/ui/theme"
	"github.com/oops1/PixGo/internal/ui/view"
	"github.com/oops1/PixGo/internal/ui/window"
)

type Options struct {
	Settings *settings.Store
	Platform *platform.Platform
	Registry *codec.Registry
}

type App struct {
	listsMu sync.Mutex

	Eng      *engine.Engine
	Ctl      *Controller
	Canvas   *view.Canvas
	Root     *window.Root
	Commands *commands.Registry
	Settings *settings.Store
	Platform *platform.Platform

	Quit func()

	infoOpen     bool
	fullscreen   bool
	fsWanted     bool
	emulated     bool
	wasMaximized bool
	ops          window.WindowOps
	info         *infopanel.Panel
	menu         *widget.PopupMenu
	infoKey      string
}

func New(eng *engine.Engine, opt Options) *App {
	a := &App{Eng: eng, Settings: opt.Settings, Platform: opt.Platform, Commands: commands.NewRegistry()}
	a.Canvas = view.New(eng)
	a.Ctl = NewController(Env{
		Registry: opt.Registry,
		Settings: opt.Settings,
		Post:     eng.Post,
		After: func(d time.Duration, fn func()) func() {
			t := eng.After(d, fn)
			return t.Stop
		},
		Invalidate: eng.Invalidate,
	}, a.Canvas)
	a.Root = window.New(a, eng, window.WindowOps{})
	a.Root.After = func(d time.Duration, fn func()) func() {
		t := eng.After(d, fn)
		return t.Stop
	}
	a.Canvas.Chrome = a.Root.Owns
	a.Canvas.OnChange = func() {
		a.Root.RefreshOverlay()
		a.Root.RefreshTitle()
	}
	a.Canvas.OnStep = func(d int) { a.Ctl.Step(d) }
	a.Canvas.OnActivity = a.Root.Wake
	a.Ctl.OnChange = func() {
		a.Root.Refresh()
		a.refreshInfo()
	}
	a.loadLists()
	a.Ctl.OnLists = a.saveLists
	a.registerCommands()
	a.menu = widget.NewPopupMenu()
	a.Root.OnMore = a.showMoreMenu
	a.Canvas.OnContextMenu = a.showMenuAt
	a.ApplySettings()
	return a
}

func (a *App) Registry() *commands.Registry { return a.Commands }

func (a *App) Title() string { return a.Ctl.Title() }

func (a *App) ZoomText() string { return i18n.T("zoom.percent", a.Canvas.ZoomPercent()) }

func (a *App) State() window.ContentState {
	switch a.Ctl.Status() {
	case StatusLoading:
		return window.StateLoading
	case StatusReady:
		return window.StateReady
	case StatusFailed:
		return window.StateFailed
	}
	return window.StateEmpty
}

func (a *App) FailureText() string {
	err := a.Ctl.Err()
	switch {
	case err == nil:
		return ""
	case errors.Is(err, codec.ErrUnknownFormat), errors.Is(err, codec.ErrUnsupported):
		return i18n.T("error.unsupported")
	case errors.Is(err, codec.ErrTooLarge):
		return i18n.T("error.tooLarge")
	case errors.Is(err, codec.ErrCorrupt):
		return i18n.T("error.corrupt")
	case errors.Is(err, fs.ErrNotExist):
		return i18n.T("error.notFound")
	case errors.Is(err, fs.ErrPermission):
		return i18n.T("error.permission")
	}
	return i18n.T("error.generic")
}

func (a *App) Favorite() bool { return a.Ctl.IsFavorite() }

func (a *App) InfoOpen() bool { return a.infoOpen }

func (a *App) CanPrev() bool {
	n := a.Ctl.Navigator()
	return n != nil && (n.Index() > 0 || (n.Loop() && n.Count() > 1))
}

func (a *App) CanNext() bool {
	n := a.Ctl.Navigator()
	return n != nil && (n.Index() < n.Count()-1 || (n.Loop() && n.Count() > 1))
}

func (a *App) Content() widget.Widget { return a.Canvas }

func (a *App) AutoHide() bool { return a.Settings.Get().AutoHidePanels }

func (a *App) Fullscreen() bool { return a.fullscreen }

func (a *App) hasImage() bool { return a.Canvas.HasImage() }

func (a *App) ApplySettings() {
	st := a.Settings.Get()
	widget.SetLanguage(i18n.Lang())
	a.Canvas.Wheel = view.WheelZoom
	if st.Wheel == settings.WheelNavigate {
		a.Canvas.Wheel = view.WheelNavigate
	}
	a.Canvas.CtrlWheelZoom = st.CtrlWheelZoom
	a.Ctl.Navigator()
	if n := a.Ctl.Navigator(); n != nil {
		n.SetLoop(st.LoopFolder)
	}
	a.applyTheme()
}

func (a *App) CurrentTheme() *theme.Theme {
	st := a.Settings.Get()
	var t *theme.Theme
	switch st.Theme {
	case settings.ThemeLight:
		t = theme.Light()
	case settings.ThemeDark:
		t = theme.Dark()
	default:
		switch {
		case a.Platform != nil && a.Platform.Theme != nil && a.Platform.Theme.HighContrast():
			t = theme.HighContrast()
		case a.Platform != nil && a.Platform.Theme != nil && !a.Platform.Theme.Dark():
			t = theme.Light()
		default:
			t = theme.Dark()
		}
	}
	if a.Platform != nil && a.Platform.Theme != nil {
		if acc, ok := a.Platform.Theme.Accent(); ok {
			t = t.WithAccent(acc)
		}
	}
	return t
}

func (a *App) applyTheme() {
	t := a.CurrentTheme()
	a.Root.Theme = t
	name := "Win11 Dark"
	if !t.Dark {
		name = "Win11 Light"
	}
	if et := widget.ThemeByName(name); et != nil {
		a.Eng.SetTheme(et)
		if a.menu != nil {
			a.menu.ApplyTheme(et)
		}
	}
	a.Canvas.SetBackground(a.background(t))
	a.Eng.Invalidate()
}

func (a *App) background(t *theme.Theme) render.Background {
	bg := render.Background{Canvas: t.Canvas, Kind: render.BgSolid, Fill: t.Canvas, Checker: [2]color.RGBA{t.Checker1, t.Checker2}, Cell: 8}
	switch a.Settings.Get().TransparentBackground {
	case settings.BackgroundWhite:
		bg.Fill = color.RGBA{255, 255, 255, 255}
	case settings.BackgroundBlack:
		bg.Fill = color.RGBA{0, 0, 0, 255}
	case settings.BackgroundChecker:
		bg.Kind = render.BgChecker
	}
	return bg
}

func (a *App) SystemThemeChanged() { a.Eng.Post(a.applyTheme) }

func (a *App) OpenFiles(paths []string) {
	if len(paths) == 0 {
		return
	}
	a.Ctl.Open(paths[0])
	a.Root.Refresh()
}

func (a *App) Close() {
	if a.Quit != nil {
		a.Quit()
	}
}
