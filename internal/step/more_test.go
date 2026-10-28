package step

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/mesh"
)

func assemblyText(reversed bool) string {
	b := newSB()
	ctx := b.context()
	solid := b.box(vec3{0, 0, 0}, vec3{1, 2, 3})
	z, x := vec3{0, 0, 1}, vec3{1, 0, 0}
	ax0 := b.ax(vec3{}, z, x)
	child := b.rep(ctx, ax0, solid)
	axA := b.ax(vec3{10, 0, 0}, z, x)
	axB := b.ax(vec3{0, 20, 5}, z, vec3{0, 1, 0})
	parent := b.e("SHAPE_REPRESENTATION('',(#%d,#%d,#%d),#%d)", ax0, axA, axB, ctx)
	pdParent := b.e("PRODUCT_DEFINITION('design','',$,$)")
	pdChild := b.e("PRODUCT_DEFINITION('design','',$,$)")
	b.e("SHAPE_DEFINITION_REPRESENTATION(#%d,#%d)", b.e("PRODUCT_DEFINITION_SHAPE('','',#%d)", pdChild), child)
	b.e("SHAPE_DEFINITION_REPRESENTATION(#%d,#%d)", b.e("PRODUCT_DEFINITION_SHAPE('','',#%d)", pdParent), parent)
	for _, ax := range []int{axA, axB} {
		r1, r2, i1, i2 := child, parent, ax0, ax
		if reversed {
			r1, r2, i1, i2 = parent, child, ax, ax0
		}
		idt := b.e("ITEM_DEFINED_TRANSFORMATION('','',#%d,#%d)", i1, i2)
		rr := b.e("( REPRESENTATION_RELATIONSHIP('','',#%d,#%d) REPRESENTATION_RELATIONSHIP_WITH_TRANSFORMATION(#%d) SHAPE_REPRESENTATION_RELATIONSHIP() )", r1, r2, idt)
		nauo := b.e("NEXT_ASSEMBLY_USAGE_OCCURRENCE('1','p','',#%d,#%d,$)", pdParent, pdChild)
		b.e("CONTEXT_DEPENDENT_SHAPE_REPRESENTATION(#%d,#%d)", rr, b.e("PRODUCT_DEFINITION_SHAPE('','',#%d)", nauo))
	}
	return b.text()
}

func TestAssembly(t *testing.T) {
	for _, rev := range []bool{false, true} {
		m, info := readText(t, assemblyText(rev), Options{})
		want := [6]float32{-2, 0, 0, 11, 21, 8}
		got := [6]float32{m.Min[0], m.Min[1], m.Min[2], m.Max[0], m.Max[1], m.Max[2]}
		for k := range got {
			if math.Abs(float64(got[k]-want[k])) > 1e-4 {
				t.Fatalf("reversed=%v bounds %v want %v", rev, got, want)
			}
		}
		if m.Parts != 2 || info.Solids != 2 || info.Faces != 12 {
			t.Errorf("parts %d info %+v", m.Parts, info)
		}
		if math.Abs(m.Volume()-12) > 1e-4 || openEdges(m) != 0 {
			t.Errorf("volume %v open %d", m.Volume(), openEdges(m))
		}
		txt := assemblyText(rev)
		_, bb, err := Probe(context.Background(), rd{txt}, int64(len(txt)), Options{})
		if err != nil || bb != want {
			t.Errorf("probe %v %v", bb, err)
		}
	}
}

func TestMappedItem(t *testing.T) {
	b := newSB()
	ctx := b.context()
	solid := b.cylinderSolid(1, 2, true)
	z, x := vec3{0, 0, 1}, vec3{1, 0, 0}
	ax0 := b.ax(vec3{}, z, x)
	child := b.rep(ctx, ax0, solid)
	rm := b.e("REPRESENTATION_MAP(#%d,#%d)", ax0, child)
	var items []int
	for i := 0; i < 3; i++ {
		tgt := b.ax(vec3{float64(5 * i), 0, 0}, vec3{1, 0, 0}, vec3{0, 1, 0})
		items = append(items, b.e("MAPPED_ITEM('',#%d,#%d)", rm, tgt))
	}
	b.e("SHAPE_REPRESENTATION('',(%s),#%d)", refList(items), ctx)
	m, _ := readText(t, b.text(), Options{})
	if m.Parts != 3 {
		t.Fatalf("parts %d", m.Parts)
	}
	if math.Abs(float64(m.Min[0])-0) > 1e-3 || math.Abs(float64(m.Max[0])-12) > 1e-3 || math.Abs(float64(m.Max[2])-1) > 1e-2 {
		t.Errorf("bounds %v %v", m.Min, m.Max)
	}
	if relErr(m.Volume(), 3*2*math.Pi) > 0.005 || openEdges(m) != 0 {
		t.Errorf("volume %v open %d", m.Volume(), openEdges(m))
	}
}

