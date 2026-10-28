package step

import (
	"math"
)

const maxGeomDepth = 12

type model struct {
	f        *stepFile
	pts      map[int]vec3
	curves   map[int]curve
	surfs    map[int]surface
	angleDeg bool
}

func newModel(f *stepFile) *model {
	m := &model{
		f:      f,
		pts:    make(map[int]vec3),
		curves: make(map[int]curve),
		surfs:  make(map[int]surface),
	}
	return m
}

func (m *model) ref(a []value, i int) int {
	id, ok := argRef(a, i)
	if !ok {
		return -1
	}
	return m.f.lookup(id)
}

func (m *model) refs(a []value, i int) []int {
	l, ok := argList(a, i)
	if !ok {
		return nil
	}
	out := make([]int, 0, len(l))
	for _, v := range l {
		if v.kind != vRef {
			continue
		}
		if ei := m.f.lookup(v.ref); ei >= 0 {
			out = append(out, ei)
		}
	}
	return out
}

func (m *model) point(ei int) (vec3, bool) {
	if p, ok := m.pts[ei]; ok {
		return p, p.finite()
	}
	p, ok := m.readPoint(ei)
	if !ok {
		p = vec3{math.NaN(), 0, 0}
	}
	m.pts[ei] = p
	return p, ok
}

func (m *model) readPoint(ei int) (vec3, bool) {
	if m.f.name(ei) != "CARTESIAN_POINT" {
		return vec3{}, false
	}
	a := m.f.args(ei)
	l, ok := argList(a, 1)
	if !ok || len(l) < 1 || len(l) > 3 {
		return vec3{}, false
	}
	var p vec3
	for i, v := range l {
		x, ok := num(v)
		if !ok || !finite(x) {
			return vec3{}, false
		}
		p[i] = x
	}
	return p, true
}

func (m *model) direction(ei int) (vec3, bool) {
	if m.f.name(ei) != "DIRECTION" {
		return vec3{}, false
	}
	a := m.f.args(ei)
	l, ok := argList(a, 1)
	if !ok || len(l) < 1 || len(l) > 3 {
		return vec3{}, false
	}
	var p vec3
	for i, v := range l {
		x, ok := num(v)
		if !ok || !finite(x) {
			return vec3{}, false
		}
		p[i] = x
	}
	return p.unit()
}

func (m *model) vector(ei int) (vec3, bool) {
	switch m.f.name(ei) {
	case "VECTOR":
		a := m.f.args(ei)
		d, ok := m.direction(m.ref(a, 1))
		if !ok {
			return vec3{}, false
		}
		mag, ok := argNum(a, 2)
		if !ok {
			return vec3{}, false
		}
		return d.mul(mag), true
	case "DIRECTION":
		return m.direction(ei)
	}
	return vec3{}, false
}

func (m *model) placement(ei int) (frame, bool) {
	a := m.f.args(ei)
	switch m.f.name(ei) {
	case "AXIS2_PLACEMENT_3D":
		o, ok := m.point(m.ref(a, 1))
		if !ok {
			return frame{}, false
		}
		z := vec3{0, 0, 1}
		if d, ok := m.direction(m.ref(a, 2)); ok {
			z = d
		}
		x := vec3{1, 0, 0}
		if d, ok := m.direction(m.ref(a, 3)); ok {
			x = d
		} else if math.Abs(z[2]) > 0.9 && math.Abs(z[0]) < 0.5 {
			x = vec3{1, 0, 0}
		} else {
			x = anyPerp(z)
		}
		return makeFrame(o, z, x), true
	case "AXIS2_PLACEMENT_2D":
		o, ok := m.point(m.ref(a, 1))
		if !ok {
			return frame{}, false
		}
		x := vec3{1, 0, 0}
		if d, ok := m.direction(m.ref(a, 2)); ok {
			x = vec3{d[0], d[1], 0}
		}
		return makeFrame(o, vec3{0, 0, 1}, x), true
	case "AXIS1_PLACEMENT":
		o, ok := m.point(m.ref(a, 1))
		if !ok {
			return frame{}, false
		}
		z := vec3{0, 0, 1}
		if d, ok := m.direction(m.ref(a, 2)); ok {
			z = d
		}
		return makeFrame(o, z, anyPerp(z)), true
	}
	return frame{}, false
}

