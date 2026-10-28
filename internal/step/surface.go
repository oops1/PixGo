package step

import (
	"math"
)

type surface interface {
	at(u, v float64) vec3
	d1(u, v float64) (vec3, vec3, vec3)
	periods() (float64, float64)
	domain() [4]float64
}

var infDomain = [4]float64{math.Inf(-1), math.Inf(1), math.Inf(-1), math.Inf(1)}

type planeSurf struct{ f frame }

func (s *planeSurf) at(u, v float64) vec3 { return s.f.at(u, v, 0) }
func (s *planeSurf) d1(u, v float64) (vec3, vec3, vec3) {
	return s.f.at(u, v, 0), s.f.x, s.f.y
}
func (s *planeSurf) periods() (float64, float64) { return 0, 0 }
func (s *planeSurf) domain() [4]float64          { return infDomain }

type cylSurf struct {
	f frame
	r float64
}

func (s *cylSurf) at(u, v float64) vec3 {
	sn, cs := math.Sincos(u)
	return s.f.at(s.r*cs, s.r*sn, v)
}
func (s *cylSurf) d1(u, v float64) (vec3, vec3, vec3) {
	sn, cs := math.Sincos(u)
	return s.f.at(s.r*cs, s.r*sn, v), s.f.x.mul(-s.r * sn).add(s.f.y.mul(s.r * cs)), s.f.z
}
func (s *cylSurf) periods() (float64, float64) { return 2 * math.Pi, 0 }
func (s *cylSurf) domain() [4]float64 {
	return [4]float64{0, 2 * math.Pi, math.Inf(-1), math.Inf(1)}
}

type coneSurf struct {
	f  frame
	r  float64
	tn float64
}

func (s *coneSurf) at(u, v float64) vec3 {
	sn, cs := math.Sincos(u)
	rho := s.r + v*s.tn
	return s.f.at(rho*cs, rho*sn, v)
}
func (s *coneSurf) d1(u, v float64) (vec3, vec3, vec3) {
	sn, cs := math.Sincos(u)
	rho := s.r + v*s.tn
	return s.f.at(rho*cs, rho*sn, v),
		s.f.x.mul(-rho * sn).add(s.f.y.mul(rho * cs)),
		s.f.x.mul(s.tn * cs).add(s.f.y.mul(s.tn * sn)).add(s.f.z)
}
func (s *coneSurf) periods() (float64, float64) { return 2 * math.Pi, 0 }
func (s *coneSurf) domain() [4]float64 {
	return [4]float64{0, 2 * math.Pi, math.Inf(-1), math.Inf(1)}
}

type sphereSurf struct {
	f frame
	r float64
}

func (s *sphereSurf) at(u, v float64) vec3 {
	su, cu := math.Sincos(u)
	sv, cv := math.Sincos(v)
	return s.f.at(s.r*cv*cu, s.r*cv*su, s.r*sv)
}
func (s *sphereSurf) d1(u, v float64) (vec3, vec3, vec3) {
	su, cu := math.Sincos(u)
	sv, cv := math.Sincos(v)
	p := s.f.at(s.r*cv*cu, s.r*cv*su, s.r*sv)
	du := s.f.x.mul(-s.r * cv * su).add(s.f.y.mul(s.r * cv * cu))
	dv := s.f.x.mul(-s.r * sv * cu).add(s.f.y.mul(-s.r * sv * su)).add(s.f.z.mul(s.r * cv))
	return p, du, dv
}
func (s *sphereSurf) periods() (float64, float64) { return 2 * math.Pi, 0 }
func (s *sphereSurf) domain() [4]float64 {
	return [4]float64{0, 2 * math.Pi, -math.Pi / 2, math.Pi / 2}
}

type torusSurf struct {
	f      frame
	rb, rs float64
}

func (s *torusSurf) at(u, v float64) vec3 {
	su, cu := math.Sincos(u)
	sv, cv := math.Sincos(v)
	rho := s.rb + s.rs*cv
	return s.f.at(rho*cu, rho*su, s.rs*sv)
}
func (s *torusSurf) d1(u, v float64) (vec3, vec3, vec3) {
	su, cu := math.Sincos(u)
	sv, cv := math.Sincos(v)
	rho := s.rb + s.rs*cv
	p := s.f.at(rho*cu, rho*su, s.rs*sv)
	du := s.f.x.mul(-rho * su).add(s.f.y.mul(rho * cu))
	dv := s.f.x.mul(-s.rs * sv * cu).add(s.f.y.mul(-s.rs * sv * su)).add(s.f.z.mul(s.rs * cv))
	return p, du, dv
}
func (s *torusSurf) periods() (float64, float64) { return 2 * math.Pi, 2 * math.Pi }
func (s *torusSurf) domain() [4]float64 {
	return [4]float64{0, 2 * math.Pi, 0, 2 * math.Pi}
}

