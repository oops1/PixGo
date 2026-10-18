package app

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/imaging"
	"github.com/oops1/PixGo/internal/jpegxform"
	"github.com/oops1/PixGo/internal/meta/exif"
	"github.com/oops1/PixGo/internal/settings"
	"github.com/oops1/PixGo/internal/ui/saveopts"
)

func decodeJPEGFile(t *testing.T, data []byte) image.Image {
	t.Helper()
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("the written file is not a JPEG: %v", err)
	}
	return img
}

func nrgbaFrom(img image.Image) *image.NRGBA {
	b := img.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			c := color.NRGBAModel.Convert(img.At(b.Min.X+x, b.Min.Y+y)).(color.NRGBA)
			out.SetNRGBA(x, y, c)
		}
	}
	return out
}

func maxChannelDiff(a, b *image.NRGBA) int {
	worst := 0
	for i := range a.Pix {
		d := int(a.Pix[i]) - int(b.Pix[i])
		if d < 0 {
			d = -d
		}
		worst = max(worst, d)
	}
	return worst
}

func (r *saveRig) confirmLossless(t *testing.T, name string) []byte {
	t.Helper()
	r.saveTo(t, name)
	r.waitDialog(t)
	r.key(widget.KeyEnter)
	return r.waitFile(t, name)
}

func (r *saveRig) drawn() *App {
	r.app.Eng.Invalidate()
	r.app.Eng.Flush()
	r.app.Eng.RenderOnce()
	return r.app
}

