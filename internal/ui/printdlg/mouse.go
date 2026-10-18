package printdlg

import (
	"image"

	"github.com/oops1/headless-gui/v3/widget"
)

func (f *form) WantsCapture(e widget.MouseEvent) bool {
	return e.Button == widget.MouseLeft && e.Pressed && image.Pt(e.X, e.Y).In(f.Bounds())
}

func (f *form) release() {
	if f.capMgr != nil {
		f.capMgr.ReleaseCapture()
	}
}

func (f *form) hitAt(x, y int) Part {
	pt := image.Pt(x, y)
	for _, p := range []Part{PartPrint, PartCancel} {
		if f.enabled(p) && pt.In(f.rect(p)) {
			return p
		}
	}
	if !pt.In(f.viewRect()) {
		return PartNone
	}
	for _, p := range order {
		if fixedPart(p) || !f.enabled(p) {
			continue
		}
		if pt.In(f.rect(p)) {
			return p
		}
	}
	for _, p := range []Part{PartCopiesDec, PartCopiesInc} {
		if f.enabled(p) && pt.In(f.rect(p)) {
			return p
		}
	}
	return PartNone
}

func (f *form) caretAt(p Part, x int) int {
	text := f.textOf(p)
	r := f.rect(p)
	left := r.Min.X + f.fieldPad()
	best, bestD := 0, 1<<30
	for i := 0; i <= len(text); i++ {
		cx := left + bodyFont.Measure(nil, text[:i])
		if d := abs(cx - x); d < bestD {
			best, bestD = i, d
		}
	}
	return best
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func (f *form) fieldPad() int { return f.lay.m.s(10) }

func (f *form) OnMouseButton(e widget.MouseEvent) bool {
	if e.Button != widget.MouseLeft {
		return false
	}
	if e.Pressed {
		f.keyNav = false
		if f.list != nil {
			if i := f.listIndexAt(e.X, e.Y); i >= 0 {
				f.pickList(i)
			} else {
				f.closeList()
			}
			f.pressed = PartNone
			f.release()
			return true
		}
		h := f.hitAt(e.X, e.Y)
		f.pressed = h
		if isField(h) {
			fl, _ := f.fieldOf(h)
			f.stop = h
			fl.caret, fl.all = f.caretAt(h, e.X), false
		} else if h != PartNone {
			f.stop = h
		}
		f.Invalidate()
		return true
	}
	over := f.hitAt(e.X, e.Y)
	pressed := f.pressed
	f.pressed = PartNone
	f.release()
	f.Invalidate()
	if pressed != PartNone && pressed == over && !isField(pressed) {
		f.activate(pressed)
	}
	return true
}

func (f *form) OnMouseMove(x, y int) {
	if f.list != nil {
		h := f.listIndexAt(x, y)
		if h != f.list.hover {
			f.list.hover = h
			if h >= 0 {
				f.list.sel = h
			}
			f.Invalidate()
		}
		return
	}
	if h := f.hitAt(x, y); h != f.hover {
		f.hover = h
		f.Invalidate()
	}
}

func (f *form) OnMouseWheelPixelsMod(x, y int, dx, dy float64, mod widget.KeyMod) bool {
	if dy == 0 || !image.Pt(x, y).In(f.Bounds()) {
		return false
	}
	if f.list != nil {
		if image.Pt(x, y).In(f.list.rect) {
			step := 1
			if dy < 0 {
				step = -1
			}
			f.listScroll(step)
		}
		return true
	}
	return f.scrollBy(int(dy))
}

func (f *form) Cursor(x, y int) widget.Cursor {
	if f.list != nil {
		if f.listIndexAt(x, y) >= 0 {
			return widget.CursorHand
		}
		return widget.CursorArrow
	}
	switch h := f.hitAt(x, y); {
	case isField(h):
		return widget.CursorIBeam
	case h == PartNone:
		return widget.CursorArrow
	}
	return widget.CursorHand
}
