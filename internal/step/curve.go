package step

import (
	"math"
)

const maxDegree = 25

type curve interface {
	at(t float64) vec3
	d1(t float64) (vec3, vec3)
	period() float64
	rng() (float64, float64)
}

type lineCurve struct {
	o, d vec3
}

func (c *lineCurve) at(t float64) vec3         { return c.o.add(c.d.mul(t)) }
func (c *lineCurve) d1(t float64) (vec3, vec3) { return c.at(t), c.d }
func (c *lineCurve) period() float64           { return 0 }
func (c *lineCurve) rng() (float64, float64)   { return math.Inf(-1), math.Inf(1) }

type circleCurve struct {
	f frame
	r float64
}

func (c *circleCurve) at(t float64) vec3 {
	s, co := math.Sincos(t)
	return c.f.at(c.r*co, c.r*s, 0)
}

func (c *circleCurve) d1(t float64) (vec3, vec3) {
	s, co := math.Sincos(t)
	return c.f.at(c.r*co, c.r*s, 0), c.f.x.mul(-c.r * s).add(c.f.y.mul(c.r * co))
}

func (c *circleCurve) period() float64         { return 2 * math.Pi }
func (c *circleCurve) rng() (float64, float64) { return 0, 2 * math.Pi }

type ellipseCurve struct {
	f    frame
	a, b float64
}

func (c *ellipseCurve) at(t float64) vec3 {
	s, co := math.Sincos(t)
	return c.f.at(c.a*co, c.b*s, 0)
}

func (c *ellipseCurve) d1(t float64) (vec3, vec3) {
	s, co := math.Sincos(t)
	return c.f.at(c.a*co, c.b*s, 0), c.f.x.mul(-c.a * s).add(c.f.y.mul(c.b * co))
}

func (c *ellipseCurve) period() float64         { return 2 * math.Pi }
func (c *ellipseCurve) rng() (float64, float64) { return 0, 2 * math.Pi }

type hyperbolaCurve struct {
	f    frame
	a, b float64
}

func (c *hyperbolaCurve) at(t float64) vec3 {
	return c.f.at(c.a*math.Cosh(t), c.b*math.Sinh(t), 0)
}

func (c *hyperbolaCurve) d1(t float64) (vec3, vec3) {
	return c.at(t), c.f.x.mul(c.a * math.Sinh(t)).add(c.f.y.mul(c.b * math.Cosh(t)))
}

func (c *hyperbolaCurve) period() float64         { return 0 }
func (c *hyperbolaCurve) rng() (float64, float64) { return math.Inf(-1), math.Inf(1) }

type parabolaCurve struct {
	f  frame
	fl float64
}

func (c *parabolaCurve) at(t float64) vec3 {
	return c.f.at(c.fl*t*t, 2*c.fl*t, 0)
}

func (c *parabolaCurve) d1(t float64) (vec3, vec3) {
	return c.at(t), c.f.x.mul(2 * c.fl * t).add(c.f.y.mul(2 * c.fl))
}

func (c *parabolaCurve) period() float64         { return 0 }
func (c *parabolaCurve) rng() (float64, float64) { return math.Inf(-1), math.Inf(1) }

type polyCurve struct {
	pts []vec3
}

func (c *polyCurve) seg(t float64) (int, float64) {
	n := len(c.pts) - 1
	if !(t > 0) {
		return 0, 0
	}
	if t >= float64(n) {
		return n - 1, 1
	}
	i := int(t)
	return i, t - float64(i)
}

func (c *polyCurve) at(t float64) vec3 {
	i, s := c.seg(t)
	return c.pts[i].add(c.pts[i+1].sub(c.pts[i]).mul(s))
}

func (c *polyCurve) d1(t float64) (vec3, vec3) {
	i, s := c.seg(t)
	d := c.pts[i+1].sub(c.pts[i])
	return c.pts[i].add(d.mul(s)), d
}

func (c *polyCurve) period() float64         { return 0 }
func (c *polyCurve) rng() (float64, float64) { return 0, float64(len(c.pts) - 1) }

type bspCurve struct {
	p      int
	cp     []vec3
	w      []float64
	kn     []float64
	lo, hi float64
}

func findSpan(kn []float64, n, p int, t float64) int {
	if t >= kn[n+1] {
		s := n
		for s > p && !(kn[s] < kn[s+1]) {
			s--
		}
		return s
	}
	if t <= kn[p] {
		s := p
		for s < n && !(kn[s] < kn[s+1]) {
			s++
		}
		return s
	}
	lo, hi := p, n+1
	for hi-lo > 1 {
		mid := (lo + hi) / 2
		if t < kn[mid] {
			hi = mid
		} else {
			lo = mid
		}
	}
	return lo
}

