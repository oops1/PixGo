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

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/fsutil"
	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/jpegxform"
	"github.com/oops1/PixGo/internal/meta"
	"github.com/oops1/PixGo/internal/platform"
	"github.com/oops1/PixGo/internal/settings"
	"github.com/oops1/PixGo/internal/ui/saveopts"
	uiwindow "github.com/oops1/PixGo/internal/ui/window"
)

type losslessSource struct {
	path     string
	size     int64
	modTime  time.Time
	op       jpegxform.Op
	perfect  bool
	crop     image.Rectangle
	stripGPS bool
}

type metaSource struct {
	path   string
	format codec.ID
	swap   bool

	converted bool
	display   []byte
}

type saveJob struct {
	img      *image.NRGBA
	produce  func(ctx contextT) (*image.NRGBA, error)
	lossless *losslessSource
	meta     *metaSource
	force    codec.ID
	snap     func(on bool)
	done     func(path string, o saveOutcome)
	aborted  func()
}

func (j *saveJob) hasPixels() bool { return j.img != nil || j.produce != nil }

func (j *saveJob) pixels(ctx contextT) (*image.NRGBA, error) {
	if j.img != nil {
		return j.img, nil
	}
	if j.produce == nil {
		return nil, errPartialPicture
	}
	return j.produce(ctx)
}

func (j *saveJob) metadata(ctx contextT, w, h int, id codec.ID, strip bool) (codec.Metadata, bool) {
	if j.meta == nil {
		return codec.Metadata{}, false
	}
	f, err := os.Open(j.meta.path)
	if err != nil {
		return codec.Metadata{}, false
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return codec.Metadata{}, false
	}
	m, err := meta.ReadBlocksContext(ctx, f, st.Size(), j.meta.format)
	if err != nil && m.Empty() {
		return codec.Metadata{}, false
	}
	out, err := meta.RewriteFor(m, meta.RewriteOptions{Width: w, Height: h, StripGPS: strip, SwapResolution: j.meta.swap}, id)
	if j.meta.converted {
		out.ICC = j.meta.display
	}
	var dropped *meta.DroppedError
	return out, errors.As(err, &dropped) && dropped.EXIF
}

type saveOutcome struct {
	trimmed     bool
	droppedEXIF bool
}

func (o saveOutcome) toastKey() string {
	switch {
	case o.trimmed:
		return "toast.savedTrimmed"
	case o.droppedEXIF:
		return "toast.savedNoExif"
	}
	return "toast.saved"
}

var (
	errSourceChanged  = errors.New("the source file changed after it was shown")
	errPartialPicture = errors.New("only part of the picture is loaded")
)

func (a *App) losslessSource() *losslessSource {
	return a.losslessSourceFor(image.Rectangle{})
}

func (a *App) losslessSourceFor(crop image.Rectangle) *losslessSource {
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
	return &losslessSource{path: path, size: st.Size(), modTime: st.ModTime(), op: op, perfect: perfect, crop: crop}
}

func (s *losslessSource) trimmed() bool { return s.crop.Empty() && !s.perfect }

func (s *losslessSource) open() (*os.File, os.FileInfo, error) {
	f, err := os.Open(s.path)
	if err != nil {
		return nil, nil, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, nil, err
	}
	if st.Size() != s.size || !st.ModTime().Equal(s.modTime) {
		f.Close()
		return nil, nil, errSourceChanged
	}
	return f, st, nil
}

func (s *losslessSource) snapped() (image.Rectangle, error) {
	f, st, err := s.open()
	if err != nil {
		return image.Rectangle{}, err
	}
	defer f.Close()
	return jpegxform.CropFor(f, st.Size(), s.op, s.crop)
}

