package step

import (
	"context"
	"math"
	"testing"
)

func fuzzSeeds() []string {
	var out []string
	for _, c := range solidCases() {
		b := newSB()
		out = append(out, b.file(c.build(b)))
	}
	out = append(out, assemblyText(false), assemblyText(true))
	b := newSB()
	s := b.box(vec3{}, vec3{1, 1, 1})
	c := b.e("COLOUR_RGB('',0.1,0.2,0.3)")
	fill := b.e("FILL_AREA_STYLE('',(#%d))", b.e("FILL_AREA_STYLE_COLOUR('',#%d)", c))
	side := b.e("SURFACE_SIDE_STYLE('',(#%d))", b.e("SURFACE_STYLE_FILL_AREA(#%d)", fill))
	b.e("STYLED_ITEM('',(#%d),#%d)", b.e("PRESENTATION_STYLE_ASSIGNMENT((#%d))", b.e("SURFACE_STYLE_USAGE(.BOTH.,#%d)", side)), s)
	out = append(out, b.file(s))
	b = newSB()
	cl := b.e("COORDINATES_LIST('',4,((0.,0.,0.),(1.,0.,0.),(0.,1.,0.),(0.,0.,1.)))")
	tf := b.e("TRIANGULATED_FACE('',#%d,4,(),$,(),((1,3,2),(1,2,4),(2,3,4),(1,4,3)))", cl)
	ct := b.e("COMPLEX_TRIANGULATED_FACE('',#%d,4,(),$,(),((1,2,3,4)),((1,2,3)))", cl)
	ts := b.e("TESSELLATED_SOLID('',(#%d,#%d),$)", tf, ct)
	b.e("TESSELLATED_SHAPE_REPRESENTATION('',(#%d),#%d)", ts, b.context())
	out = append(out, b.text())
	b = newSB()
	p := []vec3{{0, 0, 0}, {0, 2, 1}, {2, 0, 1}, {2, 2, 0}}
	sf := bspSurfText(b, 1, 1, [][]vec3{{p[0], p[1]}, {p[2], p[3]}}, nil, []float64{0, 1}, []float64{0, 1}, []int{2, 2}, []int{2, 2})
	v := []int{b.vtx(p[0]), b.vtx(p[2]), b.vtx(p[3]), b.vtx(p[1])}
	ps := []vec3{p[0], p[2], p[3], p[1]}
	var oes []int
	for i := 0; i < 4; i++ {
		e, fwd := b.lineEdge([2]int{v[i], v[(i+1)%4]}, [2]vec3{ps[i], ps[(i+1)%4]})
		oes = append(oes, b.oe(e, fwd))
	}
	out = append(out, surfaceModel(b, b.face(sf, true, bnd{loop: b.loop(oes...), orient: true})))
	return out
}

func FuzzSniff(f *testing.F) {
	f.Add([]byte("ISO-10303-21;"))
	f.Add([]byte("\xEF\xBB\xBF /* x */ ISO-10303-21 ;"))
	f.Add([]byte("/* unterminated"))
	f.Add([]byte("solid cube\nfacet normal"))
	f.Fuzz(func(t *testing.T, data []byte) {
		Sniff(data)
	})
}

func FuzzParse(f *testing.F) {
	for _, s := range fuzzSeeds()[:4] {
		f.Add([]byte(s))
	}
	f.Add([]byte("ISO-10303-21;DATA;#1=(A((((((((1))))))))B('')C(.X.));#2=X('\\X2\\00E9\\X0\\',\"0F\",1.E+5,-.5);ENDSEC;"))
	f.Add([]byte("ISO-10303-21;HEADER;FILE_NAME('\\X4\\0001F600\\X0\\\\S\\a\\X\\E9','',(),(),'','','');ENDSEC;DATA;#9999999999999=A();#1=B(#1,#2,(#3));"))
	f.Fuzz(func(t *testing.T, data []byte) {
		pf, err := parseFile(context.Background(), data, 4096)
		if err != nil {
			return
		}
		m := newModel(pf)
		for i := range pf.ents {
			if i > 2048 {
				break
			}
			pf.parts(i)
			pf.args(i)
			pf.hasPart(i, "B_SPLINE_CURVE")
			pf.partNames(i, nil)
			m.point(i)
			m.placement(i)
			m.curve(i, 0)
			m.surface(i, 0)
			m.transformOp(i)
		}
		if a := pf.head.name; len(a) > 4*len(data)+16 {
			t.Fatalf("name too long")
		}
	})
}

func FuzzRead(f *testing.F) {
	for _, s := range fuzzSeeds() {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<17 {
			return
		}
		before := facePanics.Load()
		o := Options{MaxTriangles: 400000, MaxEntities: 20000}
		ms, _, err := Read(context.Background(), rd{string(data)}, int64(len(data)), o)
		if err == nil {
			if ms.Triangles() == 0 || len(ms.N) != len(ms.V) || (ms.C != nil && len(ms.C) != ms.Triangles()*3) {
				t.Fatalf("bad mesh %d %d %d", len(ms.V), len(ms.N), len(ms.C))
			}
			for _, x := range ms.V {
				if math.IsNaN(float64(x)) {
					t.Fatalf("NaN vertex")
				}
			}
		}
		Probe(context.Background(), rd{string(data)}, int64(len(data)), o)
		if facePanics.Load() != before {
			t.Fatalf("recovered panic in tessellation")
		}
	})
}