func basisFuns(kn []float64, span, p int, t float64, N *[maxDegree + 1]float64) {
	var left, right [maxDegree + 1]float64
	N[0] = 1
	for j := 1; j <= p; j++ {
		left[j] = t - kn[span+1-j]
		right[j] = kn[span+j] - t
		saved := 0.0
		for r := 0; r < j; r++ {
			den := right[r+1] + left[j-r]
			tmp := 0.0
			if den != 0 {
				tmp = N[r] / den
			}
			N[r] = saved + right[r+1]*tmp
			saved = left[j-r] * tmp
		}
		N[j] = saved
	}
}

func basisDers(kn []float64, span, p int, t float64, N, D *[maxDegree + 1]float64) {
	basisFuns(kn, span, p, t, N)
	if p == 0 {
		D[0] = 0
		return
	}
	var M [maxDegree + 1]float64
	basisFuns(kn, span, p-1, t, &M)
	fp := float64(p)
	for j := 0; j <= p; j++ {
		i := span - p + j
		a, b := 0.0, 0.0
		if j > 0 {
			if den := kn[i+p] - kn[i]; den > 0 {
				a = M[j-1] / den
			}
		}
		if j < p {
			if den := kn[i+p+1] - kn[i+1]; den > 0 {
				b = M[j] / den
			}
		}
		D[j] = fp * (a - b)
	}
}

func validKnots(kn []float64, ncp, p int) (float64, float64, bool) {
	if p < 1 || p > maxDegree || ncp < p+1 || len(kn) != ncp+p+1 {
		return 0, 0, false
	}
	for i := range kn {
		if !finite(kn[i]) {
			return 0, 0, false
		}
		if i > 0 && kn[i] < kn[i-1] {
			return 0, 0, false
		}
	}
	lo, hi := kn[p], kn[ncp]
	if !(hi > lo) {
		return 0, 0, false
	}
	return lo, hi, true
}

func newBspCurve(p int, cp []vec3, w []float64, kn []float64) (*bspCurve, bool) {
	lo, hi, ok := validKnots(kn, len(cp), p)
	if !ok {
		return nil, false
	}
	for _, q := range cp {
		if !q.finite() {
			return nil, false
		}
	}
	if w != nil {
		if len(w) != len(cp) {
			return nil, false
		}
		uniform := true
		for _, x := range w {
			if !(x > 0) || !finite(x) {
				return nil, false
			}
			if x != w[0] {
				uniform = false
			}
		}
		if uniform {
			w = nil
		}
	}
	return &bspCurve{p: p, cp: cp, w: w, kn: kn, lo: lo, hi: hi}, true
}

func (c *bspCurve) clamp(t float64) float64 {
	if !(t > c.lo) {
		return c.lo
	}
	if t > c.hi {
		return c.hi
	}
	return t
}

func (c *bspCurve) at(t float64) vec3 {
	t = c.clamp(t)
	n := len(c.cp) - 1
	s := findSpan(c.kn, n, c.p, t)
	var N [maxDegree + 1]float64
	basisFuns(c.kn, s, c.p, t, &N)
	var a vec3
	if c.w == nil {
		for j := 0; j <= c.p; j++ {
			q := c.cp[s-c.p+j]
			a[0] += N[j] * q[0]
			a[1] += N[j] * q[1]
			a[2] += N[j] * q[2]
		}
		return a
	}
	wsum := 0.0
	for j := 0; j <= c.p; j++ {
		k := s - c.p + j
		nw := N[j] * c.w[k]
		q := c.cp[k]
		a[0] += nw * q[0]
		a[1] += nw * q[1]
		a[2] += nw * q[2]
		wsum += nw
	}
	return a.mul(1 / wsum)
}

func (c *bspCurve) d1(t float64) (vec3, vec3) {
	t = c.clamp(t)
	n := len(c.cp) - 1
	s := findSpan(c.kn, n, c.p, t)
	var N, D [maxDegree + 1]float64
	basisDers(c.kn, s, c.p, t, &N, &D)
	var a, da vec3
	if c.w == nil {
		for j := 0; j <= c.p; j++ {
			q := c.cp[s-c.p+j]
			a = a.add(q.mul(N[j]))
			da = da.add(q.mul(D[j]))
		}
		return a, da
	}
	wsum, dw := 0.0, 0.0
	for j := 0; j <= c.p; j++ {
		k := s - c.p + j
		q := c.cp[k]
		a = a.add(q.mul(N[j] * c.w[k]))
		da = da.add(q.mul(D[j] * c.w[k]))
		wsum += N[j] * c.w[k]
		dw += D[j] * c.w[k]
	}
	pt := a.mul(1 / wsum)
	return pt, da.sub(pt.mul(dw)).mul(1 / wsum)
}

func (c *bspCurve) period() float64         { return 0 }
func (c *bspCurve) rng() (float64, float64) { return c.lo, c.hi }

func (c *bspCurve) breaks(t0, t1 float64) []float64 {
	a, b := math.Min(t0, t1), math.Max(t0, t1)
	out := []float64{a}
	for i := c.p + 1; i < len(c.cp); i++ {
		k := c.kn[i]
		if k > a && k < b && k > out[len(out)-1] {
			out = append(out, k)
		}
		if len(out) > 4096 {
			break
		}
	}
	return append(out, b)
}