func (m *model) transformOp(ei int) (xform, bool) {
	switch m.f.name(ei) {
	case "AXIS2_PLACEMENT_3D", "AXIS2_PLACEMENT_2D":
		f, ok := m.placement(ei)
		if !ok {
			return xform{}, false
		}
		return frameXform(f), true
	}
	a := m.f.argsOf(ei, "CARTESIAN_TRANSFORMATION_OPERATOR")
	off := 0
	if a == nil {
		a = m.f.argsOf(ei, "CARTESIAN_TRANSFORMATION_OPERATOR_3D")
		if a == nil {
			return xform{}, false
		}
		off = 0
	}
	var axes [3]vec3
	var have [3]bool
	if m.f.complex(ei) {
		off = -2
	}
	if d, ok := m.direction(m.ref(a, 2+off)); ok {
		axes[0], have[0] = d, true
	}
	if d, ok := m.direction(m.ref(a, 3+off)); ok {
		axes[1], have[1] = d, true
	}
	o, ok := m.point(m.ref(a, 4+off))
	if !ok {
		return xform{}, false
	}
	scale := 1.0
	if s, ok := argNum(a, 5+off); ok && s > 0 {
		scale = s
	}
	if a3 := m.f.argsOf(ei, "CARTESIAN_TRANSFORMATION_OPERATOR_3D"); a3 != nil {
		k := 6
		if m.f.complex(ei) {
			k = 0
		}
		if d, ok := m.direction(m.ref(a3, k)); ok {
			axes[2], have[2] = d, true
		}
	}
	z := vec3{0, 0, 1}
	if have[2] {
		z = axes[2]
	}
	x := vec3{1, 0, 0}
	if have[0] {
		x = axes[0]
	} else if have[1] {
		x = axes[1].cross(z)
	}
	fr := makeFrame(o, z, x)
	if have[1] && !have[2] && have[0] {
		zz, ok := axes[0].cross(axes[1]).unit()
		if ok {
			fr = makeFrame(o, zz, axes[0])
		}
	}
	t := frameXform(fr)
	if scale != 1 {
		t.m = t.scaled(scale).m
	}
	return t, true
}

func (m *model) curve(ei int, depth int) curve {
	if c, ok := m.curves[ei]; ok {
		return c
	}
	if depth > maxGeomDepth {
		return nil
	}
	c := m.readCurve(ei, depth)
	if c == nil {
		m.curves[ei] = nil
		return nil
	}
	m.curves[ei] = c
	return c
}

func (m *model) readCurve(ei int, depth int) curve {
	if m.f.complex(ei) {
		return m.bspCurve(ei)
	}
	a := m.f.args(ei)
	switch m.f.name(ei) {
	case "LINE":
		o, ok := m.point(m.ref(a, 1))
		if !ok {
			return nil
		}
		d, ok := m.vector(m.ref(a, 2))
		if !ok || !(d.norm() > 0) {
			return nil
		}
		return &lineCurve{o: o, d: d}
	case "CIRCLE":
		f, ok := m.placement(m.ref(a, 1))
		r, ok2 := argNum(a, 2)
		if !ok || !ok2 || !(r > 0) {
			return nil
		}
		return &circleCurve{f: f, r: r}
	case "ELLIPSE":
		f, ok := m.placement(m.ref(a, 1))
		ra, ok2 := argNum(a, 2)
		rb, ok3 := argNum(a, 3)
		if !ok || !ok2 || !ok3 || !(ra > 0) || !(rb > 0) {
			return nil
		}
		return &ellipseCurve{f: f, a: ra, b: rb}
	case "HYPERBOLA":
		f, ok := m.placement(m.ref(a, 1))
		ra, ok2 := argNum(a, 2)
		rb, ok3 := argNum(a, 3)
		if !ok || !ok2 || !ok3 || !(ra > 0) || !(rb > 0) {
			return nil
		}
		return &hyperbolaCurve{f: f, a: ra, b: rb}
	case "PARABOLA":
		f, ok := m.placement(m.ref(a, 1))
		fl, ok2 := argNum(a, 2)
		if !ok || !ok2 || fl == 0 {
			return nil
		}
		return &parabolaCurve{f: f, fl: fl}
	case "POLYLINE":
		ps := m.refs(a, 1)
		if len(ps) < 2 {
			return nil
		}
		pts := make([]vec3, 0, len(ps))
		for _, pi := range ps {
			p, ok := m.point(pi)
			if !ok {
				return nil
			}
			pts = append(pts, p)
		}
		return &polyCurve{pts: pts}
	case "B_SPLINE_CURVE_WITH_KNOTS", "BEZIER_CURVE", "UNIFORM_CURVE", "QUASI_UNIFORM_CURVE", "B_SPLINE_CURVE":
		return m.bspCurve(ei)
	case "TRIMMED_CURVE":
		return m.curve(m.ref(a, 1), depth+1)
	case "SURFACE_CURVE", "SEAM_CURVE", "INTERSECTION_CURVE", "BOUNDED_SURFACE_CURVE":
		if c := m.curve(m.ref(a, 1), depth+1); c != nil {
			return c
		}
		for _, pi := range m.refs(a, 2) {
			if pc := m.pcurve(pi, depth+1); pc.c != nil {
				if s := m.surface(pc.surf, depth+1); s != nil {
					return &surfCurve{s: s, c2: pc.c}
				}
			}
		}
		return nil
	case "PCURVE":
		pc := m.pcurve(ei, depth+1)
		if pc.c == nil {
			return nil
		}
		s := m.surface(pc.surf, depth+1)
		if s == nil {
			return nil
		}
		return &surfCurve{s: s, c2: pc.c}
	}
	return nil
}

