package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"image"
	"image/color"
	stdjpeg "image/jpeg"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"

	"github.com/oops1/PixGo/internal/app"
	"github.com/oops1/PixGo/internal/codec"
	codecall "github.com/oops1/PixGo/internal/codec/all"
	"github.com/oops1/PixGo/internal/par"
	"github.com/oops1/PixGo/internal/settings"
	"github.com/oops1/PixGo/internal/testutil"
)

func writePerfJPEG(t testing.TB, path string, w, h int, seed int64) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	rnd := rand.New(rand.NewSource(seed))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			n := uint8(rnd.Intn(24))
			img.SetRGBA(x, y, color.RGBA{uint8(x*255/w) + n, uint8(y*255/h) + n, uint8((x+y)*255/(w+h)) + n, 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := stdjpeg.Encode(f, img, &stdjpeg.Options{Quality: 85}); err != nil {
		t.Fatal(err)
	}
}

func newPerfApp(t *testing.T, w, h int) (*engine.Engine, *app.App) {
	t.Helper()
	t.Setenv("PIXGO_CONFIG_DIR", t.TempDir())
	t.Setenv("PIXGO_STATE_DIR", t.TempDir())
	t.Setenv("PIXGO_CACHE_DIR", t.TempDir())
	t.Cleanup(func() { par.Set(0) })
	eng := testutil.NewEngine(w, h)
	t.Cleanup(eng.Stop)
	reg := codec.NewRegistry()
	codecall.Register(reg)
	a := app.New(eng, app.Options{Settings: settings.NewMemory(settings.Defaults()), Registry: reg})
	eng.SetRootFullCanvas(a.Root)
	return eng, a
}

func smokeOptions(file string, out *bytes.Buffer) perfOptions {
	return perfOptions{
		File:       file,
		Scenarios:  parseScenarios("timeline,pan,zoom,damage"),
		Frames:     24,
		PanStep:    20,
		GalleryN:   4,
		DamageSecs: 0.4,
		Width:      900,
		Height:     600,
		Scale:      1,
		Out:        out,
	}
}

func TestPerfHarnessMeasuresASmallJPEG(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "a.jpg")
	writePerfJPEG(t, first, 2400, 1600, 1)
	writePerfJPEG(t, filepath.Join(dir, "b.jpg"), 640, 480, 2)
	eng, a := newPerfApp(t, 900, 600)
	var out bytes.Buffer
	opt := smokeOptions(first, &out)
	opt.JSONPath = filepath.Join(dir, "perf.json")
	opt.CPUProfile = filepath.Join(dir, "cpu.prof")
	opt.MemProfile = filepath.Join(dir, "mem.prof")
	p := newPerfRun(opt)
	if code := p.execute(eng, a); code != 0 {
		t.Fatalf("exit code %d\n%s", code, out.String())
	}
	text := out.String()
	for _, want := range []string{"Timeline", "first picture rendered", "full resolution ready", "Memory after each phase", "Pan at 100%", "Zoom steps", "Damage per frame", "slideshow bar"} {
		if !strings.Contains(text, want) {
			t.Errorf("report lacks %q:\n%s", want, text)
		}
	}
	var rep perfReport
	data, err := os.ReadFile(opt.JSONPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &rep); err != nil {
		t.Fatal(err)
	}
	if rep.File == nil || rep.File.Width != 2400 || rep.File.Height != 1600 || rep.File.Format != "JPEG" {
		t.Fatalf("file info %+v", rep.File)
	}
	if rep.Machine.Cores < 1 || rep.Machine.Go == "" || rep.Window.Width != 900 {
		t.Fatalf("machine %+v window %+v", rep.Machine, rep.Window)
	}
	assertTimelineOrder(t, rep.Marks)
	if rep.Pan == nil || rep.Pan.Frames != 24 || rep.Pan.P95Ms <= 0 || rep.Pan.MaxMs < rep.Pan.P50Ms {
		t.Fatalf("pan %+v", rep.Pan)
	}
	if rep.Zoom == nil || len(rep.Zoom.Steps) < 2 {
		t.Fatalf("zoom %+v", rep.Zoom)
	}
	if len(rep.Damage) != 2 || rep.Damage[1].Skipped != "" || rep.Damage[1].Frames == 0 || rep.Damage[1].MeanPercent <= 0 {
		t.Fatalf("damage %+v", rep.Damage)
	}
	if len(rep.Memory) < len(rep.Marks)/2 || rep.Memory[len(rep.Memory)-1].ProcessPeak == 0 {
		t.Fatalf("memory %+v", rep.Memory)
	}
	for _, name := range []string{opt.CPUProfile, opt.MemProfile} {
		if st, err := os.Stat(name); err != nil || st.Size() == 0 {
			t.Errorf("profile %s: %v", name, err)
		}
	}
}