func labelTexts(a *App) []string {
	var out []string
	var walk func(n *widget.AccessNode)
	walk = func(n *widget.AccessNode) {
		if n.Role == widget.RoleLabel {
			out = append(out, n.Name)
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(a.Eng.AccessibilityTree())
	return out
}

func toastShown(a *App, text string) bool {
	for _, l := range labelTexts(a) {
		if l == text {
			return true
		}
	}
	return false
}

func messageShown(a *App, text string) bool {
	joined := strings.Join(strings.Fields(strings.Join(labelTexts(a), " ")), " ")
	return strings.Contains(joined, strings.Join(strings.Fields(text), " "))
}

func TestJPEGSourceOffersLosslessSwitchedOnByDefault(t *testing.T) {
	r := newJPEGSourceRig(t, nil)

	r.saveTo(t, "out.jpg")
	d := r.waitDialog(t)

	if d.Kind() != saveopts.JPEGLossless || d.Values() != (saveopts.Values{Quality: 90, Lossless: true}) {
		t.Fatalf("kind %v values %+v", d.Kind(), d.Values())
	}
	if d.Rect(saveopts.PartCheck).Empty() || d.Enabled(saveopts.PartSlider) {
		t.Fatalf("the switch must be there and the slider off")
	}
}

func TestLosslessSaveRewritesTheFileWithoutReencoding(t *testing.T) {
	r := newJPEGSourceRig(t, nil)

	data := r.confirmLossless(t, "out.jpg")

	img := decodeJPEGFile(t, data)
	if b := img.Bounds(); b.Dx() != 600 || b.Dy() != 400 {
		t.Fatalf("size %v", b)
	}
	if r.jpeg.count() != 0 {
		t.Fatal("the encoder ran although the save was lossless")
	}
	src, _ := os.ReadFile(r.src)
	if worst := maxChannelDiff(nrgbaFrom(decodeJPEGFile(t, src)), nrgbaFrom(img)); worst != 0 {
		t.Fatalf("pixels changed by %d", worst)
	}
	if len(data) > len(src)+len(src)/50 {
		t.Fatalf("%d bytes from %d", len(data), len(src))
	}
	r.settle()
	st := r.app.Settings.Get()
	if !st.SaveJPEGLossless || st.SaveJPEGQuality != 90 {
		t.Fatalf("remembered lossless %v quality %d", st.SaveJPEGLossless, st.SaveJPEGQuality)
	}
	if !waitToast(r.app, i18n.T("toast.saved")) {
		t.Fatal("no confirmation toast")
	}
}

func TestRotatedPictureIsSavedLosslessAndMatchesWhatIsShown(t *testing.T) {
	r := newJPEGSourceRig(t, nil)
	r.drawn().Canvas.RotateRight()

	data := r.confirmLossless(t, "out.jpg")

	got := nrgbaFrom(decodeJPEGFile(t, data))
	src, _ := os.ReadFile(r.src)
	want := imaging.Orient(nrgbaFrom(decodeJPEGFile(t, src)), r.app.Canvas.Orientation())
	if got.Bounds() != want.Bounds() || got.Bounds().Dx() != 400 || got.Bounds().Dy() != 600 {
		t.Fatalf("got %v want %v", got.Bounds(), want.Bounds())
	}
	if worst := maxChannelDiff(got, want); worst > 4 {
		t.Fatalf("pixels differ by %d from the picture on the screen", worst)
	}
	if r.jpeg.count() != 0 {
		t.Fatal("the encoder ran although the save was lossless")
	}
}

func TestEveryRotationAndFlipSavesLosslesslyWithTheShownOrientation(t *testing.T) {
	moves := map[string]func(*App){
		"rotate right": func(a *App) { a.Canvas.RotateRight() },
		"rotate left":  func(a *App) { a.Canvas.RotateLeft() },
		"flip h":       func(a *App) { a.Canvas.FlipH() },
		"flip v":       func(a *App) { a.Canvas.FlipV() },
		"half turn":    func(a *App) { a.Canvas.RotateRight(); a.Canvas.RotateRight() },
		"transpose":    func(a *App) { a.Canvas.RotateRight(); a.Canvas.FlipH() },
		"transverse":   func(a *App) { a.Canvas.RotateLeft(); a.Canvas.FlipH() },
	}
	for name, move := range moves {
		t.Run(name, func(t *testing.T) {
			r := newJPEGSourceRig(t, nil)
			move(r.drawn())
			o := r.app.Canvas.Orientation()

			data := r.confirmLossless(t, "out.jpg")

			got := nrgbaFrom(decodeJPEGFile(t, data))
			src, _ := os.ReadFile(r.src)
			full := imaging.Orient(nrgbaFrom(decodeJPEGFile(t, src)), o)
			ow, oh := o.Size(600, 400)
			if got.Bounds().Dx() > ow || got.Bounds().Dy() > oh || got.Bounds().Dx() < ow-15 || got.Bounds().Dy() < oh-15 {
				t.Fatalf("got %v from %dx%d", got.Bounds(), ow, oh)
			}
			if r.jpeg.count() != 0 {
				t.Fatal("re-encoded")
			}
			if jpegxform.FromOrientation(o) == jpegxform.Identity && got.Bounds() != full.Bounds() {
				t.Fatalf("identity changed the size to %v", got.Bounds())
			}
		})
	}
}

func TestImperfectSizeIsTrimmedAndTheToastSaysSo(t *testing.T) {
	r := newJPEGSourceRig(t, nil)
	r.drawn().Canvas.FlipH()

	data := r.confirmLossless(t, "out.jpg")

	b := decodeJPEGFile(t, data).Bounds()
	if b.Dx() != 592 || b.Dy() != 400 {
		t.Fatalf("size %v: 600 is not a multiple of the 16 px block, so 8 px must go", b)
	}
	r.settle()
	if !waitToast(r.app, i18n.T("toast.savedTrimmed")) || toastShown(r.app, i18n.T("toast.saved")) {
		t.Fatal("the toast must tell that the edge was trimmed")
	}
}

func TestSaveOutcomeChoosesTheToast(t *testing.T) {
	if (saveOutcome{}).toastKey() != "toast.saved" || (saveOutcome{trimmed: true}).toastKey() != "toast.savedTrimmed" {
		t.Fatal("wrong toast keys")
	}
	for _, lang := range []string{"ru", "en"} {
		i18n.SetLang(lang)
		for _, key := range []string{"toast.saved", "toast.savedTrimmed", "save.options.jpegLosslessHint"} {
			if i18n.T(key) == key {
				t.Errorf("%s: key %q has no text", lang, key)
			}
		}
	}
}

func TestSwitchingLosslessOffEncodesWithTheChosenQualityAndRemembersIt(t *testing.T) {
	r := newJPEGSourceRig(t, nil)
	r.saveTo(t, "out.jpg")
	r.waitDialog(t)

	r.key(widget.KeySpace)
	r.key(widget.KeyTab)
	r.key(widget.KeyTab)
	r.key(widget.KeyLeft)
	r.key(widget.KeyEnter)

	if got := r.waitFile(t, "out.jpg"); string(got) != "ENC-jpeg" {
		t.Fatalf("file content %q", got)
	}
	if call := r.jpeg.last(); !sameOptions(call.opts, (codec.EncodeOptions{Quality: 89})) {
		t.Fatalf("encoded with %+v", call.opts)
	}
	r.settle()
	st := r.app.Settings.Get()
	if st.SaveJPEGLossless || st.SaveJPEGQuality != 89 {
		t.Fatalf("remembered lossless %v quality %d", st.SaveJPEGLossless, st.SaveJPEGQuality)
	}

	r.saveTo(t, "again.jpg")
	if got := r.waitDialog(t).Values(); got != (saveopts.Values{Quality: 89}) {
		t.Fatalf("the next dialog starts with %+v", got)
	}
}

func TestStoredLosslessChoiceDecidesTheDialogStart(t *testing.T) {
	r := newJPEGSourceRig(t, func(s *settings.Settings) { s.SaveJPEGLossless = false })

	r.saveTo(t, "out.jpg")

	d := r.waitDialog(t)
	if d.Values().Lossless || !d.Enabled(saveopts.PartSlider) {
		t.Fatalf("values %+v", d.Values())
	}
}

func TestOtherSourcesAndTargetsKeepTheOrdinaryDialog(t *testing.T) {
	png := newSaveRig(t, nil, true)
	png.saveTo(t, "out.jpg")
	if d := png.waitDialog(t); d.Kind() != saveopts.JPEG || !d.Rect(saveopts.PartCheck).Empty() {
		t.Fatalf("a PNG source must not offer lossless JPEG: %v", d.Kind())
	}

	jp := newJPEGSourceRig(t, nil)
	jp.saveTo(t, "out.png")
	jp.waitFile(t, "out.png")
	if jp.app.saveDlg != nil || jp.png.count() != 1 {
		t.Fatal("PNG from a JPEG source is written without a dialog by the encoder")
	}
}

func TestWebPTargetFromJPEGSourceStaysWebPDialog(t *testing.T) {
	r := buildSaveRig(t, nil, true, "photo.jpg", writeBigJPEG)

	r.saveTo(t, "out.webp")

	if d := r.waitDialog(t); d.Kind() != saveopts.WebP {
		t.Fatalf("kind %v", d.Kind())
	}
}

func TestProgressiveJPEGDoesNotOfferTheSwitch(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "jpegxform", "testdata", "video-001.q50.420.progressive.jpeg"))
	if err != nil {
		t.Fatal(err)
	}
	r := buildSaveRig(t, nil, false, "photo.jpg", func(t *testing.T, path string) {
		if err := os.WriteFile(path, src, 0o644); err != nil {
			t.Fatal(err)
		}
	})

	r.saveTo(t, "out.jpg")

	d := r.waitDialog(t)
	if d.Kind() != saveopts.JPEG || !d.Rect(saveopts.PartCheck).Empty() {
		t.Fatalf("a progressive file cannot be rotated losslessly, kind %v", d.Kind())
	}
	r.key(widget.KeyEnter)
	if got := r.waitFile(t, "out.jpg"); string(got) != "ENC-jpeg" {
		t.Fatalf("content %q", got)
	}
}