type pcurve struct {
	surf int
	c    curve
}

func (m *model) pcurve(ei int, depth int) pcurve {
	if m.f.name(ei) != "PCURVE" {
		return pcurve{}
	}
	a := m.f.args(ei)
	s := m.ref(a, 1)
	dr := m.ref(a, 2)
	if s < 0 || dr < 0 || m.f.name(dr) != "DEFINITIONAL_REPRESENTATION" {
		return pcurve{}
	}
	items := m.refs(m.f.args(dr), 1)
	for _, it := range items {
		if c := m.curve(it, depth+1); c != nil {
			return pcurve{surf: s, c: c}
		}
	}
	return pcurve{}
}

func (m *model) pointList(l []value) ([]vec3, bool) {
	out := make([]vec3, 0, len(l))
	for _, v := range l {
		if v.kind != vRef {
			return nil, false
		}
		p, ok := m.point(m.f.lookup(v.ref))
		if !ok {
			return nil, false
		}
		out = append(out, p)
	}
	return out, true
}

func expandKnots(mults, knots []value, total int) ([]float64, bool) {
	if len(mults) != len(knots) || len(mults) == 0 || total <= 0 {
		return nil, false
	}
	sum := 0
	for _, v := range mults {
		x, ok := num(v)
		if !ok || x < 1 || x > 64 || x != math.Trunc(x) {
			return nil, false
		}
		sum += int(x)
	}
	if sum != total {
		return nil, false
	}
	out := make([]float64, 0, total)
	for i, v := range mults {
		x, _ := num(v)
		k, ok := num(knots[i])
		if !ok || !finite(k) {
			return nil, false
		}
		for j := 0; j < int(x); j++ {
			out = append(out, k)
		}
	}
	return out, true
}

func synthKnots(kind string, ncp, p int) ([]float64, bool) {
	if p < 1 || p > maxDegree || ncp < p+1 || ncp > 1<<20 {
		return nil, false
	}
	n := ncp + p + 1
	out := make([]float64, 0, n)
	switch kind {
	case "UNIFORM_CURVE", "UNIFORM_SURFACE":
		for i := 0; i < n; i++ {
			out = append(out, float64(i-p))
		}
	case "QUASI_UNIFORM_CURVE", "QUASI_UNIFORM_SURFACE":
		last := float64(ncp - p)
		for i := 0; i <= p; i++ {
			out = append(out, 0)
		}
		for k := 1; k < ncp-p; k++ {
			out = append(out, float64(k))
		}
		for i := 0; i <= p; i++ {
			out = append(out, last)
		}
	case "BEZIER_CURVE", "BEZIER_SURFACE":
		if (ncp-1)%p != 0 {
			return nil, false
		}
		seg := (ncp - 1) / p
		for i := 0; i <= p; i++ {
			out = append(out, 0)
		}
		for s := 1; s < seg; s++ {
			for i := 0; i < p; i++ {
				out = append(out, float64(s))
			}
		}
		for i := 0; i <= p; i++ {
			out = append(out, float64(seg))
		}
	default:
		return nil, false
	}
	if len(out) != n {
		return nil, false
	}
	return out, true
}

