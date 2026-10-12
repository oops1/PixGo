package gallery

import (
	"container/list"
	"image"
	"math"

	xdraw "golang.org/x/image/draw"

	"github.com/oops1/PixGo/internal/ui/gfx"
)

const (
	tileCacheEntries = 512
	tileCacheBytes   = 96 << 20
	outlineAlpha     = 0.14
)

type tileKey struct {
	src    *image.RGBA
	w, h   int
	radius int
}

type tileEntry struct {
	key tileKey
	img *image.RGBA
}

type tileCache struct {
	order *list.List
	items map[tileKey]*list.Element
	bytes int
}

func newTileCache() *tileCache {
	return &tileCache{order: list.New(), items: map[tileKey]*list.Element{}}
}

func (c *tileCache) get(src *image.RGBA, w, h, radius int) *image.RGBA {
	key := tileKey{src, w, h, radius}
	if el, ok := c.items[key]; ok {
		c.order.MoveToFront(el)
		return el.Value.(*tileEntry).img
	}
	img := renderTile(src, w, h, radius)
	c.items[key] = c.order.PushFront(&tileEntry{key, img})
	c.bytes += len(img.Pix)
	for c.order.Len() > tileCacheEntries || c.bytes > tileCacheBytes {
		back := c.order.Back()
		if back == nil || back == c.order.Front() {
			break
		}
		ent := back.Value.(*tileEntry)
		c.order.Remove(back)
		delete(c.items, ent.key)
		c.bytes -= len(ent.img.Pix)
	}
	return img
}

func (c *tileCache) reset() {
	c.order.Init()
	clear(c.items)
	c.bytes = 0
}

func (c *tileCache) size() int { return c.order.Len() }

func coverRect(sb image.Rectangle, w, h int) image.Rectangle {
	sw, sh := float64(sb.Dx()), float64(sb.Dy())
	scale := math.Max(float64(w)/sw, float64(h)/sh)
	cw, ch := float64(w)/scale, float64(h)/scale
	x0 := float64(sb.Min.X) + (sw-cw)/2
	y0 := float64(sb.Min.Y) + (sh-ch)/2
	r := image.Rect(int(math.Round(x0)), int(math.Round(y0)), int(math.Round(x0+cw)), int(math.Round(y0+ch)))
	r = r.Intersect(sb)
	if r.Empty() {
		return sb
	}
	return r
}

func renderTile(src *image.RGBA, w, h, radius int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	sb := src.Bounds()
	if sb.Empty() || w <= 0 || h <= 0 {
		return dst
	}
	xdraw.BiLinear.Scale(dst, dst.Bounds(), src, coverRect(sb, w, h), xdraw.Src, nil)
	roundTile(dst, radius)
	return dst
}

func roundTile(img *image.RGBA, radius int) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	dist := gfx.RoundRectDistance(b, float64(radius))
	for y := 0; y < h; y++ {
		edgeRow := y == 0 || y == h-1
		cornerRow := y < radius || y >= h-radius
		for x := 0; x < w; x++ {
			if !edgeRow && !cornerRow && x != 0 && x != w-1 {
				continue
			}
			d := dist(float64(x)+.5, float64(y)+.5)
			fill := clamp01(.5 - d)
			ring := clamp01(.5 - (math.Abs(d+.5) - .5))
			o := img.PixOffset(x, y)
			px := img.Pix[o : o+4 : o+4]
			a := ring * outlineAlpha * fill
			for i := 0; i < 4; i++ {
				v := float64(px[i]) * fill
				v = v*(1-a) + 255*a
				px[i] = uint8(v + .5)
			}
		}
	}
}

func clamp01(v float64) float64 { return math.Max(0, math.Min(1, v)) }