func assertTimelineOrder(t *testing.T, marks []perfMark) {
	t.Helper()
	want := []string{"packages initialised", "app ready", "open requested", "folder scanned", "loading frame rendered", "probe done", "first picture published", "first picture rendered", "full resolution ready", "first full frame rendered"}
	at := map[string]float64{}
	for _, m := range marks {
		at[m.Name] = m.Ms
	}
	prev := -1.0
	for _, name := range want {
		ms, ok := at[name]
		if !ok {
			t.Fatalf("timeline lacks %q: %+v", name, marks)
		}
		if ms < prev {
			t.Fatalf("%q at %.2f ms comes before the previous event at %.2f ms", name, ms, prev)
		}
		prev = ms
	}
}

func TestPerfHarnessReportsASingleImageFolderWithoutSlideshow(t *testing.T) {
	dir := t.TempDir()
	only := filepath.Join(dir, "only.jpg")
	writePerfJPEG(t, only, 800, 600, 3)
	eng, a := newPerfApp(t, 700, 500)
	var out bytes.Buffer
	opt := smokeOptions(only, &out)
	opt.Scenarios = parseScenarios("damage")
	p := newPerfRun(opt)
	if code := p.execute(eng, a); code != 0 {
		t.Fatalf("exit code %d", code)
	}
	if !strings.Contains(out.String(), "the folder holds one image") {
		t.Fatalf("a single image cannot run a slideshow:\n%s", out.String())
	}
}

func TestPerfHarnessFailsOnAMissingFile(t *testing.T) {
	eng, a := newPerfApp(t, 600, 400)
	var out bytes.Buffer
	opt := smokeOptions(filepath.Join(t.TempDir(), "missing.jpg"), &out)
	p := newPerfRun(opt)
	if code := p.execute(eng, a); code == 0 {
		t.Fatal("a missing file must fail the run")
	}
}

func TestPerfHarnessFailsOnACorruptFile(t *testing.T) {
	bad := filepath.Join(t.TempDir(), "bad.jpg")
	if err := os.WriteFile(bad, []byte("\xff\xd8\xff\xe0 not a jpeg"), 0o644); err != nil {
		t.Fatal(err)
	}
	eng, a := newPerfApp(t, 600, 400)
	var out bytes.Buffer
	p := newPerfRun(smokeOptions(bad, &out))
	if code := p.execute(eng, a); code == 0 {
		t.Fatal("a corrupt file must fail the run")
	}
}

func TestPerfGalleryMeasuresColdAndWarmThumbnails(t *testing.T) {
	dir := t.TempDir()
	for i, size := range [][2]int{{1600, 1200}, {1200, 1600}, {800, 600}} {
		writePerfJPEG(t, filepath.Join(dir, string(rune('a'+i))+".jpg"), size[0], size[1], int64(i))
	}
	os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("not an image"), 0o644)
	eng, a := newPerfApp(t, 600, 400)
	var out bytes.Buffer
	opt := smokeOptions("", &out)
	opt.Gallery = dir
	opt.Scenarios = parseScenarios("")
	p := newPerfRun(opt)
	if code := p.execute(eng, a); code != 0 {
		t.Fatalf("exit code %d\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "Gallery:") || !strings.Contains(out.String(), "cold") || !strings.Contains(out.String(), "warm") {
		t.Fatalf("gallery table missing:\n%s", out.String())
	}
}

