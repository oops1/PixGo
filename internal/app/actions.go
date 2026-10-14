package app

import (
	"bytes"
	"errors"
	"image"
	"os"
	"path/filepath"
	"strings"

	"github.com/oops1/headless-gui/v3/widget"
	"github.com/oops1/headless-gui/v3/window"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/fsutil"
	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/imaging"
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

func (a *App) copyImage() {
	img := a.orientedFrame()
	if img == nil {
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

func (a *App) saveAs() {
	img := a.orientedFrame()
	if img == nil || a.saveDlg != nil {
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
				a.writeImage(p, types, img)
				return
			}
			if errors.Is(err, platform.ErrUnsupported) {
				a.saveWithEngineDialog(dir, name, types, img)
			}
		})
		return
	}
	a.saveWithEngineDialog(dir, name, types, img)
}

func (a *App) saveWithEngineDialog(dir, name string, types []saveType, img *image.NRGBA) {
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
			a.writeImage(path, types, img)
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
	if kind == saveopts.WebP {
		return saveopts.Values{Quality: st.SaveWebPQuality, Lossless: st.SaveWebPLossless}
	}
	return saveopts.Values{Quality: st.SaveJPEGQuality}
}

func (a *App) rememberSaveChoice(kind saveopts.Kind, v saveopts.Values) {
	a.Settings.Update(func(s *settings.Settings) {
		if kind == saveopts.WebP {
			s.SaveWebPQuality, s.SaveWebPLossless = v.Quality, v.Lossless
			return
		}
		s.SaveJPEGQuality = v.Quality
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

func (a *App) writeImage(path string, types []saveType, img *image.NRGBA) {
	t, ok := typeForPath(types, path)
	if !ok {
		t = types[0]
		path += t.ext
	}
	enc, _ := a.Ctl.env.Registry.Encoder(t.id)
	if img == nil || enc == nil {
		return
	}
	kind, asks := saveKindOf(t.id)
	if !asks {
		a.encodeTo(path, enc, img, codec.EncodeOptions{})
		return
	}
	if a.saveDlg != nil {
		return
	}
	a.modalOpen.Store(true)
	a.saveDlg = saveopts.Show(a.Eng, kind, a.lastSaveChoice(kind), func(v saveopts.Values, ok bool) {
		a.modalOpen.Store(false)
		a.Eng.Post(func() {
			a.saveDlg = nil
			a.focusPage()
			if !ok {
				return
			}
			a.rememberSaveChoice(kind, v)
			a.encodeTo(path, enc, img, codec.EncodeOptions{Quality: v.Quality, Lossless: v.Lossless})
		})
	})
}

func (a *App) encodeTo(path string, enc codec.Encoder, img *image.NRGBA, opts codec.EncodeOptions) {
	a.Ctl.Async("save", func(ctx contextT) (any, error) {
		var buf bytes.Buffer
		if err := enc.Encode(&buf, img, opts); err != nil {
			return nil, err
		}
		return nil, fsutil.WriteAtomic(path, buf.Bytes(), 0o644)
	}, func(_ any, err error) {
		if err != nil {
			a.errorBox(i18n.T("error.save"))
			return
		}
		a.Root.Toast(i18n.T("toast.saved"))
		a.Ctl.RescanAsync()
	})
}
