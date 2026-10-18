package app

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/commands"
	"github.com/oops1/PixGo/internal/i18n"
	uiedit "github.com/oops1/PixGo/internal/ui/edit"
	"github.com/oops1/PixGo/internal/ui/window"
)

func TestEditCommandEntersTheEditMode(t *testing.T) {
	r := newPNGEditRig(t, 640, 480)
	if r.app.Editing() {
		t.Fatal("editing before the command")
	}
	r.enter()
	if r.app.Content() != widget.Widget(r.app.Editor) {
		t.Fatal("the editor must be the page content")
	}
	if r.app.Editor.Crop().Size() != image.Pt(640, 480) || !r.app.Editor.Crop().Whole() {
		t.Fatalf("crop %v of %v", r.app.Editor.Crop().Rect(), r.app.Editor.Crop().Size())
	}
	l := r.app.Root.Layout()
	for _, k := range []window.ZoneKind{window.ZoneToolEdit, window.ZoneToolGallery, window.ZonePrev, window.ZoneNext} {
		if !l.Rect(k).Empty() {
			t.Errorf("zone %d is still on the screen in the edit mode", k)
		}
	}
	if !r.app.Editor.Bounds().Eq(r.app.Root.Layout().View) {
		t.Fatalf("editor bounds %v, view %v", r.app.Editor.Bounds(), l.View)
	}
}

func TestCtrlEStartsEditingFromTheKeyboard(t *testing.T) {
	r := newPNGEditRig(t, 640, 480)
	r.key(widget.KeyE, widget.ModCtrl)
	r.settle()
	if !r.app.Editing() {
		t.Fatal("Ctrl+E did nothing")
	}
}

func TestToolbarEditButtonStartsEditing(t *testing.T) {
	r := newPNGEditRig(t, 640, 480)
	z := r.app.Root.Layout().Rect(window.ZoneToolEdit)
	if z.Empty() {
		t.Fatal("no edit button in the toolbar")
	}
	x, y := (z.Min.X+z.Max.X)/2, (z.Min.Y+z.Max.Y)/2
	r.app.Eng.SendMouseMove(x, y)
	r.app.Eng.SendMouseButton(x, y, widget.MouseLeft, true)
	r.app.Eng.SendMouseButton(x, y, widget.MouseLeft, false)
	r.settle()
	if !r.app.Editing() {
		t.Fatal("the toolbar button did nothing")
	}
}

func TestMenuOffersCropAndResize(t *testing.T) {
	r := newPNGEditRig(t, 640, 480)
	var crop, resize bool
	for _, it := range r.app.menuItems() {
		switch it.Text {
		case i18n.T("cmd.crop"):
			crop = !it.Disabled
		case i18n.T("cmd.resize"):
			resize = !it.Disabled
		}
	}
	if !crop || !resize {
		t.Fatalf("crop %v resize %v", crop, resize)
	}
}

func TestResizeCommandOpensTheDialogInTheEditMode(t *testing.T) {
	r := newPNGEditRig(t, 640, 480)
	r.app.Commands.Run(CmdResize)
	r.settle()
	if !r.app.Editing() || r.app.resizeDlg == nil {
		t.Fatalf("editing %v dialog %v", r.app.Editing(), r.app.resizeDlg != nil)
	}
	if got := r.app.resizeDlg.Model().Orig(); got != image.Pt(640, 480) {
		t.Fatalf("original %v", got)
	}
}

func TestCropCommandEntersWithoutADialog(t *testing.T) {
	r := newPNGEditRig(t, 640, 480)
	r.app.Commands.Run(CmdCrop)
	r.settle()
	if !r.app.Editing() || r.app.resizeDlg != nil {
		t.Fatalf("editing %v dialog %v", r.app.Editing(), r.app.resizeDlg != nil)
	}
}

