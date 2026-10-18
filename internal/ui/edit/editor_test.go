package edit

import (
	"fmt"
	"image"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/testutil"
	"github.com/oops1/PixGo/internal/viewer"
)

func TestBeginFitsThePictureBelowTheBar(t *testing.T) {
	r := newRig(t, 1100, 700, 4000, 3000)
	ed := r.ed
	if !ed.Active() || !ed.Crop().Whole() || ed.Crop().Size() != image.Pt(4000, 3000) {
		t.Fatalf("active=%v crop=%v size=%v", ed.Active(), ed.Crop().Rect(), ed.Crop().Size())
	}
	if ed.Bar().Empty() || ed.Area().Min.Y <= ed.Bar().Max.Y {
		t.Fatalf("bar %v area %v", ed.Bar(), ed.Area())
	}
	pic := ed.PictureScreen()
	if pic.Empty() || !pic.In(ed.Area().Inset(-1)) {
		t.Fatalf("picture %v is not inside the area %v", pic, ed.Area())
	}
	if got := ed.CropScreen(); got != pic {
		t.Fatalf("the whole crop %v must cover the picture %v", got, pic)
	}
	if r.canvas.Mode() != viewer.ZoomFit {
		t.Fatal("edit mode must show the picture fitted")
	}
	if got := float64(pic.Dx()) / float64(pic.Dy()); got < 1.32 || got > 1.34 {
		t.Fatalf("aspect of the picture on screen is %.3f", got)
	}
}

func TestBeginRefusesAnEmptyCanvas(t *testing.T) {
	r := newRig(t, 800, 500, 100, 100)
	r.canvas.Clear()
	r.ed.End()
	if r.ed.Begin() {
		t.Fatal("began without a picture")
	}
}

func TestPlanOfAnUntouchedEditor(t *testing.T) {
	r := newRig(t, 1100, 700, 640, 480)
	p := r.ed.Plan()
	if p.Orientation != codec.OrientNormal || !p.Crop.Empty() || p.Size != (image.Point{}) {
		t.Fatalf("plan %+v", p)
	}
	if r.ed.Dirty() {
		t.Fatal("nothing changed yet")
	}
}

func TestMouseDragMovesTheCrop(t *testing.T) {
	r := newRig(t, 1100, 700, 1000, 800)
	r.setCrop(image.Rect(200, 200, 600, 500))
	r.render()
	cr := r.ed.CropScreen()
	cx, cy := centre(cr)
	k := float64(cr.Dx()) / 400
	r.dragFrom(cx, cy, cx+int(50*k), cy+int(30*k))
	got := r.ed.Crop().Rect()
	if got.Dx() != 400 || got.Dy() != 300 {
		t.Fatalf("size changed: %v", got)
	}
	if d := got.Min.X - 250; d < -2 || d > 2 {
		t.Fatalf("moved to %v, want x near 250", got)
	}
	if d := got.Min.Y - 230; d < -2 || d > 2 {
		t.Fatalf("moved to %v, want y near 230", got)
	}
	if r.calls.changed == 0 || !r.ed.Dirty() {
		t.Fatal("the change was not reported")
	}
}

func TestMouseDragResizesFromEveryHandle(t *testing.T) {
	for _, h := range handleList {
		r := newRig(t, 1100, 700, 1000, 800)
		r.setCrop(image.Rect(300, 250, 700, 550))
		r.render()
		before := r.ed.Crop().Rect()
		x, y := r.ed.HandleCenter(h)
		if got := r.ed.HandleAt(x, y); got != h {
			t.Fatalf("handle %d: hit test gives %d at (%d,%d)", h, got, x, y)
		}
		dx, dy := 0, 0
		if h.east() {
			dx = 30
		} else if h.west() {
			dx = -30
		}
		if h.south() {
			dy = 30
		} else if h.north() {
			dy = -30
		}
		r.dragFrom(x, y, x+dx, y+dy)
		after := r.ed.Crop().Rect()
		if after == before {
			t.Fatalf("handle %d: nothing changed", h)
		}
		switch {
		case h.east() && after.Max.X <= before.Max.X, h.west() && after.Min.X >= before.Min.X,
			h.south() && after.Max.Y <= before.Max.Y, h.north() && after.Min.Y >= before.Min.Y:
			t.Fatalf("handle %d: %v -> %v went the wrong way", h, before, after)
		}
	}
}

