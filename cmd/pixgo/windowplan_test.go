package main

import (
	"image"
	"path/filepath"
	"testing"

	"github.com/oops1/PixGo/internal/settings"
)

func oneScreen() []screen {
	return []screen{{Bounds: image.Rect(0, 0, 1920, 1080), Work: image.Rect(0, 0, 1920, 1040), Scale: 1}}
}

func twoScreens() []screen {
	return []screen{
		{Bounds: image.Rect(0, 0, 1920, 1080), Work: image.Rect(0, 0, 1920, 1040), Scale: 1},
		{Bounds: image.Rect(-1280, 100, 0, 1124), Work: image.Rect(-1280, 100, 0, 1124), Scale: 1},
	}
}

func sideBySide() []screen {
	return []screen{
		{Bounds: image.Rect(0, 0, 1920, 1080), Work: image.Rect(0, 0, 1920, 1032), Scale: 1},
		{Bounds: image.Rect(1920, 0, 3840, 1080), Work: image.Rect(1920, 0, 3840, 1032), Scale: 1},
	}
}

func hiDPI() []screen {
	return []screen{{Bounds: image.Rect(0, 0, 3840, 2160), Work: image.Rect(0, 0, 3840, 2120), Scale: 2}}
}

func TestPlaceWindow(t *testing.T) {
	cases := []struct {
		name    string
		saved   settings.WindowState
		screens []screen
		x, y    int
		w, h    int
		ok      bool
	}{
		{"fits on one monitor", settings.WindowState{X: 200, Y: 120, W: 1100, H: 700}, oneScreen(), 200, 120, 1100, 700, true},
		{"nothing saved", settings.WindowState{}, oneScreen(), 0, 0, 0, 0, false},
		{"monitors unknown keeps the size only", settings.WindowState{X: 200, Y: 120, W: 1100, H: 700}, nil, 0, 0, 1100, 700, false},
		{"partly off the right edge but 300 px visible", settings.WindowState{X: 1620, Y: 100, W: 1100, H: 700}, oneScreen(), 1620, 100, 1100, 700, true},
		{"only 100 px visible on the right", settings.WindowState{X: 1820, Y: 100, W: 1100, H: 700}, oneScreen(), 0, 0, 1100, 700, false},
		{"exactly 120 px visible", settings.WindowState{X: 1800, Y: 100, W: 1100, H: 700}, oneScreen(), 1800, 100, 1100, 700, true},
		{"only 70 px visible at the bottom", settings.WindowState{X: 200, Y: 1010, W: 1100, H: 700}, oneScreen(), 0, 0, 1100, 700, false},
		{"80 px visible at the bottom", settings.WindowState{X: 200, Y: 1000, W: 1100, H: 700}, oneScreen(), 200, 1000, 1100, 700, true},
		{"title bar above the top edge is pulled down", settings.WindowState{X: 200, Y: -300, W: 1100, H: 700}, oneScreen(), 200, 0, 1100, 700, true},
		{"entirely off the top", settings.WindowState{X: 200, Y: -900, W: 1100, H: 700}, oneScreen(), 0, 0, 1100, 700, false},
		{"huge size is clamped to the work area", settings.WindowState{X: 100, Y: 100, W: 5000, H: 4000}, oneScreen(), 0, 0, 1920, 1040, true},
		{"huge width only", settings.WindowState{X: 100, Y: 100, W: 5000, H: 600}, oneScreen(), 0, 100, 1920, 600, true},
		{"second monitor on the left with negative x", settings.WindowState{X: -1200, Y: 200, W: 1000, H: 700}, twoScreens(), -1200, 200, 1000, 700, true},
		{"left monitor is clamped to its own size", settings.WindowState{X: -1250, Y: 150, W: 2000, H: 700}, twoScreens(), -1280, 150, 1280, 700, true},
		{"top of the left monitor is its own top edge", settings.WindowState{X: -1200, Y: 20, W: 1000, H: 700}, twoScreens(), -1200, 100, 1000, 700, true},
		{"left monitor removed", settings.WindowState{X: -1200, Y: 200, W: 1000, H: 700}, oneScreen(), 0, 0, 1000, 700, false},
		{"left monitor removed and the window was wider than the rest", settings.WindowState{X: -2500, Y: 200, W: 2500, H: 1500}, oneScreen(), 0, 0, 1920, 1040, false},
		{"straddling two monitors keeps the saved place", settings.WindowState{X: -300, Y: 200, W: 1000, H: 700}, twoScreens(), -300, 200, 1000, 700, true},
		{"high dpi monitor scales the size for the check", settings.WindowState{X: 3000, Y: 100, W: 1100, H: 700}, hiDPI(), 3000, 100, 1100, 700, true},
		{"high dpi clamp is in logical pixels", settings.WindowState{X: 0, Y: 0, W: 3000, H: 2000}, hiDPI(), 0, 0, 1920, 1060, true},
		{"second monitor on the right", settings.WindowState{X: 2100, Y: 80, W: 1100, H: 700}, sideBySide(), 2100, 80, 1100, 700, true},
		{"second monitor on the right is clamped to its own work area", settings.WindowState{X: 2000, Y: 80, W: 2500, H: 1200}, sideBySide(), 1920, 0, 1920, 1032, true},
		{"right monitor removed", settings.WindowState{X: 2100, Y: 80, W: 1100, H: 700}, sideBySide()[:1], 0, 0, 1100, 700, false},
		{"no work area falls back to the bounds", settings.WindowState{X: 0, Y: 0, W: 3000, H: 2000}, []screen{{Bounds: image.Rect(0, 0, 1600, 900)}}, 0, 0, 1600, 900, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			x, y, w, h, ok := placeWindow(c.saved, c.screens)
			if x != c.x || y != c.y || w != c.w || h != c.h || ok != c.ok {
				t.Fatalf("got (%d, %d, %d, %d, %v), want (%d, %d, %d, %d, %v)", x, y, w, h, ok, c.x, c.y, c.w, c.h, c.ok)
			}
		})
	}
}

