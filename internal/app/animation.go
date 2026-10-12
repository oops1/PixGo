package app

import (
	"context"
	"time"

	"github.com/oops1/PixGo/internal/imaging"
)

type contextT = context.Context

const (
	defaultFrameDelay = 100 * time.Millisecond
)

type animation struct {
	owner   *Controller
	decoded *imaging.Decoded
	frame   int
	plays   int
	active  bool
	paused  bool
	stopFn  func()
	gen     int
}

func (a *animation) start(d *imaging.Decoded) {
	a.stop()
	a.decoded = d
	a.frame = 0
	a.plays = 0
	a.gen++
	if d == nil || len(d.Frames) < 2 {
		a.decoded = nil
		return
	}
	a.active = true
	a.schedule()
}

func (a *animation) stop() {
	a.active = false
	a.decoded = nil
	a.gen++
	a.cancel()
}

func (a *animation) cancel() {
	if a.stopFn != nil {
		a.stopFn()
		a.stopFn = nil
	}
}

func (a *animation) setActive(on bool) {
	a.paused = !on
	if a.decoded == nil {
		return
	}
	if on {
		if a.stopFn == nil {
			a.schedule()
		}
		return
	}
	a.cancel()
}

func (a *animation) minDelay() time.Duration {
	fps := a.owner.env.Settings.Get().AnimationFPS
	if fps <= 0 {
		return 0
	}
	return time.Second / time.Duration(fps)
}

func (a *animation) delayOf(i int) time.Duration {
	d := a.decoded.Frames[i].Delay
	if d <= 0 {
		d = defaultFrameDelay
	}
	return max(d, a.minDelay())
}

func (a *animation) schedule() {
	if a.paused || a.decoded == nil || a.owner.env.After == nil {
		return
	}
	gen := a.gen
	a.stopFn = a.owner.env.After(a.delayOf(a.frame), func() {
		if gen != a.gen || a.decoded == nil {
			return
		}
		a.stopFn = nil
		a.advance()
	})
}

func (a *animation) advance() {
	n := len(a.decoded.Frames)
	next := a.frame + 1
	if next >= n {
		a.plays++
		loop := a.decoded.Loop
		if loop > 0 && a.plays >= loop {
			a.active = false
			return
		}
		next = 0
	}
	a.frame = next
	a.owner.canvas.ReplaceFrame(a.decoded.FrameSource(next))
	a.schedule()
}