func TestDragOutsideThePictureClamps(t *testing.T) {
	r := newRig(t, 1100, 700, 1000, 800)
	r.setCrop(image.Rect(300, 250, 700, 550))
	r.render()
	x, y := r.ed.HandleCenter(HandleSE)
	r.dragFrom(x, y, 1090, 690)
	if got := r.ed.Crop().Rect(); got.Max != image.Pt(1000, 800) {
		t.Fatalf("got %v", got)
	}
}

func TestPressOutsideTheCropDoesNothing(t *testing.T) {
	r := newRig(t, 1100, 700, 1000, 800)
	r.setCrop(image.Rect(300, 250, 700, 550))
	r.render()
	before := r.ed.Crop().Rect()
	pic := r.ed.PictureScreen()
	r.dragFrom(pic.Min.X+3, pic.Min.Y+3, pic.Min.X+60, pic.Min.Y+60)
	if r.ed.Crop().Rect() != before {
		t.Fatalf("crop moved to %v", r.ed.Crop().Rect())
	}
}

func TestCursorShapes(t *testing.T) {
	r := newRig(t, 1100, 700, 1000, 800)
	r.setCrop(image.Rect(300, 250, 700, 550))
	r.render()
	cases := map[Handle]widget.Cursor{
		HandleNW: widget.CursorSizeNWSE, HandleSE: widget.CursorSizeNWSE,
		HandleNE: widget.CursorSizeNESW, HandleSW: widget.CursorSizeNESW,
		HandleN: widget.CursorSizeNS, HandleS: widget.CursorSizeNS,
		HandleE: widget.CursorSizeWE, HandleW: widget.CursorSizeWE,
	}
	for h, want := range cases {
		x, y := r.ed.HandleCenter(h)
		if got := r.ed.Cursor(x, y); got != want {
			t.Errorf("handle %d: cursor %v want %v", h, got, want)
		}
	}
	cx, cy := centre(r.ed.CropScreen())
	if got := r.ed.Cursor(cx, cy); got != widget.CursorSizeAll {
		t.Errorf("inside: %v", got)
	}
	if got := r.ed.Cursor(5, 690); got != widget.CursorArrow {
		t.Errorf("outside: %v", got)
	}
	bx, by := centre(r.ed.BarRect(PartRotateLeft))
	if got := r.ed.Cursor(bx, by); got != widget.CursorHand {
		t.Errorf("bar: %v", got)
	}
}

func TestKeyboardMovesAndResizesTheCrop(t *testing.T) {
	r := newRig(t, 1100, 700, 1000, 800)
	r.setCrop(image.Rect(300, 250, 700, 550))
	steps := []struct {
		code widget.KeyCode
		mod  widget.KeyMod
		want image.Rectangle
	}{
		{widget.KeyRight, 0, image.Rect(301, 250, 701, 550)},
		{widget.KeyDown, 0, image.Rect(301, 251, 701, 551)},
		{widget.KeyLeft, widget.ModShift, image.Rect(291, 251, 691, 551)},
		{widget.KeyUp, widget.ModShift, image.Rect(291, 241, 691, 541)},
		{widget.KeyRight, widget.ModCtrl, image.Rect(291, 241, 692, 541)},
		{widget.KeyDown, widget.ModCtrl | widget.ModShift, image.Rect(291, 241, 692, 551)},
		{widget.KeyLeft, widget.ModCtrl, image.Rect(291, 241, 691, 551)},
		{widget.KeyUp, widget.ModCtrl, image.Rect(291, 241, 691, 550)},
	}
	for i, s := range steps {
		r.key(s.code, s.mod)
		if got := r.ed.Crop().Rect(); got != s.want {
			t.Fatalf("step %d: got %v want %v", i, got, s.want)
		}
	}
}

