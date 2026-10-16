package app

import (
	"bytes"
	"errors"
	"image"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/oops1/headless-gui/v3/widget"
	"github.com/oops1/headless-gui/v3/window"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/fsutil"
	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/imaging"
	"github.com/oops1/PixGo/internal/jpegxform"
	"github.com/oops1/PixGo/internal/platform"
	"github.com/oops1/PixGo/internal/settings"
	"github.com/oops1/PixGo/internal/ui/saveopts"
	uiwindow "github.com/oops1/PixGo/internal/ui/window"
)

func (a *App) hasFile() bool { return a.Ctl.Path() != "" && a.Ctl.Status() != StatusEmpty }

func (a *App) confirm(title, message string, yes func()) {
	widget.NewMessageBox(a.Eng).ShowQuestion(title, message, func(r widget.MessageBoxResult) {
		if r == widget.MBResultYes {
			yes()
		}
	})
}

func (a *App) errorBox(message string) {
	widget.NewMessageBox(a.Eng).ShowError(i18n.T("app.name"), message)
}

func (a *App) deleteCurrent(permanent bool) {
	a.deletePath(a.Ctl.Path(), a.Ctl.Name(), permanent)
}

func (a *App) deletePath(path, name string, permanent bool) {
	if path == "" {
		return
	}
	if permanent {
		a.confirm(i18n.T("dialog.deletePermanent.title"), i18n.T("dialog.deletePermanent.message", name), func() { a.removeFile(path, true) })
		return
	}
	if a.Platform != nil && a.Platform.Trash != nil && a.Platform.Trash.Available(path) {
		if a.Settings.Get().ConfirmDelete {
			a.confirm(i18n.T("dialog.delete.title"), i18n.T("dialog.delete.message", name), func() { a.removeFile(path, false) })
		} else {
			a.removeFile(path, false)
		}
		return
	}
	a.confirm(i18n.T("dialog.noTrash.title"), i18n.T("dialog.noTrash.message", name), func() { a.removeFile(path, true) })
}

func (a *App) removeFile(path string, permanent bool) {
	a.Ctl.Async("delete", func(ctx contextT) (any, error) {
		if permanent {
			return nil, os.Remove(path)
		}
		return nil, a.Platform.Trash.Trash(ctx, path)
	}, func(_ any, err error) {
		if err != nil {
			a.errorBox(i18n.T("error.delete"))
			return
		}
		a.Ctl.Removed(path)
		a.fileRemoved(path)
		if permanent {
			a.Root.Toast(i18n.T("toast.deletedForever"))
		} else {
			a.Root.Toast(i18n.T("toast.deleted"))
		}
	})
}

func (a *App) copyFile() {
	path := a.Ctl.Path()
	if path == "" {
		return
	}
	widget.ClipboardSetFiles([]string{path}, false)
	a.Root.Toast(i18n.T("toast.fileCopied"))
}

func (a *App) copyPath() {
	path := a.Ctl.Path()
	if path == "" {
		return
	}
	widget.ClipboardSetText(path)
	a.Root.Toast(i18n.T("toast.pathCopied"))
}

func (a *App) orientedFrame() *image.NRGBA {
	d := a.Ctl.Decoded()
	if d == nil || len(d.Levels) == 0 {
		return nil
	}
	return imaging.Orient(d.Levels[0], a.Canvas.Orientation())
}

func (a *App) hasPartialPicture() bool {
	d := a.Ctl.Shown()
	return d != nil && len(d.Levels) > 0 && !d.FullResolution()
}

func (a *App) copyImage() {
	img := a.orientedFrame()
	if img == nil {
		if a.hasPartialPicture() {
			a.errorBox(i18n.T("error.partialCopy"))
		}
		return
	}
	a.Ctl.Async("copyImage", func(ctx contextT) (any, error) {
		return nil, widget.ClipboardSetImage(img)
	}, func(_ any, err error) {
		switch {
		case err == nil:
			a.Root.Toast(i18n.T("toast.imageCopied"))
		default:
			a.errorBox(i18n.T("error.copy"))
		}
	})
}

func (a *App) reveal() {
	path := a.Ctl.Path()
	if path == "" {
		return
	}
	go func() {
		if err := window.RevealFile(path); err != nil {
			a.Eng.Post(func() { a.errorBox(i18n.T("error.reveal")) })
		}
	}()
}

type saveType struct {
	id       codec.ID
	title    string
	patterns []string
	ext      string
}

func (a *App) saveTypes() []saveType {
	all := []saveType{
		{codec.PNG, "save.type.png", []string{"*.png"}, ".png"},
		{codec.JPEG, "save.type.jpeg", []string{"*.jpg", "*.jpeg"}, ".jpg"},
		{codec.BMP, "save.type.bmp", []string{"*.bmp"}, ".bmp"},
		{codec.WebP, "save.type.webp", []string{"*.webp"}, ".webp"},
	}
	var out []saveType
	for _, t := range all {
		if _, ok := a.Ctl.env.Registry.Encoder(t.id); ok {
			out = append(out, t)
		}
	}
	return out
}

