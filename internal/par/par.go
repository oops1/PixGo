package par

import (
	"runtime"
	"sync"
	"sync/atomic"
)

const MaxThreads = 64

var configured atomic.Int32

func Set(n int) {
	configured.Store(int32(min(max(n, 0), MaxThreads)))
}

func Configured() int { return int(configured.Load()) }

func Auto() int {
	return min(max(1, min(runtime.GOMAXPROCS(0), runtime.NumCPU())), MaxThreads)
}

func Count() int {
	if n := int(configured.Load()); n > 0 {
		return n
	}
	return Auto()
}

func Share(divisor int) int {
	return max(1, Count()/max(divisor, 1))
}

func Parallel(tasks, workers int, fn func(task int)) {
	if workers > tasks {
		workers = tasks
	}
	if workers <= 1 {
		for t := 0; t < tasks; t++ {
			fn(t)
		}
		return
	}
	var next atomic.Int64
	work := func() {
		for {
			t := int(next.Add(1)) - 1
			if t >= tasks {
				return
			}
			fn(t)
		}
	}
	var wg sync.WaitGroup
	wg.Add(workers - 1)
	for w := 1; w < workers; w++ {
		go func() {
			defer wg.Done()
			work()
		}()
	}
	work()
	wg.Wait()
}
