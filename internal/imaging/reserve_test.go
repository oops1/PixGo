package imaging

import (
	"context"
	"errors"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func currentPrio() Priority  { return PrioCurrent }
func neighborPrio() Priority { return PrioNeighbor }

type reservation struct {
	r   *Reservation
	err error
}

func reserveAsync(c *Cache, ctx context.Context, bytes int64, prio func() Priority) chan reservation {
	out := make(chan reservation, 1)
	go func() {
		r, err := c.Reserve(ctx, bytes, prio)
		out <- reservation{r, err}
	}()
	return out
}

func waitWaiters(t *testing.T, c *Cache, n int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for c.Waiting() != n {
		if time.Now().After(deadline) {
			t.Fatalf("waiters %d, want %d", c.Waiting(), n)
		}
		time.Sleep(time.Millisecond)
	}
}

func expectGranted(t *testing.T, ch chan reservation) *Reservation {
	t.Helper()
	select {
	case got := <-ch:
		if got.err != nil {
			t.Fatalf("reserve failed: %v", got.err)
		}
		return got.r
	case <-time.After(5 * time.Second):
		t.Fatal("reservation was not granted")
		return nil
	}
}

func expectBlocked(t *testing.T, ch chan reservation) {
	t.Helper()
	select {
	case got := <-ch:
		t.Fatalf("reservation granted too early: %+v", got)
	case <-time.After(30 * time.Millisecond):
	}
}

func TestReserveWithinBudgetIsImmediate(t *testing.T) {
	c := NewCache(1000)
	r, err := c.Reserve(context.Background(), 400, currentPrio)
	if err != nil || r.Bytes() != 400 || c.Reserved() != 400 {
		t.Fatalf("reservation %v err %v reserved %d", r, err, c.Reserved())
	}
	r.Release()
	if c.Reserved() != 0 {
		t.Fatalf("reserved %d after release", c.Reserved())
	}
}

func TestReleaseIsIdempotentAndNilSafe(t *testing.T) {
	c := NewCache(1000)
	r, _ := c.Reserve(context.Background(), 300, nil)
	r.Release()
	r.Release()
	var none *Reservation
	none.Release()
	if none.Bytes() != 0 || c.Reserved() != 0 {
		t.Fatalf("reserved %d", c.Reserved())
	}
}

func TestReserveEvictsLeastRecentlyUsedUnpinnedEntries(t *testing.T) {
	c := NewCache(1000)
	a := decodedOf("a", 400, cacheEpoch)
	b := decodedOf("b", 400, cacheEpoch)
	c.Put(a)
	c.Put(b)
	c.Get(keyOf(a))
	r, err := c.Reserve(context.Background(), 500, currentPrio)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Get(keyOf(b)); ok {
		t.Fatal("the least recently used entry must make room")
	}
	if _, ok := c.Get(keyOf(a)); !ok {
		t.Fatal("the recently used entry must stay")
	}
	if !b.Released() {
		t.Fatal("an evicted entry must be released")
	}
	r.Release()
}

func TestReserveNeverEvictsPinnedEntries(t *testing.T) {
	c := NewCache(1000)
	a := decodedOf("a", 700, cacheEpoch)
	c.Put(a)
	c.Pin(keyOf(a))
	first, _ := c.Reserve(context.Background(), 200, currentPrio)
	second := reserveAsync(c, context.Background(), 300, currentPrio)
	expectBlocked(t, second)
	if _, ok := c.Get(keyOf(a)); !ok || a.Released() {
		t.Fatal("a pinned entry must not be evicted for a reservation")
	}
	first.Release()
	expectGranted(t, second).Release()
}

func TestReserveLetsOneJobProceedEvenWhenItNeverFits(t *testing.T) {
	c := NewCache(1000)
	huge, err := c.Reserve(context.Background(), 5000, currentPrio)
	if err != nil {
		t.Fatal(err)
	}
	next := reserveAsync(c, context.Background(), 10, currentPrio)
	expectBlocked(t, next)
	huge.Release()
	expectGranted(t, next).Release()
}

func TestReserveUnpinWakesWaiter(t *testing.T) {
	c := NewCache(1000)
	a := decodedOf("a", 800, cacheEpoch)
	c.Put(a)
	c.Pin(keyOf(a))
	hold, _ := c.Reserve(context.Background(), 100, currentPrio)
	waiting := reserveAsync(c, context.Background(), 600, currentPrio)
	expectBlocked(t, waiting)
	c.Unpin(keyOf(a))
	hold.Release()
	expectGranted(t, waiting).Release()
}

func TestReserveSetBudgetWakesWaiter(t *testing.T) {
	c := NewCache(500)
	hold, _ := c.Reserve(context.Background(), 400, currentPrio)
	waiting := reserveAsync(c, context.Background(), 400, currentPrio)
	expectBlocked(t, waiting)
	c.SetBudget(2000)
	expectGranted(t, waiting).Release()
	hold.Release()
}

func TestReserveCancelRemovesTheWaiter(t *testing.T) {
	c := NewCache(500)
	hold, _ := c.Reserve(context.Background(), 400, currentPrio)
	ctx, cancel := context.WithCancel(context.Background())
	waiting := reserveAsync(c, ctx, 400, currentPrio)
	waitWaiters(t, c, 1)
	cancel()
	got := <-waiting
	if !errors.Is(got.err, context.Canceled) || got.r != nil {
		t.Fatalf("got %+v", got)
	}
	waitWaiters(t, c, 0)
	hold.Release()
	if c.Reserved() != 0 {
		t.Fatalf("reserved %d", c.Reserved())
	}
}

func TestReserveWithCancelledContextFailsFast(t *testing.T) {
	c := NewCache(500)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Reserve(ctx, 10, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("err %v", err)
	}
	if c.Waiting() != 0 || c.Reserved() != 0 {
		t.Fatalf("waiting %d reserved %d", c.Waiting(), c.Reserved())
	}
}

func TestReserveServesCurrentBeforeNeighbor(t *testing.T) {
	c := NewCache(500)
	hold, _ := c.Reserve(context.Background(), 400, currentPrio)
	neighbor := reserveAsync(c, context.Background(), 300, neighborPrio)
	waitWaiters(t, c, 1)
	current := reserveAsync(c, context.Background(), 300, currentPrio)
	waitWaiters(t, c, 2)
	hold.Release()
	first := expectGranted(t, current)
	expectBlocked(t, neighbor)
	first.Release()
	expectGranted(t, neighbor).Release()
}

func TestReserveServesEqualPrioritiesInArrivalOrder(t *testing.T) {
	c := NewCache(500)
	hold, _ := c.Reserve(context.Background(), 400, currentPrio)
	var order []int
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := c.Reserve(context.Background(), 400, currentPrio)
			if err != nil {
				return
			}
			mu.Lock()
			order = append(order, i)
			mu.Unlock()
			r.Release()
		}()
		waitWaiters(t, c, i+1)
	}
	hold.Release()
	wg.Wait()
	for i, v := range order {
		if v != i {
			t.Fatalf("order %v", order)
		}
	}
}

