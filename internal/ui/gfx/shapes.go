package gfx

import (
	"image/color"
	"math"
	"sync"

	"github.com/oops1/headless-gui/v3/widget"
)

type Shape func(x, y float64) bool

func Box(x0, y0, x1, y1 float64) Shape {
	return func(x, y float64) bool { return x >= x0 && x < x1 && y >= y0 && y < y1 }
}

func Capsule(x1, y1, x2, y2, w float64) Shape {
	dx, dy := x2-x1, y2-y1
	l2, r2 := dx*dx+dy*dy, w*w/4
	return func(x, y float64) bool {
		t := 0.0
		if l2 > 0 {
			t = math.Max(0, math.Min(1, ((x-x1)*dx+(y-y1)*dy)/l2))
		}
		ex, ey := x-x1-t*dx, y-y1-t*dy
		return ex*ex+ey*ey <= r2
	}
}

func Path(w float64, pts ...float64) Shape {
	var s []Shape
	for i := 2; i+1 < len(pts); i += 2 {
		s = append(s, Capsule(pts[i-2], pts[i-1], pts[i], pts[i+1], w))
	}
	return Union(s...)
}

func Poly(pts ...float64) Shape {
	return func(x, y float64) bool {
		in := false
		for i, j := 0, len(pts)-2; i < len(pts); j, i = i, i+2 {
			xi, yi, xj, yj := pts[i], pts[i+1], pts[j], pts[j+1]
			if (yi > y) != (yj > y) && x < (xj-xi)*(y-yi)/(yj-yi)+xi {
				in = !in
			}
		}
		return in
	}
}

func RRect(x0, y0, x1, y1, r float64) Shape {
	return func(x, y float64) bool {
		if x < x0 || x >= x1 || y < y0 || y >= y1 {
			return false
		}
		cx := math.Max(x0+r, math.Min(x1-r, x))
		cy := math.Max(y0+r, math.Min(y1-r, y))
		return (x-cx)*(x-cx)+(y-cy)*(y-cy) <= r*r
	}
}

func Frame(x0, y0, x1, y1, r, w float64) Shape {
	return Minus(RRect(x0, y0, x1, y1, r), RRect(x0+w, y0+w, x1-w, y1-w, math.Max(0, r-w)))
}

func Circle(cx, cy, r float64) Shape {
	return func(x, y float64) bool { return (x-cx)*(x-cx)+(y-cy)*(y-cy) <= r*r }
}

func Annulus(cx, cy, r, w float64) Shape {
	return func(x, y float64) bool {
		d := math.Hypot(x-cx, y-cy)
		return d >= r-w/2 && d <= r+w/2
	}
}

func Arc(cx, cy, r, w, a0, a1 float64) Shape {
	ring := Annulus(cx, cy, r, w)
	return func(x, y float64) bool {
		if !ring(x, y) {
			return false
		}
		a := math.Atan2(y-cy, x-cx) * 180 / math.Pi
		for a < a0 {
			a += 360
		}
		return a <= a1
	}
}

func Union(s ...Shape) Shape {
	return func(x, y float64) bool {
		for _, f := range s {
			if f(x, y) {
				return true
			}
		}
		return false
	}
}

func Minus(a, b Shape) Shape { return func(x, y float64) bool { return a(x, y) && !b(x, y) } }
func Inter(a, b Shape) Shape { return func(x, y float64) bool { return a(x, y) && b(x, y) } }

func Scaled(s Shape, k float64) Shape { return func(x, y float64) bool { return s(x/k, y/k) } }

func Mask(s Shape, w, h int) []uint8 {
	const n = 8
	out := make([]uint8, w*h)
	for j := 0; j < h; j++ {
		for i := 0; i < w; i++ {
			in := 0
			for sy := 0; sy < n; sy++ {
				for sx := 0; sx < n; sx++ {
					if s(float64(i)+(float64(sx)+.5)/n, float64(j)+(float64(sy)+.5)/n) {
						in++
					}
				}
			}
			out[j*w+i] = uint8(in * 255 / (n * n))
		}
	}
	return out
}

var masks = struct {
	sync.Mutex
	m map[string][]uint8
}{m: map[string][]uint8{}}

func Paint(c widget.DrawContext, key string, x, y, w, h int, col color.RGBA, build func() Shape) {
	masks.Lock()
	m, ok := masks.m[key]
	if !ok {
		m = Mask(build(), w, h)
		masks.m[key] = m
	}
	masks.Unlock()
	PaintMask(c, m, x, y, w, h, col)
}

func PaintMask(c widget.DrawContext, m []uint8, x, y, w, h int, col color.RGBA) {
	if !Visible(c, R(x, y, w, h)) {
		return
	}
	for j := 0; j < h; j++ {
		for i := 0; i < w; i++ {
			a := m[j*w+i]
			if a == 0 {
				continue
			}
			if a == 255 && col.A == 255 {
				c.FillRect(x+i, y+j, 1, 1, col)
			} else {
				c.FillRectAlpha(x+i, y+j, 1, 1, Alpha(col, float64(a)/255*float64(col.A)/255))
			}
		}
	}
}
