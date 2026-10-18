package edit

import (
	"image"
	"time"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/i18n"
)

const (
	cornerHit = 11
	edgeHit   = 6
	tipDelay  = 600 * time.Millisecond
)

func (e *Editor) partAt(x, y int) Part {
	pt := image.Pt(x, y)
	for _, p := range barStops {
		if pt.In(e.bar.rect(p)) {
			return p
		}
	}
	return PartNone
}

func (e *Editor) inBar(x, y int) bool { return image.Pt(x, y).In(e.bar.bar) }

func (e *Editor) HandleAt(x, y int) Handle {
	if !e.active {
		return HandleNone
	}
	r := e.CropScreen()
	if r.Empty() {
		return HandleNone
	}
	near := func(a, b, tol int) bool { return a >= b-tol && a <= b+tol }
	for _, h := range handleList[:4] {
		hx, hy := e.HandleCenter(h)
		if near(x, hx, cornerHit) && near(y, hy, cornerHit) {
			return h
		}
	}
	inX := x >= r.Min.X-edgeHit && x <= r.Max.X+edgeHit
	inY := y >= r.Min.Y-edgeHit && y <= r.Max.Y+edgeHit
	switch {
	case near(y, r.Min.Y, edgeHit) && inX:
		return HandleN
	case near(y, r.Max.Y, edgeHit) && inX:
		return HandleS
	case near(x, r.Min.X, edgeHit) && inY:
		return HandleW
	case near(x, r.Max.X, edgeHit) && inY:
		return HandleE
	}
	if image.Pt(x, y).In(r) {
		return HandleMove
	}
	return HandleNone
}

func (e *Editor) passes(x, y int) bool {
	if e.Chrome == nil {
		return false
	}
	_, owns := e.Chrome(x, y)
	return owns
}

func (e *Editor) WantsCapture(ev widget.MouseEvent) bool {
	if !ev.Pressed || ev.Button != widget.MouseLeft || e.passes(ev.X, ev.Y) {
		return false
	}
	return image.Pt(ev.X, ev.Y).In(e.Bounds())
}

func (e *Editor) Cursor(x, y int) widget.Cursor {
	if e.Chrome != nil {
		if cur, owns := e.Chrome(x, y); owns {
			return cur
		}
	}
	if !e.active {
		return widget.CursorArrow
	}
	if e.dragging.on {
		return cursorOf(e.dragging.handle)
	}
	if p := e.partAt(x, y); p != PartNone {
		if e.Enabled(p) {
			return widget.CursorHand
		}
		return widget.CursorArrow
	}
	if e.busy {
		return widget.CursorWait
	}
	return cursorOf(e.HandleAt(x, y))
}

func cursorOf(h Handle) widget.Cursor {
	switch h {
	case HandleNW, HandleSE:
		return widget.CursorSizeNWSE
	case HandleNE, HandleSW:
		return widget.CursorSizeNESW
	case HandleN, HandleS:
		return widget.CursorSizeNS
	case HandleE, HandleW:
		return widget.CursorSizeWE
	case HandleMove:
		return widget.CursorSizeAll
	}
	return widget.CursorArrow
}

func (e *Editor) OnMouseButton(ev widget.MouseEvent) bool {
	switch ev.Button {
	case widget.MouseWheelUp, widget.MouseWheelDown:
		return true
	case widget.MouseLeft:
	default:
		return true
	}
	if !e.active {
		return false
	}
	if ev.Pressed {
		return e.press(ev.X, ev.Y)
	}
	return e.release(ev.X, ev.Y)
}

func (e *Editor) OnMouseWheelPixelsMod(x, y int, dx, dy float64, mod widget.KeyMod) bool { return true }

