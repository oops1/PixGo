package view

import (
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/imaging"
	"github.com/oops1/PixGo/internal/render"
	"github.com/oops1/PixGo/internal/viewer"
)

const (
	areaW = 400
	areaH = 300
)

func gradient(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{uint8(x * 255 / max(1, w-1)), uint8(y * 255 / max(1, h-1)), 128, 255})
		}
	}
	return img
}

func source(w, h int) render.Source {
	return render.Source{Levels: imaging.BuildPyramid(gradient(w, h), 64), Width: w, Height: h}
}

type rig struct {
	eng *engine.Engine
	c   *Canvas
	t   *testing.T
	now time.Time
}

func newRig(t *testing.T) *rig {
	t.Helper()
	eng := engine.New(areaW, areaH, 30)
	t.Cleanup(eng.Stop)
	c := New(eng)
	r := &rig{eng: eng, c: c, t: t, now: time.Unix(1000, 0)}
	c.Now = func() time.Time { return r.now }
	c.Background = render.Background{Canvas: color.RGBA{10, 20, 30, 255}, Kind: render.BgSolid, Fill: color.RGBA{10, 20, 30, 255}}
	eng.SetRootFullCanvas(c)
	return r
}

func (r *rig) frame() *image.RGBA {
	r.eng.Flush()
	return r.eng.RenderOnce()
}

func TestFitCentresImageOnBackground(t *testing.T) {
	r := newRig(t)
	r.c.SetImage(source(200, 100), codec.OrientNormal, viewer.ZoomFit)
	img := r.frame()
	if got := img.RGBAAt(5, 5); got != (color.RGBA{10, 20, 30, 255}) {
		t.Errorf("corner = %v, want canvas colour", got)
	}
	got := img.RGBAAt(areaW/2, areaH/2)
	if got.A != 255 || got.B < 100 || got.B > 160 {
		t.Errorf("centre = %v, want image pixel", got)
	}
	if r.c.ZoomPercent() != 100 {
		t.Errorf("zoom = %d%%, want 100%% for a small image", r.c.ZoomPercent())
	}
	rect := r.c.ImageRectLogical()
	if rect != image.Rect(100, 100, 300, 200) {
		t.Errorf("image rect = %v", rect)
	}
}

func TestFitDownscalesLargeImage(t *testing.T) {
	r := newRig(t)
	r.c.SetImage(source(1600, 1200), codec.OrientNormal, viewer.ZoomFit)
	r.frame()
	if p := r.c.ZoomPercent(); p != 25 {
		t.Errorf("fit zoom = %d%%, want 25%%", p)
	}
	if rect := r.c.ImageRectLogical(); rect != image.Rect(0, 0, 400, 300) {
		t.Errorf("image rect = %v", rect)
	}
}

func TestWheelZoomKeepsPointUnderCursor(t *testing.T) {
	r := newRig(t)
	r.c.SetImage(source(1600, 1200), codec.OrientNormal, viewer.ZoomFit)
	r.frame()
	u0, v0 := r.c.View().AreaToImage(r.c.Frame(), 300.5, 100.5)
	r.eng.SendMouseWheelPixels(300, 100, 0, -40)
	r.frame()
	if r.c.ZoomPercent() <= 25 {
		t.Fatalf("wheel up did not zoom in: %d%%", r.c.ZoomPercent())
	}
	u1, v1 := r.c.View().AreaToImage(r.c.Frame(), 300.5, 100.5)
	if d := u1 - u0; d > 0.5 || d < -0.5 {
		t.Errorf("image u under cursor moved %.2f -> %.2f", u0, u1)
	}
	if d := v1 - v0; d > 0.5 || d < -0.5 {
		t.Errorf("image v under cursor moved %.2f -> %.2f", v0, v1)
	}
}

func TestDoubleClickTogglesFitAndActual(t *testing.T) {
	r := newRig(t)
	r.c.SetImage(source(1600, 1200), codec.OrientNormal, viewer.ZoomFit)
	r.frame()
	r.c.OnMouseButton(widget.MouseEvent{X: 200, Y: 150, Button: widget.MouseLeft, Pressed: true, Clicks: 2})
	r.c.OnMouseButton(widget.MouseEvent{X: 200, Y: 150, Button: widget.MouseLeft, Pressed: false, Clicks: 2})
	if p := r.c.ZoomPercent(); p != 100 {
		t.Fatalf("after double click zoom = %d%%, want 100%%", p)
	}
	r.c.OnMouseButton(widget.MouseEvent{X: 200, Y: 150, Button: widget.MouseLeft, Pressed: true, Clicks: 2})
	r.c.OnMouseButton(widget.MouseEvent{X: 200, Y: 150, Button: widget.MouseLeft, Pressed: false, Clicks: 2})
	if p := r.c.ZoomPercent(); p != 25 {
		t.Fatalf("second double click zoom = %d%%, want 25%%", p)
	}
}