func TestPlaceWindowNeverLeavesTheTitleBarAboveItsMonitor(t *testing.T) {
	screens := twoScreens()
	for _, y := range []int{-5000, -600, -1, 0, 50, 99, 100, 500, 900} {
		for _, x := range []int{-1300, -600, 0, 800, 1700} {
			px, py, _, _, ok := placeWindow(settings.WindowState{X: x, Y: y, W: 900, H: 600}, screens)
			if !ok {
				continue
			}
			above := true
			for _, s := range screens {
				if py >= s.area().Min.Y && px < s.Bounds.Max.X && px+900 > s.Bounds.Min.X {
					above = false
				}
			}
			if above {
				t.Errorf("saved (%d, %d) was placed at (%d, %d), above every monitor it overlaps", x, y, px, py)
			}
		}
	}
}

func TestPlanWindowFollowsRememberWindow(t *testing.T) {
	ws := settings.WindowState{X: 300, Y: 200, W: 1000, H: 650}

	off := planWindow(false, ws, oneScreen())
	if off != (windowPlan{W: defaultWidth, H: defaultHeight}) {
		t.Fatalf("remember off: %+v", off)
	}
	on := planWindow(true, ws, oneScreen())
	if on != (windowPlan{W: 1000, H: 650, X: 300, Y: 200, HasPos: true}) {
		t.Fatalf("remember on: %+v", on)
	}
}

func TestPlanWindowRestoresMaximizedWithTheNormalRectangle(t *testing.T) {
	ws := settings.WindowState{X: 300, Y: 200, W: 1000, H: 650, Maximized: true}

	p := planWindow(true, ws, oneScreen())

	if !p.Maximize || !p.HasPos || p.W != 1000 || p.H != 650 || p.X != 300 || p.Y != 200 {
		t.Fatalf("%+v", p)
	}
}

func TestPlanWindowMaximizedWithoutARectangle(t *testing.T) {
	p := planWindow(true, settings.WindowState{Maximized: true}, oneScreen())

	if !p.Maximize || p.HasPos || p.W != defaultWidth || p.H != defaultHeight {
		t.Fatalf("%+v", p)
	}
	if q := planWindow(false, settings.WindowState{Maximized: true}, oneScreen()); q.Maximize {
		t.Fatalf("maximize must follow the setting: %+v", q)
	}
}

func TestPlanWindowLeavesThePositionToTheSystemWhenOffScreen(t *testing.T) {
	ws := settings.WindowState{X: -4000, Y: 100, W: 1000, H: 650}

	p := planWindow(true, ws, oneScreen())

	if p.HasPos || p.W != 1000 || p.H != 650 {
		t.Fatalf("%+v", p)
	}
}

