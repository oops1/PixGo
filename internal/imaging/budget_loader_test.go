package imaging

import (
	"image"
	"runtime"
	"testing"
	"time"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/par"
)

func fakeEstimate() int64 {
	return directEstimate(codec.Info{Format: "fake", Width: 600, Height: 400}, 0, 0)
}

func TestDirectEstimateFormula(t *testing.T) {
	jpeg := codec.Info{Format: codec.JPEG, Width: 6000, Height: 4000}
	px := int64(24_000_000)
	want := px*4 + px*4/3 + px*scratchFor(codec.JPEG)
	if got := directEstimate(jpeg, 0, 0); got != want {
		t.Fatalf("jpeg estimate %d, want %d", got, want)
	}
	anim := codec.Info{Format: codec.GIF, Width: 100, Height: 100, Animated: true, Frames: 10}
	if got := directEstimate(anim, 0, 0); got < 10*100*100*4 {
		t.Fatalf("an animation holds every frame, got %d", got)
	}
	vector := codec.Info{Format: codec.SVG, Width: 10, Height: 10, Vector: true}
	if got := directEstimate(vector, 2000, 1000); got < 2000*1000*4 {
		t.Fatalf("a vector picture is estimated at its raster size, got %d", got)
	}
	if got := directEstimate(codec.Info{Format: codec.PNG}, 0, 0); got != 0 {
		t.Fatalf("empty picture estimate %d", got)
	}
	huge := codec.Info{Format: codec.GIF, Width: 1000, Height: 1000, Animated: true, Frames: 1 << 30}
	if got := directEstimate(huge, 0, 0); got <= 0 || got > 1<<50 {
		t.Fatalf("frame count must be bounded, got %d", got)
	}
	if scratchFor("unheard-of") != defaultScratchBpp {
		t.Fatal("unknown formats use the conservative default")
	}
}

func TestTiledEstimateCountsOverviewTilesAndWindow(t *testing.T) {
	l := &Loader{}
	l.tileBudget.Store(100 << 20)
	got := l.tiledEstimate(codec.Info{Width: 16384, Height: 12288})
	overview := overviewBytes(16384, 12288)
	if got != overview+100<<20+windowReserveBytes {
		t.Fatalf("estimate %d", got)
	}
	if overview > 100<<20 {
		t.Fatalf("the overview of a huge picture is bounded, got %d", overview)
	}
}

func TestLoaderSkipsPrefetchThatDoesNotFitAndRetriesOnPromotion(t *testing.T) {
	r := newRig(t, 2)
	est := fakeEstimate()
	r.cache.SetBudget(est * 3 / 2)
	a, b := r.file("a"), r.file("b")
	r.loader.Want([]Request{req(a, PrioCurrent)})
	shown := r.waitState(a, StateReady).Img
	r.cache.Pin(shown.Key)
	r.loader.Want([]Request{req(a, PrioCurrent), req(b, PrioNeighbor)})
	r.waitFor("the neighbor to be probed and skipped", func() bool {
		e, ok := r.loader.Entry(b)
		return ok && e.State == StateProbed && r.loader.Pending() == 0
	})
	if r.fake.count("b") != 0 {
		t.Fatal("a prefetch that does not fit must not be decoded")
	}
	r.cache.SetBudget(est * 10)
	r.loader.Want([]Request{req(b, PrioCurrent), req(a, PrioNeighbor)})
	r.waitState(b, StateReady)
	if r.fake.count("b") != 1 {
		t.Fatalf("b decoded %d times", r.fake.count("b"))
	}
}

func TestLoaderSkippedPrefetchIsRequeuedByAnyLaterWant(t *testing.T) {
	r := newRig(t, 2)
	est := fakeEstimate()
	r.cache.SetBudget(est * 3 / 2)
	a, b := r.file("a"), r.file("b")
	r.loader.Want([]Request{req(a, PrioCurrent)})
	r.cache.Pin(r.waitState(a, StateReady).Img.Key)
	reqs := []Request{req(a, PrioCurrent), req(b, PrioNeighbor)}
	r.loader.Want(reqs)
	r.waitFor("skip", func() bool { return r.loader.Pending() == 0 })
	r.cache.SetBudget(est * 10)
	r.loader.Want(reqs)
	r.waitState(b, StateReady)
}