func TestKeyboardStopsAtThePictureEdge(t *testing.T) {
	r := newRig(t, 1100, 700, 1000, 800)
	r.setCrop(image.Rect(0, 0, 400, 300))
	r.key(widget.KeyLeft, widget.ModShift)
	r.key(widget.KeyUp, widget.ModShift)
	if got := r.ed.Crop().Rect(); got != image.Rect(0, 0, 400, 300) {
		t.Fatalf("got %v", got)
	}
	r.setCrop(image.Rect(600, 500, 1000, 800))
	r.key(widget.KeyRight, widget.ModCtrl)
	r.key(widget.KeyDown, widget.ModCtrl)
	if got := r.ed.Crop().Rect(); got != image.Rect(600, 500, 1000, 800) {
		t.Fatalf("got %v", got)
	}
}

func TestRotateKeepsTheContentSelected(t *testing.T) {
	r := newRig(t, 1100, 700, 1000, 800)
	r.setCrop(image.Rect(100, 50, 500, 350))
	r.ed.Rotate(true)
	if got := r.ed.Crop().Size(); got != image.Pt(800, 1000) {
		t.Fatalf("size %v", got)
	}
	if got := r.ed.Crop().Rect(); got != image.Rect(450, 100, 750, 500) {
		t.Fatalf("crop %v", got)
	}
	if got := r.ed.Plan().Orientation; got != codec.OrientNormal.RotateCW() {
		t.Fatalf("orientation %v", got)
	}
	r.ed.Rotate(false)
	if got := r.ed.Crop().Rect(); got != image.Rect(100, 50, 500, 350) {
		t.Fatalf("crop %v", got)
	}
	if r.ed.Plan().Orientation != codec.OrientNormal {
		t.Fatal("orientation did not come back")
	}
	r.ed.FlipH()
	if got := r.ed.Crop().Rect(); got != image.Rect(500, 50, 900, 350) {
		t.Fatalf("flip: %v", got)
	}
	r.ed.FlipV()
	if got := r.ed.Crop().Rect(); got != image.Rect(500, 450, 900, 750) {
		t.Fatalf("flip: %v", got)
	}
	if got := r.ed.Plan().Orientation; got != codec.OrientRotate180 {
		t.Fatalf("orientation %v", got)
	}
}

func TestRotateRespectsTheExifOrientation(t *testing.T) {
	r := newRig(t, 1100, 700, 1000, 800)
	r.ed.End()
	r.canvas.SetImage(source(1000, 800), codec.OrientRotate90, viewer.ZoomFit)
	r.ed.Begin()
	if got := r.ed.Crop().Size(); got != image.Pt(800, 1000) {
		t.Fatalf("size %v", got)
	}
	if got := r.ed.Plan().Orientation; got != codec.OrientRotate90 {
		t.Fatalf("base orientation lost: %v", got)
	}
	if r.ed.Dirty() {
		t.Fatal("an EXIF orientation is not a user change")
	}
	r.ed.Rotate(true)
	if !r.ed.Dirty() {
		t.Fatal("rotating must count as a change")
	}
	if got := r.ed.Plan().Orientation; got != codec.OrientRotate90.RotateCW() {
		t.Fatalf("orientation %v", got)
	}
	r.ed.Reset()
	if got := r.ed.Plan().Orientation; got != codec.OrientRotate90 {
		t.Fatalf("reset gave %v", got)
	}
	if r.ed.Dirty() {
		t.Fatal("reset left the editor dirty")
	}
}

