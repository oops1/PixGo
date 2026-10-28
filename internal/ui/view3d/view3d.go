package view3d

import (
	"context"
	"image"
	"image/color"
	"math"
	"sync"
	"time"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/render3d"
)

type Engine interface {
	Invalidate()
}

const (
	wheelTickPixels   = 40.0
	zoomStep          = 1.25
	minZoom           = 0.05
	maxZoom           = 400
	keyTurn           = math.Pi / 12
	keyPanPixels      = 48
	dragStartDistance = 3
	settleDelay       = 160 * time.Millisecond
	interactiveBudget = 28 * time.Millisecond
)

type request struct {
	cam     render3d.Camera
	w, h    int
	reduce  int
	samples int
	bg      color.NRGBA
	gen     uint64
}

type frame struct {
	img     *image.RGBA
	shown   image.Image
	cam     render3d.Camera
	w, h    int
	samples int
}

type Viewer struct {
	widget.Base
	eng  Engine
	post func(fn func())

	Background    color.RGBA
	Material      color.NRGBA
	Chrome        func(x, y int) (widget.Cursor, bool)
	OnContextMenu func(x, y int)
	OnChange      func()
	After         func(d time.Duration, fn func()) (stop func())
	Now           func() time.Time

	scene *render3d.Scene
	cam   render3d.Camera
	home  render3d.Camera
	gen   uint64

	mu      sync.Mutex
	pending *request
	busy    bool
	cancel  context.CancelFunc
	last    time.Duration

	cur      frame
	curGen   uint64
	scale    float64
	areaW    int
	areaH    int
	moving   bool
	stopIdle func()

	pressed    bool
	pressBtn   widget.MouseButton
	pressMod   widget.KeyMod
	pressX     int
	pressY     int
	lastX      int
	lastY      int
	dragging   bool
	wheelAccum float64
}

type poster interface {
	Post(fn func())
}

func New(eng Engine) *Viewer {
	v := &Viewer{eng: eng, scale: 1, Now: time.Now}
	if p, ok := eng.(poster); ok {
		v.post = p.Post
	}
	v.Material = render3d.DefaultStyle().Base
	return v
}

func (v *Viewer) SetPost(post func(fn func())) { v.post = post }

func (v *Viewer) SetScene(s *render3d.Scene) {
	v.scene = s
	v.home = render3d.DefaultCamera()
	v.cam = v.home
	v.cur = frame{}
	v.curGen = 0
	v.request(false)
}

func (v *Viewer) Scene() *render3d.Scene { return v.scene }

func (v *Viewer) SetBackground(c color.RGBA) {
	if c == v.Background {
		return
	}
	v.Background = c
	v.request(false)
}

func (v *Viewer) HasScene() bool { return v.scene != nil }

func (v *Viewer) Clear() {
	v.mu.Lock()
	if v.cancel != nil {
		v.cancel()
	}
	v.pending = nil
	v.mu.Unlock()
	v.scene = nil
	v.cur = frame{}
	v.curGen = 0
	v.gen++
}

func (v *Viewer) Camera() render3d.Camera { return v.cam }

func (v *Viewer) SetCamera(c render3d.Camera) {
	v.cam = clampCamera(c)
	v.interact()
}

func (v *Viewer) ZoomPercent() int {
	return int(math.Round(v.cam.Zoom * 100))
}

func (v *Viewer) Reset() {
	v.cam = v.home
	v.settle()
}

func (v *Viewer) locked() {
	if v.scene != nil && v.cam.Fit == 0 && v.areaW > 0 && v.areaH > 0 {
		v.cam = v.scene.LockFit(v.cam, v.areaW, v.areaH)
	}
}

func (v *Viewer) ZoomIn()  { v.zoomBy(zoomStep, -1, -1) }
func (v *Viewer) ZoomOut() { v.zoomBy(1/zoomStep, -1, -1) }

func (v *Viewer) Turn(cw bool) {
	d := math.Pi / 2
	if cw {
		d = -d
	}
	v.cam.Yaw = normalizeAngle(v.cam.Yaw + d)
	v.settle()
}

func (v *Viewer) TurnBy(yaw, pitch float64) {
	v.cam.Yaw = normalizeAngle(v.cam.Yaw + yaw)
	v.cam.Pitch = clampPitch(v.cam.Pitch + pitch)
	v.interact()
}

func (v *Viewer) PanSteps(right, down int) {
	h := v.physH()
	v.cam = render3d.PanBy(v.cam, float64(-right*keyPanPixels)*v.scale, float64(-down*keyPanPixels)*v.scale, h)
	v.interact()
}

func (v *Viewer) zoomBy(f float64, x, y float64) {
	if v.scene == nil {
		return
	}
	target := math.Min(maxZoom, math.Max(minZoom, v.cam.Zoom*f))
	if target == v.cam.Zoom {
		return
	}
	f = target / v.cam.Zoom
	v.locked()
	w, h := v.physW(), v.physH()
	if x < 0 || y < 0 {
		x, y = float64(w)/2, float64(h)/2
	}
	v.cam = clampCamera(v.scene.ZoomAt(v.cam, w, h, x, y, f))
	v.interact()
}

