package thumbs

import (
	"context"
	"errors"
	"image/color"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oops1/PixGo/internal/codec"
)

func TestRequestQueuesThenDelivers(t *testing.T) {
	reg, f := fakeRegistry()
	hn := newHarness(t, reg, Options{})
	p := hn.writeFake("one")[0]
	var got []string
	unsub := hn.svc.Subscribe(func(path string) { got = append(got, path) })
	if _, ok := hn.svc.Request(p, 128, true); ok {
		t.Fatal("first request cannot be ready")
	}
	hn.waitUntil(func() bool { return len(got) == 1 })
	if got[0] != p {
		t.Fatalf("notified %q", got[0])
	}
	th, ok := hn.svc.Request(p, 128, true)
	if !ok || th.Err != nil || th.Img == nil {
		t.Fatalf("second request: ok=%v thumb=%+v", ok, th)
	}
	if th.Img.Bounds().Dx() != 128 || th.Img.Bounds().Dy() != 85 {
		t.Fatalf("size %v", th.Img.Bounds())
	}
	if th.SrcW != 600 || th.SrcH != 400 || th.Orientation != codec.OrientNormal {
		t.Fatalf("metadata %+v", th)
	}
	unsub()
	p2 := hn.writeFake("two")[0]
	hn.get(p2, 128)
	if len(got) != 1 {
		t.Fatalf("unsubscribed callback still called: %v", got)
	}
	if f.decodeCount("one") != 1 {
		t.Fatal("decoded more than once")
	}
}

func TestRequestDeduplicatesJobs(t *testing.T) {
	reg, f := fakeRegistry()
	hn := newHarness(t, reg, Options{Workers: 1})
	gate := f.gate("slow")
	p := hn.writeFake("slow")[0]
	for i := 0; i < 20; i++ {
		if _, ok := hn.svc.Request(p, 128, i%2 == 0); ok {
			t.Fatal("ready too early")
		}
	}
	f.waitStarted(t, "slow")
	for i := 0; i < 20; i++ {
		hn.svc.Request(p, 128, true)
	}
	close(gate)
	hn.get(p, 128)
	if n := f.decodeCount("slow"); n != 1 {
		t.Fatalf("decoded %d times", n)
	}
}

func TestDifferentSidesAreSeparateJobs(t *testing.T) {
	reg, f := fakeRegistry()
	hn := newHarness(t, reg, Options{})
	p := hn.writeFake("multi")[0]
	a := hn.get(p, 96)
	b := hn.get(p, 192)
	if a.Img.Bounds().Dx() != 96 || b.Img.Bounds().Dx() != 192 {
		t.Fatalf("sizes %v %v", a.Img.Bounds(), b.Img.Bounds())
	}
	if f.decodeCount("multi") != 2 {
		t.Fatalf("decodes = %d", f.decodeCount("multi"))
	}
}

func TestVisibleJobsFinishFirst(t *testing.T) {
	reg, f := fakeRegistry()
	hn := newHarness(t, reg, Options{Workers: 1})
	gate := f.gate("block")
	paths := hn.writeFake("block", "a", "b", "c", "d", "e")
	hn.svc.Request(paths[0], 128, false)
	f.waitStarted(t, "block")
	for _, p := range paths[1:] {
		hn.svc.Request(p, 128, false)
	}
	hn.svc.SetVisible([]string{paths[5], paths[4]})
	close(gate)
	hn.waitUntil(func() bool { return len(f.finished()) == 6 })
	want := []string{"block", "e", "d", "a", "b", "c"}
	if got := f.finished(); !slices.Equal(got, want) {
		t.Fatalf("finished order %v, want %v", got, want)
	}
}

func TestVisibleFlagOnRequestPromotes(t *testing.T) {
	reg, f := fakeRegistry()
	hn := newHarness(t, reg, Options{Workers: 1})
	gate := f.gate("block")
	paths := hn.writeFake("block", "a", "b", "c")
	hn.svc.Request(paths[0], 128, false)
	f.waitStarted(t, "block")
	hn.svc.Request(paths[1], 128, false)
	hn.svc.Request(paths[2], 128, false)
	hn.svc.Request(paths[3], 128, true)
	hn.svc.Request(paths[2], 128, true)
	close(gate)
	hn.waitUntil(func() bool { return len(f.finished()) == 4 })
	want := []string{"block", "c", "b", "a"}
	if got := f.finished(); !slices.Equal(got, want) {
		t.Fatalf("finished order %v, want %v", got, want)
	}
}

