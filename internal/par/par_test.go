package par

import (
	"runtime"
	"sync/atomic"
	"testing"
)

func reset(t *testing.T) {
	t.Helper()
	Set(0)
	t.Cleanup(func() { Set(0) })
}

func TestCountIsAutomaticByDefault(t *testing.T) {
	reset(t)
	want := max(1, min(runtime.GOMAXPROCS(0), runtime.NumCPU()))
	if got := Count(); got != min(want, MaxThreads) {
		t.Fatalf("Count() = %d, want %d", got, want)
	}
	if Configured() != 0 {
		t.Fatalf("Configured() = %d, want 0", Configured())
	}
}

func TestSetOverridesAndClamps(t *testing.T) {
	reset(t)
	Set(3)
	if Count() != 3 || Configured() != 3 {
		t.Fatalf("Count() = %d, Configured() = %d after Set(3)", Count(), Configured())
	}
	Set(1000)
	if Count() != MaxThreads {
		t.Fatalf("Count() = %d after Set(1000), want %d", Count(), MaxThreads)
	}
	Set(-5)
	if Configured() != 0 || Count() != Auto() {
		t.Fatalf("negative value must mean automatic, got Configured() = %d, Count() = %d", Configured(), Count())
	}
}

func TestAutoFollowsGOMAXPROCS(t *testing.T) {
	reset(t)
	prev := runtime.GOMAXPROCS(1)
	defer runtime.GOMAXPROCS(prev)
	if Auto() != 1 || Count() != 1 {
		t.Fatalf("Auto() = %d, Count() = %d with GOMAXPROCS(1)", Auto(), Count())
	}
}

func TestShare(t *testing.T) {
	reset(t)
	Set(8)
	cases := []struct{ divisor, want int }{{1, 8}, {2, 4}, {3, 2}, {16, 1}, {0, 8}, {-1, 8}}
	for _, c := range cases {
		if got := Share(c.divisor); got != c.want {
			t.Errorf("Share(%d) = %d, want %d", c.divisor, got, c.want)
		}
	}
}

func TestParallelRunsEveryTaskOnce(t *testing.T) {
	for _, workers := range []int{0, 1, 2, 7, 64} {
		for _, tasks := range []int{0, 1, 5, 100} {
			seen := make([]atomic.Int32, tasks)
			Parallel(tasks, workers, func(i int) { seen[i].Add(1) })
			for i := range seen {
				if seen[i].Load() != 1 {
					t.Fatalf("workers %d tasks %d: task %d ran %d times", workers, tasks, i, seen[i].Load())
				}
			}
		}
	}
}

func TestParallelSingleWorkerIsOrdered(t *testing.T) {
	var order []int
	Parallel(5, 1, func(i int) { order = append(order, i) })
	for i, v := range order {
		if v != i {
			t.Fatalf("serial order broken: %v", order)
		}
	}
}

func BenchmarkParallelOverhead(b *testing.B) {
	var sink atomic.Int64
	for i := 0; i < b.N; i++ {
		Parallel(64, 8, func(t int) { sink.Add(int64(t)) })
	}
}