func TestDragPansWhenZoomedIn(t *testing.T) {
	r := newRig(t)
	r.c.SetImage(source(1600, 1200), codec.OrientNormal, viewer.ZoomActual)
	r.frame()
	if !r.c.CanPan() {
		t.Fatal("cannot pan at 100% of a large image")
	}
	before := r.frame().RGBAAt(200, 150)
	r.c.OnMouseButton(widget.MouseEvent{X: 200, Y: 150, Button: widget.MouseLeft, Pressed: true, Clicks: 1})
	r.c.OnMouseMove(150, 150)
	r.c.OnMouseMove(100, 150)
	r.c.OnMouseButton(widget.MouseEvent{X: 100, Y: 150, Button: widget.MouseLeft, Pressed: false, Clicks: 1})
	after := r.frame().RGBAAt(200, 150)
	if after == before {
		t.Errorf("drag left by 100 px did not change the pixel under the cursor: %v", after)
	}
	if after.R <= before.R {
		t.Errorf("dragging left should reveal image to the right (R grows): before %v after %v", before, after)
	}
}

func TestSwipeNavigatesWhenFitted(t *testing.T) {
	r := newRig(t)
	r.c.SetImage(source(1600, 1200), codec.OrientNormal, viewer.ZoomFit)
	r.frame()
	var steps []int
	r.c.OnStep = func(d int) { steps = append(steps, d) }
	swipe := func(x0, x1 int, d time.Duration) {
		r.c.OnMouseButton(widget.MouseEvent{X: x0, Y: 150, Button: widget.MouseLeft, Pressed: true, Clicks: 1})
		r.now = r.now.Add(d)
		r.c.OnMouseMove((x0+x1)/2, 150)
		r.c.OnMouseMove(x1, 150)
		r.c.OnMouseButton(widget.MouseEvent{X: x1, Y: 150, Button: widget.MouseLeft, Pressed: false, Clicks: 1})
	}
	swipe(300, 150, 100*time.Millisecond)
	swipe(100, 250, 100*time.Millisecond)
	swipe(300, 150, 900*time.Millisecond)
	swipe(300, 280, 100*time.Millisecond)
	if len(steps) != 2 || steps[0] != 1 || steps[1] != -1 {
		t.Errorf("steps = %v, want [1 -1]", steps)
	}
}

func TestWheelNavigateMode(t *testing.T) {
	r := newRig(t)
	r.c.Wheel = WheelNavigate
	r.c.SetImage(source(1600, 1200), codec.OrientNormal, viewer.ZoomFit)
	r.frame()
	var steps []int
	r.c.OnStep = func(d int) { steps = append(steps, d) }
	r.c.OnMouseWheelPixelsMod(200, 150, 0, 40, 0)
	r.c.OnMouseWheelPixelsMod(200, 150, 0, -40, 0)
	r.c.OnMouseButton(widget.MouseEvent{X: 200, Y: 150, Button: widget.MouseWheelDown})
	zoomBefore := r.c.ZoomPercent()
	r.c.OnMouseWheelPixelsMod(200, 150, 0, -40, widget.ModCtrl)
	if len(steps) != 3 || steps[0] != 1 || steps[1] != -1 || steps[2] != 1 {
		t.Errorf("steps = %v, want [1 -1 1]", steps)
	}
	if r.c.ZoomPercent() <= zoomBefore {
		t.Errorf("ctrl+wheel did not zoom: %d%% -> %d%%", zoomBefore, r.c.ZoomPercent())
	}
}

func TestRotateSwapsFittedExtent(t *testing.T) {
	r := newRig(t)
	r.c.SetImage(source(600, 200), codec.OrientNormal, viewer.ZoomFit)
	r.frame()
	r.c.RotateRight()
	r.frame()
	rect := r.c.ImageRectLogical()
	if rect.Dx() != 100 || rect.Dy() != 300 {
		t.Errorf("rotated fit rect = %v, want 100x300", rect)
	}
}

func TestExifOrientationApplied(t *testing.T) {
	r := newRig(t)
	r.c.SetImage(source(600, 200), codec.OrientRotate90, viewer.ZoomFit)
	r.frame()
	rect := r.c.ImageRectLogical()
	if rect.Dx() != 100 || rect.Dy() != 300 {
		t.Errorf("exif-rotated rect = %v, want 100x300", rect)
	}
}

