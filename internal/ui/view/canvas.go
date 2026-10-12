package view

import (
	"image"
	"math"
	"time"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/render"
	"github.com/oops1/PixGo/internal/viewer"
)

type Engine interface {
	Invalidate()
	InvalidateRect(r image.Rectangle)
}

type WheelMode uint8

const (
	WheelZoom WheelMode = iota
	WheelNavigate
)

const (
	wheelTickPixels   = 40.0
	swipeMinDistance  = 80
	swipeMaxDuration  = 400 * time.Millisecond
	swipeDominance    = 2.0
	dragStartDistance = 3
)

type Canvas struct {
	widget.Base
	eng Engine

	Background    render.Background
	Wheel         WheelMode
	CtrlWheelZoom bool
	Now           func() time.Time

	OnChange      func()
	OnStep        func(delta int)
	OnContextMenu func(x, y int)
	Chrome        func(x, y int) (widget.Cursor, bool)
	OnActivity    func()

	view viewer.View
	src  render.Source
	exif codec.Orientation
	buf  *image.RGBA

	scale float64
	areaW int
	areaH int

	pressed    bool
	pressBtn   widget.MouseButton
	pressX     int
	pressY     int
	pressAt    time.Time
	lastX      int
	lastY      int
	panning    bool
	wheelAccum float64
	dirty      bool
	gestures   widget.GestureRecognizer
}

func New(eng Engine) *Canvas {
	c := &Canvas{eng: eng, scale: 1, exif: codec.OrientNormal, Now: time.Now, CtrlWheelZoom: true}
	c.view.Reset(viewer.ZoomFit)
	return c
}

func (c *Canvas) View() *viewer.View { return &c.view }

func (c *Canvas) Frame() viewer.Frame {
	return viewer.Frame{ImgW: c.src.Width, ImgH: c.src.Height, Exif: c.exif, AreaW: c.areaW, AreaH: c.areaH}
}

func (c *Canvas) HasImage() bool { return len(c.src.Levels) > 0 && c.src.Width > 0 && c.src.Height > 0 }

func (c *Canvas) SetImage(src render.Source, exif codec.Orientation, mode viewer.ZoomMode) {
	c.src = src
	if !exif.Valid() {
		exif = codec.OrientNormal
	}
	c.exif = exif
	c.view.Reset(mode)
	c.panning = false
	c.pressed = false
	c.wheelAccum = 0
	c.invalidate()
	c.changed()
}

func (c *Canvas) ReplaceFrame(src render.Source) {
	c.src = src
	c.invalidate()
}

func (c *Canvas) Clear() {
	c.src = render.Source{}
	c.invalidate()
	c.changed()
}

func (c *Canvas) SetBackground(bg render.Background) {
	c.Background = bg
	c.invalidate()
}

func (c *Canvas) ZoomPercent() int { return c.view.ZoomPercent(c.Frame()) }

func (c *Canvas) Mode() viewer.ZoomMode { return c.view.Mode }

func (c *Canvas) CanPan() bool { return c.HasImage() && c.view.CanPan(c.Frame()) }

func (c *Canvas) ImageRectLogical() image.Rectangle {
	if !c.HasImage() {
		return image.Rectangle{}
	}
	r := c.view.ImageRect(c.Frame())
	b := c.Bounds()
	k := c.scale
	return image.Rect(
		b.Min.X+int(math.Floor(float64(r.Min.X)/k)),
		b.Min.Y+int(math.Floor(float64(r.Min.Y)/k)),
		b.Min.X+int(math.Ceil(float64(r.Max.X)/k)),
		b.Min.Y+int(math.Ceil(float64(r.Max.Y)/k)),
	).Intersect(b)
}

func (c *Canvas) invalidate() {
	c.dirty = true
	if c.eng != nil {
		c.eng.InvalidateRect(c.Bounds())
	}
}

func (c *Canvas) changed() {
	if c.OnChange != nil {
		c.OnChange()
	}
}

func (c *Canvas) apply(f func(fr viewer.Frame)) {
	if !c.HasImage() || c.areaW <= 0 || c.areaH <= 0 {
		return
	}
	before := c.view
	f(c.Frame())
	if c.view != before {
		c.invalidate()
		c.changed()
	}
}

func (c *Canvas) areaCenter() (float64, float64) {
	return float64(c.areaW) / 2, float64(c.areaH) / 2
}