func (e *Editor) press(x, y int) bool {
	e.kbd = false
	e.MenuClosed()
	e.hideTip()
	if e.busy {
		return true
	}
	if p := e.partAt(x, y); p != PartNone {
		e.pressed = p
		e.invalidatePart(p)
		return true
	}
	if e.inBar(x, y) {
		return true
	}
	e.focus = PartCrop
	h := e.HandleAt(x, y)
	if h == HandleNone {
		return true
	}
	e.dragging = drag{on: true, handle: h, start: e.crop.Rect(), px: x, py: y}
	return true
}

func (e *Editor) release(x, y int) bool {
	if e.dragging.on {
		e.dragging = drag{}
		e.eng.Invalidate()
		return true
	}
	p := e.pressed
	e.pressed = PartNone
	if p == PartNone {
		return true
	}
	e.invalidatePart(p)
	if e.partAt(x, y) == p && e.Enabled(p) {
		e.activate(p)
	}
	return true
}

func (e *Editor) OnMouseMove(x, y int) {
	if widget.CursorIsNowhere(x, y) {
		e.setHover(PartNone)
		return
	}
	if e.dragging.on {
		_, _, k, ok := e.mapping()
		if !ok || k <= 0 {
			return
		}
		dx, dy := float64(x-e.dragging.px)/k, float64(y-e.dragging.py)/k
		before := e.crop.Rect()
		e.crop.Drag(e.dragging.handle, e.dragging.start, dx, dy)
		if e.crop.Rect() != before {
			e.dropSnap()
			e.changed()
		}
		return
	}
	e.setHover(e.partAt(x, y))
}

func (e *Editor) setHover(p Part) {
	if p == e.hover {
		return
	}
	old := e.hover
	e.hover = p
	e.invalidatePart(old)
	e.invalidatePart(p)
	e.hideTip()
	e.scheduleTip(p)
}

func (e *Editor) invalidatePart(p Part) {
	if p == PartNone {
		return
	}
	if r := e.bar.rect(p); !r.Empty() {
		e.eng.InvalidateRect(r.Inset(-4))
	}
}

func (e *Editor) TipFor(p Part) string {
	switch p {
	case PartAspect:
		return i18n.T("edit.aspect")
	case PartSwap:
		return i18n.T("edit.swap")
	case PartRotateLeft:
		return i18n.T("cmd.rotateLeft") + " (Shift+R)"
	case PartRotateRight:
		return i18n.T("cmd.rotateRight") + " (R)"
	case PartFlipH:
		return i18n.T("cmd.flipH") + " (H)"
	case PartFlipV:
		return i18n.T("cmd.flipV") + " (V)"
	case PartResize:
		return i18n.T("edit.resize")
	case PartReset:
		return i18n.T("edit.reset")
	case PartSaveCopy:
		return i18n.T("edit.saveCopy") + " (Enter)"
	case PartSaveMore:
		return i18n.T("edit.saveMenu")
	case PartCancel:
		return i18n.T("edit.cancel") + " (Esc)"
	}
	return ""
}

func (e *Editor) scheduleTip(p Part) {
	if p == PartNone || e.After == nil {
		return
	}
	text := e.TipFor(p)
	if text == "" {
		return
	}
	e.tipPart = p
	e.stopTip = e.After(tipDelay, func() {
		e.stopTip = nil
		e.tipText = text
		e.eng.InvalidateRect(e.tipRect(nil))
	})
}

func (e *Editor) hideTip() {
	if e.stopTip != nil {
		e.stopTip()
		e.stopTip = nil
	}
	if e.tipText != "" {
		r := e.tipRect(nil)
		e.tipText = ""
		e.eng.InvalidateRect(r)
	}
	e.tipPart = PartNone
}

func (e *Editor) stops() []Part {
	out := []Part{PartCrop}
	for _, p := range barStops {
		if e.Enabled(p) {
			out = append(out, p)
		}
	}
	return out
}

func (e *Editor) FocusedPart() (Part, bool) { return e.focus, e.kbd }

