package app

import (
	"os"

	"github.com/oops1/headless-gui/v3/widget"
	"github.com/oops1/headless-gui/v3/window"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/fsutil"
	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/meta"
	"github.com/oops1/PixGo/internal/ui/infopanel"
	uiwindow "github.com/oops1/PixGo/internal/ui/window"
)

func (a *App) Panel() widget.Widget {
	if a.page == uiwindow.PageGallery || !a.infoOpen || a.info == nil {
		return nil
	}
	return a.info
}

func (a *App) toggleInfo() {
	a.infoOpen = !a.infoOpen
	if a.infoOpen {
		if a.info == nil {
			a.info = infopanel.New(a.Eng, a.Root.Theme)
			a.info.OnClose = a.toggleInfo
			a.info.OnCopy = func(text string) {
				widget.ClipboardSetText(text)
				a.Root.Toast(i18n.T("toast.pathCopied"))
			}
			a.info.OnReveal = a.reveal
			a.info.OnOpenURL = func(u string) {
				go window.OpenURL(u)
			}
		}
		a.info.Theme = a.Root.Theme
		a.infoKey = ""
		a.refreshInfo()
	}
	a.Root.Refresh()
}

func (a *App) refreshInfo() {
	if !a.infoOpen || a.info == nil {
		return
	}
	path := a.Ctl.Path()
	key := path
	if a.Ctl.Status() == StatusReady {
		key += "|ready"
	}
	if key == a.infoKey {
		return
	}
	a.infoKey = key
	if path == "" {
		a.info.SetModel(infopanel.Model{})
		return
	}
	in := infopanel.Input{Path: path, Name: a.Ctl.Name(), Info: a.Ctl.Info()}
	if fi, err := os.Stat(path); err == nil {
		in.FileSize = fi.Size()
		in.Modified = fi.ModTime()
	}
	in.Created, in.HasCreated = fsutil.CreationTime(path)
	a.info.SetModel(infopanel.BuildModel(in))
	if d := a.Ctl.Decoded(); d != nil && a.Ctl.Status() == StatusReady {
		a.info.SetThumbnail(d.Source(), a.Ctl.ExifOrientation())
	}
	if a.Ctl.Status() != StatusReady {
		return
	}
	a.Ctl.Async("meta", func(ctx contextT) (any, error) {
		src, err := codec.OpenFile(path)
		if err != nil {
			return nil, err
		}
		defer src.Close()
		md, _ := meta.Read(ctx, src.R, src.Size, in.Info.Format)
		return md, nil
	}, func(v any, err error) {
		if err != nil || a.Ctl.Path() != path || !a.infoOpen {
			return
		}
		md, _ := v.(*meta.Metadata)
		in.Meta = md
		a.info.SetModel(infopanel.BuildModel(in))
	})
}
