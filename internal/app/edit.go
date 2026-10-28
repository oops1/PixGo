package app

import (
	"image"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/commands"
	coreedit "github.com/oops1/PixGo/internal/edit"
	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/settings"
	"github.com/oops1/PixGo/internal/ui/confirm"
	uiedit "github.com/oops1/PixGo/internal/ui/edit"
	"github.com/oops1/PixGo/internal/ui/resize"
	"github.com/oops1/PixGo/internal/ui/window"
)

const (
	maxSidePixels = 65535
	maxSideWebP   = 16383
)

func (a *App) Editing() bool { return a.editing }

func (a *App) initEdit() {
	e := a.Editor
	e.After = a.Root.After
	e.Chrome = a.Root.Owns
	e.On = uiedit.Handlers{
		AspectMenu: a.showAspectMenu,
		SaveMenu:   a.showEditSaveMenu,
		SaveCopy:   a.saveEditCopy,
		Cancel:     a.cancelEdit,
		Resize:     a.openResize,
		Changed:    a.editChanged,
	}
}

var editAllowed = map[commands.ID]bool{
	CmdEscape: true, CmdRotateRight: true, CmdRotateLeft: true, CmdRotate180: true, CmdFlipH: true, CmdFlipV: true,
	CmdInfo: true, CmdFavorite: true, CmdCopy: true, CmdCopyPath: true, CmdCopyImage: true, CmdReveal: true,
	CmdSaveAs: true, CmdPrint: true, CmdExportPDF: true, CmdEditStart: true, CmdCrop: true, CmdResize: true, CmdContextMenu: true,
}

func (a *App) gateEnabled(id commands.ID, f func() bool) func() bool {
	return func() bool {
		if a.editing && !editAllowed[id] {
			return false
		}
		if a.m3d.active && blockedIn3D[id] {
			return false
		}
		if f == nil {
			return true
		}
		return f()
	}
}

func (a *App) rotateOrEdit(cw bool, plain func()) func() {
	return func() {
		if a.editing {
			a.Editor.Rotate(cw)
			return
		}
		plain()
	}
}

func (a *App) rotate180OrEdit() {
	if a.editing {
		a.Editor.Rotate(true)
		a.Editor.Rotate(true)
		return
	}
	a.or3D(a.Canvas.Rotate180, func() {
		a.View3D.Turn(true)
		a.View3D.Turn(true)
	})()
}

func (a *App) flipOrEdit(h bool, plain func()) func() {
	return func() {
		if a.editing {
			if h {
				a.Editor.FlipH()
			} else {
				a.Editor.FlipV()
			}
			return
		}
		plain()
	}
}

func (a *App) editRefusal() string {
	switch {
	case a.page != window.PageViewer || a.Ctl.Path() == "" || !a.hasImage():
		return "edit.refused.none"
	}
	info := a.Ctl.Info()
	switch {
	case info.Model:
		return "edit.refused.model"
	case info.Vector || info.Format == codec.SVG:
		return "edit.refused.vector"
	case info.Animated || info.Frames > 1:
		return "edit.refused.animated"
	}
	shown := a.Ctl.Shown()
	switch {
	case a.Ctl.Status() != StatusReady || shown == nil:
		return "edit.refused.loading"
	case shown.Detail == nil && !shown.FullResolution():
		return "edit.refused.loading"
	}
	return ""
}

func (a *App) canStartEdit() bool {
	return a.page == window.PageViewer && a.hasImage() && !a.fullscreen && !a.slideshowActive()
}

func (a *App) startEdit() { a.beginEdit(false) }

func (a *App) startResize() { a.beginEdit(true) }

func (a *App) beginEdit(thenResize bool) {
	if a.editing {
		if thenResize {
			a.openResize()
		}
		return
	}
	if key := a.editRefusal(); key != "" {
		a.Root.Toast(i18n.T(key))
		return
	}
	a.Editor.Theme = a.Root.Theme
	if !a.Editor.Begin() {
		return
	}
	a.editing = true
	a.editPath = a.Ctl.Path()
	a.Root.Refresh()
	a.Eng.SetFocus(a.Editor)
	a.Eng.Invalidate()
	if thenResize {
		a.openResize()
	}
}

func (a *App) cancelEdit() {
	if !a.editing || a.Editor.Busy() {
		return
	}
	if a.Editor.Dirty() {
		a.confirm(i18n.T("edit.discard"), i18n.T("edit.discardText"), a.endEdit)
		return
	}
	a.endEdit()
}

func (a *App) endEdit() {
	if !a.editing {
		return
	}
	a.snapOff()
	if a.resizeDlg != nil {
		a.Eng.CloseModal(a.resizeDlg.Widget())
		a.resizeDlg = nil
	}
	a.editing = false
	a.editPath = ""
	a.Editor.End()
	a.Root.Refresh()
	a.focusPage()
}