func TestEscapeLeavesAnUntouchedEditorAtOnce(t *testing.T) {
	r := newPNGEditRig(t, 640, 480)
	r.enter()
	r.key(widget.KeyEscape, 0)
	r.settle()
	if r.app.Editing() {
		t.Fatal("still editing")
	}
	if r.app.Content() != widget.Widget(r.app.Canvas) {
		t.Fatal("the viewer is not back")
	}
	if r.app.Root.Layout().Rect(window.ZoneToolEdit).Empty() {
		t.Fatal("the toolbar did not come back")
	}
}

func TestEscapeWithChangesAsksBeforeDiscarding(t *testing.T) {
	r := newPNGEditRig(t, 640, 480)
	r.enter()
	r.cropTo(image.Rect(10, 10, 300, 200))
	r.key(widget.KeyEscape, 0)
	r.settle()
	if !r.app.Editing() {
		t.Fatal("left without asking")
	}
	if !messageShown(r.app, i18n.T("edit.discardText")) {
		t.Fatal("no confirmation")
	}
	r.key(widget.KeyEscape, 0)
	r.settle()
	if !r.app.Editing() {
		t.Fatal("dismissing the question must keep editing")
	}
	r.app.Editor.Crop().Reset()
}

func TestRotateCommandsAreRoutedToTheEditor(t *testing.T) {
	r := newPNGEditRig(t, 640, 480)
	r.enter()
	before := r.app.Canvas.Orientation()
	r.app.Commands.Run(CmdRotateRight)
	if r.app.Editor.Crop().Size() != image.Pt(480, 640) {
		t.Fatalf("crop size %v", r.app.Editor.Crop().Size())
	}
	if r.app.Canvas.Orientation() == before {
		t.Fatal("the picture did not turn")
	}
	r.app.Commands.Run(CmdRotateLeft)
	r.app.Commands.Run(CmdFlipH)
	r.app.Commands.Run(CmdFlipV)
	if !r.app.Editor.Dirty() || r.app.Editor.Plan().Orientation != codec.OrientRotate180 {
		t.Fatalf("flip H + flip V = 180: %v", r.app.Editor.Plan().Orientation)
	}
	r.app.Commands.Run(CmdRotate180)
	if r.app.Editor.Dirty() || r.app.Editor.Plan().Orientation != codec.OrientNormal {
		t.Fatalf("180 + 180 must cancel out: %v", r.app.Editor.Plan().Orientation)
	}
}

func TestViewerCommandsAreOffWhileEditing(t *testing.T) {
	r := newPNGEditRig(t, 640, 480)
	r.enter()
	for _, id := range []commands.ID{CmdZoomIn, CmdZoomOut, CmdZoomFit, CmdZoomActual, CmdPrev, CmdNext, CmdDelete, CmdGallery, CmdBack, CmdSettings, CmdFullscreen, CmdSlideshow} {
		if r.app.Commands.Get(id).IsEnabled() {
			t.Errorf("%s is enabled in the edit mode", id)
		}
	}
	for _, id := range []commands.ID{CmdRotateLeft, CmdFlipH, CmdSaveAs, CmdInfo} {
		if !r.app.Commands.Get(id).IsEnabled() {
			t.Errorf("%s is disabled in the edit mode", id)
		}
	}
	r.app.endEdit()
	for _, id := range []commands.ID{CmdZoomIn, CmdDelete, CmdFullscreen} {
		if !r.app.Commands.Get(id).IsEnabled() {
			t.Errorf("%s stays disabled after the edit mode", id)
		}
	}
}

func TestEditIsRefusedForAnimatedPictures(t *testing.T) {
	r := newEditRig(t, "anim.gif", writeAnimatedGIF, nil)
	r.app.Commands.Run(CmdEditStart)
	r.settle()
	if r.app.Editing() {
		t.Fatal("an animation was opened for editing")
	}
	if r.app.Root.ToastText() != i18n.T("edit.refused.animated") {
		t.Fatalf("notice %q", r.app.Root.ToastText())
	}
}

func TestEditIsRefusedForVectorPictures(t *testing.T) {
	r := newEditRig(t, "logo.svg", writeSVGFile, nil)
	r.app.Commands.Run(CmdEditStart)
	r.settle()
	if r.app.Editing() {
		t.Fatal("an SVG was opened for editing")
	}
	if r.app.Root.ToastText() != i18n.T("edit.refused.vector") {
		t.Fatalf("notice %q", r.app.Root.ToastText())
	}
}