func TestReserveFollowsPriorityChangesAfterKick(t *testing.T) {
	c := NewCache(500)
	hold, _ := c.Reserve(context.Background(), 400, currentPrio)
	var promoted atomic.Bool
	dynamic := func() Priority {
		if promoted.Load() {
			return PrioCurrent
		}
		return PrioPrefetch
	}
	first := reserveAsync(c, context.Background(), 300, neighborPrio)
	waitWaiters(t, c, 1)
	second := reserveAsync(c, context.Background(), 300, dynamic)
	waitWaiters(t, c, 2)
	promoted.Store(true)
	c.Kick()
	hold.Release()
	expectGranted(t, second).Release()
	expectGranted(t, first).Release()
}

func TestTryReserveGrantsOnlyWhatFitsAndNeverWaits(t *testing.T) {
	c := NewCache(1000)
	if r, ok := c.TryReserve(600); !ok || c.Reserved() != 600 {
		t.Fatalf("first try: %v %v", r, ok)
	}
	if _, ok := c.TryReserve(600); ok {
		t.Fatal("a second 600 must not fit into 1000")
	}
	if c.Waiting() != 0 {
		t.Fatal("TryReserve must not queue")
	}
	if r, ok := c.TryReserve(400); !ok {
		t.Fatal("400 still fits")
	} else {
		r.Release()
	}
}

func TestTryReserveYieldsToWaitingCurrent(t *testing.T) {
	c := NewCache(500)
	hold, _ := c.Reserve(context.Background(), 400, currentPrio)
	waiting := reserveAsync(c, context.Background(), 300, currentPrio)
	waitWaiters(t, c, 1)
	if _, ok := c.TryReserve(10); ok {
		t.Fatal("a prefetch must not overtake a waiting current job")
	}
	hold.Release()
	expectGranted(t, waiting).Release()
}

