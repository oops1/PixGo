package view3d

import (
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/mesh"
	"github.com/oops1/PixGo/internal/render3d"
)

type syncEngine struct{ invalidated int }

func (e *syncEngine) Invalidate() { e.invalidated++ }

func cube() *mesh.Mesh {
	c := [8][3]float32{{0, 0, 0}, {1, 0, 0}, {1, 1, 0}, {0, 1, 0}, {0, 0, 1}, {1, 0, 1}, {1, 1, 1}, {0, 1, 1}}
	faces := [12][3]int{{0, 2, 1}, {0, 3, 2}, {4, 5, 6}, {4, 6, 7}, {0, 1, 5}, {0, 5, 4}, {1, 2, 6}, {1, 6, 5}, {2, 3, 7}, {2, 7, 6}, {3, 0, 4}, {3, 4, 7}}
	m := &mesh.Mesh{}
	for _, f := range faces {
		for _, i := range f {
			m.V = append(m.V, c[i][0], c[i][1], c[i][2])
		}
	}
	m.UpdateBounds()
	return m
}

func newViewer(t *testing.T) (*Viewer, chan func()) {
	t.Helper()
	posted := make(chan func(), 64)
	v := New(&syncEngine{})
	v.SetPost(func(fn func()) { posted <- fn })
	v.Background = color.RGBA{32, 32, 32, 255}
	v.SetBounds(image.Rect(0, 0, 320, 200))
	v.areaW, v.areaH = 320, 200
	v.SetScene(render3d.NewScene(cube()))
	return v, posted
}

func waitFrame(t *testing.T, v *Viewer, posted chan func()) {
	t.Helper()
	deadline := time.After(10 * time.Second)
	for v.cur.img == nil {
		select {
		case fn := <-posted:
			fn()
		case <-deadline:
			t.Fatal("no frame rendered")
		}
	}
}

func TestFirstFrameIsRenderedAndLocked(t *testing.T) {
	v, posted := newViewer(t)
	waitFrame(t, v, posted)
	if v.cur.img.Bounds().Dx() != 320 || v.cur.img.Bounds().Dy() != 200 {
		t.Fatalf("frame %v", v.cur.img.Bounds())
	}
	if v.cam.Fit <= 0 {
		t.Fatal("the fit is locked so that turning keeps the scale")
	}
	if v.ZoomPercent() != 100 {
		t.Fatalf("zoom %d", v.ZoomPercent())
	}
}

func TestDragTurnsAndRightDragMoves(t *testing.T) {
	v, posted := newViewer(t)
	waitFrame(t, v, posted)
	yaw, pitch := v.cam.Yaw, v.cam.Pitch
	v.OnMouseButton(widget.MouseEvent{X: 100, Y: 100, Button: widget.MouseLeft, Pressed: true})
	v.OnMouseMove(160, 70)
	v.OnMouseButton(widget.MouseEvent{X: 160, Y: 70, Button: widget.MouseLeft})
	if v.cam.Yaw == yaw || v.cam.Pitch == pitch {
		t.Fatal("a left drag turns the model")
	}
	px, py := v.cam.PanX, v.cam.PanY
	v.OnMouseButton(widget.MouseEvent{X: 100, Y: 100, Button: widget.MouseRight, Pressed: true})
	v.OnMouseMove(140, 120)
	v.OnMouseButton(widget.MouseEvent{X: 140, Y: 120, Button: widget.MouseRight})
	if v.cam.PanX == px && v.cam.PanY == py {
		t.Fatal("a right drag moves the model")
	}
}

func TestRightClickWithoutDragOpensTheMenu(t *testing.T) {
	v, posted := newViewer(t)
	waitFrame(t, v, posted)
	opened := false
	v.OnContextMenu = func(x, y int) { opened = true }
	v.OnMouseButton(widget.MouseEvent{X: 50, Y: 50, Button: widget.MouseRight, Pressed: true})
	v.OnMouseButton(widget.MouseEvent{X: 50, Y: 50, Button: widget.MouseRight})
	if !opened {
		t.Fatal("a right click opens the menu")
	}
}

func TestWheelZoomsAndDoubleClickResets(t *testing.T) {
	v, posted := newViewer(t)
	waitFrame(t, v, posted)
	v.OnMouseWheelPixelsMod(160, 100, 0, -120, 0)
	if v.cam.Zoom <= 1 {
		t.Fatalf("wheel up zooms in, zoom %v", v.cam.Zoom)
	}
	v.OnMouseButton(widget.MouseEvent{X: 160, Y: 100, Button: widget.MouseLeft, Pressed: true, Clicks: 2})
	if v.cam.Zoom != 1 || v.cam.Yaw != render3d.DefaultCamera().Yaw {
		t.Fatalf("double click returns to the first view, %+v", v.cam)
	}
}

func TestKeysTurnPanAndReset(t *testing.T) {
	v, posted := newViewer(t)
	waitFrame(t, v, posted)
	yaw := v.cam.Yaw
	if !v.HandleKey(widget.KeyEvent{Code: widget.KeyLeft, Pressed: true}) || v.cam.Yaw == yaw {
		t.Fatal("left turns")
	}
	px := v.cam.PanX
	if !v.HandleKey(widget.KeyEvent{Code: widget.KeyRight, Mod: widget.ModShift, Pressed: true}) || v.cam.PanX == px {
		t.Fatal("shift+right pans")
	}
	if v.HandleKey(widget.KeyEvent{Code: widget.KeyLeft, Mod: widget.ModCtrl, Pressed: true}) {
		t.Fatal("ctrl+arrow belongs to the window")
	}
	v.HandleKey(widget.KeyEvent{Code: widget.KeyHome, Pressed: true})
	if v.cam.Yaw != render3d.DefaultCamera().Yaw || v.cam.PanX != 0 {
		t.Fatal("Home resets the view")
	}
	v.Turn(true)
	v.Turn(true)
	v.Turn(true)
	v.Turn(true)
	if d := v.cam.Yaw - render3d.DefaultCamera().Yaw; d > 1e-9 || d < -1e-9 {
		t.Fatalf("four quarter turns come back, off by %v", d)
	}
}

func TestClearDropsLateFrames(t *testing.T) {
	v, posted := newViewer(t)
	v.Clear()
	time.Sleep(50 * time.Millisecond)
	for len(posted) > 0 {
		(<-posted)()
	}
	if v.cur.img != nil || v.HasScene() {
		t.Fatal("a cleared viewer shows nothing")
	}
}
