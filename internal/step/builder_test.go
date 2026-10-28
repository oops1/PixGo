package step

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/oops1/PixGo/internal/mesh"
)

type sb struct {
	lines []string
	n     int
	edges map[[2]int]int
	unit  string
}

func newSB() *sb {
	return &sb{edges: map[[2]int]int{}, unit: "( LENGTH_UNIT() NAMED_UNIT(*) SI_UNIT(.MILLI.,.METRE.) )"}
}

func rs(x float64) string {
	s := strconv.FormatFloat(x, 'E', -1, 64)
	m, e, _ := strings.Cut(s, "E")
	if !strings.Contains(m, ".") {
		m += "."
	}
	return m + "E" + e
}

func (b *sb) e(format string, args ...any) int {
	b.n++
	b.lines = append(b.lines, fmt.Sprintf("#%d = %s;", b.n, fmt.Sprintf(format, args...)))
	return b.n
}

func (b *sb) point(p vec3) int {
	return b.e("CARTESIAN_POINT('',(%s,%s,%s))", rs(p[0]), rs(p[1]), rs(p[2]))
}

func (b *sb) point2(u, v float64) int {
	return b.e("CARTESIAN_POINT('',(%s,%s))", rs(u), rs(v))
}

func (b *sb) dir(d vec3) int {
	return b.e("DIRECTION('',(%s,%s,%s))", rs(d[0]), rs(d[1]), rs(d[2]))
}

func (b *sb) dir2(u, v float64) int {
	return b.e("DIRECTION('',(%s,%s))", rs(u), rs(v))
}

func (b *sb) ax(o, z, x vec3) int {
	return b.e("AXIS2_PLACEMENT_3D('',#%d,#%d,#%d)", b.point(o), b.dir(z), b.dir(x))
}

func (b *sb) ax2(o [2]float64, x [2]float64) int {
	return b.e("AXIS2_PLACEMENT_2D('',#%d,#%d)", b.point2(o[0], o[1]), b.dir2(x[0], x[1]))
}

func (b *sb) vtx(p vec3) int {
	return b.e("VERTEX_POINT('',#%d)", b.point(p))
}

func (b *sb) line(o, d vec3) int {
	l := d.norm()
	return b.e("LINE('',#%d,#%d)", b.point(o), b.e("VECTOR('',#%d,%s)", b.dir(d), rs(l)))
}

func (b *sb) line2(o, d [2]float64) int {
	l := math.Hypot(d[0], d[1])
	return b.e("LINE('',#%d,#%d)", b.point2(o[0], o[1]), b.e("VECTOR('',#%d,%s)", b.dir2(d[0]/l, d[1]/l), rs(l)))
}

func (b *sb) circle(o, z, x vec3, r float64) int {
	return b.e("CIRCLE('',#%d,%s)", b.ax(o, z, x), rs(r))
}

func (b *sb) edge(v0, v1, curve int, sense bool) int {
	return b.e("EDGE_CURVE('',#%d,#%d,#%d,%s)", v0, v1, curve, boolS(sense))
}

func (b *sb) pcurve(surf, c2 int) int {
	ctx := b.e("( GEOMETRIC_REPRESENTATION_CONTEXT(2) PARAMETRIC_REPRESENTATION_CONTEXT() REPRESENTATION_CONTEXT('2D SPACE','') )")
	dr := b.e("DEFINITIONAL_REPRESENTATION('',(#%d),#%d)", c2, ctx)
	return b.e("PCURVE('',#%d,#%d)", surf, dr)
}

func (b *sb) surfCurve(c3 int, pcs ...int) int {
	return b.e("SURFACE_CURVE('',#%d,(%s),.PCURVE_S1.)", c3, refList(pcs))
}

func (b *sb) seamCurve(c3 int, pcs ...int) int {
	return b.e("SEAM_CURVE('',#%d,(%s),.PCURVE_S1.)", c3, refList(pcs))
}

func boolS(v bool) string {
	if v {
		return ".T."
	}
	return ".F."
}