func TestColours(t *testing.T) {
	b := newSB()
	s := b.box(vec3{}, vec3{1, 1, 1})
	col := func(r, g, bl float64) int {
		c := b.e("COLOUR_RGB('',%s,%s,%s)", rs(r), rs(g), rs(bl))
		fill := b.e("FILL_AREA_STYLE('',(#%d))", b.e("FILL_AREA_STYLE_COLOUR('',#%d)", c))
		side := b.e("SURFACE_SIDE_STYLE('',(#%d))", b.e("SURFACE_STYLE_FILL_AREA(#%d)", fill))
		return b.e("PRESENTATION_STYLE_ASSIGNMENT((#%d))", b.e("SURFACE_STYLE_USAGE(.BOTH.,#%d)", side))
	}
	b.e("STYLED_ITEM('color',(#%d),#%d)", col(0.2, 0.5, 0.9), s)
	txt := b.file(s)
	face := 0
	for _, l := range b.lines {
		if strings.Contains(l, "ADVANCED_FACE") {
			fmt.Sscanf(l, "#%d", &face)
			break
		}
	}
	m, _ := readText(t, txt, Options{})
	if !m.HasColors() {
		t.Fatal("no colours")
	}
	for i := 0; i < len(m.C); i += 3 {
		if m.C[i] != 51 || m.C[i+1] != 128 || m.C[i+2] != 230 {
			t.Fatalf("colour %v", m.C[i:i+3])
		}
	}
	b2 := newSB()
	s2 := b2.box(vec3{}, vec3{1, 1, 1})
	face = 0
	for _, l := range b2.lines {
		if strings.Contains(l, "ADVANCED_FACE") {
			fmt.Sscanf(l, "#%d", &face)
			break
		}
	}
	c := b2.e("DRAUGHTING_PRE_DEFINED_COLOUR('red')")
	fill := b2.e("FILL_AREA_STYLE('',(#%d))", b2.e("FILL_AREA_STYLE_COLOUR('',#%d)", c))
	side := b2.e("SURFACE_SIDE_STYLE('',(#%d))", b2.e("SURFACE_STYLE_FILL_AREA(#%d)", fill))
	psa := b2.e("PRESENTATION_STYLE_ASSIGNMENT((#%d))", b2.e("SURFACE_STYLE_USAGE(.BOTH.,#%d)", side))
	b2.e("STYLED_ITEM('',(#%d),#%d)", psa, face)
	m2, _ := readText(t, b2.file(s2), Options{})
	red, grey := 0, 0
	for i := 0; i < len(m2.C); i += 3 {
		switch {
		case m2.C[i] == 255 && m2.C[i+1] == 0:
			red++
		case m2.C[i] == defaultGreyByte:
			grey++
		}
	}
	if red != 2 || grey != 10 {
		t.Errorf("red %d grey %d", red, grey)
	}
	m3, _ := readText(t, func() string { b := newSB(); return b.file(b.box(vec3{}, vec3{1, 1, 1})) }(), Options{})
	if m3.C != nil {
		t.Error("colours without styles")
	}
}

