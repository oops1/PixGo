package icons

import (
	"image/color"
	"math"
	"sync"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/ui/gfx"
)

type Size struct{ W, H int }

const (
	primary = iota
	secondary
	fixed
)

type layer struct {
	shape  func() gfx.Shape
	tone   int
	top    color.RGBA
	bottom color.RGBA
}

type def struct {
	size   Size
	layers []layer
}

var defs = map[string]def{}

func init() {
	for _, group := range []map[string]def{chromeDefs, interfaceDefs, colourDefs} {
		for k, v := range group {
			defs[k] = v
		}
	}
	for k, p := range tunings {
		defs[k] = tune(defs[k], p[0], p[1], p[2], p[3], p[4])
	}
}

func mono(w, h int, shape func() gfx.Shape) def {
	return def{Size{w, h}, []layer{{shape: shape, tone: primary}}}
}

func multi(w, h int, layers ...layer) def { return def{Size{w, h}, layers} }

func ink(shape func() gfx.Shape) layer { return layer{shape: shape, tone: primary} }

func soft(shape func() gfx.Shape) layer { return layer{shape: shape, tone: secondary} }

func flat(col uint32, a float64, shape func() gfx.Shape) layer {
	c := gfx.RGB(col)
	c.A = uint8(a*255 + .5)
	return layer{shape: shape, tone: fixed, top: c, bottom: c}
}

func ramp(top, bottom uint32, a float64, shape func() gfx.Shape) layer {
	t, b := gfx.RGB(top), gfx.RGB(bottom)
	t.A, b.A = uint8(a*255+.5), uint8(a*255+.5)
	return layer{shape: shape, tone: fixed, top: t, bottom: b}
}

var cache = struct {
	sync.Mutex
	m map[string][]uint8
}{m: map[string][]uint8{}}

func maskOf(name string, i int, d def) []uint8 {
	key := name + "#" + string(rune('0'+i))
	cache.Lock()
	defer cache.Unlock()
	m, ok := cache.m[key]
	if !ok {
		m = gfx.Mask(d.layers[i].shape(), d.size.W, d.size.H)
		cache.m[key] = m
	}
	return m
}

type hiKey struct {
	name   string
	layer  int
	k      float64
	pw, ph int
}

var hiCache = struct {
	sync.Mutex
	m map[hiKey][]uint8
}{m: map[hiKey][]uint8{}}

func maskOfHi(name string, i int, d def, k float64, pw, ph int) []uint8 {
	key := hiKey{name, i, k, pw, ph}
	hiCache.Lock()
	defer hiCache.Unlock()
	m, ok := hiCache.m[key]
	if !ok {
		m = gfx.MaskHi(d.layers[i].shape(), pw, ph, k)
		hiCache.m[key] = m
	}
	return m
}

func drawHi(c widget.DrawContext, name string, d def, x, y int, a, b color.RGBA, k float64) {
	pw, ph := gfx.HiPhysSize(x, y, d.size.W, d.size.H, k)
	for i, l := range d.layers {
		m := maskOfHi(name, i, d, k, pw, ph)
		switch l.tone {
		case primary:
			gfx.PaintMaskHi(c, m, x, y, d.size.W, d.size.H, k, func(int, int) color.RGBA { return a })
		case secondary:
			gfx.PaintMaskHi(c, m, x, y, d.size.W, d.size.H, k, func(int, int) color.RGBA { return b })
		default:
			top, bottom := l.top, l.bottom
			gfx.PaintMaskHi(c, m, x, y, d.size.W, d.size.H, k, func(row, rows int) color.RGBA {
				f := (float64(row) + .5) / float64(rows)
				return color.RGBA{lerp(top.R, bottom.R, f), lerp(top.G, bottom.G, f), lerp(top.B, bottom.B, f), lerp(top.A, bottom.A, f)}
			})
		}
	}
}

func Has(name string) bool { _, ok := defs[name]; return ok }

func SizeOf(name string) Size { return defs[name].size }

func Draw(c widget.DrawContext, name string, x, y int, col color.RGBA) {
	DrawTwoTone(c, name, x, y, col, gfx.Alpha(col, .6))
}

func DrawTwoTone(c widget.DrawContext, name string, x, y int, a, b color.RGBA) {
	d, ok := defs[name]
	if !ok {
		return
	}
	if k := widget.ContextScale(c); k != 1 {
		drawHi(c, name, d, x, y, a, b, k)
		return
	}
	for i, l := range d.layers {
		m := maskOf(name, i, d)
		switch l.tone {
		case primary:
			gfx.PaintMask(c, m, x, y, d.size.W, d.size.H, a)
		case secondary:
			gfx.PaintMask(c, m, x, y, d.size.W, d.size.H, b)
		default:
			paintRamp(c, m, x, y, d.size.W, d.size.H, l.top, l.bottom)
		}
	}
}

func lerp(a, b uint8, f float64) uint8 { return uint8(float64(a) + (float64(b)-float64(a))*f + .5) }

