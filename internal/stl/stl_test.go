package stl

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/mesh"
	"github.com/oops1/PixGo/internal/par"
)

type tri [9]float32

func cubeTris(x, y, z, s float32) []tri {
	c := [8][3]float32{
		{x, y, z}, {x + s, y, z}, {x + s, y + s, z}, {x, y + s, z},
		{x, y, z + s}, {x + s, y, z + s}, {x + s, y + s, z + s}, {x, y + s, z + s},
	}
	faces := [12][3]int{
		{0, 3, 2}, {0, 2, 1},
		{4, 5, 6}, {4, 6, 7},
		{0, 1, 5}, {0, 5, 4},
		{2, 3, 7}, {2, 7, 6},
		{0, 4, 7}, {0, 7, 3},
		{1, 2, 6}, {1, 6, 5},
	}
	out := make([]tri, 0, 12)
	for _, f := range faces {
		var t tri
		for k := 0; k < 3; k++ {
			copy(t[k*3:], c[f[k]][:])
		}
		out = append(out, t)
	}
	return out
}

func flatten(ts []tri) []float32 {
	v := make([]float32, 0, len(ts)*9)
	for _, t := range ts {
		v = append(v, t[:]...)
	}
	return v
}

func binarySTL(header string, ts []tri, attrs []uint16, trailing int) []byte {
	h := make([]byte, 80)
	copy(h, header)
	out := append([]byte(nil), h...)
	out = binary.LittleEndian.AppendUint32(out, uint32(len(ts)))
	for i, t := range ts {
		out = append(out, make([]byte, 12)...)
		for _, f := range t {
			out = binary.LittleEndian.AppendUint32(out, math.Float32bits(f))
		}
		var a uint16
		if i < len(attrs) {
			a = attrs[i]
		}
		out = binary.LittleEndian.AppendUint16(out, a)
	}
	return append(out, make([]byte, trailing)...)
}

type asciiStyle struct {
	crlf     bool
	upper    bool
	tabs     bool
	exp      bool
	noEnd    bool
	bom      bool
	garbage  string
	leading  string
	cr       bool
	oneSolid bool
}

func (s asciiStyle) word(w string) string {
	if s.upper {
		return strings.ToUpper(w)
	}
	return w
}

func (s asciiStyle) num(f float32) string {
	if s.exp {
		return strconv.FormatFloat(float64(f), 'e', -1, 32)
	}
	return strconv.FormatFloat(float64(f), 'f', -1, 32)
}

func asciiSTL(name string, groups [][]tri, s asciiStyle) []byte {
	nl := "\n"
	if s.crlf {
		nl = "\r\n"
	}
	if s.cr {
		nl = "\r"
	}
	sep := " "
	if s.tabs {
		sep = "\t"
	}
	var b strings.Builder
	if s.bom {
		b.WriteString("\xEF\xBB\xBF")
	}
	b.WriteString(s.leading)
	for gi, g := range groups {
		fmt.Fprintf(&b, "%s%s%s%s", s.word("solid"), sep, name, nl)
		for _, t := range g {
			fmt.Fprintf(&b, "%s%s%s%s0%s0%s1%s", s.word("facet"), sep, s.word("normal"), sep, sep, sep, nl)
			fmt.Fprintf(&b, "  %s%s%s%s", s.word("outer"), sep, s.word("loop"), nl)
			for k := 0; k < 3; k++ {
				fmt.Fprintf(&b, "    %s%s%s%s%s%s%s%s", s.word("vertex"), sep, s.num(t[k*3]), sep, s.num(t[k*3+1]), sep, s.num(t[k*3+2]), nl)
			}
			fmt.Fprintf(&b, "  %s%s", s.word("endloop"), nl)
			fmt.Fprintf(&b, "%s%s", s.word("endfacet"), nl)
		}
		if !(s.noEnd && gi == len(groups)-1) {
			fmt.Fprintf(&b, "%s%s%s%s", s.word("endsolid"), sep, name, nl)
		}
	}
	b.WriteString(s.garbage)
	return []byte(b.String())
}

func read(t testing.TB, data []byte, o Options) (*mesh.Mesh, Info, error) {
	t.Helper()
	return Read(context.Background(), bytes.NewReader(data), int64(len(data)), o)
}

func mustRead(t testing.TB, data []byte) (*mesh.Mesh, Info) {
	t.Helper()
	m, info, err := read(t, data, Options{})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	return m, info
}

func sameFloats(a, b []float32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if math.Float32bits(a[i]) != math.Float32bits(b[i]) {
			return false
		}
	}
	return true
}