func TestLoaderCurrentWaitsForARunningPrefetchToReleaseTheBudget(t *testing.T) {
	r := newRig(t, 2)
	est := fakeEstimate()
	r.cache.SetBudget(est * 6 / 5)
	n, c := r.file("n"), r.file("c")
	gate := r.fake.gate("n")
	r.loader.Want([]Request{req(n, PrioNeighbor)})
	r.waitStarted(1)
	r.loader.Want([]Request{req(n, PrioNeighbor), req(c, PrioCurrent)})
	r.waitFor("the current job to queue for the budget", func() bool { return r.cache.Waiting() == 1 })
	time.Sleep(20 * time.Millisecond)
	if r.fake.count("c") != 0 {
		t.Fatal("the current picture must wait while the budget is taken")
	}
	close(gate)
	r.waitState(c, StateReady)
	if r.cache.Reserved() != 0 {
		r.waitFor("reservations to be released", func() bool { return r.cache.Reserved() == 0 })
	}
}

func TestLoaderPrefetchWaitsForTheCurrentPicture(t *testing.T) {
	r := newRig(t, 3)
	cur, nb := r.file("cur"), r.file("nb")
	gate := r.fake.gate("cur")
	r.loader.Want([]Request{req(cur, PrioCurrent), req(nb, PrioNeighbor)})
	r.waitStarted(1)
	time.Sleep(40 * time.Millisecond)
	if r.fake.count("nb") != 0 {
		t.Fatal("a neighbor must not start while the current picture is still decoding")
	}
	close(gate)
	r.waitState(cur, StateReady)
	r.waitState(nb, StateReady)
}

func TestLoaderPrefetchStartsRightAwayWhenNothingCurrentIsRunning(t *testing.T) {
	r := newRig(t, 2)
	a := r.file("a")
	r.loader.Want([]Request{req(a, PrioNeighbor)})
	r.waitState(a, StateReady)
}

func TestLoaderTinyBudgetStillCompletesEveryJob(t *testing.T) {
	r := newRig(t, 4)
	r.cache.SetBudget(1)
	var reqs []Request
	var paths []string
	for _, n := range []string{"a", "b", "c", "d", "e", "f"} {
		p := r.file(n)
		paths = append(paths, p)
		reqs = append(reqs, req(p, PrioCurrent))
	}
	r.loader.Want(reqs)
	for _, p := range paths {
		r.waitState(p, StateReady)
	}
	r.waitFor("reservations to drain", func() bool { return r.cache.Reserved() == 0 && r.cache.Waiting() == 0 })
}

func TestLoaderCancelledCurrentJobLeavesNoReservation(t *testing.T) {
	r := newRig(t, 2)
	est := fakeEstimate()
	r.cache.SetBudget(est * 6 / 5)
	n, c, other := r.file("n"), r.file("c"), r.file("other")
	gate := r.fake.gate("n")
	r.loader.Want([]Request{req(n, PrioNeighbor)})
	r.waitStarted(1)
	r.loader.Want([]Request{req(n, PrioNeighbor), req(c, PrioCurrent)})
	r.waitFor("waiter", func() bool { return r.cache.Waiting() == 1 })
	r.loader.Want([]Request{req(other, PrioCurrent)})
	r.waitFor("the abandoned waiter to leave", func() bool { return r.cache.Waiting() <= 1 })
	close(gate)
	r.waitState(other, StateReady)
	r.waitFor("all reservations released", func() bool { return r.cache.Reserved() == 0 && r.cache.Waiting() == 0 })
	if r.fake.count("c") != 0 {
		t.Fatal("a cancelled job must never decode")
	}
}

func TestLoaderHugeCurrentPictureSuppressesPrefetchOfOtherFiles(t *testing.T) {
	r := newBigRig(t, 3, tiledLimits(), true)
	huge := r.file("huge", 20000, 15000)
	small := r.file("small", 800, 600)
	r.loader.Want([]Request{req(huge, PrioCurrent), req(small, PrioNeighbor)})
	d := r.waitState(huge, StateReady).Img
	if d.Detail == nil {
		t.Fatal("expected a tiled picture")
	}
	time.Sleep(60 * time.Millisecond)
	if len(r.big.optionsOf("small")) != 0 {
		t.Fatal("no prefetch is allowed next to a tiled picture")
	}
	r.loader.Want([]Request{req(small, PrioCurrent), req(huge, PrioNeighbor)})
	r.waitState(small, StateReady)
	if len(r.big.optionsOf("small")) == 0 {
		t.Fatal("the picture must decode once it becomes the current one")
	}
}