func TestSetVisibleKeepsOrderOfTheGivenList(t *testing.T) {
	reg, f := fakeRegistry()
	hn := newHarness(t, reg, Options{Workers: 1})
	gate := f.gate("block")
	paths := hn.writeFake("block", "a", "b", "c", "d")
	hn.svc.Request(paths[0], 128, false)
	f.waitStarted(t, "block")
	for _, p := range paths[1:] {
		hn.svc.Request(p, 128, false)
	}
	hn.svc.SetVisible([]string{paths[3], paths[1], paths[3], paths[2]})
	close(gate)
	hn.waitUntil(func() bool { return len(f.finished()) == 5 })
	want := []string{"block", "c", "a", "b", "d"}
	if got := f.finished(); !slices.Equal(got, want) {
		t.Fatalf("finished order %v, want %v", got, want)
	}
}

func TestScrolledAwayJobsAreCancelled(t *testing.T) {
	reg, f := fakeRegistry()
	hn := newHarness(t, reg, Options{Workers: 1})
	f.gate("run")
	cancelled := f.watchCancel("run")
	paths := hn.writeFake("run", "queued", "keep")
	hn.svc.Request(paths[0], 128, true)
	f.waitStarted(t, "run")
	hn.svc.Request(paths[1], 128, false)
	hn.svc.Request(paths[2], 128, false)
	hn.clock.advance(3 * time.Second)
	hn.svc.SetVisible([]string{paths[2]})
	select {
	case <-cancelled:
	case <-time.After(10 * time.Second):
		t.Fatal("running job was not cancelled")
	}
	hn.waitUntil(func() bool {
		_, ok := hn.svc.Request(paths[2], 128, true)
		return ok
	})
	if f.decodeCount("queued") != 0 {
		t.Fatal("cancelled queued job was decoded")
	}
	if f.decodeCount("run") != 1 {
		t.Fatalf("run decoded %d times", f.decodeCount("run"))
	}
	if _, ok := hn.svc.Request(paths[1], 128, true); ok {
		t.Fatal("cancelled path must not be ready")
	}
	hn.waitUntil(func() bool { return f.decodeCount("queued") == 1 })
}

func TestRecentlyRequestedJobsSurviveSetVisible(t *testing.T) {
	reg, f := fakeRegistry()
	hn := newHarness(t, reg, Options{Workers: 1})
	gate := f.gate("block")
	paths := hn.writeFake("block", "prefetch", "stale")
	hn.svc.Request(paths[0], 128, true)
	f.waitStarted(t, "block")
	hn.svc.Request(paths[2], 128, false)
	hn.clock.advance(1500 * time.Millisecond)
	hn.svc.Request(paths[1], 128, false)
	hn.clock.advance(700 * time.Millisecond)
	hn.svc.SetVisible([]string{paths[0]})
	close(gate)
	hn.waitUntil(func() bool { return f.decodeCount("prefetch") == 1 })
	hn.waitUntil(func() bool { _, ok := hn.svc.Request(paths[1], 128, false); return ok })
	if f.decodeCount("stale") != 0 {
		t.Fatal("job older than two seconds should have been cancelled")
	}
}

func TestCancelledJobDoesNotNotify(t *testing.T) {
	reg, f := fakeRegistry()
	hn := newHarness(t, reg, Options{Workers: 1})
	f.gate("run")
	cancelled := f.watchCancel("run")
	p := hn.writeFake("run")[0]
	var notified atomic.Int32
	hn.svc.Subscribe(func(string) { notified.Add(1) })
	hn.svc.Request(p, 128, true)
	f.waitStarted(t, "run")
	hn.clock.advance(5 * time.Second)
	hn.svc.SetVisible(nil)
	<-cancelled
	time.Sleep(20 * time.Millisecond)
	hn.ui.drain()
	if notified.Load() != 0 {
		t.Fatal("cancelled job notified subscribers")
	}
	if _, ok := hn.svc.Request(p, 128, false); ok {
		t.Fatal("cancelled job must not leave a cached result")
	}
}