func TestMeasureGalleryHandlesMissingAndEmptyFolders(t *testing.T) {
	_, a := newPerfApp(t, 600, 400)
	for _, dir := range []string{filepath.Join(t.TempDir(), "nope"), t.TempDir()} {
		res := measureGallery(a, perfOptions{Gallery: dir, GalleryN: 3})
		if res.Wanted != 0 || len(res.Runs) != 0 {
			t.Errorf("%s: %+v", dir, res)
		}
	}
}

func TestChildArgumentsDropTheRepeatFlags(t *testing.T) {
	cases := []struct {
		in, want []string
	}{
		{[]string{"-perf", "a.jpg", "-perf-runs", "5", "-perf-json", "out.json"}, []string{"-perf", "a.jpg"}},
		{[]string{"-perf=a.jpg", "-perf-runs=5", "--perf-json=x", "-width", "800"}, []string{"-perf=a.jpg", "-width", "800"}},
		{[]string{"-perf", "-perf-runs"}, []string{"-perf"}},
	}
	for _, c := range cases {
		got := childArguments(c.in, "tmp.json")
		want := append(append([]string(nil), c.want...), "-perf-runs=1", "-perf-json=tmp.json")
		if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
			t.Errorf("childArguments(%q) = %q, want %q", c.in, got, want)
		}
	}
}

func TestSpreadAndSummarise(t *testing.T) {
	if m, lo, hi := spread(nil); m != 0 || lo != 0 || hi != 0 {
		t.Fatal("empty spread")
	}
	if m, lo, hi := spread([]float64{5, 1, 9}); m != 5 || lo != 1 || hi != 9 {
		t.Fatalf("odd spread %v %v %v", m, lo, hi)
	}
	if m, _, _ := spread([]float64{4, 2, 8, 6}); m != 5 {
		t.Fatalf("even median %v", m)
	}
	p50, p95, mx, mean := summarise([]float64{1, 2, 3, 4, 100})
	if p50 != 3 || mx != 100 || p95 != 100 || mean != 22 {
		t.Fatalf("summary %v %v %v %v", p50, p95, mx, mean)
	}
	if a, b, c, d := summarise(nil); a+b+c+d != 0 {
		t.Fatal("empty summary")
	}
}

func TestPrintAggregateCombinesRuns(t *testing.T) {
	mk := func(shift float64) *perfReport {
		return &perfReport{
			Version: "test", Machine: machineInfo{CPU: "cpu", Cores: 4, OS: "os", Go: "go"},
			Window: windowInfo{Width: 100, Height: 80, Scale: 1},
			Marks:  []perfMark{{Name: "start", Ms: 10 + shift}, {Name: "done", Ms: 50 + shift}},
			Memory: []perfMemory{{Phase: "done", ProcessPeak: uint64(100+shift) << 20}},
			Pan:    &frameSeries{P50Ms: 3 + shift, P95Ms: 4 + shift, MaxMs: 5 + shift, Placeholders: 2},
		}
	}
	var out bytes.Buffer
	printAggregate(&out, []*perfReport{mk(0), mk(10), mk(20)})
	text := out.String()
	for _, want := range []string{"3 runs", "start", "done", "30.0", "110", "Pan frame ms"} {
		if !strings.Contains(text, want) {
			t.Errorf("aggregate lacks %q:\n%s", want, text)
		}
	}
	printAggregate(&out, nil)
}