func refList(ids []int) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = "#" + strconv.Itoa(id)
	}
	return strings.Join(parts, ",")
}

func (b *sb) oe(edge int, fwd bool) int {
	return b.e("ORIENTED_EDGE('',*,*,#%d,%s)", edge, boolS(fwd))
}

func (b *sb) loop(oes ...int) int {
	return b.e("EDGE_LOOP('',(%s))", refList(oes))
}

type bnd struct {
	loop   int
	orient bool
	outer  bool
}

func (b *sb) face(surf int, sense bool, bounds ...bnd) int {
	var ids []int
	for _, x := range bounds {
		name := "FACE_BOUND"
		if x.outer {
			name = "FACE_OUTER_BOUND"
		}
		ids = append(ids, b.e("%s('',#%d,%s)", name, x.loop, boolS(x.orient)))
	}
	return b.e("ADVANCED_FACE('',(%s),#%d,%s)", refList(ids), surf, boolS(sense))
}

func (b *sb) solid(faces ...int) int {
	sh := b.e("CLOSED_SHELL('',(%s))", refList(faces))
	return b.e("MANIFOLD_SOLID_BREP('',#%d)", sh)
}

func (b *sb) context() int {
	u := b.e("%s", b.unit)
	return b.e("( GEOMETRIC_REPRESENTATION_CONTEXT(3) GLOBAL_UNIT_ASSIGNED_CONTEXT((#%d)) REPRESENTATION_CONTEXT('Context','3D') )", u)
}

func (b *sb) rep(ctx int, items ...int) int {
	return b.e("ADVANCED_BREP_SHAPE_REPRESENTATION('',(%s),#%d)", refList(items), ctx)
}

func (b *sb) text() string {
	var sbuf bytes.Buffer
	sbuf.WriteString("ISO-10303-21;\nHEADER;\nFILE_DESCRIPTION(('test'),'2;1');\n")
	sbuf.WriteString("FILE_NAME('Generated part','2026-10-28T00:00:00',('a'),('b'),'pp 1.0','Test System','');\n")
	sbuf.WriteString("FILE_SCHEMA(('AUTOMOTIVE_DESIGN { 1 0 10303 214 1 1 1 1 }'));\nENDSEC;\nDATA;\n")
	for _, l := range b.lines {
		sbuf.WriteString(l)
		sbuf.WriteByte('\n')
	}
	sbuf.WriteString("ENDSEC;\nEND-ISO-10303-21;\n")
	return sbuf.String()
}

func (b *sb) file(items ...int) string {
	ctx := b.context()
	b.rep(ctx, items...)
	return b.text()
}

func (b *sb) lineEdge(v [2]int, p [2]vec3) (int, bool) {
	if e, ok := b.edges[v]; ok {
		return e, true
	}
	if e, ok := b.edges[[2]int{v[1], v[0]}]; ok {
		return e, false
	}
	e := b.edge(v[0], v[1], b.line(p[0], p[1].sub(p[0])), true)
	b.edges[v] = e
	return e, true
}

func (b *sb) polyFace(vs []int, ps []vec3) int {
	var n vec3
	for i := range ps {
		p, q := ps[i], ps[(i+1)%len(ps)]
		n[0] += (p[1] - q[1]) * (p[2] + q[2])
		n[1] += (p[2] - q[2]) * (p[0] + q[0])
		n[2] += (p[0] - q[0]) * (p[1] + q[1])
	}
	z, _ := n.unit()
	x, _ := ps[1].sub(ps[0]).unit()
	pl := b.e("PLANE('',#%d)", b.ax(ps[0], z, x))
	var oes []int
	for i := range vs {
		j := (i + 1) % len(vs)
		e, fwd := b.lineEdge([2]int{vs[i], vs[j]}, [2]vec3{ps[i], ps[j]})
		oes = append(oes, b.oe(e, fwd))
	}
	return b.face(pl, true, bnd{loop: b.loop(oes...), orient: true, outer: true})
}

