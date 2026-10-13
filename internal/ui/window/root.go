package window

import (
	"image"
	"sync"
	"time"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/commands"
	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/ui/gfx"
	"github.com/oops1/PixGo/internal/ui/theme"
)

const DefaultHideDelay = 2500 * time.Millisecond

type Root struct {
	widget.Base
	host  Host
	eng   Engine
	ops   WindowOps
	Theme *theme.Theme

	Actions   map[ZoneKind]commands.ID
	After     func(d time.Duration, fn func()) (stop func())
	Now       func() time.Time
	HideDelay time.Duration

	focused bool

	ForceEnabled bool

	hover     ZoneKind
	pressed   ZoneKind
	pressing  bool
	moving    bool
	downX     int
	downY     int
	lastClick time.Time

	shown        bool
	stopHide     func()
	cursorHidden bool

	hitMu    sync.Mutex
	hitZones []Zone

	tip       tooltip
	toastText string
	stopToast func()

	OnMore          func(anchor image.Rectangle)
	OnSlideSettings func(anchor image.Rectangle)
	OnOverlayChange func(visible bool)

	lastChord    commands.Chord
	chordFromKey bool
}

func New(host Host, eng Engine, ops WindowOps) *Root {
	r := &Root{host: host, eng: eng, ops: ops, Theme: theme.Dark(), Now: time.Now, HideDelay: DefaultHideDelay, shown: true}
	r.Actions = DefaultActions()
	return r
}

func (r *Root) SetOps(ops WindowOps) { r.ops = ops }

func (r *Root) SetTheme(t *theme.Theme) {
	r.Theme = t
	r.eng.Invalidate()
}

func (r *Root) SetFocused(f bool) { r.focused = f }
func (r *Root) IsFocused() bool   { return r.focused }

func (r *Root) Children() []widget.Widget {
	var out []widget.Widget
	if c := r.host.Content(); c != nil {
		out = append(out, c)
	}
	if p := r.host.Panel(); p != nil {
		out = append(out, p)
	}
	if m := r.host.Menu(); m != nil {
		out = append(out, m)
	}
	return out
}

func (r *Root) size() (int, int) {
	b := r.Bounds()
	return b.Dx(), b.Dy()
}

func (r *Root) autoHide() bool {
	return r.host.Page() == PageViewer && (r.host.AutoHide() || r.host.Fullscreen())
}

func (r *Root) overlayVisible() bool { return !r.autoHide() || r.shown }

func (r *Root) OverlayVisible() bool { return r.host.Page() == PageViewer && r.overlayVisible() }

func (r *Root) options() Options {
	o := Options{
		Fullscreen: r.host.Fullscreen(),
		PanelOpen:  r.host.InfoOpen(),
		Resizable:  r.ops.BeginResize != nil || r.ops.MoveBy != nil,
		Page:       r.host.Page(),
	}
	if o.Fullscreen {
		o.CounterExtra = r.counterExtra()
	}
	if r.ops.Maximized != nil {
		o.Maximized = r.ops.Maximized()
	}
	if o.Page != PageViewer {
		o.Holes = r.host.HeaderHoles()
		return o
	}
	if r.overlayVisible() {
		o.Toolbar = true
		o.Arrows = r.host.State() == StateReady && (r.host.CanPrev() || r.host.CanNext())
	}
	if !o.Fullscreen {
		o.ZoomLabelW = textWidth(zoomFont, r.host.ZoomText())
	}
	return o
}

func (r *Root) counterExtra() int {
	wide := textWidth(slideCounterFont, r.host.Slideshow().widestCounterText())
	return max(0, wide-referenceTextWidth(slideCounterFont, i18n.T("slideshow.counter", 24, 24)))
}

func (r *Root) Layout() Layout {
	w, h := r.size()
	return Compute(w, h, r.options())
}

func (r *Root) fullOverlayLayout() Layout {
	w, h := r.size()
	o := r.options()
	o.Toolbar, o.Arrows = true, true
	return Compute(w, h, o)
}

func (r *Root) placeContent(l Layout) {
	if c := r.host.Content(); c != nil && c.Bounds() != l.View {
		c.SetBounds(l.View)
	}
	if p := r.host.Panel(); p != nil && p.Bounds() != l.Panel {
		p.SetBounds(l.Panel)
	}
}

func (r *Root) SetBounds(b image.Rectangle) {
	r.Base.SetBounds(b)
	r.placeContent(r.Layout())
}

