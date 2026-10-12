package imaging

import (
	"container/list"
	"sync"
)

type cacheKey struct {
	path string
	size int64
	sec  int64
	nsec int
}

func makeCacheKey(k Key) cacheKey {
	return cacheKey{path: k.Path, size: k.Size, sec: k.ModTime.Unix(), nsec: k.ModTime.Nanosecond()}
}

type cacheItem struct {
	key   cacheKey
	img   *Decoded
	bytes int64
}

type Cache struct {
	mu     sync.Mutex
	budget int64
	used   int64
	order  *list.List
	items  map[cacheKey]*list.Element
	pinned map[cacheKey]int
}

func NewCache(budget int64) *Cache {
	return &Cache{
		budget: budget,
		order:  list.New(),
		items:  map[cacheKey]*list.Element{},
		pinned: map[cacheKey]int{},
	}
}

func (c *Cache) Get(k Key) (*Decoded, bool) {
	ck := makeCacheKey(k)
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.items[ck]
	if !ok {
		return nil, false
	}
	c.order.MoveToFront(e)
	return e.Value.(*cacheItem).img, true
}

func (c *Cache) Put(d *Decoded) {
	if d == nil {
		return
	}
	ck := makeCacheKey(d.Key)
	bytes := d.Bytes()
	c.mu.Lock()
	defer c.mu.Unlock()
	for e := c.order.Front(); e != nil; {
		next := e.Next()
		it := e.Value.(*cacheItem)
		if it.key.path == ck.path {
			c.removeLocked(e, it.key != ck)
		}
		e = next
	}
	e := c.order.PushFront(&cacheItem{key: ck, img: d, bytes: bytes})
	c.items[ck] = e
	c.used += bytes
	c.evictLocked(e)
}

func (c *Cache) Remove(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for e := c.order.Front(); e != nil; {
		next := e.Next()
		if it := e.Value.(*cacheItem); it.key.path == path {
			c.removeLocked(e, true)
		}
		e = next
	}
	for k := range c.pinned {
		if k.path == path {
			delete(c.pinned, k)
		}
	}
}

func (c *Cache) SetBudget(budget int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.budget = budget
	c.evictLocked(nil)
}

func (c *Cache) Pin(k Key) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pinned[makeCacheKey(k)]++
}

func (c *Cache) Unpin(k Key) {
	ck := makeCacheKey(k)
	c.mu.Lock()
	defer c.mu.Unlock()
	if n := c.pinned[ck]; n > 1 {
		c.pinned[ck] = n - 1
	} else {
		delete(c.pinned, ck)
	}
	c.evictLocked(nil)
}

func (c *Cache) Used() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.used
}

func (c *Cache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.order.Len()
}

func (c *Cache) removeLocked(e *list.Element, dropPin bool) {
	it := e.Value.(*cacheItem)
	c.order.Remove(e)
	delete(c.items, it.key)
	c.used -= it.bytes
	if dropPin {
		delete(c.pinned, it.key)
	}
}

func (c *Cache) evictLocked(keep *list.Element) {
	for e := c.order.Back(); e != nil && c.used > c.budget; {
		prev := e.Prev()
		it := e.Value.(*cacheItem)
		if e != keep && c.pinned[it.key] == 0 {
			c.removeLocked(e, false)
		}
		e = prev
	}
}