func (b *sb) box(o, s vec3) int {
	var vid [8]int
	var pos [8]vec3
	for i := 0; i < 8; i++ {
		p := vec3{o[0] + float64(i&1)*s[0], o[1] + float64(i>>1&1)*s[1], o[2] + float64(i>>2&1)*s[2]}
		pos[i] = p
		vid[i] = b.vtx(p)
	}
	c := func(x, y, z int) int { return x | y<<1 | z<<2 }
	quads := [][4]int{
		{c(0, 0, 0), c(0, 1, 0), c(1, 1, 0), c(1, 0, 0)},
		{c(0, 0, 1), c(1, 0, 1), c(1, 1, 1), c(0, 1, 1)},
		{c(0, 0, 0), c(1, 0, 0), c(1, 0, 1), c(0, 0, 1)},
		{c(0, 1, 0), c(0, 1, 1), c(1, 1, 1), c(1, 1, 0)},
		{c(0, 0, 0), c(0, 0, 1), c(0, 1, 1), c(0, 1, 0)},
		{c(1, 0, 0), c(1, 1, 0), c(1, 1, 1), c(1, 0, 1)},
	}
	var faces []int
	for _, q := range quads {
		faces = append(faces, b.polyFace([]int{vid[q[0]], vid[q[1]], vid[q[2]], vid[q[3]]}, []vec3{pos[q[0]], pos[q[1]], pos[q[2]], pos[q[3]]}))
	}
	return b.solid(faces...)
}

type revolved struct {
	withPcurves bool
}

func (b *sb) cylinderSolid(r, h float64, pcurves bool) int {
	o := vec3{}
	z := vec3{0, 0, 1}
	x := vec3{1, 0, 0}
	cyl := b.e("CYLINDRICAL_SURFACE('',#%d,%s)", b.ax(o, z, x), rs(r))
	bot := b.e("PLANE('',#%d)", b.ax(o, vec3{0, 0, -1}, x))
	top := b.e("PLANE('',#%d)", b.ax(vec3{0, 0, h}, z, x))
	vb := b.vtx(vec3{r, 0, 0})
	vt := b.vtx(vec3{r, 0, h})
	cb := b.circle(o, z, x, r)
	ct := b.circle(vec3{0, 0, h}, z, x, r)
	sl := b.line(vec3{r, 0, 0}, vec3{0, 0, 1})
	gb, gt, gs := cb, ct, sl
	if pcurves {
		gb = b.surfCurve(cb, b.pcurve(cyl, b.line2([2]float64{0, 0}, [2]float64{1, 0})), b.pcurve(bot, b.e("CIRCLE('',#%d,%s)", b.ax2([2]float64{0, 0}, [2]float64{1, 0}), rs(r))))
		gt = b.surfCurve(ct, b.pcurve(cyl, b.line2([2]float64{0, h}, [2]float64{1, 0})), b.pcurve(top, b.e("CIRCLE('',#%d,%s)", b.ax2([2]float64{0, 0}, [2]float64{1, 0}), rs(r))))
		gs = b.seamCurve(sl, b.pcurve(cyl, b.line2([2]float64{2 * math.Pi, 0}, [2]float64{0, 1})), b.pcurve(cyl, b.line2([2]float64{0, 0}, [2]float64{0, 1})))
	}
	eb := b.edge(vb, vb, gb, true)
	et := b.edge(vt, vt, gt, true)
	es := b.edge(vb, vt, gs, true)
	lat := b.face(cyl, true, bnd{loop: b.loop(b.oe(eb, true), b.oe(es, true), b.oe(et, false), b.oe(es, false)), orient: true})
	fb := b.face(bot, true, bnd{loop: b.loop(b.oe(eb, false)), orient: true})
	ft := b.face(top, true, bnd{loop: b.loop(b.oe(et, true)), orient: true})
	return b.solid(lat, fb, ft)
}

