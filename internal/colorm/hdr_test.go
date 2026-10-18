package colorm

import (
	"image"
	"math"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
)

func hdrTransform(t *testing.T, transfer uint8, o Options) *Transform {
	t.Helper()
	p, err := FromCICP(codec.CICP{Primaries: 9, Transfer: transfer, FullRange: true})
	if err != nil {
		t.Fatal(err)
	}
	return mustTransform(t, p, SRGB(), o)
}

func deepPixel(r, g, b float64) *image.NRGBA64 {
	img := image.NewNRGBA64(image.Rect(0, 0, 1, 1))
	for i, v := range []float64{r, g, b, 1} {
		c := uint16(math.Round(clamp01(v) * 65535))
		img.Pix[2*i], img.Pix[2*i+1] = byte(c>>8), byte(c)
	}
	return img
}

func convertOne(t *testing.T, tf *Transform, r, g, b float64) [3]uint8 {
	t.Helper()
	out := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	if err := tf.Convert(out, deepPixel(r, g, b)); err != nil {
		t.Fatal(err)
	}
	return [3]uint8{out.Pix[0], out.Pix[1], out.Pix[2]}
}

func TestPQReferenceWhiteMapsToSDRWhite(t *testing.T) {
	tf := hdrTransform(t, 16, Options{NoDither: true})
	if !tf.HDR() {
		t.Fatal("PQ source must use the HDR path")
	}
	code := pqInverse(203.0 / 10000)
	if math.Abs(code-0.5806) > 5e-4 {
		t.Fatalf("PQ code of 203 nits is %.5f", code)
	}
	got := convertOne(t, tf, code, code, code)
	for c := 0; c < 3; c++ {
		if got[c] < 254 {
			t.Fatalf("203 nits became %v, want SDR white", got)
		}
	}
	if z := convertOne(t, tf, 0, 0, 0); z != [3]uint8{} {
		t.Fatalf("PQ zero became %v", z)
	}
	half := convertOne(t, tf, pqInverse(0.18*203/10000), pqInverse(0.18*203/10000), pqInverse(0.18*203/10000))
	want := srgbEncode(0.18) * 255
	if math.Abs(float64(half[0])-want) > 1.5 {
		t.Fatalf("18%% grey at 203 nits became %d want %.1f", half[0], want)
	}
}

func TestPQRampIsMonotone(t *testing.T) {
	tf := hdrTransform(t, 16, Options{NoDither: true})
	w := 4096
	deep := image.NewNRGBA64(image.Rect(0, 0, w, 1))
	for x := 0; x < w; x++ {
		v := uint16(x * 16)
		for c := 0; c < 3; c++ {
			deep.Pix[8*x+2*c], deep.Pix[8*x+2*c+1] = byte(v>>8), byte(v)
		}
		deep.Pix[8*x+6], deep.Pix[8*x+7] = 0xff, 0xff
	}
	out := image.NewNRGBA(deep.Rect)
	if err := tf.Convert(out, deep); err != nil {
		t.Fatal(err)
	}
	prev := uint8(0)
	for x := 0; x < w; x++ {
		v := out.Pix[4*x]
		if v < prev {
			t.Fatalf("ramp decreases at %d: %d after %d", x, v, prev)
		}
		if out.Pix[4*x+1] != v || out.Pix[4*x+2] != v {
			t.Fatalf("neutral ramp lost neutrality at %d: %v", x, out.Pix[4*x:4*x+3])
		}
		prev = v
	}
	if out.Pix[0] != 0 || out.Pix[4*(w-1)] != 255 {
		t.Fatalf("ramp ends: %d %d", out.Pix[0], out.Pix[4*(w-1)])
	}
}