func TestEditorResetRestoresEverything(t *testing.T) {
	r := newRig(t, 1100, 700, 1000, 800)
	r.setCrop(image.Rect(100, 50, 500, 350))
	r.ed.SetAspect(Aspect169)
	r.ed.Rotate(true)
	r.ed.SetOutputSize(200, 300)
	r.ed.SetSnapped(image.Rect(0, 0, 16, 16))
	if !r.ed.Dirty() {
		t.Fatal("expected changes")
	}
	r.ed.Reset()
	if r.ed.Dirty() || !r.ed.Crop().Whole() || r.ed.Crop().Size() != image.Pt(1000, 800) || r.ed.HasResize() || !r.ed.Snapped().Empty() {
		t.Fatalf("not restored: crop %v resize %v", r.ed.Crop().Rect(), r.ed.HasResize())
	}
	if r.ed.Crop().Aspect() != AspectFree {
		t.Fatal("aspect not reset")
	}
}

func TestEndRestoresTheViewerState(t *testing.T) {
	r := newRig(t, 1100, 700, 1000, 800)
	r.ed.End()
	r.canvas.SetMode(viewer.ZoomActual)
	r.canvas.RotateRight()
	before := r.canvas.Snapshot()
	r.ed.Begin()
	r.ed.Rotate(true)
	r.ed.Rotate(true)
	r.ed.End()
	if r.ed.Active() {
		t.Fatal("still active")
	}
	if got := r.canvas.Snapshot(); got != before {
		t.Fatalf("view not restored: %+v vs %+v", got, before)
	}
}

func TestResizeFactorsFollowTheCrop(t *testing.T) {
	r := newRig(t, 1100, 700, 1000, 800)
	r.setCrop(image.Rect(0, 0, 400, 200))
	r.ed.SetOutputSize(200, 100)
	if got := r.ed.OutputSize(); got != image.Pt(200, 100) {
		t.Fatalf("got %v", got)
	}
	if p := r.ed.Plan(); p.Size != image.Pt(200, 100) || p.Crop != image.Rect(0, 0, 400, 200) {
		t.Fatalf("plan %+v", p)
	}
	r.setCrop(image.Rect(0, 0, 600, 300))
	if got := r.ed.OutputSize(); got != image.Pt(300, 150) {
		t.Fatalf("after the crop changed: %v", got)
	}
	r.ed.SetOutputSize(600, 300)
	if r.ed.HasResize() || r.ed.Plan().Size != (image.Point{}) {
		t.Fatal("a size equal to the crop is no resize")
	}
	r.ed.SetOutputSize(10, 10)
	r.ed.ClearResize()
	if r.ed.HasResize() {
		t.Fatal("ClearResize failed")
	}
}

func TestAspectPresetsThroughTheEditor(t *testing.T) {
	r := newRig(t, 1100, 700, 1600, 1200)
	r.ed.SetAspect(Aspect169)
	got := r.ed.Crop().Rect()
	if !ratioOK(got, 16.0/9) || got.Dx() != 1600 {
		t.Fatalf("got %v", got)
	}
	if p := r.ed.Plan(); p.Crop != got {
		t.Fatalf("plan crop %v", p.Crop)
	}
	r.ed.SwapAspect()
	if g := r.ed.Crop().Rect(); g.Dy() <= g.Dx() || !ratioOK(g, 9.0/16) {
		t.Fatalf("swap: %v", g)
	}
	r.ed.SetAspect(AspectFree)
	r.ed.SetAspect(AspectOriginal)
	if !ratioOK(r.ed.Crop().Rect(), 4.0/3) {
		t.Fatalf("original: %v", r.ed.Crop().Rect())
	}
}