type surfCurve struct {
	s  surface
	c2 curve
}

func (c *surfCurve) at(t float64) vec3 {
	q := c.c2.at(t)
	return c.s.at(q[0], q[1])
}

func (c *surfCurve) d1(t float64) (vec3, vec3) {
	q, dq := c.c2.d1(t)
	p, su, sv := c.s.d1(q[0], q[1])
	return p, su.mul(dq[0]).add(sv.mul(dq[1]))
}

func (c *surfCurve) period() float64         { return c.c2.period() }
func (c *surfCurve) rng() (float64, float64) { return c.c2.rng() }

func wrapAngle(t float64) float64 {
	t = math.Mod(t, 2*math.Pi)
	if t < 0 {
		t += 2 * math.Pi
	}
	return t
}

func invertCurve(c curve, p vec3) (float64, bool) {
	switch k := c.(type) {
	case *lineCurve:
		d2 := k.d.dot(k.d)
		if !(d2 > 0) {
			return 0, false
		}
		return p.sub(k.o).dot(k.d) / d2, true
	case *circleCurve:
		l := k.f.local(p)
		return wrapAngle(math.Atan2(l[1], l[0])), true
	case *ellipseCurve:
		l := k.f.local(p)
		t := wrapAngle(math.Atan2(l[1]/k.b, l[0]/k.a))
		return refineCurve(c, p, t, 0, 0, false), true
	case *hyperbolaCurve:
		l := k.f.local(p)
		return math.Asinh(l[1] / k.b), true
	case *parabolaCurve:
		l := k.f.local(p)
		return l[1] / (2 * k.fl), true
	case *polyCurve:
		best, bt := math.Inf(1), 0.0
		for i := 0; i+1 < len(k.pts); i++ {
			a, b := k.pts[i], k.pts[i+1]
			d := b.sub(a)
			s := 0.0
			if l2 := d.dot(d); l2 > 0 {
				s = math.Max(0, math.Min(1, p.sub(a).dot(d)/l2))
			}
			if e := dist(a.add(d.mul(s)), p); e < best {
				best, bt = e, float64(i)+s
			}
		}
		return bt, true
	}
	lo, hi := c.rng()
	if !finite(lo) || !finite(hi) || !(hi > lo) {
		return 0, false
	}
	n := 64
	if b, ok := c.(*bspCurve); ok {
		n = min(max(16, 4*(len(b.cp)-b.p)), 2048)
	}
	best, bt := math.Inf(1), lo
	for i := 0; i <= n; i++ {
		t := lo + (hi-lo)*float64(i)/float64(n)
		if e := dist(c.at(t), p); e < best {
			best, bt = e, t
		}
	}
	return refineCurve(c, p, bt, lo, hi, true), true
}

func refineCurve(c curve, p vec3, t, lo, hi float64, bounded bool) float64 {
	for it := 0; it < 32; it++ {
		q, d := c.d1(t)
		d2 := d.dot(d)
		if !(d2 > 1e-300) {
			break
		}
		dt := -q.sub(p).dot(d) / d2
		if !finite(dt) {
			break
		}
		if bounded {
			lim := (hi - lo) / 4
			dt = math.Max(-lim, math.Min(lim, dt))
		}
		nt := t + dt
		if bounded {
			nt = math.Max(lo, math.Min(hi, nt))
		}
		if math.Abs(nt-t) <= 1e-15*(1+math.Abs(t)) {
			t = nt
			break
		}
		t = nt
	}
	return t
}

func curveClosed(c curve, tol float64) bool {
	if c.period() > 0 {
		return true
	}
	lo, hi := c.rng()
	if !finite(lo) || !finite(hi) {
		return false
	}
	return dist(c.at(lo), c.at(hi)) <= tol
}

type wrapCurve struct {
	c       curve
	lo, per float64
}

func (w *wrapCurve) wrap(t float64) float64    { return wrapPeriod(t, w.lo, w.per) }
func (w *wrapCurve) at(t float64) vec3         { return w.c.at(w.wrap(t)) }
func (w *wrapCurve) d1(t float64) (vec3, vec3) { return w.c.d1(w.wrap(t)) }
func (w *wrapCurve) period() float64           { return w.per }
func (w *wrapCurve) rng() (float64, float64)   { return w.lo, w.lo + w.per }

func periodicize(c curve) curve {
	if c.period() > 0 {
		return c
	}
	lo, hi := c.rng()
	if !finite(lo) || !finite(hi) || !(hi > lo) {
		return c
	}
	a, b := c.at(lo), c.at(hi)
	size := dist(a, c.at((lo+hi)/2))
	if dist(a, b) <= 1e-7*(1+a.norm())+1e-6*size {
		return &wrapCurve{c: c, lo: lo, per: hi - lo}
	}
	return c
}