type extrSurf struct {
	c   curve
	dir vec3
}

func (s *extrSurf) at(u, v float64) vec3 { return s.c.at(u).add(s.dir.mul(v)) }
func (s *extrSurf) d1(u, v float64) (vec3, vec3, vec3) {
	p, d := s.c.d1(u)
	return p.add(s.dir.mul(v)), d, s.dir
}
func (s *extrSurf) periods() (float64, float64) { return s.c.period(), 0 }
func (s *extrSurf) domain() [4]float64 {
	lo, hi := s.c.rng()
	return [4]float64{lo, hi, math.Inf(-1), math.Inf(1)}
}

type revSurf struct {
	c curve
	o vec3
	a vec3
}

func (s *revSurf) rot(q vec3, sn, cs float64) vec3 {
	par := s.a.mul(q.dot(s.a))
	perp := q.sub(par)
	return par.add(perp.mul(cs)).add(s.a.cross(perp).mul(sn))
}

func (s *revSurf) at(u, v float64) vec3 {
	sn, cs := math.Sincos(u)
	return s.o.add(s.rot(s.c.at(v).sub(s.o), sn, cs))
}

func (s *revSurf) d1(u, v float64) (vec3, vec3, vec3) {
	sn, cs := math.Sincos(u)
	c, dc := s.c.d1(v)
	q := c.sub(s.o)
	p := s.o.add(s.rot(q, sn, cs))
	perp := q.sub(s.a.mul(q.dot(s.a)))
	du := perp.mul(-sn).add(s.a.cross(perp).mul(cs))
	dv := s.rot(dc, sn, cs)
	return p, du, dv
}
func (s *revSurf) periods() (float64, float64) { return 2 * math.Pi, s.c.period() }
func (s *revSurf) domain() [4]float64 {
	lo, hi := s.c.rng()
	return [4]float64{0, 2 * math.Pi, lo, hi}
}

type offSurf struct {
	b surface
	d float64
}

func surfNormal(s surface, u, v float64) (vec3, bool) {
	_, su, sv := s.d1(u, v)
	return su.cross(sv).unit()
}

func (s *offSurf) at(u, v float64) vec3 {
	p, su, sv := s.b.d1(u, v)
	n, ok := su.cross(sv).unit()
	if !ok {
		return p
	}
	return p.add(n.mul(s.d))
}

func (s *offSurf) d1(u, v float64) (vec3, vec3, vec3) {
	p, su, sv := s.b.d1(u, v)
	n, ok := su.cross(sv).unit()
	if ok {
		p = p.add(n.mul(s.d))
	}
	return p, su, sv
}
func (s *offSurf) periods() (float64, float64) { return s.b.periods() }
func (s *offSurf) domain() [4]float64          { return s.b.domain() }

type bspSurf struct {
	pu, pv     int
	nu, nv     int
	cp         []vec3
	w          []float64
	ku, kv     []float64
	dom        [4]float64
	perU, perV float64
}

func newBspSurf(pu, pv, nu, nv int, cp []vec3, w []float64, ku, kv []float64) (*bspSurf, bool) {
	if nu < 1 || nv < 1 || len(cp) != nu*nv {
		return nil, false
	}
	u0, u1, ok := validKnots(ku, nu, pu)
	if !ok {
		return nil, false
	}
	v0, v1, ok := validKnots(kv, nv, pv)
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
	s := &bspSurf{pu: pu, pv: pv, nu: nu, nv: nv, cp: cp, w: w, ku: ku, kv: kv, dom: [4]float64{u0, u1, v0, v1}}
	s.detectClosed()
	return s, true
}