func TestBarButtonsCallTheirHandlers(t *testing.T) {
	r := newRig(t, 1100, 700, 1000, 800)
	r.setCrop(image.Rect(100, 100, 500, 400))
	r.clickPart(PartAspect)
	if len(r.calls.aspectMenu) != 1 || r.calls.aspectMenu[0] != r.ed.BarRect(PartAspect) {
		t.Fatalf("aspect menu: %v", r.calls.aspectMenu)
	}
	r.clickPart(PartSaveMore)
	if len(r.calls.saveMenu) != 1 {
		t.Fatalf("save menu: %v", r.calls.saveMenu)
	}
	r.clickPart(PartSaveCopy)
	r.clickPart(PartResize)
	r.clickPart(PartCancel)
	if r.calls.saveCopy != 1 || r.calls.resize != 1 || r.calls.cancel != 1 {
		t.Fatalf("calls %+v", *r.calls)
	}
	r.clickPart(PartRotateRight)
	if r.ed.Crop().Size() != image.Pt(800, 1000) {
		t.Fatal("rotate button did nothing")
	}
	r.clickPart(PartFlipH)
	r.clickPart(PartFlipV)
	r.clickPart(PartRotateLeft)
	r.clickPart(PartReset)
	if r.ed.Dirty() {
		t.Fatal("reset button did not restore")
	}
}

func TestSaveAndResetAreOffWithoutChanges(t *testing.T) {
	r := newRig(t, 1100, 700, 1000, 800)
	for _, p := range []Part{PartSaveCopy, PartSaveMore, PartReset} {
		if r.ed.Enabled(p) {
			t.Errorf("part %d enabled on an untouched picture", p)
		}
	}
	r.clickPart(PartSaveCopy)
	if r.calls.saveCopy != 0 {
		t.Fatal("saved an unchanged picture")
	}
	r.key(widget.KeyEnter, 0)
	if r.calls.saveCopy != 0 {
		t.Fatal("Enter saved an unchanged picture")
	}
	for _, p := range []Part{PartAspect, PartSwap, PartRotateLeft, PartRotateRight, PartFlipH, PartFlipV, PartResize, PartCancel} {
		if !r.ed.Enabled(p) {
			t.Errorf("part %d disabled", p)
		}
	}
}

func TestEnterSavesACopyAndEscapeCancels(t *testing.T) {
	r := newRig(t, 1100, 700, 1000, 800)
	r.setCrop(image.Rect(100, 100, 500, 400))
	r.key(widget.KeyEnter, 0)
	if r.calls.saveCopy != 1 {
		t.Fatalf("calls %+v", *r.calls)
	}
	r.key(widget.KeyEscape, 0)
	if r.calls.cancel != 1 {
		t.Fatalf("calls %+v", *r.calls)
	}
}

func TestTabWalksTheBar(t *testing.T) {
	r := newRig(t, 1100, 700, 1000, 800)
	r.setCrop(image.Rect(100, 100, 500, 400))
	var order []Part
	for i := 0; i < 14; i++ {
		r.key(widget.KeyTab, 0)
		p, kbd := r.ed.FocusedPart()
		if !kbd {
			t.Fatal("keyboard focus not shown")
		}
		order = append(order, p)
	}
	want := []Part{PartCrop, PartAspect, PartSwap, PartRotateLeft, PartRotateRight, PartFlipH, PartFlipV, PartResize, PartReset, PartSaveCopy, PartSaveMore, PartCancel, PartCrop, PartAspect}
	if fmt.Sprint(order) != fmt.Sprint(want) {
		t.Fatalf("order %v want %v", order, want)
	}
	r.key(widget.KeyTab, widget.ModShift)
	if p, _ := r.ed.FocusedPart(); p != PartCrop {
		t.Fatalf("shift-tab landed on %d", p)
	}
	r.key(widget.KeyTab, widget.ModShift)
	if p, _ := r.ed.FocusedPart(); p != PartCancel {
		t.Fatalf("shift-tab wrapped to %d", p)
	}
}

func TestEnterAndSpaceActivateTheFocusedButton(t *testing.T) {
	r := newRig(t, 1100, 700, 1000, 800)
	r.setCrop(image.Rect(100, 100, 500, 400))
	for p := PartNone; p != PartResize; p, _ = r.ed.FocusedPart() {
		r.key(widget.KeyTab, 0)
	}
	r.key(widget.KeyEnter, 0)
	if r.calls.resize != 1 || r.calls.saveCopy != 0 {
		t.Fatalf("calls %+v", *r.calls)
	}
	r.key(widget.KeySpace, 0)
	if r.calls.resize != 2 {
		t.Fatalf("space: %+v", *r.calls)
	}
}