func TestNextWindowStateKeepsLastFileWithoutRememberWindow(t *testing.T) {
	old := settings.WindowState{X: 10, Y: 20, W: 900, H: 600, LastFile: "old.jpg"}
	snap := windowSnapshot{X: 500, Y: 400, W: 1200, H: 800, Valid: true}

	got := nextWindowState(old, snap, false, true, "new.jpg")

	want := settings.WindowState{X: 10, Y: 20, W: 900, H: 600, LastFile: "new.jpg"}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestNextWindowStateKeepsTheRectangleWithoutRestoreLastImage(t *testing.T) {
	old := settings.WindowState{X: 10, Y: 20, W: 900, H: 600, LastFile: "old.jpg"}
	snap := windowSnapshot{X: 500, Y: 400, W: 1200, H: 800, Valid: true}

	got := nextWindowState(old, snap, true, false, "new.jpg")

	want := settings.WindowState{X: 500, Y: 400, W: 1200, H: 800}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestNextWindowStateBothOffForgetsTheFileOnly(t *testing.T) {
	old := settings.WindowState{X: 10, Y: 20, W: 900, H: 600, LastFile: "old.jpg"}

	got := nextWindowState(old, windowSnapshot{X: 1, Y: 2, W: 3, H: 4, Valid: true}, false, false, "new.jpg")

	if want := (settings.WindowState{X: 10, Y: 20, W: 900, H: 600}); got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestNextWindowStateMaximizedKeepsTheNormalRectangle(t *testing.T) {
	old := settings.WindowState{X: 10, Y: 20, W: 900, H: 600}
	snap := windowSnapshot{Maximized: true, Valid: true}

	got := nextWindowState(old, snap, true, true, "a.jpg")

	want := settings.WindowState{X: 10, Y: 20, W: 900, H: 600, Maximized: true, LastFile: "a.jpg"}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	back := nextWindowState(got, windowSnapshot{X: 30, Y: 40, W: 800, H: 500, Valid: true}, true, true, "a.jpg")
	if back.Maximized || back.X != 30 || back.W != 800 {
		t.Fatalf("restoring from maximized: %+v", back)
	}
}

func TestNextWindowStateWithoutAWindowStillRecordsTheFile(t *testing.T) {
	got := nextWindowState(settings.WindowState{W: 900, H: 600}, windowSnapshot{}, true, true, "a.jpg")

	if got.LastFile != "a.jpg" || got.W != 900 || got.Maximized {
		t.Fatalf("%+v", got)
	}
}

func TestNextWindowStateSurvivesASaveAndLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "window.json")
	ws := nextWindowState(settings.WindowState{}, windowSnapshot{X: -1200, Y: 150, W: 1000, H: 700, Valid: true}, true, true, "D:/a.jpg")
	if err := ws.SaveTo(path); err != nil {
		t.Fatal(err)
	}

	got, err := settings.LoadWindowStateFrom(path)

	if err != nil || got != ws {
		t.Fatalf("got %+v %v, want %+v", got, err, ws)
	}
}

func TestLastFileToRestoreFollowsRestoreLastImageOnly(t *testing.T) {
	ws := settings.WindowState{LastFile: "a.jpg"}
	there := func(string) bool { return true }
	gone := func(string) bool { return false }

	if got := lastFileToRestore(true, ws, there); got != "a.jpg" {
		t.Fatalf("got %q", got)
	}
	if got := lastFileToRestore(false, ws, there); got != "" {
		t.Fatalf("restore off: %q", got)
	}
	if got := lastFileToRestore(true, ws, gone); got != "" {
		t.Fatalf("the file is gone: %q", got)
	}
	if got := lastFileToRestore(true, settings.WindowState{}, there); got != "" {
		t.Fatalf("nothing saved: %q", got)
	}
}

func TestLastFileSurvivesWithRememberWindowOffEndToEnd(t *testing.T) {
	path := filepath.Join(t.TempDir(), "window.json")
	next := nextWindowState(settings.WindowState{}, windowSnapshot{X: 5, Y: 6, W: 900, H: 600, Valid: true}, false, true, "D:/photos/a.jpg")
	if err := next.SaveTo(path); err != nil {
		t.Fatal(err)
	}

	loaded, err := settings.LoadWindowStateFrom(path)

	if err != nil || loaded.HasBounds() || lastFileToRestore(true, loaded, func(string) bool { return true }) != "D:/photos/a.jpg" {
		t.Fatalf("%+v %v", loaded, err)
	}
}

type fakePlacer struct {
	bounds    []int
	maximized bool
}

func (f *fakePlacer) SetInitialBounds(x, y, w, h int) { f.bounds = []int{x, y, w, h} }

func (f *fakePlacer) SetInitialMaximized(v bool) { f.maximized = v }

func TestInitialPlacementIsHandedToTheWindowBeforeItIsShown(t *testing.T) {
	var f fakePlacer
	applyInitialPlacement(&f, windowPlan{W: 900, H: 600, X: -1500, Y: 120, HasPos: true, Maximize: true}, false, 900, 600)
	if len(f.bounds) != 4 || f.bounds[0] != -1500 || f.bounds[1] != 120 || f.bounds[2] != 900 || f.bounds[3] != 600 || !f.maximized {
		t.Fatalf("bounds %v maximized %v", f.bounds, f.maximized)
	}
	var none fakePlacer
	applyInitialPlacement(&none, windowPlan{W: 900, H: 600}, false, 900, 600)
	if none.bounds != nil || none.maximized {
		t.Fatalf("nothing saved, yet bounds %v maximized %v", none.bounds, none.maximized)
	}
	var forced fakePlacer
	applyInitialPlacement(&forced, windowPlan{W: 900, H: 600, X: 10, Y: 10, HasPos: true, Maximize: true}, true, 640, 480)
	if forced.bounds != nil || forced.maximized {
		t.Fatal("an explicit -width or -height must win over the saved placement")
	}
}
