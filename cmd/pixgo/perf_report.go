package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/oops1/PixGo/internal/buildinfo"
	"github.com/oops1/PixGo/internal/imaging"
	"github.com/oops1/PixGo/internal/par"
	"github.com/oops1/PixGo/internal/platform"
	"github.com/oops1/PixGo/internal/settings"
)

const megabyte = 1 << 20

type perfMark struct {
	Name string  `json:"name"`
	Ms   float64 `json:"ms"`
}

type perfMemory struct {
	Phase        string  `json:"phase"`
	HeapInuse    uint64  `json:"heapInuse"`
	HeapAlloc    uint64  `json:"heapAlloc"`
	Sys          uint64  `json:"sys"`
	TotalAlloc   uint64  `json:"totalAlloc"`
	NumGC        uint32  `json:"numGC"`
	PauseTotalMs float64 `json:"gcPauseTotalMs"`
	Process      uint64  `json:"processBytes"`
	ProcessPeak  uint64  `json:"processPeakBytes"`
}

type machineInfo struct {
	CPU        string `json:"cpu"`
	Cores      int    `json:"logicalCores"`
	RAMBytes   uint64 `json:"ramBytes"`
	OS         string `json:"os"`
	Go         string `json:"go"`
	GOMAXPROCS int    `json:"gomaxprocs"`
}

type fileInfo struct {
	Path      string  `json:"path"`
	Bytes     int64   `json:"bytes"`
	Format    string  `json:"format"`
	Width     int     `json:"width"`
	Height    int     `json:"height"`
	Megapixel float64 `json:"megapixels"`
	Tiled     bool    `json:"tiled"`
	Preview   bool    `json:"previewStage"`
	Animated  bool    `json:"animated"`
}

type windowInfo struct {
	Width  int     `json:"width"`
	Height int     `json:"height"`
	Scale  float64 `json:"scale"`
}

type threadInfo struct {
	Configured int `json:"configured"`
	Effective  int `json:"effective"`
	Loader     int `json:"loaderWorkers"`
}

type frameSeries struct {
	Frames       int     `json:"frames"`
	Paced        bool    `json:"paced"`
	StepPixels   int     `json:"stepPixels"`
	P50Ms        float64 `json:"p50Ms"`
	P95Ms        float64 `json:"p95Ms"`
	MaxMs        float64 `json:"maxMs"`
	MeanMs       float64 `json:"meanMs"`
	Over16Ms     int     `json:"framesOver16ms"`
	Placeholders uint64  `json:"placeholderFrames"`
	WindowReqs   uint64  `json:"windowRequests"`
	TilesDecoded uint64  `json:"tilesDecoded"`
	SettleMs     float64 `json:"settleAfterMs"`
	Tiled        bool    `json:"tiled"`
}

type zoomStep struct {
	Percent  int     `json:"percent"`
	FrameMs  float64 `json:"firstFrameMs"`
	SettleMs float64 `json:"settleMs"`
}

type zoomSeries struct {
	Steps    []zoomStep `json:"steps"`
	P50Ms    float64    `json:"p50Ms"`
	P95Ms    float64    `json:"p95Ms"`
	MaxMs    float64    `json:"maxMs"`
	SettleMs float64    `json:"maxSettleMs"`
}

type damageResult struct {
	Scenario     string  `json:"scenario"`
	Seconds      float64 `json:"seconds"`
	Frames       int     `json:"frames"`
	FramesPerSec float64 `json:"framesPerSecond"`
	MeanPixels   float64 `json:"meanDamagePixels"`
	MaxPixels    int     `json:"maxDamagePixels"`
	MeanPercent  float64 `json:"meanDamagePercent"`
	MaxPercent   float64 `json:"maxDamagePercent"`
	WindowPixels int     `json:"windowPixels"`
	Skipped      string  `json:"skipped,omitempty"`
}

type perfReport struct {
	Version     string             `json:"version"`
	Date        string             `json:"date"`
	Machine     machineInfo        `json:"machine"`
	Window      windowInfo         `json:"window"`
	Threads     threadInfo         `json:"threads"`
	StartSource string             `json:"processStartSource"`
	File        *fileInfo          `json:"file,omitempty"`
	Marks       []perfMark         `json:"timeline"`
	Memory      []perfMemory       `json:"memory"`
	Pan         *frameSeries       `json:"pan,omitempty"`
	Zoom        *zoomSeries        `json:"zoom,omitempty"`
	Damage      []damageResult     `json:"damage,omitempty"`
	Gallery     *galleryResult     `json:"gallery,omitempty"`
	Tiles       *imaging.TileStats `json:"tileManager,omitempty"`
	Notes       []string           `json:"notes,omitempty"`
}