func TestTabSkipsDisabledButtons(t *testing.T) {
	r := newRig(t, 1100, 700, 1000, 800)
	var seen []Part
	for i := 0; i < 10; i++ {
		r.key(widget.KeyTab, 0)
		p, _ := r.ed.FocusedPart()
		seen = append(seen, p)
	}
	for _, p := range seen {
		if p == PartSaveCopy || p == PartSaveMore || p == PartReset {
			t.Fatalf("tab reached the disabled part %d: %v", p, seen)
		}
	}
}

func TestBusyBlocksEverything(t *testing.T) {
	r := newRig(t, 1100, 700, 1000, 800)
	r.setCrop(image.Rect(100, 100, 500, 400))
	r.ed.SetBusy(true)
	r.key(widget.KeyEnter, 0)
	r.key(widget.KeyEscape, 0)
	r.key(widget.KeyRight, 0)
	r.key(widget.KeyTab, 0)
	r.clickPart(PartCancel)
	if r.calls.saveCopy+r.calls.cancel != 0 {
		t.Fatalf("calls %+v", *r.calls)
	}
	if r.ed.Crop().Rect() != image.Rect(100, 100, 500, 400) {
		t.Fatal("the crop moved while busy")
	}
	if got := r.ed.Cursor(500, 300); got != widget.CursorWait {
		t.Fatalf("cursor %v", got)
	}
	r.ed.SetBusy(false)
	r.key(widget.KeyEnter, 0)
	if r.calls.saveCopy != 1 {
		t.Fatal("Enter does not work after busy ends")
	}
}

func TestWheelDoesNotZoom(t *testing.T) {
	r := newRig(t, 1100, 700, 4000, 3000)
	before := r.canvas.Snapshot()
	cx, cy := centre(r.ed.Area())
	r.eng.SendMouseMove(cx, cy)
	r.eng.SendMouseButton(cx, cy, widget.MouseWheelUp, true)
	r.eng.SendMouseButton(cx, cy, widget.MouseWheelUp, false)
	r.eng.SendMouseWheelPixels(cx, cy, 0, -120)
	if got := r.canvas.Snapshot(); got != before {
		t.Fatalf("view changed: %+v", got)
	}
}

func TestSnappedRectangleIsDrawnAndDroppedOnChange(t *testing.T) {
	r := newRig(t, 1100, 700, 1000, 800)
	r.setCrop(image.Rect(100, 100, 500, 400))
	r.ed.SetSnapped(image.Rect(96, 96, 500, 400))
	if r.ed.Snapped().Empty() {
		t.Fatal("not stored")
	}
	img := r.render()
	ox, oy, k, _ := r.ed.mapping()
	s := toScreen(image.Rect(96, 96, 500, 400), ox, oy, k)
	accent := r.ed.Theme.Accent
	found := false
	for x := s.Min.X; x < s.Max.X && !found; x++ {
		if c := img.RGBAAt(x, s.Min.Y); c.R == accent.R && c.G == accent.G && c.B == accent.B {
			found = true
		}
	}
	if !found {
		t.Fatal("the dashed outline was not drawn")
	}
	r.key(widget.KeyRight, 0)
	if !r.ed.Snapped().Empty() {
		t.Fatal("the snapped rectangle must go when the crop moves")
	}
}

