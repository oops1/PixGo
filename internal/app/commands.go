package app

import (
	"github.com/oops1/PixGo/internal/commands"
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
		CmdRotate180:       a.Canvas.Rotate180,
		CmdFlipH:           a.Canvas.FlipH,
		CmdFlipV:           a.Canvas.FlipV,
		CmdFavorite:        func() { a.Ctl.ToggleFavorite() },
		CmdOpen:            a.OpenGallery,
		CmdGallery:         a.OpenGallery,
		CmdBack:            a.OpenGallery,
		CmdSettings:        a.OpenSettings,
		CmdDelete:          func() { a.deleteCurrent(false) },
		CmdDeletePermanent: func() { a.deleteCurrent(true) },
		CmdCopy:            a.copyFile,
		CmdCopyPath:        a.copyPath,
		CmdCopyImage:       a.copyImage,
		CmdReveal:          a.reveal,
		CmdSaveAs:          a.saveAs,
		CmdFullscreen:      a.toggleFullscreen,
		CmdSlideshow:       a.toggleSlideshow,
		CmdSlideshowToggle: a.toggleSlideshowPause,
		CmdInfo:            a.toggleInfo,
		CmdEscape:          a.escape,
		CmdCheckUpdates:    a.CheckForUpdates,
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
		CmdRotate180:       img,
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
		CmdSlideshow:       a.canSlideshow,
		CmdSlideshowToggle: a.canSlideshow,
		CmdEscape:          func() bool { return a.fullscreen || a.slideshowActive() || a.infoOpen },
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
