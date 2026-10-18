package app

import (
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/codec"
	jpegcodec "github.com/oops1/PixGo/internal/codec/jpeg"
	pngcodec "github.com/oops1/PixGo/internal/codec/png"
	model "github.com/oops1/PixGo/internal/gallery"
	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/platform"
	"github.com/oops1/PixGo/internal/settings"
	"github.com/oops1/PixGo/internal/ui/saveopts"
)

type encodeCall struct {
	opts codec.EncodeOptions
	w, h int
}

type recordingEncoder struct {
	id    codec.ID
	ext   string
	mu    sync.Mutex
	calls []encodeCall
	fail  error
}

func (r *recordingEncoder) ID() codec.ID      { return r.id }
func (r *recordingEncoder) Name() string      { return string(r.id) }
func (r *recordingEncoder) Extension() string { return r.ext }
func (r *recordingEncoder) Encode(w io.Writer, img *image.NRGBA, o codec.EncodeOptions) error {
	r.mu.Lock()
	r.calls = append(r.calls, encodeCall{opts: o, w: img.Bounds().Dx(), h: img.Bounds().Dy()})
	r.mu.Unlock()
	if r.fail != nil {
		return r.fail
	}
	_, err := w.Write([]byte("ENC-" + string(r.id)))
	return err
}

func (r *recordingEncoder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.calls)
}

func (r *recordingEncoder) last() encodeCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls[len(r.calls)-1]
}

type fakeSaveDialog struct {
	path  string
	asked atomic.Int32
}

func (f *fakeSaveDialog) Save(o platform.SaveOptions) (string, bool, error) {
	f.asked.Add(1)
	return f.path, f.path != "", nil
}

type saveRig struct {
	app    *App
	jpeg   *recordingEncoder
	png    *recordingEncoder
	webp   *recordingEncoder
	bmp    *recordingEncoder
	dialog *fakeSaveDialog
	dir    string
	src    string
}

