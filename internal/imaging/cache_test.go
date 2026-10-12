package imaging

import (
	"fmt"
	"image"
	"sync"
	"testing"
	"time"

	"github.com/oops1/PixGo/internal/codec"
)

var cacheEpoch = time.Date(2026, 1, 2, 3, 4, 5, 600, time.UTC)

func decodedOf(path string, bytes int, mod time.Time) *Decoded {
	side := bytes / 4
	img := image.NewNRGBA(image.Rect(0, 0, side, 1))
	return &Decoded{
		Key:    Key{Path: path, Size: int64(bytes), ModTime: mod},
		Levels: []*image.NRGBA{img},
	}
}

func keyOf(d *Decoded) Key { return d.Key }

func TestCacheLRUEvictsLeastRecentlyUsed(t *testing.T) {
	c := NewCache(1000)
	a := decodedOf("a", 400, cacheEpoch)
	b := decodedOf("b", 400, cacheEpoch)
	d := decodedOf("c", 400, cacheEpoch)
	c.Put(a)
	c.Put(b)
	if _, ok := c.Get(keyOf(a)); !ok {
		t.Fatal("a must be present")
	}
	c.Put(d)
	if _, ok := c.Get(keyOf(b)); ok {
		t.Fatal("b was least recently used and must be evicted")
	}
	if _, ok := c.Get(keyOf(a)); !ok {
		t.Fatal("a was touched and must stay")
	}
	if _, ok := c.Get(keyOf(d)); !ok {
		t.Fatal("c must be present")
	}
	if c.Len() != 2 || c.Used() != 800 {
		t.Fatalf("len %d used %d", c.Len(), c.Used())
	}
}

func TestCacheKeyEquality(t *testing.T) {
	c := NewCache(1 << 20)
	d := decodedOf("a", 400, cacheEpoch)
	c.Put(d)
	same := Key{Path: "a", Size: 400, ModTime: cacheEpoch.In(time.FixedZone("x", 3600))}
	if _, ok := c.Get(same); !ok {
		t.Fatal("same instant in another zone must match")
	}
	for name, k := range map[string]Key{
		"size":  {Path: "a", Size: 401, ModTime: cacheEpoch},
		"mtime": {Path: "a", Size: 400, ModTime: cacheEpoch.Add(time.Nanosecond)},
		"path":  {Path: "b", Size: 400, ModTime: cacheEpoch},
	} {
		if _, ok := c.Get(k); ok {
			t.Fatalf("differing %s must miss", name)
		}
	}
}

func TestCachePutReplacesStaleVersionOfSamePath(t *testing.T) {
	c := NewCache(1 << 20)
	old := decodedOf("a", 400, cacheEpoch)
	fresh := decodedOf("a", 800, cacheEpoch.Add(time.Second))
	c.Put(old)
	c.Put(fresh)
	if c.Len() != 1 || c.Used() != 800 {
		t.Fatalf("len %d used %d", c.Len(), c.Used())
	}
	if _, ok := c.Get(keyOf(old)); ok {
		t.Fatal("stale key must be gone")
	}
	c.Put(fresh)
	if c.Len() != 1 || c.Used() != 800 {
		t.Fatalf("re-put changed accounting: len %d used %d", c.Len(), c.Used())
	}
}

func TestCachePinnedEntriesSurvive(t *testing.T) {
	c := NewCache(1000)
	a := decodedOf("a", 400, cacheEpoch)
	b := decodedOf("b", 400, cacheEpoch)
	d := decodedOf("c", 400, cacheEpoch)
	c.Put(a)
	c.Pin(keyOf(a))
	c.Put(b)
	c.Put(d)
	if _, ok := c.Get(keyOf(a)); !ok {
		t.Fatal("pinned entry evicted")
	}
	if _, ok := c.Get(keyOf(b)); ok {
		t.Fatal("unpinned older entry must be evicted instead")
	}
	c.Unpin(keyOf(a))
	e := decodedOf("e", 400, cacheEpoch)
	c.Put(e)
	c.Put(decodedOf("f", 400, cacheEpoch))
	if _, ok := c.Get(keyOf(a)); ok {
		t.Fatal("unpinned entry must become evictable")
	}
}