func TestFacetedAndTessellated(t *testing.T) {
	b := newSB()
	var pts [8]int
	for i := 0; i < 8; i++ {
		pts[i] = b.point(vec3{float64(i & 1), float64(i >> 1 & 1), float64(i >> 2 & 1)})
	}
	quads := [][4]int{{0, 2, 3, 1}, {4, 5, 7, 6}, {0, 1, 5, 4}, {2, 6, 7, 3}, {0, 4, 6, 2}, {1, 3, 7, 5}}
	var faces []int
	for _, q := range quads {
		pl := b.e("POLY_LOOP('',(#%d,#%d,#%d,#%d))", pts[q[0]], pts[q[1]], pts[q[2]], pts[q[3]])
		faces = append(faces, b.e("FACE('',(#%d))", b.e("FACE_OUTER_BOUND('',#%d,.T.)", pl)))
	}
	sh := b.e("CLOSED_SHELL('',(%s))", refList(faces))
	fb := b.e("FACETED_BREP('',#%d)", sh)
	ctx := b.context()
	b.e("FACETED_BREP_SHAPE_REPRESENTATION('',(#%d),#%d)", fb, ctx)
	m, _ := readText(t, b.text(), Options{})
	if math.Abs(m.Volume()-1) > 1e-6 || math.Abs(m.Area()-6) > 1e-6 || openEdges(m) != 0 {
		t.Errorf("faceted volume %v area %v open %d", m.Volume(), m.Area(), openEdges(m))
	}

	b = newSB()
	var coords []string
	for i := 0; i < 8; i++ {
		coords = append(coords, fmt.Sprintf("(%s,%s,%s)", rs(float64(i&1)), rs(float64(i>>1&1)), rs(float64(i>>2&1))))
	}
	cl := b.e("COORDINATES_LIST('',8,(%s))", strings.Join(coords, ","))
	var tris []string
	for _, q := range quads[:3] {
		tris = append(tris, fmt.Sprintf("(%d,%d,%d)", q[0]+1, q[1]+1, q[2]+1), fmt.Sprintf("(%d,%d,%d)", q[0]+1, q[2]+1, q[3]+1))
	}
	tf := b.e("TRIANGULATED_FACE('',#%d,8,(),$,(),(%s))", cl, strings.Join(tris, ","))
	var strips []string
	for _, q := range quads[3:] {
		strips = append(strips, fmt.Sprintf("(%d,%d,%d,%d)", q[0]+1, q[1]+1, q[3]+1, q[2]+1))
	}
	ctf := b.e("COMPLEX_TRIANGULATED_FACE('',#%d,8,(),$,(),(%s),())", cl, strings.Join(strips, ","))
	ts := b.e("TESSELLATED_SOLID('',(#%d,#%d),$)", tf, ctf)
	ctx = b.context()
	b.e("TESSELLATED_SHAPE_REPRESENTATION('',(#%d),#%d)", ts, ctx)
	m, _ = readText(t, b.text(), Options{})
	if m.Triangles() != 12 || math.Abs(m.Volume()-1) > 1e-6 || openEdges(m) != 0 {
		t.Errorf("tessellated tris %d volume %v open %d", m.Triangles(), m.Volume(), openEdges(m))
	}
}

func bspSurfText(b *sb, pu, pv int, grid [][]vec3, w [][]float64, ku, kv []float64, mu, mv []int) int {
	var rows []string
	for _, r := range grid {
		var ids []int
		for _, p := range r {
			ids = append(ids, b.point(p))
		}
		rows = append(rows, "("+refList(ids)+")")
	}
	ints := func(x []int) string {
		var s []string
		for _, v := range x {
			s = append(s, fmt.Sprint(v))
		}
		return strings.Join(s, ",")
	}
	reals := func(x []float64) string {
		var s []string
		for _, v := range x {
			s = append(s, rs(v))
		}
		return strings.Join(s, ",")
	}
	if w == nil {
		return b.e("B_SPLINE_SURFACE_WITH_KNOTS('',%d,%d,(%s),.UNSPECIFIED.,.F.,.F.,.F.,(%s),(%s),(%s),(%s),.UNSPECIFIED.)",
			pu, pv, strings.Join(rows, ","), ints(mu), ints(mv), reals(ku), reals(kv))
	}
	var wr []string
	for _, r := range w {
		wr = append(wr, "("+reals(r)+")")
	}
	return b.e("( BOUNDED_SURFACE() B_SPLINE_SURFACE(%d,%d,(%s),.UNSPECIFIED.,.F.,.F.,.F.) B_SPLINE_SURFACE_WITH_KNOTS((%s),(%s),(%s),(%s),.UNSPECIFIED.) GEOMETRIC_REPRESENTATION_ITEM() RATIONAL_B_SPLINE_SURFACE((%s)) REPRESENTATION_ITEM('') SURFACE() )",
		pu, pv, strings.Join(rows, ","), ints(mu), ints(mv), reals(ku), reals(kv), strings.Join(wr, ","))
}

