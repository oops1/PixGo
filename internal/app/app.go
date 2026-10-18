package app

import (
	"errors"
	"image"
	"image/color"
	"io/fs"
	"sync"
	"sync/atomic"
	"time"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/commands"
	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/platform"
	"github.com/oops1/PixGo/internal/render"
	"github.com/oops1/PixGo/internal/settings"
	"github.com/oops1/PixGo/internal/ui/confirm"
	uiedit "github.com/oops1/PixGo/internal/ui/edit"
	"github.com/oops1/PixGo/internal/ui/infopanel"
	"github.com/oops1/PixGo/internal/ui/resize"
	"github.com/oops1/PixGo/internal/ui/saveopts"
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

	infoOpen   bool
	fullscreen bool
	fsWanted   bool

	slideEnteredFullscreen bool
	emulated               bool
	wasMaximized           bool
	ops                    window.WindowOps
	info                   *infopanel.Panel
	menu                   *contextMenu
	infoKey                string
	upd                    updateHooks
	page                   window.Page
	gal                    *galleryState
	set                    *settingsState
	saveDlg                *saveopts.Dialog
	modalOpen              atomic.Bool
	modals                 modalTracker

	Editor    *uiedit.Editor
	editing   bool
	editPath  string
	resizeDlg *resize.Dialog
	ovConfirm *confirm.Dialog
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
	if w, h := eng.PhysicalSize(); w > 0 && h > 0 {
		a.Ctl.SetViewportHint(w, h)
	}
	a.Root = window.New(a, eng, window.WindowOps{})
	a.Root.After = func(d time.Duration, fn func()) func() {
		t := eng.After(d, fn)
		return t.Stop
	}
	a.Editor = uiedit.New(eng, a.Canvas)
	a.initEdit()
	a.Canvas.Chrome = a.Root.Owns
	a.Canvas.After = a.Root.After
	a.Canvas.OnChange = func() {
		a.Root.RefreshOverlay()
		a.Root.RefreshTitle()
	}
	a.Canvas.OnStep = func(d int) { a.Ctl.Step(d) }
	a.Canvas.OnActivity = a.Root.Wake
	a.Ctl.OnChange = func() {
		a.syncEdit()
		a.Root.Refresh()
		a.refreshInfo()
	}
	a.trackModals()
	a.initUpdates()
	a.loadLists()
	a.Ctl.OnLists = a.saveLists
	a.registerCommands()
	a.registerKeyboardCommands()
	a.Canvas.SetDescription(func() string { return a.Ctl.Title() })
	a.menu = newContextMenu(eng.SetFocus)
	a.Root.OnMore = a.showMoreMenu
	a.initSlideshow()
	a.Canvas.OnContextMenu = a.showMenuAt
	a.ApplySettings()
	a.Settings.Subscribe(func(old, current settings.Settings) {
		eng.Post(func() { a.settingsChanged(old, current) })
	})
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
	return n != nil && (n.Position() > 0 || (n.Loop() && n.Count() > 1))
}

func (a *App) CanNext() bool {
	n := a.Ctl.Navigator()
	return n != nil && (n.Position() < n.Count()-1 || (n.Loop() && n.Count() > 1))
}

func (a *App) Content() widget.Widget {
	switch {
	case a.editing && a.page == window.PageViewer:
		return a.Editor
	case a.page == window.PageGallery && a.gal != nil:
		return a.gal.page
	case a.page == window.PageSettings && a.set != nil:
		return a.set.page
	}
	return a.Canvas
}

func (a *App) Preview() bool { return a.Canvas.Preview() }

func (a *App) Reduced() bool {
	d := a.Ctl.Shown()
	return d != nil && d.Reduced
}

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
	if n := a.Ctl.Navigator(); n != nil {
		n.SetLoop(st.LoopFolder)
	}
	a.Canvas.Smooth = st.SmoothZoom && !a.reduceMotion()
	a.Ctl.ApplyThreads()
	a.Ctl.ApplyCache()
	a.applyTheme()
}

func (a *App) reduceMotion() bool {
	return a.Platform != nil && a.Platform.Theme != nil && a.Platform.Theme.ReduceMotion()
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
	a.Editor.Theme = t
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

func (a *App) SystemThemeChanged() { a.Eng.Post(a.ApplySettings) }

func (a *App) OpenFiles(paths []string) {
	if len(paths) == 0 {
		return
	}
	a.CloseSettings()
	a.CloseGallery()
	a.Ctl.Open(paths[0])
	a.Root.Refresh()
}

func (a *App) Handoff(args []string) {
	token, paths := DecodeHandoff(args)
	a.OpenFiles(paths)
	if a.ops.Raise != nil {
		a.ops.Raise(token)
	}
}

func (a *App) Close() {
	if a.Quit != nil {
		a.Quit()
	}
}

func (a *App) Page() window.Page { return a.page }

func (a *App) HeaderHoles() []image.Rectangle {
	switch {
	case a.page == window.PageGallery && a.gal != nil:
		return a.gal.page.HeaderHoles()
	case a.page == window.PageSettings && a.set != nil:
		return a.set.page.HeaderHoles()
	}
	return nil
}