func TestPerfFlagsOnlyActivateWithAFileOrFolder(t *testing.T) {
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	old := flag.CommandLine
	flag.CommandLine = fs
	defer func() { flag.CommandLine = old }()
	f := registerPerfFlags()
	if err := fs.Parse(nil); err != nil || f.active() {
		t.Fatalf("no flags must leave the harness off: %v", err)
	}
	if err := fs.Parse([]string{"-perf", "x.jpg", "-perf-frames", "7", "-perf-unpaced", "-perf-scenarios", "pan, zoom"}); err != nil {
		t.Fatal(err)
	}
	opt := f.options(0, 0, 1.5)
	if !f.active() || opt.File != "x.jpg" || opt.Frames != 7 || opt.Paced || !opt.Scenarios["pan"] || !opt.Scenarios["zoom"] || opt.Scenarios["damage"] {
		t.Fatalf("options %+v", opt)
	}
	if perfSize(0, 1100) != 1100 || perfSize(640, 1100) != 640 {
		t.Fatal("window size fallback")
	}
}

func TestStartCPUProfileToAnUnwritablePathIsHarmless(t *testing.T) {
	stop := startCPUProfile(filepath.Join(t.TempDir(), "missing-dir", "cpu.prof"))
	stop()
	stop = startMemProfile(filepath.Join(t.TempDir(), "missing-dir", "mem.prof"))
	stop()
	startCPUProfile("")()
	startMemProfile("")()
}

func init() { registerPerfFlags() }

const repeatHelperEnv = "PIXGO_PERF_REPEAT_HELPER"

func TestRepeatHelperProcess(t *testing.T) {
	mode := os.Getenv(repeatHelperEnv)
	if mode == "" {
		t.Skip("only runs as a child of TestRunRepeated")
	}
	var path string
	for _, a := range os.Args {
		if v, ok := strings.CutPrefix(a, "-perf-json="); ok {
			path = v
		}
	}
	if mode == "fail" {
		os.Exit(3)
	}
	if mode == "garbage" {
		os.WriteFile(path, []byte("{"), 0o644)
		os.Exit(0)
	}
	rep := perfReport{
		Version: "child",
		Marks:   []perfMark{{Name: "start", Ms: 11}, {Name: "done", Ms: 33}},
		Memory:  []perfMemory{{Phase: "done", ProcessPeak: 64 << 20}},
		Pan:     &frameSeries{P50Ms: 2, P95Ms: 3, MaxMs: 4},
	}
	data, _ := json.Marshal(rep)
	os.WriteFile(path, data, 0o644)
	os.Exit(0)
}

func TestRunRepeatedAggregatesChildRuns(t *testing.T) {
	t.Setenv(repeatHelperEnv, "ok")
	dir := t.TempDir()
	merged := filepath.Join(dir, "all.json")
	var out bytes.Buffer
	args := []string{"-test.run=TestRepeatHelperProcess", "-perf", "x.jpg", "-perf-runs", "3"}
	if code := runRepeated(os.Args[0], 3, args, merged, &out); code != 0 {
		t.Fatalf("exit code %d\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "3 runs") || !strings.Contains(out.String(), "33.0") {
		t.Fatalf("aggregate output:\n%s", out.String())
	}
	var all []perfReport
	data, err := os.ReadFile(merged)
	if err != nil || json.Unmarshal(data, &all) != nil || len(all) != 3 {
		t.Fatalf("merged JSON: %v %d", err, len(all))
	}
}

func TestRunRepeatedReportsChildFailures(t *testing.T) {
	args := []string{"-test.run=TestRepeatHelperProcess", "-perf", "x.jpg"}
	for _, mode := range []string{"fail", "garbage"} {
		t.Setenv(repeatHelperEnv, mode)
		var out bytes.Buffer
		if code := runRepeated(os.Args[0], 2, args, "", &out); code == 0 {
			t.Errorf("%s: a failing child must fail the run", mode)
		}
	}
	if code := runRepeated(filepath.Join(t.TempDir(), "no-such-binary"), 1, nil, "", &bytes.Buffer{}); code == 0 {
		t.Error("a missing executable must fail the run")
	}
}