func bspCurveText(b *sb, p int, pts []vec3, w []float64, kn []float64, mult []int) int {
	var ids []int
	for _, q := range pts {
		ids = append(ids, b.point(q))
	}
	var ks, ms, ws []string
	for i := range kn {
		ks = append(ks, rs(kn[i]))
		ms = append(ms, fmt.Sprint(mult[i]))
	}
	if w == nil {
		return b.e("B_SPLINE_CURVE_WITH_KNOTS('',%d,(%s),.UNSPECIFIED.,.F.,.F.,(%s),(%s),.UNSPECIFIED.)", p, refList(ids), strings.Join(ms, ","), strings.Join(ks, ","))
	}
	for _, x := range w {
		ws = append(ws, rs(x))
	}
	return b.e("( BOUNDED_CURVE() B_SPLINE_CURVE(%d,(%s),.UNSPECIFIED.,.F.,.F.) B_SPLINE_CURVE_WITH_KNOTS((%s),(%s),.UNSPECIFIED.) CURVE() GEOMETRIC_REPRESENTATION_ITEM() RATIONAL_B_SPLINE_CURVE((%s)) REPRESENTATION_ITEM('') )",
		p, refList(ids), strings.Join(ms, ","), strings.Join(ks, ","), strings.Join(ws, ","))
}

func surfaceModel(b *sb, faces ...int) string {
	sh := b.e("OPEN_SHELL('',(%s))", refList(faces))
	sbsm := b.e("SHELL_BASED_SURFACE_MODEL('',(#%d))", sh)
	ctx := b.context()
	b.e("MANIFOLD_SURFACE_SHAPE_REPRESENTATION('',(#%d),#%d)", sbsm, ctx)
	return b.text()
}

