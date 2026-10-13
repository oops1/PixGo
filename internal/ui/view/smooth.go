package view

import (
	"math"
	"time"

	"github.com/oops1/PixGo/internal/viewer"
)

const (
	smoothFrames   = 6
	smoothInterval = 20 * time.Millisecond
)

type smoothZoom struct {
	active bool
	target viewer.View
	from   float64
	px, py float64
	frame  int
	stop   func()
}

func (c *Canvas) smoothAllowed() bool {
	return c.Smooth && c.After != nil && c.HasImage() && c.areaW > 0 && c.areaH > 0
}

func (c *Canvas) smoothStep(steps int, px, py float64) {
	f := c.Frame()
	base := c.view
	if c.sm.active {
		base = c.sm.target
	}
	target := base
	target.StepZoom(f, steps, px, py)
	if target == base {
		return
	}
	c.sm.target = target
	c.sm.px, c.sm.py = px, py
	c.sm.from = c.view.EffectiveZoom(f)
	c.sm.frame = 0
	if !c.sm.active {
		c.sm.active = true
		c.scheduleSmooth()
	}
}

func (c *Canvas) scheduleSmooth() {
	c.sm.stop = c.After(smoothInterval, c.smoothTick)
}

func (c *Canvas) smoothTick() {
	c.sm.stop = nil
	if !c.sm.active {
		return
	}
	c.sm.frame++
	if c.sm.frame >= smoothFrames {
		c.finishSmooth()
		return
	}
	f := c.Frame()
	t := 1 - math.Pow(1-float64(c.sm.frame)/smoothFrames, 3)
	end := c.sm.target.EffectiveZoom(f)
	z := c.sm.from * math.Pow(end/c.sm.from, t)
	before := c.view
	c.view.ZoomTo(f, z, c.sm.px, c.sm.py)
	if c.view != before {
		c.invalidate()
		c.changed()
	}
	c.scheduleSmooth()
}

func (c *Canvas) finishSmooth() {
	if !c.sm.active {
		return
	}
	if c.sm.stop != nil {
		c.sm.stop()
		c.sm.stop = nil
	}
	c.sm.active = false
	if c.view != c.sm.target {
		c.view = c.sm.target
		c.invalidate()
		c.changed()
	}
}

func (c *Canvas) settle() { c.finishSmooth() }

func (c *Canvas) cancelSmooth() {
	if c.sm.stop != nil {
		c.sm.stop()
		c.sm.stop = nil
	}
	c.sm.active = false
}