func checkCube(t *testing.T, m *mesh.Mesh, info Info, ascii bool, side float64) {
	t.Helper()
	if info.ASCII != ascii {
		t.Errorf("Info.ASCII = %v, want %v", info.ASCII, ascii)
	}
	if info.Triangles != 12 || m.Triangles() != 12 {
		t.Fatalf("triangles = %d / %d, want 12", info.Triangles, m.Triangles())
	}
	if m.Parts != 1 {
		t.Errorf("Parts = %d, want 1", m.Parts)
	}
	vol := m.Volume()
	want := side * side * side
	if math.Abs(vol-want) > 1e-6*want {
		t.Errorf("volume = %v, want %v", vol, want)
	}
	area := m.Area()
	if math.Abs(area-6*side*side) > 1e-6*6*side*side {
		t.Errorf("area = %v, want %v", area, 6*side*side)
	}
	lo, hi := m.Min, m.Max
	m.UpdateBounds()
	if lo != m.Min || hi != m.Max {
		t.Errorf("bounds %v %v differ from UpdateBounds %v %v", lo, hi, m.Min, m.Max)
	}
}

func TestBinaryCube(t *testing.T) {
	data := binarySTL("cube", cubeTris(1, 2, 3, 4), nil, 0)
	m, info := mustRead(t, data)
	checkCube(t, m, info, false, 4)
	if m.Min != [3]float32{1, 2, 3} || m.Max != [3]float32{5, 6, 7} {
		t.Errorf("bounds %v %v", m.Min, m.Max)
	}
	if m.HasNormals() || m.HasColors() || info.Colors {
		t.Errorf("unexpected normals or colours")
	}
	if info.Name != "cube" {
		t.Errorf("name = %q", info.Name)
	}
	if !sameFloats(m.V, flatten(cubeTris(1, 2, 3, 4))) {
		t.Errorf("vertices differ")
	}
}

func TestASCIICube(t *testing.T) {
	data := asciiSTL("cube", [][]tri{cubeTris(-1, -2, -3, 2.5)}, asciiStyle{})
	m, info := mustRead(t, data)
	checkCube(t, m, info, true, 2.5)
	if info.Name != "cube" {
		t.Errorf("name = %q", info.Name)
	}
	if !sameFloats(m.V, flatten(cubeTris(-1, -2, -3, 2.5))) {
		t.Errorf("vertices differ")
	}
}

func TestASCIIStyles(t *testing.T) {
	styles := map[string]asciiStyle{
		"crlf":      {crlf: true},
		"cr":        {cr: true},
		"upper":     {upper: true},
		"tabs":      {tabs: true},
		"exp":       {exp: true},
		"noendsold": {noEnd: true},
		"bom":       {bom: true},
		"leading":   {leading: "  \r\n\t\n"},
		"garbage":   {garbage: "\x00\x01 junk facet nonsense vertex 1 2\n"},
		"all":       {crlf: true, upper: true, tabs: true, exp: true, bom: true, noEnd: true, leading: "\n\n"},
	}
	want := flatten(cubeTris(0.1, 0.2, 0.3, 7.77))
	for name, st := range styles {
		t.Run(name, func(t *testing.T) {
			data := asciiSTL("part "+name, [][]tri{cubeTris(0.1, 0.2, 0.3, 7.77)}, st)
			if !Sniff(data) {
				t.Fatalf("Sniff rejected the file")
			}
			m, info := mustRead(t, data)
			if info.Triangles != 12 || !sameFloats(m.V, want) {
				t.Fatalf("triangles %d, vertices equal %v", info.Triangles, sameFloats(m.V, want))
			}
			if m.Parts != 1 {
				t.Errorf("Parts = %d", m.Parts)
			}
			pi, b, err := Probe(context.Background(), bytes.NewReader(data), int64(len(data)))
			if err != nil {
				t.Fatalf("Probe: %v", err)
			}
			if pi != info || b != [6]float32{m.Min[0], m.Min[1], m.Min[2], m.Max[0], m.Max[1], m.Max[2]} {
				t.Errorf("Probe = %+v %v, Read = %+v %v %v", pi, b, info, m.Min, m.Max)
			}
		})
	}
}

func TestASCIIMultipleSolids(t *testing.T) {
	a, b, c := cubeTris(0, 0, 0, 1), cubeTris(5, 0, 0, 2), cubeTris(0, 9, 0, 3)
	data := asciiSTL("p", [][]tri{a, b, c}, asciiStyle{})
	m, info := mustRead(t, data)
	if info.Triangles != 36 || m.Parts != 3 {
		t.Fatalf("triangles %d parts %d", info.Triangles, m.Parts)
	}
	if math.Abs(m.Volume()-(1+8+27)) > 1e-5 {
		t.Errorf("volume = %v", m.Volume())
	}
	if m.Min != [3]float32{0, 0, 0} || m.Max != [3]float32{7, 12, 3} {
		t.Errorf("bounds %v %v", m.Min, m.Max)
	}
}