func TestBSplinePatches(t *testing.T) {
	b := newSB()
	p00, p01, p10, p11 := vec3{0, 0, 0}, vec3{0, 3, 0}, vec3{2, 0, 0}, vec3{2.5, 3.5, 0}
	s := bspSurfText(b, 1, 1, [][]vec3{{p00, p01}, {p10, p11}}, nil, []float64{0, 1}, []float64{0, 1}, []int{2, 2}, []int{2, 2})
	v := []int{b.vtx(p00), b.vtx(p10), b.vtx(p11), b.vtx(p01)}
	ps := []vec3{p00, p10, p11, p01}
	var oes []int
	for i := 0; i < 4; i++ {
		e, fwd := b.lineEdge([2]int{v[i], v[(i+1)%4]}, [2]vec3{ps[i], ps[(i+1)%4]})
		oes = append(oes, b.oe(e, fwd))
	}
	f := b.face(s, true, bnd{loop: b.loop(oes...), orient: true})
	m, _ := readText(t, surfaceModel(b, f), Options{})
	want := 0.5*math.Abs((p10[0]-p00[0])*(p01[1]-p00[1])-(p01[0]-p00[0])*(p10[1]-p00[1])) + 0.5*math.Abs((p10[0]-p11[0])*(p01[1]-p11[1])-(p01[0]-p11[0])*(p10[1]-p11[1]))
	if relErr(m.Area(), want) > 1e-9 || m.Failed != 0 {
		t.Errorf("bilinear area %v want %v", m.Area(), want)
	}

	b = newSB()
	r, h := 3.0, 4.0
	w2 := math.Sqrt2 / 2
	grid := [][]vec3{{{r, 0, 0}, {r, 0, h}}, {{r, r, 0}, {r, r, h}}, {{0, r, 0}, {0, r, h}}}
	s = bspSurfText(b, 2, 1, grid, [][]float64{{1, 1}, {w2, w2}, {1, 1}}, []float64{0, 1}, []float64{0, 1}, []int{3, 3}, []int{2, 2})
	va, vb, vc, vd := b.vtx(vec3{r, 0, 0}), b.vtx(vec3{0, r, 0}), b.vtx(vec3{0, r, h}), b.vtx(vec3{r, 0, h})
	eBot := b.edge(va, vb, b.circle(vec3{}, vec3{0, 0, 1}, vec3{1, 0, 0}, r), true)
	eTop := b.edge(vd, vc, bspCurveText(b, 2, []vec3{{r, 0, h}, {r, r, h}, {0, r, h}}, []float64{1, w2, 1}, []float64{0, 1}, []int{3, 3}), true)
	eL := b.edge(vb, vc, b.line(vec3{0, r, 0}, vec3{0, 0, 1}), true)
	eR := b.edge(va, vd, b.line(vec3{r, 0, 0}, vec3{0, 0, 1}), true)
	f = b.face(s, true, bnd{loop: b.loop(b.oe(eBot, true), b.oe(eL, true), b.oe(eTop, false), b.oe(eR, false)), orient: true})
	m, _ = readText(t, surfaceModel(b, f), Options{})
	want = r * math.Pi / 2 * h
	t.Logf("rational quarter cylinder: area %v want %v (%.4f%%) tris %d", m.Area(), want, 100*relErr(m.Area(), want), m.Triangles())
	if relErr(m.Area(), want) > 0.005 || m.Failed != 0 {
		t.Errorf("rational patch area %v want %v failed %d", m.Area(), want, m.Failed)
	}
	for i := 0; i+2 < len(m.V); i += 3 {
		d := math.Hypot(float64(m.V[i]), float64(m.V[i+1]))
		if math.Abs(d-r) > 0.0008*math.Sqrt(2*r*r+h*h)+1e-6 {
			t.Fatalf("vertex off cylinder: %v", d)
		}
	}

	b = newSB()
	cg := [][]vec3{
		{{0, 0, 0}, {0, 2, 1}, {0, 4, 0}},
		{{2, 0, 2}, {2, 2, 3}, {2, 4, 2}},
		{{4, 0, 0}, {4, 2, -1}, {4, 4, 0}},
	}
	s = bspSurfText(b, 2, 2, cg, nil, []float64{0, 1}, []float64{0, 1}, []int{3, 3}, []int{3, 3})
	corner := []vec3{cg[0][0], cg[2][0], cg[2][2], cg[0][2]}
	vv := []int{b.vtx(corner[0]), b.vtx(corner[1]), b.vtx(corner[2]), b.vtx(corner[3])}
	rowsCurves := [][]vec3{
		{cg[0][0], cg[1][0], cg[2][0]},
		{cg[2][0], cg[2][1], cg[2][2]},
		{cg[2][2], cg[1][2], cg[0][2]},
		{cg[0][2], cg[0][1], cg[0][0]},
	}
	oes = nil
	for i := 0; i < 4; i++ {
		c := bspCurveText(b, 2, rowsCurves[i], nil, []float64{0, 1}, []int{3, 3})
		oes = append(oes, b.oe(b.edge(vv[i], vv[(i+1)%4], c, true), true))
	}
	f = b.face(s, true, bnd{loop: b.loop(oes...), orient: true})
	m, _ = readText(t, surfaceModel(b, f), Options{})
	ref, _ := newBspSurf(2, 2, 3, 3, []vec3{cg[0][0], cg[0][1], cg[0][2], cg[1][0], cg[1][1], cg[1][2], cg[2][0], cg[2][1], cg[2][2]}, nil, []float64{0, 0, 0, 1, 1, 1}, []float64{0, 0, 0, 1, 1, 1})
	want = 0
	const n = 400
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			_, su, sv := ref.d1((float64(i)+0.5)/n, (float64(j)+0.5)/n)
			want += su.cross(sv).norm() / (n * n)
		}
	}
	t.Logf("biquadratic patch: area %v want %v (%.4f%%) tris %d", m.Area(), want, 100*relErr(m.Area(), want), m.Triangles())
	if relErr(m.Area(), want) > 0.005 || m.Failed != 0 {
		t.Errorf("curved patch area %v want %v", m.Area(), want)
	}
}

func TestRationalCircleCylinder(t *testing.T) {
	b := newSB()
	r, h := 2.0, 3.0
	w2 := math.Sqrt2 / 2
	circ := func(z float64) int {
		pts := []vec3{{r, 0, z}, {r, r, z}, {0, r, z}, {-r, r, z}, {-r, 0, z}, {-r, -r, z}, {0, -r, z}, {r, -r, z}, {r, 0, z}}
		return bspCurveText(b, 2, pts, []float64{1, w2, 1, w2, 1, w2, 1, w2, 1}, []float64{0, 0.25, 0.5, 0.75, 1}, []int{3, 2, 2, 2, 3})
	}
	o, z, x := vec3{}, vec3{0, 0, 1}, vec3{1, 0, 0}
	cyl := b.e("CYLINDRICAL_SURFACE('',#%d,%s)", b.ax(o, z, x), rs(r))
	bot := b.e("PLANE('',#%d)", b.ax(o, vec3{0, 0, -1}, x))
	top := b.e("PLANE('',#%d)", b.ax(vec3{0, 0, h}, z, x))
	vb := b.vtx(vec3{r, 0, 0})
	vt := b.vtx(vec3{r, 0, h})
	eb := b.edge(vb, vb, circ(0), true)
	et := b.edge(vt, vt, circ(h), true)
	es := b.edge(vb, vt, b.line(vec3{r, 0, 0}, z), true)
	lat := b.face(cyl, true, bnd{loop: b.loop(b.oe(eb, true), b.oe(es, true), b.oe(et, false), b.oe(es, false)), orient: true})
	fb := b.face(bot, true, bnd{loop: b.loop(b.oe(eb, false)), orient: true})
	ft := b.face(top, true, bnd{loop: b.loop(b.oe(et, true)), orient: true})
	m, _ := readText(t, b.file(b.solid(lat, fb, ft)), Options{})
	want := math.Pi * r * r * h
	t.Logf("rational circle cylinder: volume %v want %v (%.4f%%) open %d", m.Volume(), want, 100*relErr(m.Volume(), want), openEdges(m))
	if relErr(m.Volume(), want) > 0.005 || openEdges(m) != 0 || m.Failed != 0 {
		t.Errorf("volume %v want %v open %d failed %d", m.Volume(), want, openEdges(m), m.Failed)
	}
}