func TestSourceChangedAfterTheDialogOpenedFallsBackToTheShownPicture(t *testing.T) {
	r := newJPEGSourceRig(t, nil)
	r.saveTo(t, "out.jpg")
	r.waitDialog(t)
	writeJPEGSized(t, r.src, 50, 30)
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(r.src, later, later); err != nil {
		t.Fatal(err)
	}

	r.key(widget.KeyEnter)

	if got := r.waitFile(t, "out.jpg"); string(got) != "ENC-jpeg" {
		t.Fatalf("content %q", got)
	}
	if call := r.jpeg.last(); call.w != 600 || call.h != 400 {
		t.Fatalf("re-encoded a %dx%d picture, want the shown 600x400", call.w, call.h)
	}
}

func TestSourceDeletedAfterTheDialogOpenedFallsBackToTheShownPicture(t *testing.T) {
	r := newJPEGSourceRig(t, nil)
	r.saveTo(t, "out.jpg")
	r.waitDialog(t)
	if err := os.Remove(r.src); err != nil {
		t.Fatal(err)
	}

	r.key(widget.KeyEnter)

	if got := r.waitFile(t, "out.jpg"); string(got) != "ENC-jpeg" {
		t.Fatalf("content %q", got)
	}
}

func TestSavingOverTheSourceReplacesItLosslessly(t *testing.T) {
	r := newJPEGSourceRig(t, nil)
	r.drawn().Canvas.RotateRight()
	r.dialog.path = r.src
	r.app.saveAs()
	r.waitDialog(t)

	r.key(widget.KeyEnter)

	eventually(t, r.app, "the source is replaced", func() bool {
		data, err := os.ReadFile(r.src)
		if err != nil {
			return false
		}
		img, err := jpeg.Decode(bytes.NewReader(data))
		return err == nil && img.Bounds().Dx() == 400 && img.Bounds().Dy() == 600
	})
	if r.jpeg.count() != 0 {
		t.Fatal("re-encoded")
	}
}

