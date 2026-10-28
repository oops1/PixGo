package step

import (
	"math"
)

type edgeGeom struct {
	ei     int
	c      curve
	v0, v1 vec3
	closed bool
	sense  bool
	pcs    []pcurve
	pts    []vec3
	ts     []float64
	ok     bool
	defl   float64
	ang    float64
	set    bool
}

type edgeUse struct {
	e    *edgeGeom
	fwd  bool
	poly []vec3
}

type loopGeom struct {
	uses []edgeUse
}

type triSet struct {
	pos  []vec3
	nrm  []vec3
	tris [][3]int32
}

type faceGeom struct {
	ei    int
	surf  surface
	sid   int
	sense bool
	loops []loopGeom
	tri   *triSet
	bad   bool
	defl  float64
	ang   float64
	v     []float32
	n     []float32
	fail  bool
}

func (m *model) vertexPoint(ei int) (vec3, bool) {
	if m.f.name(ei) != "VERTEX_POINT" {
		return vec3{}, false
	}
	return m.point(m.ref(m.f.args(ei), 1))
}

func (m *model) edge(ei int, cache map[int]*edgeGeom) *edgeGeom {
	if e, ok := cache[ei]; ok {
		return e
	}
	e := &edgeGeom{ei: ei}
	cache[ei] = e
	if m.f.name(ei) != "EDGE_CURVE" {
		return e
	}
	a := m.f.args(ei)
	va, vb := m.ref(a, 1), m.ref(a, 2)
	p0, ok0 := m.vertexPoint(va)
	p1, ok1 := m.vertexPoint(vb)
	if !ok0 || !ok1 {
		return e
	}
	e.v0, e.v1 = p0, p1
	e.closed = va == vb || p0 == p1
	e.sense = argBool(a, 4, true)
	g := m.ref(a, 3)
	e.c = m.curve(g, 0)
	switch m.f.name(g) {
	case "SURFACE_CURVE", "SEAM_CURVE", "INTERSECTION_CURVE", "BOUNDED_SURFACE_CURVE":
		for _, pi := range m.refs(m.f.args(g), 2) {
			if pc := m.pcurve(pi, 0); pc.c != nil {
				e.pcs = append(e.pcs, pc)
			}
		}
	case "PCURVE":
		if pc := m.pcurve(g, 0); pc.c != nil {
			e.pcs = append(e.pcs, pc)
		}
	}
	e.ok = e.c != nil
	return e
}

func (m *model) orientedEdge(ei int, depth int) (int, bool, bool) {
	if depth > maxTopoDepth || ei < 0 {
		return -1, false, false
	}
	switch m.f.name(ei) {
	case "ORIENTED_EDGE":
		a := m.f.args(ei)
		inner, fwd, ok := m.orientedEdge(m.ref(a, 3), depth+1)
		if !ok {
			return -1, false, false
		}
		return inner, fwd == argBool(a, 4, true), true
	case "EDGE_CURVE":
		return ei, true, true
	}
	return -1, false, false
}

func (m *model) resolveFace(ei int, cache map[int]*edgeGeom) *faceGeom {
	fg := &faceGeom{ei: ei, sense: true}
	f := m.f
	nm := f.name(ei)
	switch nm {
	case "TRIANGULATED_FACE", "COMPLEX_TRIANGULATED_FACE", "TRIANGULATED_SURFACE_SET", "COMPLEX_TRIANGULATED_SURFACE_SET":
		fg.tri = m.triangulated(ei)
		if fg.tri == nil {
			fg.bad = true
		}
		return fg
	case "ADVANCED_FACE", "FACE_SURFACE", "FACE":
	default:
		fg.bad = true
		return fg
	}
	a := f.args(ei)
	if a == nil {
		fg.bad = true
		return fg
	}
	if nm != "FACE" {
		fg.sid = m.ref(a, 2)
		fg.surf = m.surface(fg.sid, 0)
		fg.sense = argBool(a, 3, true)
		if fg.surf == nil {
			fg.bad = true
			return fg
		}
	}
	for _, b := range m.refs(a, 1) {
		bn := f.name(b)
		if bn != "FACE_BOUND" && bn != "FACE_OUTER_BOUND" {
			continue
		}
		ba := f.args(b)
		orient := argBool(ba, 2, true)
		lp := m.ref(ba, 1)
		var lg loopGeom
		switch f.name(lp) {
		case "EDGE_LOOP":
			for _, oe := range m.refs(f.args(lp), 1) {
				e, fwd, ok := m.orientedEdge(oe, 0)
				if !ok {
					fg.bad = true
					return fg
				}
				lg.uses = append(lg.uses, edgeUse{e: m.edge(e, cache), fwd: fwd})
			}
		case "POLY_LOOP":
			pts, ok := m.pointList(listArg(f.args(lp), 1))
			if !ok || len(pts) < 3 {
				fg.bad = true
				return fg
			}
			pts = append(pts, pts[0])
			lg.uses = append(lg.uses, edgeUse{fwd: true, poly: pts})
		case "VERTEX_LOOP":
			continue
		default:
			fg.bad = true
			return fg
		}
		if len(lg.uses) == 0 {
			continue
		}
		if !orient {
			n := len(lg.uses)
			r := make([]edgeUse, n)
			for i, u := range lg.uses {
				u.fwd = !u.fwd
				r[n-1-i] = u
			}
			lg.uses = r
		}
		fg.loops = append(fg.loops, lg)
	}
	if fg.surf == nil {
		pl, ok := newellPlane(fg.loops)
		if !ok {
			fg.bad = true
			return fg
		}
		fg.surf = pl
		fg.sid = -1
	}
	return fg
}