func TestOnChangeFiresOnZoom(t *testing.T) {
	r := newRig(t)
	r.c.SetImage(source(1600, 1200), codec.OrientNormal, viewer.ZoomFit)
	r.frame()
	n := 0
	r.c.OnChange = func() { n++ }
	r.c.ZoomIn()
	r.c.Actual()
	if n < 2 {
		t.Errorf("OnChange fired %d times, want at least 2", n)
	}
	n = 0
	r.c.Actual()
	if n != 0 {
		t.Errorf("OnChange fired for a no-op mode change")
	}
}

func TestEmptyDrawsBackgroundOnly(t *testing.T) {
	r := newRig(t)
	img := r.frame()
	if got := img.RGBAAt(200, 150); got != (color.RGBA{10, 20, 30, 255}) {
		t.Errorf("empty view pixel = %v", got)
	}
	r.c.ZoomIn()
	r.c.Fit()
	r.c.RotateLeft()
}

func TestHiDPIRendersExactPhysicalTile(t *testing.T) {
	for _, k := range []float64{1.25, 1.5, 2} {
		eng := engine.New(areaW, areaH, 30)
		t.Cleanup(eng.Stop)
		eng.SetScale(k)
		c := New(eng)
		c.Background = render.Background{Canvas: color.RGBA{10, 20, 30, 255}, Kind: render.BgSolid, Fill: color.RGBA{10, 20, 30, 255}}
		eng.SetRootFullCanvas(c)
		c.SetImage(source(1600, 1200), codec.OrientNormal, viewer.ZoomFit)
		eng.Flush()
		img := eng.RenderOnce()
		pw, ph := img.Bounds().Dx(), img.Bounds().Dy()
		if c.areaW != pw || c.areaH != ph {
			t.Errorf("scale %.2f: canvas area %dx%d, frame %dx%d", k, c.areaW, c.areaH, pw, ph)
		}
		mid := img.RGBAAt(pw/2, ph/2)
		if mid.A != 255 || mid == (color.RGBA{10, 20, 30, 255}) {
			t.Errorf("scale %.2f: centre pixel %v is not image content", k, mid)
		}
		want := int(25*k + .5)
		if got := c.ZoomPercent(); got < want-1 || got > want+1 {
			t.Errorf("scale %.2f: zoom %d%%, want %d%% (one image pixel per physical pixel is 100%%)", k, got, want)
		}
	}
}

func touch(c *Canvas, id, x, y int, ph widget.TouchPhase, at time.Duration) {
	c.OnTouch(widget.TouchEvent{ID: id, X: x, Y: y, Phase: ph, Time: at})
}

func TestPinchZoomsAroundCentre(t *testing.T) {
	r := newRig(t)
	r.c.SetImage(source(1600, 1200), codec.OrientNormal, viewer.ZoomFit)
	r.frame()
	before := r.c.ZoomPercent()
	touch(r.c, 1, 180, 150, widget.TouchBegan, 0)
	touch(r.c, 2, 220, 150, widget.TouchBegan, 0)
	for i := 1; i <= 10; i++ {
		d := time.Duration(i) * 16 * time.Millisecond
		touch(r.c, 1, 180-i*8, 150, widget.TouchMoved, d)
		touch(r.c, 2, 220+i*8, 150, widget.TouchMoved, d)
	}
	touch(r.c, 1, 100, 150, widget.TouchEnded, 200*time.Millisecond)
	touch(r.c, 2, 300, 150, widget.TouchEnded, 200*time.Millisecond)
	if after := r.c.ZoomPercent(); after <= before {
		t.Errorf("pinch out did not zoom in: %d%% -> %d%%", before, after)
	}
}

func TestOneFingerPanMovesZoomedImage(t *testing.T) {
	r := newRig(t)
	r.c.SetImage(source(1600, 1200), codec.OrientNormal, viewer.ZoomActual)
	r.frame()
	before := r.frame().RGBAAt(200, 150)
	touch(r.c, 1, 300, 150, widget.TouchBegan, 0)
	for i := 1; i <= 12; i++ {
		touch(r.c, 1, 300-i*10, 150, widget.TouchMoved, time.Duration(i)*20*time.Millisecond)
	}
	touch(r.c, 1, 180, 150, widget.TouchEnded, 300*time.Millisecond)
	if after := r.frame().RGBAAt(200, 150); after == before {
		t.Errorf("one-finger pan did not move the image")
	}
}