func (b *sb) coneSolid(r1, h, ang float64) int {
	o := vec3{}
	z := vec3{0, 0, 1}
	x := vec3{1, 0, 0}
	r2 := r1 + h*math.Tan(ang)
	cone := b.e("CONICAL_SURFACE('',#%d,%s,%s)", b.ax(o, z, x), rs(r1), rs(ang))
	bot := b.e("PLANE('',#%d)", b.ax(o, vec3{0, 0, -1}, x))
	top := b.e("PLANE('',#%d)", b.ax(vec3{0, 0, h}, z, x))
	vb := b.vtx(vec3{r1, 0, 0})
	vt := b.vtx(vec3{r2, 0, h})
	eb := b.edge(vb, vb, b.circle(o, z, x, r1), true)
	et := b.edge(vt, vt, b.circle(vec3{0, 0, h}, z, x, r2), true)
	es := b.edge(vb, vt, b.line(vec3{r1, 0, 0}, vec3{r2 - r1, 0, h}), true)
	lat := b.face(cone, true, bnd{loop: b.loop(b.oe(eb, true), b.oe(es, true), b.oe(et, false), b.oe(es, false)), orient: true})
	fb := b.face(bot, true, bnd{loop: b.loop(b.oe(eb, false)), orient: true})
	ft := b.face(top, true, bnd{loop: b.loop(b.oe(et, true)), orient: true})
	return b.solid(lat, fb, ft)
}

func (b *sb) sphereSolid(c vec3, r float64, mode int) int {
	sph := b.e("SPHERICAL_SURFACE('',#%d,%s)", b.ax(c, vec3{0, 0, 1}, vec3{1, 0, 0}), rs(r))
	switch mode {
	case 1:
		return b.solid(b.e("ADVANCED_FACE('',(),#%d,.T.)", sph))
	case 2:
		vs := b.vtx(c.add(vec3{0, 0, -r}))
		vn := b.vtx(c.add(vec3{0, 0, r}))
		l1 := b.e("VERTEX_LOOP('',#%d)", vs)
		l2 := b.e("VERTEX_LOOP('',#%d)", vn)
		return b.solid(b.face(sph, true, bnd{loop: l1, orient: true}, bnd{loop: l2, orient: true}))
	}
	vs := b.vtx(c.add(vec3{0, 0, -r}))
	vn := b.vtx(c.add(vec3{0, 0, r}))
	mer := b.circle(c, vec3{0, -1, 0}, vec3{1, 0, 0}, r)
	e := b.edge(vs, vn, mer, true)
	return b.solid(b.face(sph, true, bnd{loop: b.loop(b.oe(e, true), b.oe(e, false)), orient: true}))
}

func (b *sb) torusSolid(R, r float64, noBounds bool) int {
	o := vec3{}
	tor := b.e("TOROIDAL_SURFACE('',#%d,%s,%s)", b.ax(o, vec3{0, 0, 1}, vec3{1, 0, 0}), rs(R), rs(r))
	if noBounds {
		return b.solid(b.e("ADVANCED_FACE('',(),#%d,.T.)", tor))
	}
	v := b.vtx(vec3{R + r, 0, 0})
	eq := b.edge(v, v, b.circle(o, vec3{0, 0, 1}, vec3{1, 0, 0}, R+r), true)
	tube := b.edge(v, v, b.circle(vec3{R, 0, 0}, vec3{0, -1, 0}, vec3{1, 0, 0}, r), true)
	return b.solid(b.face(tor, true, bnd{loop: b.loop(b.oe(eq, true), b.oe(tube, true), b.oe(eq, false), b.oe(tube, false)), orient: true}))
}