func (a *App) editChanged() {
	a.Root.Refresh()
}

func (a *App) syncEdit() {
	if !a.editing {
		return
	}
	if a.Ctl.Path() != a.editPath || !a.hasImage() {
		a.endEdit()
	}
}

func (a *App) showAspectMenu(anchor image.Rectangle) {
	cur := a.Editor.Crop().Aspect()
	var items []widget.MenuItem
	for _, asp := range uiedit.Aspects {
		asp := asp
		items = append(items, widget.MenuItem{
			Text:       i18n.T(uiedit.AspectKey(asp)),
			Checkable:  true,
			Checked:    asp == cur,
			RadioGroup: "edit.aspect",
			OnClick: func() {
				a.Editor.SetAspect(asp)
				a.Editor.MenuClosed()
			},
		})
	}
	a.menu.SetItems(items)
	a.menu.Show(anchor.Min.X, anchor.Max.Y+4)
}

func (a *App) showEditSaveMenu(anchor image.Rectangle) {
	items := []widget.MenuItem{
		{
			Text:     i18n.T("edit.save"),
			Disabled: !a.canOverwrite(),
			OnClick:  func() { a.Editor.MenuClosed(); a.saveEditOverwrite() },
		},
		{
			Text:    i18n.T("edit.saveAs"),
			OnClick: func() { a.Editor.MenuClosed(); a.saveEditAs() },
		},
	}
	a.menu.SetItems(items)
	a.menu.Show(max(0, anchor.Max.X-220), anchor.Max.Y+4)
}

func (a *App) maxSide() int {
	if a.Ctl.Info().Format == codec.WebP {
		return maxSideWebP
	}
	return maxSidePixels
}

func (a *App) openResize() {
	if !a.editing || a.resizeDlg != nil || a.Editor.Busy() {
		return
	}
	lim := resize.Limits{MaxSide: a.maxSide(), MaxPixels: a.Ctl.Limits().MaxPixels}
	a.resizeDlg = resize.Show(a.Eng, a.Editor.CropSize(), a.Editor.OutputSize(), lim, func(size image.Point, ok bool) {
		a.Eng.Post(func() {
			a.resizeDlg = nil
			if ok {
				a.Editor.SetOutputSize(size.X, size.Y)
			}
			a.focusPage()
		})
	})
}

func (a *App) snapOff() {
	if a.Editor != nil {
		a.Editor.SetSnapped(image.Rectangle{})
	}
}

func (a *App) snapJob(src *losslessSource) func(bool) {
	return func(on bool) {
		if !on || src == nil || src.crop.Empty() {
			a.Editor.SetSnapped(image.Rectangle{})
			return
		}
		if r, err := src.snapped(); err == nil {
			a.Editor.SetSnapped(r)
		}
	}
}

func (a *App) editTargetID() (codec.ID, bool) {
	var id codec.ID
	switch a.Ctl.Info().Format {
	case codec.JPEG:
		id = codec.JPEG
	case codec.PNG, codec.APNG:
		id = codec.PNG
	case codec.WebP:
		id = codec.WebP
	case codec.BMP:
		id = codec.BMP
	}
	if id != "" {
		if _, ok := a.Ctl.env.Registry.Encoder(id); ok {
			return id, true
		}
	}
	return codec.PNG, false
}

func (a *App) canOverwrite() bool {
	_, exact := a.editTargetID()
	return a.editing && exact && !a.Editor.Busy()
}

func (a *App) typeOf(id codec.ID) (saveType, bool) {
	for _, t := range a.saveTypes() {
		if t.id == id {
			return t, true
		}
	}
	return saveType{}, false
}

func storedRectOf(o codec.Orientation, r image.Rectangle, stored image.Point) image.Rectangle {
	w, h := float64(stored.X), float64(stored.Y)
	x0, y0 := o.ToSource(float64(r.Min.X), float64(r.Min.Y), w, h)
	x1, y1 := o.ToSource(float64(r.Max.X), float64(r.Max.Y), w, h)
	return image.Rect(int(min(x0, x1)+.5), int(min(y0, y1)+.5), int(max(x0, x1)+.5), int(max(y0, y1)+.5))
}