func (s *bspSurf) detectClosed() {
	lo, hi := vec3{math.Inf(1), math.Inf(1), math.Inf(1)}, vec3{math.Inf(-1), math.Inf(-1), math.Inf(-1)}
	for _, q := range s.cp {
		for k := 0; k < 3; k++ {
			lo[k] = math.Min(lo[k], q[k])
			hi[k] = math.Max(hi[k], q[k])
		}
	}
	tol := 1e-6 * math.Max(dist(lo, hi), 1e-9)
	d := s.dom
	cu, cv := true, true
	for i := 0; i <= 6; i++ {
		t := float64(i) / 6
		v := d[2] + (d[3]-d[2])*t
		if dist(s.at(d[0], v), s.at(d[1], v)) > tol {
			cu = false
		}
		u := d[0] + (d[1]-d[0])*t
		if dist(s.at(u, d[2]), s.at(u, d[3])) > tol {
			cv = false
		}
	}
	if cu && !s.degenerateRow(true, d[0]) {
		s.perU = d[1] - d[0]
	}
	if cv && !s.degenerateRow(false, d[2]) {
		s.perV = d[3] - d[2]
	}
}

func (s *bspSurf) degenerateRow(alongV bool, c float64) bool {
	d := s.dom
	var pts []vec3
	for i := 0; i <= 6; i++ {
		t := float64(i) / 6
		if alongV {
			pts = append(pts, s.at(c, d[2]+(d[3]-d[2])*t))
		} else {
			pts = append(pts, s.at(d[0]+(d[1]-d[0])*t, c))
		}
	}
	span := 0.0
	for _, p := range pts {
		span = math.Max(span, dist(p, pts[0]))
	}
	return span < 1e-9*(1+pts[0].norm())
}

func wrapPeriod(t, lo, per float64) float64 {
	if per <= 0 {
		return t
	}
	if t >= lo && t <= lo+per {
		return t
	}
	t = math.Mod(t-lo, per)
	if t < 0 {
		t += per
	}
	return lo + t
}

func (s *bspSurf) norm(u, v float64) (float64, float64) {
	u = wrapPeriod(u, s.dom[0], s.perU)
	v = wrapPeriod(v, s.dom[2], s.perV)
	u = math.Max(s.dom[0], math.Min(s.dom[1], u))
	v = math.Max(s.dom[2], math.Min(s.dom[3], v))
	if math.IsNaN(u) {
		u = s.dom[0]
	}
	if math.IsNaN(v) {
		v = s.dom[2]
	}
	return u, v
}

func (s *bspSurf) at(u, v float64) vec3 {
	u, v = s.norm(u, v)
	su := findSpan(s.ku, s.nu-1, s.pu, u)
	sv := findSpan(s.kv, s.nv-1, s.pv, v)
	var Nu, Nv [maxDegree + 1]float64
	basisFuns(s.ku, su, s.pu, u, &Nu)
	basisFuns(s.kv, sv, s.pv, v, &Nv)
	var a vec3
	wsum := 0.0
	for i := 0; i <= s.pu; i++ {
		row := (su - s.pu + i) * s.nv
		for j := 0; j <= s.pv; j++ {
			k := row + sv - s.pv + j
			b := Nu[i] * Nv[j]
			if s.w != nil {
				b *= s.w[k]
				wsum += b
			}
			q := s.cp[k]
			a[0] += b * q[0]
			a[1] += b * q[1]
			a[2] += b * q[2]
		}
	}
	if s.w != nil {
		return a.mul(1 / wsum)
	}
	return a
}

func (s *bspSurf) d1(u, v float64) (vec3, vec3, vec3) {
	u, v = s.norm(u, v)
	su := findSpan(s.ku, s.nu-1, s.pu, u)
	sv := findSpan(s.kv, s.nv-1, s.pv, v)
	var Nu, Du, Nv, Dv [maxDegree + 1]float64
	basisDers(s.ku, su, s.pu, u, &Nu, &Du)
	basisDers(s.kv, sv, s.pv, v, &Nv, &Dv)
	var a, au, av vec3
	w, wu, wv := 0.0, 0.0, 0.0
	for i := 0; i <= s.pu; i++ {
		row := (su - s.pu + i) * s.nv
		for j := 0; j <= s.pv; j++ {
			k := row + sv - s.pv + j
			wk := 1.0
			if s.w != nil {
				wk = s.w[k]
			}
			b := Nu[i] * Nv[j] * wk
			bu := Du[i] * Nv[j] * wk
			bv := Nu[i] * Dv[j] * wk
			q := s.cp[k]
			for c := 0; c < 3; c++ {
				a[c] += b * q[c]
				au[c] += bu * q[c]
				av[c] += bv * q[c]
			}
			w += b
			wu += bu
			wv += bv
		}
	}
	if s.w == nil {
		return a, au, av
	}
	p := a.mul(1 / w)
	return p, au.sub(p.mul(wu)).mul(1 / w), av.sub(p.mul(wv)).mul(1 / w)
}

