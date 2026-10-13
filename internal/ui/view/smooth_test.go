package view

import (
	"testing"
	"time"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/viewer"
)

type smoothTimers struct {
	pending []func()
	started int
}

func (s *smoothTimers) after(d time.Duration, fn func()) func() {
	s.started++
	idx := len(s.pending)
	s.pending = append(s.pending, fn)
	return func() { s.pending[idx] = nil }
}

func (s *smoothTimers) fire() bool {
	for i, fn := range s.pending {
		if fn != nil {
			s.pending[i] = nil
			fn()
			return true
		}
	}
	return false
}

func (s *smoothTimers) live() int {
	n := 0
	for _, fn := range s.pending {
		if fn != nil {
			n++
		}
	}
	return n
}

func smoothRig(t *testing.T, smooth bool) (*rig, *smoothTimers) {
	t.Helper()
	r := newRig(t)
	ts := &smoothTimers{}
	r.c.Smooth = smooth
	r.c.After = ts.after
	r.c.SetImage(source(1600, 1200), codec.OrientNormal, viewer.ZoomFit)
	r.frame()
	return r, ts
}

func runAll(ts *smoothTimers) {
	for ts.fire() {
	}
}

func TestSmoothZoomMovesInStepsAndEndsOnTheSameZoomAsAnInstantStep(t *testing.T) {
	instant, _ := smoothRig(t, false)
	instant.c.ZoomIn()
	want := instant.c.ZoomPercent()
	r, ts := smoothRig(t, true)
	start := r.c.ZoomPercent()

	r.c.ZoomIn()

	if r.c.ZoomPercent() != start {
		t.Fatalf("zoom jumped to %d%% before the first frame", r.c.ZoomPercent())
	}
	seen := map[int]bool{}
	for ts.fire() {
		seen[r.c.ZoomPercent()] = true
	}
	if r.c.ZoomPercent() != want {
		t.Fatalf("smooth zoom ended on %d%%, an instant step gives %d%%", r.c.ZoomPercent(), want)
	}
	if len(seen) < 3 {
		t.Fatalf("zoom took %d distinct values on the way: %v", len(seen), seen)
	}
}

func TestSmoothZoomKeepsOnlyOneTimerPending(t *testing.T) {
	r, ts := smoothRig(t, true)

	r.c.ZoomIn()
	r.c.ZoomIn()
	r.c.ZoomIn()

	if ts.live() != 1 {
		t.Fatalf("%d timers pending, want exactly one", ts.live())
	}
	runAll(ts)
	if ts.live() != 0 {
		t.Fatalf("%d timers left after the animation", ts.live())
	}
}

func TestSuccessiveSmoothStepsAddUp(t *testing.T) {
	instant, _ := smoothRig(t, false)
	instant.c.ZoomIn()
	instant.c.ZoomIn()
	want := instant.c.ZoomPercent()
	r, ts := smoothRig(t, true)

	r.c.ZoomIn()
	ts.fire()
	r.c.ZoomIn()
	runAll(ts)

	if r.c.ZoomPercent() != want {
		t.Fatalf("two quick steps ended on %d%%, want %d%%", r.c.ZoomPercent(), want)
	}
}

func TestSmoothZoomKeepsThePointUnderTheCursor(t *testing.T) {
	r, ts := smoothRig(t, true)
	u0, v0 := r.c.View().AreaToImage(r.c.Frame(), 300.5, 100.5)

	r.c.stepZoom(2, true, 300.5, 100.5)
	runAll(ts)

	u1, v1 := r.c.View().AreaToImage(r.c.Frame(), 300.5, 100.5)
	if d := u1 - u0; d > 0.5 || d < -0.5 {
		t.Errorf("u moved %.2f -> %.2f", u0, u1)
	}
	if d := v1 - v0; d > 0.5 || d < -0.5 {
		t.Errorf("v moved %.2f -> %.2f", v0, v1)
	}
}

func TestAnotherChangeSettlesTheAnimationFirst(t *testing.T) {
	instant, _ := smoothRig(t, false)
	instant.c.ZoomIn()
	want := instant.c.ZoomPercent()
	r, ts := smoothRig(t, true)

	r.c.ZoomIn()
	ts.fire()
	r.c.RotateRight()

	if r.c.ZoomPercent() != want {
		t.Fatalf("rotating mid-animation left zoom at %d%%, want %d%%", r.c.ZoomPercent(), want)
	}
	if ts.live() != 0 {
		t.Fatalf("%d timers pending after settling", ts.live())
	}
}

func TestNewImageDropsTheAnimation(t *testing.T) {
	r, ts := smoothRig(t, true)
	r.c.ZoomIn()

	r.c.SetImage(source(800, 600), codec.OrientNormal, viewer.ZoomFit)
	runAll(ts)

	if r.c.Mode() != viewer.ZoomFit {
		t.Fatalf("a stale animation changed the mode of the new image: %v", r.c.Mode())
	}
	r.c.Clear()
	r.c.ZoomIn()
	if ts.live() != 0 {
		t.Fatal("an empty canvas must not animate")
	}
}

func TestSmoothZoomNeedsATimerSource(t *testing.T) {
	r, ts := smoothRig(t, true)
	r.c.After = nil
	start := r.c.ZoomPercent()

	r.c.ZoomIn()

	if r.c.ZoomPercent() == start || ts.started != 0 {
		t.Fatalf("without timers the step must be immediate: %d%% -> %d%%", start, r.c.ZoomPercent())
	}
}

func TestSmoothZoomAtTheLimitDoesNothing(t *testing.T) {
	r, ts := smoothRig(t, true)
	for i := 0; i < 80; i++ {
		r.c.ZoomOut()
		runAll(ts)
	}
	min := r.c.ZoomPercent()
	r.c.ZoomOut()

	if ts.live() != 0 || r.c.ZoomPercent() != min {
		t.Fatalf("at the lowest zoom: %d timers, zoom %d%% -> %d%%", ts.live(), min, r.c.ZoomPercent())
	}
}
