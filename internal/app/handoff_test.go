package app

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/platform"

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
		if a.Ctl.Name() == want && a.Ctl.Status() == StatusReady && !a.Ctl.Scanning() {
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
	a.SetOps(window.WindowOps{Raise: func(string) { raised++ }})

	a.Handoff([]string{first})
	waitName(t, a, "a.jpg")

	if raised != 1 {
		t.Fatalf("window raised %d times, want once", raised)
	}
}

func TestHandoffWithoutFilesStillBringsTheWindowForward(t *testing.T) {
	a, _ := goldenApp(t, 770, 504, nil)
	raised := 0
	a.SetOps(window.WindowOps{Raise: func(string) { raised++ }})

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

func TestHandoffPassesTheActivationTokenToTheWindow(t *testing.T) {
	a, _ := goldenApp(t, 770, 504, nil)
	first, _ := twoImages(t)
	var tokens []string
	a.SetOps(window.WindowOps{Raise: func(token string) { tokens = append(tokens, token) }})

	a.Handoff(EncodeHandoff("wayland-token-1", []string{first}))
	waitName(t, a, "a.jpg")
	a.Handoff([]string{first})

	if len(tokens) != 2 || tokens[0] != "wayland-token-1" || tokens[1] != "" {
		t.Fatalf("tokens = %q", tokens)
	}
}

func TestHandoffEncodingRoundTripsTokenAndPaths(t *testing.T) {
	cases := []struct {
		token string
		paths []string
	}{
		{"", []string{"/a.jpg", "/b.jpg"}},
		{"tok", []string{"/a.jpg"}},
		{"tok", nil},
		{"", nil},
		{"x_TIME123", []string{`C:\pics\a b.jpg`}},
	}
	for _, c := range cases {
		args := EncodeHandoff(c.token, c.paths)
		token, paths := DecodeHandoff(args)
		if token != c.token || len(paths) != len(c.paths) {
			t.Errorf("%v -> %q %q", args, token, paths)
			continue
		}
		for i := range paths {
			if paths[i] != c.paths[i] {
				t.Errorf("path %d: %q != %q", i, paths[i], c.paths[i])
			}
		}
	}
}

func TestHandoffWithoutATokenSendsPlainPaths(t *testing.T) {
	args := EncodeHandoff("", []string{"/a.jpg"})

	if len(args) != 1 || args[0] != "/a.jpg" {
		t.Fatalf("args = %q", args)
	}
}

func TestAPathNeverLooksLikeAnActivationToken(t *testing.T) {
	token, paths := DecodeHandoff([]string{"/home/user/activation:x.jpg", "activation:y"})

	if token != "" || len(paths) != 2 {
		t.Fatalf("token %q paths %q", token, paths)
	}
}

func TestTokenTravelsThroughTheRealSingleInstanceChannel(t *testing.T) {
	name := fmt.Sprintf("pixgo-handoff-%d", time.Now().UnixNano())
	first := platform.NewInstance(name)
	primary, err := first.Acquire()
	if err != nil || !primary {
		t.Skipf("no single-instance channel here: primary %v, err %v", primary, err)
	}
	t.Cleanup(func() { first.Close() })
	got := make(chan []string, 1)
	if err := first.Listen(func(args []string) { got <- args }); err != nil {
		t.Fatal(err)
	}
	second := platform.NewInstance(name)
	if isPrimary, err := second.Acquire(); err != nil || isPrimary {
		t.Skipf("second instance became primary: %v, err %v", isPrimary, err)
	}

	if err := second.Send(EncodeHandoff("xdg-token", []string{"/pics/a.jpg"})); err != nil {
		t.Fatal(err)
	}

	select {
	case args := <-got:
		token, paths := DecodeHandoff(args)
		if token != "xdg-token" || len(paths) != 1 || paths[0] != "/pics/a.jpg" {
			t.Fatalf("received %q: token %q paths %q", args, token, paths)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the primary never received the message")
	}
}