func TestFailureIsCachedUntilTheFileChanges(t *testing.T) {
	reg, cj := jpegRegistry()
	hn := newHarness(t, reg, Options{})
	p := hn.write("bad.jpg", append([]byte{0xff, 0xd8, 0xff, 0xe0}, make([]byte, 64)...))
	var notified atomic.Int32
	hn.svc.Subscribe(func(string) { notified.Add(1) })
	th := hn.get(p, 128)
	if th.Err == nil || th.Img != nil {
		t.Fatalf("expected a failure thumbnail, got %+v", th)
	}
	if !errors.Is(th.Err, codec.ErrCorrupt) {
		t.Fatalf("error %v does not wrap ErrCorrupt", th.Err)
	}
	for i := 0; i < 50; i++ {
		again, ok := hn.svc.Request(p, 128, true)
		if !ok || again.Err == nil {
			t.Fatalf("failure not served from the cache: ok=%v %+v", ok, again)
		}
	}
	hn.ui.drain()
	if notified.Load() != 1 {
		t.Fatalf("notified %d times, want 1", notified.Load())
	}
	if cj.probes.Load() != 1 {
		t.Fatalf("probed %d times, the corrupt file must not be retried", cj.probes.Load())
	}
	good := jpegBytes(t, solid(200, 100, colGreen), 90)
	if err := os.WriteFile(p, good, 0o600); err != nil {
		t.Fatal(err)
	}
	hn.touch(p, time.Hour)
	th = hn.get(p, 128)
	if th.Err != nil || th.Img == nil {
		t.Fatalf("repaired file still fails: %+v", th)
	}
	if got := pixelAt(th.Img, 64, 32); !colorNear(got, colGreen, 12) {
		t.Fatalf("pixel %v", got)
	}
}

func TestMissingFileAndDirectory(t *testing.T) {
	reg, _ := fakeRegistry()
	hn := newHarness(t, reg, Options{})
	th, ok := hn.svc.Request(filepath.Join(hn.dir, "nope.fake"), 128, true)
	if !ok || !errors.Is(th.Err, os.ErrNotExist) {
		t.Fatalf("missing file: ok=%v err=%v", ok, th.Err)
	}
	th, ok = hn.svc.Request(hn.dir, 128, true)
	if !ok || !errors.Is(th.Err, ErrNotRegular) {
		t.Fatalf("directory: ok=%v err=%v", ok, th.Err)
	}
}

func TestInvalidSide(t *testing.T) {
	reg, _ := fakeRegistry()
	hn := newHarness(t, reg, Options{})
	p := hn.writeFake("x")[0]
	for _, side := range []int{0, -5, maxSide + 1} {
		th, ok := hn.svc.Request(p, side, true)
		if !ok || !errors.Is(th.Err, ErrBadSide) {
			t.Fatalf("side %d: ok=%v err=%v", side, ok, th.Err)
		}
	}
}

func TestUnknownFormatIsAFailure(t *testing.T) {
	reg, _ := fakeRegistry()
	hn := newHarness(t, reg, Options{})
	p := hn.write("junk.fake", []byte("this is not an image at all"))
	th := hn.get(p, 128)
	if !errors.Is(th.Err, codec.ErrUnknownFormat) {
		t.Fatalf("err = %v", th.Err)
	}
}

func TestStatIsCachedForTwoSeconds(t *testing.T) {
	reg, f := fakeRegistry()
	hn := newHarness(t, reg, Options{})
	p := hn.writeFake("s")[0]
	hn.get(p, 128)
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	hn.clock.advance(1900 * time.Millisecond)
	th, ok := hn.svc.Request(p, 128, true)
	if !ok || th.Err != nil || th.Img == nil {
		t.Fatalf("within the stat window the cached thumbnail must be served: ok=%v %+v", ok, th)
	}
	hn.clock.advance(200 * time.Millisecond)
	th, ok = hn.svc.Request(p, 128, true)
	if !ok || !errors.Is(th.Err, os.ErrNotExist) {
		t.Fatalf("after the window the missing file must be noticed: ok=%v %+v", ok, th)
	}
	if f.decodeCount("s") != 1 {
		t.Fatal("extra decode")
	}
}