func (s *bspSurf) periods() (float64, float64) { return s.perU, s.perV }
func (s *bspSurf) domain() [4]float64          { return s.dom }

func invertSurf(s surface, p vec3, hint [2]float64, hasHint bool) ([2]float64, bool) {
	switch k := s.(type) {
	case *planeSurf:
		l := k.f.local(p)
		return [2]float64{l[0], l[1]}, true
	case *cylSurf:
		l := k.f.local(p)
		return [2]float64{wrapAngle(math.Atan2(l[1], l[0])), l[2]}, true
	case *coneSurf:
		l := k.f.local(p)
		return [2]float64{wrapAngle(math.Atan2(l[1], l[0])), l[2]}, true
	case *sphereSurf:
		l := k.f.local(p)
		return [2]float64{wrapAngle(math.Atan2(l[1], l[0])), math.Atan2(l[2], math.Hypot(l[0], l[1]))}, true
	case *torusSurf:
		l := k.f.local(p)
		rho := math.Hypot(l[0], l[1]) - k.rb
		return [2]float64{wrapAngle(math.Atan2(l[1], l[0])), wrapAngle(math.Atan2(l[2], rho))}, true
	case *offSurf:
		uv, ok := invertSurf(k.b, p, hint, hasHint)
		if !ok {
			return uv, false
		}
		return newtonSurf(s, p, uv), true
	}
	if hasHint {
		uv := newtonSurf(s, p, hint)
		if q := s.at(uv[0], uv[1]); q.finite() && dist(q, p) < 1e-6*(1+p.norm()) {
			return uv, true
		}
	}
	start, ok := surfStart(s, p)
	if !ok {
		return [2]float64{}, false
	}
	uv := newtonSurf(s, p, start)
	if hasHint {
		h := newtonSurf(s, p, hint)
		if dist(s.at(h[0], h[1]), p) <= dist(s.at(uv[0], uv[1]), p) {
			uv = h
		}
	}
	return uv, true
}

func surfStart(s surface, p vec3) ([2]float64, bool) {
	d := s.domain()
	if r, ok := s.(*revSurf); ok {
		return revStart(r, p)
	}
	if e, ok := s.(*extrSurf); ok {
		lo, hi := e.c.rng()
		if !finite(lo) || !finite(hi) {
			u, ok := invertCurve(e.c, p)
			if !ok {
				return [2]float64{}, false
			}
			d2 := e.dir.dot(e.dir)
			if !(d2 > 0) {
				return [2]float64{}, false
			}
			return [2]float64{u, p.sub(e.c.at(u)).dot(e.dir) / d2}, true
		}
		d2 := e.dir.dot(e.dir)
		if !(d2 > 0) {
			return [2]float64{}, false
		}
		best, bu, bv := math.Inf(1), lo, 0.0
		for i := 0; i <= 128; i++ {
			u := lo + (hi-lo)*float64(i)/128
			c := e.c.at(u)
			v := p.sub(c).dot(e.dir) / d2
			if q := dist(c.add(e.dir.mul(v)), p); q < best {
				best, bu, bv = q, u, v
			}
		}
		return [2]float64{bu, bv}, true
	}
	for _, x := range d {
		if !finite(x) {
			return [2]float64{}, false
		}
	}
	nu, nv := 24, 24
	if b, ok := s.(*bspSurf); ok {
		nu = min(max(12, 3*(b.nu-b.pu)), 96)
		nv = min(max(12, 3*(b.nv-b.pv)), 96)
	}
	best := math.Inf(1)
	var buv [2]float64
	for i := 0; i <= nu; i++ {
		u := d[0] + (d[1]-d[0])*float64(i)/float64(nu)
		for j := 0; j <= nv; j++ {
			v := d[2] + (d[3]-d[2])*float64(j)/float64(nv)
			if q := dist(s.at(u, v), p); q < best {
				best, buv = q, [2]float64{u, v}
			}
		}
	}
	return buv, finite(best)
}