func listArg(a []value, i int) []value {
	l, _ := argList(a, i)
	return l
}

func newellPlane(loops []loopGeom) (surface, bool) {
	best := -1.0
	var bn, bc vec3
	for _, l := range loops {
		var pts []vec3
		for _, u := range l.uses {
			if u.poly != nil {
				pts = append(pts, u.poly[:len(u.poly)-1]...)
			} else if u.e != nil {
				pts = append(pts, u.e.v0)
			}
		}
		if len(pts) < 3 {
			continue
		}
		var n, c vec3
		for i := range pts {
			p, q := pts[i], pts[(i+1)%len(pts)]
			n[0] += (p[1] - q[1]) * (p[2] + q[2])
			n[1] += (p[2] - q[2]) * (p[0] + q[0])
			n[2] += (p[0] - q[0]) * (p[1] + q[1])
			c = c.add(p)
		}
		if l := n.norm(); l > best {
			best, bn, bc = l, n, c.mul(1/float64(len(pts)))
		}
	}
	z, ok := bn.unit()
	if !ok {
		return nil, false
	}
	return &planeSurf{f: makeFrame(bc, z, anyPerp(z))}, true
}

func (m *model) triangulated(ei int) *triSet {
	f := m.f
	a := f.args(ei)
	nm := f.name(ei)
	cl := m.ref(a, 1)
	if f.name(cl) != "COORDINATES_LIST" {
		return nil
	}
	ca := f.args(cl)
	cl2, ok := argList(ca, 2)
	if !ok || len(cl2) == 0 || len(cl2) > 1<<24 {
		return nil
	}
	pos := make([]vec3, len(cl2))
	for i, v := range cl2 {
		if v.kind != vList || len(v.list) != 3 {
			return nil
		}
		xs, ok := numList(v.list)
		if !ok {
			return nil
		}
		pos[i] = vec3{xs[0], xs[1], xs[2]}
	}
	ts := &triSet{pos: pos}
	nl, _ := argList(a, 3)
	pnIdx := 5
	triIdx := 6
	if nm == "TRIANGULATED_SURFACE_SET" || nm == "COMPLEX_TRIANGULATED_SURFACE_SET" {
		pnIdx, triIdx = 4, 5
	}
	pnl, _ := argList(a, pnIdx)
	var pn []int
	for _, v := range pnl {
		x, ok := num(v)
		if !ok || x < 1 || x > float64(len(pos)) || x != math.Trunc(x) {
			return nil
		}
		pn = append(pn, int(x)-1)
	}
	count := len(pos)
	if len(pn) > 0 {
		count = len(pn)
	}
	resolve := func(v value) (int32, bool) {
		x, ok := num(v)
		if !ok || x < 1 || x > float64(count) || x != math.Trunc(x) {
			return 0, false
		}
		k := int(x) - 1
		if len(pn) > 0 {
			k = pn[k]
		}
		return int32(k), true
	}
	if len(nl) == count && count > 0 {
		ts.nrm = make([]vec3, len(pos))
		for i, v := range nl {
			if v.kind != vList || len(v.list) != 3 {
				ts.nrm = nil
				break
			}
			xs, ok := numList(v.list)
			if !ok {
				ts.nrm = nil
				break
			}
			k := i
			if len(pn) > 0 {
				k = pn[i]
			}
			ts.nrm[k] = vec3{xs[0], xs[1], xs[2]}
		}
	}
	add := func(x, y, z value) bool {
		i, ok1 := resolve(x)
		j, ok2 := resolve(y)
		k, ok3 := resolve(z)
		if !ok1 || !ok2 || !ok3 {
			return false
		}
		if i == j || j == k || i == k {
			return true
		}
		if len(ts.tris) >= 1<<24 {
			return false
		}
		ts.tris = append(ts.tris, [3]int32{i, j, k})
		return true
	}
	if nm == "TRIANGULATED_FACE" || nm == "TRIANGULATED_SURFACE_SET" {
		tl, ok := argList(a, triIdx)
		if !ok {
			return nil
		}
		for _, t := range tl {
			if t.kind != vList || len(t.list) != 3 {
				return nil
			}
			if !add(t.list[0], t.list[1], t.list[2]) {
				return nil
			}
		}
	} else {
		strips, _ := argList(a, triIdx)
		fans, _ := argList(a, triIdx+1)
		for _, s := range strips {
			if s.kind != vList {
				return nil
			}
			for j := 2; j < len(s.list); j++ {
				var ok bool
				if j%2 == 0 {
					ok = add(s.list[j-2], s.list[j-1], s.list[j])
				} else {
					ok = add(s.list[j-1], s.list[j-2], s.list[j])
				}
				if !ok {
					return nil
				}
			}
		}
		for _, s := range fans {
			if s.kind != vList {
				return nil
			}
			for j := 2; j < len(s.list); j++ {
				if !add(s.list[0], s.list[j-1], s.list[j]) {
					return nil
				}
			}
		}
	}
	if len(ts.tris) == 0 {
		return nil
	}
	return ts
}