func TestChangedFileIsRegenerated(t *testing.T) {
	reg, f := fakeRegistry()
	hn := newHarness(t, reg, Options{})
	p := hn.writeFake("c")[0]
	first := hn.get(p, 128)
	hn.touch(p, time.Minute)
	second := hn.get(p, 128)
	if f.decodeCount("c") != 2 {
		t.Fatalf("decodes = %d, a new mtime must trigger regeneration", f.decodeCount("c"))
	}
	if first.Img == second.Img {
		t.Fatal("same image object served for a changed file")
	}
}

func TestStaleJobForChangedFileIsReplaced(t *testing.T) {
	reg, f := fakeRegistry()
	hn := newHarness(t, reg, Options{Workers: 1})
	f.gate("block")
	cancelled := f.watchCancel("block")
	paths := hn.writeFake("block")
	hn.svc.Request(paths[0], 128, true)
	f.waitStarted(t, "block")
	hn.touch(paths[0], time.Minute)
	f.mu.Lock()
	delete(f.gates, "block")
	f.mu.Unlock()
	hn.svc.Request(paths[0], 128, true)
	select {
	case <-cancelled:
	case <-time.After(10 * time.Second):
		t.Fatal("job for the old mtime was not cancelled")
	}
	hn.waitUntil(func() bool { _, ok := hn.svc.Request(paths[0], 128, true); return ok })
}

func TestMemoryCacheEvictsLeastRecentlyUsed(t *testing.T) {
	reg, f := fakeRegistry()
	hn := newHarness(t, reg, Options{MemoryEntries: 3})
	paths := hn.writeFake("a", "b", "c", "d")
	for _, p := range paths[:3] {
		hn.get(p, 128)
	}
	hn.get(paths[0], 128)
	hn.get(paths[3], 128)
	if _, ok := hn.svc.Request(paths[0], 128, true); !ok {
		t.Fatal("recently used entry was evicted")
	}
	if _, ok := hn.svc.Request(paths[3], 128, true); !ok {
		t.Fatal("newest entry missing")
	}
	if _, ok := hn.svc.Request(paths[1], 128, false); ok {
		t.Fatal("least recently used entry survived")
	}
	hn.waitUntil(func() bool { return f.decodeCount("b") == 2 })
	hn.waitUntil(func() bool { _, ok := hn.svc.Request(paths[1], 128, false); return ok })
}

func TestLRU(t *testing.T) {
	c := newLRU(2)
	k := func(i int) ckey { return ckey{path: "p", size: int64(i)} }
	c.put(k(1), Thumb{SrcW: 1})
	c.put(k(2), Thumb{SrcW: 2})
	if v, ok := c.get(k(1)); !ok || v.SrcW != 1 {
		t.Fatal("get 1")
	}
	c.put(k(3), Thumb{SrcW: 3})
	if _, ok := c.get(k(2)); ok {
		t.Fatal("2 should have been evicted")
	}
	c.put(k(1), Thumb{SrcW: 10})
	if v, _ := c.get(k(1)); v.SrcW != 10 {
		t.Fatal("update in place")
	}
	if c.len() != 2 {
		t.Fatalf("len = %d", c.len())
	}
}

func TestLimitsProduceFailures(t *testing.T) {
	reg, _ := fakeRegistry()
	small := codec.Limits{MaxPixels: 1000, MaxFileBytes: 1 << 20}
	hn := newHarness(t, reg, Options{Limits: func() codec.Limits { return small }})
	p := hn.writeFake("huge")[0]
	th := hn.get(p, 128)
	if !errors.Is(th.Err, codec.ErrTooLarge) {
		t.Fatalf("pixel limit: err = %v", th.Err)
	}
	if th.SrcW != 600 || th.SrcH != 400 {
		t.Fatalf("failure should still report the source size, got %dx%d", th.SrcW, th.SrcH)
	}

	reg2, f2 := fakeRegistry()
	tiny := codec.Limits{MaxFileBytes: 8}
	hn2 := newHarness(t, reg2, Options{Limits: func() codec.Limits { return tiny }})
	p2 := hn2.writeFake("bigfile")[0]
	th = hn2.get(p2, 128)
	if !errors.Is(th.Err, codec.ErrTooLarge) {
		t.Fatalf("file limit: err = %v", th.Err)
	}
	if f2.probes["bigfile"] != 0 || f2.decodeCount("bigfile") != 0 {
		t.Fatal("oversized file must be rejected before any codec runs")
	}
}