func (v *Viewer) physW() int { return max(1, v.areaW) }
func (v *Viewer) physH() int { return max(1, v.areaH) }

func (v *Viewer) changed() {
	if v.OnChange != nil {
		v.OnChange()
	}
}

func (v *Viewer) interact() {
	v.moving = true
	v.request(true)
	v.changed()
	v.scheduleSettle()
}

func (v *Viewer) settle() {
	v.moving = false
	v.request(false)
	v.changed()
}

func (v *Viewer) scheduleSettle() {
	if v.stopIdle != nil {
		v.stopIdle()
		v.stopIdle = nil
	}
	if v.After == nil {
		return
	}
	gen := v.gen
	v.stopIdle = v.After(settleDelay, func() {
		v.stopIdle = nil
		if gen == v.gen && !v.dragging {
			v.settle()
		}
	})
}

func (v *Viewer) request(fast bool) {
	if v.scene == nil || v.areaW <= 0 || v.areaH <= 0 {
		v.invalidate()
		return
	}
	v.locked()
	v.gen++
	r := &request{cam: v.cam, w: v.areaW, h: v.areaH, reduce: 1, samples: 2, gen: v.gen}
	r.bg = color.NRGBA{v.Background.R, v.Background.G, v.Background.B, 255}
	if fast {
		r.samples = 1
		v.mu.Lock()
		last := v.last
		v.mu.Unlock()
		if last > interactiveBudget {
			r.reduce = 2
		}
		if last > 4*interactiveBudget {
			r.reduce = 3
		}
	}
	scene := v.scene
	style := render3d.DefaultStyle()
	style.Background = r.bg
	style.Base = v.Material
	v.mu.Lock()
	v.pending = r
	if v.busy {
		if v.cancel != nil && !fast {
			v.cancel()
		}
		v.mu.Unlock()
		return
	}
	v.busy = true
	v.mu.Unlock()
	go v.work(scene, style)
}

func (v *Viewer) work(scene *render3d.Scene, style render3d.Style) {
	for {
		v.mu.Lock()
		r := v.pending
		v.pending = nil
		if r == nil {
			v.busy = false
			v.mu.Unlock()
			return
		}
		ctx, cancel := context.WithCancel(context.Background())
		v.cancel = cancel
		v.mu.Unlock()

		w, h := max(1, (r.w+r.reduce-1)/r.reduce), max(1, (r.h+r.reduce-1)/r.reduce)
		img := image.NewRGBA(image.Rect(0, 0, w, h))
		st := style
		st.Samples = r.samples
		st.Background = r.bg
		start := time.Now()
		err := scene.RenderInto(ctx, r.cam, st, img)
		took := time.Since(start)
		cancel()
		if err != nil {
			continue
		}
		v.mu.Lock()
		if r.samples == 1 {
			v.last = took * time.Duration(r.reduce*r.reduce)
		}
		v.mu.Unlock()
		done := frame{img: img, shown: widget.OpaqueImage(img), cam: r.cam, w: r.w, h: r.h, samples: r.samples}
		gen := r.gen
		apply := func() {
			if v.scene != scene {
				return
			}
			if gen < v.curGen && v.cur.img != nil {
				return
			}
			v.cur = done
			v.curGen = gen
			v.invalidate()
		}
		if v.post != nil {
			v.post(apply)
		} else {
			apply()
		}
	}
}

func (v *Viewer) invalidate() {
	if v.eng != nil {
		v.eng.Invalidate()
	}
}

func (v *Viewer) Draw(ctx widget.DrawContext) {
	b := v.Bounds()
	if b.Empty() {
		return
	}
	v.scale = widget.ContextScale(ctx)
	p := widget.PhysicalRect(ctx, b)
	if p.X != v.areaW || p.Y != v.areaH {
		v.areaW, v.areaH = p.X, p.Y
		v.request(v.moving)
	}
	ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), v.Background)
	if v.cur.img == nil {
		return
	}
	ctx.DrawImageScaled(v.cur.shown, b.Min.X, b.Min.Y, b.Dx(), b.Dy())
}

func (v *Viewer) toPhysical(x, y int) (float64, float64) {
	b := v.Bounds()
	return (float64(x-b.Min.X) + 0.5) * v.scale, (float64(y-b.Min.Y) + 0.5) * v.scale
}

func (v *Viewer) Cursor(x, y int) widget.Cursor {
	if v.Chrome != nil {
		if cur, owns := v.Chrome(x, y); owns {
			return cur
		}
	}
	if v.dragging {
		return widget.CursorGrabbing
	}
	return widget.CursorGrab
}

func (v *Viewer) passes(x, y int) bool {
	if v.Chrome == nil {
		return false
	}
	_, owns := v.Chrome(x, y)
	return owns
}

func (v *Viewer) WantsCapture(e widget.MouseEvent) bool {
	if v.passes(e.X, e.Y) {
		return false
	}
	return e.Pressed && (e.Button == widget.MouseLeft || e.Button == widget.MouseRight || e.Button == widget.MouseMiddle)
}

