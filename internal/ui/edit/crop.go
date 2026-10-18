package edit

import (
	"image"
	"math"
)

const MinSide = 16

type Handle uint8

const (
	HandleNone Handle = iota
	HandleNW
	HandleN
	HandleNE
	HandleE
	HandleSE
	HandleS
	HandleSW
	HandleW
	HandleMove
)

var handleList = []Handle{HandleNW, HandleNE, HandleSE, HandleSW, HandleN, HandleE, HandleS, HandleW}

func (h Handle) west() bool  { return h == HandleNW || h == HandleW || h == HandleSW }
func (h Handle) east() bool  { return h == HandleNE || h == HandleE || h == HandleSE }
func (h Handle) north() bool { return h == HandleNW || h == HandleN || h == HandleNE }
func (h Handle) south() bool { return h == HandleSW || h == HandleS || h == HandleSE }

func (h Handle) corner() bool {
	return (h.west() || h.east()) && (h.north() || h.south())
}

type Aspect uint8

const (
	AspectFree Aspect = iota
	AspectOriginal
	AspectSquare
	Aspect43
	Aspect32
	Aspect169
)

var Aspects = []Aspect{AspectFree, AspectOriginal, AspectSquare, Aspect43, Aspect32, Aspect169}

func (a Aspect) base(size image.Point) float64 {
	switch a {
	case AspectOriginal:
		if size.X <= 0 || size.Y <= 0 {
			return 1
		}
		return float64(max(size.X, size.Y)) / float64(min(size.X, size.Y))
	case Aspect43:
		return 4.0 / 3.0
	case Aspect32:
		return 3.0 / 2.0
	case Aspect169:
		return 16.0 / 9.0
	}
	return 1
}

type Crop struct {
	size     image.Point
	rect     image.Rectangle
	aspect   Aspect
	portrait bool
}

func NewCrop(w, h int) *Crop {
	c := &Crop{size: image.Pt(max(w, 1), max(h, 1))}
	c.rect = c.bounds()
	return c
}

func (c *Crop) bounds() image.Rectangle { return image.Rectangle{Max: c.size} }

func (c *Crop) Size() image.Point { return c.size }

func (c *Crop) Rect() image.Rectangle { return c.rect }

func (c *Crop) Whole() bool { return c.rect == c.bounds() }

func (c *Crop) Aspect() Aspect { return c.aspect }

func (c *Crop) Portrait() bool { return c.portrait }

func (c *Crop) minSide() int { return min(MinSide, min(c.size.X, c.size.Y)) }

func (c *Crop) Ratio() float64 {
	switch c.aspect {
	case AspectFree:
		return 0
	case AspectSquare:
		return 1
	}
	r := c.aspect.base(c.size)
	if c.portrait {
		return 1 / r
	}
	return r
}

func (c *Crop) Reset() {
	c.rect = c.bounds()
	c.aspect = AspectFree
	c.portrait = false
}

func (c *Crop) SetRect(r image.Rectangle) {
	r = r.Canon().Intersect(c.bounds())
	m := c.minSide()
	if r.Dx() < m || r.Dy() < m {
		return
	}
	c.rect = r
}

func (c *Crop) SetAspect(a Aspect) {
	c.aspect = a
	c.portrait = c.size.Y > c.size.X
	if a == AspectFree {
		return
	}
	c.rect = c.fitInside(c.rect, c.Ratio())
}

func (c *Crop) Swap() {
	w, h := c.rect.Dx(), c.rect.Dy()
	if c.aspect == AspectFree || c.aspect == AspectSquare {
		c.rect = c.centered(c.rect, h, w)
		return
	}
	c.portrait = !c.portrait
	c.rect = c.fitInside(c.centered(c.rect, h, w), c.Ratio())
}

func (c *Crop) centered(r image.Rectangle, w, h int) image.Rectangle {
	w, h = min(w, c.size.X), min(h, c.size.Y)
	cx, cy := float64(r.Min.X+r.Max.X)/2, float64(r.Min.Y+r.Max.Y)/2
	x := int(math.Round(cx - float64(w)/2))
	y := int(math.Round(cy - float64(h)/2))
	x = min(max(x, 0), c.size.X-w)
	y = min(max(y, 0), c.size.Y-h)
	return image.Rect(x, y, x+w, y+h)
}

