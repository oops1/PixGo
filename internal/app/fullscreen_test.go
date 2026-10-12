package app

import (
	"image/color"
	"path/filepath"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/commands"
	"github.com/oops1/PixGo/internal/settings"
	"github.com/oops1/PixGo/internal/ui/window"
)

type fakeWindow struct {
	maximized bool
	fsAccept  bool
	fsCalls   []bool
	maxCalls  []bool
}

func (f *fakeWindow) ops() window.WindowOps {
	return window.WindowOps{
		Maximized: func() bool { return f.maximized },
		Maximize:  func(on bool) { f.maximized = on; f.maxCalls = append(f.maxCalls, on) },
		SetFullscreen: func(on bool) bool {
			f.fsCalls = append(f.fsCalls, on)
			return f.fsAccept
		},
	}
}

func fsApp(t *testing.T, w *fakeWindow) *App {
	t.Helper()
	a, _ := goldenApp(t, 770, 504, func(s *settings.Settings) { s.AutoHidePanels = false })
	a.SetOps(w.ops())
	return a
}

func TestFullscreenAcceptedByWindow(t *testing.T) {
	w := &fakeWindow{fsAccept: true}
	a := fsApp(t, w)
	a.SetFullscreen(true)
	if a.Fullscreen() {
		t.Fatal("fullscreen flag set before the window confirmed it")
	}
	a.OnFullscreenChanged(true)
	if !a.Fullscreen() || a.emulated {
		t.Fatalf("fullscreen=%v emulated=%v", a.Fullscreen(), a.emulated)
	}
	a.SetFullscreen(false)
	a.OnFullscreenChanged(false)
	if a.Fullscreen() {
		t.Error("still fullscreen after leaving")
	}
	if len(w.maxCalls) != 0 {
		t.Errorf("a real fullscreen must not touch maximize: %v", w.maxCalls)
	}
}

func TestFullscreenRefusedFallsBackToMaximize(t *testing.T) {
	w := &fakeWindow{fsAccept: true}
	a := fsApp(t, w)
	a.SetFullscreen(true)
	a.OnFullscreenChanged(false)
	if !a.Fullscreen() || !a.emulated {
		t.Fatalf("compositor refusal did not fall back: fullscreen=%v emulated=%v", a.Fullscreen(), a.emulated)
	}
	if !w.maximized {
		t.Error("fallback did not maximise the window")
	}
	a.escape()
	if a.Fullscreen() || w.maximized {
		t.Errorf("leaving the fallback: fullscreen=%v maximized=%v", a.Fullscreen(), w.maximized)
	}
}

func TestFullscreenUnsupportedBackendFallsBack(t *testing.T) {
	w := &fakeWindow{fsAccept: false}
	a := fsApp(t, w)
	a.SetFullscreen(true)
	if !a.Fullscreen() || !a.emulated || !w.maximized {
		t.Fatalf("fullscreen=%v emulated=%v maximized=%v", a.Fullscreen(), a.emulated, w.maximized)
	}
}

func TestFallbackKeepsPreviouslyMaximizedWindow(t *testing.T) {
	w := &fakeWindow{fsAccept: false, maximized: true}
	a := fsApp(t, w)
	a.SetFullscreen(true)
	a.SetFullscreen(false)
	if !w.maximized {
		t.Error("a window that was maximised before must stay maximised")
	}
}

func TestFullscreenChromeHidesTitle(t *testing.T) {
	w := &fakeWindow{fsAccept: true}
	a := fsApp(t, w)
	a.OnFullscreenChanged(true)
	l := a.Root.Layout()
	if !l.Title.Empty() {
		t.Errorf("title bar present in fullscreen: %v", l.Title)
	}
}

func TestEscapeOutsideFullscreenDoesNothing(t *testing.T) {
	w := &fakeWindow{fsAccept: true}
	a := fsApp(t, w)
	a.escape()
	if len(w.fsCalls) != 0 {
		t.Errorf("escape touched the window: %v", w.fsCalls)
	}
}

func TestF11AndEscapeThroughTheKeyboard(t *testing.T) {
	w := &fakeWindow{fsAccept: true}
	a := fsApp(t, w)
	p := filepath.Join(t.TempDir(), "x.png")
	writePNG(t, p, 30, 20, color.NRGBA{1, 2, 3, 255})
	if err := a.Ctl.Open(p); err != nil {
		t.Fatal(err)
	}
	waitReady(t, a)
	a.Root.HandleKey(widget.KeyEvent{Code: widget.KeyF11, Pressed: true})
	if len(w.fsCalls) != 1 || !w.fsCalls[0] {
		t.Fatalf("F11 did not request fullscreen: %v", w.fsCalls)
	}
	a.OnFullscreenChanged(true)
	a.Root.HandleKey(widget.KeyEvent{Code: widget.KeyEscape, Pressed: true})
	if len(w.fsCalls) != 2 || w.fsCalls[1] {
		t.Fatalf("Esc did not leave fullscreen: %v", w.fsCalls)
	}
	a.OnFullscreenChanged(false)
	w.fsCalls = nil
	a.Root.HandleKey(widget.KeyEvent{Code: widget.KeyEnter, Mod: widget.ModAlt, Pressed: true})
	if len(w.fsCalls) != 1 || !w.fsCalls[0] {
		t.Fatalf("Alt+Enter did not request fullscreen: %v", w.fsCalls)
	}
}

func TestEveryImplementedSpecIsEnabledWhenItShould(t *testing.T) {
	a, _ := goldenApp(t, 770, 504, nil)
	p := filepath.Join(t.TempDir(), "x.png")
	writePNG(t, p, 30, 20, color.NRGBA{1, 2, 3, 255})
	a.Ctl.Open(p)
	waitReady(t, a)
	for _, id := range []commands.ID{CmdZoomIn, CmdZoomOut, CmdZoomActual, CmdZoomFit, CmdZoomWidth, CmdRotateLeft, CmdRotateRight, CmdFlipH, CmdFlipV,
		CmdFavorite, CmdFullscreen, CmdDelete, CmdCopy, CmdCopyPath, CmdCopyImage, CmdReveal, CmdSaveAs, CmdOpen, CmdGallery} {
		if c := a.Commands.Get(id); c == nil || !c.IsEnabled() {
			t.Errorf("command %s is not enabled with an image open", id)
		}
	}
}
