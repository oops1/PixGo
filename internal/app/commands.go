package app

import (
	"path/filepath"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/commands"
	"github.com/oops1/PixGo/internal/i18n"
)

func (a *App) registerCommands() {
	img := a.hasImage
	run := map[commands.ID]func(){
		CmdPrev:            func() { a.Ctl.Step(-1) },
		CmdNext:            func() { a.Ctl.Step(1) },
		CmdFirst:           func() { a.Ctl.First() },
		CmdLast:            func() { a.Ctl.Last() },
		CmdZoomIn:          a.Canvas.ZoomIn,
		CmdZoomOut:         a.Canvas.ZoomOut,
		CmdZoomActual:      a.Canvas.Actual,
		CmdZoomFit:         a.Canvas.Fit,
		CmdZoomWidth:       a.Canvas.FitWidth,
		CmdZoomToggle:      a.Canvas.ToggleFitActual,
		CmdRotateRight:     a.Canvas.RotateRight,
		CmdRotateLeft:      a.Canvas.RotateLeft,
		CmdFlipH:           a.Canvas.FlipH,
		CmdFlipV:           a.Canvas.FlipV,
		CmdFavorite:        func() { a.Ctl.ToggleFavorite() },
		CmdOpen:            a.openDialog,
		CmdGallery:         a.openDialog,
		CmdDelete:          func() { a.deleteCurrent(false) },
		CmdDeletePermanent: func() { a.deleteCurrent(true) },
		CmdCopy:            a.copyFile,
		CmdCopyPath:        a.copyPath,
		CmdCopyImage:       a.copyImage,
		CmdReveal:          a.reveal,
		CmdSaveAs:          a.saveAs,
		CmdFullscreen:      a.toggleFullscreen,
		CmdInfo:            a.toggleInfo,
		CmdEscape:          a.escape,
	}
	enabled := map[commands.ID]func() bool{
		CmdPrev:            a.CanPrev,
		CmdNext:            a.CanNext,
		CmdFirst:           a.CanPrev,
		CmdLast:            a.CanNext,
		CmdZoomIn:          img,
		CmdZoomOut:         img,
		CmdZoomActual:      img,
		CmdZoomFit:         img,
		CmdZoomWidth:       img,
		CmdZoomToggle:      img,
		CmdRotateRight:     img,
		CmdRotateLeft:      img,
		CmdFlipH:           img,
		CmdFlipV:           img,
		CmdFavorite:        img,
		CmdDelete:          a.hasFile,
		CmdDeletePermanent: a.hasFile,
		CmdCopy:            a.hasFile,
		CmdCopyPath:        a.hasFile,
		CmdCopyImage:       img,
		CmdReveal:          a.hasFile,
		CmdSaveAs:          img,
		CmdFullscreen:      img,
		CmdInfo:            a.hasFile,
		CmdEscape:          func() bool { return a.fullscreen },
	}
	for _, s := range Specs {
		a.Commands.Add(commands.Command{
			ID:      s.ID,
			Title:   s.Title,
			Icon:    s.Icon,
			Keys:    s.Keys,
			Run:     run[s.ID],
			Enabled: enabled[s.ID],
		})
	}
}

func (a *App) openDialog() {
	dir := ""
	if p := a.Ctl.Path(); p != "" {
		dir = filepath.Dir(p)
	} else if a.Platform != nil && a.Platform.Folders != nil {
		dir = a.Platform.Folders.Folders().Pictures
	}
	mb := widget.NewMessageBox(a.Eng)
	mb.ShowOpenFile(widget.FileDialogOptions{
		Title:    i18n.T("cmd.open"),
		StartDir: dir,
		Filters: []widget.FileFilter{
			{Label: i18n.T("open.filter.images"), Exts: a.Ctl.env.Registry.Extensions()},
		},
	}, func(path string, ok bool) {
		if ok {
			a.OpenFiles([]string{path})
		}
	})
}