func (b *sb) plateWithHole(w, d, t, cx, cy, r float64) int {
	var vid [8]int
	var pos [8]vec3
	for i := 0; i < 8; i++ {
		p := vec3{float64(i&1) * w, float64(i>>1&1) * d, float64(i>>2&1) * t}
		pos[i] = p
		vid[i] = b.vtx(p)
	}
	c := func(x, y, z int) int { return x | y<<1 | z<<2 }
	side := [][4]int{
		{c(0, 0, 0), c(1, 0, 0), c(1, 0, 1), c(0, 0, 1)},
		{c(0, 1, 0), c(0, 1, 1), c(1, 1, 1), c(1, 1, 0)},
		{c(0, 0, 0), c(0, 0, 1), c(0, 1, 1), c(0, 1, 0)},
		{c(1, 0, 0), c(1, 1, 0), c(1, 1, 1), c(1, 0, 1)},
	}
	var faces []int
	for _, q := range side {
		faces = append(faces, b.polyFace([]int{vid[q[0]], vid[q[1]], vid[q[2]], vid[q[3]]}, []vec3{pos[q[0]], pos[q[1]], pos[q[2]], pos[q[3]]}))
	}
	z := vec3{0, 0, 1}
	x := vec3{1, 0, 0}
	ctr := vec3{cx, cy, 0}
	vb := b.vtx(vec3{cx + r, cy, 0})
	vt := b.vtx(vec3{cx + r, cy, t})
	eb := b.edge(vb, vb, b.circle(ctr, z, x, r), true)
	et := b.edge(vt, vt, b.circle(ctr.add(vec3{0, 0, t}), z, x, r), true)
	es := b.edge(vb, vt, b.line(vec3{cx + r, cy, 0}, z), true)
	cyl := b.e("CYLINDRICAL_SURFACE('',#%d,%s)", b.ax(ctr, z, x), rs(r))
	faces = append(faces, b.face(cyl, false, bnd{loop: b.loop(b.oe(eb, true), b.oe(es, true), b.oe(et, false), b.oe(es, false)), orient: false}))
	quadLoop := func(q [4]int) int {
		var oes []int
		for i := 0; i < 4; i++ {
			j := (i + 1) % 4
			e, fwd := b.lineEdge([2]int{vid[q[i]], vid[q[j]]}, [2]vec3{pos[q[i]], pos[q[j]]})
			oes = append(oes, b.oe(e, fwd))
		}
		return b.loop(oes...)
	}
	bot := b.e("PLANE('',#%d)", b.ax(vec3{}, vec3{0, 0, -1}, x))
	top := b.e("PLANE('',#%d)", b.ax(vec3{0, 0, t}, z, x))
	faces = append(faces, b.face(bot, true,
		bnd{loop: quadLoop([4]int{c(0, 0, 0), c(0, 1, 0), c(1, 1, 0), c(1, 0, 0)}), orient: true, outer: true},
		bnd{loop: b.loop(b.oe(eb, true)), orient: true}))
	faces = append(faces, b.face(top, true,
		bnd{loop: quadLoop([4]int{c(0, 0, 1), c(1, 0, 1), c(1, 1, 1), c(0, 1, 1)}), orient: true, outer: true},
		bnd{loop: b.loop(b.oe(et, false)), orient: true}))
	return b.solid(faces...)
}

type rd struct{ s string }

func (r rd) ReadAt(p []byte, off int64) (int, error) {
	if off >= int64(len(r.s)) {
		return 0, fmt.Errorf("eof")
	}
	n := copy(p, r.s[off:])
	if n < len(p) {
		return n, fmt.Errorf("eof")
	}
	return n, nil
}

func readText(t testing.TB, s string, o Options) (*mesh.Mesh, Info) {
	t.Helper()
	m, info, err := Read(context.Background(), rd{s}, int64(len(s)), o)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	return m, info
}

func openEdges(m *mesh.Mesh) int {
	type key [6]float32
	cnt := map[key]int{}
	nt := m.Triangles()
	for t := 0; t < nt; t++ {
		for k := 0; k < 3; k++ {
			a := m.V[t*9+k*3 : t*9+k*3+3]
			b := m.V[t*9+(k+1)%3*3 : t*9+(k+1)%3*3+3]
			var kk key
			if a[0] < b[0] || a[0] == b[0] && (a[1] < b[1] || a[1] == b[1] && a[2] < b[2]) {
				kk = key{a[0], a[1], a[2], b[0], b[1], b[2]}
			} else {
				kk = key{b[0], b[1], b[2], a[0], a[1], a[2]}
			}
			cnt[kk]++
		}
	}
	open := 0
	for _, c := range cnt {
		if c != 2 {
			open++
		}
	}
	return open
}

func relErr(got, want float64) float64 {
	return math.Abs(got-want) / math.Abs(want)
}