func TestTryReserveEvictsCachedEntriesForRoom(t *testing.T) {
	c := NewCache(1000)
	old := decodedOf("old", 800, cacheEpoch)
	c.Put(old)
	r, ok := c.TryReserve(500)
	if !ok {
		t.Fatal("an evictable entry must make room for a prefetch")
	}
	if !old.Released() {
		t.Fatal("the evicted entry must be released")
	}
	r.Release()
}

func TestTryReserveRefusesWhenPinnedEntriesFillTheBudget(t *testing.T) {
	c := NewCache(1000)
	a := decodedOf("a", 800, cacheEpoch)
	c.Put(a)
	c.Pin(keyOf(a))
	if _, ok := c.TryReserve(500); ok {
		t.Fatal("pinned data cannot be evicted, the prefetch must be refused")
	}
}

func TestZeroBudgetMeansNoLimit(t *testing.T) {
	c := NewCache(0)
	var held []*Reservation
	for i := 0; i < 5; i++ {
		r, err := c.Reserve(context.Background(), 1<<30, currentPrio)
		if err != nil {
			t.Fatal(err)
		}
		held = append(held, r)
	}
	if _, ok := c.TryReserve(1 << 30); !ok {
		t.Fatal("an unlimited cache grants every prefetch")
	}
	for _, r := range held {
		r.Release()
	}
}

func TestReserveNegativeSizeIsTreatedAsZero(t *testing.T) {
	c := NewCache(100)
	r, err := c.Reserve(context.Background(), -50, nil)
	if err != nil || r.Bytes() != 0 || c.Reserved() != 0 {
		t.Fatalf("%v %v %d", r, err, c.Reserved())
	}
	if r2, ok := c.TryReserve(-5); !ok || r2.Bytes() != 0 {
		t.Fatal("a negative try must be a zero reservation")
	}
}

func TestReserveManyConcurrentJobsNeverDeadlock(t *testing.T) {
	c := NewCache(1000)
	for i := 0; i < 6; i++ {
		c.Put(decodedOf("p"+string(rune('a'+i)), 200, cacheEpoch))
	}
	var wg sync.WaitGroup
	var inFlight, peak atomic.Int64
	for g := 0; g < 16; g++ {
		g := g
		wg.Add(1)
		go func() {
			defer wg.Done()
			rnd := rand.New(rand.NewSource(int64(g)))
			for i := 0; i < 60; i++ {
				bytes := int64(50 + rnd.Intn(700))
				prio := Priority(rnd.Intn(3))
				var r *Reservation
				if prio == PrioCurrent || rnd.Intn(2) == 0 {
					ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					got, err := c.Reserve(ctx, bytes, func() Priority { return prio })
					cancel()
					if err != nil {
						t.Errorf("reserve failed: %v", err)
						return
					}
					r = got
				} else if got, ok := c.TryReserve(bytes); ok {
					r = got
				} else {
					continue
				}
				n := inFlight.Add(1)
				for {
					p := peak.Load()
					if n <= p || peak.CompareAndSwap(p, n) {
						break
					}
				}
				time.Sleep(time.Duration(rnd.Intn(200)) * time.Microsecond)
				inFlight.Add(-1)
				r.Release()
			}
		}()
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(60 * time.Second):
		t.Fatal("reservations deadlocked")
	}
	if c.Reserved() != 0 || c.Waiting() != 0 {
		t.Fatalf("reserved %d waiting %d after all jobs finished", c.Reserved(), c.Waiting())
	}
	if peak.Load() < 2 {
		t.Fatalf("expected some concurrency, peak %d", peak.Load())
	}
}

func TestReservedBytesNeverExceedBudgetWhileSeveralJobsRun(t *testing.T) {
	c := NewCache(1000)
	var wg sync.WaitGroup
	var violations atomic.Int64
	for g := 0; g < 12; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 40; i++ {
				r, err := c.Reserve(context.Background(), 300, currentPrio)
				if err != nil {
					return
				}
				if c.Reserved() > 1000 {
					violations.Add(1)
				}
				time.Sleep(50 * time.Microsecond)
				r.Release()
			}
		}()
	}
	wg.Wait()
	if violations.Load() != 0 {
		t.Fatalf("%d observations above the budget", violations.Load())
	}
}

func TestBudgetAccessor(t *testing.T) {
	c := NewCache(123)
	if c.Budget() != 123 {
		t.Fatalf("budget %d", c.Budget())
	}
	c.SetBudget(456)
	if c.Budget() != 456 {
		t.Fatalf("budget %d", c.Budget())
	}
}