func TestToneCurve(t *testing.T) {
	c := HDRConfig{}.withDefaults()
	for _, l := range []float64{0.001, 1, 20, 100, 202.9} {
		if got := c.ToneNits(l); math.Abs(got-l) > 1e-9*l+1e-12 {
			t.Errorf("below the knee %.3f changed to %.6f", l, got)
		}
	}
	if got := c.ToneNits(203); math.Abs(got-203) > 0.01 {
		t.Errorf("203 nits -> %.4f", got)
	}
	prev := 0.0
	for l := 0.0; l <= 10000; l += 7 {
		v := c.ToneNits(l)
		if v < prev-1e-9 {
			t.Fatalf("not monotone at %.0f nits: %.5f after %.5f", l, v, prev)
		}
		if v > c.Peak+1e-6 && l <= c.SourcePeak+1 {
			t.Fatalf("%.0f nits mapped above the display peak: %.3f", l, v)
		}
		prev = v
	}
	if got := c.ToneNits(c.SourcePeak); math.Abs(got-c.Peak) > 0.5 {
		t.Errorf("source peak maps to %.2f, want the display peak %.0f", got, c.Peak)
	}
	flat := HDRConfig{White: 203, Peak: 203, SourcePeak: 1000}
	if got := flat.ToneNits(1000); math.Abs(got-203) > 0.01 {
		t.Errorf("peak == white must clip to white, got %.3f", got)
	}
}

func TestPQSaturatedRedKeepsHue(t *testing.T) {
	tf := hdrTransform(t, 16, Options{NoDither: true})
	for _, level := range []float64{0.2, 0.35, 0.5, 0.58, 0.65, 0.75, 0.9} {
		low := level * 0.35
		got := convertOne(t, tf, level, low, low)
		if got[0] < got[1] || got[0] < got[2] {
			t.Fatalf("level %.2f: red lost its dominance: %v", level, got)
		}
		if got[2] < got[1] {
			t.Fatalf("level %.2f: channel order flipped: %v", level, got)
		}
	}
	prevR := uint8(0)
	for level := 0.1; level <= 0.95; level += 0.05 {
		got := convertOne(t, tf, level, 0.04, 0.04)
		if got[0] < prevR {
			t.Fatalf("red channel not monotone at %.2f", level)
		}
		prevR = got[0]
		if got[0] < got[1] || got[0] < got[2] {
			t.Fatalf("pure red at %.2f flipped to %v", level, got)
		}
	}
	bt2020Green := convertOne(t, tf, 0, pqInverse(203.0/10000), 0)
	if bt2020Green[1] < 200 || bt2020Green[0] > bt2020Green[1] || bt2020Green[2] > bt2020Green[1] {
		t.Fatalf("BT.2020 green mapped to %v", bt2020Green)
	}
}

func TestHLGReferenceWhite(t *testing.T) {
	tf := hdrTransform(t, 18, Options{NoDither: true})
	if !tf.HDR() {
		t.Fatal("HLG source must use the HDR path")
	}
	got := convertOne(t, tf, 0.75, 0.75, 0.75)
	for c := 0; c < 3; c++ {
		if got[c] < 253 {
			t.Fatalf("HLG 75%% became %v, want SDR white", got)
		}
	}
	if z := convertOne(t, tf, 0, 0, 0); z != [3]uint8{} {
		t.Fatalf("HLG zero became %v", z)
	}
	mid := convertOne(t, tf, 0.5, 0.5, 0.5)
	if mid[0] < 60 || mid[0] > 200 || mid[0] != mid[1] || mid[1] != mid[2] {
		t.Fatalf("HLG 50%% became %v", mid)
	}
	prev := uint8(0)
	for v := 0.0; v <= 1.0001; v += 1.0 / 128 {
		g := convertOne(t, tf, v, v, v)
		if g[0] < prev {
			t.Fatalf("HLG ramp not monotone at %.3f", v)
		}
		prev = g[0]
	}
}