func (s *losslessSource) transform(ctx contextT) ([]byte, error) {
	f, st, err := s.open()
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var buf bytes.Buffer
	o := jpegxform.Options{Trim: !s.perfect || !s.crop.Empty(), Crop: s.crop, StripGPS: s.stripGPS}
	if err := jpegxform.TransformContext(ctx, &buf, f, st.Size(), s.op, o); err != nil {
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
	job := &saveJob{img: a.orientedFrame(), lossless: a.losslessSource()}
	if path := a.Ctl.Path(); path != "" {
		job.meta = &metaSource{path: path, format: a.Ctl.Info().Format, swap: a.Canvas.Orientation().Transposes(), converted: shown.Color.Converted, display: a.Ctl.Color().DisplayICC()}
	}
	return job
}

func (j *saveJob) partialMessage() string {
	if j.lossless == nil {
		return i18n.T("error.partialSaveNone")
	}
	return i18n.T("error.partialSave")
}

func (a *App) saveAs() {
	if a.editing {
		a.saveEditAs()
		return
	}
	job := a.captureSaveJob()
	if job == nil {
		return
	}
	a.runSaveAs(job, "")
}

func (a *App) runSaveAs(job *saveJob, name string) {
	if a.saveDlg != nil {
		return
	}
	if !job.hasPixels() && job.lossless == nil {
		a.errorBox(job.partialMessage())
		return
	}
	types := a.saveTypes()
	if len(types) == 0 {
		return
	}
	src := a.Ctl.Path()
	def := 0
	for i, t := range types {
		if t.id == a.Ctl.Info().Format {
			def = i
		}
	}
	if name == "" {
		base := strings.TrimSuffix(filepath.Base(src), filepath.Ext(src))
		name = base + types[def].ext
	}
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
	v := saveopts.Values{Quality: st.SaveJPEGQuality, StripGPS: st.SaveStripGPS}
	switch kind {
	case saveopts.WebP:
		v.Quality, v.Lossless = st.SaveWebPQuality, st.SaveWebPLossless
	case saveopts.JPEGLossless:
		v.Lossless = st.SaveJPEGLossless
	}
	return v
}

func (a *App) rememberSaveChoice(kind saveopts.Kind, v saveopts.Values) {
	a.Settings.Update(func(s *settings.Settings) {
		s.SaveStripGPS = v.StripGPS
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
	var t saveType
	if job != nil && job.force != "" {
		for _, c := range types {
			if c.id == job.force {
				t = c
			}
		}
		if t.id == "" {
			return
		}
	} else {
		var ok bool
		if t, ok = typeForPath(types, path); !ok {
			t = types[0]
			path += t.ext
		}
	}
	enc, _ := a.Ctl.env.Registry.Encoder(t.id)
	if job == nil || enc == nil {
		return
	}
	if !job.hasPixels() && (t.id != codec.JPEG || job.lossless == nil) {
		a.errorBox(job.partialMessage())
		return
	}
	kind, asks := saveKindOf(t.id)
	if !asks {
		a.encodeTo(path, enc, job, codec.EncodeOptions{}, a.Settings.Get().SaveStripGPS)
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
	if !job.hasPixels() {
		initial.Lossless = true
	}
	var watch []saveopts.Option
	if job.snap != nil && kind == saveopts.JPEGLossless {
		watch = append(watch, saveopts.WithLosslessWatch(job.snap))
	}
	a.saveDlg = saveopts.Show(a.Eng, kind|saveopts.GPS, initial, func(v saveopts.Values, ok bool) {
		a.modalOpen.Store(false)
		a.Eng.Post(func() {
			a.saveDlg = nil
			a.focusPage()
			if job.snap != nil {
				job.snap(false)
			}
			if !ok {
				if job.aborted != nil {
					job.aborted()
				}
				return
			}
			viaFile := kind == saveopts.JPEGLossless && v.Lossless
			if !job.hasPixels() && !viaFile {
				a.errorBox(job.partialMessage())
				return
			}
			a.rememberSaveChoice(kind, v)
			opts := codec.EncodeOptions{Quality: v.Quality, Lossless: v.Lossless && kind == saveopts.WebP}
			if viaFile {
				job.lossless.stripGPS = v.StripGPS
				a.writeLossless(path, enc, job, opts, v.StripGPS)
				return
			}
			a.encodeTo(path, enc, job, opts, v.StripGPS)
		})
	}, watch...)
}

func (a *App) markBusy(on bool) {
	if a.editing {
		a.Editor.SetBusy(on)
	}
}

func (a *App) writeLossless(path string, enc codec.Encoder, job *saveJob, fallback codec.EncodeOptions, strip bool) {
	a.markBusy(true)
	a.Ctl.Async("save", func(ctx contextT) (any, error) {
		data, err := job.lossless.transform(ctx)
		outcome := saveOutcome{trimmed: job.lossless.trimmed()}
		if err != nil {
			if !losslessUnavailable(err) {
				return nil, err
			}
			if !job.hasPixels() {
				return nil, errPartialPicture
			}
			data, outcome, err = a.encodeBytes(ctx, enc, job, fallback, strip)
			if err != nil {
				return nil, err
			}
		}
		return outcome, fsutil.WriteAtomic(path, data, 0o644)
	}, a.savedHandler(job, path))
}

func (a *App) encodeBytes(ctx contextT, enc codec.Encoder, job *saveJob, opts codec.EncodeOptions, strip bool) ([]byte, saveOutcome, error) {
	img, err := job.pixels(ctx)
	if err != nil {
		return nil, saveOutcome{}, err
	}
	var outcome saveOutcome
	b := img.Bounds()
	opts.Metadata, outcome.droppedEXIF = job.metadata(ctx, b.Dx(), b.Dy(), enc.ID(), strip)
	var buf bytes.Buffer
	if err := enc.Encode(&buf, img, opts); err != nil {
		return nil, outcome, err
	}
	return buf.Bytes(), outcome, nil
}

func (a *App) encodeTo(path string, enc codec.Encoder, job *saveJob, opts codec.EncodeOptions, strip bool) {
	a.markBusy(true)
	a.Ctl.Async("save", func(ctx contextT) (any, error) {
		data, outcome, err := a.encodeBytes(ctx, enc, job, opts, strip)
		if err != nil {
			return nil, err
		}
		return outcome, fsutil.WriteAtomic(path, data, 0o644)
	}, a.savedHandler(job, path))
}

func (a *App) savedHandler(job *saveJob, path string) func(any, error) {
	return func(v any, err error) {
		a.markBusy(false)
		switch {
		case errors.Is(err, errPartialPicture):
			a.errorBox(i18n.T("error.partialSave"))
			return
		case errors.Is(err, codec.ErrTooLarge):
			a.errorBox(i18n.T("error.editTooLarge"))
			return
		case err != nil:
			a.errorBox(i18n.T("error.save"))
			return
		}
		outcome, _ := v.(saveOutcome)
		if job != nil && job.done != nil {
			job.done(path, outcome)
			return
		}
		a.Root.Toast(i18n.T(outcome.toastKey()))
		a.Ctl.RescanAsync()
	}
}
