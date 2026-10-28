package render3d

import (
	"context"
	"math"
	"sync/atomic"

	"github.com/oops1/PixGo/internal/par"
)

const (
	subBits     = 8
	subUnit     = 1 << subBits
	subHalf     = subUnit / 2
	guard       = 1 << 20
	emptyKey    = math.MaxUint64
	qScale      = 4294967040.0
	rasterChunk = 8192
)

type rasterizer struct {
	v        *view
	buf      []uint64
	ws       int64
	ry0      int64
	ylo, yhi int64
}

type svert struct{ x, y, q float64 }

type cvert struct{ x, y, z float64 }

func (s *Scene) rasterize(ctx context.Context, v *view, buf []uint64, ry0, ry1 int) error {
	chunks := (s.tris + rasterChunk - 1) / rasterChunk
	workers := par.Count()
	rows := ry1 - ry0
	stripes := 1
	if chunks < 4*workers && workers > 1 {
		stripes = max(1, min(4*workers/chunks, rows/8))
	}
	stripeRows := (rows + stripes - 1) / stripes
	stripes = (rows + stripeRows - 1) / stripeRows
	var stop atomic.Bool
	vtx := s.m.V
	par.Parallel(chunks*stripes, workers, func(t int) {
		if stop.Load() {
			return
		}
		if ctx.Err() != nil {
			stop.Store(true)
			return
		}
		c, st := t/stripes, t%stripes
		y0 := ry0 + st*stripeRows
		y1 := min(y0+stripeRows, ry1)
		r := rasterizer{v: v, buf: buf, ws: int64(v.ws), ry0: int64(ry0), ylo: int64(y0), yhi: int64(y1)}
		end := min((c+1)*rasterChunk, s.tris)
		if v.ortho {
			for i := c * rasterChunk; i < end; i++ {
				r.ortho(vtx[i*9:i*9+9:i*9+9], uint32(i))
			}
		} else {
			for i := c * rasterChunk; i < end; i++ {
				r.persp(vtx[i*9:i*9+9:i*9+9], uint32(i))
			}
		}
	})
	if stop.Load() {
		return ctx.Err()
	}
	return nil
}

func (r *rasterizer) ortho(p []float32, id uint32) {
	if !finiteTri(p) {
		return
	}
	v := r.v
	var sv [3]svert
	inGuard := true
	for k := 0; k < 3; k++ {
		px, py, pz := float64(p[3*k]), float64(p[3*k+1]), float64(p[3*k+2])
		x := v.ax[0]*px + v.ax[1]*py + v.ax[2]*pz + v.ax[3]
		y := v.ay[0]*px + v.ay[1]*py + v.ay[2]*pz + v.ay[3]
		q := v.aq[0]*px + v.aq[1]*py + v.aq[2]*pz + v.aq[3]
		if !(x > -guard && x < guard && y > -guard && y < guard) {
			inGuard = false
		}
		sv[k] = svert{x, y, q}
	}
	if inGuard {
		r.emit(sv[0], sv[1], sv[2], id)
		return
	}
	r.clipped(sv[:], id)
}

func (r *rasterizer) persp(p []float32, id uint32) {
	if !finiteTri(p) {
		return
	}
	v := r.v
	var cv [3]cvert
	inside := 0
	for k := 0; k < 3; k++ {
		px, py, pz := float64(p[3*k]), float64(p[3*k+1]), float64(p[3*k+2])
		cv[k] = cvert{
			v.ax[0]*px + v.ax[1]*py + v.ax[2]*pz + v.ax[3],
			v.ay[0]*px + v.ay[1]*py + v.ay[2]*pz + v.ay[3],
			v.az[0]*px + v.az[1]*py + v.az[2]*pz + v.az[3],
		}
		if cv[k].z >= v.near {
			inside++
		}
	}
	if inside == 0 {
		return
	}
	var poly [12]svert
	n := 0
	if inside == 3 {
		for k := 0; k < 3; k++ {
			poly[k] = v.project(cv[k])
		}
		n = 3
	} else {
		for k := 0; k < 3; k++ {
			a, b := cv[k], cv[(k+1)%3]
			ina, inb := a.z >= v.near, b.z >= v.near
			if ina {
				poly[n] = v.project(a)
				n++
			}
			if ina != inb {
				t := (v.near - a.z) / (b.z - a.z)
				c := cvert{a.x + (b.x-a.x)*t, a.y + (b.y-a.y)*t, v.near}
				poly[n] = v.project(c)
				n++
			}
		}
	}
	inGuard := true
	for k := 0; k < n; k++ {
		x, y := poly[k].x, poly[k].y
		if !(x > -guard && x < guard && y > -guard && y < guard) {
			inGuard = false
			break
		}
	}
	if !inGuard {
		r.clipped(poly[:n], id)
		return
	}
	for k := 1; k+1 < n; k++ {
		r.emit(poly[0], poly[k], poly[k+1], id)
	}
}

func (v *view) project(c cvert) svert {
	iz := 1 / c.z
	return svert{v.cx + v.ps*c.x*iz, v.cy - v.ps*c.y*iz, 1 - v.near*iz}
}

func (r *rasterizer) clipped(in []svert, id uint32) {
	var a, b [16]svert
	n := copy(a[:], in)
	src, dst := a[:], b[:]
	for plane := 0; plane < 4 && n > 0; plane++ {
		m := 0
		for k := 0; k < n; k++ {
			p, q := src[k], src[(k+1)%n]
			dp, dq := guardDist(p, plane), guardDist(q, plane)
			if dp >= 0 {
				dst[m] = p
				m++
			}
			if (dp >= 0) != (dq >= 0) && m < len(dst) {
				t := dp / (dp - dq)
				dst[m] = svert{p.x + (q.x-p.x)*t, p.y + (q.y-p.y)*t, p.q + (q.q-p.q)*t}
				m++
			}
		}
		src, dst = dst, src
		n = m
	}
	for k := 1; k+1 < n; k++ {
		r.emit(src[0], src[k], src[k+1], id)
	}
}

