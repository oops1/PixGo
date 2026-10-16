package thumbs

import (
	"runtime"
	"testing"
	"time"

	"github.com/oops1/PixGo/internal/par"
)

func settledCount(limit int) int {
	n := runtime.NumGoroutine()
	for i := 0; i < 300 && n > limit; i++ {
		time.Sleep(10 * time.Millisecond)
		n = runtime.NumGoroutine()
	}
	return n
}

func TestDefaultWorkersFollowTheThreadBudget(t *testing.T) {
	t.Cleanup(func() { par.Set(0) })
	cases := []struct{ threads, want int }{{1, 1}, {3, 1}, {8, 2}, {20, 5}, {64, 16}}
	for _, c := range cases {
		par.Set(c.threads)
		if got := defaultWorkers(); got != c.want {
			t.Errorf("%d threads: %d workers, want %d", c.threads, got, c.want)
		}
	}
}

func TestSetWorkersResizesTheLivePool(t *testing.T) {
	t.Cleanup(func() { par.Set(0) })
	reg, _ := fakeRegistry()
	base := runtime.NumGoroutine()
	hn := newHarness(t, reg, Options{Workers: 2})
	if hn.svc.Workers() != 2 {
		t.Fatalf("workers %d", hn.svc.Workers())
	}
	hn.svc.SetWorkers(7)
	if hn.svc.Workers() != 7 {
		t.Fatalf("workers %d after growing", hn.svc.Workers())
	}
	if n := runtime.NumGoroutine(); n < base+7 {
		t.Fatalf("goroutines %d, expected at least %d", n, base+7)
	}
	hn.svc.SetWorkers(1)
	if hn.svc.Workers() != 1 {
		t.Fatalf("workers %d after shrinking", hn.svc.Workers())
	}
	if n := settledCount(base + 2); n > base+2 {
		t.Fatalf("pool did not shrink: %d goroutines, base %d", n, base)
	}
	par.Set(8)
	hn.svc.SetWorkers(0)
	if hn.svc.Workers() != 2 {
		t.Fatalf("automatic count for 8 threads is %d, want 2", hn.svc.Workers())
	}
}

func TestShrunkPoolStillServesRequests(t *testing.T) {
	reg, _ := fakeRegistry()
	hn := newHarness(t, reg, Options{Workers: 4})
	hn.svc.SetWorkers(1)
	p := hn.writeFake("after-shrink")[0]
	if th := hn.get(p, 128); th.Err != nil || th.Img == nil {
		t.Fatalf("thumbnail %+v", th)
	}
}

func TestSetWorkersAfterCloseDoesNothing(t *testing.T) {
	reg, _ := fakeRegistry()
	hn := newHarness(t, reg, Options{Workers: 1})
	hn.svc.Close()
	base := runtime.NumGoroutine()
	hn.svc.SetWorkers(9)
	if n := runtime.NumGoroutine(); n > base {
		t.Fatalf("SetWorkers after Close started %d goroutines", n-base)
	}
}

func TestAbandonedLimitScalesWithWorkers(t *testing.T) {
	reg, _ := fakeRegistry()
	hn := newHarness(t, reg, Options{Workers: 1})
	if got := hn.svc.maxAbandoned.Load(); got != 2 {
		t.Fatalf("limit %d for one worker", got)
	}
	hn.svc.SetWorkers(6)
	if got := hn.svc.maxAbandoned.Load(); got != 12 {
		t.Fatalf("limit %d for six workers", got)
	}
}
