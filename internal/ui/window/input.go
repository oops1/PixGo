package window

import (
	"image"
	"time"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/commands"
)

const doubleClickInterval = 500 * time.Millisecond

func (r *Root) zoneAt(x, y int) Zone {
	return r.Layout().ZoneAt(image.Pt(x, y))
}

func resizeEdges(k ZoneKind) int {
	switch k {
	case ZoneResizeTop:
		return EdgeTop
	case ZoneResizeBottom:
		return EdgeBottom
	case ZoneResizeLeft:
		return EdgeLeft
	case ZoneResizeRight:
		return EdgeRight
	case ZoneResizeTopLeft:
		return EdgeTop | EdgeLeft
	case ZoneResizeTopRight:
		return EdgeTop | EdgeRight
	case ZoneResizeBottomLeft:
		return EdgeBottom | EdgeLeft
	case ZoneResizeBottomRight:
		return EdgeBottom | EdgeRight
	}
	return 0
}

func isResize(k ZoneKind) bool { return k >= ZoneResizeTop }

func (r *Root) Cursor(x, y int) widget.Cursor {
	cur, _ := r.CursorAt(x, y)
	return cur
}

func (r *Root) CursorAt(x, y int) (widget.Cursor, bool) {
	z := r.zoneAt(x, y)
	switch z.Kind {
	case ZoneNone:
		return widget.CursorArrow, false
	case ZoneResizeTop, ZoneResizeBottom:
		return widget.CursorSizeNS, true
	case ZoneResizeLeft, ZoneResizeRight:
		return widget.CursorSizeWE, true
	case ZoneResizeTopLeft, ZoneResizeBottomRight:
		return widget.CursorSizeNWSE, true
	case ZoneResizeTopRight, ZoneResizeBottomLeft:
		return widget.CursorSizeNESW, true
	}
	return widget.CursorArrow, true
}

func (r *Root) setHover(k ZoneKind) {
	if k == r.hover {
		return
	}
	old := r.hover
	r.hover = k
	r.invalidateZone(old)
	r.invalidateZone(k)
	r.hideTip()
	r.scheduleTip(k)
}

func (r *Root) invalidateZone(k ZoneKind) {
	if k == ZoneNone || k == ZoneDrag || k == ZoneSlideBar || isResize(k) {
		return
	}
	l := r.fullOverlayLayout()
	rect := l.Rect(k)
	if k == ZonePrev || k == ZoneNext {
		rect = rect.Inset(-3)
	}
	r.invalidateRect(rect)
}

func (r *Root) OnMouseMove(x, y int) {
	if widget.CursorIsNowhere(x, y) {
		r.setHover(ZoneNone)
		return
	}
	if r.moving {
		if r.ops.MoveBy != nil {
			r.ops.MoveBy(x-r.downX, y-r.downY)
		}
		return
	}
	r.Wake()
	r.setHover(r.zoneAt(x, y).Kind)
}

func (r *Root) WantsCapture(e widget.MouseEvent) bool {
	if e.Button != widget.MouseLeft || !e.Pressed {
		return false
	}
	k := r.zoneAt(e.X, e.Y).Kind
	return k != ZoneNone && !isResize(k)
}

func (r *Root) OnMouseButton(e widget.MouseEvent) bool {
	if e.Button == widget.MouseButtonX1 || e.Button == widget.MouseButtonX2 {
		if r.host.Page() != PageViewer {
			return false
		}
		if e.Pressed {
			k := ZonePrev
			if e.Button == widget.MouseButtonX2 {
				k = ZoneNext
			}
			r.activate(k)
		}
		return true
	}
	if e.Button != widget.MouseLeft {
		return false
	}
	if e.Pressed {
		return r.press(e)
	}
	return r.release(e)
}