func TestSwipeFlipsImageWhenFitted(t *testing.T) {
	r := newRig(t)
	r.c.SetImage(source(1600, 1200), codec.OrientNormal, viewer.ZoomFit)
	r.frame()
	var steps []int
	r.c.OnStep = func(d int) { steps = append(steps, d) }
	touch(r.c, 1, 320, 150, widget.TouchBegan, 0)
	touch(r.c, 1, 260, 150, widget.TouchMoved, 20*time.Millisecond)
	touch(r.c, 1, 200, 150, widget.TouchMoved, 40*time.Millisecond)
	touch(r.c, 1, 120, 150, widget.TouchMoved, 60*time.Millisecond)
	touch(r.c, 1, 100, 150, widget.TouchEnded, 70*time.Millisecond)
	if len(steps) != 1 || steps[0] != 1 {
		t.Errorf("steps = %v, want [1] for a swipe left", steps)
	}
}

func TestDoubleTapTogglesZoom(t *testing.T) {
	r := newRig(t)
	r.c.SetImage(source(1600, 1200), codec.OrientNormal, viewer.ZoomFit)
	r.frame()
	tap := func(at time.Duration) {
		touch(r.c, 1, 200, 150, widget.TouchBegan, at)
		touch(r.c, 1, 200, 150, widget.TouchEnded, at+40*time.Millisecond)
	}
	tap(0)
	tap(150 * time.Millisecond)
	if p := r.c.ZoomPercent(); p != 100 {
		t.Errorf("double tap zoom = %d%%, want 100%%", p)
	}
}

func TestTouchOutsideImageNotClaimed(t *testing.T) {
	r := newRig(t)
	if r.c.OnTouch(widget.TouchEvent{ID: 1, X: 10, Y: 10, Phase: widget.TouchBegan}) {
		t.Error("touch claimed with no image loaded")
	}
	r.c.SetImage(source(100, 100), codec.OrientNormal, viewer.ZoomFit)
	r.c.Chrome = func(x, y int) (widget.Cursor, bool) { return widget.CursorArrow, y > 250 }
	if r.c.OnTouch(widget.TouchEvent{ID: 2, X: 10, Y: 280, Phase: widget.TouchBegan}) {
		t.Error("touch on chrome claimed by the canvas")
	}
}

func TestRotate180KeepsTheFittedExtent(t *testing.T) {
	r := newRig(t)
	r.c.SetImage(source(600, 200), codec.OrientNormal, viewer.ZoomFit)
	r.frame()
	before := r.c.ImageRectLogical()
	r.c.Rotate180()
	r.frame()
	if r.c.Orientation() != codec.OrientRotate180 {
		t.Fatalf("orientation %d", r.c.Orientation())
	}
	if rect := r.c.ImageRectLogical(); rect != before {
		t.Errorf("rect after a half turn %v, want %v", rect, before)
	}
}

func TestTheViewerBufferIsAlwaysOpaqueAsPromisedToTheEngine(t *testing.T) {
	translucent := image.NewNRGBA(image.Rect(0, 0, 60, 40))
	for i := 0; i < len(translucent.Pix); i += 4 {
		translucent.Pix[i], translucent.Pix[i+1], translucent.Pix[i+2], translucent.Pix[i+3] = 200, 40, 90, uint8(i/4%256)
	}
	clear := render.Source{Levels: imaging.BuildPyramid(translucent, 16), Width: 60, Height: 40}
	checker := render.Background{Canvas: color.RGBA{1, 2, 3, 255}, Kind: render.BgChecker, Checker: [2]color.RGBA{{200, 200, 200, 255}, {90, 90, 90, 255}}, Cell: 8}
	for name, setup := range map[string]func(r *rig){
		"empty":                 func(r *rig) {},
		"translucent on solid":  func(r *rig) { r.c.SetImage(clear, codec.OrientNormal, viewer.ZoomActual) },
		"translucent on checks": func(r *rig) { r.c.SetBackground(checker); r.c.SetImage(clear, codec.OrientNormal, viewer.ZoomFit) },
		"opaque rotated":        func(r *rig) { r.c.SetImage(source(300, 120), codec.OrientRotate90, viewer.ZoomFit) },
	} {
		t.Run(name, func(t *testing.T) {
			r := newRig(t)
			setup(r)
			r.frame()
			if r.c.buf == nil {
				t.Fatal("no buffer was drawn")
			}
			for i := 3; i < len(r.c.buf.Pix); i += 4 {
				if r.c.buf.Pix[i] != 0xff {
					t.Fatalf("pixel %d has alpha %d; the buffer is declared opaque", i/4, r.c.buf.Pix[i])
				}
			}
		})
	}
}