func guardDist(p svert, plane int) float64 {
	const g = guard - 1
	switch plane {
	case 0:
		return p.x + g
	case 1:
		return g - p.x
	case 2:
		return p.y + g
	}
	return g - p.y
}

func fix(x float64) int64 {
	return int64(math.Floor(x*subUnit + 0.5))
}

func (r *rasterizer) emit(a, b, c svert, id uint32) {
	if !(a.x-a.x == 0 && b.x-b.x == 0 && c.x-c.x == 0 && a.y-a.y == 0 && b.y-b.y == 0 && c.y-c.y == 0) {
		return
	}
	r.triangle(fix(a.x), fix(a.y), fix(b.x), fix(b.y), fix(c.x), fix(c.y), a.q, b.q, c.q, id)
}

func (r *rasterizer) triangle(x0, y0, x1, y1, x2, y2 int64, q0, q1, q2 float64, id uint32) {
	area := (x1-x0)*(y2-y0) - (y1-y0)*(x2-x0)
	if area == 0 {
		return
	}
	if area < 0 {
		x1, y1, q1, x2, y2, q2 = x2, y2, q2, x1, y1, q1
		area = -area
	}
	i0 := max((min(x0, x1, x2)-subHalf+subUnit-1)>>subBits, 0)
	i1 := min((max(x0, x1, x2)-subHalf)>>subBits, r.ws-1)
	j0 := max((min(y0, y1, y2)-subHalf+subUnit-1)>>subBits, r.ylo)
	j1 := min((max(y0, y1, y2)-subHalf)>>subBits, r.yhi-1)
	if i0 > i1 || j0 > j1 {
		return
	}
	a0, b0 := y1-y2, x2-x1
	a1, b1 := y2-y0, x0-x2
	a2, b2 := y0-y1, x1-x0
	c0 := x1*y2 - x2*y1
	c1 := x2*y0 - x0*y2
	c2 := x0*y1 - x1*y0
	px := i0*subUnit + subHalf
	py := j0*subUnit + subHalf
	w0 := a0*px + b0*py + c0 + topLeftBias(x1, y1, x2, y2)
	w1 := a1*px + b1*py + c1 + topLeftBias(x2, y2, x0, y0)
	w2 := a2*px + b2*py + c2 + topLeftBias(x0, y0, x1, y1)
	inv := 1 / float64(area)
	qx := (float64(a0)*q0 + float64(a1)*q1 + float64(a2)*q2) * subUnit * inv
	qy := (float64(b0)*q0 + float64(b1)*q1 + float64(b2)*q2) * subUnit * inv
	qs := (float64(a0*px+b0*py+c0)*q0 + float64(a1*px+b1*py+c1)*q1 + float64(a2*px+b2*py+c2)*q2) * inv
	sx0, sx1, sx2 := a0*subUnit, a1*subUnit, a2*subUnit
	sy0, sy1, sy2 := b0*subUnit, b1*subUnit, b2*subUnit
	tag := uint64(id)
	if i1-i0 >= 12 {
		for j := j0; j <= j1; j++ {
			lo, hi := spanEdge(w0, sx0, 0, i1-i0)
			lo, hi = spanEdge(w1, sx1, lo, hi)
			lo, hi = spanEdge(w2, sx2, lo, hi)
			w0 += sy0
			w1 += sy1
			w2 += sy2
			if lo > hi {
				continue
			}
			row := r.buf[(j-r.ry0)*r.ws+i0 : (j-r.ry0+1)*r.ws]
			qr := qs + qy*float64(j-j0)
			for d := lo; d <= hi; d++ {
				q := qr + qx*float64(d)
				q = max(0, min(1, q))
				atomicMin(&row[d], uint64(q*qScale)<<32|tag)
			}
		}
		return
	}
	for j := j0; j <= j1; j++ {
		e0, e1, e2 := w0, w1, w2
		row := r.buf[(j-r.ry0)*r.ws : (j-r.ry0+1)*r.ws]
		qr := qs + qy*float64(j-j0)
		for i := i0; i <= i1; i++ {
			if e0|e1|e2 >= 0 {
				q := qr + qx*float64(i-i0)
				q = max(0, min(1, q))
				key := uint64(q*qScale)<<32 | tag
				atomicMin(&row[i], key)
			}
			e0 += sx0
			e1 += sx1
			e2 += sx2
		}
		w0 += sy0
		w1 += sy1
		w2 += sy2
	}
}

func spanEdge(e, step, lo, hi int64) (int64, int64) {
	switch {
	case step > 0:
		if e < 0 {
			lo = max(lo, (-e+step-1)/step)
		}
	case step < 0:
		if e < 0 {
			return 1, 0
		}
		hi = min(hi, e/(-step))
	default:
		if e < 0 {
			return 1, 0
		}
	}
	return lo, hi
}

func topLeftBias(ax, ay, bx, by int64) int64 {
	dy, dx := by-ay, bx-ax
	if dy < 0 || (dy == 0 && dx > 0) {
		return 0
	}
	return -1
}

func atomicMin(p *uint64, key uint64) {
	for {
		old := atomic.LoadUint64(p)
		if key >= old {
			return
		}
		if atomic.CompareAndSwapUint64(p, old, key) {
			return
		}
	}
}