func (r *Root) press(e widget.MouseEvent) bool {
	z := r.zoneAt(e.X, e.Y)
	if z.Kind == ZoneNone {
		return false
	}
	if isResize(z.Kind) {
		if r.ops.BeginResize != nil && r.ops.BeginResize(resizeEdges(z.Kind)) {
			return true
		}
		return false
	}
	r.hideTip()
	r.pressed, r.pressing = z.Kind, true
	r.downX, r.downY = e.X, e.Y
	r.invalidateZone(z.Kind)
	if z.Kind != ZoneDrag {
		return true
	}
	now := r.Now()
	double := now.Sub(r.lastClick) < doubleClickInterval
	r.lastClick = now
	if double {
		r.lastClick = time.Time{}
		r.pressing = false
		if r.ops.ToggleMaximize != nil {
			r.ops.ToggleMaximize()
		}
		return true
	}
	if r.ops.BeginMove != nil && r.ops.BeginMove() {
		r.pressing = false
		return true
	}
	r.moving = r.ops.MoveBy != nil
	return true
}

func (r *Root) release(e widget.MouseEvent) bool {
	wasMoving := r.moving
	r.moving = false
	if !r.pressing {
		return wasMoving
	}
	r.pressing = false
	k := r.pressed
	r.pressed = ZoneNone
	r.invalidateZone(k)
	if wasMoving {
		return true
	}
	z := r.zoneAt(e.X, e.Y)
	if z.Kind != k {
		return true
	}
	r.activate(k)
	return true
}

func (r *Root) activate(k ZoneKind) {
	switch k {
	case ZoneMin:
		if r.ops.Minimize != nil {
			r.ops.Minimize()
		}
		return
	case ZoneMax:
		if r.ops.ToggleMaximize != nil {
			r.ops.ToggleMaximize()
		}
		return
	case ZoneClose:
		if r.ops.Close != nil {
			r.ops.Close()
		}
		return
	case ZoneDrag, ZoneSlideBar:
		return
	}
	if (k == ZoneMore || k == ZoneToolMore) && r.OnMore != nil {
		r.OnMore(r.fullOverlayLayout().Rect(k))
		return
	}
	if k == ZoneSlideSettings && r.OnSlideSettings != nil {
		r.OnSlideSettings(r.fullOverlayLayout().Rect(k))
		return
	}
	if cmd := r.command(k); cmd != nil && cmd.IsEnabled() {
		cmd.Run()
		r.Refresh()
	}
}

var galleryCommands = map[commands.ID]bool{
	"app.escape":   true,
	"view.gallery": true,
	"file.open":    true,
}

func (r *Root) handleGalleryKey(e widget.KeyEvent) bool {
	if h, ok := r.host.Content().(KeyHandler); ok && h.HandleKey(e) {
		return true
	}
	ch, ok := ChordOf(e)
	if !ok {
		return false
	}
	cmd := r.host.Registry().ForChord(ch)
	if cmd == nil || !galleryCommands[cmd.ID] {
		return false
	}
	if cmd.IsEnabled() {
		cmd.Run()
		r.Refresh()
	}
	return true
}

type modalAware interface{ ModalOpen() bool }

func (r *Root) modalOpen() bool {
	m, ok := r.host.(modalAware)
	return ok && m.ModalOpen()
}

func (r *Root) HandleKey(e widget.KeyEvent) bool {
	if !e.Pressed || r.modalOpen() {
		return false
	}
	if r.host.Page() != PageViewer {
		return r.handleGalleryKey(e)
	}
	ch, ok := ChordOf(e)
	if !ok {
		return false
	}
	fromRune := e.Code == widget.KeyUnknown
	if fromRune && r.chordFromKey && ch == r.lastChord {
		r.chordFromKey = false
		return true
	}
	cmd := r.host.Registry().ForChord(ch)
	if cmd == nil {
		return false
	}
	r.lastChord, r.chordFromKey = ch, !fromRune
	if cmd.IsEnabled() {
		cmd.Run()
		r.Refresh()
	}
	return true
}

func (r *Root) Owns(x, y int) (widget.Cursor, bool) {
	return r.CursorAt(x, y)
}
