package step

import (
	"math"
	"testing"
)

func (b *sb) revolvedCylinder(kind string, r, h float64) int {
	o, z, x := vec3{}, vec3{0, 0, 1}, vec3{1, 0, 0}
	var lat int
	switch kind {
	case "extrusion":
		lat = b.e("SURFACE_OF_LINEAR_EXTRUSION('',#%d,#%d)", b.circle(o, z, x, r), b.e("VECTOR('',#%d,%s)", b.dir(z), rs(h)))
	case "revolution":
		axis := b.e("AXIS1_PLACEMENT('',#%d,#%d)", b.point(o), b.dir(z))
		lat = b.e("SURFACE_OF_REVOLUTION('',#%d,#%d)", b.line(vec3{r, 0, 0}, z), axis)
	case "offset":
		lat = b.e("OFFSET_SURFACE('',#%d,%s,.F.)", b.e("CYLINDRICAL_SURFACE('',#%d,%s)", b.ax(o, z, x), rs(r-0.5)), rs(0.5))
	}
	bot := b.e("PLANE('',#%d)", b.ax(o, vec3{0, 0, -1}, x))
	top := b.e("PLANE('',#%d)", b.ax(vec3{0, 0, h}, z, x))
	vb := b.vtx(vec3{r, 0, 0})
	vt := b.vtx(vec3{r, 0, h})
	eb := b.edge(vb, vb, b.circle(o, z, x, r), true)
	et := b.edge(vt, vt, b.circle(vec3{0, 0, h}, z, x, r), true)
	es := b.edge(vb, vt, b.line(vec3{r, 0, 0}, z), true)
	loop := b.loop(b.oe(eb, true), b.oe(es, true), b.oe(et, false), b.oe(es, false))
	fl := b.face(lat, true, bnd{loop: loop, orient: true})
	fb := b.face(bot, true, bnd{loop: b.loop(b.oe(eb, false)), orient: true})
	ft := b.face(top, true, bnd{loop: b.loop(b.oe(et, true)), orient: true})
	return b.solid(fl, fb, ft)
}

func TestSweptAndOffsetSurfaces(t *testing.T) {
	r, h := 2.0, 3.0
	for _, kind := range []string{"extrusion", "revolution", "offset"} {
		b := newSB()
		m, _ := readText(t, b.file(b.revolvedCylinder(kind, r, h)), Options{})
		want := math.Pi * r * r * h
		t.Logf("%s: volume %v (%.4f%%) area %v tris %d open %d", kind, m.Volume(), 100*relErr(m.Volume(), want), m.Area(), m.Triangles(), openEdges(m))
		if relErr(m.Volume(), want) > 0.005 || relErr(m.Area(), 2*math.Pi*r*h+2*math.Pi*r*r) > 0.005 || openEdges(m) != 0 || m.Failed != 0 {
			t.Errorf("%s: volume %v want %v open %d failed %d", kind, m.Volume(), want, openEdges(m), m.Failed)
		}
		windingMatchesNormals(t, kind, m.V, m.N)
	}
}

func windingMatchesNormals(t *testing.T, name string, v, n []float32) {
	bad := 0
	for i := 0; i+8 < len(v); i += 9 {
		a := vec3{float64(v[i]), float64(v[i+1]), float64(v[i+2])}
		b := vec3{float64(v[i+3]), float64(v[i+4]), float64(v[i+5])}
		c := vec3{float64(v[i+6]), float64(v[i+7]), float64(v[i+8])}
		g := b.sub(a).cross(c.sub(a))
		if g.norm() < 1e-9 {
			continue
		}
		var s vec3
		for k := 0; k < 3; k++ {
			s = s.add(vec3{float64(n[i+3*k]), float64(n[i+3*k+1]), float64(n[i+3*k+2])})
		}
		if g.dot(s) <= 0 {
			bad++
		}
	}
	if bad != 0 {
		t.Errorf("%s: %d triangles whose winding disagrees with normals", name, bad)
	}
}

func TestNormalsAgreeWithWinding(t *testing.T) {
	for _, c := range solidCases() {
		b := newSB()
		m, _ := readText(t, b.file(c.build(b)), Options{})
		windingMatchesNormals(t, c.name, m.V, m.N)
	}
}