func (v *Viewer) OnMouseButton(e widget.MouseEvent) bool {
	if !v.pressed && v.passes(e.X, e.Y) {
		return false
	}
	switch e.Button {
	case widget.MouseWheelUp:
		px, py := v.toPhysical(e.X, e.Y)
		v.zoomBy(zoomStep, px, py)
		return true
	case widget.MouseWheelDown:
		px, py := v.toPhysical(e.X, e.Y)
		v.zoomBy(1/zoomStep, px, py)
		return true
	case widget.MouseLeft, widget.MouseRight, widget.MouseMiddle:
	default:
		return false
	}
	if e.Pressed {
		if v.pressed {
			return true
		}
		v.pressed = true
		v.pressBtn = e.Button
		v.pressMod = e.Mod
		v.pressX, v.pressY = e.X, e.Y
		v.lastX, v.lastY = e.X, e.Y
		v.dragging = false
		if e.Button == widget.MouseLeft && e.Clicks == 2 {
			v.pressed = false
			v.Reset()
		}
		return true
	}
	if !v.pressed || e.Button != v.pressBtn {
		return v.pressed
	}
	v.pressed = false
	wasDragging := v.dragging
	v.dragging = false
	if wasDragging {
		v.settle()
		return true
	}
	if e.Button == widget.MouseRight && v.OnContextMenu != nil {
		v.OnContextMenu(e.X, e.Y)
	}
	return true
}

func (v *Viewer) OnMouseMove(x, y int) {
	if widget.CursorIsNowhere(x, y) || !v.pressed || v.scene == nil {
		return
	}
	if !v.dragging {
		dx, dy := x-v.pressX, y-v.pressY
		if dx*dx+dy*dy < dragStartDistance*dragStartDistance {
			return
		}
		v.dragging = true
	}
	dx := float64(x-v.lastX) * v.scale
	dy := float64(y-v.lastY) * v.scale
	v.lastX, v.lastY = x, y
	if dx == 0 && dy == 0 {
		return
	}
	pan := v.pressBtn == widget.MouseRight || v.pressBtn == widget.MouseMiddle || v.pressMod&widget.ModShift != 0
	if pan {
		v.cam = render3d.PanBy(v.cam, dx, dy, v.physH())
	} else {
		v.cam = clampCamera(render3d.Orbit(v.cam, dx, dy, v.physH()))
	}
	v.moving = true
	v.request(true)
	v.changed()
}

func (v *Viewer) OnMouseWheelPixelsMod(x, y int, dx, dy float64, mod widget.KeyMod) bool {
	if dy == 0 {
		return true
	}
	v.wheelAccum += dy
	steps := 0
	for v.wheelAccum >= wheelTickPixels {
		v.wheelAccum -= wheelTickPixels
		steps--
	}
	for v.wheelAccum <= -wheelTickPixels {
		v.wheelAccum += wheelTickPixels
		steps++
	}
	if steps != 0 {
		px, py := v.toPhysical(x, y)
		v.zoomBy(math.Pow(zoomStep, float64(steps)), px, py)
	}
	return true
}

func (v *Viewer) HandleKey(e widget.KeyEvent) bool {
	if !e.Pressed || v.scene == nil {
		return false
	}
	if e.Mod&(widget.ModCtrl|widget.ModAlt) != 0 {
		return false
	}
	shift := e.Mod&widget.ModShift != 0
	switch e.Code {
	case widget.KeyLeft:
		if shift {
			v.PanSteps(-1, 0)
		} else {
			v.TurnBy(keyTurn, 0)
		}
	case widget.KeyRight:
		if shift {
			v.PanSteps(1, 0)
		} else {
			v.TurnBy(-keyTurn, 0)
		}
	case widget.KeyUp:
		if shift {
			v.PanSteps(0, -1)
		} else {
			v.TurnBy(0, keyTurn)
		}
	case widget.KeyDown:
		if shift {
			v.PanSteps(0, 1)
		} else {
			v.TurnBy(0, -keyTurn)
		}
	case widget.KeyHome:
		v.Reset()
	default:
		return false
	}
	return true
}

func clampCamera(c render3d.Camera) render3d.Camera {
	c.Zoom = math.Min(maxZoom, math.Max(minZoom, c.Zoom))
	c.Pitch = clampPitch(c.Pitch)
	c.Yaw = normalizeAngle(c.Yaw)
	return c
}

func clampPitch(p float64) float64 {
	lim := 89.9 * math.Pi / 180
	return math.Min(lim, math.Max(-lim, p))
}

func normalizeAngle(a float64) float64 {
	a = math.Mod(a, 2*math.Pi)
	if a > math.Pi {
		a -= 2 * math.Pi
	}
	if a <= -math.Pi {
		a += 2 * math.Pi
	}
	return a
}

func (v *Viewer) Settled() bool {
	return v.cur.img != nil && v.cur.samples >= 2 && v.cur.cam == v.cam && !v.moving
}
