package thumbs

import "container/list"

type ckey struct {
	path  string
	size  int64
	mtime int64
	side  int
	color string
}

type lruEntry struct {
	key   ckey
	thumb Thumb
}

type lru struct {
	max   int
	order *list.List
	items map[ckey]*list.Element
}

func newLRU(max int) *lru {
	return &lru{max: max, order: list.New(), items: map[ckey]*list.Element{}}
}

func (c *lru) get(k ckey) (Thumb, bool) {
	el, ok := c.items[k]
	if !ok {
		return Thumb{}, false
	}
	c.order.MoveToFront(el)
	return el.Value.(*lruEntry).thumb, true
}

func (c *lru) put(k ckey, t Thumb) {
	if el, ok := c.items[k]; ok {
		el.Value.(*lruEntry).thumb = t
		c.order.MoveToFront(el)
		return
	}
	c.items[k] = c.order.PushFront(&lruEntry{key: k, thumb: t})
	for c.order.Len() > c.max {
		back := c.order.Back()
		delete(c.items, back.Value.(*lruEntry).key)
		c.order.Remove(back)
	}
}

func (c *lru) len() int { return c.order.Len() }
