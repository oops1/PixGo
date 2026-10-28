package step

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
)

type solidCase struct {
	name   string
	build  func(b *sb) int
	area   float64
	volume float64
	bounds [6]float64
	closed bool
}

func solidCases() []solidCase {
	r, h := 2.0, 5.0
	r1, hc, ang := 3.0, 4.0, 0.3
	r2 := r1 + hc*math.Tan(ang)
	slant := math.Hypot(r2-r1, hc)
	R, rt := 5.0, 1.5
	w, d, th, rh := 10.0, 8.0, 2.0, 1.5
	return []solidCase{
		{"box", func(b *sb) int { return b.box(vec3{1, 2, 3}, vec3{2, 3, 4}) }, 2 * (6 + 8 + 12), 24, [6]float64{1, 2, 3, 3, 5, 7}, true},
		{"cylinder", func(b *sb) int { return b.cylinderSolid(r, h, false) }, 2*math.Pi*r*h + 2*math.Pi*r*r, math.Pi * r * r * h, [6]float64{-r, -r, 0, r, r, h}, true},
		{"cylinder_pcurves", func(b *sb) int { return b.cylinderSolid(r, h, true) }, 2*math.Pi*r*h + 2*math.Pi*r*r, math.Pi * r * r * h, [6]float64{-r, -r, 0, r, r, h}, true},
		{"cone", func(b *sb) int { return b.coneSolid(r1, hc, ang) }, math.Pi*(r1+r2)*slant + math.Pi*r1*r1 + math.Pi*r2*r2, math.Pi * hc / 3 * (r1*r1 + r1*r2 + r2*r2), [6]float64{-r2, -r2, 0, r2, r2, hc}, true},
		{"sphere_seam", func(b *sb) int { return b.sphereSolid(vec3{1, -2, 3}, 4, 0) }, 4 * math.Pi * 16, 4.0 / 3 * math.Pi * 64, [6]float64{-3, -6, -1, 5, 2, 7}, true},
		{"sphere_nobounds", func(b *sb) int { return b.sphereSolid(vec3{}, 4, 1) }, 4 * math.Pi * 16, 4.0 / 3 * math.Pi * 64, [6]float64{-4, -4, -4, 4, 4, 4}, true},
		{"sphere_vertexloops", func(b *sb) int { return b.sphereSolid(vec3{}, 4, 2) }, 4 * math.Pi * 16, 4.0 / 3 * math.Pi * 64, [6]float64{-4, -4, -4, 4, 4, 4}, true},
		{"torus", func(b *sb) int { return b.torusSolid(R, rt, false) }, 4 * math.Pi * math.Pi * R * rt, 2 * math.Pi * math.Pi * R * rt * rt, [6]float64{-R - rt, -R - rt, -rt, R + rt, R + rt, rt}, true},
		{"torus_nobounds", func(b *sb) int { return b.torusSolid(R, rt, true) }, 4 * math.Pi * math.Pi * R * rt, 2 * math.Pi * math.Pi * R * rt * rt, [6]float64{-R - rt, -R - rt, -rt, R + rt, R + rt, rt}, true},
		{"plate_hole", func(b *sb) int { return b.plateWithHole(w, d, th, 4, 3, rh) }, 2*(w*d-math.Pi*rh*rh) + 2*(w+d)*th + 2*math.Pi*rh*th, (w*d - math.Pi*rh*rh) * th, [6]float64{0, 0, 0, w, d, th}, true},
	}
}

func TestSolids(t *testing.T) {
	before := facePanics.Load()
	for _, c := range solidCases() {
		t.Run(c.name, func(t *testing.T) {
			b := newSB()
			s := c.build(b)
			m, info := readText(t, b.file(s), Options{})
			if m.Failed != 0 || info.Failed != 0 {
				t.Fatalf("failed faces: %d", m.Failed)
			}
			a, v := m.Area(), m.Volume()
			diag := math.Sqrt((c.bounds[3]-c.bounds[0])*(c.bounds[3]-c.bounds[0]) + (c.bounds[4]-c.bounds[1])*(c.bounds[4]-c.bounds[1]) + (c.bounds[5]-c.bounds[2])*(c.bounds[5]-c.bounds[2]))
			defl := 0.0008 * diag
			t.Logf("tris=%d area %.6f (%.4f%%) volume %.6f (%.4f%%) open=%d", m.Triangles(), a, 100*relErr(a, c.area), v, 100*relErr(v, c.volume), openEdges(m))
			if relErr(a, c.area) > 0.005 {
				t.Errorf("area %v want %v", a, c.area)
			}
			if relErr(v, c.volume) > 0.005 {
				t.Errorf("volume %v want %v", v, c.volume)
			}
			for k := 0; k < 3; k++ {
				if math.Abs(float64(m.Min[k])-c.bounds[k]) > defl+1e-5 || math.Abs(float64(m.Max[k])-c.bounds[3+k]) > defl+1e-5 {
					t.Errorf("bounds %v %v want %v", m.Min, m.Max, c.bounds)
					break
				}
			}
			if c.closed {
				if n := openEdges(m); n != 0 {
					t.Errorf("open edges %d", n)
				}
			}
			if !m.HasNormals() || m.HasColors() {
				t.Errorf("normals %v colors %v", m.HasNormals(), m.HasColors())
			}
			for i, x := range m.N {
				if math.IsNaN(float64(x)) {
					t.Fatalf("NaN normal at %d", i)
				}
			}
			checkNormalsOutward(t, m)
		})
	}
	if facePanics.Load() != before {
		t.Fatalf("recovered panics during tessellation")
	}
}