func TestFailedFaceIsolated(t *testing.T) {
	b := newSB()
	s := b.box(vec3{}, vec3{1, 1, 1})
	txt := b.file(s)
	txt = strings.Replace(txt, "PLANE(''", "MYSTERY_SURFACE(''", 1)
	m, info := readText(t, txt, Options{})
	if m.Failed != 1 || info.Failed != 1 || m.Triangles() != 10 {
		t.Errorf("failed %d tris %d", m.Failed, m.Triangles())
	}
}

func TestLimitsAndCancel(t *testing.T) {
	b := newSB()
	txt := b.file(b.box(vec3{}, vec3{1, 1, 1}))
	_, _, err := Read(context.Background(), rd{txt}, int64(len(txt)), Options{MaxTriangles: 5})
	if !errors.Is(err, codec.ErrTooLarge) || !errors.Is(err, mesh.ErrTooManyTriangles) {
		t.Errorf("max triangles: %v", err)
	}
	_, _, err = Read(context.Background(), rd{txt}, int64(len(txt)), Options{MaxEntities: 10})
	if !errors.Is(err, codec.ErrTooLarge) {
		t.Errorf("max entities: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err = Read(ctx, rd{txt}, int64(len(txt)), Options{})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("cancel: %v", err)
	}
	_, _, err = Probe(ctx, rd{txt}, int64(len(txt)), Options{})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("probe cancel: %v", err)
	}
	_, _, err = Read(context.Background(), rd{txt}, int64(len(txt))+10, Options{})
	if !errors.Is(err, codec.ErrCorrupt) {
		t.Errorf("short read: %v", err)
	}
}

func TestLexer(t *testing.T) {
	src := "\xEF\xBB\xBF/* lead */ ISO-10303-21;\nHEADER;\nFILE_DESCRIPTION(('a;b'),'2;1');\n" +
		"FILE_NAME('Caf\\X2\\00E9\\X0\\ ''q''','t',(''),(''),'pre','',$);\nFILE_SCHEMA(('CONFIG_CONTROL_DESIGN'));\nENDSEC;\n" +
		"DATA;\n#10=CARTESIAN_POINT('x;y/*no*/',(1.E-07,-0.,5.));/* c ; */\n" +
		"#7 = ( A(1,(2,3)) B('s''t') C(.T.,$,*) );\n#99999999=LENGTH_MEASURE_WITH_UNIT(LENGTH_MEASURE(25.4),#10);\n" +
		"#3=DIRECTION('',(0.,0.,1.));\nENDSEC;\nDATA;\n#4=VECTOR('',#3,2.5E+00);\nENDSEC;\nEND-ISO-10303-21;\n"
	f, err := parseFile(context.Background(), []byte(src), 100)
	if err != nil {
		t.Fatal(err)
	}
	if f.head.name != "Café 'q'" || f.head.schema != "CONFIG_CONTROL_DESIGN" || f.head.system != "pre" {
		t.Errorf("header %+v", f.head)
	}
	if len(f.ents) != 5 || f.sparse == nil {
		t.Fatalf("entities %d sparse %v", len(f.ents), f.sparse != nil)
	}
	m := newModel(f)
	p, ok := m.point(f.lookup(10))
	if !ok || p != (vec3{1e-7, 0, 5}) {
		t.Errorf("point %v %v", p, ok)
	}
	parts := f.parts(f.lookup(7))
	if len(parts) != 3 || parts[1].name != "B" || string(parts[1].args[0].text) != "s''t" || parts[2].args[0].kind != vEnum {
		t.Errorf("parts %+v", parts)
	}
	a := f.args(f.lookup(99999999))
	if x, ok := argNum(a, 0); !ok || x != 25.4 {
		t.Errorf("typed %v", a)
	}
	v, ok := m.vector(f.lookup(4))
	if !ok || v != (vec3{0, 0, 2.5}) {
		t.Errorf("vector %v", v)
	}
	for _, s := range []string{"1.", "-2.5E-3", "+7", "0.000123", "123456789012345678901234567890.", "1E400"} {
		x, ok := parseNum([]byte(s))
		t.Logf("%s -> %v %v", s, x, ok)
	}
	if x, ok := parseNum([]byte("1E400")); ok {
		t.Errorf("overflow accepted %v", x)
	}
}

