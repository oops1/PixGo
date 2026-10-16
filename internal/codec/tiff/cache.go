package tiff

import (
	"container/list"
	"context"
	"errors"
	"image"
	"sync"

	"github.com/oops1/PixGo/internal/codec/internal/shield"
)

type bandKey struct {
	chunk, band int
}

type cacheItem struct {
	key   bandKey
	img   *image.NRGBA
	err   error
	ready chan struct{}
	elem  *list.Element
}

type cache struct {
	mu      sync.Mutex
	budget  int64
	used    int64
	closed  bool
	items   map[bandKey]*cacheItem
	recency *list.List
}

func newCache(budget int64) *cache {
	return &cache{budget: budget, items: make(map[bandKey]*cacheItem), recency: list.New()}
}

func isContextError(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func imageBytes(img *image.NRGBA) int64 {
	return int64(len(img.Pix))
}

func (c *cache) get(ctx context.Context, key bandKey, load func(context.Context) (*image.NRGBA, error)) (*image.NRGBA, error) {
	if c.budget <= 0 {
		return load(ctx)
	}
	for {
		c.mu.Lock()
		if c.closed {
			c.mu.Unlock()
			return load(ctx)
		}
		if item, ok := c.items[key]; ok {
			if item.elem != nil {
				c.recency.MoveToFront(item.elem)
			}
			c.mu.Unlock()
			select {
			case <-item.ready:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			if item.err == nil {
				return item.img, nil
			}
			if ctx.Err() == nil && isContextError(item.err) {
				continue
			}
			return nil, item.err
		}
		item := &cacheItem{key: key, ready: make(chan struct{})}
		c.items[key] = item
		c.mu.Unlock()
		var img *image.NRGBA
		err := shield.Run("tiff", func() (loadErr error) {
			img, loadErr = load(ctx)
			return loadErr
		})
		c.finish(item, img, err)
		return img, err
	}
}

func (c *cache) finish(item *cacheItem, img *image.NRGBA, err error) {
	c.mu.Lock()
	item.img, item.err = img, err
	size := int64(0)
	if img != nil {
		size = imageBytes(img)
	}
	if err != nil || size > c.budget || c.closed {
		if c.items[item.key] == item {
			delete(c.items, item.key)
		}
	} else {
		item.elem = c.recency.PushFront(item)
		c.used += size
		for c.used > c.budget {
			oldest := c.recency.Back()
			victim := oldest.Value.(*cacheItem)
			c.recency.Remove(oldest)
			delete(c.items, victim.key)
			c.used -= imageBytes(victim.img)
		}
	}
	c.mu.Unlock()
	close(item.ready)
}

func (c *cache) drop() {
	c.mu.Lock()
	c.closed = true
	c.items = make(map[bandKey]*cacheItem)
	c.recency.Init()
	c.used = 0
	c.mu.Unlock()
}