func TestASCIIEmptySolidsDoNotCount(t *testing.T) {
	data := []byte("solid a\nendsolid a\nsolid b\n")
	data = append(data, asciiSTL("c", [][]tri{cubeTris(0, 0, 0, 1)}, asciiStyle{})...)
	data = append(data, []byte("solid d\nendsolid d\n")...)
	m, _ := mustRead(t, data)
	if m.Parts != 1 {
		t.Errorf("Parts = %d, want 1", m.Parts)
	}
}

func TestASCIIPolygon(t *testing.T) {
	text := "solid q\nfacet normal 0 0 1\nouter loop\nvertex 0 0 0\nvertex 1 0 0\nvertex 1 1 0\nvertex 0 1 0\nendloop\nendfacet\n" +
		"facet normal 0 0 1\nouter loop\nvertex 0 0 0\nvertex 2 0 0\nvertex 2 1 0\nvertex 1 2 0\nvertex 0 1 0\nendloop\nendfacet\nendsolid q\n"
	m, info := mustRead(t, []byte(text))
	if info.Triangles != 5 {
		t.Fatalf("triangles = %d, want 5", info.Triangles)
	}
	if math.Abs(m.Area()-4) > 1e-6 {
		t.Errorf("area = %v, want 4", m.Area())
	}
}

func TestASCIIAbnormalNumbers(t *testing.T) {
	text := "solid x\n" +
		"facet normal 0 0 1\nouter loop\nvertex 1.5e+02 -0.0 +3.\nvertex .5 1E1 2e0\nvertex 0 0 0\nendloop\nendfacet\n" +
		"facet normal 0 0 1\nouter loop\nvertex nan 0 0\nvertex 1 1 1\nvertex 2 2 2\nendloop\nendfacet\n" +
		"facet normal 0 0 1\nouter loop\nvertex 1.#QNAN 0 0\nvertex 1 1 1\nvertex 2 2 2\nendloop\nendfacet\n" +
		"facet normal 0 0 1\nouter loop\nvertex inf 0 0\nvertex 1 1 1\nvertex 2 2 2\nendloop\nendfacet\n" +
		"facet normal 0 0 1\nouter loop\nvertex 1e999 0 0\nvertex 1 1 1\nvertex 2 2 2\nendloop\nendfacet\n" +
		"facet normal 0 0 1\nouter loop\nvertex 1 1 1\nvertex 1 1 1\nvertex 1 1 1\nendloop\nendfacet\n" +
		"facet normal 0 0 1\nouter loop\nvertex 4 4 4\nvertex 5 5\nendloop\nendfacet\n" +
		"endsolid x\n"
	m, info := mustRead(t, []byte(text))
	if info.Triangles != 2 {
		t.Fatalf("triangles = %d, want 2 (finite and zero-area kept)", info.Triangles)
	}
	want := []float32{150, float32(math.Copysign(0, -1)), 3, 0.5, 10, 2, 0, 0, 0, 1, 1, 1, 1, 1, 1, 1, 1, 1}
	if !sameFloats(m.V, want) {
		t.Errorf("V = %v, want %v", m.V, want)
	}
	if m.Min != [3]float32{0, -0, 0} || m.Max != [3]float32{150, 10, 3} {
		t.Errorf("bounds %v %v", m.Min, m.Max)
	}
}

func TestBinarySolidHeaderStillBinary(t *testing.T) {
	data := binarySTL("solid exported by SolidWorks facet endsolid", cubeTris(0, 0, 0, 3), nil, 0)
	if !IsBinary(data, int64(len(data))) {
		t.Fatalf("IsBinary = false")
	}
	if !Sniff(data[:80]) {
		t.Fatalf("test header should look like ASCII to Sniff")
	}
	m, info := mustRead(t, data)
	checkCube(t, m, info, false, 3)
	if info.Name != "exported by SolidWorks facet endsolid" {
		t.Errorf("name = %q", info.Name)
	}
}

func TestIsBinary(t *testing.T) {
	cube := binarySTL("", cubeTris(0, 0, 0, 1), nil, 0)
	cases := []struct {
		name string
		head []byte
		size int64
		want bool
	}{
		{"exact", cube, int64(len(cube)), true},
		{"slack0", cube, int64(len(cube)), true},
		{"slack512", cube, int64(len(cube)) + 512, true},
		{"slack513", cube, int64(len(cube)) + 513, false},
		{"short file", cube, int64(len(cube)) - 1, false},
		{"short head", cube[:83], int64(len(cube)), false},
		{"tiny size", cube, 83, false},
		{"zero", make([]byte, 84), 84, true},
		{"zero slack", make([]byte, 84), 84 + 512, true},
		{"empty", nil, 0, false},
	}
	for _, c := range cases {
		if got := IsBinary(c.head, c.size); got != c.want {
			t.Errorf("%s: IsBinary = %v, want %v", c.name, got, c.want)
		}
	}
	huge := make([]byte, 84)
	binary.LittleEndian.PutUint32(huge[80:], math.MaxUint32)
	if IsBinary(huge, 1<<40) {
		t.Errorf("a 4 billion triangle header must not match a 1 TiB file")
	}
}