func writeBigPNG(t *testing.T, path string) {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 600, 400))
	for y := 0; y < 400; y++ {
		for x := 0; x < 600; x++ {
			img.SetNRGBA(x, y, color.NRGBA{uint8(x * 255 / 600), uint8(y * 255 / 400), 140, 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

func newSaveRig(t *testing.T, mutate func(*settings.Settings), withWebP bool) *saveRig {
	t.Helper()
	return buildSaveRig(t, mutate, withWebP, "photo.png", writeBigPNG)
}

func newJPEGSourceRig(t *testing.T, mutate func(*settings.Settings)) *saveRig {
	t.Helper()
	return buildSaveRig(t, mutate, false, "photo.jpg", writeBigJPEG)
}

func buildSaveRig(t *testing.T, mutate func(*settings.Settings), withWebP bool, name string, write func(*testing.T, string)) *saveRig {
	t.Helper()
	reg := codec.NewRegistry()
	reg.RegisterDecoder(jpegcodec.Decoder{})
	reg.RegisterDecoder(pngcodec.Decoder{})
	r := &saveRig{
		jpeg: &recordingEncoder{id: codec.JPEG, ext: ".jpg"},
		png:  &recordingEncoder{id: codec.PNG, ext: ".png"},
		webp: &recordingEncoder{id: codec.WebP, ext: ".webp"},
		bmp:  &recordingEncoder{id: codec.BMP, ext: ".bmp"},
	}
	reg.RegisterEncoder(r.png)
	reg.RegisterEncoder(r.jpeg)
	reg.RegisterEncoder(r.bmp)
	if withWebP {
		reg.RegisterEncoder(r.webp)
	}
	r.app = formatsApp(t, reg, mutate)
	r.dir = t.TempDir()
	r.src = filepath.Join(r.dir, name)
	write(t, r.src)
	r.app.OpenFiles([]string{r.src})
	waitName(t, r.app, name)
	r.dialog = &fakeSaveDialog{}
	r.app.Platform = &platform.Platform{SaveDialog: r.dialog}
	return r
}

func (r *saveRig) saveTo(t *testing.T, name string) {
	t.Helper()
	r.dialog.path = filepath.Join(r.dir, name)
	r.app.saveAs()
}

func (r *saveRig) waitDialog(t *testing.T) *saveopts.Dialog {
	t.Helper()
	eventually(t, r.app, "the save options dialog opens", func() bool { return r.app.saveDlg != nil })
	return r.app.saveDlg
}

func (r *saveRig) key(code widget.KeyCode) {
	r.app.Eng.SendKeyEvent(widget.KeyEvent{Code: code, Pressed: true})
	r.app.Eng.SendKeyEvent(widget.KeyEvent{Code: code, Pressed: false})
}

func (r *saveRig) waitFile(t *testing.T, name string) []byte {
	t.Helper()
	path := filepath.Join(r.dir, name)
	var data []byte
	eventually(t, r.app, "the file "+name+" is written", func() bool {
		b, err := os.ReadFile(path)
		data = b
		return err == nil && len(b) > 0
	})
	return data
}

func (r *saveRig) settle() {
	for i := 0; i < 4; i++ {
		r.app.Eng.Flush()
	}
}

func TestJPEGAsksForTheQualityAfterThePathIsChosen(t *testing.T) {
	r := newSaveRig(t, nil, false)

	r.saveTo(t, "out.jpg")
	d := r.waitDialog(t)

	if d.Kind() != saveopts.JPEG || d.Values().Quality != 90 {
		t.Fatalf("kind %v values %+v", d.Kind(), d.Values())
	}
	if r.jpeg.count() != 0 {
		t.Fatal("nothing may be written before the user confirms")
	}
	if got := r.dialog.asked.Load(); got != 1 {
		t.Fatalf("the file dialog was asked %d times", got)
	}
}

func TestJPEGDialogStartsFromTheStoredQuality(t *testing.T) {
	r := newSaveRig(t, func(s *settings.Settings) { s.SaveJPEGQuality = 37 }, false)

	r.saveTo(t, "out.jpg")

	if got := r.waitDialog(t).Values().Quality; got != 37 {
		t.Fatalf("the dialog starts at %d, want the stored 37", got)
	}
}

func TestConfirmingJPEGWritesWithTheChosenQualityAndRemembersIt(t *testing.T) {
	r := newSaveRig(t, nil, false)
	r.saveTo(t, "out.jpg")
	r.waitDialog(t)

	r.key(widget.KeyLeft)
	r.key(widget.KeyPageDown)
	r.key(widget.KeyEnter)

	if got := r.waitFile(t, "out.jpg"); string(got) != "ENC-jpeg" {
		t.Fatalf("file content %q", got)
	}
	if call := r.jpeg.last(); !sameOptions(call.opts, (codec.EncodeOptions{Quality: 79})) || call.w != 600 || call.h != 400 {
		t.Fatalf("encoded with %+v", call)
	}
	r.settle()
	if got := r.app.Settings.Get().SaveJPEGQuality; got != 79 {
		t.Fatalf("remembered %d, want 79", got)
	}
	if r.app.saveDlg != nil || r.app.ModalOpen() {
		t.Fatal("the dialog must be gone")
	}
}

func TestNextJPEGSaveStartsFromTheLastChoice(t *testing.T) {
	r := newSaveRig(t, nil, false)
	r.saveTo(t, "one.jpg")
	r.waitDialog(t)
	r.key(widget.KeyHome)
	r.key(widget.KeyEnter)
	r.waitFile(t, "one.jpg")
	r.settle()

	r.saveTo(t, "two.jpg")

	if got := r.waitDialog(t).Values().Quality; got != 1 {
		t.Fatalf("the second dialog starts at %d, want 1", got)
	}
}

func TestCancellingTheOptionsWritesNothingAndKeepsTheStoredChoice(t *testing.T) {
	r := newSaveRig(t, nil, false)
	r.saveTo(t, "out.jpg")
	r.waitDialog(t)
	r.key(widget.KeyHome)

	r.key(widget.KeyEscape)
	r.settle()

	if r.app.saveDlg != nil {
		t.Fatal("the dialog is still open")
	}
	if r.jpeg.count() != 0 {
		t.Fatal("an encoder ran after the cancellation")
	}
	if _, err := os.Stat(filepath.Join(r.dir, "out.jpg")); err == nil {
		t.Fatal("a file appeared after the cancellation")
	}
	if got := r.app.Settings.Get().SaveJPEGQuality; got != 90 {
		t.Fatalf("a cancelled dialog changed the stored quality to %d", got)
	}
}

func TestCancelButtonByMouseAlsoWritesNothing(t *testing.T) {
	r := newSaveRig(t, nil, false)
	r.saveTo(t, "out.jpg")
	d := r.waitDialog(t)
	rc := d.Rect(saveopts.PartCancel)

	x, y := (rc.Min.X+rc.Max.X)/2, (rc.Min.Y+rc.Max.Y)/2
	r.app.Eng.SendMouseMove(x, y)
	r.app.Eng.SendMouseButton(x, y, widget.MouseLeft, true)
	r.app.Eng.SendMouseButton(x, y, widget.MouseLeft, false)
	r.settle()

	if r.app.saveDlg != nil || r.jpeg.count() != 0 {
		t.Fatalf("dialog %v, encoder calls %d", r.app.saveDlg, r.jpeg.count())
	}
}

func TestSaveButtonByMouseWritesTheFile(t *testing.T) {
	r := newSaveRig(t, nil, false)
	r.saveTo(t, "out.jpeg")
	d := r.waitDialog(t)
	rc := d.Rect(saveopts.PartSave)

	x, y := (rc.Min.X+rc.Max.X)/2, (rc.Min.Y+rc.Max.Y)/2
	r.app.Eng.SendMouseMove(x, y)
	r.app.Eng.SendMouseButton(x, y, widget.MouseLeft, true)
	r.app.Eng.SendMouseButton(x, y, widget.MouseLeft, false)

	r.waitFile(t, "out.jpeg")
	if r.jpeg.last().opts.Quality != 90 {
		t.Fatalf("encoded with %+v", r.jpeg.last().opts)
	}
}

func TestPNGAndBMPAreSavedWithoutAnyDialog(t *testing.T) {
	tests := []struct {
		name string
		file string
		enc  func(*saveRig) *recordingEncoder
	}{
		{"png", "out.png", func(r *saveRig) *recordingEncoder { return r.png }},
		{"bmp", "out.bmp", func(r *saveRig) *recordingEncoder { return r.bmp }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := newSaveRig(t, nil, true)

			r.saveTo(t, tc.file)
			r.waitFile(t, tc.file)

			if r.app.saveDlg != nil {
				t.Fatal("a dialog appeared for a format without options")
			}
			if !sameOptions(tc.enc(r).last().opts, (codec.EncodeOptions{})) {
				t.Fatalf("encoded with %+v", tc.enc(r).last().opts)
			}
		})
	}
}

func TestNameWithoutAKnownExtensionGetsTheFirstFormatAndNoDialog(t *testing.T) {
	r := newSaveRig(t, nil, false)

	r.saveTo(t, "mystery")
	r.waitFile(t, "mystery.png")

	if r.app.saveDlg != nil || r.png.count() != 1 {
		t.Fatalf("dialog %v png calls %d", r.app.saveDlg, r.png.count())
	}
}

func TestWebPOffersLosslessAndRemembersBothChoices(t *testing.T) {
	r := newSaveRig(t, nil, true)
	r.saveTo(t, "out.webp")
	d := r.waitDialog(t)
	if d.Kind() != saveopts.WebP || d.Values() != (saveopts.Values{Quality: 80}) {
		t.Fatalf("kind %v values %+v", d.Kind(), d.Values())
	}
	if d.Rect(saveopts.PartCheck).Empty() {
		t.Fatal("a WebP dialog needs the lossless switch")
	}

	r.key(widget.KeySpace)
	r.key(widget.KeyEnter)

	r.waitFile(t, "out.webp")
	if call := r.webp.last(); !sameOptions(call.opts, (codec.EncodeOptions{Quality: 80, Lossless: true})) {
		t.Fatalf("encoded with %+v", call.opts)
	}
	r.settle()
	st := r.app.Settings.Get()
	if st.SaveWebPQuality != 80 || !st.SaveWebPLossless || st.SaveJPEGQuality != 90 {
		t.Fatalf("remembered webp %d lossless %v jpeg %d", st.SaveWebPQuality, st.SaveWebPLossless, st.SaveJPEGQuality)
	}

	r.saveTo(t, "again.webp")
	if got := r.waitDialog(t).Values(); got != (saveopts.Values{Quality: 80, Lossless: true}) {
		t.Fatalf("the next dialog starts with %+v", got)
	}
}

func TestWebPLossyWritesTheChosenQuality(t *testing.T) {
	r := newSaveRig(t, func(s *settings.Settings) { s.SaveWebPLossless = true }, true)
	r.saveTo(t, "out.webp")
	r.waitDialog(t)

	r.key(widget.KeySpace)
	r.key(widget.KeyTab)
	r.key(widget.KeyTab)
	r.key(widget.KeyEnd)
	r.key(widget.KeyLeft)
	r.key(widget.KeyEnter)

	r.waitFile(t, "out.webp")
	if call := r.webp.last(); !sameOptions(call.opts, (codec.EncodeOptions{Quality: 99})) {
		t.Fatalf("encoded with %+v", call.opts)
	}
	r.settle()
	if st := r.app.Settings.Get(); st.SaveWebPQuality != 99 || st.SaveWebPLossless {
		t.Fatalf("remembered %d lossless %v", st.SaveWebPQuality, st.SaveWebPLossless)
	}
}

func TestWebPIsNotOfferedWithoutAnEncoder(t *testing.T) {
	r := newSaveRig(t, nil, false)

	for _, ty := range r.app.saveTypes() {
		if ty.id == codec.WebP {
			t.Fatal("WebP must appear only once its encoder is registered")
		}
	}
}

func writeJPEGSized(t *testing.T, path string, w, h int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, color.RGBA{uint8(x * 255 / w), uint8(y * 255 / h), 90, 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := jpeg.Encode(f, img, &jpeg.Options{Quality: 85}); err != nil {
		t.Fatal(err)
	}
}

func TestTheImageBeingSavedIsTheOneThatWasShownWhenSaveWasChosen(t *testing.T) {
	r := newSaveRig(t, nil, false)
	writeJPEGSized(t, filepath.Join(r.dir, "zzz.jpg"), 50, 30)
	r.app.Ctl.RescanAsync()
	eventually(t, r.app, "the folder sees the second file", func() bool { return folderCount(r.app) == 2 })
	r.saveTo(t, "out.jpg")
	r.waitDialog(t)

	r.app.Ctl.Step(1)
	waitName(t, r.app, "zzz.jpg")
	r.key(widget.KeyEnter)

	r.waitFile(t, "out.jpg")
	if call := r.jpeg.last(); call.w != 600 || call.h != 400 {
		t.Fatalf("saved a %d x %d image, want the original 600 x 400", call.w, call.h)
	}
}

func TestShortcutsAreHeldBackWhileTheOptionsDialogIsOpen(t *testing.T) {
	r := newSaveRig(t, nil, false)
	writeJPEGSized(t, filepath.Join(r.dir, "zzz.jpg"), 50, 30)
	r.app.Ctl.RescanAsync()
	eventually(t, r.app, "the folder sees the second file", func() bool { return folderCount(r.app) == 2 })
	r.saveTo(t, "out.jpg")
	r.waitDialog(t)
	arrow := widget.KeyEvent{Code: widget.KeyRight, Pressed: true}
	escape := widget.KeyEvent{Code: widget.KeyEscape, Pressed: true}

	swallowedArrow := r.app.Root.HandleKey(arrow)
	swallowedEscape := r.app.Root.HandleKey(escape)

	if swallowedArrow || swallowedEscape {
		t.Fatalf("the viewer swallowed keys meant for the dialog: arrow %v escape %v", swallowedArrow, swallowedEscape)
	}
	if r.app.Ctl.Name() != "photo.png" {
		t.Fatalf("the viewer moved to %s behind the dialog", r.app.Ctl.Name())
	}
	r.app.Eng.SendKeyEvent(escape)
	r.settle()
	if !r.app.Root.HandleKey(arrow) {
		t.Fatal("the arrow key must page through the folder again once the dialog is closed")
	}
	waitName(t, r.app, "zzz.jpg")
}

func TestFocusLeavesTheDialogWhenItCloses(t *testing.T) {
	r := newSaveRig(t, nil, false)
	r.saveTo(t, "out.jpg")
	d := r.waitDialog(t)
	if d.Focused() == saveopts.PartNone {
		t.Fatal("the dialog must hold the keyboard focus while it is open")
	}

	r.key(widget.KeyEscape)
	r.settle()

	if d.Focused() != saveopts.PartNone || r.app.ModalOpen() {
		t.Fatalf("focus still on the closed dialog (%v) or the modal flag is stuck", d.Focused())
	}
}

func TestASecondSaveWhileTheDialogIsOpenIsIgnored(t *testing.T) {
	r := newSaveRig(t, nil, false)
	r.saveTo(t, "one.jpg")
	first := r.waitDialog(t)

	r.app.saveAs()
	r.settle()

	if r.app.saveDlg != first || r.dialog.asked.Load() != 1 {
		t.Fatalf("a second dialog or file chooser appeared (asked %d)", r.dialog.asked.Load())
	}
}

func TestEncoderFailureReportsAnError(t *testing.T) {
	r := newSaveRig(t, nil, false)
	r.jpeg.fail = os.ErrInvalid
	r.saveTo(t, "out.jpg")
	r.waitDialog(t)

	r.key(widget.KeyEnter)

	eventually(t, r.app, "the encoder ran", func() bool { return r.jpeg.count() == 1 })
	r.settle()
	if _, err := os.Stat(filepath.Join(r.dir, "out.jpg")); err == nil {
		t.Fatal("a file was written although the encoder failed")
	}
}

func TestSaveKindOnlyKnowsTheFormatsWithOptions(t *testing.T) {
	tests := []struct {
		id   codec.ID
		want saveopts.Kind
		ok   bool
	}{
		{codec.JPEG, saveopts.JPEG, true},
		{codec.WebP, saveopts.WebP, true},
		{codec.PNG, 0, false},
		{codec.BMP, 0, false},
		{codec.GIF, 0, false},
		{codec.TIFF, 0, false},
	}
	for _, tc := range tests {
		t.Run(string(tc.id), func(t *testing.T) {
			got, ok := saveKindOf(tc.id)

			if got != tc.want || ok != tc.ok {
				t.Fatalf("saveKindOf(%s) = %v, %v", tc.id, got, ok)
			}
		})
	}
}

func TestDialogTextFollowsTheInterfaceLanguage(t *testing.T) {
	r := newSaveRig(t, nil, false)
	i18n.SetLang("en")

	r.saveTo(t, "out.jpg")
	d := r.waitDialog(t)

	if d.Widget().Title != "Save options" {
		t.Fatalf("title %q", d.Widget().Title)
	}
}

type unsupportedSaveDialog struct{ asked atomic.Int32 }

func (u *unsupportedSaveDialog) Save(platform.SaveOptions) (string, bool, error) {
	u.asked.Add(1)
	return "", false, platform.ErrUnsupported
}

func TestPlatformWithoutANativeSaveDialogFallsBackToTheEngineOne(t *testing.T) {
	r := newSaveRig(t, nil, false)
	native := &unsupportedSaveDialog{}
	r.app.Platform = &platform.Platform{SaveDialog: native}

	r.app.saveAs()
	eventually(t, r.app, "the native dialog was asked", func() bool { return native.asked.Load() == 1 })
	r.settle()

	if r.app.saveDlg != nil || r.jpeg.count() != 0 || r.png.count() != 0 {
		t.Fatal("the fallback must wait for the file chooser, not write anything")
	}
}

func TestNothingHappensWithoutAnImageOrWithoutEncoders(t *testing.T) {
	r := newSaveRig(t, nil, false)
	r.app.Ctl.Removed(r.src)
	r.dialog.path = filepath.Join(r.dir, "x.jpg")

	r.app.saveAs()
	r.settle()

	if r.dialog.asked.Load() != 0 || r.app.saveDlg != nil {
		t.Fatalf("saved with no image: asked %d", r.dialog.asked.Load())
	}

	bare := formatsApp(t, formatsRegistry(), nil)
	bare.Platform = &platform.Platform{SaveDialog: r.dialog}
	writeBigJPEG(t, filepath.Join(r.dir, "again.jpg"))
	bare.OpenFiles([]string{filepath.Join(r.dir, "again.jpg")})
	waitName(t, bare, "again.jpg")

	bare.saveAs()
	bare.Eng.Flush()

	if r.dialog.asked.Load() != 0 {
		t.Fatal("a build without encoders must not offer saving")
	}
}

func TestSaveDialogWorksFromTheGalleryPageToo(t *testing.T) {
	r := newSaveRig(t, nil, false)
	r.app.OpenGalleryAt(model.Folder(r.dir), "")
	eventually(t, r.app, "gallery listed", func() bool { return r.app.gal != nil && !r.app.gal.loading })
	r.saveTo(t, "g.jpg")
	d := r.waitDialog(t)

	r.key(widget.KeyEscape)
	r.settle()

	if d.Focused() != saveopts.PartNone || r.app.saveDlg != nil {
		t.Fatal("the dialog must be closed and unfocused")
	}
}
