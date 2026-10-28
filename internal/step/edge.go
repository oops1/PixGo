package step

import (
	"math"
)

const (
	maxEdgePoints = 1 << 16
	maxAdaptDepth = 22
)

func arcStep(r, defl, ang float64) float64 {
	if !(r > defl) {
		return ang
	}
	return math.Min(ang, 2*math.Acos(1-defl/r))
}

func (e *edgeGeom) discretize() {
	e.pts, e.ts = nil, nil
	if !e.ok || e.c == nil {
		e.ok = false
		return
	}
	t0, t1, ok := e.paramRange()
	if !ok {
		e.ok = false
		return
	}
	var ts []float64
	switch c := e.c.(type) {
	case *lineCurve:
		ts = []float64{t0, t1}
	case *circleCurve:
		n := int(math.Ceil(math.Abs(t1-t0) / arcStep(c.r, e.defl, e.ang)))
		if e.closed {
			n = max(n, 3)
		}
		n = min(max(n, 1), maxEdgePoints)
		ts = uniform(t0, t1, n)
	case *polyCurve:
		ts = []float64{t0}
		lo, hi := math.Min(t0, t1), math.Max(t0, t1)
		var mids []float64
		for k := math.Floor(lo) + 1; k < hi && len(mids) < maxEdgePoints; k++ {
			mids = append(mids, k)
		}
		if t1 < t0 {
			for i, j := 0, len(mids)-1; i < j; i, j = i+1, j-1 {
				mids[i], mids[j] = mids[j], mids[i]
			}
		}
		ts = append(ts, mids...)
		ts = append(ts, t1)
	case *bspCurve:
		br := c.breaks(t0, t1)
		var init []float64
		for i := 0; i+1 < len(br); i++ {
			sub := uniform(br[i], br[i+1], max(2, c.p))
			if i > 0 {
				sub = sub[1:]
			}
			init = append(init, sub...)
			if len(init) > maxEdgePoints/4 {
				break
			}
		}
		if init[len(init)-1] != br[len(br)-1] {
			init = append(init, br[len(br)-1])
		}
		if t1 < t0 {
			for i, j := 0, len(init)-1; i < j; i, j = i+1, j-1 {
				init[i], init[j] = init[j], init[i]
			}
		}
		ts = adaptive(e.c, init, e.defl, e.ang)
	default:
		ts = adaptive(e.c, uniform(t0, t1, 8), e.defl, e.ang)
	}
	pts := make([]vec3, len(ts))
	for i, t := range ts {
		pts[i] = e.c.at(t)
		if !pts[i].finite() {
			e.ok = false
			return
		}
	}
	pts[0] = e.v0
	pts[len(pts)-1] = e.v1
	e.pts, e.ts = pts, ts
}

func uniform(a, b float64, n int) []float64 {
	if n < 1 {
		n = 1
	}
	out := make([]float64, n+1)
	for i := 0; i <= n; i++ {
		out[i] = a + (b-a)*float64(i)/float64(n)
	}
	out[n] = b
	return out
}

func (e *edgeGeom) paramRange() (float64, float64, bool) {
	c := e.c
	t0, ok0 := invertCurve(c, e.v0)
	t1, ok1 := invertCurve(c, e.v1)
	if !ok0 || !ok1 || !finite(t0) || !finite(t1) {
		return 0, 0, false
	}
	per := c.period()
	if per > 0 {
		if e.closed {
			if e.sense {
				return t0, t0 + per, true
			}
			return t0, t0 - per, true
		}
		eps := 1e-12 * per
		if e.sense {
			for t1 <= t0+eps {
				t1 += per
			}
			for t1 > t0+per {
				t1 -= per
			}
		} else {
			for t1 >= t0-eps {
				t1 -= per
			}
			for t1 < t0-per {
				t1 += per
			}
		}
		return t0, t1, true
	}
	lo, hi := c.rng()
	bounded := finite(lo) && finite(hi)
	closedCurve := bounded && dist(c.at(lo), c.at(hi)) <= 1e-7*(1+e.v0.norm())+1e-3*e.defl
	if e.closed {
		if !closedCurve {
			return t0, t0, true
		}
		if e.sense {
			return lo, hi, true
		}
		return hi, lo, true
	}
	if closedCurve {
		span := hi - lo
		near := func(t, x float64) bool { return math.Abs(t-x) <= 1e-9*span }
		if e.sense {
			if near(t0, hi) {
				t0 = lo
			}
			if near(t1, lo) {
				t1 = hi
			}
		} else {
			if near(t0, lo) {
				t0 = hi
			}
			if near(t1, hi) {
				t1 = lo
			}
		}
	}
	return t0, t1, true
}

func segDist(p, a, b vec3) float64 {
	d := b.sub(a)
	l2 := d.dot(d)
	if !(l2 > 0) {
		return dist(p, a)
	}
	s := p.sub(a).dot(d) / l2
	s = math.Max(0, math.Min(1, s))
	return dist(p, a.add(d.mul(s)))
}

func angleBetween(a, b vec3) float64 {
	la, lb := a.norm(), b.norm()
	if !(la > 1e-300) || !(lb > 1e-300) {
		return 0
	}
	c := a.dot(b) / (la * lb)
	return math.Acos(math.Max(-1, math.Min(1, c)))
}

func adaptive(c curve, init []float64, defl, ang float64) []float64 {
	if len(init) < 2 {
		return init
	}
	type node struct {
		a, b   float64
		pa, pb vec3
		da, db vec3
		depth  int
	}
	out := make([]float64, 0, len(init)*2)
	out = append(out, init[0])
	pa, da := c.d1(init[0])
	stack := make([]node, 0, 64)
	for i := 1; i < len(init); i++ {
		pb, db := c.d1(init[i])
		stack = append(stack[:0], node{a: init[i-1], b: init[i], pa: pa, pb: pb, da: da, db: db})
		for len(stack) > 0 {
			nd := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			split := false
			var pm, dm vec3
			tm := (nd.a + nd.b) / 2
			if nd.depth < maxAdaptDepth && len(out) < maxEdgePoints {
				pm, dm = c.d1(tm)
				if segDist(pm, nd.pa, nd.pb) > defl || angleBetween(nd.da, nd.db) > ang {
					split = true
				} else if segDist(c.at((nd.a+tm)/2), nd.pa, nd.pb) > defl || segDist(c.at((tm+nd.b)/2), nd.pa, nd.pb) > defl {
					split = true
				}
			}
			if split {
				stack = append(stack,
					node{a: tm, b: nd.b, pa: pm, pb: nd.pb, da: dm, db: nd.db, depth: nd.depth + 1},
					node{a: nd.a, b: tm, pa: nd.pa, pb: pm, da: nd.da, db: dm, depth: nd.depth + 1})
				continue
			}
			out = append(out, nd.b)
		}
		pa, da = pb, db
	}
	return out
}
