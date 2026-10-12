package app

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/settings"
	"github.com/oops1/PixGo/internal/ui/window"
)

func twoImages(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	first, second := filepath.Join(dir, "a.jpg"), filepath.Join(dir, "b.jpg")
	writeBigJPEG(t, first)
	writeBigJPEG(t, second)
	return first, second
}

func waitName(t *testing.T, a *App, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		a.Eng.Flush()
		if a.Ctl.Name() == want && a.Ctl.Status() == StatusReady {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("current image is %q (status %d), want %q", a.Ctl.Name(), a.Ctl.Status(), want)
}

func TestHandoffOpensTheFileAndBringsTheWindowForward(t *testing.T) {
	a, _ := goldenApp(t, 770, 504, nil)
	first, _ := twoImages(t)
	raised := 0
	a.SetOps(window.WindowOps{Raise: func() { raised++ }})

	a.Handoff([]string{first})
	waitName(t, a, "a.jpg")

	if raised != 1 {
		t.Fatalf("window raised %d times, want once", raised)
	}
}

func TestHandoffWithoutFilesStillBringsTheWindowForward(t *testing.T) {
	a, _ := goldenApp(t, 770, 504, nil)
	raised := 0
	a.SetOps(window.WindowOps{Raise: func() { raised++ }})

	a.Handoff(nil)

	if raised != 1 || a.Ctl.Path() != "" {
		t.Fatalf("raised %d times, path %q", raised, a.Ctl.Path())
	}
}

func TestHandoffWorksWhenTheWindowCannotBeRaised(t *testing.T) {
	a, _ := goldenApp(t, 770, 504, nil)
	first, _ := twoImages(t)
	a.SetOps(window.WindowOps{})

	a.Handoff([]string{first})

	waitName(t, a, "a.jpg")
}

func TestWheelPagesThroughTheFolderByDefault(t *testing.T) {
	a, _ := goldenApp(t, 770, 504, nil)
	first, _ := twoImages(t)
	a.OpenFiles([]string{first})
	waitName(t, a, "a.jpg")

	a.Canvas.OnMouseButton(widget.MouseEvent{X: 300, Y: 200, Button: widget.MouseWheelDown})
	waitName(t, a, "b.jpg")

	a.Canvas.OnMouseButton(widget.MouseEvent{X: 300, Y: 200, Button: widget.MouseWheelUp})
	waitName(t, a, "a.jpg")
}

func TestControlWheelZoomsInsteadOfPaging(t *testing.T) {
	a, _ := goldenApp(t, 770, 504, nil)
	first, _ := twoImages(t)
	a.OpenFiles([]string{first})
	waitName(t, a, "a.jpg")
	before := zoomNow(a)

	a.Canvas.OnMouseButton(widget.MouseEvent{X: 300, Y: 200, Button: widget.MouseWheelUp, Mod: widget.ModCtrl})
	a.Eng.Flush()

	if a.Ctl.Name() != "a.jpg" {
		t.Fatalf("ctrl+wheel paged to %q", a.Ctl.Name())
	}
	if zoomNow(a) <= before {
		t.Fatalf("ctrl+wheel did not zoom in: %d%% -> %d%%", before, zoomNow(a))
	}
}

func TestWheelStillZoomsWhenTheSettingAsksForIt(t *testing.T) {
	a, _ := goldenApp(t, 770, 504, func(s *settings.Settings) { s.Wheel = settings.WheelZoom })
	first, _ := twoImages(t)
	a.OpenFiles([]string{first})
	waitName(t, a, "a.jpg")
	before := zoomNow(a)

	a.Canvas.OnMouseButton(widget.MouseEvent{X: 300, Y: 200, Button: widget.MouseWheelUp})
	a.Eng.Flush()

	if a.Ctl.Name() != "a.jpg" || zoomNow(a) <= before {
		t.Fatalf("name %q, zoom %d%% -> %d%%", a.Ctl.Name(), before, zoomNow(a))
	}
}

func zoomNow(a *App) int {
	a.Eng.Flush()
	a.Eng.RenderOnce()
	return a.Canvas.ZoomPercent()
}
