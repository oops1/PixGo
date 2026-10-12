package main

import (
	"errors"
	"flag"
	"fmt"
	"image"
	"image/png"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/window"

	"github.com/oops1/PixGo/internal/app"
	"github.com/oops1/PixGo/internal/buildinfo"
	"github.com/oops1/PixGo/internal/codec"
	codecall "github.com/oops1/PixGo/internal/codec/all"
	"github.com/oops1/PixGo/internal/fonts"
	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/platform"
	"github.com/oops1/PixGo/internal/settings"
	"github.com/oops1/PixGo/internal/ui/icons"
	uiwin "github.com/oops1/PixGo/internal/ui/window"
)

const (
	appID         = "pixgo"
	defaultWidth  = 1100
	defaultHeight = 700
	minWidth      = 420
	minHeight     = 320

	updateCheckDelay = 4 * time.Second
)

func main() {
	screenshot := flag.String("screenshot", "", "render one PNG without a window and exit")
	width := flag.Int("width", 0, "window width")
	height := flag.Int("height", 0, "window height")
	themeName := flag.String("theme", "", "system, light or dark")
	lang := flag.String("lang", "", "interface language: ru or en")
	newWindow := flag.Bool("new-window", false, "do not hand the files to a running window")
	mockup := flag.Bool("mockup", false, "draw every control enabled")
	showVersion := flag.Bool("version", false, "print the version and exit")
	setDefault := flag.Bool("set-default", false, "make PixGo the default image viewer and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println("PixGo", buildinfo.String())
		return
	}
	if *setDefault {
		if err := platform.NewAssociations().SetAsDefault(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if *lang != "" {
		i18n.SetLang(*lang)
	}
	paths := absPaths(flag.Args())

	store, err := settings.Load(settings.Path())
	if err != nil {
		slog.Warn("settings", "err", err)
	}
	if *themeName != "" {
		th := *themeName
		store.Update(func(s *settings.Settings) { s.Theme = th })
	}
	if *lang != "" {
		l := *lang
		store.Update(func(s *settings.Settings) { s.Language = l })
	} else if l := store.Get().Language; l != settings.LanguageSystem && l != "" {
		i18n.SetLang(l)
	}
	st := store.Get()

	plat := platform.New()
	var primary bool
	if *screenshot == "" {
		var ierr error
		primary, ierr = plat.Instance.Acquire()
		if ierr == nil && !primary && !*newWindow && st.SingleWindow {
			if err := plat.Instance.Send(paths); err == nil {
				return
			}
		}
	}

	ws, _ := settings.LoadWindowState()
	w, h := defaultWidth, defaultHeight
	if st.RememberWindow && ws.HasBounds() {
		w, h = ws.W, ws.H
	}
	if *width > 0 {
		w = *width
	}
	if *height > 0 {
		h = *height
	}
	eng := engine.New(max(minWidth, w), max(minHeight, h), 60)
	if err := fonts.Register(eng); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	reg := codec.NewRegistry()
	codecall.Register(reg)
	for _, id := range settings.OptionalFormats {
		reg.SetEnabled(codec.ID(id), st.FormatEnabled(id))
	}

	a := app.New(eng, app.Options{Settings: store, Platform: plat, Registry: reg})
	a.Root.ForceEnabled = *mockup

	if *screenshot != "" {
		eng.SetRootFullCanvas(a.Root)
		if len(paths) > 0 {
			a.Ctl.Open(paths[0])
		}
		waitForImage(eng, a)
		a.Root.Refresh()
		eng.Flush()
		if err := savePNG(*screenshot, eng.RenderOnce()); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		a.Ctl.Close()
		return
	}

	eng.SetRootFullCanvas(a.Root)
	eng.SetFocus(a.Canvas)
	win := window.New(&uiwin.KeyRouter{Engine: eng, Target: a.Root}, i18n.T("app.name")).SetResizable(true).SetAppID(appID)
	win.SetMinSize(minWidth, minHeight)
	win.SetIcon(icons.AppIcon(256), icons.AppIcon(64), icons.AppIcon(32), icons.AppIcon(16))
	win.SetCornerRadius(uiwin.CornerRadius)
	win.SetHitTest(func(x, y int) window.HitArea { return window.HitArea(a.Root.HitTest(x, y)) })
	a.SetOps(windowOps(eng, win, a))
	win.SetTouchEnabled(true)
	win.SetOnFullscreenChanged(func(on bool) { eng.Post(func() { a.OnFullscreenChanged(on) }) })
	a.Quit = func() {
		saveWindowState(eng, win, a)
		store.Flush()
		a.Ctl.Close()
		win.Close()
	}
	win.SetOnCloseRequest(func() bool { a.Close(); return false })
	win.SetOnFilesDropped(func(p []string, x, y int) { eng.Post(func() { a.OpenFiles(p) }) })
	stopTheme := plat.Theme.Watch(a.SystemThemeChanged)
	defer stopTheme()
	if primary {
		plat.Instance.Listen(func(args []string) { eng.Post(func() { a.Handoff(absPaths(args)) }) })
		defer plat.Instance.Close()
	}
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sig
		eng.Post(a.Close)
	}()
	eng.Post(func() {
		switch {
		case len(paths) > 0:
			a.OpenFiles(paths)
		case st.RestoreLastImage && ws.LastFile != "":
			if _, err := os.Stat(ws.LastFile); err == nil {
				a.OpenFiles([]string{ws.LastFile})
			}
		}
		a.Root.Refresh()
		a.Root.Wake()
	})
	eng.After(updateCheckDelay, a.ScheduleUpdateCheck)
	eng.Start()
	if err := win.Run(); err != nil {
		slog.Error("window", "err", err)
	}
	eng.Stop()
}

func windowOps(eng *engine.Engine, win *window.Window, a *app.App) uiwin.WindowOps {
	return uiwin.WindowOps{
		Minimize: func() { win.Native().Minimize() },
		ToggleMaximize: func() {
			n := win.Native()
			if n.IsMaximized() {
				n.Restore()
			} else {
				n.Maximize()
			}
			a.Root.Refresh()
		},
		Maximized: func() bool { return win.Native() != nil && win.Native().IsMaximized() },
		Close:     a.Close,
		MoveBy: func(dx, dy int) {
			n := win.Native()
			if n.IsMaximized() {
				return
			}
			x, y := n.GetPosition()
			k := eng.Scale()
			n.SetPosition(x+int(float64(dx)*k), y+int(float64(dy)*k))
		},
		BeginMove:     func() bool { return !win.Native().IsMaximized() && win.BeginMove() },
		BeginResize:   func(edges int) bool { return !win.Native().IsMaximized() && win.BeginResize(edges) },
		SetFullscreen: win.SetFullscreen,
		Raise: func() {
			if err := platform.RaiseWindow(win.Native()); err != nil && !errors.Is(err, platform.ErrUnsupported) {
				slog.Warn("raise window", "err", err)
			}
		},
		Maximize: func(on bool) {
			n := win.Native()
			if on {
				n.Maximize()
			} else {
				n.Restore()
			}
		},
	}
}

func saveWindowState(eng *engine.Engine, win *window.Window, a *app.App) {
	if !a.Settings.Get().RememberWindow {
		return
	}
	n := win.Native()
	if n == nil {
		return
	}
	ws := settings.DefaultWindowState()
	ws.Maximized = n.IsMaximized()
	if !ws.Maximized {
		ws.W, ws.H = n.GetSize()
		ws.X, ws.Y = n.GetPosition()
	} else if old, err := settings.LoadWindowState(); err == nil {
		ws = old
		ws.Maximized = true
	}
	ws.LastFile = a.Ctl.Path()
	if err := ws.Save(); err != nil {
		slog.Warn("window state", "err", err)
	}
}

func waitForImage(eng *engine.Engine, a *app.App) {
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		eng.Flush()
		if s := a.Ctl.Status(); s != app.StatusLoading {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	eng.Flush()
}

func absPaths(args []string) []string {
	var out []string
	for _, p := range args {
		if abs, err := filepath.Abs(p); err == nil {
			out = append(out, abs)
		}
	}
	return out
}

func savePNG(path string, img *image.RGBA) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
