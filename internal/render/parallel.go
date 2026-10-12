package render

import (
	"runtime"
	"sync"
	"sync/atomic"
)

func workerCount() int {
	n := runtime.GOMAXPROCS(0)
	if c := runtime.NumCPU(); c < n {
		n = c
	}
	return max(1, n)
}

func parallel(tasks, workers int, fn func(task int)) {
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