func revStart(r *revSurf, p vec3) ([2]float64, bool) {
	lo, hi := r.c.rng()
	var ts []float64
	if finite(lo) && finite(hi) {
		for i := 0; i <= 96; i++ {
			ts = append(ts, lo+(hi-lo)*float64(i)/96)
		}
	} else {
		t0, ok := invertCurve(r.c, p)
		if !ok {
			return [2]float64{}, false
		}
		_, d := r.c.d1(t0)
		sp := 2 * dist(r.c.at(t0), p) / math.Max(d.norm(), 1e-300)
		if !finite(sp) {
			sp = 1
		}
		for i := -48; i <= 48; i++ {
			ts = append(ts, t0+sp*float64(i)/48)
		}
	}
	q := p.sub(r.o)
	pp := q.sub(r.a.mul(q.dot(r.a)))
	best := math.Inf(1)
	var buv [2]float64
	for _, t := range ts {
		c := r.c.at(t).sub(r.o)
		cp := c.sub(r.a.mul(c.dot(r.a)))
		u := math.Atan2(cp.cross(pp).dot(r.a), cp.dot(pp))
		u = wrapAngle(u)
		if e := dist(r.at(u, t), p); e < best {
			best, buv = e, [2]float64{u, t}
		}
	}
	return buv, finite(best)
}

func newtonSurf(s surface, p vec3, uv [2]float64) [2]float64 {
	d := s.domain()
	pu, pv := s.periods()
	u, v := uv[0], uv[1]
	limU, limV := math.Inf(1), math.Inf(1)
	if finite(d[0]) && finite(d[1]) {
		limU = (d[1] - d[0]) / 4
	}
	if finite(d[2]) && finite(d[3]) {
		limV = (d[3] - d[2]) / 4
	}
	for it := 0; it < 40; it++ {
		q, su, sv := s.d1(u, v)
		r := p.sub(q)
		a11, a12, a22 := su.dot(su), su.dot(sv), sv.dot(sv)
		b1, b2 := su.dot(r), sv.dot(r)
		det := a11*a22 - a12*a12
		var du, dv float64
		if det > 1e-24*(a11*a22+1e-300) {
			du = (b1*a22 - b2*a12) / det
			dv = (a11*b2 - a12*b1) / det
		} else {
			if a11 > 1e-300 {
				du = b1 / a11
			}
			if a22 > 1e-300 {
				dv = b2 / a22
			}
		}
		if !finite(du) || !finite(dv) {
			break
		}
		du = math.Max(-limU, math.Min(limU, du))
		dv = math.Max(-limV, math.Min(limV, dv))
		nu, nv := u+du, v+dv
		if pu <= 0 && finite(d[0]) {
			nu = math.Max(d[0], math.Min(d[1], nu))
		}
		if pv <= 0 && finite(d[2]) {
			nv = math.Max(d[2], math.Min(d[3], nv))
		}
		done := math.Abs(nu-u) <= 1e-14*(1+math.Abs(u)) && math.Abs(nv-v) <= 1e-14*(1+math.Abs(v))
		u, v = nu, nv
		if done {
			break
		}
	}
	return [2]float64{u, v}
}

func singularRows(s surface, dir int) []float64 {
	switch k := s.(type) {
	case *sphereSurf:
		if dir == 0 {
			return []float64{-math.Pi / 2, math.Pi / 2}
		}
		return nil
	case *coneSurf:
		if dir == 0 && k.tn != 0 {
			return []float64{-k.r / k.tn}
		}
		return nil
	case *torusSurf:
		if dir == 0 && k.rs > 0 && math.Abs(k.rb) <= k.rs {
			a := math.Acos(-k.rb / k.rs)
			return []float64{a, 2*math.Pi - a}
		}
		return nil
	case *planeSurf, *cylSurf, *extrSurf:
		return nil
	case *offSurf:
		return singularRows(k.b, dir)
	}
	d := s.domain()
	o := 1 - dir
	var out []float64
	for _, c := range []float64{d[2*o], d[2*o+1]} {
		if !finite(c) || !finite(d[2*dir]) || !finite(d[2*dir+1]) {
			continue
		}
		var pts []vec3
		for i := 0; i <= 8; i++ {
			t := d[2*dir] + (d[2*dir+1]-d[2*dir])*float64(i)/8
			if dir == 0 {
				pts = append(pts, s.at(t, c))
			} else {
				pts = append(pts, s.at(c, t))
			}
		}
		span := 0.0
		for _, p := range pts {
			span = math.Max(span, dist(p, pts[0]))
		}
		if span <= 1e-9*(1+pts[0].norm()) {
			out = append(out, c)
		}
	}
	return out
}

func isAnalytic(s surface) bool {
	switch s.(type) {
	case *planeSurf, *cylSurf, *coneSurf, *sphereSurf, *torusSurf:
		return true
	}
	return false
}
