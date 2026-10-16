package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/oops1/headless-gui/v3/engine"

	"github.com/oops1/PixGo/internal/app"
	"github.com/oops1/PixGo/internal/platform"
)

var processInit = time.Now()

const (
	defaultPerfFrames     = 120
	defaultPerfPanStep    = 32
	defaultPerfGalleryN   = 24
	defaultPerfDamageSecs = 2.5
	defaultPerfRuns       = 1
	perfProfileHz         = 1000
	perfMemProfileRate    = 4096
)

type perfFlagSet struct {
	file       *string
	jsonPath   *string
	gallery    *string
	cpuProfile *string
	memProfile *string
	scenarios  *string
	runs       *int
	frames     *int
	panStep    *int
	galleryN   *int
	damageSecs *float64
	unpaced    *bool
}

func registerPerfFlags() *perfFlagSet {
	return &perfFlagSet{
		file:       flag.String("perf", "", "measure opening this file offscreen and print a table"),
		jsonPath:   flag.String("perf-json", "", "also write the measurements to this JSON file"),
		gallery:    flag.String("perf-gallery", "", "measure thumbnail generation for the images in this folder"),
		cpuProfile: flag.String("perf-cpuprofile", "", "write a CPU profile of the measured run to this file"),
		memProfile: flag.String("perf-memprofile", "", "write an allocation profile of the measured run to this file"),
		scenarios:  flag.String("perf-scenarios", "timeline,pan,zoom,damage", "comma separated scenarios to run for -perf"),
		runs:       flag.Int("perf-runs", defaultPerfRuns, "run the measurement in this many fresh processes and report medians"),
		frames:     flag.Int("perf-frames", defaultPerfFrames, "frames in the pan scenario"),
		panStep:    flag.Int("perf-pan-step", defaultPerfPanStep, "pixels the picture moves per frame in the pan scenario"),
		galleryN:   flag.Int("perf-gallery-n", defaultPerfGalleryN, "thumbnails to wait for in the gallery scenario"),
		damageSecs: flag.Float64("perf-damage-seconds", defaultPerfDamageSecs, "seconds to observe each damage scenario"),
		unpaced:    flag.Bool("perf-unpaced", false, "do not wait between pan frames (stress test instead of 60 fps pacing)"),
	}
}

type perfOptions struct {
	File       string
	JSONPath   string
	Gallery    string
	CPUProfile string
	MemProfile string
	Scenarios  map[string]bool
	Frames     int
	PanStep    int
	GalleryN   int
	DamageSecs float64
	Paced      bool
	Width      int
	Height     int
	Scale      float64
	Out        io.Writer
}

func (f *perfFlagSet) active() bool { return *f.file != "" || *f.gallery != "" }

func parseScenarios(list string) map[string]bool {
	out := map[string]bool{}
	for _, name := range strings.Split(list, ",") {
		if name = strings.TrimSpace(name); name != "" {
			out[name] = true
		}
	}
	return out
}

func (f *perfFlagSet) options(width, height int, scale float64) perfOptions {
	return perfOptions{
		File:       *f.file,
		JSONPath:   *f.jsonPath,
		Gallery:    *f.gallery,
		CPUProfile: *f.cpuProfile,
		MemProfile: *f.memProfile,
		Scenarios:  parseScenarios(*f.scenarios),
		Frames:     max(*f.frames, 1),
		PanStep:    max(*f.panStep, 1),
		GalleryN:   max(*f.galleryN, 1),
		DamageSecs: *f.damageSecs,
		Paced:      !*f.unpaced,
		Width:      width,
		Height:     height,
		Scale:      scale,
		Out:        os.Stdout,
	}
}

type perfRun struct {
	opt     perfOptions
	started time.Time
	source  string
	marks   []perfMark
	memory  []perfMemory
	stop    func()
}

func newPerfRun(opt perfOptions) *perfRun {
	p := &perfRun{opt: opt, started: processInit, source: "package init"}
	if t, err := platform.ProcessStart(); err == nil && !t.After(processInit) {
		p.started, p.source = t, "operating system"
	}
	stopCPU := startCPUProfile(opt.CPUProfile)
	stopMem := startMemProfile(opt.MemProfile)
	p.stop = func() {
		stopCPU()
		stopMem()
	}
	p.mark("packages initialised", processInit)
	p.snapshot("packages initialised")
	return p
}

func (p *perfRun) mark(name string, at time.Time) {
	if p == nil {
		return
	}
	p.marks = append(p.marks, perfMark{Name: name, Ms: toMs(at.Sub(p.started))})
}

func (p *perfRun) markNow(name string) {
	if p == nil {
		return
	}
	p.mark(name, time.Now())
}

func (p *perfRun) phase(name string) {
	if p == nil {
		return
	}
	p.markNow(name)
	p.snapshot(name)
}

func (p *perfRun) wants(name string) bool { return p.opt.Scenarios[name] }

func (p *perfRun) execute(eng *engine.Engine, a *app.App) int {
	p.phase("app ready")
	defer p.stop()
	rep := &perfReport{}
	switch {
	case p.opt.Gallery != "":
		rep.Gallery = measureGallery(a, p.opt)
	case p.opt.File != "":
		if err := p.measureFile(eng, a, rep); err != nil {
			fmt.Fprintln(os.Stderr, "pixgo: perf:", err)
			a.Shutdown()
			return 1
		}
	}
	rep.finish(p)
	a.Shutdown()
	if err := rep.print(p.opt.Out); err != nil {
		fmt.Fprintln(os.Stderr, "pixgo: perf:", err)
		return 1
	}
	if p.opt.JSONPath != "" {
		if err := rep.writeJSON(p.opt.JSONPath); err != nil {
			fmt.Fprintln(os.Stderr, "pixgo: perf:", err)
			return 1
		}
	}
	return 0
}

func perfSize(flagValue, fallback int) int {
	if flagValue > 0 {
		return flagValue
	}
	return fallback
}