func (c *Crop) fitInside(r image.Rectangle, ratio float64) image.Rectangle {
	if ratio <= 0 {
		return r
	}
	w, h := float64(r.Dx()), float64(r.Dy())
	if w/h > ratio {
		w = h * ratio
	} else {
		h = w / ratio
	}
	wi := max(int(math.Round(w)), 1)
	hi := max(int(math.Round(float64(wi)/ratio)), 1)
	if hi > r.Dy() {
		hi = r.Dy()
		wi = max(int(math.Round(float64(hi)*ratio)), 1)
	}
	if wi > r.Dx() {
		wi = r.Dx()
		hi = max(int(math.Round(float64(wi)/ratio)), 1)
	}
	wi, hi = c.atLeastMin(wi, hi, ratio)
	return c.centered(r, wi, hi)
}

func (c *Crop) atLeastMin(w, h int, ratio float64) (int, int) {
	m := c.minSide()
	if w >= m && h >= m {
		return w, h
	}
	if ratio <= 0 {
		return max(w, m), max(h, m)
	}
	w = max(w, int(math.Ceil(float64(m)*math.Max(1, ratio))))
	h = max(int(math.Round(float64(w)/ratio)), m)
	if w > c.size.X {
		w = c.size.X
		h = max(int(math.Round(float64(w)/ratio)), 1)
	}
	if h > c.size.Y {
		h = c.size.Y
		w = max(int(math.Round(float64(h)*ratio)), 1)
	}
	return min(w, c.size.X), min(h, c.size.Y)
}

func (c *Crop) MoveTo(start image.Rectangle, dx, dy float64) {
	w, h := start.Dx(), start.Dy()
	x := int(math.Round(float64(start.Min.X) + dx))
	y := int(math.Round(float64(start.Min.Y) + dy))
	x = min(max(x, 0), c.size.X-w)
	y = min(max(y, 0), c.size.Y-h)
	c.rect = image.Rect(x, y, x+w, y+h)
}

func (c *Crop) Nudge(dx, dy int) {
	c.MoveTo(c.rect, float64(dx), float64(dy))
}

func (c *Crop) Grow(dw, dh int) {
	r := c.rect
	ratio := c.Ratio()
	if ratio <= 0 {
		c.dragFree(HandleSE, r, float64(dw), float64(dh))
		return
	}
	w := float64(r.Dy()+dh) * ratio
	if dw != 0 {
		w = float64(r.Dx() + dw)
	}
	wmax := float64(c.size.X - r.Min.X)
	hmax := float64(c.size.Y - r.Min.Y)
	w = math.Max(math.Min(w, math.Min(wmax, hmax*ratio)), 0)
	wi, hi := c.finish(w, ratio, int(wmax), int(hmax))
	c.rect = image.Rect(r.Min.X, r.Min.Y, r.Min.X+wi, r.Min.Y+hi)
}

func (c *Crop) Drag(h Handle, start image.Rectangle, dx, dy float64) {
	switch {
	case h == HandleMove:
		c.MoveTo(start, dx, dy)
	case h == HandleNone:
	case c.aspect == AspectFree || c.Ratio() <= 0:
		c.dragFree(h, start, dx, dy)
	case h.corner():
		c.dragCorner(h, start, dx, dy)
	default:
		c.dragEdge(h, start, dx, dy)
	}
}

func (c *Crop) dragFree(h Handle, s image.Rectangle, dx, dy float64) {
	m := c.minSide()
	l, t, r, b := s.Min.X, s.Min.Y, s.Max.X, s.Max.Y
	if h.west() {
		l = min(max(int(math.Round(float64(s.Min.X)+dx)), 0), r-m)
	}
	if h.east() {
		r = max(min(int(math.Round(float64(s.Max.X)+dx)), c.size.X), l+m)
	}
	if h.north() {
		t = min(max(int(math.Round(float64(s.Min.Y)+dy)), 0), b-m)
	}
	if h.south() {
		b = max(min(int(math.Round(float64(s.Max.Y)+dy)), c.size.Y), t+m)
	}
	c.rect = image.Rect(l, t, r, b)
}

