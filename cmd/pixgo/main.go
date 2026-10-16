package main

import (
	"flag"
	"fmt"
	"image"
	"image/png"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
	"github.com/oops1/headless-gui/v3/window"

	"github.com/oops1/PixGo/internal/app"
	"github.com/oops1/PixGo/internal/buildinfo"
	"github.com/oops1/PixGo/internal/codec"
	codecall "github.com/oops1/PixGo/internal/codec/all"
	"github.com/oops1/PixGo/internal/fonts"
	model "github.com/oops1/PixGo/internal/gallery"
	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/platform"
	"github.com/oops1/PixGo/internal/settings"
	"github.com/oops1/PixGo/internal/ui/icons"
	uiwin "github.com/oops1/PixGo/internal/ui/window"
)

const (
	defaultWidth  = 1100
	defaultHeight = 700
	minWidth      = 420
	minHeight     = 320

	updateCheckDelay = 4 * time.Second
)

var appID = "pixgo"

func main() {
	platform.SetDesktopID(appID + ".desktop")
	activationToken := window.TakeActivationToken()
	screenshot := flag.String("screenshot", "", "render one PNG without a window and exit")
	width := flag.Int("width", 0, "window width")
	height := flag.Int("height", 0, "window height")
	themeName := flag.String("theme", "", "system, light or dark")
	lang := flag.String("lang", "", "interface language: ru or en")
	newWindow := flag.Bool("new-window", false, "do not hand the files to a running window")
	mockup := flag.Bool("mockup", false, "draw every control enabled")
	showVersion := flag.Bool("version", false, "print the version and exit")
	setDefault := flag.Bool("set-default", false, "make PixGo the default image viewer and exit")
	galleryDir := flag.String("gallery", "", "open the gallery on this folder")
	openSettings := flag.Bool("settings", false, "open the settings page")
	fontSize := flag.Float64("font-size", 0, "body text size in points; 0 follows the system")
	scale := flag.Float64("scale", 0, "interface scale: 1, 1.25, 1.5, 1.75 or 2; 0 follows the display")
	perfFlags := registerPerfFlags()
	flag.Parse()
	uiScale, err := parseScale(*scale)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	var perf *perfRun
	if perfFlags.active() {
		if *perfFlags.runs > 1 {
			os.Exit(runRepeatedSelf(*perfFlags.runs, os.Args[1:], *perfFlags.jsonPath))
		}
		perf = newPerfRun(perfFlags.options(perfSize(*width, defaultWidth), perfSize(*height, defaultHeight), uiScale))
	}
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
	perf.phase("settings loaded")

	plat := platform.New()
	var primary bool
	if *screenshot == "" && perf == nil {
		var ierr error
		primary, ierr = plat.Instance.Acquire()
		if ierr == nil && !primary && !*newWindow && st.SingleWindow {
			if err := plat.Instance.Send(app.EncodeHandoff(activationToken, paths)); err == nil {
				return
			}
		}
	}

	ws, _ := settings.LoadWindowState()
	var screens []screen
	if *screenshot == "" && perf == nil {
		screens = screensOf(window.Monitors())
	}
	plan := planWindow(st.RememberWindow && perf == nil, ws, screens)
	w, h := plan.W, plan.H
	if *width > 0 {
		w = *width
	}
	if *height > 0 {
		h = *height
	}
	bodyPt := plat.Fonts.BodyPt()
	if *fontSize > 0 {
		bodyPt = *fontSize
	}
	fonts.SetTextScale(fonts.ScaleForBody(bodyPt))
	widget.SetDefaultFontSize(bodyPt)
	eng := engine.New(max(minWidth, w), max(minHeight, h), 60)
	if err := fonts.Register(eng); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if uiScale > 0 {
		if *screenshot != "" || perf != nil {
			eng.SetScale(uiScale)
		} else {
			os.Setenv(scaleEnv, strconv.FormatFloat(uiScale, 'g', -1, 64))
		}
	}

	reg := codec.NewRegistry()
	codecall.Register(reg)
	for _, id := range settings.OptionalFormats {
		reg.SetEnabled(codec.ID(id), st.FormatEnabled(id))
	}

	perf.phase("engine ready")
	a := app.New(eng, app.Options{Settings: store, Platform: plat, Registry: reg})
	tuneGC()
	a.Root.ForceEnabled = *mockup
	if perf != nil {
		eng.SetRootFullCanvas(a.Root)
		os.Exit(perf.execute(eng, a))
	}

	if *screenshot != "" {
		eng.SetRootFullCanvas(a.Root)
		if len(paths) > 0 {
			a.Ctl.Open(paths[0])
		}
		if *galleryDir != "" {
			if abs, err := filepath.Abs(*galleryDir); err == nil {
				a.OpenGalleryAt(model.Folder(abs), "")
			}
		}
		waitForImage(eng, a)
		waitForGallery(eng, a)
		if *openSettings {
			a.OpenSettings()
		}
		a.Root.Refresh()
		eng.Flush()
		if err := savePNG(*screenshot, eng.RenderOnce()); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		a.Shutdown()
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
		a.Shutdown()
		win.Close()
	}
	win.SetOnCloseRequest(func() bool { a.Close(); return false })
	win.SetOnFilesDropped(func(p []string, x, y int) { eng.Post(func() { a.OpenFiles(p) }) })
	stopTheme := plat.Theme.Watch(a.SystemThemeChanged)
	defer stopTheme()
	if primary {
		plat.Instance.Listen(func(args []string) { eng.Post(func() { a.Handoff(args) }) })
		defer plat.Instance.Close()
	}
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sig
		eng.Post(a.Close)
	}()
	eng.Post(func() {
		opened := false
		switch {
		case len(paths) > 0:
			a.OpenFiles(paths)
			opened = true
		case lastFileToRestore(st.RestoreLastImage, ws, fileExists) != "":
			a.OpenFiles([]string{ws.LastFile})
			opened = true
		}
		switch {
		case *galleryDir != "":
			if abs, err := filepath.Abs(*galleryDir); err == nil {
				a.OpenGalleryAt(model.Folder(abs), "")
			}
		case !opened:
			a.OpenGallery()
		}
		if *openSettings {
			a.OpenSettings()
		}
		a.Root.Refresh()
		a.Root.Wake()
	})
	eng.After(updateCheckDelay, a.ScheduleUpdateCheck)
	eng.After(100*time.Millisecond, func() { hookActivation(eng, win, a, 50) })
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
		Raise: func(token string) {
			if !win.ActivateWithToken(token) {
				slog.Debug("window was not brought to the front")
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
	st := a.Settings.Get()
	old, _ := settings.LoadWindowState()
	ws := nextWindowState(old, snapshotWindow(eng, win), st.RememberWindow, st.RestoreLastImage, a.Ctl.Path())
	if ws == old {
		return
	}
	if err := ws.Save(); err != nil {
		slog.Warn("window state", "err", err)
	}
}

func snapshotWindow(eng *engine.Engine, win *window.Window) windowSnapshot {
	n := win.Native()
	if n == nil || win.IsFullscreen() {
		return windowSnapshot{}
	}
	s := windowSnapshot{Maximized: n.IsMaximized(), Valid: true}
	if !s.Maximized {
		w, h := n.GetSize()
		s.W, s.H = logicalSize(w, h, eng.Scale())
		s.X, s.Y = n.GetPosition()
	}
	return s
}

func screensOf(list []window.Monitor) []screen {
	out := make([]screen, 0, len(list))
	for _, m := range list {
		out = append(out, screen{Bounds: m.Bounds, Work: m.Work, Scale: m.Scale})
	}
	return out
}

func waitForImage(eng *engine.Engine, a *app.App) {
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		eng.Flush()
		if s := a.Ctl.Status(); s != app.StatusLoading && !a.Ctl.Scanning() && !a.GalleryBusy() {
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

func waitForGallery(eng *engine.Engine, a *app.App) {
	if !a.GalleryOpen() {
		return
	}
	deadline := time.Now().Add(15 * time.Second)
	idle := 0
	for time.Now().Before(deadline) && idle < 3 {
		eng.Flush()
		eng.RenderOnce()
		if a.GalleryBusy() {
			idle = 0
		} else {
			idle++
		}
		time.Sleep(10 * time.Millisecond)
	}
	eng.Flush()
}

type activationHook interface {
	SetOnActivate(fn func(active bool))
}

func hookActivation(eng *engine.Engine, win *window.Window, a *app.App, attempts int) {
	if n, ok := win.Native().(activationHook); ok {
		n.SetOnActivate(func(active bool) { eng.Post(func() { a.SetActive(active) }) })
		return
	}
	if attempts > 0 {
		eng.After(100*time.Millisecond, func() { hookActivation(eng, win, a, attempts-1) })
	}
}
