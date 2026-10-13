package app

import (
	"image"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/ui/window"
)

func (a *App) initSlideshow() {
	s := a.Ctl.Slides()
	s.Visible = func() bool { return a.fullscreen && a.Root.OverlayVisible() }
	s.OnState = a.Root.RefreshSlide
	s.OnProgress = a.Root.RefreshSlideProgress
	a.Root.OnOverlayChange = func(bool) { s.Replan() }
	a.Root.OnSlideSettings = a.showSlideshowMenu
}

func (a *App) Slideshow() window.SlideshowView {
	v := a.Ctl.Slides().View()
	return window.SlideshowView{Active: v.Active, Playing: v.Playing, Index: v.Index, Total: v.Total, Progress: v.Progress, Interval: v.Interval}
}

func (a *App) canSlideshow() bool {
	return a.page == window.PageViewer && a.Ctl.Slides().CanStart()
}

func (a *App) slideshowActive() bool { return a.Ctl.Slides().Active() }

func (a *App) startSlideshow() {
	if !a.canSlideshow() {
		return
	}
	if !a.fullscreen {
		a.slideEnteredFullscreen = true
		a.SetFullscreen(true)
	}
	a.Ctl.Slides().Start()
	a.Ctl.Slides().Replan()
}

func (a *App) stopSlideshow() {
	a.Ctl.Slides().Stop()
	leave := a.slideEnteredFullscreen && a.fullscreen
	a.slideEnteredFullscreen = false
	if leave {
		a.SetFullscreen(false)
	}
}

func (a *App) toggleSlideshow() {
	if a.slideshowActive() {
		a.stopSlideshow()
		return
	}
	a.startSlideshow()
}

func (a *App) toggleSlideshowPause() {
	if !a.slideshowActive() {
		a.startSlideshow()
		return
	}
	a.Ctl.Slides().TogglePause()
}

func (a *App) slideshowLabelKey() string {
	switch {
	case !a.slideshowActive():
		return "cmd.slideshow"
	default:
		return "cmd.slideshowStop"
	}
}

func (a *App) slideshowMenuItems() []widget.MenuItem {
	cfg := a.Settings.Get().Slideshow
	s := a.Ctl.Slides()
	var items []widget.MenuItem
	for _, sec := range SlideIntervals {
		sec := sec
		items = append(items, widget.MenuItem{
			Text:       i18n.T("slideshow.seconds", sec),
			Checkable:  true,
			Checked:    cfg.Interval == sec,
			RadioGroup: "slideshow.interval",
			OnClick:    func() { s.SetInterval(sec) },
		})
	}
	items = append(items,
		widget.MenuItem{Separator: true},
		widget.MenuItem{Text: i18n.T("slideshow.repeat"), Checkable: true, Checked: cfg.Loop, OnClick: func() { s.SetLoop(!cfg.Loop) }},
		widget.MenuItem{Text: i18n.T("slideshow.mix"), Checkable: true, Checked: cfg.Shuffle, OnClick: func() { s.SetShuffle(!cfg.Shuffle) }},
	)
	return items
}

func (a *App) showSlideshowMenu(anchor image.Rectangle) {
	if a.menu == nil {
		return
	}
	items := a.slideshowMenuItems()
	a.menu.SetItems(items)
	h := menuHeight(items)
	a.menu.Show(anchor.Min.X, max(0, anchor.Min.Y-4-h))
}