func TestPanickingDecoderBecomesFailure(t *testing.T) {
	reg, f := fakeRegistry()
	f.panics["boom"] = true
	hn := newHarness(t, reg, Options{Workers: 1})
	paths := hn.writeFake("boom", "fine")
	th := hn.get(paths[0], 128)
	if th.Err == nil || th.Img != nil {
		t.Fatalf("panic must become a failure thumbnail: %+v", th)
	}
	var pe *codec.PanicError
	if !errors.As(th.Err, &pe) {
		t.Fatalf("error %v is not a codec.PanicError", th.Err)
	}
	if ok := hn.get(paths[1], 128); ok.Err != nil {
		t.Fatalf("service did not survive the panic: %v", ok.Err)
	}
}

func TestTimeBudgetFreesTheWorker(t *testing.T) {
	reg, f := fakeRegistry()
	release := f.hangOn("stuck")
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	hn := newHarness(t, reg, Options{Workers: 1, budget: 60 * time.Millisecond})
	paths := hn.writeFake("stuck", "next")
	hn.svc.Request(paths[0], 128, true)
	hn.svc.Request(paths[1], 128, false)
	start := time.Now()
	hn.waitUntil(func() bool { _, ok := hn.svc.Request(paths[0], 128, true); return ok })
	th, _ := hn.svc.Request(paths[0], 128, true)
	if !errors.Is(th.Err, ErrTimeout) {
		t.Fatalf("err = %v, want ErrTimeout", th.Err)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatal("budget was not enforced")
	}
	hn.waitUntil(func() bool { _, ok := hn.svc.Request(paths[1], 128, false); return ok })
	if th, _ := hn.svc.Request(paths[1], 128, false); th.Err != nil {
		t.Fatalf("job queued behind the stuck one failed: %v", th.Err)
	}
	close(release)
	hn.svc.Close()
}

func TestHonouredScaleHint(t *testing.T) {
	reg, f := fakeRegistry()
	f.honorScale = true
	hn := newHarness(t, reg, Options{})
	p := hn.writeFake("scaled")[0]
	th := hn.get(p, 64)
	if th.Err != nil {
		t.Fatal(th.Err)
	}
	if o := f.opts["scaled"]; o.Scale != 8 {
		t.Fatalf("Scale hint = %d, want 8", o.Scale)
	}
	if th.Img.Bounds().Dx() != 64 || th.Img.Bounds().Dy() != 43 {
		t.Fatalf("size %v, want 64x43", th.Img.Bounds())
	}
	if th.SrcW != 600 || th.SrcH != 400 {
		t.Fatalf("source %dx%d", th.SrcW, th.SrcH)
	}
}

func TestIgnoredScaleHint(t *testing.T) {
	reg, f := fakeRegistry()
	hn := newHarness(t, reg, Options{})
	p := hn.writeFake("full")[0]
	th := hn.get(p, 64)
	if th.Err != nil {
		t.Fatal(th.Err)
	}
	if o := f.opts["full"]; o.Scale != 8 {
		t.Fatalf("Scale hint = %d, want 8", o.Scale)
	}
	if th.Img.Bounds().Dx() != 64 || th.Img.Bounds().Dy() != 43 {
		t.Fatalf("size %v, want 64x43", th.Img.Bounds())
	}
}

func TestNoScaleHintWhenSourceIsSmall(t *testing.T) {
	reg, f := fakeRegistry()
	hn := newHarness(t, reg, Options{})
	p := hn.writeFake("small")[0]
	hn.get(p, 384)
	if o := f.opts["small"]; o.Scale > 1 {
		t.Fatalf("Scale hint = %d, want none", o.Scale)
	}
}

