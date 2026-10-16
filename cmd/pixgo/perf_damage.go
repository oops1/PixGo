package main

import (
	"fmt"
	"sync"
	"time"

	"github.com/oops1/headless-gui/v3/engine"

	"github.com/oops1/PixGo/internal/app"
)

const damageWarmup = 400 * time.Millisecond

type damageSample struct {
	at     time.Time
	pixels int
}

type damageCollector struct {
	mu      sync.Mutex
	samples []damageSample
	done    chan struct{}
}

func collectDamage(eng *engine.Engine) *damageCollector {
	c := &damageCollector{done: make(chan struct{})}
	go func() {
		defer close(c.done)
		for frame := range eng.Frames() {
			pixels := 0
			for _, t := range frame.Tiles {
				pixels += t.W * t.H
			}
			c.mu.Lock()
			c.samples = append(c.samples, damageSample{at: time.Now(), pixels: pixels})
			c.mu.Unlock()
		}
	}()
	return c
}

func (c *damageCollector) between(from, to time.Time) []damageSample {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []damageSample
	for _, s := range c.samples {
		if !s.at.Before(from) && s.at.Before(to) {
			out = append(out, s)
		}
	}
	return out
}

func summariseDamage(name string, samples []damageSample, seconds float64, window int) damageResult {
	res := damageResult{Scenario: name, Seconds: seconds, Frames: len(samples), WindowPixels: window}
	if seconds > 0 {
		res.FramesPerSec = float64(len(samples)) / seconds
	}
	total := 0
	for _, s := range samples {
		total += s.pixels
		res.MaxPixels = max(res.MaxPixels, s.pixels)
	}
	if len(samples) > 0 {
		res.MeanPixels = float64(total) / float64(len(samples))
	}
	if window > 0 {
		res.MeanPercent = res.MeanPixels * 100 / float64(window)
		res.MaxPercent = float64(res.MaxPixels) * 100 / float64(window)
	}
	return res
}

func (p *perfRun) observe(c *damageCollector, name string, window int) damageResult {
	time.Sleep(damageWarmup)
	from := time.Now()
	time.Sleep(time.Duration(p.opt.DamageSecs * float64(time.Second)))
	to := time.Now()
	return summariseDamage(name, c.between(from, to), to.Sub(from).Seconds(), window)
}

func (p *perfRun) damage(eng *engine.Engine, a *app.App) []damageResult {
	w, h := eng.PhysicalSize()
	window := w * h
	collector := collectDamage(eng)
	eng.Start()
	defer func() {
		eng.Stop()
		<-collector.done
	}()

	var out []damageResult
	name := "still picture"
	if a.Ctl.Info().Animated {
		name = fmt.Sprintf("animation, %d frames", a.Ctl.Info().Frames)
	}
	out = append(out, p.observe(collector, name, window))

	if n := a.Ctl.Navigator(); n == nil || n.Count() < 2 {
		return append(out, damageResult{Scenario: "slideshow bar", Skipped: "the folder holds one image"})
	}
	eng.Post(func() {
		a.Commands.Run(app.CmdSlideshow)
		a.Root.Wake()
	})
	eng.Flush()
	out = append(out, p.observe(collector, "slideshow bar", window))
	eng.Post(func() { a.Commands.Run(app.CmdSlideshow) })
	eng.Flush()
	return out
}
