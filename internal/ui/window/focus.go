package window

import (
	"slices"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/ui/gfx"
)

type FocusGroup interface {
	FocusEnter(dir int)
	FocusStep(dir int) bool
	FocusLeave()
	FocusActive() bool
	HandleKey(e widget.KeyEvent) bool
}

func (r *Root) panelGroup() FocusGroup {
	g, _ := r.host.Panel().(FocusGroup)
	return g
}

func (r *Root) KeyboardFocus() (ZoneKind, bool) { return r.focus, r.kbd }

func (r *Root) focusStops() []ZoneKind {
	if r.host.Page() != PageViewer {
		return nil
	}
	l := r.fullOverlayLayout()
	var order []ZoneKind
	if l.Opt.Fullscreen {
		order = append(order, ZoneNone)
		order = append(order, slideZones[:3]...)
		order = append(order, arrowZones...)
		order = append(order, ZoneExit)
	} else {
		order = append(order, titleZones...)
		order = append(order, ZoneNone)
		order = append(order, toolZones...)
		order = append(order, arrowZones...)
		order = append(order, ZonePanel)
	}
	var out []ZoneKind
	for _, k := range order {
		switch {
		case k == ZoneNone:
		case k == ZonePanel:
			if r.panelGroup() == nil {
				continue
			}
		case l.Rect(k).Empty() || !r.zoneEnabled(k):
			continue
		}
		out = append(out, k)
	}
	return out
}

func (r *Root) tab(dir int) {
	r.Wake()
	if r.kbd && r.focus == ZonePanel {
		if g := r.panelGroup(); g != nil && g.FocusStep(dir) {
			r.eng.Invalidate()
			return
		}
	}
	stops := r.focusStops()
	if len(stops) == 0 {
		return
	}
	cur := slices.Index(stops, r.focus)
	if cur < 0 {
		cur = slices.Index(stops, ZoneNone)
	}
	r.setFocus(stops[(cur+dir+len(stops))%len(stops)], dir)
}

func (r *Root) setFocus(k ZoneKind, dir int) {
	old := r.focus
	if old == ZonePanel && k != ZonePanel {
		if g := r.panelGroup(); g != nil {
			g.FocusLeave()
		}
	}
	r.focus, r.kbd = k, true
	r.invalidateFocus(old)
	r.invalidateFocus(k)
	switch k {
	case ZonePanel:
		if g := r.panelGroup(); g != nil {
			g.FocusEnter(dir)
		}
		r.eng.Invalidate()
	case ZoneNone:
		r.FocusContent()
	}
	r.armHide()
}

func (r *Root) invalidateFocus(k ZoneKind) {
	if k == ZoneNone || k == ZonePanel {
		return
	}
	if rect := r.fullOverlayLayout().Rect(k); !rect.Empty() {
		r.invalidateRect(rect.Inset(-4))
	}
}

func (r *Root) clearKeyboardFocus() {
	if !r.kbd {
		return
	}
	r.kbd = false
	r.invalidateFocus(r.focus)
	if r.focus == ZonePanel {
		if g := r.panelGroup(); g != nil {
			g.FocusLeave()
		}
		r.eng.Invalidate()
	}
}

func (r *Root) PointerPressed() { r.clearKeyboardFocus() }

func (r *Root) kbdOverlay() bool { return r.kbd && overlayZone(r.focus) }

func (r *Root) validateFocus() {
	if r.focus == ZoneNone || r.host.Page() != PageViewer {
		return
	}
	if r.focus == ZonePanel {
		if r.panelGroup() == nil {
			r.focus = ZoneInfo
			r.invalidateFocus(ZoneInfo)
		}
		return
	}
	if !slices.Contains(r.focusStops(), r.focus) {
		r.focus = ZoneNone
	}
}

func (r *Root) handleFocusKey(e widget.KeyEvent) bool {
	if e.Code == widget.KeyTab && e.Mod&(widget.ModCtrl|widget.ModAlt) == 0 {
		dir := 1
		if e.Mod&widget.ModShift != 0 {
			dir = -1
		}
		r.tab(dir)
		return true
	}
	if !r.kbd || r.focus == ZoneNone {
		return false
	}
	if r.focus == ZonePanel {
		if g := r.panelGroup(); g != nil {
			return g.HandleKey(e)
		}
		return false
	}
	if e.Mod&(widget.ModCtrl|widget.ModAlt|widget.ModShift) != 0 {
		return false
	}
	switch e.Code {
	case widget.KeyEnter, widget.KeySpace:
		if !e.Repeat {
			r.activateFocused()
		}
		return true
	case widget.KeyEscape:
		if !r.host.Fullscreen() {
			r.setFocus(ZoneNone, 1)
			return true
		}
	}
	return false
}

func (r *Root) activateFocused() {
	k := r.focus
	r.activate(k)
	if k == ZoneInfo && r.host.InfoOpen() && r.panelGroup() != nil {
		r.setFocus(ZonePanel, 1)
	}
}

func (r *Root) drawFocus(c widget.DrawContext, l Layout) {
	if !r.kbd || r.focus == ZoneNone || r.focus == ZonePanel {
		return
	}
	z := l.Rect(r.focus)
	if z.Empty() {
		return
	}
	ring := gfx.RingOf(r.Theme)
	cx, cy := float64(z.Min.X)+float64(z.Dx())/2, float64(z.Min.Y)+float64(z.Dy())/2
	switch r.focus {
	case ZonePrev, ZoneNext:
		radius := arrowRadius
		if l.Opt.Fullscreen {
			radius = fullArrowRadius
			cy += fullArrowShiftY
		}
		ring.Circle(c, cx, cy, radius)
	case ZoneExit:
		ring.Circle(c, cx, cy, exitBackingRadius)
	case ZoneBack, ZoneInfo, ZoneFavorite, ZoneMore:
		ring.Rect(c, z, hoverRadius)
	default:
		ring.Rect(c, z.Inset(2), 6)
	}
}