func TestDrawDimsOutsideTheCrop(t *testing.T) {
	r := newRig(t, 1100, 700, 1000, 800)
	r.setCrop(image.Rect(250, 200, 750, 600))
	img := r.render()
	ox, oy, k, _ := r.ed.mapping()
	inside := toScreen(image.Rect(250, 200, 750, 600), ox, oy, k)
	cx, cy := centre(inside)
	pic := r.ed.PictureScreen()
	out := image.Pt(pic.Min.X+4, pic.Min.Y+4)
	plain := r.canvas
	_ = plain
	in := img.RGBAAt(cx, cy)
	dimmed := img.RGBAAt(out.X, out.Y)
	ref := gradient(1000, 800)
	wantOut := ref.NRGBAAt(int(float64(out.X-pic.Min.X)/k), int(float64(out.Y-pic.Min.Y)/k))
	if dimmed.R >= wantOut.R && dimmed.G >= wantOut.G && wantOut.R > 20 {
		t.Fatalf("outside pixel %v is not darker than %v", dimmed, wantOut)
	}
	wantIn := ref.NRGBAAt(500, 400)
	if d := int(in.R) - int(wantIn.R); d < -4 || d > 4 {
		t.Fatalf("inside pixel %v changed (want near %v)", in, wantIn)
	}
}

func TestBarLayoutFitsEveryWidth(t *testing.T) {
	for _, lang := range i18n.Supported() {
		for _, k := range []float64{1, 1.33, 1.6} {
			for _, w := range []int{420, 520, 700, 900, 1100, 1600} {
				t.Run(fmt.Sprintf("%s/%.2f/%d", lang, k, w), func(t *testing.T) {
					withScale(t, k, lang)
					r := newRig(t, w, 600, 800, 600)
					bar := r.ed.Bar()
					if !bar.In(r.ed.Bounds()) {
						t.Fatalf("bar %v outside %v", bar, r.ed.Bounds())
					}
					var rects []image.Rectangle
					for _, p := range barStops {
						rc := r.ed.BarRect(p)
						if rc.Empty() {
							t.Fatalf("part %d has no rect", p)
						}
						if !rc.In(bar) {
							t.Errorf("part %d %v outside the bar %v", p, rc, bar)
						}
						for _, o := range rects {
							if rc.Overlaps(o) {
								t.Errorf("part %d %v overlaps %v", p, rc, o)
							}
						}
						rects = append(rects, rc)
					}
					if r.ed.Area().Min.Y < bar.Max.Y {
						t.Errorf("area %v overlaps the bar %v", r.ed.Area(), bar)
					}
				})
			}
		}
	}
}

func TestBarUsesOneRowWhenWide(t *testing.T) {
	r := newRig(t, 1100, 700, 800, 600)
	if r.ed.bar.rows != 1 || r.ed.bar.level != levelFull {
		t.Fatalf("rows=%d level=%v", r.ed.bar.rows, r.ed.bar.level)
	}
	if r.ed.BarRect(PartSaveCopy).Min.X < r.ed.BarRect(PartReset).Max.X {
		t.Fatal("the actions must sit right of the tools")
	}
	n := newRig(t, 420, 600, 800, 600)
	if n.ed.bar.rows != 2 {
		t.Fatalf("a narrow window needs two rows, got %d", n.ed.bar.rows)
	}
}

func TestAccessTreeNamesEveryControl(t *testing.T) {
	r := newRig(t, 1100, 700, 1000, 800)
	r.setCrop(image.Rect(10, 20, 410, 320))
	tree := widget.BuildAccessTree(r.ed, nil)
	flat := testutil.FlatAccess(tree)
	names := map[string]bool{}
	for _, n := range flat {
		if testutil.InteractiveRole(n.Role) && n.Name == "" {
			t.Errorf("unnamed %s at %v", n.Role, n.Bounds)
		}
		names[n.Name] = true
	}
	for _, key := range []string{"edit.aspect", "edit.swap", "cmd.rotateLeft", "cmd.rotateRight", "cmd.flipH", "cmd.flipV", "edit.resize", "edit.reset", "edit.saveCopy", "edit.saveMenu", "edit.cancel", "edit.cropArea"} {
		if !names[i18n.T(key)] {
			t.Errorf("no node named %q", i18n.T(key))
		}
	}
	crop := testutil.FindAccess(tree, widget.RoleGroup, i18n.T("edit.cropArea"))
	if crop == nil || crop.Value != "10, 20, 400 × 300" {
		t.Fatalf("crop node %+v", crop)
	}
	combo := testutil.FindAccess(tree, widget.RoleComboBox, i18n.T("edit.aspect"))
	if combo == nil || combo.Value != i18n.T("edit.aspect.free") {
		t.Fatalf("aspect node %+v", combo)
	}
	r.ed.SetAspect(Aspect43)
	tree = widget.BuildAccessTree(r.ed, nil)
	if combo = testutil.FindAccess(tree, widget.RoleComboBox, i18n.T("edit.aspect")); combo == nil || combo.Value != "4:3" {
		t.Fatalf("aspect node %+v", combo)
	}
}