func TestHLGTableMatchesFloatReference(t *testing.T) {
	tf := hdrTransform(t, 18, Options{NoDither: true})
	for _, c := range [][3]float64{{0.3, 0.4, 0.5}, {0.75, 0.2, 0.1}, {0.6, 0.6, 0.9}, {0.05, 0.5, 0.05}, {0.9, 0.9, 0.9}} {
		got := convertOne(t, tf, c[0], c[1], c[2])
		r, g, b := tf.Pixel(c[0], c[1], c[2])
		ref := [3]float64{r * 255, g * 255, b * 255}
		for i := 0; i < 3; i++ {
			if math.Abs(float64(got[i])-ref[i]) > 1.5 {
				t.Errorf("%v: channel %d table %d vs reference %.2f", c, i, got[i], ref[i])
			}
		}
	}
}

func TestPQTableMatchesFloatReference(t *testing.T) {
	tf := hdrTransform(t, 16, Options{NoDither: true})
	for _, c := range [][3]float64{{0.3, 0.4, 0.5}, {0.58, 0.2, 0.1}, {0.6, 0.6, 0.7}, {0.05, 0.5, 0.05}, {0.9, 0.9, 0.9}, {0.5, 0.5, 0.5}} {
		got := convertOne(t, tf, c[0], c[1], c[2])
		r, g, b := tf.Pixel(c[0], c[1], c[2])
		ref := [3]float64{r * 255, g * 255, b * 255}
		for i := 0; i < 3; i++ {
			if math.Abs(float64(got[i])-ref[i]) > 1.5 {
				t.Errorf("%v: channel %d table %d vs reference %.2f", c, i, got[i], ref[i])
			}
		}
	}
}

func TestHDREightBitInput(t *testing.T) {
	tf := hdrTransform(t, 16, Options{})
	img := one(148, 148, 148, 90)
	tf.Apply(img)
	want := convertOne(t, mustTransform(t, mustHDR(t), SRGB(), Options{NoDither: true}), 148.0/255, 148.0/255, 148.0/255)
	if absDiff(img.Pix[0], want[0]) > 1 || img.Pix[3] != 90 {
		t.Fatalf("8-bit PQ gave %v want %v", img.Pix, want)
	}
}

func mustHDR(t *testing.T) *Profile {
	p, err := FromCICP(codec.CICP{Primaries: 9, Transfer: 16})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestHDRToWideDisplayProfile(t *testing.T) {
	p3 := mustParse(t, displayP3())
	tf := hdrTransform(t, 16, Options{})
	if tf.dst != SRGB() {
		t.Fatal("dst")
	}
	wide := mustTransform(t, mustHDR(t), p3, Options{SoftClip: true})
	if !wide.HDR() {
		t.Fatal("HDR into a P3 display must stay on the HDR path")
	}
	code := pqInverse(203.0 / 10000)
	got := convertOne(t, wide, code, code, code)
	if got[0] < 253 || got[1] < 253 || got[2] < 253 {
		t.Fatalf("white on a P3 display: %v", got)
	}
	lut := mustParse(t, lutDisplay())
	if _, err := NewTransform(mustHDR(t), lut, Options{}); err == nil {
		t.Fatal("HDR into a LUT display profile is not supported and must say so")
	}
}

func TestICCWithCICPTagIsHDR(t *testing.T) {
	cicp := append([]byte("cicp"), 0, 0, 0, 0, 9, 16, 0, 1)
	data := buildICC(4, "mntr", "RGB ", "XYZ ", []testTag{
		{"desc", descV4("PQ")}, {"wtpt", xyzTag(d50)},
		{"rXYZ", xyzTag(srgbD50.r)}, {"gXYZ", xyzTag(srgbD50.g)}, {"bXYZ", xyzTag(srgbD50.b)},
		{"rTRC", srgbPara()}, {"gTRC", srgbPara()}, {"bTRC", srgbPara()},
		{"cicp", cicp},
	})
	p := mustParse(t, data)
	if p.HDR() != "PQ" || p.CICP() == nil || p.CICP().Primaries != 9 {
		t.Fatalf("cicp tag not read: %v %v", p.HDR(), p.CICP())
	}
}