func TestCancellingTheLosslessDialogWritesNothingAndKeepsTheChoice(t *testing.T) {
	r := newJPEGSourceRig(t, nil)
	r.saveTo(t, "out.jpg")
	r.waitDialog(t)
	r.key(widget.KeySpace)

	r.key(widget.KeyEscape)
	r.settle()

	if _, err := os.Stat(filepath.Join(r.dir, "out.jpg")); err == nil {
		t.Fatal("a file appeared after the cancellation")
	}
	if !r.app.Settings.Get().SaveJPEGLossless {
		t.Fatal("a cancelled dialog changed the stored choice")
	}
}

func jpegWithOrientation(t *testing.T, path string, w, h int, orientation uint16) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, color.RGBA{uint8(x * 255 / w), uint8(y * 255 / h), 70, 255})
		}
	}
	var plain bytes.Buffer
	if err := jpeg.Encode(&plain, img, &jpeg.Options{Quality: 85}); err != nil {
		t.Fatal(err)
	}
	le := binary.LittleEndian
	tiff := []byte{'I', 'I', 42, 0, 8, 0, 0, 0, 1, 0}
	entry := make([]byte, 12)
	le.PutUint16(entry[0:], 0x0112)
	le.PutUint16(entry[2:], 3)
	le.PutUint32(entry[4:], 1)
	le.PutUint16(entry[8:], orientation)
	tiff = append(append(tiff, entry...), 0, 0, 0, 0)
	payload := append([]byte("Exif\x00\x00"), tiff...)
	var out bytes.Buffer
	out.Write(plain.Bytes()[:2])
	out.Write([]byte{0xFF, 0xE1, byte((len(payload) + 2) >> 8), byte(len(payload) + 2)})
	out.Write(payload)
	out.Write(plain.Bytes()[2:])
	if err := os.WriteFile(path, out.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestEXIFRotatedPhotoIsSavedUprightWithANormalOrientationTag(t *testing.T) {
	r := buildSaveRig(t, nil, false, "photo.jpg", func(t *testing.T, path string) { jpegWithOrientation(t, path, 160, 96, 6) })
	if o := r.app.Canvas.Orientation(); o != codec.OrientRotate90 {
		t.Fatalf("the viewer shows orientation %d", o)
	}

	data := r.confirmLossless(t, "out.jpg")

	img := decodeJPEGFile(t, data)
	if b := img.Bounds(); b.Dx() != 96 || b.Dy() != 160 {
		t.Fatalf("size %v: the file must be stored upright", b)
	}
	payload, err := exif.FromJPEG(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	if o, ok := exif.Orientation(payload); !ok || o != codec.OrientNormal {
		t.Fatalf("orientation tag %d, %v", o, ok)
	}
}

func TestEXIFOrientationIgnoredByTheSettingIsNotAppliedToTheFile(t *testing.T) {
	r := buildSaveRig(t, func(s *settings.Settings) { s.ApplyExifOrientation = false }, false, "photo.jpg", func(t *testing.T, path string) { jpegWithOrientation(t, path, 160, 96, 6) })
	if o := r.app.Canvas.Orientation(); o != codec.OrientNormal {
		t.Fatalf("the viewer shows orientation %d", o)
	}

	data := r.confirmLossless(t, "out.jpg")

	if b := decodeJPEGFile(t, data).Bounds(); b.Dx() != 160 || b.Dy() != 96 {
		t.Fatalf("size %v: the file must keep the stored layout, the same picture as on screen", b)
	}
}

func waitToast(a *App, text string) bool {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		a.Eng.Flush()
		if toastShown(a, text) {
			return true
		}
		time.Sleep(2 * time.Millisecond)
	}
	return false
}
