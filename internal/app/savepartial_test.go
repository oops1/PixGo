package app

import (
	"bytes"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/imaging"
	"github.com/oops1/PixGo/internal/settings"
	"github.com/oops1/PixGo/internal/ui/saveopts"
)

func writeGradientJPEG(t *testing.T, path string, w, h int) {
	t.Helper()
	img := image.NewYCbCr(image.Rect(0, 0, w, h), image.YCbCrSubsampleRatio420)
	for y := 0; y < h; y++ {
		row := img.Y[y*img.YStride : y*img.YStride+w]
		for x := range row {
			row[x] = uint8((x/16 + y/16) * 5)
		}
	}
	for i := range img.Cb {
		img.Cb[i], img.Cr[i] = 120, 140
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func newTiledJPEGRig(t *testing.T, withWebP bool) *saveRig {
	t.Helper()
	r := buildSaveRig(t, func(s *settings.Settings) { s.MaxPixels = 4 }, withWebP, "huge.jpg", func(t *testing.T, path string) {
		writeGradientJPEG(t, path, 4800, 3600)
	})
	shown := r.app.Ctl.Shown()
	if shown == nil || shown.FullResolution() || shown.Detail == nil {
		t.Fatalf("the 17 Mpx picture was not opened in tiles: %+v", shown)
	}
	return r
}

func (r *saveRig) showReduced(t *testing.T) {
	t.Helper()
	shown := r.app.Ctl.Shown()
	r.app.Ctl.shown = &imaging.Decoded{Key: shown.Key, Info: shown.Info, Levels: shown.Levels, Shift: 1, Reduced: true}
	if r.app.Ctl.Decoded() != nil || r.app.orientedFrame() != nil {
		t.Fatal("a reduced picture must not give a pixel frame")
	}
}

func (r *saveRig) refused(t *testing.T, key string) {
	t.Helper()
	eventually(t, r.app, "the refusal is shown", func() bool { return messageShown(r.app, i18n.T(key)) })
}

func TestHugeJPEGShownInTilesSavesLosslesslyFromTheFile(t *testing.T) {
	r := newTiledJPEGRig(t, false)
	r.drawn().Canvas.RotateRight()

	r.saveTo(t, "out.jpg")
	d := r.waitDialog(t)
	if d.Kind() != saveopts.JPEGLossless || !d.Values().Lossless || d.Enabled(saveopts.PartSlider) {
		t.Fatalf("kind %v values %+v", d.Kind(), d.Values())
	}
	r.key(widget.KeyEnter)

	data := r.waitFile(t, "out.jpg")
	if b := decodeJPEGFile(t, data).Bounds(); b.Dx() != 3600 || b.Dy() != 4800 {
		t.Fatalf("size %v, want 3600x4800", b)
	}
	if r.jpeg.count() != 0 {
		t.Fatal("a downscaled frame was encoded")
	}
}

func TestReducedPreviewStillSavesLossless(t *testing.T) {
	r := newJPEGSourceRig(t, nil)
	r.showReduced(t)

	data := r.confirmLossless(t, "out.jpg")

	if b := decodeJPEGFile(t, data).Bounds(); b.Dx() != 600 || b.Dy() != 400 {
		t.Fatalf("size %v", b)
	}
	if r.jpeg.count() != 0 {
		t.Fatal("re-encoded")
	}
}

func TestPartialPictureIsNotSavedInOtherFormats(t *testing.T) {
	for _, name := range []string{"out.png", "out.bmp", "out.webp"} {
		t.Run(name, func(t *testing.T) {
			r := buildSaveRig(t, nil, true, "photo.jpg", writeBigJPEG)
			r.showReduced(t)

			r.saveTo(t, name)

			r.refused(t, "error.partialSave")
			r.settle()
			if r.app.saveDlg != nil {
				t.Fatal("a dialog was opened for a format that cannot be written")
			}
			if _, err := os.Stat(filepath.Join(r.dir, name)); err == nil {
				t.Fatal("a file was written")
			}
			if r.png.count()+r.bmp.count()+r.webp.count()+r.jpeg.count() != 0 {
				t.Fatal("an encoder ran on a partial picture")
			}
		})
	}
}

func TestHugeJPEGIsNotSavedAsPNG(t *testing.T) {
	r := newTiledJPEGRig(t, false)

	r.saveTo(t, "out.png")

	r.refused(t, "error.partialSave")
	if r.png.count() != 0 {
		t.Fatal("PNG was encoded from a downscaled frame")
	}
}

func TestPartialPictureDialogStartsLosslessAndRefusesToEncodeWithoutIt(t *testing.T) {
	r := newJPEGSourceRig(t, func(s *settings.Settings) { s.SaveJPEGLossless = false })
	r.showReduced(t)
	r.saveTo(t, "out.jpg")
	d := r.waitDialog(t)
	if !d.Values().Lossless {
		t.Fatal("the dialog must start lossless: nothing else can be saved")
	}

	r.key(widget.KeySpace)
	r.key(widget.KeyEnter)

	r.refused(t, "error.partialSave")
	r.settle()
	if _, err := os.Stat(filepath.Join(r.dir, "out.jpg")); err == nil {
		t.Fatal("a file was written")
	}
	if r.jpeg.count() != 0 {
		t.Fatal("a reduced frame was encoded")
	}
	if r.app.Settings.Get().SaveJPEGLossless {
		t.Fatal("a refused choice must not be remembered")
	}
}

func TestFailedLosslessCopyOfAPartialPictureDoesNotFallBackToPixels(t *testing.T) {
	r := newJPEGSourceRig(t, nil)
	r.showReduced(t)
	r.saveTo(t, "out.jpg")
	r.waitDialog(t)
	writeJPEGSized(t, r.src, 50, 30)
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(r.src, later, later); err != nil {
		t.Fatal(err)
	}

	r.key(widget.KeyEnter)

	r.refused(t, "error.partialSave")
	if _, err := os.Stat(filepath.Join(r.dir, "out.jpg")); err == nil {
		t.Fatal("a file was written")
	}
	if r.jpeg.count() != 0 {
		t.Fatal("the fallback encoded a reduced frame")
	}
}

func TestPartialPictureThatCannotBeCopiedLosslesslyIsRefusedBeforeAnyDialog(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "jpegxform", "testdata", "video-001.q50.420.progressive.jpeg"))
	if err != nil {
		t.Fatal(err)
	}
	r := buildSaveRig(t, nil, false, "photo.jpg", func(t *testing.T, path string) {
		if err := os.WriteFile(path, src, 0o644); err != nil {
			t.Fatal(err)
		}
	})
	r.showReduced(t)

	r.saveTo(t, "out.jpg")

	r.refused(t, "error.partialSaveNone")
	if r.dialog.asked.Load() != 0 || r.app.saveDlg != nil {
		t.Fatalf("the file chooser was asked %d times", r.dialog.asked.Load())
	}
}

func TestCopyingAPartialPictureSaysSo(t *testing.T) {
	r := newJPEGSourceRig(t, nil)
	r.showReduced(t)

	r.app.copyImage()

	r.refused(t, "error.partialCopy")
}

func TestNothingShownStaysSilent(t *testing.T) {
	r := newJPEGSourceRig(t, nil)
	r.app.Ctl.Removed(r.src)
	r.app.copyImage()
	r.saveTo(t, "out.jpg")
	r.settle()

	if r.dialog.asked.Load() != 0 || messageShown(r.app, i18n.T("error.partialSave")) || messageShown(r.app, i18n.T("error.partialSaveNone")) || messageShown(r.app, i18n.T("error.partialCopy")) {
		t.Fatal("an empty viewer must not complain about partial pictures")
	}
}

func TestPartialPictureMessagesExistInBothLanguages(t *testing.T) {
	for _, lang := range []string{"ru", "en"} {
		i18n.SetLang(lang)
		for _, key := range []string{"error.partialSave", "error.partialSaveNone", "error.partialCopy"} {
			if i18n.T(key) == key || i18n.T(key) == "" {
				t.Errorf("%s: %q has no text", lang, key)
			}
		}
	}
}
