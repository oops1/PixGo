package app

import (
	"image"
	"os"
	"path/filepath"
	"strings"

	"github.com/oops1/headless-gui/v3/widget"
	"github.com/oops1/headless-gui/v3/window"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/imaging"
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