func TestSmallSourceIsNotUpscaled(t *testing.T) {
	reg, f := fakeRegistry()
	f.width, f.height = 40, 30
	hn := newHarness(t, reg, Options{})
	p := hn.writeFake("tiny")[0]
	th := hn.get(p, 256)
	if th.Err != nil {
		t.Fatal(th.Err)
	}
	if th.Img.Bounds().Dx() != 40 || th.Img.Bounds().Dy() != 30 {
		t.Fatalf("size %v, want 40x30", th.Img.Bounds())
	}
	if got := pixelAt(th.Img, 5, 5); got.A != 255 {
		t.Fatalf("pixel %v", got)
	}
}

func TestDecodedOrientationIsApplied(t *testing.T) {
	reg, f := fakeRegistry()
	f.orient = codec.OrientRotate90
	hn := newHarness(t, reg, Options{})
	p := hn.writeFake("rot")[0]
	th := hn.get(p, 128)
	if th.Err != nil {
		t.Fatal(th.Err)
	}
	if th.Img.Bounds().Dx() != 85 || th.Img.Bounds().Dy() != 128 {
		t.Fatalf("size %v, want 85x128", th.Img.Bounds())
	}
	if th.Orientation != codec.OrientRotate90 {
		t.Fatalf("orientation %d", th.Orientation)
	}
	if th.SrcW != 600 || th.SrcH != 400 {
		t.Fatalf("source %dx%d must be reported as stored", th.SrcW, th.SrcH)
	}
}

func TestAlphaSurvivesTheService(t *testing.T) {
	reg, _ := jpegRegistry()
	hn := newHarness(t, reg, Options{})
	src := solid(64, 64, color.NRGBA{R: 255, G: 0, B: 0, A: 100})
	var buf = pngBytes(t, src)
	p := hn.write("a.png", buf)
	th := hn.get(p, 32)
	if th.Err != nil {
		t.Fatal(th.Err)
	}
	got := pixelAt(th.Img, 10, 10)
	if got.A != 100 || got.R != 100 || got.G != 0 || got.B != 0 {
		t.Fatalf("pixel %v, want premultiplied {100 0 0 100}", got)
	}
}