func (e *Editor) SetFocusPart(p Part) {
	old := e.focus
	e.focus, e.kbd = p, true
	e.invalidatePart(old)
	e.invalidatePart(p)
	e.eng.Invalidate()
}

func (e *Editor) tab(dir int) {
	list := e.stops()
	at := -1
	for i, p := range list {
		if p == e.focus {
			at = i
		}
	}
	switch {
	case !e.kbd:
		at = 0
		if dir < 0 {
			at = len(list) - 1
		}
	case at < 0:
		at = 0
	default:
		at = (at + dir + len(list)) % len(list)
	}
	e.SetFocusPart(list[at])
}

func (e *Editor) activate(p Part) {
	if !e.Enabled(p) {
		return
	}
	switch p {
	case PartAspect:
		e.menuPart = p
		if e.On.AspectMenu != nil {
			e.On.AspectMenu(e.bar.rect(p))
		}
	case PartSwap:
		e.SwapAspect()
	case PartRotateLeft:
		e.Rotate(false)
	case PartRotateRight:
		e.Rotate(true)
	case PartFlipH:
		e.FlipH()
	case PartFlipV:
		e.FlipV()
	case PartResize:
		if e.On.Resize != nil {
			e.On.Resize()
		}
	case PartReset:
		e.Reset()
	case PartSaveCopy:
		if e.On.SaveCopy != nil {
			e.On.SaveCopy()
		}
	case PartSaveMore:
		e.menuPart = p
		if e.On.SaveMenu != nil {
			e.On.SaveMenu(e.bar.rect(p))
		}
	case PartCancel:
		if e.On.Cancel != nil {
			e.On.Cancel()
		}
	}
}

func (e *Editor) MenuClosed() {
	old := e.menuPart
	e.menuPart = PartNone
	e.invalidatePart(old)
}

func step(e widget.KeyEvent) int {
	if e.Mod&widget.ModShift != 0 {
		return 10
	}
	return 1
}

func (e *Editor) OnKeyEvent(ev widget.KeyEvent) { e.HandleKey(ev) }

func (e *Editor) HandleKey(ev widget.KeyEvent) bool {
	if !e.active || !ev.Pressed {
		return false
	}
	if ev.Mod&(widget.ModAlt) != 0 {
		return false
	}
	ctrl := ev.Mod&widget.ModCtrl != 0
	switch ev.Code {
	case widget.KeyTab:
		if ctrl {
			return false
		}
		if e.busy {
			return true
		}
		dir := 1
		if ev.Mod&widget.ModShift != 0 {
			dir = -1
		}
		e.tab(dir)
		return true
	case widget.KeyEscape:
		if e.busy {
			return true
		}
		if e.On.Cancel != nil {
			e.On.Cancel()
		}
		return true
	case widget.KeyEnter:
		if ev.Repeat {
			return true
		}
		if e.kbd && e.focus != PartCrop {
			e.activate(e.focus)
			return true
		}
		e.activate(PartSaveCopy)
		return true
	case widget.KeySpace:
		if ev.Repeat {
			return true
		}
		if e.kbd && e.focus != PartCrop {
			e.activate(e.focus)
			return true
		}
		return false
	case widget.KeyLeft, widget.KeyRight, widget.KeyUp, widget.KeyDown:
		return e.arrow(ev)
	}
	return false
}

func (e *Editor) arrow(ev widget.KeyEvent) bool {
	if e.busy {
		return true
	}
	n := step(ev)
	dx, dy := 0, 0
	switch ev.Code {
	case widget.KeyLeft:
		dx = -n
	case widget.KeyRight:
		dx = n
	case widget.KeyUp:
		dy = -n
	case widget.KeyDown:
		dy = n
	}
	if ev.Mod&widget.ModCtrl != 0 {
		e.Grow(dx, dy)
	} else {
		e.Nudge(dx, dy)
	}
	return true
}

func (e *Editor) AcceptsTab() bool { return e.active }