func TestAccessActionsDoWhatClicksDo(t *testing.T) {
	r := newRig(t, 1100, 700, 1000, 800)
	r.setCrop(image.Rect(10, 20, 410, 320))
	infos := r.ed.AccessChildren()
	testutil.RequireActionIDs(t, infos)
	for _, in := range infos {
		if in.ID == idSize {
			if r.ed.AccessAction(in.ID, widget.AccessInvoke) {
				t.Error("the size label must not be invokable")
			}
		}
	}
	if !r.ed.AccessAction(PartID(PartRotateRight), widget.AccessInvoke) || r.ed.Crop().Size() != image.Pt(800, 1000) {
		t.Fatal("invoke rotate")
	}
	if !r.ed.AccessAction(PartID(PartResize), widget.AccessInvoke) || r.calls.resize != 1 {
		t.Fatal("invoke resize")
	}
	if !r.ed.AccessAction(PartID(PartSaveCopy), widget.AccessInvoke) || r.calls.saveCopy != 1 {
		t.Fatal("invoke save")
	}
	if !r.ed.AccessAction(PartID(PartAspect), widget.AccessExpand) || len(r.calls.aspectMenu) != 1 {
		t.Fatal("expand aspect")
	}
	if !r.ed.AccessAction(PartID(PartCancel), widget.AccessFocus) {
		t.Fatal("focus cancel")
	}
	if p, kbd := r.ed.FocusedPart(); p != PartCancel || !kbd {
		t.Fatalf("focus %d %v", p, kbd)
	}
	if !r.ed.AccessAction(idCrop, widget.AccessFocus) {
		t.Fatal("focus crop")
	}
	if p, _ := r.ed.FocusedPart(); p != PartCrop {
		t.Fatalf("focus %d", p)
	}
	if r.ed.AccessAction(idCrop, widget.AccessInvoke) || r.ed.AccessAction("nope", widget.AccessInvoke) || r.ed.AccessAction(PartID(PartRotateLeft), widget.AccessToggle) {
		t.Fatal("unexpected success")
	}
	r.ed.Reset()
	if r.ed.AccessAction(PartID(PartReset), widget.AccessInvoke) {
		t.Fatal("a disabled button accepted an action")
	}
}

func TestTooltipAppearsAfterTheDelay(t *testing.T) {
	r := newRig(t, 1100, 700, 1000, 800)
	var pending []func()
	r.ed.After = func(d time.Duration, fn func()) func() {
		pending = append(pending, fn)
		return func() {}
	}
	x, y := centre(r.ed.BarRect(PartFlipH))
	r.eng.SendMouseMove(x, y)
	if len(pending) != 1 || r.ed.tipText != "" {
		t.Fatalf("pending %d tip %q", len(pending), r.ed.tipText)
	}
	pending[0]()
	if r.ed.tipText != i18n.T("cmd.flipH")+" (H)" {
		t.Fatalf("tip %q", r.ed.tipText)
	}
	rc := r.ed.tipRect(nil)
	if rc.Empty() || rc.Overlaps(r.ed.Bar()) || !rc.In(r.ed.Bounds()) {
		t.Fatalf("tip rect %v bar %v", rc, r.ed.Bar())
	}
	r.render()
	r.eng.SendMouseMove(5, 690)
	if r.ed.tipText != "" {
		t.Fatalf("tip stayed: %q", r.ed.tipText)
	}
}
