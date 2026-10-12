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
)

const defaultJPEGQuality = 90

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
	if a.orientedFrame() == nil {
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
				a.writeImage(p, types)
				return
			}
			if errors.Is(err, platform.ErrUnsupported) {
				a.saveWithEngineDialog(dir, name, types)
			}
		})
		return
	}
	a.saveWithEngineDialog(dir, name, types)
}

func (a *App) saveWithEngineDialog(dir, name string, types []saveType) {
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
			a.writeImage(path, types)
		}
	})
}

func (a *App) writeImage(path string, types []saveType) {
	t, ok := typeForPath(types, path)
	if !ok {
		t = types[0]
		path += t.ext
	}
	img := a.orientedFrame()
	enc, _ := a.Ctl.env.Registry.Encoder(t.id)
	if img == nil || enc == nil {
		return
	}
	a.Ctl.Async("save", func(ctx contextT) (any, error) {
		var buf bytes.Buffer
		if err := enc.Encode(&buf, img, codec.EncodeOptions{Quality: defaultJPEGQuality}); err != nil {
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