func typeForPath(types []saveType, path string) (saveType, bool) {
	ext := strings.ToLower(filepath.Ext(path))
	for _, t := range types {
		for _, p := range t.patterns {
			if strings.EqualFold(strings.TrimPrefix(p, "*"), ext) {
				return t, true
			}
		}
	}
	return saveType{}, false
}

type losslessSource struct {
	path    string
	size    int64
	modTime time.Time
	op      jpegxform.Op
	perfect bool
}

type saveJob struct {
	img      *image.NRGBA
	lossless *losslessSource
}

type saveOutcome struct {
	trimmed bool
}

func (o saveOutcome) toastKey() string {
	if o.trimmed {
		return "toast.savedTrimmed"
	}
	return "toast.saved"
}

var (
	errSourceChanged  = errors.New("the source file changed after it was shown")
	errPartialPicture = errors.New("only part of the picture is loaded")
)

func (a *App) losslessSource() *losslessSource {
	if a.Ctl.Info().Format != codec.JPEG {
		return nil
	}
	path := a.Ctl.Path()
	if path == "" {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() {
		return nil
	}
	op := jpegxform.FromOrientation(a.Canvas.Orientation())
	perfect, err := jpegxform.Perfect(f, st.Size(), op)
	if err != nil {
		return nil
	}
	return &losslessSource{path: path, size: st.Size(), modTime: st.ModTime(), op: op, perfect: perfect}
}

func (s *losslessSource) transform(ctx contextT) ([]byte, error) {
	f, err := os.Open(s.path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if st.Size() != s.size || !st.ModTime().Equal(s.modTime) {
		return nil, errSourceChanged
	}
	var buf bytes.Buffer
	if err := jpegxform.TransformContext(ctx, &buf, f, s.size, s.op, jpegxform.Options{Trim: !s.perfect}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func losslessUnavailable(err error) bool {
	for _, target := range []error{errSourceChanged, codec.ErrUnsupported, codec.ErrCorrupt, codec.ErrTooLarge, jpegxform.ErrImperfect, fs.ErrNotExist} {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}

func (a *App) captureSaveJob() *saveJob {
	shown := a.Ctl.Shown()
	if shown == nil || len(shown.Levels) == 0 {
		return nil
	}
	return &saveJob{img: a.orientedFrame(), lossless: a.losslessSource()}
}

func (j *saveJob) partialMessage() string {
	if j.lossless == nil {
		return i18n.T("error.partialSaveNone")
	}
	return i18n.T("error.partialSave")
}

func (a *App) saveAs() {
	if a.saveDlg != nil {
		return
	}
	job := a.captureSaveJob()
	if job == nil {
		return
	}
	if job.img == nil && job.lossless == nil {
		a.errorBox(job.partialMessage())
		return
	}
	types := a.saveTypes()
	if len(types) == 0 {
		return
	}
	src := a.Ctl.Path()
	base := strings.TrimSuffix(filepath.Base(src), filepath.Ext(src))
	def := 0
	for i, t := range types {
		if t.id == a.Ctl.Info().Format {
			def = i
		}
	}
	name := base + types[def].ext
	dir := filepath.Dir(src)
	if a.Platform != nil && a.Platform.SaveDialog != nil {
		opts := platform.SaveOptions{Title: i18n.T("save.title"), Dir: dir, Name: name, FilterIndex: def + 1}
		for _, t := range types {
			opts.Filters = append(opts.Filters, platform.FileFilter{Name: i18n.T(t.title), Patterns: t.patterns})
		}
		a.Ctl.Async("saveDialog", func(ctx contextT) (any, error) {
			p, ok, err := a.Platform.SaveDialog.Save(opts)
			if err != nil || !ok {
				return "", err
			}
			return p, nil
		}, func(v any, err error) {
			if p, _ := v.(string); err == nil && p != "" {
				a.writeImage(p, types, job)
				return
			}
			if errors.Is(err, platform.ErrUnsupported) {
				a.saveWithEngineDialog(dir, name, types, job)
			}
		})
		return
	}
	a.saveWithEngineDialog(dir, name, types, job)
}

func (a *App) saveWithEngineDialog(dir, name string, types []saveType, job *saveJob) {
	var filters []widget.FileFilter
	for _, t := range types {
		var exts []string
		for _, p := range t.patterns {
			exts = append(exts, strings.TrimPrefix(p, "*"))
		}
		filters = append(filters, widget.FileFilter{Label: i18n.T(t.title), Exts: exts})
	}
	widget.NewMessageBox(a.Eng).ShowSaveFile(widget.FileDialogOptions{
		Title:       i18n.T("save.title"),
		StartDir:    dir,
		InitialName: name,
		Filters:     filters,
	}, func(path string, ok bool) {
		if ok {
			a.writeImage(path, types, job)
		}
	})
}

func saveKindOf(id codec.ID) (saveopts.Kind, bool) {
	switch id {
	case codec.JPEG:
		return saveopts.JPEG, true
	case codec.WebP:
		return saveopts.WebP, true
	}
	return 0, false
}

func (a *App) lastSaveChoice(kind saveopts.Kind) saveopts.Values {
	st := a.Settings.Get()
	switch kind {
	case saveopts.WebP:
		return saveopts.Values{Quality: st.SaveWebPQuality, Lossless: st.SaveWebPLossless}
	case saveopts.JPEGLossless:
		return saveopts.Values{Quality: st.SaveJPEGQuality, Lossless: st.SaveJPEGLossless}
	}
	return saveopts.Values{Quality: st.SaveJPEGQuality}
}

func (a *App) rememberSaveChoice(kind saveopts.Kind, v saveopts.Values) {
	a.Settings.Update(func(s *settings.Settings) {
		switch kind {
		case saveopts.WebP:
			s.SaveWebPQuality, s.SaveWebPLossless = v.Quality, v.Lossless
		case saveopts.JPEGLossless:
			s.SaveJPEGQuality, s.SaveJPEGLossless = v.Quality, v.Lossless
		default:
			s.SaveJPEGQuality = v.Quality
		}
	})
}

func (a *App) ModalOpen() bool { return a.modalOpen.Load() }

func (a *App) focusPage() {
	if a.page == uiwindow.PageGallery && a.gal != nil {
		a.Eng.SetFocus(a.gal.page)
		return
	}
	a.Root.FocusContent()
}

func (a *App) writeImage(path string, types []saveType, job *saveJob) {
	t, ok := typeForPath(types, path)
	if !ok {
		t = types[0]
		path += t.ext
	}
	enc, _ := a.Ctl.env.Registry.Encoder(t.id)
	if job == nil || enc == nil {
		return
	}
	if job.img == nil && (t.id != codec.JPEG || job.lossless == nil) {
		a.errorBox(job.partialMessage())
		return
	}
	kind, asks := saveKindOf(t.id)
	if !asks {
		a.encodeTo(path, enc, job.img, codec.EncodeOptions{})
		return
	}
	if kind == saveopts.JPEG && job.lossless != nil {
		kind = saveopts.JPEGLossless
	}
	if a.saveDlg != nil {
		return
	}
	a.modalOpen.Store(true)
	initial := a.lastSaveChoice(kind)
	if job.img == nil {
		initial.Lossless = true
	}
	a.saveDlg = saveopts.Show(a.Eng, kind, initial, func(v saveopts.Values, ok bool) {
		a.modalOpen.Store(false)
		a.Eng.Post(func() {
			a.saveDlg = nil
			a.focusPage()
			if !ok {
				return
			}
			viaFile := kind == saveopts.JPEGLossless && v.Lossless
			if job.img == nil && !viaFile {
				a.errorBox(job.partialMessage())
				return
			}
			a.rememberSaveChoice(kind, v)
			opts := codec.EncodeOptions{Quality: v.Quality, Lossless: v.Lossless && kind == saveopts.WebP}
			if viaFile {
				a.writeLossless(path, enc, job, opts)
				return
			}
			a.encodeTo(path, enc, job.img, opts)
		})
	})
}

func (a *App) writeLossless(path string, enc codec.Encoder, job *saveJob, fallback codec.EncodeOptions) {
	a.Ctl.Async("save", func(ctx contextT) (any, error) {
		data, err := job.lossless.transform(ctx)
		outcome := saveOutcome{trimmed: !job.lossless.perfect}
		if err != nil {
			if !losslessUnavailable(err) {
				return nil, err
			}
			if job.img == nil {
				return nil, errPartialPicture
			}
			var buf bytes.Buffer
			if err := enc.Encode(&buf, job.img, fallback); err != nil {
				return nil, err
			}
			data, outcome = buf.Bytes(), saveOutcome{}
		}
		return outcome, fsutil.WriteAtomic(path, data, 0o644)
	}, a.savedHandler)
}

func (a *App) encodeTo(path string, enc codec.Encoder, img *image.NRGBA, opts codec.EncodeOptions) {
	a.Ctl.Async("save", func(ctx contextT) (any, error) {
		var buf bytes.Buffer
		if err := enc.Encode(&buf, img, opts); err != nil {
			return nil, err
		}
		return saveOutcome{}, fsutil.WriteAtomic(path, buf.Bytes(), 0o644)
	}, a.savedHandler)
}

func (a *App) savedHandler(v any, err error) {
	if errors.Is(err, errPartialPicture) {
		a.errorBox(i18n.T("error.partialSave"))
		return
	}
	if err != nil {
		a.errorBox(i18n.T("error.save"))
		return
	}
	outcome, _ := v.(saveOutcome)
	a.Root.Toast(i18n.T(outcome.toastKey()))
	a.Ctl.RescanAsync()
}