func (a *App) captureEditJob() *saveJob {
	if !a.editing {
		return nil
	}
	plan := a.Editor.Plan()
	stored := a.Editor.StoredSize()
	path := a.Ctl.Path()
	info := a.Ctl.Info()
	job := &saveJob{meta: &metaSource{path: path, format: info.Format, swap: plan.Orientation.Transposes()}}
	if shown := a.Ctl.Shown(); shown != nil {
		job.meta.converted = shown.Color.Converted
		job.meta.display = a.Ctl.Color().DisplayICC()
	}
	if d := a.Ctl.Decoded(); d != nil && len(d.Levels) > 0 {
		src := d.Levels[0]
		job.produce = func(ctx contextT) (*image.NRGBA, error) { return coreedit.Apply(ctx, src, plan) }
	} else {
		reg, lim := a.Ctl.Registry(), a.Ctl.Limits()
		job.produce = func(ctx contextT) (*image.NRGBA, error) {
			return applyTiled(ctx, reg, lim, path, plan, stored)
		}
	}
	if plan.LosslessCapable(stored.X, stored.Y) {
		job.lossless = a.losslessSourceFor(plan.Crop)
		if job.lossless != nil && !plan.Crop.Empty() {
			job.snap = a.snapJob(job.lossless)
		}
	}
	return job
}

func (a *App) copyName(dir, base, ext string) string {
	for n := 2; ; n++ {
		name := base + " (" + strconv.Itoa(n) + ")" + ext
		if _, err := os.Stat(filepath.Join(dir, name)); os.IsNotExist(err) {
			return name
		}
	}
}

func (a *App) editNames() (dir, base, ext string, id codec.ID, exact bool) {
	src := a.Ctl.Path()
	dir = filepath.Dir(src)
	ext = filepath.Ext(src)
	base = strings.TrimSuffix(filepath.Base(src), ext)
	id, exact = a.editTargetID()
	if !exact {
		ext = ".png"
	} else if t, ok := a.typeOf(id); ok {
		if _, known := typeForPath([]saveType{t}, src); !known {
			ext = t.ext
		}
	}
	return
}

func (a *App) editJobReady() (*saveJob, bool) {
	if !a.editing || a.Editor.Busy() || !a.Editor.Dirty() || a.saveDlg != nil {
		return nil, false
	}
	job := a.captureEditJob()
	return job, job != nil
}

func (a *App) saveEditCopy() {
	job, ok := a.editJobReady()
	if !ok {
		return
	}
	dir, base, ext, id, exact := a.editNames()
	types := a.saveTypes()
	if len(types) == 0 {
		return
	}
	if !exact {
		a.Root.Toast(i18n.T("edit.noEncoder"))
	}
	name := a.copyName(dir, base, ext)
	job.force = id
	job.done = func(path string, o saveOutcome) {
		a.finishEdit(false, filepath.Base(path), o)
	}
	a.writeImage(filepath.Join(dir, name), types, job)
}

func (a *App) saveEditAs() {
	job, ok := a.editJobReady()
	if !ok {
		return
	}
	dir, base, ext, _, _ := a.editNames()
	_ = dir
	job.done = func(path string, o saveOutcome) {
		a.finishEdit(false, filepath.Base(path), o)
	}
	a.runSaveAs(job, a.copyName(filepath.Dir(a.Ctl.Path()), base, ext))
}

func (a *App) saveEditOverwrite() {
	job, ok := a.editJobReady()
	if !ok {
		return
	}
	if !a.canOverwrite() {
		a.Root.Toast(i18n.T("edit.overwriteOff"))
		return
	}
	id, _ := a.editTargetID()
	path := a.Ctl.Path()
	types := a.saveTypes()
	job.force = id
	job.done = func(p string, o saveOutcome) {
		a.finishEdit(true, filepath.Base(p), o)
	}
	run := func() { a.writeImage(path, types, job) }
	if !a.Settings.Get().ConfirmOverwrite {
		run()
		return
	}
	if a.ovConfirm != nil {
		return
	}
	a.ovConfirm = confirm.Show(a.Eng, confirm.Config{
		Title:   i18n.T("dialog.overwrite.title"),
		Message: i18n.T("dialog.overwrite.message", filepath.Base(path)),
		Yes:     i18n.T("dialog.overwrite.yes"),
		Check:   i18n.T("dialog.dontAskAgain"),
	}, func(r confirm.Result) {
		a.Eng.Post(func() {
			a.ovConfirm = nil
			a.focusPage()
			if !r.Yes {
				return
			}
			if r.Checked {
				a.Settings.Update(func(s *settings.Settings) { s.ConfirmOverwrite = false })
			}
			run()
		})
	})
}

func (a *App) finishEdit(overwritten bool, name string, o saveOutcome) {
	switch {
	case o.droppedEXIF && !overwritten:
		a.Root.Toast(i18n.T("toast.copySavedNoExif", name))
	case o.droppedEXIF:
		a.Root.Toast(i18n.T("toast.savedNoExif"))
	case overwritten:
		a.Root.Toast(i18n.T("toast.overwritten"))
	default:
		a.Root.Toast(i18n.T("toast.copySaved", name))
	}
	a.endEdit()
	if overwritten {
		a.Ctl.Reload()
	}
	a.Ctl.RescanAsync()
}