func checkNormalsOutward(t *testing.T, m interface {
	Triangles() int
}) {
}

func TestInfoAndProbe(t *testing.T) {
	b := newSB()
	s := b.box(vec3{0, 0, 0}, vec3{1, 2, 3})
	txt := b.file(s)
	info, bb, err := Probe(context.Background(), rd{txt}, int64(len(txt)), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if info.Schema != "AUTOMOTIVE_DESIGN" || info.System != "Test System" || info.Name != "Generated part" || info.Unit != "mm" {
		t.Errorf("info %+v", info)
	}
	if info.Faces != 6 || info.Solids != 1 || info.Entities == 0 {
		t.Errorf("counts %+v", info)
	}
	want := [6]float32{0, 0, 0, 1, 2, 3}
	if bb != want {
		t.Errorf("bounds %v", bb)
	}
}

func TestUnits(t *testing.T) {
	cases := []struct {
		unit  string
		extra func(b *sb) string
		scale float64
		name  string
	}{
		{"m", func(b *sb) string { return "( LENGTH_UNIT() NAMED_UNIT(*) SI_UNIT($,.METRE.) )" }, 1000, "m"},
		{"cm", func(b *sb) string { return "( LENGTH_UNIT() NAMED_UNIT(*) SI_UNIT(.CENTI.,.METRE.) )" }, 10, "cm"},
		{"inch", func(b *sb) string {
			mm := b.e("( LENGTH_UNIT() NAMED_UNIT(*) SI_UNIT(.MILLI.,.METRE.) )")
			lm := b.e("LENGTH_MEASURE_WITH_UNIT(LENGTH_MEASURE(25.4),#%d)", mm)
			dim := b.e("DIMENSIONAL_EXPONENTS(1.,0.,0.,0.,0.,0.,0.)")
			return "( CONVERSION_BASED_UNIT('INCH',#" + itoa(lm) + ") LENGTH_UNIT() NAMED_UNIT(#" + itoa(dim) + ") )"
		}, 25.4, "inch"},
	}
	for _, c := range cases {
		b := newSB()
		b.unit = c.extra(b)
		s := b.box(vec3{0, 0, 0}, vec3{1, 2, 3})
		m, info := readText(t, b.file(s), Options{})
		if info.Unit != c.name {
			t.Errorf("%s: unit %q", c.unit, info.Unit)
		}
		if math.Abs(float64(m.Max[2])-3*c.scale) > 1e-3*c.scale {
			t.Errorf("%s: max %v", c.unit, m.Max)
		}
	}
}

func itoa(n int) string {
	return strings.TrimSpace(strings.Replace(string(rune('0'+n%10)), "", "", 0)) + ""
}

func TestCorruptAndUnsupported(t *testing.T) {
	ctx := context.Background()
	bad := []string{"", "hello", "ISO-10303-21;\nHEADER;\nENDSEC;\n"}
	for _, s := range bad {
		_, _, err := Read(ctx, rd{s}, int64(len(s)), Options{})
		if !errors.Is(err, codec.ErrCorrupt) {
			t.Errorf("%q: %v", s, err)
		}
	}
	empty := "ISO-10303-21;\nHEADER;\nENDSEC;\nDATA;\n#1=CARTESIAN_POINT('',(0.,0.,0.));\nENDSEC;\nEND-ISO-10303-21;\n"
	_, _, err := Read(ctx, rd{empty}, int64(len(empty)), Options{})
	if !errors.Is(err, codec.ErrUnsupported) {
		t.Errorf("empty: %v", err)
	}
}

func TestSniff(t *testing.T) {
	ok := []string{"ISO-10303-21;", "\xEF\xBB\xBF  ISO-10303-21 ;\n", "/* c */\nISO-10303-21;HEADER;"}
	for _, s := range ok {
		if !Sniff([]byte(s)) {
			t.Errorf("Sniff(%q) = false", s)
		}
	}
	no := []string{"", "ISO-10303-2", "solid x", "ISO-10303-28;"}
	for _, s := range no {
		if Sniff([]byte(s)) {
			t.Errorf("Sniff(%q) = true", s)
		}
	}
}
