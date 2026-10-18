package colorm

import (
	"image"
	"math"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
)

func parseCurve(t *testing.T, b []byte) *Curve {
	t.Helper()
	c, n, err := parseCurveElement(b)
	if err != nil || n != len(b) {
		t.Fatalf("parse: %v consumed %d of %d", err, n, len(b))
	}
	return c
}

func TestParametricCurveTypes(t *testing.T) {
	cases := []struct {
		name string
		tag  []byte
		f    func(x float64) float64
	}{
		{"type 0", paraTag(0, 2.2), func(x float64) float64 { return math.Pow(x, 2.2) }},
		{"type 1", paraTag(1, 2.0, 0.9, 0.1), func(x float64) float64 {
			if x >= -0.1/0.9 {
				return math.Pow(0.9*x+0.1, 2)
			}
			return 0
		}},
		{"type 2", paraTag(2, 2.0, 0.9, 0.1, 0.05), func(x float64) float64 { return math.Pow(0.9*x+0.1, 2) + 0.05 }},
		{"type 3", paraTag(3, 2.4, 1/1.055, 0.055/1.055, 1/12.92, 0.04045), srgbDecode},
		{"type 4", paraTag(4, 2.4, 1/1.055, 0.055/1.055, 1/12.92, 0.04045, 0, 0), srgbDecode},
	}
	for _, c := range cases {
		cv := parseCurve(t, c.tag)
		for x := 0.0; x <= 1; x += 0.05 {
			want := c.f(x)
			if c.name == "type 2" {
				want = math.Min(want, 1)
			}
			if got := cv.Eval(x); math.Abs(got-clamp01(want)) > 2e-4 {
				t.Errorf("%s: f(%.2f)=%.5f want %.5f", c.name, x, got, want)
			}
		}
		for y := 0.0; y <= 1; y += 0.1 {
			x := cv.Inverse(y)
			if c.name == "type 2" || c.name == "type 1" {
				continue
			}
			if math.Abs(cv.Eval(x)-y) > 1e-3 {
				t.Errorf("%s: inverse(%.2f)=%.5f does not round trip", c.name, y, x)
			}
		}
	}
	if _, _, err := parseCurveElement(paraTag(7, 1)); err == nil {
		t.Error("unknown parametric type accepted")
	}
	if c := parseCurve(t, paraTag(0, 0)); !c.IsIdentity() && c.Eval(0.5) != 0.5 {
		t.Error("gamma 0 must not blow up")
	}
}

func TestCurveTables(t *testing.T) {
	up := parseCurve(t, curvTable([]float64{0, 0.1, 0.1, 0.5, 1}))
	if got := up.Eval(0.5); math.Abs(got-0.1) > 1e-4 {
		t.Errorf("table eval %f", got)
	}
	if x := up.Inverse(0.3); x < 0.5 || x > 0.75 {
		t.Errorf("inverse across a flat part: %f", x)
	}
	down := parseCurve(t, curvTable([]float64{1, 0.5, 0}))
	if x := down.Inverse(0.5); math.Abs(x-0.5) > 1e-3 {
		t.Errorf("decreasing inverse %f", x)
	}
	if x := down.Inverse(2); x != 0 {
		t.Errorf("out of range inverse %f", x)
	}
	id := parseCurve(t, curvTable([]float64{0, 0.5, 1}))
	if !id.IsIdentity() {
		t.Error("linear table is the identity")
	}
	if !(*Curve)(nil).IsIdentity() || (*Curve)(nil).Eval(0.3) != 0.3 || (*Curve)(nil).Inverse(0.3) != 0.3 {
		t.Error("nil curve")
	}
	if g := parseCurve(t, curvGamma(1)); !g.IsIdentity() {
		t.Error("gamma 1")
	}
	if _, _, err := parseCurveElement([]byte("curv\x00\x00\x00\x00\xff\xff\xff\xff")); err == nil {
		t.Error("huge table accepted")
	}
	if _, _, err := parseCurveElement([]byte("zzzz0000000000")); err == nil {
		t.Error("unknown element accepted")
	}
}

