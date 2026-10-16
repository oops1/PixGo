package app

import (
	"testing"

	"github.com/oops1/headless-gui/v3/widget"

	uiwin "github.com/oops1/PixGo/internal/ui/window"
	"github.com/oops1/PixGo/internal/viewer"
)

type typist struct {
	t  *testing.T
	a  *App
	kr *uiwin.KeyRouter
}

func newTypist(t *testing.T, a *App) *typist {
	return &typist{t: t, a: a, kr: &uiwin.KeyRouter{Engine: a.Eng, Target: a.Root}}
}

func (k *typist) press(code widget.KeyCode, mod widget.KeyMod) {
	k.kr.SendKeyEvent(widget.KeyEvent{Code: code, Mod: mod, Pressed: true})
	k.a.Eng.Flush()
}

func (k *typist) tab(n int) {
	for i := 0; i < n; i++ {
		k.press(widget.KeyTab, 0)
	}
}

func (k *typist) focus() uiwin.ZoneKind {
	z, _ := k.a.Root.KeyboardFocus()
	return z
}

func TestEscapeClosesAConfirmationDialog(t *testing.T) {
	a := viewerWithImage(t)
	k := newTypist(t, a)
	a.confirm("Title", "Message", func() {})
	a.Eng.Flush()
	if !a.ModalsOpen() {
		t.Fatal("the dialog is not tracked as open")
	}
	k.press(widget.KeyEscape, 0)
	if a.ModalsOpen() {
		t.Error("escape did not close the dialog")
	}
}

func TestEnterOnTheDialogDefaultButtonConfirms(t *testing.T) {
	a := viewerWithImage(t)
	k := newTypist(t, a)
	confirmed := false
	a.confirm("Title", "Message", func() { confirmed = true })
	a.Eng.Flush()
	k.press(widget.KeyEnter, 0)
	if !confirmed || a.ModalsOpen() {
		t.Errorf("confirmed=%v open=%v", confirmed, a.ModalsOpen())
	}
}

func TestChordsDoNotActBehindADialog(t *testing.T) {
	r := newGalleryRig(t, nil)
	r.app.OpenFiles([]string{r.b})
	waitName(t, r.app, "b.jpg")
	k := newTypist(t, r.app)
	r.app.confirm("Title", "Message", func() {})
	r.app.Eng.Flush()
	k.press(widget.KeyLeft, 0)
	k.press(widget.KeyF11, 0)
	if r.app.Ctl.Name() != "b.jpg" || r.app.fullscreen {
		t.Errorf("a key acted behind the dialog: file %q fullscreen %v", r.app.Ctl.Name(), r.app.fullscreen)
	}
	k.press(widget.KeyEscape, 0)
	k.press(widget.KeyLeft, 0)
	waitName(t, r.app, "a.jpg")
}

func TestTabAndEnterRunAToolbarCommand(t *testing.T) {
	r := newGalleryRig(t, nil)
	r.app.OpenFiles([]string{r.b})
	waitName(t, r.app, "b.jpg")
	r.app.Root.ForceEnabled = false
	k := newTypist(t, r.app)
	k.tab(1)
	if k.focus() != uiwin.ZoneToolGallery {
		t.Fatalf("first tab stop after the image is %d, want the gallery button", k.focus())
	}
	k.press(widget.KeyEnter, 0)
	if !r.app.GalleryOpen() {
		t.Error("enter on the gallery button did not open the gallery")
	}
}

func TestInfoButtonOpensThePanelAndEscapeClosesIt(t *testing.T) {
	a := viewerWithImage(t)
	k := newTypist(t, a)
	for i := 0; i < 30 && k.focus() != uiwin.ZoneInfo; i++ {
		k.tab(1)
	}
	if k.focus() != uiwin.ZoneInfo {
		t.Fatalf("the info button is not reachable with tab, stopped on %d", k.focus())
	}
	k.press(widget.KeyEnter, 0)
	if !a.InfoOpen() || k.focus() != uiwin.ZonePanel {
		t.Fatalf("info open %v, focus %d", a.InfoOpen(), k.focus())
	}
	k.press(widget.KeyEscape, 0)
	if a.InfoOpen() {
		t.Error("escape did not close the panel")
	}
	if k.focus() != uiwin.ZoneInfo {
		t.Errorf("focus %d after closing, want the info button", k.focus())
	}
}

func TestShiftF10OpensTheMenuAndTheArrowsDriveIt(t *testing.T) {
	r := newGalleryRig(t, nil)
	r.app.OpenFiles([]string{r.b})
	waitName(t, r.app, "b.jpg")
	k := newTypist(t, r.app)
	k.press(widget.KeyF10, widget.ModShift)
	if !r.app.menu.IsOpen() {
		t.Fatal("shift+F10 did not open the menu")
	}
	k.press(widget.KeyDown, 0)
	k.press(widget.KeyEnter, 0)
	if r.app.menu.IsOpen() {
		t.Error("enter did not close the menu")
	}
	if !r.app.GalleryOpen() {
		t.Error("the first menu item (Open) did not run")
	}
}

func TestEscapeClosesTheMenuAndFocusReturnsToTheImage(t *testing.T) {
	a := viewerWithImage(t)
	k := newTypist(t, a)
	k.press(widget.KeyMenu, 0)
	if !a.menu.IsOpen() {
		t.Fatal("menu key did not open the menu")
	}
	k.press(widget.KeyEscape, 0)
	if a.menu.IsOpen() {
		t.Fatal("escape did not close the menu")
	}
	k.press(widget.KeyHome, 0)
	if a.Ctl.Name() == "" {
		t.Error("the image is gone")
	}
}