func (r *perfReport) finish(p *perfRun) {
	r.Version = buildinfo.String()
	r.Date = time.Now().Format(time.RFC3339)
	r.Machine = describeMachine()
	r.Window = windowInfo{Width: p.opt.Width, Height: p.opt.Height, Scale: max(p.opt.Scale, 1)}
	r.Threads.Configured = par.Configured()
	r.Threads.Effective = par.Count()
	r.StartSource = p.source
	r.Marks = p.marks
	r.Memory = p.memory
}

func toMs(d time.Duration) float64 { return float64(d) / float64(time.Millisecond) }

func (p *perfRun) snapshot(phase string) {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	snap := perfMemory{
		Phase:        phase,
		HeapInuse:    ms.HeapInuse,
		HeapAlloc:    ms.HeapAlloc,
		Sys:          ms.Sys,
		TotalAlloc:   ms.TotalAlloc,
		NumGC:        ms.NumGC,
		PauseTotalMs: float64(ms.PauseTotalNs) / 1e6,
	}
	if usage, err := platform.ProcessMemory(); err == nil {
		snap.Process, snap.ProcessPeak = usage.Current, usage.Peak
	}
	p.memory = append(p.memory, snap)
}

func describeMachine() machineInfo {
	return machineInfo{
		CPU:        cpuModel(),
		Cores:      runtime.NumCPU(),
		RAMBytes:   settings.TotalRAM(),
		OS:         osDescription(),
		Go:         runtime.Version(),
		GOMAXPROCS: runtime.GOMAXPROCS(0),
	}
}

func (r *perfReport) writeJSON(path string) error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func mb(b uint64) float64 { return float64(b) / megabyte }

func (r *perfReport) print(w io.Writer) error {
	var b strings.Builder
	r.printHeader(&b)
	r.printTimeline(&b)
	r.printMemory(&b)
	r.printFrames(&b)
	r.printDamage(&b)
	r.printGallery(&b)
	_, err := io.WriteString(w, b.String())
	return err
}

func (r *perfReport) printHeader(b *strings.Builder) {
	m := r.Machine
	fmt.Fprintf(b, "PixGo %s on %s\n", r.Version, m.OS)
	fmt.Fprintf(b, "CPU %s, %d logical cores, %.1f GiB RAM, %s, GOMAXPROCS %d\n", m.CPU, m.Cores, float64(m.RAMBytes)/(1<<30), m.Go, m.GOMAXPROCS)
	fmt.Fprintf(b, "window %dx%d at scale %g, threads %d (setting %d), loader workers %d\n", r.Window.Width, r.Window.Height, r.Window.Scale, r.Threads.Effective, r.Threads.Configured, r.Threads.Loader)
	if f := r.File; f != nil {
		mode := "direct"
		switch {
		case f.Tiled:
			mode = "tiled"
		case f.Preview:
			mode = "preview then full"
		}
		fmt.Fprintf(b, "file %s: %s %dx%d (%.1f Mpx), %.1f MB on disk, %s\n", f.Path, f.Format, f.Width, f.Height, f.Megapixel, float64(f.Bytes)/1e6, mode)
	}
	b.WriteString("\n")
}

func (r *perfReport) printTimeline(b *strings.Builder) {
	if len(r.Marks) == 0 {
		return
	}
	fmt.Fprintf(b, "Timeline, from process creation (taken from the %s)\n", r.StartSource)
	tw := tabwriter.NewWriter(b, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "event\tat ms\tstep ms")
	prev := 0.0
	for _, m := range r.Marks {
		fmt.Fprintf(tw, "%s\t%.1f\t%+.1f\n", m.Name, m.Ms, m.Ms-prev)
		prev = m.Ms
	}
	tw.Flush()
	b.WriteString("\n")
}