func TestCurveInversion(t *testing.T) {
	f := makeFrame(vec3{1, 2, 3}, vec3{0.3, -0.2, 1}, vec3{1, 0.5, 0})
	kn := []float64{0, 0, 0, 0.3, 0.7, 1, 1, 1}
	bc, _ := newBspCurve(2, []vec3{{0, 0, 0}, {1, 2, 0}, {3, 2, 1}, {4, 0, 2}, {6, 1, 1}}, []float64{1, 0.5, 2, 1, 1}, kn)
	curves := map[string]curve{
		"line":      &lineCurve{o: vec3{1, 1, 1}, d: vec3{2, 0, 1}},
		"circle":    &circleCurve{f: f, r: 3},
		"ellipse":   &ellipseCurve{f: f, a: 4, b: 1.5},
		"hyperbola": &hyperbolaCurve{f: f, a: 0.6, b: 0.9},
		"parabola":  &parabolaCurve{f: f, fl: 0.7},
		"polyline":  &polyCurve{pts: []vec3{{0, 0, 0}, {1, 0, 0}, {1, 2, 0}, {3, 3, 3}}},
		"bspline":   bc,
	}
	for name, c := range curves {
		lo, hi := -1.2, 1.3
		if a, b := c.rng(); finite(a) && finite(b) {
			lo, hi = a, b
		}
		for i := 0; i <= 10; i++ {
			tt := lo + (hi-lo)*float64(i)/10
			p := c.at(tt)
			got, ok := invertCurve(c, p)
			if !ok || dist(c.at(got), p) > 1e-7 {
				t.Errorf("%s: t=%v inverted to %v (%v)", name, tt, got, dist(c.at(got), p))
			}
			if _, d := c.d1(tt); d.norm() > 0 && i > 0 && i < 10 {
				h := 1e-6
				fd := c.at(tt + h).sub(c.at(tt - h)).mul(1 / (2 * h))
				if (name != "polyline" || math.Abs(tt-math.Round(tt)) > 1e-3) && dist(fd, d) > 1e-4*(1+d.norm()) {
					t.Errorf("%s: derivative at %v: %v vs %v", name, tt, d, fd)
				}
			}
		}
	}
}

func TestSurfaceInversion(t *testing.T) {
	f := makeFrame(vec3{1, -2, 0.5}, vec3{0.2, 0.1, 1}, vec3{1, 0, 0})
	bs, _ := newBspSurf(2, 2, 3, 3, []vec3{{0, 0, 0}, {0, 1, 1}, {0, 2, 0}, {1, 0, 1}, {1, 1, 2}, {1, 2, 1}, {2, 0, 0}, {2, 1, 1}, {2, 2, 0}}, []float64{1, 2, 1, 1, 0.5, 1, 1, 2, 1}, []float64{0, 0, 0, 1, 1, 1}, []float64{0, 0, 0, 1, 1, 1})
	surfs := map[string]surface{
		"plane":  &planeSurf{f: f},
		"cyl":    &cylSurf{f: f, r: 2},
		"cone":   &coneSurf{f: f, r: 2, tn: 0.4},
		"sphere": &sphereSurf{f: f, r: 3},
		"torus":  &torusSurf{f: f, rb: 4, rs: 1},
		"extr":   &extrSurf{c: &circleCurve{f: f, r: 2}, dir: vec3{0, 0.3, 2}},
		"rev":    &revSurf{c: &lineCurve{o: vec3{3, 0, 0}, d: vec3{1, 0, 2}}, o: vec3{}, a: vec3{0, 0, 1}},
		"bsp":    bs,
		"offset": &offSurf{b: &sphereSurf{f: f, r: 3}, d: 0.5},
	}
	for name, s := range surfs {
		for i := 1; i < 6; i++ {
			for j := 1; j < 6; j++ {
				u, v := 0.15*float64(i), 0.15*float64(j)
				p := s.at(u, v)
				uv, ok := invertSurf(s, p, [2]float64{}, false)
				if !ok || dist(s.at(uv[0], uv[1]), p) > 1e-6 {
					t.Errorf("%s: (%v,%v) -> %v err %v", name, u, v, uv, dist(s.at(uv[0], uv[1]), p))
				}
				_, su, sv := s.d1(u, v)
				h := 1e-6
				fu := s.at(u+h, v).sub(s.at(u-h, v)).mul(1 / (2 * h))
				fv := s.at(u, v+h).sub(s.at(u, v-h)).mul(1 / (2 * h))
				if name != "offset" && (dist(fu, su) > 1e-4*(1+su.norm()) || dist(fv, sv) > 1e-4*(1+sv.norm())) {
					t.Errorf("%s: derivatives at (%v,%v)", name, u, v)
				}
			}
		}
	}
}