func TestEditIsRefusedWithoutAPicture(t *testing.T) {
	reg := codec.NewRegistry()
	a := formatsApp(t, reg, nil)
	if a.Commands.Get(CmdEditStart).IsEnabled() {
		t.Fatal("enabled with nothing shown")
	}
	a.Commands.Run(CmdEditStart)
	if a.Editing() {
		t.Fatal("editing nothing")
	}
}

func TestOpeningAnotherPictureEndsTheEdit(t *testing.T) {
	r := newPNGEditRig(t, 640, 480)
	other := r.dir + "/other.png"
	writePNGFile(t, other, 100, 80)
	r.enter()
	r.app.OpenFiles([]string{other})
	waitName(t, r.app, "other.png")
	if r.app.Editing() {
		t.Fatal("still editing the previous picture")
	}
}

func TestSettingsAndOverwriteFieldsDefault(t *testing.T) {
	r := newPNGEditRig(t, 64, 48)
	st := r.app.Settings.Get()
	if !st.ConfirmOverwrite || st.SaveStripGPS {
		t.Fatalf("defaults %+v", st)
	}
}

func TestBarButtonsWorkWithTheMouseInsideTheApp(t *testing.T) {
	r := newPNGEditRig(t, 640, 480)
	r.enter()
	rc := r.app.Editor.BarRect(uiedit.PartRotateRight)
	x, y := (rc.Min.X+rc.Max.X)/2, (rc.Min.Y+rc.Max.Y)/2
	r.app.Eng.SendMouseMove(x, y)
	r.app.Eng.SendMouseButton(x, y, widget.MouseLeft, true)
	r.app.Eng.SendMouseButton(x, y, widget.MouseLeft, false)
	r.settle()
	if r.app.Editor.Crop().Size() != image.Pt(480, 640) {
		t.Fatalf("size %v", r.app.Editor.Crop().Size())
	}
}

func TestCropDragWorksWithTheMouseInsideTheApp(t *testing.T) {
	r := newPNGEditRig(t, 640, 480)
	r.enter()
	r.app.Eng.Invalidate()
	r.settle()
	x, y := r.app.Editor.HandleCenter(uiedit.HandleSE)
	r.app.Eng.SendMouseMove(x, y)
	r.app.Eng.SendMouseButton(x, y, widget.MouseLeft, true)
	r.app.Eng.SendMouseMove(x-40, y-30)
	r.app.Eng.SendMouseMove(x-80, y-60)
	r.app.Eng.SendMouseButton(x-80, y-60, widget.MouseLeft, false)
	r.settle()
	got := r.app.Editor.Crop().Rect()
	if got.Max.X >= 640 || got.Max.Y >= 480 || got.Min != (image.Point{}) {
		t.Fatalf("crop %v", got)
	}
	if !r.app.Editor.Dirty() {
		t.Fatal("not dirty")
	}
}

func TestTitleBarStaysUsableWhileEditing(t *testing.T) {
	r := newPNGEditRig(t, 640, 480)
	r.enter()
	l := r.app.Root.Layout()
	if cur, owns := r.app.Root.Owns(l.Rect(window.ZoneClose).Min.X+2, 10); !owns || cur != widget.CursorArrow {
		t.Fatalf("the caption belongs to the window chrome: %v %v", cur, owns)
	}
}

func TestEditPlanStoresResizeFactorsAcrossCropChanges(t *testing.T) {
	r := newPNGEditRig(t, 640, 480)
	r.enter()
	r.cropTo(image.Rect(0, 0, 400, 200))
	r.app.Editor.SetOutputSize(200, 100)
	r.cropTo(image.Rect(0, 0, 600, 300))
	if got := r.app.Editor.Plan().Size; got != image.Pt(300, 150) {
		t.Fatalf("plan size %v", got)
	}
}
