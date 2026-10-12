package jobs

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type uiQueue struct {
	mu    sync.Mutex
	items []func()
}

func (q *uiQueue) Post(fn func()) {
	q.mu.Lock()
	q.items = append(q.items, fn)
	q.mu.Unlock()
}

func (q *uiQueue) size() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items)
}

func (q *uiQueue) take() func() {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) == 0 {
		return nil
	}
	fn := q.items[0]
	q.items = q.items[1:]
	return fn
}

func (q *uiQueue) runAll() int {
	n := 0
	for fn := q.take(); fn != nil; fn = q.take() {
		fn()
		n++
	}
	return n
}

func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

func checkLeaks(t *testing.T) {
	t.Helper()
	before := runtime.NumGoroutine()
	t.Cleanup(func() {
		deadline := time.Now().Add(3 * time.Second)
		for runtime.NumGoroutine() > before && time.Now().Before(deadline) {
			time.Sleep(5 * time.Millisecond)
		}
		if now := runtime.NumGoroutine(); now > before {
			buf := make([]byte, 1<<16)
			buf = buf[:runtime.Stack(buf, true)]
			t.Errorf("goroutine leak: %d before, %d after\n%s", before, now, buf)
		}
	})
}

func blockUntilCancelled(ctx context.Context) (any, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestGoDeliversResultThroughPost(t *testing.T) {
	checkLeaks(t)
	q := &uiQueue{}
	r := NewRunner(q.Post)
	defer r.Close()
	var got any
	var gotErr error
	r.Go("decode:a", 0, func(ctx context.Context) (any, error) { return 42, nil }, func(v any, err error) {
		got, gotErr = v, err
	})
	waitUntil(t, "posted result", func() bool { return q.size() == 1 })
	if got != nil {
		t.Fatal("done ran before the post queue was drained")
	}
	q.runAll()
	if got != 42 || gotErr != nil {
		t.Fatalf("got %v, %v", got, gotErr)
	}
	if r.Pending() != 0 {
		t.Fatalf("pending = %d", r.Pending())
	}
}

func TestGoDeliversError(t *testing.T) {
	checkLeaks(t)
	r := NewRunner(nil)
	defer r.Close()
	want := errors.New("boom")
	res := make(chan error, 1)
	r.Go("k", 0, func(ctx context.Context) (any, error) { return nil, want }, func(v any, err error) { res <- err })
	select {
	case err := <-res:
		if err != want {
			t.Fatalf("got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no result")
	}
}

func TestNilPostRunsDoneDirectly(t *testing.T) {
	checkLeaks(t)
	r := NewRunner(nil)
	defer r.Close()
	res := make(chan int, 1)
	r.Go("k", 0, func(ctx context.Context) (any, error) { return 7, nil }, func(v any, err error) { res <- v.(int) })
	select {
	case v := <-res:
		if v != 7 {
			t.Fatalf("got %d", v)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no result")
	}
}

func TestSameKeyCancelsPrevious(t *testing.T) {
	checkLeaks(t)
	q := &uiQueue{}
	r := NewRunner(q.Post)
	defer r.Close()
	firstStarted := make(chan struct{})
	firstCancelled := make(chan struct{})
	var firstDone atomic.Bool
	r.Go("decode:x", 1, func(ctx context.Context) (any, error) {
		close(firstStarted)
		<-ctx.Done()
		close(firstCancelled)
		return "first", nil
	}, func(any, error) { firstDone.Store(true) })
	<-firstStarted
	var second any
	r.Go("decode:x", 1, func(ctx context.Context) (any, error) { return "second", nil }, func(v any, err error) { second = v })
	select {
	case <-firstCancelled:
	case <-time.After(5 * time.Second):
		t.Fatal("first job context was not cancelled")
	}
	waitUntil(t, "second result", func() bool { return q.size() >= 1 })
	time.Sleep(20 * time.Millisecond)
	q.runAll()
	if second != "second" {
		t.Fatalf("second = %v", second)
	}
	if firstDone.Load() {
		t.Fatal("cancelled job delivered its result")
	}
}

func TestDifferentKeysRunIndependently(t *testing.T) {
	checkLeaks(t)
	q := &uiQueue{}
	r := NewRunner(q.Post)
	defer r.Close()
	var count atomic.Int32
	for i := 0; i < 10; i++ {
		r.Go(fmt.Sprintf("thumb:%d", i), 0, func(ctx context.Context) (any, error) { return nil, nil }, func(any, error) { count.Add(1) })
	}
	waitUntil(t, "all posts", func() bool { return q.size() == 10 })
	q.runAll()
	if count.Load() != 10 {
		t.Fatalf("delivered %d of 10", count.Load())
	}
}

func TestCancelStopsContextAndSuppressesDone(t *testing.T) {
	checkLeaks(t)
	q := &uiQueue{}
	r := NewRunner(q.Post)
	defer r.Close()
	started := make(chan struct{})
	finished := make(chan struct{})
	var delivered atomic.Bool
	r.Go("scan:/p", 0, func(ctx context.Context) (any, error) {
		close(started)
		_, err := blockUntilCancelled(ctx)
		close(finished)
		return nil, err
	}, func(any, error) { delivered.Store(true) })
	<-started
	r.Cancel("scan:/p")
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("context not cancelled")
	}
	time.Sleep(10 * time.Millisecond)
	q.runAll()
	if delivered.Load() {
		t.Fatal("done ran after Cancel")
	}
	if r.Pending() != 0 {
		t.Fatalf("pending = %d", r.Pending())
	}
}

func TestCancelAfterPostedBeforeDelivered(t *testing.T) {
	checkLeaks(t)
	q := &uiQueue{}
	r := NewRunner(q.Post)
	defer r.Close()
	var delivered atomic.Bool
	r.Go("k", 0, func(ctx context.Context) (any, error) { return 1, nil }, func(any, error) { delivered.Store(true) })
	waitUntil(t, "post", func() bool { return q.size() == 1 })
	r.Cancel("k")
	q.runAll()
	if delivered.Load() {
		t.Fatal("result delivered after Cancel returned")
	}
}

func TestReplacedAfterPostedBeforeDelivered(t *testing.T) {
	checkLeaks(t)
	q := &uiQueue{}
	r := NewRunner(q.Post)
	defer r.Close()
	var first, second atomic.Bool
	r.Go("k", 0, func(ctx context.Context) (any, error) { return 1, nil }, func(any, error) { first.Store(true) })
	waitUntil(t, "first post", func() bool { return q.size() == 1 })
	r.Go("k", 0, func(ctx context.Context) (any, error) { return 2, nil }, func(any, error) { second.Store(true) })
	waitUntil(t, "second post", func() bool { return q.size() == 2 })
	q.runAll()
	if first.Load() || !second.Load() {
		t.Fatalf("first=%v second=%v", first.Load(), second.Load())
	}
}

func TestCancelPrefix(t *testing.T) {
	checkLeaks(t)
	q := &uiQueue{}
	r := NewRunner(q.Post)
	defer r.Close()
	var started sync.WaitGroup
	cancelled := make(chan string, 8)
	for _, key := range []string{"decode:a", "decode:b", "thumb:a"} {
		started.Add(1)
		r.Go(key, 0, func(ctx context.Context) (any, error) {
			started.Done()
			<-ctx.Done()
			cancelled <- key
			return nil, nil
		}, func(any, error) { t.Errorf("%s delivered", key) })
	}
	started.Wait()
	r.CancelPrefix("decode:")
	for i := 0; i < 2; i++ {
		select {
		case key := <-cancelled:
			if !strings.HasPrefix(key, "decode:") {
				t.Fatalf("cancelled %s", key)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("prefix jobs not cancelled")
		}
	}
	if r.Pending() != 1 {
		t.Fatalf("pending = %d, want the thumb job", r.Pending())
	}
	r.Cancel("thumb:a")
	<-cancelled
	q.runAll()
}

func TestGenerationDropsStaleResult(t *testing.T) {
	checkLeaks(t)
	q := &uiQueue{}
	r := NewRunner(q.Post)
	defer r.Close()
	release := make(chan struct{})
	var staleDelivered atomic.Bool
	r.SetGeneration("meta:f", 1)
	r.Go("meta:f", 1, func(ctx context.Context) (any, error) {
		<-release
		return "old", nil
	}, func(any, error) { staleDelivered.Store(true) })
	r.SetGeneration("meta:f", 2)
	close(release)
	var fresh any
	r.Go("meta:f", 2, func(ctx context.Context) (any, error) { return "new", nil }, func(v any, err error) { fresh = v })
	waitUntil(t, "fresh post", func() bool { return q.size() >= 1 })
	time.Sleep(20 * time.Millisecond)
	q.runAll()
	if staleDelivered.Load() {
		t.Fatal("stale generation delivered")
	}
	if fresh != "new" {
		t.Fatalf("fresh = %v", fresh)
	}
}

func TestGenerationMismatchOnGoNeverRuns(t *testing.T) {
	checkLeaks(t)
	r := NewRunner(nil)
	defer r.Close()
	r.SetGeneration("k", 5)
	var ran atomic.Bool
	r.Go("k", 4, func(ctx context.Context) (any, error) { ran.Store(true); return nil, nil }, func(any, error) { ran.Store(true) })
	time.Sleep(30 * time.Millisecond)
	if ran.Load() {
		t.Fatal("stale Go ran")
	}
	if r.Pending() != 0 {
		t.Fatalf("pending = %d", r.Pending())
	}
}

func TestGenerationDoesNotCancelMatchingJob(t *testing.T) {
	checkLeaks(t)
	r := NewRunner(nil)
	defer r.Close()
	res := make(chan int, 1)
	release := make(chan struct{})
	r.Go("k", 3, func(ctx context.Context) (any, error) { <-release; return 9, nil }, func(v any, err error) { res <- v.(int) })
	r.SetGeneration("k", 3)
	close(release)
	select {
	case v := <-res:
		if v != 9 {
			t.Fatalf("got %d", v)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("matching generation was dropped")
	}
}

func TestUnsetGenerationAcceptsAnyGen(t *testing.T) {
	checkLeaks(t)
	r := NewRunner(nil)
	defer r.Close()
	res := make(chan struct{}, 1)
	r.Go("k", 123456, func(ctx context.Context) (any, error) { return nil, nil }, func(any, error) { res <- struct{}{} })
	select {
	case <-res:
	case <-time.After(5 * time.Second):
		t.Fatal("not delivered")
	}
}

func TestCloseCancelsAndWaits(t *testing.T) {
	checkLeaks(t)
	r := NewRunner(nil)
	var running sync.WaitGroup
	var exited atomic.Int32
	for i := 0; i < 20; i++ {
		running.Add(1)
		r.Go(fmt.Sprintf("decode:%d", i), 0, func(ctx context.Context) (any, error) {
			running.Done()
			<-ctx.Done()
			time.Sleep(5 * time.Millisecond)
			exited.Add(1)
			return nil, ctx.Err()
		}, func(any, error) { t.Error("delivered after Close") })
	}
	running.Wait()
	r.Close()
	if exited.Load() != 20 {
		t.Fatalf("Close returned with %d of 20 jobs finished", exited.Load())
	}
	r.Close()
	called := false
	r.Go("late", 0, func(ctx context.Context) (any, error) { called = true; return nil, nil }, nil)
	r.SetGeneration("late", 1)
	r.Cancel("late")
	r.CancelPrefix("l")
	time.Sleep(10 * time.Millisecond)
	if called || r.Pending() != 0 {
		t.Fatal("Go after Close started work")
	}
}

func TestPanicInWorkBecomesError(t *testing.T) {
	checkLeaks(t)
	r := NewRunner(nil)
	defer r.Close()
	res := make(chan error, 1)
	r.Go("k", 0, func(ctx context.Context) (any, error) { panic("kaput") }, func(v any, err error) { res <- err })
	select {
	case err := <-res:
		var pe *PanicError
		if !errors.As(err, &pe) || pe.Key != "k" || pe.Value != "kaput" || len(pe.Stack) == 0 {
			t.Fatalf("got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no result")
	}
}

func TestNilDoneIsAllowed(t *testing.T) {
	checkLeaks(t)
	r := NewRunner(nil)
	defer r.Close()
	var ran atomic.Bool
	r.Go("k", 0, func(ctx context.Context) (any, error) { ran.Store(true); return nil, nil }, nil)
	waitUntil(t, "work", func() bool { return ran.Load() && r.Pending() == 0 })
}

func TestDoneMayScheduleSameKey(t *testing.T) {
	checkLeaks(t)
	r := NewRunner(nil)
	defer r.Close()
	res := make(chan int, 1)
	r.Go("k", 0, func(ctx context.Context) (any, error) { return 1, nil }, func(v any, err error) {
		r.Go("k", 0, func(ctx context.Context) (any, error) { return 2, nil }, func(v any, err error) { res <- v.(int) })
	})
	select {
	case v := <-res:
		if v != 2 {
			t.Fatalf("got %d", v)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("chained job did not complete")
	}
}

func TestStressSingleUIThread(t *testing.T) {
	checkLeaks(t)
	q := &uiQueue{}
	r := NewRunner(q.Post)
	const keyCount = 48
	key := func(k int) string { return fmt.Sprintf("%s:%d", []string{"decode", "thumb", "meta"}[k%3], k) }
	live := make([]int, keyCount)
	seqs := make([]int, keyCount)
	gens := make([]uint64, keyCount)
	for i := range live {
		live[i] = -1
	}
	var started, finished, delivered atomic.Int64
	rng := rand.New(rand.NewSource(7))
	for i := 0; i < 30000; i++ {
		k := rng.Intn(keyCount)
		switch op := rng.Intn(20); {
		case op < 11:
			seqs[k]++
			seq := seqs[k]
			live[k] = seq
			style := rng.Intn(3)
			r.Go(key(k), gens[k], func(ctx context.Context) (any, error) {
				started.Add(1)
				defer finished.Add(1)
				switch style {
				case 1:
					runtime.Gosched()
				case 2:
					select {
					case <-ctx.Done():
					case <-time.After(150 * time.Microsecond):
					}
				}
				return seq, ctx.Err()
			}, func(v any, err error) {
				delivered.Add(1)
				if v.(int) != seq {
					t.Errorf("key %d: payload %v for job %d", k, v, seq)
				}
				if live[k] != seq {
					t.Errorf("key %d: stale job %d delivered, live is %d", k, seq, live[k])
				}
				if err != nil {
					t.Errorf("key %d: delivered cancelled job: %v", k, err)
				}
				live[k] = -1
			})
		case op == 11:
			r.Cancel(key(k))
			live[k] = -1
		case op == 12:
			prefix := []string{"decode", "thumb", "meta"}[rng.Intn(3)]
			r.CancelPrefix(prefix)
			for j := range live {
				if strings.HasPrefix(key(j), prefix) {
					live[j] = -1
				}
			}
		case op == 13:
			gens[k]++
			r.SetGeneration(key(k), gens[k])
			live[k] = -1
		default:
			for n := rng.Intn(4); n > 0; n-- {
				if fn := q.take(); fn != nil {
					fn()
				}
			}
		}
	}
	waitUntil(t, "all live jobs delivered", func() bool {
		q.runAll()
		return r.Pending() == 0
	})
	for k, seq := range live {
		if seq != -1 {
			t.Errorf("key %d: job %d never delivered", k, seq)
		}
	}
	r.Close()
	q.runAll()
	if started.Load() != finished.Load() {
		t.Errorf("started %d, finished %d", started.Load(), finished.Load())
	}
	if delivered.Load() == 0 {
		t.Error("nothing was delivered")
	}
}

func TestStressConcurrentCallers(t *testing.T) {
	checkLeaks(t)
	q := &uiQueue{}
	r := NewRunner(q.Post)
	stopDrain := make(chan struct{})
	drained := make(chan struct{})
	go func() {
		defer close(drained)
		for {
			q.runAll()
			select {
			case <-stopDrain:
				q.runAll()
				return
			default:
				runtime.Gosched()
			}
		}
	}()
	var started, finished, delivered atomic.Int64
	var wg sync.WaitGroup
	for g := 0; g < 12; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			rng := rand.New(rand.NewSource(int64(g)))
			for i := 0; i < 3000; i++ {
				key := fmt.Sprintf("k%d:%d", g%3, rng.Intn(16))
				switch op := rng.Intn(12); {
				case op < 8:
					block := rng.Intn(2) == 0
					r.Go(key, uint64(rng.Intn(2)), func(ctx context.Context) (any, error) {
						started.Add(1)
						defer finished.Add(1)
						if block {
							<-ctx.Done()
						}
						return nil, nil
					}, func(any, error) { delivered.Add(1) })
				case op == 8:
					r.Cancel(key)
				case op == 9:
					r.CancelPrefix(fmt.Sprintf("k%d", rng.Intn(3)))
				case op == 10:
					r.SetGeneration(key, uint64(rng.Intn(2)))
				default:
					r.Pending()
				}
			}
		}(g)
	}
	wg.Wait()
	closed := make(chan struct{})
	go func() {
		r.Close()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(10 * time.Second):
		t.Fatal("Close did not return")
	}
	close(stopDrain)
	<-drained
	if started.Load() != finished.Load() {
		t.Errorf("started %d, finished %d", started.Load(), finished.Load())
	}
	if delivered.Load() > started.Load() {
		t.Errorf("delivered %d > started %d", delivered.Load(), started.Load())
	}
}

func TestNoGoroutineLeakAfterManyJobs(t *testing.T) {
	checkLeaks(t)
	r := NewRunner(nil)
	for i := 0; i < 500; i++ {
		r.Go(fmt.Sprintf("k%d", i%50), 0, func(ctx context.Context) (any, error) {
			select {
			case <-ctx.Done():
			case <-time.After(time.Millisecond):
			}
			return nil, nil
		}, func(any, error) {})
	}
	r.Close()
}