func TestCloseStopsEverything(t *testing.T) {
	reg, f := fakeRegistry()
	before := runtime.NumGoroutine()
	hn := newHarness(t, reg, Options{Workers: 2, DiskDir: t.TempDir()})
	f.gate("g1")
	f.gate("g2")
	paths := hn.writeFake("g1", "g2", "q1", "q2", "ok")
	hn.get(paths[4], 128)
	for _, p := range paths[:4] {
		hn.svc.Request(p, 128, true)
	}
	f.waitStarted(t, "g1")
	f.waitStarted(t, "g2")
	hn.svc.Close()
	hn.svc.Close()
	if _, ok := hn.svc.Request(paths[4], 128, true); ok {
		t.Fatal("closed service must not serve")
	}
	hn.svc.SetVisible(nil)
	hn.svc.Subscribe(func(string) {})()
	deadline := time.Now().Add(5 * time.Second)
	var after int
	for {
		after = runtime.NumGoroutine()
		if after <= before || time.Now().After(deadline) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if after > before {
		buf := make([]byte, 1<<16)
		n := runtime.Stack(buf, true)
		t.Fatalf("goroutines: %d before, %d after Close\n%s", before, after, buf[:n])
	}
	if f.decodeCount("q1") != 0 || f.decodeCount("q2") != 0 {
		t.Fatal("queued jobs ran after Close")
	}
}

func TestPostAfterCloseIsHarmless(t *testing.T) {
	reg, f := fakeRegistry()
	hn := newHarness(t, reg, Options{Workers: 1})
	p := hn.writeFake("late")[0]
	var notified atomic.Int32
	hn.svc.Subscribe(func(string) { notified.Add(1) })
	hn.svc.Request(p, 128, true)
	hn.waitUntil(func() bool { return f.decodeCount("late") == 1 })
	hn.svc.Close()
	hn.ui.drain()
	if notified.Load() > 1 {
		t.Fatalf("notified %d times", notified.Load())
	}
}

func TestNilPostRunsInline(t *testing.T) {
	reg, _ := fakeRegistry()
	dir := t.TempDir()
	p := filepath.Join(dir, "n.fake")
	if err := os.WriteFile(p, []byte("FAKE n"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := New(reg, nil, Options{Workers: 1})
	defer s.Close()
	done := make(chan struct{}, 1)
	s.Subscribe(func(string) { done <- struct{}{} })
	s.Request(p, 128, true)
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("no notification")
	}
	if _, ok := s.Request(p, 128, true); !ok {
		t.Fatal("result missing")
	}
}

func TestDefaults(t *testing.T) {
	reg, _ := fakeRegistry()
	s := New(reg, nil, Options{})
	defer s.Close()
	if s.opt.Workers != max(1, runtime.NumCPU()/4) {
		t.Fatalf("workers = %d", s.opt.Workers)
	}
	if s.opt.MemoryEntries != 512 || s.opt.DiskLimitBytes != 256<<20 {
		t.Fatalf("defaults %+v", s.opt)
	}
	if s.disk != nil {
		t.Fatal("a zero Options value must not touch the disk")
	}
	if s.DiskUsage() != 0 || s.ClearDisk() != nil {
		t.Fatal("disabled disk cache must be a no-op")
	}
	d := DefaultOptions()
	if d.DiskDir == "" || filepath.Base(d.DiskDir) != "thumbs" || d.Workers < 1 || d.MemoryEntries != 512 || d.DiskLimitBytes != 256<<20 {
		t.Fatalf("DefaultOptions() = %+v", d)
	}
}

func TestGuardedRecoversPanics(t *testing.T) {
	reg, _ := fakeRegistry()
	s := New(reg, nil, Options{Workers: 1})
	defer s.Close()
	_, err := guarded(s, context.Background(), func() (int, error) { panic("x") })
	if err == nil {
		t.Fatal("panic must become an error")
	}
	v, err := guarded(s, context.Background(), func() (int, error) { return 7, nil })
	if err != nil || v != 7 {
		t.Fatalf("v=%d err=%v", v, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := guarded(s, ctx, func() (int, error) { return 1, nil }); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
}

func TestGuardedAbandonsAndAccounts(t *testing.T) {
	reg, _ := fakeRegistry()
	s := New(reg, nil, Options{Workers: 1})
	defer s.Close()
	release := make(chan struct{})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err := guarded(s, ctx, func() (int, error) { <-release; return 1, nil })
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v", err)
	}
	if s.abandoned.Load() != 1 {
		t.Fatalf("abandoned = %d", s.abandoned.Load())
	}
	close(release)
	deadline := time.Now().Add(5 * time.Second)
	for s.abandoned.Load() != 0 {
		if time.Now().After(deadline) {
			t.Fatal("abandoned counter never returned to zero")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestPendingCountsQueuedRequestsUntilTheyFinish(t *testing.T) {
	reg, _ := fakeRegistry()
	hn := newHarness(t, reg, Options{})
	p := hn.writeFake("one")[0]
	done := false
	hn.svc.Subscribe(func(string) { done = true })

	hn.svc.Request(p, 128, true)
	hn.waitUntil(func() bool { return done })

	if n := hn.svc.Pending(); n != 0 {
		t.Fatalf("pending after delivery = %d", n)
	}
}

func TestDirectoryHelpersMeasureAndClearTheCacheOfAnotherRun(t *testing.T) {
	reg, _ := fakeRegistry()
	dir := t.TempDir()
	hn := newHarness(t, reg, Options{DiskDir: dir})
	p := hn.writeFake("one")[0]
	hn.get(p, 128)
	hn.waitUntil(func() bool { return DirectoryUsage(dir) > 0 })

	if err := ClearDirectory(dir); err != nil {
		t.Fatal(err)
	}

	if n := DirectoryUsage(dir); n != 0 {
		t.Fatalf("usage after clearing = %d", n)
	}
	if DirectoryUsage("") != 0 || ClearDirectory("") != nil {
		t.Fatal("an empty directory name must be harmless")
	}
}
