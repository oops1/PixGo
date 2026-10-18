package edit

import (
	"image"
	"math"
	"time"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/codec"
	coreedit "github.com/oops1/PixGo/internal/edit"
	"github.com/oops1/PixGo/internal/ui/theme"
	"github.com/oops1/PixGo/internal/ui/view"
	"github.com/oops1/PixGo/internal/viewer"
)

type Engine interface {
	Invalidate()
	InvalidateRect(r image.Rectangle)
	SetFocus(w widget.Widget)
}

type Handlers struct {
	AspectMenu func(anchor image.Rectangle)
	SaveMenu   func(anchor image.Rectangle)
	SaveCopy   func()
	Cancel     func()
	Resize     func()
	Changed    func()
}

type drag struct {
	on     bool
	handle Handle
	start  image.Rectangle
	px, py int
}

type Editor struct {
	widget.Base
	eng    Engine
	canvas *view.Canvas

	Theme  *theme.Theme
	After  func(d time.Duration, fn func()) (stop func())
	Chrome func(x, y int) (widget.Cursor, bool)
	On     Handlers

	active bool
	busy   bool
	saved  viewer.View
	base   codec.Orientation
	crop   *Crop
	fx, fy float64
	snap   image.Rectangle

	bar  barLayout
	area image.Rectangle

	hover     Part
	pressed   Part
	focus     Part
	kbd       bool
	menuPart  Part
	dragging  drag
	hoverHand Handle

	tipText string
	tipPart Part
	stopTip func()
}

func New(eng Engine, canvas *view.Canvas) *Editor {
	e := &Editor{eng: eng, canvas: canvas, Theme: theme.Dark(), crop: NewCrop(1, 1), focus: PartCrop}
	return e
}

func (e *Editor) Canvas() *view.Canvas { return e.canvas }

func (e *Editor) Active() bool { return e.active }

func (e *Editor) Crop() *Crop { return e.crop }

func (e *Editor) Busy() bool { return e.busy }

func (e *Editor) SetBusy(on bool) {
	if e.busy == on {
		return
	}
	e.busy = on
	e.eng.Invalidate()
}

func (e *Editor) Begin() bool {
	c := e.canvas
	if !c.HasImage() {
		return false
	}
	e.saved = c.Snapshot()
	c.ForceFit()
	fr := c.Frame()
	e.base = c.Orientation()
	ow, oh := e.base.Size(fr.ImgW, fr.ImgH)
	e.crop = NewCrop(ow, oh)
	e.fx, e.fy = 0, 0
	e.snap = image.Rectangle{}
	e.active, e.busy = true, false
	e.hover, e.pressed, e.menuPart = PartNone, PartNone, PartNone
	e.focus, e.kbd = PartCrop, false
	e.dragging = drag{}
	e.hideTip()
	e.layout()
	e.eng.Invalidate()
	return true
}

func (e *Editor) End() {
	if !e.active {
		return
	}
	e.active = false
	e.busy = false
	e.dragging = drag{}
	e.hideTip()
	e.canvas.Restore(e.saved)
	e.eng.Invalidate()
}

func (e *Editor) StoredSize() image.Point {
	fr := e.canvas.Frame()
	return image.Pt(fr.ImgW, fr.ImgH)
}

func (e *Editor) Orientation() codec.Orientation { return e.canvas.Orientation() }

func (e *Editor) BaseOrientation() codec.Orientation { return e.base }

func (e *Editor) HasResize() bool { return e.fx != 0 && e.fy != 0 }

func (e *Editor) CropSize() image.Point { return e.crop.Rect().Size() }

func (e *Editor) OutputSize() image.Point {
	r := e.crop.Rect()
	if !e.HasResize() {
		return r.Size()
	}
	return image.Pt(max(1, int(math.Round(float64(r.Dx())*e.fx))), max(1, int(math.Round(float64(r.Dy())*e.fy))))
}

func (e *Editor) SetOutputSize(w, h int) {
	r := e.crop.Rect()
	if w <= 0 || h <= 0 || w == r.Dx() && h == r.Dy() {
		e.fx, e.fy = 0, 0
	} else {
		e.fx, e.fy = float64(w)/float64(r.Dx()), float64(h)/float64(r.Dy())
	}
	e.changed()
}

func (e *Editor) ClearResize() {
	if e.fx == 0 && e.fy == 0 {
		return
	}
	e.fx, e.fy = 0, 0
	e.changed()
}

func (e *Editor) Plan() coreedit.Plan {
	p := coreedit.Plan{Orientation: e.canvas.Orientation()}
	if !e.crop.Whole() {
		p.Crop = e.crop.Rect()
	}
	if e.HasResize() {
		p.Size = e.OutputSize()
	}
	return p
}

func (e *Editor) Dirty() bool {
	return e.active && (!e.crop.Whole() || e.HasResize() || e.canvas.Orientation() != e.base)
}

func (e *Editor) SetSnapped(r image.Rectangle) {
	if e.snap == r {
		return
	}
	e.snap = r
	e.eng.Invalidate()
}

