package imaging

import (
	"context"
	"sync"
)

type waiter struct {
	prio  func() Priority
	seq   uint64
	ready chan struct{}
}

type Reservation struct {
	cache *Cache
	bytes int64
	once  sync.Once
}

func (r *Reservation) Bytes() int64 {
	if r == nil {
		return 0
	}
	return r.bytes
}

func (r *Reservation) Release() {
	if r == nil {
		return
	}
	r.once.Do(func() {
		c := r.cache
		c.mu.Lock()
		c.reserved -= r.bytes
		c.unlockAndRelease(nil)
	})
}

func (c *Cache) Budget() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.budget
}

func (c *Cache) Reserved() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.reserved
}

func (c *Cache) Waiting() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.waiters)
}

func (c *Cache) headLocked() *waiter {
	var best *waiter
	for _, w := range c.waiters {
		p, bp := w.prio(), Priority(0)
		if best != nil {
			bp = best.prio()
		}
		if best == nil || p < bp || (p == bp && w.seq < best.seq) {
			best = w
		}
	}
	return best
}

func (c *Cache) wakeLocked() {
	if len(c.waiters) == 0 {
		return
	}
	if w := c.headLocked(); w != nil {
		select {
		case w.ready <- struct{}{}:
		default:
		}
	}
}

func (c *Cache) removeWaiterLocked(w *waiter) {
	for i, x := range c.waiters {
		if x == w {
			c.waiters = append(c.waiters[:i], c.waiters[i+1:]...)
			return
		}
	}
}

func (c *Cache) evictToLocked(target int64) {
	for e := c.order.Back(); e != nil && c.used > target; {
		prev := e.Prev()
		it := e.Value.(*cacheItem)
		if c.pinned[it.key] == 0 {
			c.removeLocked(e, false)
		}
		e = prev
	}
}

func (c *Cache) fitsLocked(bytes int64, alone bool) bool {
	if c.budget <= 0 {
		return true
	}
	c.evictToLocked(c.budget - c.reserved - bytes)
	if alone && c.reserved == 0 {
		return true
	}
	return c.used+c.reserved+bytes <= c.budget
}

func (c *Cache) grantLocked(bytes int64) *Reservation {
	c.reserved += bytes
	return &Reservation{cache: c, bytes: max(bytes, 0)}
}

func (c *Cache) TryReserve(bytes int64) (*Reservation, bool) {
	bytes = max(bytes, 0)
	c.mu.Lock()
	if len(c.waiters) > 0 || !c.fitsLocked(bytes, false) {
		c.unlockAndDrop(nil)
		return nil, false
	}
	r := c.grantLocked(bytes)
	c.unlockAndDrop(nil)
	return r, true
}

func (c *Cache) Reserve(ctx context.Context, bytes int64, prio func() Priority) (*Reservation, error) {
	bytes = max(bytes, 0)
	if prio == nil {
		prio = func() Priority { return PrioCurrent }
	}
	w := &waiter{prio: prio, ready: make(chan struct{}, 1)}
	c.mu.Lock()
	c.waitSeq++
	w.seq = c.waitSeq
	c.waiters = append(c.waiters, w)
	for {
		if err := ctx.Err(); err != nil {
			c.removeWaiterLocked(w)
			c.unlockAndRelease(nil)
			return nil, err
		}
		if c.headLocked() == w && c.fitsLocked(bytes, true) {
			c.removeWaiterLocked(w)
			r := c.grantLocked(bytes)
			c.unlockAndRelease(nil)
			return r, nil
		}
		c.unlockAndDrop(nil)
		select {
		case <-ctx.Done():
		case <-w.ready:
		}
		c.mu.Lock()
	}
}

func (c *Cache) Kick() {
	c.mu.Lock()
	c.wakeLocked()
	c.mu.Unlock()
}