func TestCachePinBeforePutAndNesting(t *testing.T) {
	c := NewCache(500)
	a := decodedOf("a", 400, cacheEpoch)
	c.Pin(keyOf(a))
	c.Pin(keyOf(a))
	c.Put(a)
	c.Put(decodedOf("b", 400, cacheEpoch))
	if _, ok := c.Get(keyOf(a)); !ok {
		t.Fatal("pin placed before Put must protect the entry")
	}
	c.Unpin(keyOf(a))
	c.Put(decodedOf("c", 400, cacheEpoch))
	if _, ok := c.Get(keyOf(a)); !ok {
		t.Fatal("one pin still outstanding")
	}
	c.Unpin(keyOf(a))
	c.Put(decodedOf("d", 400, cacheEpoch))
	if _, ok := c.Get(keyOf(a)); ok {
		t.Fatal("all pins released, entry must be evictable")
	}
}

func TestCacheNewestEntrySurvivesEvenWhenOverBudget(t *testing.T) {
	c := NewCache(100)
	big := decodedOf("big", 400, cacheEpoch)
	c.Put(decodedOf("small", 40, cacheEpoch))
	c.Put(big)
	if _, ok := c.Get(keyOf(big)); !ok {
		t.Fatal("the entry just stored must stay")
	}
	if c.Len() != 1 {
		t.Fatalf("len %d", c.Len())
	}
}

func TestCacheSetBudgetEvicts(t *testing.T) {
	c := NewCache(10000)
	for i := 0; i < 10; i++ {
		c.Put(decodedOf(fmt.Sprint("p", i), 400, cacheEpoch))
	}
	if c.Len() != 10 {
		t.Fatalf("len %d", c.Len())
	}
	c.SetBudget(1000)
	if c.Used() > 1000 || c.Len() != 2 {
		t.Fatalf("used %d len %d", c.Used(), c.Len())
	}
	if _, ok := c.Get(Key{Path: "p9", Size: 400, ModTime: cacheEpoch}); !ok {
		t.Fatal("most recent entries must survive")
	}
	c.SetBudget(0)
	if c.Len() != 0 || c.Used() != 0 {
		t.Fatalf("budget 0 must empty the cache: len %d used %d", c.Len(), c.Used())
	}
}

func TestCacheRemoveByPath(t *testing.T) {
	c := NewCache(1 << 20)
	a := decodedOf("a", 400, cacheEpoch)
	c.Put(a)
	c.Put(decodedOf("b", 400, cacheEpoch))
	c.Pin(keyOf(a))
	c.Remove("a")
	if _, ok := c.Get(keyOf(a)); ok {
		t.Fatal("removed")
	}
	if c.Len() != 1 || c.Used() != 400 {
		t.Fatalf("len %d used %d", c.Len(), c.Used())
	}
	c.Put(a)
	c.SetBudget(0)
	if _, ok := c.Get(keyOf(a)); ok {
		t.Fatal("pin must have been dropped together with the entry")
	}
	c.Remove("missing")
}

func TestCacheBytesCountsFramesAndLevels(t *testing.T) {
	l0 := image.NewNRGBA(image.Rect(0, 0, 10, 10))
	l1 := image.NewNRGBA(image.Rect(0, 0, 5, 5))
	f1 := image.NewNRGBA(image.Rect(0, 0, 10, 10))
	d := &Decoded{
		Levels: []*image.NRGBA{l0, l1},
		Frames: []codec.Frame{{Pix: l0}, {Pix: f1}},
	}
	if got, want := d.Bytes(), int64(400+100+400); got != want {
		t.Fatalf("bytes %d want %d", got, want)
	}
}

func TestCacheConcurrentUse(t *testing.T) {
	c := NewCache(4000)
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 500; i++ {
				d := decodedOf(fmt.Sprint("f", (g*7+i)%25), 400, cacheEpoch)
				switch i % 7 {
				case 0:
					c.Remove(d.Key.Path)
				case 1:
					c.Pin(d.Key)
				case 2:
					c.Unpin(d.Key)
				case 3:
					c.SetBudget(int64(3000 + i%2000))
				default:
					c.Put(d)
					c.Get(d.Key)
				}
				c.Used()
				c.Len()
			}
		}(g)
	}
	wg.Wait()
	if c.Used() < 0 {
		t.Fatal("negative usage")
	}
}