func (e *Editor) Snapped() image.Rectangle { return e.snap }

func (e *Editor) changed() {
	e.eng.Invalidate()
	if e.On.Changed != nil {
		e.On.Changed()
	}
}

func (e *Editor) Rotate(cw bool) {
	if !e.active {
		return
	}
	before := e.canvas.Orientation()
	if cw {
		e.canvas.RotateRight()
	} else {
		e.canvas.RotateLeft()
	}
	if e.canvas.Orientation() == before {
		return
	}
	e.crop.Rotate(cw)
	e.snap = image.Rectangle{}
	e.changed()
}

func (e *Editor) FlipH() { e.flip(true) }

func (e *Editor) FlipV() { e.flip(false) }

func (e *Editor) flip(h bool) {
	if !e.active {
		return
	}
	before := e.canvas.Orientation()
	if h {
		e.canvas.FlipH()
	} else {
		e.canvas.FlipV()
	}
	if e.canvas.Orientation() == before {
		return
	}
	if h {
		e.crop.FlipH()
	} else {
		e.crop.FlipV()
	}
	e.snap = image.Rectangle{}
	e.changed()
}

func (e *Editor) Reset() {
	if !e.active {
		return
	}
	e.canvas.SetUserOrientation(e.saved.User)
	fr := e.canvas.Frame()
	ow, oh := e.canvas.Orientation().Size(fr.ImgW, fr.ImgH)
	e.crop = NewCrop(ow, oh)
	e.fx, e.fy = 0, 0
	e.snap = image.Rectangle{}
	e.changed()
}

func (e *Editor) SetAspect(a Aspect) {
	if !e.active {
		return
	}
	e.crop.SetAspect(a)
	e.changed()
}

func (e *Editor) SwapAspect() {
	if !e.active {
		return
	}
	e.crop.Swap()
	e.changed()
}

func (e *Editor) Nudge(dx, dy int) {
	if !e.active {
		return
	}
	before := e.crop.Rect()
	e.crop.Nudge(dx, dy)
	if e.crop.Rect() != before {
		e.dropSnap()
		e.changed()
	}
}

func (e *Editor) Grow(dw, dh int) {
	if !e.active {
		return
	}
	before := e.crop.Rect()
	e.crop.Grow(dw, dh)
	if e.crop.Rect() != before {
		e.dropSnap()
		e.changed()
	}
}

func (e *Editor) dropSnap() {
	e.snap = image.Rectangle{}
}

func (e *Editor) SetBounds(b image.Rectangle) {
	e.Base.SetBounds(b)
	e.layout()
}

func (e *Editor) layout() {
	b := e.Bounds()
	e.bar = e.computeBar(b)
	top := e.bar.bar.Max.Y + areaGap
	e.area = image.Rect(b.Min.X+areaSide, top, b.Max.X-areaSide, b.Max.Y-areaBottom)
	if e.area.Dx() < 1 || e.area.Dy() < 1 {
		e.area = image.Rectangle{Min: e.area.Min, Max: e.area.Min}
	}
	if e.canvas.Bounds() != e.area {
		e.canvas.SetBounds(e.area)
	}
	e.canvas.EnsureArea()
}

func (e *Editor) Area() image.Rectangle { return e.area }

func (e *Editor) BarRect(p Part) image.Rectangle { return e.bar.rect(p) }

func (e *Editor) Bar() image.Rectangle { return e.bar.bar }

func (e *Editor) mapping() (ox, oy, k float64, ok bool) { return e.canvas.ImageToScreen() }

func (e *Editor) CropScreen() image.Rectangle {
	ox, oy, k, ok := e.mapping()
	if !ok {
		return image.Rectangle{}
	}
	return toScreen(e.crop.Rect(), ox, oy, k)
}

func toScreen(r image.Rectangle, ox, oy, k float64) image.Rectangle {
	return image.Rect(
		int(math.Round(ox+k*float64(r.Min.X))),
		int(math.Round(oy+k*float64(r.Min.Y))),
		int(math.Round(ox+k*float64(r.Max.X))),
		int(math.Round(oy+k*float64(r.Max.Y))))
}

func (e *Editor) PictureScreen() image.Rectangle {
	ox, oy, k, ok := e.mapping()
	if !ok {
		return image.Rectangle{}
	}
	return toScreen(image.Rectangle{Max: e.crop.Size()}, ox, oy, k)
}

func (e *Editor) HandleCenter(h Handle) (int, int) {
	r := e.CropScreen()
	cx, cy := (r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2
	x, y := cx, cy
	switch {
	case h.west():
		x = r.Min.X
	case h.east():
		x = r.Max.X
	}
	switch {
	case h.north():
		y = r.Min.Y
	case h.south():
		y = r.Max.Y
	}
	return x, y
}

func (e *Editor) Enabled(p Part) bool {
	if !e.active || e.busy {
		return false
	}
	switch p {
	case PartReset, PartSaveCopy, PartSaveMore:
		return e.Dirty()
	}
	return true
}