func (c *Canvas) ZoomIn()  { c.stepZoom(1, false, 0, 0) }
func (c *Canvas) ZoomOut() { c.stepZoom(-1, false, 0, 0) }

func (c *Canvas) stepZoom(steps int, atPoint bool, px, py float64) {
	c.apply(func(f viewer.Frame) {
		if !atPoint {
			px, py = c.areaCenter()
		}
		c.view.StepZoom(f, steps, px, py)
	})
}

func (c *Canvas) SetMode(mode viewer.ZoomMode) {
	c.apply(func(f viewer.Frame) { c.view.SetMode(f, mode) })
}

func (c *Canvas) Fit()      { c.SetMode(viewer.ZoomFit) }
func (c *Canvas) Actual()   { c.SetMode(viewer.ZoomActual) }
func (c *Canvas) FitWidth() { c.SetMode(viewer.ZoomFitWidth) }

func (c *Canvas) ToggleFitActual() {
	c.apply(func(f viewer.Frame) {
		px, py := c.areaCenter()
		c.view.ToggleFitActual(f, px, py)
	})
}

func (c *Canvas) RotateRight() { c.apply(func(f viewer.Frame) { c.view.Rotate(f, true) }) }
func (c *Canvas) RotateLeft()  { c.apply(func(f viewer.Frame) { c.view.Rotate(f, false) }) }
func (c *Canvas) FlipH()       { c.apply(func(f viewer.Frame) { c.view.FlipH(f) }) }
func (c *Canvas) FlipV()       { c.apply(func(f viewer.Frame) { c.view.FlipV(f) }) }

func (c *Canvas) Draw(ctx widget.DrawContext) {
	b := c.Bounds()
	if b.Empty() {
		return
	}
	c.scale = widget.ContextScale(ctx)
	p := widget.PhysicalRect(ctx, b)
	if p.X != c.areaW || p.Y != c.areaH {
		c.areaW, c.areaH = p.X, p.Y
		if c.HasImage() {
			defer c.changed()
		}
	}
	if p.X <= 0 || p.Y <= 0 {
		return
	}
	if c.buf == nil || c.buf.Bounds().Dx() != p.X || c.buf.Bounds().Dy() != p.Y {
		c.buf = image.NewRGBA(image.Rect(0, 0, p.X, p.Y))
		c.dirty = true
	}
	if c.dirty {
		var m render.Mapping
		if c.HasImage() {
			m = c.view.Mapping(c.Frame())
		}
		render.Render(c.buf, c.src, m, c.Background)
		c.dirty = false
	}
	ctx.DrawImageScaled(c.buf, b.Min.X, b.Min.Y, b.Dx(), b.Dy())
}

func (c *Canvas) toPhysical(x, y int) (float64, float64) {
	b := c.Bounds()
	return (float64(x-b.Min.X) + 0.5) * c.scale, (float64(y-b.Min.Y) + 0.5) * c.scale
}

func (c *Canvas) Cursor(x, y int) widget.Cursor {
	if c.Chrome != nil {
		if cur, owns := c.Chrome(x, y); owns {
			return cur
		}
	}
	if c.panning {
		return widget.CursorGrabbing
	}
	if c.CanPan() {
		return widget.CursorGrab
	}
	return widget.CursorArrow
}

func (c *Canvas) WantsCapture(e widget.MouseEvent) bool {
	if c.passes(e.X, e.Y) {
		return false
	}
	return e.Pressed && e.Button == widget.MouseLeft
}

func (c *Canvas) OnMouseButton(e widget.MouseEvent) bool {
	if !c.pressed && c.passes(e.X, e.Y) {
		return false
	}
	if e.Button == widget.MouseButtonX1 || e.Button == widget.MouseButtonX2 {
		return false
	}
	switch e.Button {
	case widget.MouseWheelUp:
		c.wheelTicks(-1, e)
		return true
	case widget.MouseWheelDown:
		c.wheelTicks(1, e)
		return true
	case widget.MouseRight:
		if !e.Pressed && c.OnContextMenu != nil {
			c.OnContextMenu(e.X, e.Y)
		}
		return true
	case widget.MouseLeft, widget.MouseMiddle:
	default:
		return false
	}
	if e.Pressed {
		c.pressed = true
		c.pressBtn = e.Button
		c.pressX, c.pressY = e.X, e.Y
		c.lastX, c.lastY = e.X, e.Y
		c.pressAt = c.Now()
		c.panning = false
		if e.Button == widget.MouseLeft && e.Clicks == 2 {
			px, py := c.toPhysical(e.X, e.Y)
			c.apply(func(f viewer.Frame) { c.view.ToggleFitActual(f, px, py) })
			c.pressed = false
		}
		return true
	}
	wasPressed := c.pressed && c.pressBtn == e.Button
	c.pressed = false
	if wasPressed && e.Button == widget.MouseLeft && !c.panning {
		c.maybeSwipe(e.X, e.Y)
	}
	c.panning = false
	return wasPressed
}