func (m *model) bspCurve(ei int) curve {
	var deg int
	var cpl []value
	var kn []float64
	var w []float64
	var ok bool
	if !m.f.complex(ei) {
		a := m.f.args(ei)
		deg, ok = argInt(a, 1)
		if !ok {
			return nil
		}
		cpl, ok = argList(a, 2)
		if !ok {
			return nil
		}
		switch m.f.name(ei) {
		case "B_SPLINE_CURVE_WITH_KNOTS":
			mu, ok1 := argList(a, 6)
			ks, ok2 := argList(a, 7)
			if !ok1 || !ok2 {
				return nil
			}
			kn, ok = expandKnots(mu, ks, len(cpl)+deg+1)
		default:
			kn, ok = synthKnots(m.f.name(ei), len(cpl), deg)
		}
		if !ok {
			return nil
		}
	} else {
		parts := m.f.parts(ei)
		var base, knots, rat []value
		kind := ""
		for _, p := range parts {
			switch p.name {
			case "B_SPLINE_CURVE":
				base = p.args
			case "B_SPLINE_CURVE_WITH_KNOTS":
				knots = p.args
				kind = p.name
			case "BEZIER_CURVE", "UNIFORM_CURVE", "QUASI_UNIFORM_CURVE":
				kind = p.name
			case "RATIONAL_B_SPLINE_CURVE":
				rat = p.args
			}
		}
		if base == nil || kind == "" {
			return nil
		}
		deg, ok = argInt(base, 0)
		if !ok {
			return nil
		}
		cpl, ok = argList(base, 1)
		if !ok {
			return nil
		}
		if kind == "B_SPLINE_CURVE_WITH_KNOTS" {
			mu, ok1 := argList(knots, 0)
			ks, ok2 := argList(knots, 1)
			if !ok1 || !ok2 {
				return nil
			}
			kn, ok = expandKnots(mu, ks, len(cpl)+deg+1)
		} else {
			kn, ok = synthKnots(kind, len(cpl), deg)
		}
		if !ok {
			return nil
		}
		if rat != nil {
			wl, ok := argList(rat, 0)
			if !ok {
				return nil
			}
			w, ok = numList(wl)
			if !ok {
				return nil
			}
		}
	}
	if deg < 1 || deg > maxDegree {
		return nil
	}
	cps, ok := m.pointList(cpl)
	if !ok {
		return nil
	}
	c, ok := newBspCurve(deg, cps, w, kn)
	if !ok {
		return nil
	}
	return c
}

func (m *model) surface(ei int, depth int) surface {
	if s, ok := m.surfs[ei]; ok {
		return s
	}
	if depth > maxGeomDepth {
		return nil
	}
	s := m.readSurface(ei, depth)
	m.surfs[ei] = s
	return s
}

func (m *model) readSurface(ei int, depth int) surface {
	if m.f.complex(ei) {
		return m.bspSurface(ei)
	}
	a := m.f.args(ei)
	switch m.f.name(ei) {
	case "PLANE":
		f, ok := m.placement(m.ref(a, 1))
		if !ok {
			return nil
		}
		return &planeSurf{f: f}
	case "CYLINDRICAL_SURFACE":
		f, ok := m.placement(m.ref(a, 1))
		r, ok2 := argNum(a, 2)
		if !ok || !ok2 || !(r > 0) {
			return nil
		}
		return &cylSurf{f: f, r: r}
	case "CONICAL_SURFACE":
		f, ok := m.placement(m.ref(a, 1))
		r, ok2 := argNum(a, 2)
		ang, ok3 := argNum(a, 3)
		if !ok || !ok2 || !ok3 || r < 0 {
			return nil
		}
		if m.angleDeg || math.Abs(ang) >= math.Pi/2 {
			ang *= math.Pi / 180
		}
		if !(math.Abs(ang) < math.Pi/2) {
			return nil
		}
		return &coneSurf{f: f, r: r, tn: math.Tan(ang)}
	case "SPHERICAL_SURFACE":
		f, ok := m.placement(m.ref(a, 1))
		r, ok2 := argNum(a, 2)
		if !ok || !ok2 || !(r > 0) {
			return nil
		}
		return &sphereSurf{f: f, r: r}
	case "TOROIDAL_SURFACE", "DEGENERATE_TOROIDAL_SURFACE":
		f, ok := m.placement(m.ref(a, 1))
		rb, ok2 := argNum(a, 2)
		rs, ok3 := argNum(a, 3)
		if !ok || !ok2 || !ok3 || !(rs > 0) || rb < 0 {
			return nil
		}
		return &torusSurf{f: f, rb: rb, rs: rs}
	case "B_SPLINE_SURFACE_WITH_KNOTS", "BEZIER_SURFACE", "UNIFORM_SURFACE", "QUASI_UNIFORM_SURFACE":
		return m.bspSurface(ei)
	case "SURFACE_OF_LINEAR_EXTRUSION":
		c := m.curve(m.ref(a, 1), depth+1)
		d, ok := m.vector(m.ref(a, 2))
		if c == nil || !ok || !(d.norm() > 0) {
			return nil
		}
		return &extrSurf{c: periodicize(c), dir: d}
	case "SURFACE_OF_REVOLUTION":
		c := m.curve(m.ref(a, 1), depth+1)
		f, ok := m.placement(m.ref(a, 2))
		if c == nil || !ok {
			return nil
		}
		return &revSurf{c: periodicize(c), o: f.o, a: f.z}
	case "OFFSET_SURFACE":
		b := m.surface(m.ref(a, 1), depth+1)
		d, ok := argNum(a, 2)
		if b == nil || !ok {
			return nil
		}
		if d == 0 {
			return b
		}
		return &offSurf{b: b, d: d}
	case "RECTANGULAR_TRIMMED_SURFACE", "CURVE_BOUNDED_SURFACE":
		return m.surface(m.ref(a, 1), depth+1)
	}
	return nil
}