func TestSniff(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"solid a\nfacet normal 0 0 1", true},
		{"SOLID a\nFACET NORMAL 0 0 1", true},
		{"\xEF\xBB\xBF  \r\n\tsolid a\n facet", true},
		{"solid\nendsolid", true},
		{"solid a endsolid a", true},
		{"solid", false},
		{"solid a\nnothing here", false},
		{"solids a\nfacet", false},
		{"facet normal\nsolid", false},
		{"solid a\nfacet\x00", true},
		{"solid a\x00\nfacet", false},
		{"solid a\nnothing\x00 facet", false},
		{"", false},
		{"   ", false},
		{"hello solid facet", false},
	}
	for _, c := range cases {
		if got := Sniff([]byte(c.in)); got != c.want {
			t.Errorf("Sniff(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestBinarySlackRead(t *testing.T) {
	cube := cubeTris(0, 0, 0, 1)
	m, info := mustRead(t, binarySTL("x", cube, nil, 200))
	checkCube(t, m, info, false, 1)
	long := binarySTL("x", cube, nil, 1573)
	if IsBinary(long, int64(len(long))) {
		t.Errorf("IsBinary accepted 1573 trailing bytes")
	}
	m, info = mustRead(t, long)
	checkCube(t, m, info, false, 1)
	short := binarySTL("x", cube, nil, 0)
	short[80]++
	if _, _, err := read(t, short, Options{}); !errors.Is(err, codec.ErrCorrupt) {
		t.Errorf("count above the file size: err = %v, want ErrCorrupt", err)
	}
}

func TestColorsViscam(t *testing.T) {
	cube := cubeTris(0, 0, 0, 1)
	attrs := make([]uint16, 12)
	attrs[0] = 0x8000 | 31<<10
	attrs[1] = 0x8000 | 31<<5
	attrs[2] = 0x8000 | 31
	attrs[3] = 0x8000 | 16<<10 | 8<<5 | 4
	attrs[4] = 0x7FFF
	m, info := mustRead(t, binarySTL("viscam", cube, attrs, 0))
	if !info.Colors || !m.HasColors() {
		t.Fatalf("colours not detected")
	}
	want := [][3]uint8{{255, 0, 0}, {0, 255, 0}, {0, 0, 255}, {132, 66, 33}}
	for i, w := range want {
		got := [3]uint8{m.C[i*3], m.C[i*3+1], m.C[i*3+2]}
		if got != w {
			t.Errorf("face %d colour %v, want %v", i, got, w)
		}
	}
	for i := 4; i < 12; i++ {
		got := [3]uint8{m.C[i*3], m.C[i*3+1], m.C[i*3+2]}
		if got != defaultColor {
			t.Errorf("face %d colour %v, want default %v", i, got, defaultColor)
		}
	}
}

func TestColorsMaterialise(t *testing.T) {
	cube := cubeTris(0, 0, 0, 1)
	header := "solid part COLOR=\x0a\x14\x1e\xff MATERIAL=\x01\x02\x03\xff"
	attrs := make([]uint16, 12)
	attrs[0] = 31
	attrs[1] = 31 << 5
	attrs[2] = 31 << 10
	attrs[3] = 0x8000 | 31
	m, info := mustRead(t, binarySTL(header, cube, attrs, 0))
	if !info.Colors || !m.HasColors() {
		t.Fatalf("colours not detected")
	}
	want := map[int][3]uint8{0: {255, 0, 0}, 1: {0, 255, 0}, 2: {0, 0, 255}, 3: {10, 20, 30}, 4: {10, 20, 30}, 11: {10, 20, 30}}
	for i, w := range want {
		got := [3]uint8{m.C[i*3], m.C[i*3+1], m.C[i*3+2]}
		if got != w {
			t.Errorf("face %d colour %v, want %v", i, got, w)
		}
	}
	if info.Name != "part" {
		t.Errorf("name = %q", info.Name)
	}
}

func TestNoColorsGivesNilC(t *testing.T) {
	cube := cubeTris(0, 0, 0, 1)
	for _, header := range []string{"plain", "solid COLOR=\x0a\x14\x1e\xff"} {
		attrs := make([]uint16, 12)
		if strings.Contains(header, "COLOR") {
			for i := range attrs {
				attrs[i] = 0x8000
			}
		}
		m, info := mustRead(t, binarySTL(header, cube, attrs, 0))
		if m.C != nil || info.Colors {
			t.Errorf("header %q: C = %v, Colors = %v", header, m.C, info.Colors)
		}
	}
}

func TestBinaryDropsNonFinite(t *testing.T) {
	ts := cubeTris(0, 0, 0, 1)
	nan := float32(math.NaN())
	ts[1][4] = nan
	ts[5][0] = float32(math.Inf(1))
	ts[7][8] = float32(math.Inf(-1))
	attrs := make([]uint16, len(ts))
	for i := range attrs {
		attrs[i] = 0x8000 | uint16(i)
	}
	m, info := mustRead(t, binarySTL("", ts, attrs, 0))
	if info.Triangles != 9 {
		t.Fatalf("triangles = %d, want 9", info.Triangles)
	}
	for _, f := range m.V {
		if !isFinite(f) {
			t.Fatalf("non-finite coordinate survived")
		}
	}
	if len(m.C) != 27 {
		t.Fatalf("len(C) = %d", len(m.C))
	}
	kept := []int{0, 2, 3, 4, 6, 8, 9, 10, 11}
	for k, i := range kept {
		if !sameFloats(m.V[k*9:k*9+9], ts[i][:]) {
			t.Errorf("triangle %d moved wrongly", i)
		}
		want := expand5(uint16(i))
		if m.C[k*3+2] != want {
			t.Errorf("colour of kept triangle %d is %d, want %d", i, m.C[k*3+2], want)
		}
	}
	bad := make([]tri, 3)
	for i := range bad {
		bad[i] = tri{nan, 0, 0, 0, 0, 0, 0, 0, 0}
	}
	_, _, err := read(t, binarySTL("", bad, nil, 0), Options{})
	if !errors.Is(err, codec.ErrCorrupt) {
		t.Errorf("all non-finite: err = %v", err)
	}
}

func TestBinaryZeroAreaKept(t *testing.T) {
	ts := []tri{{1, 1, 1, 1, 1, 1, 1, 1, 1}, {0, 0, 0, 1, 0, 0, 2, 0, 0}}
	_, info := mustRead(t, binarySTL("", ts, nil, 0))
	if info.Triangles != 2 {
		t.Errorf("triangles = %d, want 2", info.Triangles)
	}
}

func TestTruncatedBinary(t *testing.T) {
	data := binarySTL("trunc", cubeTris(0, 0, 0, 1), nil, 0)
	for n := 0; n < len(data); n++ {
		_, _, err := read(t, data[:n], Options{})
		if err == nil {
			t.Fatalf("prefix %d of %d: no error", n, len(data))
		}
		if !errors.Is(err, codec.ErrCorrupt) {
			t.Fatalf("prefix %d: err = %v, want ErrCorrupt", n, err)
		}
	}
}

func TestTruncatedASCII(t *testing.T) {
	data := asciiSTL("trunc", [][]tri{cubeTris(0, 0, 0, 1)}, asciiStyle{})
	for n := 0; n < len(data); n++ {
		m, info, err := read(t, data[:n], Options{})
		if err != nil {
			if !errors.Is(err, codec.ErrCorrupt) {
				t.Fatalf("prefix %d: err = %v", n, err)
			}
			continue
		}
		if info.Triangles > 12 || m.Triangles() != info.Triangles {
			t.Fatalf("prefix %d: %d triangles", n, info.Triangles)
		}
		for i := 0; i < m.Triangles(); i++ {
			if !sameFloats(m.V[i*9:i*9+9], cubeTris(0, 0, 0, 1)[i][:]) && info.Triangles != 0 {
				t.Fatalf("prefix %d: triangle %d is wrong", n, i)
			}
		}
	}
}

func TestNotSTL(t *testing.T) {
	cases := map[string][]byte{
		"empty":     nil,
		"text":      []byte("hello world, this is not a mesh at all"),
		"solid":     []byte("solid only a name"),
		"solid nul": append([]byte("solid x\nfacet normal 0 0 1\n"), 0, 1, 2),
		"zeros":     make([]byte, 200),
		"short":     []byte("abc"),
	}
	for name, data := range cases {
		_, _, err := read(t, data, Options{})
		if !errors.Is(err, codec.ErrCorrupt) {
			t.Errorf("%s: err = %v, want ErrCorrupt", name, err)
		}
		if _, _, err := Probe(context.Background(), bytes.NewReader(data), int64(len(data))); !errors.Is(err, codec.ErrCorrupt) {
			t.Errorf("%s: Probe err = %v, want ErrCorrupt", name, err)
		}
	}
	_, _, err := read(t, []byte("\xFF\xFEs\x00o\x00l\x00i\x00d\x00"), Options{})
	if !errors.Is(err, codec.ErrUnsupported) {
		t.Errorf("UTF-16: err = %v, want ErrUnsupported", err)
	}
}

func TestEmptyBinaryIsCorrupt(t *testing.T) {
	_, _, err := read(t, binarySTL("empty", nil, nil, 0), Options{})
	if !errors.Is(err, codec.ErrCorrupt) {
		t.Errorf("err = %v", err)
	}
}

func TestTooManyTriangles(t *testing.T) {
	cube := cubeTris(0, 0, 0, 1)
	for name, data := range map[string][]byte{
		"binary": binarySTL("", cube, nil, 0),
		"ascii":  asciiSTL("a", [][]tri{cube}, asciiStyle{}),
	} {
		_, _, err := read(t, data, Options{MaxTriangles: 11})
		if !errors.Is(err, codec.ErrTooLarge) || !errors.Is(err, mesh.ErrTooManyTriangles) {
			t.Errorf("%s: err = %v", name, err)
		}
		if _, _, err := read(t, data, Options{MaxTriangles: 12}); err != nil {
			t.Errorf("%s: limit equal to count: %v", name, err)
		}
	}
}

type zeroReader struct {
	head []byte
	size int64
}

func (z zeroReader) ReadAt(p []byte, off int64) (int, error) {
	if off >= z.size {
		return 0, io.EOF
	}
	n := int(min(int64(len(p)), z.size-off))
	for i := 0; i < n; i++ {
		if o := off + int64(i); o < int64(len(z.head)) {
			p[i] = z.head[o]
		} else {
			p[i] = 0
		}
	}
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func allocated() uint64 {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return ms.TotalAlloc
}

func TestHugeCountDoesNotAllocate(t *testing.T) {
	head := make([]byte, 84)
	binary.LittleEndian.PutUint32(head[80:], 10_000_000)
	before := allocated()
	_, _, err := Read(context.Background(), zeroReader{head, 1000}, 1000, Options{})
	if !errors.Is(err, codec.ErrCorrupt) {
		t.Errorf("small file, huge count: err = %v", err)
	}
	if grew := allocated() - before; grew > 1<<20 {
		t.Errorf("allocated %d bytes for a 1000 byte file", grew)
	}

	n := 1_000_000
	binary.LittleEndian.PutUint32(head[80:], uint32(n))
	size := int64(84 + 50*n)
	before = allocated()
	_, _, err = Read(context.Background(), zeroReader{head, size}, size, Options{MaxTriangles: 1000})
	if !errors.Is(err, codec.ErrTooLarge) {
		t.Errorf("over limit: err = %v", err)
	}
	if grew := allocated() - before; grew > 1<<20 {
		t.Errorf("allocated %d bytes before rejecting a count over the limit", grew)
	}
}

func TestZeroTrianglesBinaryFile(t *testing.T) {
	head := make([]byte, 84)
	binary.LittleEndian.PutUint32(head[80:], 1000)
	_, _, err := Read(context.Background(), zeroReader{head, 84 + 50*1000}, 84+50*1000, Options{})
	if err != nil {
		t.Fatalf("a file of zero-area triangles at the origin is valid: %v", err)
	}
}

func TestContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	data := binarySTL("", cubeTris(0, 0, 0, 1), nil, 0)
	if _, _, err := Read(ctx, bytes.NewReader(data), int64(len(data)), Options{}); !errors.Is(err, context.Canceled) {
		t.Errorf("Read: err = %v", err)
	}
	if _, _, err := Probe(ctx, bytes.NewReader(data), int64(len(data))); !errors.Is(err, context.Canceled) {
		t.Errorf("Probe: err = %v", err)
	}
}

type cancelAfter struct {
	r      io.ReaderAt
	calls  atomic.Int64
	after  int64
	cancel context.CancelFunc
}

func (c *cancelAfter) ReadAt(p []byte, off int64) (int, error) {
	if c.calls.Add(1) == c.after {
		c.cancel()
	}
	return c.r.ReadAt(p, off)
}

func bigTriangles(n int, seed int64) []tri {
	rng := rand.New(rand.NewSource(seed))
	ts := make([]tri, n)
	for i := range ts {
		for k := range ts[i] {
			ts[i][k] = float32(rng.NormFloat64() * 100)
		}
	}
	return ts
}

func TestContextCanceledMidway(t *testing.T) {
	bin := binarySTL("", bigTriangles(binaryChunkTriangles*par.Count()*4, 1), nil, 0)
	old := asciiBlockBytes
	asciiBlockBytes = 2_000
	defer func() { asciiBlockBytes = old }()
	asc := asciiSTL("x", [][]tri{bigTriangles(2000, 2)}, asciiStyle{exp: true})
	for name, data := range map[string][]byte{"binary": bin, "ascii": asc} {
		for _, probe := range []bool{false, true} {
			ctx, cancel := context.WithCancel(context.Background())
			r := &cancelAfter{r: bytes.NewReader(data), after: 3, cancel: cancel}
			var err error
			if probe {
				_, _, err = Probe(ctx, r, int64(len(data)))
			} else {
				_, _, err = Read(ctx, r, int64(len(data)), Options{})
			}
			cancel()
			if !errors.Is(err, context.Canceled) {
				t.Errorf("%s probe=%v: err = %v, want context.Canceled", name, probe, err)
			}
		}
	}
}

type failingReader struct {
	r    io.ReaderAt
	fail int64
	n    atomic.Int64
}

var errBoom = errors.New("boom")

func (f *failingReader) ReadAt(p []byte, off int64) (int, error) {
	if f.n.Add(1) >= f.fail {
		return 0, errBoom
	}
	return f.r.ReadAt(p, off)
}

func TestReaderErrorPropagates(t *testing.T) {
	bin := binarySTL("", bigTriangles(50_000, 3), nil, 0)
	asc := asciiSTL("x", [][]tri{bigTriangles(500, 4)}, asciiStyle{})
	old := asciiBlockBytes
	asciiBlockBytes = 5_000
	defer func() { asciiBlockBytes = old }()
	for name, data := range map[string][]byte{"binary": bin, "ascii": asc} {
		for fail := int64(1); fail <= 4; fail++ {
			_, _, err := Read(context.Background(), &failingReader{r: bytes.NewReader(data), fail: fail}, int64(len(data)), Options{})
			if !errors.Is(err, errBoom) {
				t.Errorf("%s fail=%d: err = %v", name, fail, err)
			}
		}
	}
}

func TestLargeBinaryAgainstOracle(t *testing.T) {
	ts := bigTriangles(200_003, 5)
	nan := float32(math.NaN())
	var want []tri
	attrs := make([]uint16, len(ts))
	for i := range ts {
		if i%977 == 5 {
			ts[i][i%9] = nan
		} else {
			want = append(want, ts[i])
		}
		attrs[i] = uint16(i * 7)
	}
	data := binarySTL("big", ts, attrs, 100)
	m, info := mustRead(t, data)
	if info.Triangles != len(want) || !sameFloats(m.V, flatten(want)) {
		t.Fatalf("triangles %d, want %d; equal %v", info.Triangles, len(want), sameFloats(m.V, flatten(want)))
	}
	if !info.Colors || len(m.C) != 3*len(want) {
		t.Fatalf("colours: %v %d", info.Colors, len(m.C))
	}
	lo, hi := m.Min, m.Max
	m.UpdateBounds()
	if lo != m.Min || hi != m.Max {
		t.Errorf("bounds differ from UpdateBounds")
	}
	pi, b, err := Probe(context.Background(), bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	if pi != info || b != [6]float32{lo[0], lo[1], lo[2], hi[0], hi[1], hi[2]} {
		t.Errorf("Probe %+v %v disagrees with Read %+v %v %v", pi, b, info, lo, hi)
	}
}

func TestASCIIBlockBoundaries(t *testing.T) {
	groups := [][]tri{bigTriangles(700, 6), cubeTris(1, 1, 1, 1), nil, bigTriangles(300, 7)}
	var want []tri
	for _, g := range groups {
		want = append(want, g...)
	}
	styles := []asciiStyle{{}, {crlf: true, upper: true}, {exp: true, tabs: true}, {cr: true, exp: true, noEnd: true}}
	old := asciiBlockBytes
	defer func() { asciiBlockBytes = old }()
	for si, st := range styles {
		data := asciiSTL("blocks", groups, st)
		asciiBlockBytes = 1 << 30
		ref, refInfo := mustRead(t, data)
		if !sameFloats(ref.V, flatten(want)) {
			t.Fatalf("style %d: reference parse differs from input", si)
		}
		if ref.Parts != 3 {
			t.Fatalf("style %d: reference Parts = %d", si, ref.Parts)
		}
		for _, bs := range []int64{1, 7, 37, 200, 1000, 4096, 5000, 100_000} {
			asciiBlockBytes = bs
			m, info := mustRead(t, data)
			if info != refInfo || m.Parts != ref.Parts || m.Min != ref.Min || m.Max != ref.Max || !sameFloats(m.V, ref.V) {
				t.Fatalf("style %d block %d: result differs from the single-block parse (%d triangles, parts %d)", si, bs, info.Triangles, m.Parts)
			}
			pi, b, err := Probe(context.Background(), bytes.NewReader(data), int64(len(data)))
			if err != nil || pi != refInfo || b != [6]float32{ref.Min[0], ref.Min[1], ref.Min[2], ref.Max[0], ref.Max[1], ref.Max[2]} {
				t.Fatalf("style %d block %d: Probe %+v %v %v", si, bs, pi, b, err)
			}
		}
	}
}

func TestASCIILongLineWithoutEndFacet(t *testing.T) {
	var b strings.Builder
	b.WriteString("solid x\n")
	for i := 0; i < 3000; i++ {
		fmt.Fprintf(&b, "facet normal 0 0 1\nouter loop\nvertex %d 0 0\nvertex 0 %d 0\nvertex 0 0 %d\nendloop\n", i+1, i+1, i+1)
	}
	old := asciiBlockBytes
	asciiBlockBytes = 1000
	defer func() { asciiBlockBytes = old }()
	_, info := mustRead(t, []byte(b.String()))
	if info.Triangles != 3000 {
		t.Errorf("triangles = %d, want 3000", info.Triangles)
	}
}

func TestNames(t *testing.T) {
	cases := []struct {
		data []byte
		want string
	}{
		{asciiSTL("  my part v2  ", [][]tri{cubeTris(0, 0, 0, 1)}, asciiStyle{}), "my part v2"},
		{asciiSTL("", [][]tri{cubeTris(0, 0, 0, 1)}, asciiStyle{}), ""},
		{binarySTL("solid", cubeTris(0, 0, 0, 1), nil, 0), ""},
		{binarySTL("Exported from Fusion 360", cubeTris(0, 0, 0, 1), nil, 0), "Exported from Fusion 360"},
		{binarySTL("solid thing\x00garbage", cubeTris(0, 0, 0, 1), nil, 0), "thing"},
		{binarySTL("  \x01\x02ctrl  ", cubeTris(0, 0, 0, 1), nil, 0), "ctrl"},
	}
	for i, c := range cases {
		_, info := mustRead(t, c.data)
		if info.Name != c.want {
			t.Errorf("case %d: name %q, want %q", i, info.Name, c.want)
		}
	}
	long := strings.Repeat("n", 1000)
	_, info := mustRead(t, asciiSTL(long, [][]tri{cubeTris(0, 0, 0, 1)}, asciiStyle{}))
	if len(info.Name) != maxNameBytes {
		t.Errorf("long name has %d bytes", len(info.Name))
	}
}

func TestParseFloat32AgainstStrconv(t *testing.T) {
	check := func(s string) {
		t.Helper()
		got := parseFloat32([]byte(s))
		f, err := strconv.ParseFloat(s, 32)
		if err != nil && !errors.Is(err, strconv.ErrRange) {
			if !math.IsNaN(float64(got)) {
				t.Errorf("%q: got %v, want NaN", s, got)
			}
			return
		}
		if math.Float32bits(got) != math.Float32bits(float32(f)) {
			t.Errorf("%q: got %v (%#x), want %v (%#x)", s, got, math.Float32bits(got), float32(f), math.Float32bits(float32(f)))
		}
	}
	for _, s := range []string{
		"0", "-0", "+0", "0.0", "-0.0e5", "1", "-1", "+1", ".5", "5.", "-.5", "1e5", "1E+05", "1.5e+02", "1e", "e5", "", "-", ".", "+.", "1.e1", "0.1", "0.3",
		"16777216", "16777217", "16777219", "33554435", "9007199254740993", "9007199254740992", "0.1234567890123456789012", "12345678901234567890",
		"1e-50", "1e-45", "1.4e-45", "7e-46", "1e38", "3.4028235e38", "3.4028236e38", "3.4028235677973366e38", "3.40282357e38", "1e39", "1e999", "-1e999", "1e-999",
		"1.17549435e-38", "1.1754942e-38", "nan", "-nan", "inf", "-inf", "Infinity", "1.#QNAN", "0x1p-2", "1_0", "1e0005", "1e00005", "123456e-3", "123456789e-9", "8388609", "8388609.5",
		"0.000000000000000000000000000000000000000000001", "100000000000000000000000", "1e22", "1e23", "1e-22", "1e-23", "4.35", "2.675", "1.00000017881393432617187499", "1.00000017881393432617187500", "1.00000017881393432617187501",
	} {
		check(s)
	}
	rng := rand.New(rand.NewSource(11))
	for i := 0; i < 300000; i++ {
		var f float32
		switch i % 4 {
		case 0:
			f = float32(rng.NormFloat64() * 100)
		case 1:
			f = math.Float32frombits(rng.Uint32())
			if !isFinite(f) {
				continue
			}
		case 2:
			f = float32(rng.Intn(2000000)-1000000) / float32(1+rng.Intn(1000))
		default:
			f = float32(rng.Float64())
		}
		for _, fm := range []struct {
			c byte
			p int
		}{{'e', -1}, {'e', 6}, {'e', 8}, {'f', -1}, {'g', 9}, {'e', 17}} {
			check(strconv.FormatFloat(float64(f), fm.c, fm.p, 32))
		}
	}
	for i := 0; i < 100000; i++ {
		digits := 1 + rng.Intn(25)
		var b strings.Builder
		if rng.Intn(2) == 0 {
			b.WriteByte('-')
		}
		dot := rng.Intn(digits + 1)
		for d := 0; d < digits; d++ {
			if d == dot {
				b.WriteByte('.')
			}
			b.WriteByte(byte('0' + rng.Intn(10)))
		}
		if rng.Intn(2) == 0 {
			fmt.Fprintf(&b, "e%+d", rng.Intn(100)-50)
		}
		check(b.String())
	}
}

func TestParallelismIsBounded(t *testing.T) {
	data := binarySTL("", bigTriangles(100_000, 8), nil, 0)
	before := runtime.NumGoroutine()
	if _, _, err := read(t, data, Options{}); err != nil {
		t.Fatal(err)
	}
	if after := runtime.NumGoroutine(); after > before+2 {
		t.Errorf("goroutines leaked: %d -> %d", before, after)
	}
}
