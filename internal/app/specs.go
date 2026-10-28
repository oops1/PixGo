package app

import "github.com/oops1/PixGo/internal/commands"

const (
	CmdPrev            commands.ID = "nav.prev"
	CmdNext            commands.ID = "nav.next"
	CmdFirst           commands.ID = "nav.first"
	CmdLast            commands.ID = "nav.last"
	CmdZoomIn          commands.ID = "view.zoomIn"
	CmdZoomOut         commands.ID = "view.zoomOut"
	CmdZoomActual      commands.ID = "view.zoomActual"
	CmdZoomFit         commands.ID = "view.zoomFit"
	CmdZoomWidth       commands.ID = "view.zoomWidth"
	CmdRotateRight     commands.ID = "view.rotateRight"
	CmdRotateLeft      commands.ID = "view.rotateLeft"
	CmdRotate180       commands.ID = "view.rotate180"
	CmdFlipH           commands.ID = "view.flipH"
	CmdFlipV           commands.ID = "view.flipV"
	CmdFullscreen      commands.ID = "view.fullscreen"
	CmdInfo            commands.ID = "view.info"
	CmdGallery         commands.ID = "view.gallery"
	CmdOpen            commands.ID = "file.open"
	CmdSaveAs          commands.ID = "file.saveAs"
	CmdPrint           commands.ID = "file.print"
	CmdExportPDF       commands.ID = "file.exportPdf"
	CmdCopy            commands.ID = "file.copy"
	CmdCopyImage       commands.ID = "file.copyImage"
	CmdCopyPath        commands.ID = "file.copyPath"
	CmdDelete          commands.ID = "file.delete"
	CmdDeletePermanent commands.ID = "file.deletePermanent"
	CmdReveal          commands.ID = "file.reveal"
	CmdFavorite        commands.ID = "file.favorite"
	CmdSettings        commands.ID = "app.settings"
	CmdCheckUpdates    commands.ID = "app.checkUpdates"
	CmdSlideshow       commands.ID = "slideshow.start"
	CmdSlideshowToggle commands.ID = "slideshow.toggle"
	CmdEscape          commands.ID = "app.escape"
	CmdBack            commands.ID = "app.back"
	CmdMore            commands.ID = "app.more"
	CmdZoomToggle      commands.ID = "view.zoomToggle"
	CmdEditStart       commands.ID = "edit.start"
	CmdView3D          commands.ID = "view.3d"
	CmdCrop            commands.ID = "edit.crop"
	CmdResize          commands.ID = "edit.resize"
)

type Spec struct {
	ID    commands.ID
	Title string
	Icon  string
	Keys  []commands.Chord
}

var Specs = []Spec{
	{CmdPrev, "cmd.prev", "chevron-left", []commands.Chord{commands.C(0, commands.KeyLeft), commands.C(0, commands.KeyPageUp)}},
	{CmdNext, "cmd.next", "chevron-right", []commands.Chord{commands.C(0, commands.KeyRight), commands.C(0, commands.KeyPageDown)}},
	{CmdFirst, "cmd.first", "", []commands.Chord{commands.C(0, commands.KeyHome)}},
	{CmdLast, "cmd.last", "", []commands.Chord{commands.C(0, commands.KeyEnd)}},
	{CmdZoomIn, "cmd.zoomIn", "zoom-in", []commands.Chord{commands.C(0, commands.KeyPlus), commands.C(commands.Ctrl, commands.KeyPlus), commands.C(commands.Shift, commands.KeyPlus), commands.C(commands.Ctrl|commands.Shift, commands.KeyPlus)}},
	{CmdZoomOut, "cmd.zoomOut", "zoom-out", []commands.Chord{commands.C(0, commands.KeyMinus), commands.C(commands.Ctrl, commands.KeyMinus)}},
	{CmdZoomActual, "cmd.zoomActual", "", []commands.Chord{commands.C(0, '0'), commands.C(commands.Ctrl, '0')}},
	{CmdZoomFit, "cmd.zoomFit", "", []commands.Chord{commands.C(0, '1')}},
	{CmdZoomWidth, "cmd.zoomWidth", "", []commands.Chord{commands.C(0, '2')}},
	{CmdRotateRight, "cmd.rotateRight", "rotate-right", []commands.Chord{commands.C(0, 'R')}},
	{CmdRotateLeft, "cmd.rotateLeft", "rotate-left", []commands.Chord{commands.C(commands.Shift, 'R')}},
	{CmdRotate180, "cmd.rotate180", "", nil},
	{CmdFlipH, "cmd.flipH", "", []commands.Chord{commands.C(0, 'H')}},
	{CmdFlipV, "cmd.flipV", "", []commands.Chord{commands.C(0, 'V')}},
	{CmdFullscreen, "cmd.fullscreen", "fullscreen", []commands.Chord{commands.C(0, commands.KeyF11), commands.C(commands.Alt, commands.KeyEnter)}},
	{CmdInfo, "cmd.info", "info", []commands.Chord{commands.C(commands.Ctrl, 'I')}},
	{CmdGallery, "cmd.gallery", "gallery", []commands.Chord{commands.C(commands.Ctrl, 'G')}},
	{CmdOpen, "cmd.open", "gallery", []commands.Chord{commands.C(commands.Ctrl, 'O')}},
	{CmdSaveAs, "cmd.saveAs", "", []commands.Chord{commands.C(commands.Ctrl|commands.Shift, 'S')}},
	{CmdPrint, "cmd.print", "", []commands.Chord{commands.C(commands.Ctrl, 'P')}},
	{CmdExportPDF, "cmd.exportPdf", "", nil},
	{CmdCopy, "cmd.copy", "copy", []commands.Chord{commands.C(commands.Ctrl, 'C')}},
	{CmdCopyImage, "cmd.copyImage", "", nil},
	{CmdCopyPath, "cmd.copyPath", "", []commands.Chord{commands.C(commands.Ctrl|commands.Shift, 'C')}},
	{CmdDelete, "cmd.delete", "delete", []commands.Chord{commands.C(0, commands.KeyDelete)}},
	{CmdDeletePermanent, "cmd.deletePermanent", "", []commands.Chord{commands.C(commands.Shift, commands.KeyDelete)}},
	{CmdReveal, "cmd.reveal", "", []commands.Chord{commands.C(commands.Ctrl|commands.Shift, 'R')}},
	{CmdFavorite, "cmd.favorite.add", "heart", []commands.Chord{commands.C(commands.Ctrl, 'D')}},
	{CmdSettings, "cmd.settings", "settings", []commands.Chord{commands.C(commands.Ctrl, commands.KeyComma)}},
	{CmdCheckUpdates, "cmd.checkUpdates", "", nil},
	{CmdSlideshow, "cmd.slideshow", "play", []commands.Chord{commands.C(0, commands.KeyF5)}},
	{CmdSlideshowToggle, "cmd.slideshowPause", "pause", []commands.Chord{commands.C(0, commands.KeySpace)}},
	{CmdEscape, "dialog.cancel", "", []commands.Chord{commands.C(0, commands.KeyEscape)}},
	{CmdBack, "cmd.back", "back", nil},
	{CmdMore, "cmd.more", "more", nil},
	{CmdZoomToggle, "zoom.label", "", nil},
	{CmdEditStart, "cmd.edit", "edit", []commands.Chord{commands.C(commands.Ctrl, 'E')}},
	{CmdCrop, "cmd.crop", "crop", nil},
	{CmdView3D, "cmd.view3d", "cube-3d", []commands.Chord{commands.C(0, '3')}},
	{CmdResize, "cmd.resize", "resize", nil},
}
