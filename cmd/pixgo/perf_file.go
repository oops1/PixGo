package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/app"
	"github.com/oops1/PixGo/internal/imaging"
)

const (
	perfLoadLimit    = 90 * time.Second
	perfPollInterval = 200 * time.Microsecond
	perfSettleLimit  = 20 * time.Second

	perfBackgroundLimit = 10 * time.Second
	perfFrameBudget     = 16667 * time.Microsecond
	perfSlowFrame       = 16 * time.Millisecond
	perfMaxZoomSteps    = 14
	perfTopZoom         = 800
)

type perfDriver struct {
	eng *engine.Engine
	a   *app.App
}

func (d perfDriver) frame() time.Duration {
	start := time.Now()
	d.eng.RenderFrameNow()
	return time.Since(start)
}

func (d perfDriver) tiles() *imaging.TileManager {
	if shown := d.a.Ctl.Shown(); shown != nil {
		if m, ok := shown.Detail.(*imaging.TileManager); ok {
			return m
		}
	}
	return nil
}

func (d perfDriver) settle() time.Duration {
	m := d.tiles()
	if m == nil {
		return 0
	}
	start := time.Now()
	quiet := 0
	for time.Since(start) < perfSettleLimit {
		d.eng.Flush()
		before := m.Stats().Placeholders
		d.frame()
		s := m.Stats()
		if s.Placeholders == before && s.Pending == 0 && s.Inflight == 0 {
			quiet++
			if quiet >= 2 {
				return time.Since(start)
			}
		} else {
			quiet = 0
		}
		time.Sleep(perfPollInterval)
	}
	return perfSettleLimit
}

func (p *perfRun) measureFile(eng *engine.Engine, a *app.App, rep *perfReport) error {
	path, err := filepath.Abs(p.opt.File)
	if err != nil {
		return err
	}
	st, err := os.Stat(path)
	if err != nil {
		return err
	}
	rep.File = &fileInfo{Path: path, Bytes: st.Size()}
	d := perfDriver{eng: eng, a: a}
	if err := p.timeline(d, rep); err != nil {
		return err
	}
	a.Canvas.Smooth = false
	if p.wants("pan") {
		rep.Pan = p.pan(d)
		p.phase("pan done")
	}
	if p.wants("zoom") {
		rep.Zoom = p.zoom(d)
		p.phase("zoom done")
	}
	if m := d.tiles(); m != nil {
		stats := m.Stats()
		rep.Tiles = &stats
	}
	if p.wants("damage") {
		rep.Damage = p.damage(eng, a)
		p.phase("damage done")
	}
	rep.Threads.Loader = a.Ctl.LoaderWorkers()
	return nil
}

func (p *perfRun) timeline(d perfDriver, rep *perfReport) error {
	a, eng := d.a, d.eng
	p.markNow("open requested")
	if err := a.Ctl.Open(p.opt.File); err != nil {
		return err
	}
	a.Root.Refresh()
	p.phase("folder scanned")
	d.frame()
	p.phase("loading frame rendered")
	probed, published, sawPreview := false, false, false
	deadline := time.Now().Add(perfLoadLimit)
	for time.Now().Before(deadline) {
		eng.Flush()
		if !probed && a.Ctl.Info().Width > 0 {
			probed = true
			p.phase("probe done")
		}
		if shown := a.Ctl.Shown(); !published && shown != nil {
			published = true
			sawPreview = shown.Reduced
			p.phase("first picture published")
			a.Root.Refresh()
			d.frame()
			p.phase("first picture rendered")
		}
		switch a.Ctl.Status() {
		case app.StatusReady:
			p.phase("full resolution ready")
			a.Root.Refresh()
			d.frame()
			p.phase("first full frame rendered")
			p.describeFile(d, rep, sawPreview)
			p.settleBackground(d)
			return nil
		case app.StatusFailed:
			return fmt.Errorf("%s: %w", p.opt.File, orFailed(a.Ctl.Err()))
		}
		time.Sleep(perfPollInterval)
	}
	return fmt.Errorf("%s did not finish loading in %v", p.opt.File, perfLoadLimit)
}

