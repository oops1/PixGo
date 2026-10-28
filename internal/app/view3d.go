package app

import (
	"context"
	"image/color"

	"github.com/oops1/PixGo/internal/commands"
	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/model3d"
	"github.com/oops1/PixGo/internal/render"
	"github.com/oops1/PixGo/internal/render3d"
	"github.com/oops1/PixGo/internal/ui/theme"
	"github.com/oops1/PixGo/internal/ui/view3d"
	"github.com/oops1/PixGo/internal/ui/window"
)

var blockedIn3D = map[commands.ID]bool{
	CmdFlipH: true, CmdFlipV: true, CmdEditStart: true, CmdCrop: true, CmdResize: true,
	CmdSaveAs: true, CmdCopyImage: true, CmdPrint: true, CmdExportPDF: true,
	CmdSlideshow: true, CmdSlideshowToggle: true, CmdZoomWidth: true,
}

type model3DState struct {
	active  bool
	path    string
	loading bool
	cancel  context.CancelFunc
	hinted  bool
	model   bool
}

func (a *App) initView3D() {
	v := view3d.New(a.Eng)
	v.Chrome = a.Root.Owns
	v.After = a.Root.After
	v.OnContextMenu = a.showMenuAt
	v.OnChange = func() {
		a.Root.RefreshOverlay()
		a.Root.Wake()
	}
	a.View3D = v
}

func (a *App) ShowsModel() bool {
	return a.page == window.PageViewer && a.Ctl.Path() != "" && a.hasImage() && a.Ctl.Info().Model
}

func (a *App) Viewing3D() bool { return a.m3d.active }

func (a *App) canView3D() bool {
	return a.ShowsModel() && !a.editing && !a.slideshowActive()
}

func (a *App) toggleView3D() {
	if a.m3d.active {
		a.exitView3D()
		return
	}
	a.enterView3D()
}

func (a *App) enterView3D() {
	if !a.canView3D() {
		return
	}
	a.m3d.active = true
	a.View3D.Background = a.Root.Theme.Canvas
	a.load3D(a.Ctl.Path())
	if !a.m3d.hinted {
		a.m3d.hinted = true
		a.Root.Toast(i18n.T("view3d.hint"))
	}
	a.afterModeChange()
}

func (a *App) exitView3D() {
	if !a.m3d.active {
		return
	}
	a.m3d.active = false
	a.stopLoad3D()
	a.m3d.path = ""
	a.View3D.Clear()
	a.afterModeChange()
}

func (a *App) afterModeChange() {
	a.Root.Refresh()
	a.Root.FocusContent()
	a.Eng.Invalidate()
}

func (a *App) stopLoad3D() {
	if a.m3d.cancel != nil {
		a.m3d.cancel()
		a.m3d.cancel = nil
	}
	a.m3d.loading = false
}

func (a *App) load3D(path string) {
	if path == a.m3d.path && (a.m3d.loading || a.View3D.HasScene()) {
		return
	}
	a.stopLoad3D()
	a.View3D.Clear()
	a.m3d.path = path
	a.m3d.loading = true
	ctx, cancel := context.WithCancel(context.Background())
	a.m3d.cancel = cancel
	go func() {
		m, _, err := model3d.Load(ctx, path)
		var sc *render3d.Scene
		if err == nil {
			sc = render3d.NewScene(m)
		}
		a.Eng.Post(func() {
			if ctx.Err() != nil || !a.m3d.active || a.m3d.path != path {
				return
			}
			a.m3d.loading = false
			a.m3d.cancel = nil
			if err != nil {
				a.m3d.active = false
				a.m3d.path = ""
				a.Root.Toast(i18n.T("view3d.failed"))
				a.afterModeChange()
				return
			}
			a.View3D.SetScene(sc)
			a.afterModeChange()
		})
	}()
}

func (a *App) sync3D() {
	model := a.ShowsModel()
	if model != a.m3d.model {
		a.m3d.model = model
		a.Canvas.SetBackground(a.background(a.Root.Theme))
	}
	if !a.m3d.active {
		return
	}
	if a.editing || a.slideshowActive() || a.page != window.PageViewer {
		a.exitView3D()
		return
	}
	path := a.Ctl.Path()
	if path != a.m3d.path {
		a.stopLoad3D()
		a.View3D.Clear()
		a.m3d.path = ""
	}
	switch {
	case a.Ctl.Status() == StatusLoading:
	case model:
		a.load3D(path)
	default:
		a.exitView3D()
	}
}

func (a *App) modelBackground(t *theme.Theme) (render.Background, bool) {
	if !a.m3d.model {
		return render.Background{}, false
	}
	return render.Background{Canvas: t.Canvas, Kind: render.BgSolid, Fill: t.Canvas, Checker: [2]color.RGBA{t.Checker1, t.Checker2}, Cell: 8}, true
}

func (a *App) or3D(plain, in3D func()) func() {
	return func() {
		if a.m3d.active && a.View3D.HasScene() {
			in3D()
			return
		}
		if a.m3d.active {
			return
		}
		plain()
	}
}

func (a *App) zoomText3D() (string, bool) {
	if !a.m3d.active || !a.View3D.HasScene() {
		return "", false
	}
	return i18n.T("zoom.percent", a.View3D.ZoomPercent()), true
}