func TestLoaderPrefetchResumesAfterLeavingTheHugePicture(t *testing.T) {
	r := newBigRig(t, 3, tiledLimits(), true)
	huge := r.file("huge", 20000, 15000)
	a := r.file("a", 800, 600)
	b := r.file("b", 800, 600)
	r.loader.Want([]Request{req(huge, PrioCurrent), req(a, PrioNeighbor)})
	r.waitState(huge, StateReady)
	r.loader.Want([]Request{req(a, PrioCurrent), req(b, PrioNeighbor), req(huge, PrioNeighbor)})
	r.waitState(a, StateReady)
	r.waitState(b, StateReady)
}

func TestLoaderSetWorkersGrowsAndShrinksTheLivePool(t *testing.T) {
	base := runtime.NumGoroutine()
	par.Set(0)
	l := NewLoader(codec.NewRegistry(), NewCache(1<<20), (&uiQueue{}).Post, 2, nil)
	defer l.Close()
	if l.Workers() != 2 {
		t.Fatalf("workers %d", l.Workers())
	}
	l.SetWorkers(6)
	if l.Workers() != 6 {
		t.Fatalf("workers %d after growing", l.Workers())
	}
	if n := runtime.NumGoroutine(); n < base+6 {
		t.Fatalf("goroutines %d, expected at least %d", n, base+6)
	}
	l.SetWorkers(1)
	if n := settledGoroutines(base + 1); n > base+1 {
		t.Fatalf("pool did not shrink: %d goroutines, base %d", n, base)
	}
	if l.Workers() != 1 {
		t.Fatalf("workers %d after shrinking", l.Workers())
	}
}

func TestLoaderAutomaticWorkersFollowTheThreadBudget(t *testing.T) {
	t.Cleanup(func() { par.Set(0) })
	l := NewLoader(codec.NewRegistry(), NewCache(1<<20), (&uiQueue{}).Post, 0, nil)
	defer l.Close()
	par.Set(16)
	l.SetWorkers(0)
	if l.Workers() != 8 {
		t.Fatalf("workers %d for 16 threads", l.Workers())
	}
	par.Set(1)
	l.SetWorkers(0)
	if l.Workers() != MinLoaderWorkers {
		t.Fatalf("workers %d for 1 thread, want the minimum %d", l.Workers(), MinLoaderWorkers)
	}
	l.SetWorkers(5)
	par.Set(32)
	l.SetWorkers(5)
	if l.Workers() != 5 {
		t.Fatalf("an explicit count must win, got %d", l.Workers())
	}
	if got := l.tileWorkerCount(); got != 2 {
		t.Fatalf("tile workers %d for an explicit 5", got)
	}
	l.SetWorkers(0)
	if got := l.tileWorkerCount(); got != 0 {
		t.Fatalf("automatic tile workers follow the thread budget at run time, got %d", got)
	}
}

func TestLoaderSetWorkersAfterCloseIsHarmless(t *testing.T) {
	l := NewLoader(codec.NewRegistry(), NewCache(1<<20), (&uiQueue{}).Post, 2, nil)
	l.Close()
	base := runtime.NumGoroutine()
	l.SetWorkers(8)
	if n := runtime.NumGoroutine(); n > base {
		t.Fatalf("SetWorkers after Close started %d goroutines", n-base)
	}
}

func TestTileManagerWorkersFollowTheThreadBudgetWhenAutomatic(t *testing.T) {
	t.Cleanup(func() { par.Set(0) })
	rd := newFakeReader(picture{w: 4096, h: 4096})
	m := NewTileManager(rd, []*image.NRGBA{image.NewNRGBA(image.Rect(0, 0, 512, 512))}, 3, 4096, 4096, TileOptions{}, nil)
	defer m.Close()
	par.Set(1)
	if got := m.workerLimit(); got != 1 {
		t.Fatalf("limit %d with 1 thread", got)
	}
	par.Set(4)
	if got := m.workerLimit(); got != 2 {
		t.Fatalf("limit %d with 4 threads", got)
	}
	par.Set(64)
	if got := m.workerLimit(); got != MaxTileWorkers {
		t.Fatalf("limit %d with 64 threads", got)
	}
	fixed := NewTileManager(newFakeReader(picture{w: 4096, h: 4096}), []*image.NRGBA{image.NewNRGBA(image.Rect(0, 0, 512, 512))}, 3, 4096, 4096, TileOptions{Workers: 2}, nil)
	defer fixed.Close()
	par.Set(1)
	if got := fixed.workerLimit(); got != 2 {
		t.Fatalf("an explicit worker count must stay, got %d", got)
	}
}