func (c *Crop) dragCorner(h Handle, s image.Rectangle, dx, dy float64) {
	ratio := c.Ratio()
	var ax, ay, sx, sy, px, py float64
	if h.east() {
		ax, sx, px = float64(s.Min.X), 1, float64(s.Max.X)+dx
	} else {
		ax, sx, px = float64(s.Max.X), -1, float64(s.Min.X)+dx
	}
	if h.south() {
		ay, sy, py = float64(s.Min.Y), 1, float64(s.Max.Y)+dy
	} else {
		ay, sy, py = float64(s.Max.Y), -1, float64(s.Min.Y)+dy
	}
	w := math.Max((px-ax)*sx, 0)
	hh := math.Max((py-ay)*sy, 0)
	w = math.Max(w, hh*ratio)
	wmax := ax
	if sx > 0 {
		wmax = float64(c.size.X) - ax
	}
	hmax := ay
	if sy > 0 {
		hmax = float64(c.size.Y) - ay
	}
	w = math.Min(w, math.Min(wmax, hmax*ratio))
	wi, hi := c.finish(w, ratio, int(wmax), int(hmax))
	x0, y0 := int(ax), int(ay)
	if sx < 0 {
		x0 -= wi
	}
	if sy < 0 {
		y0 -= hi
	}
	c.rect = image.Rect(x0, y0, x0+wi, y0+hi)
}

func (c *Crop) finish(w, ratio float64, wmax, hmax int) (int, int) {
	wi := int(math.Round(w))
	hi := int(math.Round(float64(wi) / ratio))
	wi, hi = c.atLeastMin(max(wi, 1), max(hi, 1), ratio)
	for (wi > wmax || hi > hmax) && wi > 1 && hi > 1 {
		wi--
		hi = int(math.Round(float64(wi) / ratio))
	}
	return wi, hi
}

func (c *Crop) dragEdge(h Handle, s image.Rectangle, dx, dy float64) {
	ratio := c.Ratio()
	if h == HandleE || h == HandleW {
		cy := float64(s.Min.Y+s.Max.Y) / 2
		w, wmax := float64(s.Dx())+dx, float64(c.size.X-s.Min.X)
		if h == HandleW {
			w, wmax = float64(s.Dx())-dx, float64(s.Max.X)
		}
		vmax := 2 * math.Min(cy, float64(c.size.Y)-cy)
		wmax = math.Min(wmax, vmax*ratio)
		w = math.Max(math.Min(w, wmax), 0)
		wi, hi := c.finish(w, ratio, int(wmax), int(vmax))
		x0 := s.Min.X
		if h == HandleW {
			x0 = s.Max.X - wi
		}
		y0 := min(max(int(math.Round(cy-float64(hi)/2)), 0), c.size.Y-hi)
		c.rect = image.Rect(x0, y0, x0+wi, y0+hi)
		return
	}
	cx := float64(s.Min.X+s.Max.X) / 2
	hh, hmax := float64(s.Dy())+dy, float64(c.size.Y-s.Min.Y)
	if h == HandleN {
		hh, hmax = float64(s.Dy())-dy, float64(s.Max.Y)
	}
	hmax = math.Min(hmax, 2*math.Min(cx, float64(c.size.X)-cx)/ratio)
	hh = math.Max(math.Min(hh, hmax), 0)
	wi, hi := c.finish(hh*ratio, ratio, int(2*math.Min(cx, float64(c.size.X)-cx)), int(hmax))
	y0 := s.Min.Y
	if h == HandleN {
		y0 = s.Max.Y - hi
	}
	x0 := min(max(int(math.Round(cx-float64(wi)/2)), 0), c.size.X-wi)
	c.rect = image.Rect(x0, y0, x0+wi, y0+hi)
}

func (c *Crop) Rotate(cw bool) {
	r, w, h := c.rect, c.size.X, c.size.Y
	if cw {
		c.rect = image.Rect(h-r.Max.Y, r.Min.X, h-r.Min.Y, r.Max.X)
	} else {
		c.rect = image.Rect(r.Min.Y, w-r.Max.X, r.Max.Y, w-r.Min.X)
	}
	c.size = image.Pt(h, w)
	if c.aspect != AspectFree && c.aspect != AspectSquare {
		c.portrait = !c.portrait
	}
}

func (c *Crop) FlipH() {
	r := c.rect
	c.rect = image.Rect(c.size.X-r.Max.X, r.Min.Y, c.size.X-r.Min.X, r.Max.Y)
}

func (c *Crop) FlipV() {
	r := c.rect
	c.rect = image.Rect(r.Min.X, c.size.Y-r.Max.Y, r.Max.X, c.size.Y-r.Min.Y)
}
