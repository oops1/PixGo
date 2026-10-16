package app

import "github.com/oops1/PixGo/internal/ui/window"

func (a *App) SetOps(ops window.WindowOps) {
	a.ops = ops
	a.Root.SetOps(ops)
}

func (a *App) toggleFullscreen() { a.SetFullscreen(!a.fullscreen) }

func (a *App) SetFullscreen(on bool) {
	if on == a.fullscreen {
		return
	}
	if !on && a.emulated {
		a.emulateFullscreen(false)
		return
	}
	a.fsWanted = on
	if a.ops.SetFullscreen != nil && a.ops.SetFullscreen(on) {
		return
	}
	a.emulateFullscreen(on)
}

func (a *App) OnFullscreenChanged(on bool) {
	if !on && a.fsWanted && !a.fullscreen {
		a.fsWanted = false
		a.emulateFullscreen(true)
		return
	}
	a.fullscreen = on
	a.fsWanted = on
	a.fullscreenSettled()
}

func (a *App) fullscreenSettled() {
	if a.fullscreen {
		a.Root.Wake()
	} else {
		a.Ctl.Slides().Stop()
		a.slideEnteredFullscreen = false
	}
	a.Root.Refresh()
	a.Ctl.Slides().Replan()
}

func (a *App) emulateFullscreen(on bool) {
	if on {
		a.wasMaximized = a.ops.Maximized != nil && a.ops.Maximized()
		if a.ops.Maximize != nil && !a.wasMaximized {
			a.ops.Maximize(true)
		}
	} else if a.ops.Maximize != nil && !a.wasMaximized {
		a.ops.Maximize(false)
	}
	a.emulated = on
	a.fullscreen = on
	a.fsWanted = on
	a.fullscreenSettled()
}

func (a *App) escape() {
	active := a.slideshowActive()
	a.slideEnteredFullscreen = false
	if active {
		a.Ctl.Slides().Stop()
	}
	if a.fullscreen {
		a.SetFullscreen(false)
		return
	}
	if a.infoOpen && !active {
		a.toggleInfo()
	}
}
