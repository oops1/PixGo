package view

import (
	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/viewer"
)

func (c *Canvas) OnTouch(ev widget.TouchEvent) bool {
	if ev.Phase == widget.TouchBegan && (!c.HasImage() || c.passes(ev.X, ev.Y)) {
		return false
	}
	if c.OnActivity != nil {
		c.OnActivity()
	}
	for _, g := range c.gestures.Feed(ev) {
		c.gesture(g)
	}
	return true
}

func (c *Canvas) gesture(g widget.Gesture) {
	switch v := g.(type) {
	case widget.Pan:
		if !c.CanPan() {
			return
		}
		dx, dy := float64(v.Delta.X)*c.scale, float64(v.Delta.Y)*c.scale
		c.apply(func(f viewer.Frame) { c.view.Pan(f, dx, dy) })
	case widget.Pinch:
		if v.Phase == widget.GestureCancelled || v.Step <= 0 {
			return
		}
		px, py := c.toPhysical(v.Center.X, v.Center.Y)
		dx, dy := float64(v.CenterDelta.X)*c.scale, float64(v.CenterDelta.Y)*c.scale
		c.apply(func(f viewer.Frame) {
			c.view.ZoomAt(f, px, py, v.Step)
			c.view.Pan(f, dx, dy)
		})
	case widget.Swipe:
		if c.OnStep == nil || c.CanPan() {
			return
		}
		switch v.Dir {
		case widget.SwipeLeft:
			c.OnStep(1)
		case widget.SwipeRight:
			c.OnStep(-1)
		}
	case widget.Tap:
		if v.Count == 2 {
			px, py := c.toPhysical(v.X, v.Y)
			c.apply(func(f viewer.Frame) { c.view.ToggleFitActual(f, px, py) })
		}
	}
}