func TestTransferFunctionsRoundTrip(t *testing.T) {
	for name, pair := range map[string][2]func(float64) float64{
		"srgb": {srgbDecode, srgbEncode}, "bt709": {bt709Decode, bt709Encode},
		"pq": {pqEOTF, pqInverse}, "hlg": {hlgOETFInverse, hlgOETF},
	} {
		for x := 0.0; x <= 1; x += 0.01 {
			if y := pair[1](pair[0](x)); math.Abs(y-x) > 1e-9 {
				t.Errorf("%s: %f -> %f", name, x, y)
			}
		}
	}
	if pqEOTF(1)*pqPeakNits < 9999 || pqEOTF(1)*pqPeakNits > 10001 {
		t.Errorf("PQ peak %f", pqEOTF(1)*pqPeakNits)
	}
	if hlgOETFInverse(1) < 0.99 || hlgOETFInverse(1) > 1.01 {
		t.Errorf("HLG white %f", hlgOETFInverse(1))
	}
}

func TestDescriptionAndTagReaders(t *testing.T) {
	p := mustParse(t, srgbV4())
	if p.Description != "sRGB v4" {
		t.Fatal(p.Description)
	}
	data := buildICC(4, "mntr", "RGB ", "XYZ ", []testTag{
		{"desc", descV4("Wide gamut")}, {"wtpt", xyzTag(d50)}, {"bkpt", xyzTag(Vec3{0.001, 0.001, 0.001})},
		{"chad", chadTag(identity3)},
		{"rXYZ", xyzTag(p3D50.r)}, {"gXYZ", xyzTag(p3D50.g)}, {"bXYZ", xyzTag(p3D50.b)},
		{"rTRC", srgbPara()}, {"gTRC", srgbPara()}, {"bTRC", srgbPara()},
	})
	q := mustParse(t, data)
	if _, ok := q.Adaptation(); !ok {
		t.Error("chad")
	}
	if w, ok := q.MediaWhite(); !ok || math.Abs(w[0]-0.9642) > 1e-3 {
		t.Error("wtpt")
	}
	if _, ok := p.Adaptation(); ok {
		t.Error("no chad in this profile")
	}
	if _, ok := p.MediaWhite(); !ok {
		t.Error("wtpt")
	}
	cicp, _ := FromCICP(codec.CICP{Primaries: 12, Transfer: 13})
	if _, ok := cicp.Adaptation(); !ok || cicp.CICP() == nil || cicp.HDR() != "" || cicp.IsHDR() {
		t.Error("CICP profile metadata")
	}
	if d := cleanText("a\x00b"); d != "a" {
		t.Errorf("clean %q", d)
	}
	if d := cleanText(string(make([]byte, 0)) + "ctl\x01xé"); d != "ctlxé" {
		t.Errorf("clean %q", d)
	}
}

func TestPQAndHLGDeepLUTAndWrappedTiles(t *testing.T) {
	lut := mustParse(t, labProfile(mft2Lab(17, srgbModel)))
	tf := mustTransform(t, lut, SRGB(), Options{NoDither: true})
	deep := image.NewNRGBA64(image.Rect(0, 0, 2, 1))
	for i, v := range []uint16{65535, 65535, 65535, 65535, 0, 0, 0, 65535} {
		deep.Pix[2*i], deep.Pix[2*i+1] = byte(v>>8), byte(v)
	}
	out := image.NewNRGBA(deep.Rect)
	if err := tf.Convert(out, deep); err != nil {
		t.Fatal(err)
	}
	if out.Pix[0] < 252 || out.Pix[4] > 3 {
		t.Fatalf("LUT deep white %v black %v", out.Pix[:3], out.Pix[4:7])
	}
	if x, y, z := tf.Pixel(1, 1, 1); math.Abs(x-1) > 0.02 || math.Abs(y-1) > 0.02 || math.Abs(z-1) > 0.02 {
		t.Errorf("pixel reference %f %f %f", x, y, z)
	}
	cm := mustTransform(t, mustParse(t, cmykProfile(5, true, false)), SRGB(), Options{})
	if r, g, b := cm.PixelCMYK(0, 0, 0, 0); r < 0.95 || g < 0.95 || b < 0.95 {
		t.Errorf("CMYK paper %f %f %f", r, g, b)
	}
}
