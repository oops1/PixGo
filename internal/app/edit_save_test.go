package app

import (
	"bytes"
	"image"
	"image/draw"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/imaging"
	"github.com/oops1/PixGo/internal/settings"
	"github.com/oops1/PixGo/internal/ui/saveopts"
)

func TestSaveCopyNamesTheFileNextToTheOriginal(t *testing.T) {
	r := newPNGEditRig(t, 640, 480)
	r.enter()
	r.cropTo(image.Rect(10, 20, 330, 260))
	r.app.saveEditCopy()
	data := r.waitFile("photo (2).png")
	r.waitEnded()
	if b := decodeFile(t, data).Bounds(); b.Dx() != 320 || b.Dy() != 240 {
		t.Fatalf("copy is %v", b)
	}
	if r.app.Ctl.Path() != r.src {
		t.Fatalf("the viewer moved to %s", r.app.Ctl.Path())
	}
	if got := r.app.Root.ToastText(); got != i18n.T("toast.copySaved", "photo (2).png") {
		t.Fatalf("toast %q", got)
	}
	if b := decodeFile(t, mustRead(t, r.src)).Bounds(); b.Dx() != 640 {
		t.Fatalf("the original changed: %v", b)
	}

	r.enter()
	r.cropTo(image.Rect(0, 0, 100, 100))
	r.app.saveEditCopy()
	r.waitFile("photo (3).png")
	r.waitEnded()
	eventually(t, r.app, "the copies are in the folder", func() bool { return r.app.Ctl.Navigator().Count() == 3 })
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestEnterSavesACopyFromTheKeyboard(t *testing.T) {
	r := newPNGEditRig(t, 640, 480)
	r.enter()
	r.key(widget.KeyEnter, 0)
	r.settle()
	if !r.missing("photo (2).png") {
		t.Fatal("an unchanged picture was saved")
	}
	r.cropTo(image.Rect(0, 0, 200, 100))
	r.key(widget.KeyEnter, 0)
	r.waitFile("photo (2).png")
	r.waitEnded()
}

func TestRotationAndFlipAreBakedIntoTheCopy(t *testing.T) {
	r := newPNGEditRig(t, 640, 480)
	r.enter()
	r.app.Commands.Run(CmdRotateRight)
	r.app.Commands.Run(CmdFlipH)
	r.app.saveEditCopy()
	data := r.waitFile("photo (2).png")
	img := decodeFile(t, data)
	if b := img.Bounds(); b.Dx() != 480 || b.Dy() != 640 {
		t.Fatalf("copy is %v", b)
	}
	want := imaging.Orient(gradientNRGBA(640, 480), codec.OrientNormal.RotateCW().FlipH())
	got := image.NewNRGBA(img.Bounds())
	draw.Draw(got, got.Bounds(), img, image.Point{}, draw.Src)
	if !bytes.Equal(got.Pix, want.Pix) {
		t.Fatal("the pixels differ from the source rotated right and mirrored")
	}
}

func TestSaveAsThroughTheFileDialog(t *testing.T) {
	r := newPNGEditRig(t, 640, 480)
	r.enter()
	r.cropTo(image.Rect(0, 0, 320, 240))
	r.dialog.path = filepath.Join(r.dir, "chosen.png")
	r.app.saveAs()
	data := r.waitFile("chosen.png")
	r.waitEnded()
	if b := decodeFile(t, data).Bounds(); b.Dx() != 320 || b.Dy() != 240 {
		t.Fatalf("saved %v", b)
	}
	if got := r.dialog.asked.Load(); got != 1 {
		t.Fatalf("asked %d times", got)
	}
}

func TestSaveAsSuggestsTheCopyName(t *testing.T) {
	r := newPNGEditRig(t, 640, 480)
	r.enter()
	r.cropTo(image.Rect(0, 0, 320, 240))
	var asked atomic.Pointer[string]
	r.app.Platform = platformSaving(func(name string) string { asked.Store(&name); return "" })
	r.app.saveAs()
	eventually(t, r.app, "the dialog was asked", func() bool { return asked.Load() != nil })
	if got := *asked.Load(); got != "photo (2).png" {
		t.Fatalf("suggested %q", got)
	}
}

func TestOverwriteAsksAndKeepsTheOriginalOnNo(t *testing.T) {
	r := newPNGEditRig(t, 640, 480)
	r.enter()
	r.cropTo(image.Rect(0, 0, 320, 240))
	before := mustRead(t, r.src)
	r.app.saveEditOverwrite()
	r.settle()
	if r.app.ovConfirm == nil {
		t.Fatal("no confirmation")
	}
	r.key(widget.KeyEscape, 0)
	r.settle()
	if r.app.ovConfirm != nil {
		t.Fatal("the confirmation stays")
	}
	if !r.app.Editing() {
		t.Fatal("declining must keep the editor")
	}
	if !bytes.Equal(before, mustRead(t, r.src)) {
		t.Fatal("the original was touched")
	}
}

func TestOverwriteReplacesTheOriginalAndReloads(t *testing.T) {
	r := newPNGEditRig(t, 640, 480)
	r.enter()
	r.cropTo(image.Rect(0, 0, 320, 240))
	r.app.saveEditOverwrite()
	r.settle()
	r.key(widget.KeyEnter, 0)
	r.waitEnded()
	eventually(t, r.app, "the new picture is shown", func() bool {
		return r.app.Ctl.Status() == StatusReady && r.app.Ctl.Info().Width == 320
	})
	if b := decodeFile(t, mustRead(t, r.src)).Bounds(); b.Dx() != 320 || b.Dy() != 240 {
		t.Fatalf("original is now %v", b)
	}
	if r.app.Root.ToastText() != i18n.T("toast.overwritten") {
		t.Fatalf("toast %q", r.app.Root.ToastText())
	}
	entries, _ := os.ReadDir(r.dir)
	if len(entries) != 1 {
		t.Fatalf("stray files: %v", entries)
	}
}

func TestOverwriteRemembersDontAskAgain(t *testing.T) {
	r := newPNGEditRig(t, 640, 480)
	r.enter()
	r.cropTo(image.Rect(0, 0, 320, 240))
	r.app.saveEditOverwrite()
	r.settle()
	r.key(widget.KeyTab, widget.ModShift)
	r.key(widget.KeySpace, 0)
	r.key(widget.KeyEnter, 0)
	r.waitEnded()
	if r.app.Settings.Get().ConfirmOverwrite {
		t.Fatal("the choice was not remembered")
	}
	eventually(t, r.app, "reloaded", func() bool { return r.app.Ctl.Status() == StatusReady && r.app.Ctl.Info().Width == 320 })
	r.enter()
	r.cropTo(image.Rect(0, 0, 100, 100))
	r.app.saveEditOverwrite()
	r.waitEnded()
	if r.app.ovConfirm != nil {
		t.Fatal("asked again")
	}
	eventually(t, r.app, "reloaded again", func() bool { return r.app.Ctl.Status() == StatusReady && r.app.Ctl.Info().Width == 100 })
}

func TestOverwriteWithoutConfirmationSettingWritesAtOnce(t *testing.T) {
	r := newEditRig(t, "photo.png", func(t *testing.T, p string) { writePNGFile(t, p, 200, 100) }, func(s *settings.Settings) { s.ConfirmOverwrite = false })
	r.enter()
	r.cropTo(image.Rect(0, 0, 50, 50))
	r.app.saveEditOverwrite()
	r.waitEnded()
	if b := decodeFile(t, mustRead(t, r.src)).Bounds(); b.Dx() != 50 {
		t.Fatalf("original %v", b)
	}
}

func TestFormatsWithoutAnEncoderFallBackToPNGAndCannotBeOverwritten(t *testing.T) {
	r := newEditRig(t, "still.gif", func(t *testing.T, p string) {
		writeStillGIF(t, p)
	}, nil)
	r.enter()
	r.cropTo(image.Rect(0, 0, 32, 32))
	if r.app.canOverwrite() {
		t.Fatal("a GIF cannot be written")
	}
	r.app.saveEditOverwrite()
	r.settle()
	if r.app.Root.ToastText() != i18n.T("edit.overwriteOff") {
		t.Fatalf("toast %q", r.app.Root.ToastText())
	}
	r.app.saveEditCopy()
	data := r.waitFile("still (2).png")
	if b := decodeFile(t, data).Bounds(); b.Dx() != 32 {
		t.Fatalf("copy %v", b)
	}
}

func TestResizeThroughTheDialogShrinksTheCopy(t *testing.T) {
	r := newPNGEditRig(t, 640, 480)
	r.app.Commands.Run(CmdResize)
	r.settle()
	dlg := r.app.resizeDlg
	if dlg == nil {
		t.Fatal("no resize dialog")
	}
	for _, ch := range "320" {
		r.app.Eng.SendKeyEvent(widget.KeyEvent{Code: widget.KeyCode(ch), Rune: ch, Pressed: true})
	}
	r.app.Eng.SendKeyEvent(widget.KeyEvent{Code: widget.KeyEnter, Pressed: true})
	r.settle()
	if r.app.resizeDlg != nil {
		t.Fatal("dialog stays")
	}
	if got := r.app.Editor.OutputSize(); got != image.Pt(320, 240) {
		t.Fatalf("output %v", got)
	}
	if !r.app.Editor.Dirty() {
		t.Fatal("a resize is a change")
	}
	r.app.saveEditCopy()
	data := r.waitFile("photo (2).png")
	if b := decodeFile(t, data).Bounds(); b.Dx() != 320 || b.Dy() != 240 {
		t.Fatalf("copy %v", b)
	}
}

func TestResizeAfterCropWorksOnTheCroppedSize(t *testing.T) {
	r := newPNGEditRig(t, 640, 480)
	r.enter()
	r.cropTo(image.Rect(0, 0, 400, 200))
	r.app.openResize()
	r.settle()
	if got := r.app.resizeDlg.Model().Orig(); got != image.Pt(400, 200) {
		t.Fatalf("original %v", got)
	}
}

func TestEscapeInTheResizeDialogKeepsTheEditor(t *testing.T) {
	r := newPNGEditRig(t, 640, 480)
	r.app.Commands.Run(CmdResize)
	r.settle()
	r.app.Eng.SendKeyEvent(widget.KeyEvent{Code: widget.KeyEscape, Pressed: true})
	r.settle()
	if r.app.resizeDlg != nil || !r.app.Editing() || r.app.Editor.Dirty() {
		t.Fatalf("dialog %v editing %v dirty %v", r.app.resizeDlg != nil, r.app.Editing(), r.app.Editor.Dirty())
	}
}

func TestLosslessCopySnapsTheCropAndKeepsMetadata(t *testing.T) {
	r := newJPEGEditRig(t, 640, 480, nil)
	r.enter()
	r.cropTo(image.Rect(21, 22, 301, 203))
	r.app.saveEditCopy()
	r.waitDialog()
	d := r.app.saveDlg
	if d.Kind() != saveopts.JPEGLossless || !d.Values().Lossless {
		t.Fatalf("kind %v values %+v", d.Kind(), d.Values())
	}
	snap := r.app.Editor.Snapped()
	if snap.Empty() || snap.Max != image.Pt(301, 203) || snap.Min.X > 21 || snap.Min.Y > 22 || snap.Min.X%16 != 0 || snap.Min.Y%16 != 0 {
		t.Fatalf("snapped rectangle %v", snap)
	}
	r.key(widget.KeyEnter, 0)
	data := r.waitFile("photo (2).jpg")
	r.waitEnded()
	if b := decodeFile(t, data).Bounds(); b.Dx() != snap.Dx() || b.Dy() != snap.Dy() {
		t.Fatalf("lossless copy is %v, the overlay promised %v", b, snap)
	}
	if !r.app.Editor.Snapped().Empty() {
		t.Fatal("the snapped rectangle must disappear with the dialog")
	}
	m := blocksOf(t, data, codec.JPEG)
	if len(m.EXIF) == 0 || !bytes.Contains(m.EXIF, []byte("Cam")) || !hasGPS(t, m.EXIF) {
		t.Fatalf("EXIF lost: %d bytes, gps %v", len(m.EXIF), hasGPS(t, m.EXIF))
	}
}

func TestLosslessCopyCanDropTheLocation(t *testing.T) {
	r := newJPEGEditRig(t, 640, 480, nil)
	r.enter()
	r.cropTo(image.Rect(32, 32, 320, 240))
	r.app.saveEditCopy()
	r.waitDialog()
	r.key(widget.KeyTab, 0)
	r.key(widget.KeySpace, 0)
	if !r.app.saveDlg.Values().StripGPS {
		t.Fatal("the GPS box did not turn on")
	}
	r.key(widget.KeyEnter, 0)
	data := r.waitFile("photo (2).jpg")
	if b := decodeFile(t, data).Bounds(); b.Dx() != 288 || b.Dy() != 208 {
		t.Fatalf("copy %v", b)
	}
	m := blocksOf(t, data, codec.JPEG)
	if !bytes.Contains(m.EXIF, []byte("Cam")) || hasGPS(t, m.EXIF) {
		t.Fatalf("EXIF %d bytes gps %v", len(m.EXIF), hasGPS(t, m.EXIF))
	}
	r.settle()
	if !r.app.Settings.Get().SaveStripGPS {
		t.Fatal("the choice was not remembered")
	}
}

func TestReencodedCopyKeepsMetadataAndFixesTheSize(t *testing.T) {
	r := newJPEGEditRig(t, 640, 480, nil)
	r.enter()
	r.cropTo(image.Rect(21, 22, 301, 203))
	r.app.saveEditCopy()
	r.waitDialog()
	r.key(widget.KeySpace, 0)
	if r.app.saveDlg.Values().Lossless {
		t.Fatal("the switch did not turn off")
	}
	if !r.app.Editor.Snapped().Empty() {
		t.Fatal("the snapped rectangle must go with the switch")
	}
	r.key(widget.KeyEnter, 0)
	data := r.waitFile("photo (2).jpg")
	if b := decodeFile(t, data).Bounds(); b.Dx() != 280 || b.Dy() != 181 {
		t.Fatalf("copy %v, want exactly the crop", b)
	}
	m := blocksOf(t, data, codec.JPEG)
	if !bytes.Contains(m.EXIF, []byte("Cam")) || !hasGPS(t, m.EXIF) {
		t.Fatalf("EXIF lost")
	}
}

func TestReencodedCopyStripsLocationOnRequest(t *testing.T) {
	r := newJPEGEditRig(t, 640, 480, nil)
	r.enter()
	r.cropTo(image.Rect(21, 22, 301, 203))
	r.app.saveEditCopy()
	r.waitDialog()
	r.key(widget.KeySpace, 0)
	r.key(widget.KeyTab, 0)
	r.key(widget.KeySpace, 0)
	r.key(widget.KeyEnter, 0)
	data := r.waitFile("photo (2).jpg")
	m := blocksOf(t, data, codec.JPEG)
	if !bytes.Contains(m.EXIF, []byte("Cam")) || hasGPS(t, m.EXIF) {
		t.Fatalf("EXIF %d bytes gps %v", len(m.EXIF), hasGPS(t, m.EXIF))
	}
}

func TestResizedJPEGCannotBeLossless(t *testing.T) {
	r := newJPEGEditRig(t, 640, 480, nil)
	r.enter()
	r.app.Editor.SetOutputSize(320, 240)
	r.app.saveEditCopy()
	r.waitDialog()
	if r.app.saveDlg.Kind() != saveopts.JPEG {
		t.Fatalf("kind %v", r.app.saveDlg.Kind())
	}
	r.key(widget.KeyEnter, 0)
	data := r.waitFile("photo (2).jpg")
	if b := decodeFile(t, data).Bounds(); b.Dx() != 320 || b.Dy() != 240 {
		t.Fatalf("copy %v", b)
	}
	if m := blocksOf(t, data, codec.JPEG); !bytes.Contains(m.EXIF, []byte("Cam")) {
		t.Fatal("EXIF lost")
	}
}

func TestCancellingTheQualityDialogKeepsEditing(t *testing.T) {
	r := newJPEGEditRig(t, 640, 480, nil)
	r.enter()
	r.cropTo(image.Rect(32, 32, 320, 240))
	r.app.saveEditCopy()
	r.waitDialog()
	r.key(widget.KeyEscape, 0)
	eventually(t, r.app, "dialog gone", func() bool { return r.app.saveDlg == nil })
	if !r.app.Editing() || !r.missing("photo (2).jpg") {
		t.Fatalf("editing %v", r.app.Editing())
	}
	if !r.app.Editor.Snapped().Empty() {
		t.Fatal("the snapped rectangle stays")
	}
}

func TestOrdinarySaveAsCarriesTheMetadata(t *testing.T) {
	r := newJPEGEditRig(t, 640, 480, nil)
	r.dialog.path = filepath.Join(r.dir, "out.png")
	r.app.saveAs()
	data := r.waitFile("out.png")
	m := blocksOf(t, data, codec.PNG)
	if !bytes.Contains(m.EXIF, []byte("Cam")) || !hasGPS(t, m.EXIF) {
		t.Fatalf("EXIF %d bytes", len(m.EXIF))
	}
}

func TestOrdinarySaveAsStripsLocationFromTheSetting(t *testing.T) {
	r := newJPEGEditRig(t, 640, 480, func(s *settings.Settings) { s.SaveStripGPS = true })
	r.dialog.path = filepath.Join(r.dir, "out.png")
	r.app.saveAs()
	data := r.waitFile("out.png")
	m := blocksOf(t, data, codec.PNG)
	if !bytes.Contains(m.EXIF, []byte("Cam")) || hasGPS(t, m.EXIF) {
		t.Fatalf("EXIF %d bytes gps %v", len(m.EXIF), hasGPS(t, m.EXIF))
	}
}

func TestOrdinarySaveAsOfRotatedPictureResetsTheOrientation(t *testing.T) {
	r := newJPEGEditRig(t, 640, 480, nil)
	r.app.Canvas.RotateRight()
	r.dialog.path = filepath.Join(r.dir, "turned.png")
	r.app.saveAs()
	data := r.waitFile("turned.png")
	if b := decodeFile(t, data).Bounds(); b.Dx() != 480 || b.Dy() != 640 {
		t.Fatalf("size %v", b)
	}
	m := blocksOf(t, data, codec.PNG)
	if len(m.EXIF) == 0 {
		t.Fatal("no EXIF")
	}
}

func TestTiledPictureCropsFromTheFile(t *testing.T) {
	r := newEditRig(t, "huge.jpg", func(t *testing.T, p string) { writeGradientJPEG(t, p, 4800, 3600) }, func(s *settings.Settings) { s.MaxPixels = 4 })
	shown := r.app.Ctl.Shown()
	if shown == nil || shown.FullResolution() || shown.Detail == nil {
		t.Fatalf("not tiled: %+v", shown)
	}
	r.enter()
	r.cropTo(image.Rect(100, 100, 1100, 900))
	r.app.saveEditCopy()
	r.waitDialog()
	r.key(widget.KeySpace, 0)
	r.key(widget.KeyEnter, 0)
	data := r.waitFile("huge (2).jpg")
	if b := decodeFile(t, data).Bounds(); b.Dx() != 1000 || b.Dy() != 800 {
		t.Fatalf("copy %v", b)
	}
}

func TestTiledPictureLosslessCropStillWorks(t *testing.T) {
	r := newEditRig(t, "huge.jpg", func(t *testing.T, p string) { writeGradientJPEG(t, p, 4800, 3600) }, func(s *settings.Settings) { s.MaxPixels = 4 })
	r.enter()
	r.cropTo(image.Rect(160, 160, 2400, 1600))
	r.app.saveEditCopy()
	r.waitDialog()
	r.key(widget.KeyEnter, 0)
	data := r.waitFile("huge (2).jpg")
	if b := decodeFile(t, data).Bounds(); b.Dx() != 2240 || b.Dy() != 1440 {
		t.Fatalf("copy %v", b)
	}
}

func TestTiledPictureTooLargeToRecodeIsRefused(t *testing.T) {
	r := newEditRig(t, "huge.jpg", func(t *testing.T, p string) { writeGradientJPEG(t, p, 4800, 3600) }, func(s *settings.Settings) { s.MaxPixels = 4 })
	r.enter()
	r.cropTo(image.Rect(0, 0, 4000, 3000))
	r.app.saveEditCopy()
	r.waitDialog()
	r.key(widget.KeySpace, 0)
	r.key(widget.KeyEnter, 0)
	eventually(t, r.app, "the refusal is shown", func() bool { return messageShown(r.app, i18n.T("error.editTooLarge")) })
	if !r.missing("huge (2).jpg") {
		t.Fatal("a file was written")
	}
	if !r.app.Editing() || r.app.Editor.Busy() {
		t.Fatalf("editing %v busy %v", r.app.Editing(), r.app.Editor.Busy())
	}
}

func TestTiledPictureResizedFromTheOverview(t *testing.T) {
	r := newEditRig(t, "huge.jpg", func(t *testing.T, p string) { writeGradientJPEG(t, p, 4800, 3600) }, func(s *settings.Settings) { s.MaxPixels = 4 })
	r.enter()
	r.app.Editor.SetOutputSize(1200, 900)
	r.app.saveEditCopy()
	r.waitDialog()
	r.key(widget.KeyEnter, 0)
	data := r.waitFile("huge (2).jpg")
	if b := decodeFile(t, data).Bounds(); b.Dx() != 1200 || b.Dy() != 900 {
		t.Fatalf("copy %v", b)
	}
}

func TestBusyEditorIgnoresASecondSave(t *testing.T) {
	r := newPNGEditRig(t, 640, 480)
	r.enter()
	r.cropTo(image.Rect(0, 0, 100, 100))
	r.app.Editor.SetBusy(true)
	r.app.saveEditCopy()
	r.settle()
	if !r.missing("photo (2).png") {
		t.Fatal("saved while busy")
	}
}

func TestCopyNameSkipsExistingEntries(t *testing.T) {
	r := newPNGEditRig(t, 640, 480)
	r.enter()
	r.cropTo(image.Rect(0, 0, 100, 100))
	if err := os.Mkdir(filepath.Join(r.dir, "photo (2).png"), 0o755); err != nil {
		t.Fatal(err)
	}
	r.app.saveEditCopy()
	r.waitFile("photo (3).png")
}

func TestEditOfAnExifRotatedPictureBakesTheOrientation(t *testing.T) {
	r := newEditRig(t, "photo.jpg", func(t *testing.T, p string) { writeOrientedJPEG(t, p, 640, 480, 6) }, nil)
	if r.app.Ctl.Info().Orientation != codec.OrientRotate90 {
		t.Fatalf("source orientation %v", r.app.Ctl.Info().Orientation)
	}
	r.enter()
	if r.app.Editor.Crop().Size() != image.Pt(480, 640) {
		t.Fatalf("crop size %v", r.app.Editor.Crop().Size())
	}
	r.cropTo(image.Rect(0, 0, 240, 320))
	r.app.saveEditCopy()
	r.waitDialog()
	r.key(widget.KeySpace, 0)
	r.key(widget.KeyEnter, 0)
	data := r.waitFile("photo (2).jpg")
	if b := decodeFile(t, data).Bounds(); b.Dx() != 240 || b.Dy() != 320 {
		t.Fatalf("copy %v", b)
	}
	if got := orientationOf(blocksOf(t, data, codec.JPEG).EXIF); got != 1 {
		t.Fatalf("EXIF orientation %d, the pixels are already turned", got)
	}
}

func TestLosslessEditOfAnExifRotatedPicture(t *testing.T) {
	r := newEditRig(t, "photo.jpg", func(t *testing.T, p string) { writeOrientedJPEG(t, p, 640, 480, 6) }, nil)
	r.enter()
	r.cropTo(image.Rect(0, 0, 240, 320))
	r.app.saveEditCopy()
	r.waitDialog()
	r.key(widget.KeyEnter, 0)
	data := r.waitFile("photo (2).jpg")
	if b := decodeFile(t, data).Bounds(); b.Dx() != 240 || b.Dy() != 320 {
		t.Fatalf("copy %v", b)
	}
	if got := orientationOf(blocksOf(t, data, codec.JPEG).EXIF); got != 1 {
		t.Fatalf("EXIF orientation %d", got)
	}
}