func (r *Root) Refresh() {
	r.placeContent(r.Layout())
	r.eng.Invalidate()
}

func (r *Root) RefreshTitle() {
	w, _ := r.size()
	r.eng.InvalidateRect(image.Rect(0, 0, w, TitleHeight+1))
}

func (r *Root) RefreshOverlay() {
	l := r.fullOverlayLayout()
	r.eng.InvalidateRect(l.Toolbar.Inset(-3))
	r.eng.InvalidateRect(l.Prev.Inset(-3))
	r.eng.InvalidateRect(l.Next.Inset(-3))
	if r.host.Fullscreen() {
		r.eng.InvalidateRect(l.Rect(ZoneExit).Inset(-3))
	}
}

func (r *Root) RefreshSlide() {
	if !r.host.Fullscreen() || !r.OverlayVisible() {
		return
	}
	r.invalidateRect(r.fullOverlayLayout().Slide.Panel)
}

func (r *Root) RefreshSlideProgress() {
	if !r.host.Fullscreen() || !r.OverlayVisible() {
		return
	}
	if sl := r.fullOverlayLayout().Slide; sl.HasTrack() {
		r.eng.InvalidateRect(sl.Damage)
	}
}

func (r *Root) invalidateRect(rect image.Rectangle) {
	if !rect.Empty() {
		r.eng.InvalidateRect(rect.Inset(-2))
	}
}

func (r *Root) FocusContent() {
	if c := r.host.Content(); c != nil {
		r.eng.SetFocus(c)
		return
	}
	r.eng.SetFocus(r)
}

func (r *Root) Wake() {
	r.showCursor()
	changed := !r.shown
	r.shown = true
	if changed {
		r.Refresh()
		r.overlayChanged(true)
	}
	r.armHide()
}

func (r *Root) overlayChanged(visible bool) {
	if r.OnOverlayChange != nil {
		r.OnOverlayChange(visible)
	}
}

func (r *Root) menuOpen() bool {
	m, ok := r.host.Menu().(interface{ IsOpen() bool })
	return ok && m.IsOpen()
}

func (r *Root) armHide() {
	r.cancelHide()
	if !r.autoHide() || r.After == nil {
		return
	}
	r.stopHide = r.After(r.HideDelay, r.hideNow)
}

func (r *Root) cancelHide() {
	if r.stopHide != nil {
		r.stopHide()
		r.stopHide = nil
	}
}

func overlayZone(k ZoneKind) bool {
	return k >= ZonePrev && k <= ZoneSlideBar
}

func (r *Root) hideNow() {
	r.stopHide = nil
	if !r.autoHide() || !r.shown {
		return
	}
	if overlayZone(r.hover) || r.pressing || r.menuOpen() {
		r.armHide()
		return
	}
	r.shown = false
	r.hideCursor()
	r.Refresh()
	r.overlayChanged(false)
}

func (r *Root) hideCursor() {
	if !r.host.Fullscreen() || r.cursorHidden {
		return
	}
	if h, ok := r.eng.(CursorHider); ok {
		h.SetCursorOverride(widget.CursorNone)
		r.cursorHidden = true
	}
}

func (r *Root) showCursor() {
	if !r.cursorHidden {
		return
	}
	r.cursorHidden = false
	if h, ok := r.eng.(CursorHider); ok {
		h.ClearCursorOverride()
	}
}

func (r *Root) HitTest(x, y int) int {
	r.hitMu.Lock()
	defer r.hitMu.Unlock()
	pt := image.Pt(x, y)
	for _, z := range r.hitZones {
		if pt.In(z.Rect) {
			return HitAreaOf(z.Kind)
		}
	}
	return HitClient
}

func (r *Root) cacheZones(l Layout) {
	r.hitMu.Lock()
	r.hitZones = l.Zones
	r.hitMu.Unlock()
}

func (r *Root) Draw(c widget.DrawContext) {
	l := r.Layout()
	r.placeContent(l)
	r.cacheZones(l)
	r.drawBackdrop(c, l)
	if l.Opt.Page != PageViewer {
		gfx.DrawChildren(c, r)
		r.drawGalleryCaptions(c, l)
		r.drawToast(c, l)
		r.drawTip(c)
		r.drawFrame(c, l)
		return
	}
	if !l.Title.Empty() {
		r.drawTitle(c, l)
	}
	gfx.DrawChildren(c, r)
	r.drawState(c, l)
	r.drawOverlay(c, l)
	r.drawToast(c, l)
	r.drawTip(c)
	r.drawFrame(c, l)
}