func TestTabIsSwallowedWhileTheMenuIsOpen(t *testing.T) {
	a := viewerWithImage(t)
	k := newTypist(t, a)
	k.press(widget.KeyMenu, 0)
	k.tab(3)
	if !a.menu.IsOpen() {
		t.Error("tab closed the menu")
	}
	if z, kbd := a.Root.KeyboardFocus(); kbd || z != uiwin.ZoneNone {
		t.Errorf("tab moved the window focus to %d while the menu was open", z)
	}
}

func TestControlArrowsPanAZoomedImage(t *testing.T) {
	a := viewerWithImage(t)
	a.Canvas.Smooth = false
	a.Canvas.SetMode(viewer.ZoomActual)
	for i := 0; i < 4; i++ {
		a.Canvas.ZoomIn()
	}
	a.Eng.Flush()
	a.Eng.RenderOnce()
	if !a.Canvas.CanPan() {
		t.Fatal("the image cannot be panned")
	}
	k := newTypist(t, a)
	k.press(widget.KeyRight, widget.ModCtrl)
	before := a.Canvas.View().CenterU
	k.press(widget.KeyRight, widget.ModCtrl)
	if a.Canvas.View().CenterU <= before {
		t.Errorf("ctrl+right did not pan: %v -> %v", before, a.Canvas.View().CenterU)
	}
	before = a.Canvas.View().CenterV
	k.press(widget.KeyDown, widget.ModCtrl)
	if a.Canvas.View().CenterV <= before {
		t.Errorf("ctrl+down did not pan: %v -> %v", before, a.Canvas.View().CenterV)
	}
}

func TestPanKeysAreDisabledWhenTheImageFits(t *testing.T) {
	a := viewerWithImage(t)
	if cmd := a.Commands.Get(CmdPanLeft); cmd == nil || cmd.IsEnabled() {
		t.Errorf("pan command %+v", cmd)
	}
}

func TestEscapeLeavesTheGalleryFromAnyControl(t *testing.T) {
	r := newGalleryRig(t, nil)
	r.open(t)
	k := newTypist(t, r.app)
	k.tab(3)
	k.press(widget.KeyEscape, 0)
	if r.app.GalleryOpen() {
		t.Error("escape did not close the gallery")
	}
}

func TestEscapeInSettingsGoesBack(t *testing.T) {
	a, _ := goldenApp(t, 770, 504, nil)
	a.OpenSettings()
	k := newTypist(t, a)
	k.tab(2)
	k.press(widget.KeyEscape, 0)
	if a.SettingsOpen() {
		t.Error("escape did not leave the settings")
	}
}

func TestMouseClickHidesTheRingOnTheWholeWindow(t *testing.T) {
	a := viewerWithImage(t)
	k := newTypist(t, a)
	k.tab(1)
	if _, kbd := a.Root.KeyboardFocus(); !kbd {
		t.Fatal("tab did not start keyboard navigation")
	}
	k.kr.SendMouseButton(300, 200, widget.MouseLeft, true)
	k.kr.SendMouseButton(300, 200, widget.MouseLeft, false)
	if _, kbd := a.Root.KeyboardFocus(); kbd {
		t.Error("the ring stayed after a click on the image")
	}
}

func TestEscapeClosesTheInfoPanelWhenTheImageHasFocus(t *testing.T) {
	a := viewerWithImage(t)
	k := newTypist(t, a)
	a.toggleInfo()
	if !a.InfoOpen() {
		t.Fatal("the panel did not open")
	}
	k.press(widget.KeyEscape, 0)
	if a.InfoOpen() {
		t.Error("escape did not close the panel")
	}
	k.press(widget.KeyEscape, 0)
	if a.fullscreen {
		t.Error("a second escape changed the window state")
	}
}

func TestEscapeLeavesFullscreenBeforeAnythingElse(t *testing.T) {
	a := viewerWithImage(t)
	k := newTypist(t, a)
	a.toggleInfo()
	a.SetFullscreen(true)
	k.press(widget.KeyEscape, 0)
	if a.fullscreen {
		t.Error("escape did not leave the full screen")
	}
	if !a.InfoOpen() {
		t.Error("the same escape also closed the panel")
	}
}

func TestTypedPlusAndNumpadDigitsDriveTheZoom(t *testing.T) {
	a := viewerWithImage(t)
	k := newTypist(t, a)

	k.press(widget.KeyOemPlus, widget.ModShift)
	if a.Canvas.Mode() != viewer.ZoomCustom {
		t.Fatalf("Shift+= left the mode at %v", a.Canvas.Mode())
	}
	k.press(widget.KeyNumpad1, 0)
	if a.Canvas.Mode() != viewer.ZoomFit {
		t.Fatalf("numpad 1 gave mode %v", a.Canvas.Mode())
	}
	k.press(widget.KeyNumpad2, 0)
	if a.Canvas.Mode() != viewer.ZoomFitWidth {
		t.Fatalf("numpad 2 gave mode %v", a.Canvas.Mode())
	}
	k.press(widget.KeyNumpad0, 0)
	if a.Canvas.Mode() != viewer.ZoomActual {
		t.Fatalf("numpad 0 gave mode %v", a.Canvas.Mode())
	}
	before := a.Canvas.ZoomPercent()
	k.press(widget.KeyOemPlus, widget.ModCtrl|widget.ModShift)
	if a.Canvas.ZoomPercent() <= before {
		t.Fatalf("Ctrl+Shift+= did not zoom in: %d -> %d", before, a.Canvas.ZoomPercent())
	}
	k.press(widget.KeySubtract, 0)
	k.press(widget.KeyAdd, 0)
}