func TestParallelLexer(t *testing.T) {
	b := newSB()
	var solids []int
	for i := 0; i < 14; i++ {
		for j := 0; j < 14; j++ {
			solids = append(solids, b.box(vec3{float64(3 * i), float64(3 * j), 0}, vec3{1, 2, 3}))
		}
	}
	b.e("PRODUCT('p','name with \n#12 = FAKE(1);\n inside','',())")
	txt := b.file(solids...)
	if len(txt) < 1<<20 {
		t.Fatalf("text only %d bytes", len(txt))
	}
	m, info := readText(t, txt, Options{})
	if m.Parts != 196 || info.Faces != 1176 || math.Abs(m.Volume()-196*6) > 1e-3 || openEdges(m) != 0 {
		t.Errorf("parts %d faces %d volume %v", m.Parts, info.Faces, m.Volume())
	}
	seq := &stepFile{data: []byte(txt), byName: map[string]int32{}}
	f, err := parseFile(context.Background(), []byte(txt), defaultMaxEntities)
	if err != nil {
		t.Fatal(err)
	}
	i := strings.Index(txt, "DATA;") + 5
	if _, err := seq.parseDataSeq(context.Background(), i, len(txt), defaultMaxEntities); err != nil {
		t.Fatal(err)
	}
	if len(seq.ents) != len(f.ents) {
		t.Fatalf("parallel %d sequential %d", len(f.ents), len(seq.ents))
	}
	for k := range f.ents {
		if f.ents[k].id != seq.ents[k].id || f.ents[k].start != seq.ents[k].start || f.name(k) != seq.name(k) {
			t.Fatalf("entity %d differs", k)
		}
	}
}

func TestMixedUnitAssembly(t *testing.T) {
	b := newSB()
	mm := b.context()
	b.unit = "( LENGTH_UNIT() NAMED_UNIT(*) SI_UNIT($,.METRE.) )"
	m := b.context()
	solid := b.box(vec3{0, 0, 0}, vec3{0.001, 0.002, 0.003})
	z, x := vec3{0, 0, 1}, vec3{1, 0, 0}
	axChild := b.ax(vec3{0.01, 0, 0}, z, x)
	child := b.rep(m, axChild, solid)
	axP := b.ax(vec3{100, 0, 0}, z, x)
	parent := b.e("SHAPE_REPRESENTATION('',(#%d),#%d)", axP, mm)
	idt := b.e("ITEM_DEFINED_TRANSFORMATION('','',#%d,#%d)", axChild, axP)
	b.e("( REPRESENTATION_RELATIONSHIP('','',#%d,#%d) REPRESENTATION_RELATIONSHIP_WITH_TRANSFORMATION(#%d) SHAPE_REPRESENTATION_RELATIONSHIP() )", child, parent, idt)
	ms, info := readText(t, b.text(), Options{})
	want := [6]float32{90, 0, 0, 91, 2, 3}
	got := [6]float32{ms.Min[0], ms.Min[1], ms.Min[2], ms.Max[0], ms.Max[1], ms.Max[2]}
	for k := range got {
		if math.Abs(float64(got[k]-want[k])) > 1e-3 {
			t.Fatalf("bounds %v want %v unit %q", got, want, info.Unit)
		}
	}
	if info.Unit != "mm" || math.Abs(ms.Volume()-6) > 1e-3 {
		t.Errorf("unit %q volume %v", info.Unit, ms.Volume())
	}
}