func paintRamp(c widget.DrawContext, m []uint8, x, y, w, h int, top, bottom color.RGBA) {
	if !gfx.Visible(c, gfx.R(x, y, w, h)) {
		return
	}
	for j := 0; j < h; j++ {
		f := (float64(j) + .5) / float64(h)
		col := color.RGBA{lerp(top.R, bottom.R, f), lerp(top.G, bottom.G, f), lerp(top.B, bottom.B, f), lerp(top.A, bottom.A, f)}
		for i := 0; i < w; i++ {
			a := m[j*w+i]
			switch {
			case a == 0:
			case a == 255 && col.A == 255:
				c.FillRect(x+i, y+j, 1, 1, col)
			default:
				c.FillRectAlpha(x+i, y+j, 1, 1, gfx.Alpha(col, float64(a)/255*float64(col.A)/255))
			}
		}
	}
}

func DrawCentered(c widget.DrawContext, name string, cx, cy int, col color.RGBA) {
	d, ok := defs[name]
	if !ok {
		return
	}
	Draw(c, name, cx-d.size.W/2, cy-d.size.H/2, col)
}

func arcPts(cx, cy, r, a0, a1 float64, n int) []float64 {
	out := make([]float64, 0, 2*n+2)
	for i := 0; i <= n; i++ {
		a := (a0 + (a1-a0)*float64(i)/float64(n)) * math.Pi / 180
		out = append(out, cx+r*math.Cos(a), cy+r*math.Sin(a))
	}
	return out
}

func heartPts(cx, cy, dx, r, ty float64) []float64 {
	ny := cy - math.Sqrt(r*r-dx*dx)
	a0 := math.Atan2(ny-cy, -dx) * 180 / math.Pi
	vx, vy := cx-(cx+dx), ty-cy
	phi := math.Atan2(vy, vx)
	d := math.Hypot(vx, vy)
	a1 := (phi - math.Acos(r/d)) * 180 / math.Pi
	if a0 > 0 {
		a0 -= 360
	}
	right := arcPts(cx+dx, cy, r, a0, a1, 24)
	out := append([]float64{}, right...)
	out = append(out, cx, ty)
	for i := len(right) - 2; i >= 0; i -= 2 {
		out = append(out, 2*cx-right[i], right[i+1])
	}
	return out
}

func gearPts(cx, cy, rOut, rIn float64, teeth int, top, base float64) []float64 {
	var out []float64
	step := 2 * math.Pi / float64(teeth)
	put := func(r, a float64) { out = append(out, cx+r*math.Cos(a), cy+r*math.Sin(a)) }
	for i := 0; i < teeth; i++ {
		a := float64(i)*step - math.Pi/2
		put(rIn, a-base)
		put(rOut, a-top)
		put(rOut, a+top)
		put(rIn, a+base)
		put(rIn, a+base+(step-2*base)/2)
	}
	out = append(out, out[0], out[1])
	return out
}

func ellipseRing(cx, cy, rx, ry, w float64) gfx.Shape {
	t := 2 * w / (rx + ry) / 2
	return func(x, y float64) bool {
		return math.Abs(math.Hypot((x-cx)/rx, (y-cy)/ry)-1) <= t
	}
}

func transpose(pts []float64) []float64 {
	out := make([]float64, len(pts))
	for i := 0; i+1 < len(pts); i += 2 {
		out[i], out[i+1] = pts[i+1], pts[i]
	}
	return out
}

func scaleY(pts []float64, k float64) []float64 {
	out := make([]float64, len(pts))
	copy(out, pts)
	for i := 1; i < len(out); i += 2 {
		out[i] *= k
	}
	return out
}

func grown(f gfx.Shape, r float64) gfx.Shape {
	if r == 0 {
		return f
	}
	k := r * math.Sqrt2 / 2
	offs := [8][2]float64{{r, 0}, {-r, 0}, {0, r}, {0, -r}, {k, k}, {-k, k}, {k, -k}, {-k, -k}}
	if r > 0 {
		return func(x, y float64) bool {
			if f(x, y) {
				return true
			}
			for _, o := range offs {
				if f(x+o[0], y+o[1]) {
					return true
				}
			}
			return false
		}
	}
	return func(x, y float64) bool {
		if !f(x, y) {
			return false
		}
		for _, o := range offs {
			if !f(x+o[0], y+o[1]) {
				return false
			}
		}
		return true
	}
}

func tune(d def, dx, dy, sx, sy, grow float64) def {
	cx, cy := float64(d.size.W)/2, float64(d.size.H)/2
	layers := make([]layer, len(d.layers))
	for i, l := range d.layers {
		inner := l.shape
		l.shape = func() gfx.Shape {
			f := grown(inner(), grow)
			return func(x, y float64) bool { return f((x-cx-dx)/sx+cx, (y-cy-dy)/sy+cy) }
		}
		layers[i] = l
	}
	return def{d.size, layers}
}