func (r *perfReport) printMemory(b *strings.Builder) {
	if len(r.Memory) == 0 {
		return
	}
	b.WriteString("Memory after each phase, MiB\n")
	tw := tabwriter.NewWriter(b, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "phase\theap in use\theap alloc\truntime sys\tprocess\tprocess peak\tallocated so far\tGC runs\tGC pause ms")
	for _, m := range r.Memory {
		fmt.Fprintf(tw, "%s\t%.0f\t%.0f\t%.0f\t%.0f\t%.0f\t%.0f\t%d\t%.1f\n", m.Phase, mb(m.HeapInuse), mb(m.HeapAlloc), mb(m.Sys), mb(m.Process), mb(m.ProcessPeak), mb(m.TotalAlloc), m.NumGC, m.PauseTotalMs)
	}
	tw.Flush()
	b.WriteString("\n")
}

func (r *perfReport) printFrames(b *strings.Builder) {
	if p := r.Pan; p != nil {
		pacing := "paced to 60 fps"
		if !p.Paced {
			pacing = "unpaced"
		}
		fmt.Fprintf(b, "Pan at 100%%, %d frames, %d px per frame, %s\n", p.Frames, p.StepPixels, pacing)
		fmt.Fprintf(b, "  frame ms: p50 %.2f  p95 %.2f  max %.2f  mean %.2f  over 16.7 ms: %d\n", p.P50Ms, p.P95Ms, p.MaxMs, p.MeanMs, p.Over16Ms)
		if p.Tiled {
			fmt.Fprintf(b, "  tiles: %d window requests, %d frames drawn with a placeholder, %d tiles decoded, sharp again %.0f ms after the last frame\n", p.WindowReqs, p.Placeholders, p.TilesDecoded, p.SettleMs)
		}
		b.WriteString("\n")
	}
	if z := r.Zoom; z != nil {
		b.WriteString("Zoom steps\n")
		tw := tabwriter.NewWriter(b, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "zoom %\tfirst frame ms\tsharp after ms")
		for _, s := range z.Steps {
			fmt.Fprintf(tw, "%d\t%.2f\t%.0f\n", s.Percent, s.FrameMs, s.SettleMs)
		}
		tw.Flush()
		fmt.Fprintf(b, "  first frame ms: p50 %.2f  p95 %.2f  max %.2f\n\n", z.P50Ms, z.P95Ms, z.MaxMs)
	}
}

func (r *perfReport) printDamage(b *strings.Builder) {
	if len(r.Damage) == 0 {
		return
	}
	b.WriteString("Damage per frame (tiles the engine reports as changed)\n")
	tw := tabwriter.NewWriter(b, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "scenario\tframes\tfps\tmean px\tmax px\tmean %\tmax %")
	for _, d := range r.Damage {
		if d.Skipped != "" {
			fmt.Fprintf(tw, "%s\tskipped: %s\t\t\t\t\t\n", d.Scenario, d.Skipped)
			continue
		}
		fmt.Fprintf(tw, "%s\t%d\t%.1f\t%.0f\t%d\t%.2f\t%.2f\n", d.Scenario, d.Frames, d.FramesPerSec, d.MeanPixels, d.MaxPixels, d.MeanPercent, d.MaxPercent)
	}
	tw.Flush()
	b.WriteString("\n")
}

func (r *perfReport) printGallery(b *strings.Builder) {
	g := r.Gallery
	if g == nil {
		return
	}
	fmt.Fprintf(b, "Gallery: %s, %d thumbnails of %d px, %d worker threads\n", g.Folder, g.Wanted, g.Side, g.Workers)
	tw := tabwriter.NewWriter(b, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "cache\tfirst ms\tall ready ms\tfailed")
	for _, run := range g.Runs {
		fmt.Fprintf(tw, "%s\t%.0f\t%.0f\t%d\n", run.Cache, run.FirstMs, run.AllMs, run.Failed)
	}
	tw.Flush()
	b.WriteString("\n")
}

func percentile(sorted []float64, q float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(q*float64(len(sorted)-1) + 0.5)
	return sorted[min(max(idx, 0), len(sorted)-1)]
}

func summarise(values []float64) (p50, p95, maxv, mean float64) {
	if len(values) == 0 {
		return
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	sum := 0.0
	for _, v := range sorted {
		sum += v
	}
	return percentile(sorted, 0.5), percentile(sorted, 0.95), sorted[len(sorted)-1], sum / float64(len(sorted))
}