func (m *model) bspSurface(ei int) surface {
	var pu, pv int
	var grid []value
	var ku, kv []float64
	var w []float64
	var muL, mvL, kuL, kvL []value
	kind := ""
	var ok bool
	var rat []value
	if !m.f.complex(ei) {
		a := m.f.args(ei)
		kind = m.f.name(ei)
		pu, ok = argInt(a, 1)
		if !ok {
			return nil
		}
		pv, ok = argInt(a, 2)
		if !ok {
			return nil
		}
		grid, ok = argList(a, 3)
		if !ok {
			return nil
		}
		if kind == "B_SPLINE_SURFACE_WITH_KNOTS" {
			muL, _ = argList(a, 8)
			mvL, _ = argList(a, 9)
			kuL, _ = argList(a, 10)
			kvL, _ = argList(a, 11)
		}
	} else {
		var base []value
		for _, p := range m.f.parts(ei) {
			switch p.name {
			case "B_SPLINE_SURFACE":
				base = p.args
			case "B_SPLINE_SURFACE_WITH_KNOTS":
				kind = p.name
				muL, _ = argList(p.args, 0)
				mvL, _ = argList(p.args, 1)
				kuL, _ = argList(p.args, 2)
				kvL, _ = argList(p.args, 3)
			case "BEZIER_SURFACE", "UNIFORM_SURFACE", "QUASI_UNIFORM_SURFACE":
				kind = p.name
			case "RATIONAL_B_SPLINE_SURFACE":
				rat = p.args
			}
		}
		if base == nil || kind == "" {
			return nil
		}
		pu, ok = argInt(base, 0)
		if !ok {
			return nil
		}
		pv, ok = argInt(base, 1)
		if !ok {
			return nil
		}
		grid, ok = argList(base, 2)
		if !ok {
			return nil
		}
	}
	if pu < 1 || pv < 1 || pu > maxDegree || pv > maxDegree || len(grid) == 0 {
		return nil
	}
	nu := len(grid)
	nv := -1
	cps := make([]vec3, 0)
	for _, row := range grid {
		if row.kind != vList {
			return nil
		}
		if nv < 0 {
			nv = len(row.list)
		} else if nv != len(row.list) {
			return nil
		}
		if nu*nv > 1<<22 {
			return nil
		}
		ps, ok := m.pointList(row.list)
		if !ok {
			return nil
		}
		cps = append(cps, ps...)
	}
	if nv < 1 {
		return nil
	}
	if kind == "B_SPLINE_SURFACE_WITH_KNOTS" {
		ku, ok = expandKnots(muL, kuL, nu+pu+1)
		if !ok {
			return nil
		}
		kv, ok = expandKnots(mvL, kvL, nv+pv+1)
		if !ok {
			return nil
		}
	} else {
		ku, ok = synthKnots(kind, nu, pu)
		if !ok {
			return nil
		}
		kv, ok = synthKnots(kind, nv, pv)
		if !ok {
			return nil
		}
	}
	if rat != nil {
		wl, ok := argList(rat, 0)
		if !ok || len(wl) != nu {
			return nil
		}
		w = make([]float64, 0, nu*nv)
		for _, row := range wl {
			if row.kind != vList || len(row.list) != nv {
				return nil
			}
			r, ok := numList(row.list)
			if !ok {
				return nil
			}
			w = append(w, r...)
		}
	}
	s, ok := newBspSurf(pu, pv, nu, nv, cps, w, ku, kv)
	if !ok {
		return nil
	}
	return s
}