func (c *Canvas) maybeSwipe(x, y int) {
	if c.OnStep == nil || c.CanPan() {
		return
	}
	dx, dy := float64(x-c.pressX), float64(y-c.pressY)
	if math.Abs(dx) < swipeMinDistance || math.Abs(dx) < swipeDominance*math.Abs(dy) {
		return
	}
	if c.Now().Sub(c.pressAt) > swipeMaxDuration {
		return
	}
	if dx < 0 {
		c.OnStep(1)
	} else {
		c.OnStep(-1)
	}
}

func (c *Canvas) OnMouseMove(x, y int) {
	if widget.CursorIsNowhere(x, y) {
		return
	}
	if !c.pressed || !c.CanPan() {
		return
	}
	if !c.panning {
		dx, dy := x-c.pressX, y-c.pressY
		if dx*dx+dy*dy < dragStartDistance*dragStartDistance {
			return
		}
		c.panning = true
	}
	dx := float64(x-c.lastX) * c.scale
	dy := float64(y-c.lastY) * c.scale
	c.lastX, c.lastY = x, y
	if dx == 0 && dy == 0 {
		return
	}
	c.apply(func(f viewer.Frame) { c.view.Pan(f, dx, dy) })
}

func (c *Canvas) wheelTicks(dir int, e widget.MouseEvent) {
	ctrl := e.Mod&widget.ModCtrl != 0
	if c.Wheel == WheelNavigate && !ctrl {
		if c.OnStep != nil {
			c.OnStep(dir)
		}
		return
	}
	if ctrl && !c.CtrlWheelZoom {
		return
	}
	px, py := c.toPhysical(e.X, e.Y)
	c.stepZoom(-dir, true, px, py)
}

func (c *Canvas) OnMouseWheelPixelsMod(x, y int, dx, dy float64, mod widget.KeyMod) bool {
	if dy == 0 {
		return true
	}
	ctrl := mod&widget.ModCtrl != 0
	if c.Wheel == WheelNavigate && !ctrl {
		c.wheelAccum += dy
		for c.wheelAccum >= wheelTickPixels {
			c.wheelAccum -= wheelTickPixels
			if c.OnStep != nil {
				c.OnStep(1)
			}
		}
		for c.wheelAccum <= -wheelTickPixels {
			c.wheelAccum += wheelTickPixels
			if c.OnStep != nil {
				c.OnStep(-1)
			}
		}
		return true
	}
	if ctrl && !c.CtrlWheelZoom {
		return true
	}
	c.wheelAccum += dy
	steps := 0
	for c.wheelAccum >= wheelTickPixels {
		c.wheelAccum -= wheelTickPixels
		steps--
	}
	for c.wheelAccum <= -wheelTickPixels {
		c.wheelAccum += wheelTickPixels
		steps++
	}
	if steps != 0 {
		px, py := c.toPhysical(x, y)
		c.stepZoom(steps, true, px, py)
	}
	return true
}

func (c *Canvas) SetImageKeepView(src render.Source, exif codec.Orientation) {
	c.src = src
	if !exif.Valid() {
		exif = codec.OrientNormal
	}
	c.exif = exif
	c.view.CenterU, c.view.CenterV = 0, 0
	c.panning = false
	c.pressed = false
	c.invalidate()
	c.changed()
}

func (c *Canvas) passes(x, y int) bool {
	if c.Chrome == nil {
		return false
	}
	_, owns := c.Chrome(x, y)
	return owns
}

func (c *Canvas) Orientation() codec.Orientation { return c.view.Orient(c.Frame()) }