func (p *perfRun) settleBackground(d perfDriver) {
	deadline := time.Now().Add(perfBackgroundLimit)
	for time.Now().Before(deadline) {
		d.eng.Flush()
		if d.a.Ctl.LoaderPending() == 0 && !d.a.Ctl.Scanning() {
			break
		}
		time.Sleep(perfPollInterval)
	}
	d.eng.Flush()
	p.phase("background loads done")
}

func orFailed(err error) error {
	if err == nil {
		return errors.New("loading failed")
	}
	return err
}

func (p *perfRun) describeFile(d perfDriver, rep *perfReport, sawPreview bool) {
	info := d.a.Ctl.Info()
	rep.File.Format = info.FormatName
	rep.File.Width, rep.File.Height = info.Width, info.Height
	rep.File.Megapixel = float64(info.Width) * float64(info.Height) / 1e6
	rep.File.Animated = info.Animated
	rep.File.Preview = sawPreview
	rep.File.Tiled = d.tiles() != nil
}

func (p *perfRun) pan(d perfDriver) *frameSeries {
	a := d.a
	a.Canvas.Actual()
	d.frame()
	d.settle()
	series := &frameSeries{Frames: p.opt.Frames, Paced: p.opt.Paced, StepPixels: p.opt.PanStep, Tiled: d.tiles() != nil}
	var before imaging.TileStats
	if m := d.tiles(); m != nil {
		before = m.Stats()
	}
	times := d.dragFrames(p.opt)
	for _, t := range times {
		if t > toMs(perfSlowFrame) {
			series.Over16Ms++
		}
	}
	series.P50Ms, series.P95Ms, series.MaxMs, series.MeanMs = summarise(times)
	if m := d.tiles(); m != nil {
		after := m.Stats()
		series.Placeholders = after.Placeholders - before.Placeholders
		series.WindowReqs = after.Requests - before.Requests
		series.TilesDecoded = after.Decoded - before.Decoded
	}
	series.SettleMs = toMs(d.settle())
	return series
}

func (d perfDriver) dragFrames(opt perfOptions) []float64 {
	canvas := d.a.Canvas
	b := canvas.Bounds()
	cx, cy := (b.Min.X+b.Max.X)/2, (b.Min.Y+b.Max.Y)/2
	step := opt.PanStep
	leg := max(8, int(0.7*float64(b.Dx())/float64(step)))
	times := make([]float64, 0, opt.Frames)
	for len(times) < opt.Frames {
		x := cx - leg*step/2
		y := cy - leg*step/6
		canvas.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft, Pressed: true, Clicks: 1})
		for i := 0; i < leg && len(times) < opt.Frames; i++ {
			frameStart := time.Now()
			x += step
			y += step / 3
			canvas.OnMouseMove(x, y)
			d.eng.Flush()
			times = append(times, toMs(d.frame()))
			if opt.Paced {
				if rest := perfFrameBudget - time.Since(frameStart); rest > 0 {
					time.Sleep(rest)
				}
			}
		}
		canvas.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft, Pressed: false, Clicks: 1})
	}
	return times
}

func (p *perfRun) zoom(d perfDriver) *zoomSeries {
	a := d.a
	a.Canvas.Fit()
	d.frame()
	d.settle()
	series := &zoomSeries{}
	var frames []float64
	record := func() {
		step := zoomStep{Percent: a.Canvas.ZoomPercent()}
		step.FrameMs = toMs(d.frame())
		step.SettleMs = toMs(d.settle())
		series.Steps = append(series.Steps, step)
		frames = append(frames, step.FrameMs)
		series.SettleMs = max(series.SettleMs, step.SettleMs)
	}
	for i := 0; i < perfMaxZoomSteps && a.Canvas.ZoomPercent() < perfTopZoom; i++ {
		a.Canvas.ZoomIn()
		record()
	}
	for i := 0; i < perfMaxZoomSteps*2 && a.Canvas.ZoomPercent() > 1; i++ {
		before := a.Canvas.ZoomPercent()
		a.Canvas.ZoomOut()
		if a.Canvas.ZoomPercent() == before {
			break
		}
		record()
	}
	series.P50Ms, series.P95Ms, series.MaxMs, _ = summarise(frames)
	return series
}
