package colorm

import (
	"bytes"
	"image"
	"math"
	"math/rand/v2"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
)

func mustParse(t *testing.T, data []byte) *Profile {
	t.Helper()
	p, err := ParseICC(data)
	if err != nil {
		t.Fatalf("ParseICC: %v", err)
	}
	return p
}

func mustTransform(t *testing.T, src, dst *Profile, o Options) *Transform {
	t.Helper()
	tf, err := NewTransform(src, dst, o)
	if err != nil {
		t.Fatalf("NewTransform: %v", err)
	}
	return tf
}

func one(r, g, b, a uint8) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	copy(img.Pix, []uint8{r, g, b, a})
	return img
}

func absDiff(a, b uint8) int {
	if a > b {
		return int(a - b)
	}
	return int(b - a)
}

func randomImage(w, h int, seed uint64) *image.NRGBA {
	rng := rand.New(rand.NewPCG(seed, seed^0x9e3779b9))
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := range img.Pix {
		img.Pix[i] = uint8(rng.UintN(256))
	}
	return img
}

func maxDiff(a, b *image.NRGBA) int {
	d := 0
	for i := range a.Pix {
		if i%4 == 3 {
			if a.Pix[i] != b.Pix[i] {
				return 255
			}
			continue
		}
		d = max(d, absDiff(a.Pix[i], b.Pix[i]))
	}
	return d
}

func TestPrimariesMatchPublishedD50Values(t *testing.T) {
	m, ok := PrimariesToD50(0.64, 0.33, 0.30, 0.60, 0.15, 0.06, 0.3127, 0.3290)
	if !ok {
		t.Fatal("matrix")
	}
	want := srgbD50.matrix()
	if !m.near(want, 6e-4) {
		t.Fatalf("sRGB D50 matrix %v want %v", m, want)
	}
	p3, _ := PrimariesToD50(0.68, 0.32, 0.265, 0.69, 0.15, 0.06, 0.3127, 0.3290)
	if !p3.near(p3D50.matrix(), 1.5e-3) {
		t.Fatalf("P3 D50 matrix %v want %v", p3, p3D50.matrix())
	}
}

func TestParseProfiles(t *testing.T) {
	cases := []struct {
		name  string
		data  []byte
		desc  string
		major uint8
		space Space
	}{
		{"srgb v2 table", srgbV2(), "sRGB IEC61966-2.1", 2, SpaceRGB},
		{"srgb v4 para", srgbV4(), "sRGB v4", 4, SpaceRGB},
		{"p3", displayP3(), "Display P3", 4, SpaceRGB},
		{"adobe", adobeRGB(), "Adobe RGB (1998)", 2, SpaceRGB},
		{"gray", grayProfile(2.2), "Gray Gamma 2.2", 2, SpaceGray},
		{"cmyk", cmykProfile(5, true, false), "Test CMYK", 2, SpaceCMYK},
		{"lut display", lutDisplay(), "LUT display", 4, SpaceRGB},
	}
	for _, c := range cases {
		p := mustParse(t, c.data)
		if p.Description != c.desc || p.Major != c.major || p.Space != c.space {
			t.Errorf("%s: got %q v%d space %d", c.name, p.Description, p.Major, p.Space)
		}
		if p.Channels() != map[Space]int{SpaceRGB: 3, SpaceGray: 1, SpaceCMYK: 4}[c.space] {
			t.Errorf("%s: channels", c.name)
		}
	}
}

func TestParseRejectsBrokenProfiles(t *testing.T) {
	good := srgbV4()
	bad := map[string][]byte{
		"empty":        nil,
		"short":        good[:100],
		"no acsp":      append(append([]byte(nil), good[:36]...), append([]byte("xxxx"), good[40:]...)...),
		"size too big": func() []byte { b := append([]byte(nil), good...); b[3] = 0xff; b[2] = 0xff; return b }(),
		"huge tags":    func() []byte { b := append([]byte(nil), good...); b[128] = 0x7f; return b }(),
		"offset oob":   func() []byte { b := append([]byte(nil), good...); b[128+4+4+3] = 0xff; b[128+4+4+2] = 0xff; return b }(),
		"truncated":    good[:len(good)-30],
	}
	for name, data := range bad {
		if p, err := ParseICC(data); err == nil {
			t.Errorf("%s: parsed as %v", name, p.Description)
		}
	}
}

func TestIdentityIsByteExact(t *testing.T) {
	for name, src := range map[string][]byte{"v2 table": srgbV2(), "v4 para": srgbV4()} {
		tf := mustTransform(t, mustParse(t, src), SRGB(), Options{})
		if !tf.Identity() {
			t.Errorf("%s: sRGB profile did not collapse to identity", name)
		}
		img := randomImage(97, 53, 1)
		want := bytes.Clone(img.Pix)
		tf.Apply(img)
		if !bytes.Equal(img.Pix, want) {
			t.Errorf("%s: identity changed pixels", name)
		}
	}
	tf := mustTransform(t, SRGB(), SRGB(), Options{})
	if !tf.Identity() {
		t.Fatal("builtin sRGB to itself")
	}
}

func TestDisplayP3ToSRGB(t *testing.T) {
	p3 := mustParse(t, displayP3())
	tf := mustTransform(t, p3, SRGB(), Options{})
	if tf.Identity() {
		t.Fatal("P3 must not be identity")
	}
	red := one(255, 0, 0, 255)
	tf.Apply(red)
	if red.Pix[0] != 255 || red.Pix[1] != 0 || red.Pix[2] != 0 {
		t.Fatalf("P3 red hard-clipped to %v", red.Pix)
	}
	soft := mustTransform(t, p3, SRGB(), Options{SoftClip: true})
	sr := one(255, 0, 0, 255)
	soft.Apply(sr)
	if sr.Pix[0] != 255 || sr.Pix[1] < 25 || sr.Pix[2] < 15 || sr.Pix[1] > 120 {
		t.Fatalf("P3 red soft-clipped to %v", sr.Pix)
	}
	if sr.Pix[1] > sr.Pix[0] || sr.Pix[2] > sr.Pix[0] {
		t.Fatalf("soft clip lost the dominant channel: %v", sr.Pix)
	}
	for _, v := range []uint8{0, 17, 64, 128, 200, 255} {
		g := one(v, v, v, 255)
		tf.Apply(g)
		if absDiff(g.Pix[0], v) > 1 || absDiff(g.Pix[1], v) > 1 || absDiff(g.Pix[2], v) > 1 {
			t.Errorf("neutral %d became %v", v, g.Pix)
		}
	}
	inside := one(180, 120, 90, 77)
	tf.Apply(inside)
	if inside.Pix[3] != 77 {
		t.Fatal("alpha touched")
	}
	if inside.Pix[0] <= 180 || inside.Pix[1] >= 120 {
		t.Fatalf("P3 (180,120,90) should be more saturated in sRGB terms, got %v", inside.Pix)
	}
}

func TestRoundTripSRGBToP3AndBack(t *testing.T) {
	p3 := mustParse(t, displayP3())
	fwd := mustTransform(t, SRGB(), p3, Options{})
	back := mustTransform(t, p3, SRGB(), Options{})
	rng := rand.New(rand.NewPCG(7, 8))
	for i := 0; i < 5000; i++ {
		r, g, b := rng.Float64(), rng.Float64(), rng.Float64()
		r1, g1, b1 := fwd.Pixel(r, g, b)
		r2, g2, b2 := back.Pixel(r1, g1, b1)
		if math.Abs(r2-r)*255 > 0.05 || math.Abs(g2-g)*255 > 0.05 || math.Abs(b2-b)*255 > 0.05 {
			t.Fatalf("float round trip (%f %f %f) -> (%f %f %f)", r, g, b, r2, g2, b2)
		}
	}
	img := randomImage(64, 64, 3)
	for i := 0; i < len(img.Pix); i += 4 {
		for c := 0; c < 3; c++ {
			img.Pix[i+c] = uint8(40 + int(img.Pix[i+c])*175/255)
		}
	}
	orig := image.NewNRGBA(img.Rect)
	copy(orig.Pix, img.Pix)
	fwd.Apply(img)
	back.Apply(img)
	if d := maxDiff(orig, img); d > 2 {
		t.Fatalf("8-bit round trip differs by %d levels", d)
	}
}

func TestTablePathMatchesFloatReference(t *testing.T) {
	cases := map[string]*Transform{
		"p3->srgb":    mustTransform(t, mustParse(t, displayP3()), SRGB(), Options{}),
		"adobe->srgb": mustTransform(t, mustParse(t, adobeRGB()), SRGB(), Options{}),
		"srgb->p3":    mustTransform(t, SRGB(), mustParse(t, displayP3()), Options{}),
		"gray->srgb":  mustTransform(t, mustParse(t, grayProfile(2.2)), SRGB(), Options{}),
		"v2->adobe":   mustTransform(t, mustParse(t, srgbV2()), mustParse(t, adobeRGB()), Options{}),
	}
	for name, tf := range cases {
		img := randomImage(128, 64, 11)
		ref := image.NewNRGBA(img.Rect)
		for i := 0; i < len(img.Pix); i += 4 {
			r, g, b := tf.Pixel(float64(img.Pix[i])/255, float64(img.Pix[i+1])/255, float64(img.Pix[i+2])/255)
			ref.Pix[i], ref.Pix[i+1], ref.Pix[i+2], ref.Pix[i+3] = uint8(math.Round(r*255)), uint8(math.Round(g*255)), uint8(math.Round(b*255)), img.Pix[i+3]
		}
		tf.Apply(img)
		if d := maxDiff(ref, img); d > 1 {
			t.Errorf("%s: table path differs from reference by %d", name, d)
		}
	}
}

func TestAdobeRGBGrayRampMatchesFormula(t *testing.T) {
	tf := mustTransform(t, mustParse(t, adobeRGB()), SRGB(), Options{})
	for v := 0; v <= 255; v += 5 {
		img := one(uint8(v), uint8(v), uint8(v), 255)
		tf.Apply(img)
		want := srgbEncode(math.Pow(float64(v)/255, 2.19921875)) * 255
		for c := 0; c < 3; c++ {
			if math.Abs(float64(img.Pix[c])-want) > 1.01 {
				t.Fatalf("adobe gray %d: channel %d = %d want %.2f", v, c, img.Pix[c], want)
			}
		}
	}
}

func TestGrayProfileToSRGB(t *testing.T) {
	tf := mustTransform(t, mustParse(t, grayProfile(2.2)), SRGB(), Options{})
	for v := 0; v <= 255; v += 15 {
		img := one(uint8(v), uint8(v), uint8(v), 255)
		tf.Apply(img)
		want := srgbEncode(math.Pow(float64(v)/255, 2.2)) * 255
		for c := 0; c < 3; c++ {
			if math.Abs(float64(img.Pix[c])-want) > 1.01 {
				t.Fatalf("gray %d: channel %d = %d want %.2f", v, c, img.Pix[c], want)
			}
		}
	}
}

func TestLUTProfilesMatchMatrixProfiles(t *testing.T) {
	ref := mustParse(t, srgbV4())
	p3 := mustParse(t, displayP3())
	mabOnly := func(a2b []byte) []byte {
		return buildICC(4, "mntr", "RGB ", "XYZ ", []testTag{{"desc", descV4("mAB")}, {"wtpt", xyzTag(d50)}, {"A2B1", a2b}})
	}
	variants := map[string][]byte{
		"mAB matrix": mabOnly(mabMatrixA2B(srgbD50, srgbTable(1024))),
		"mft2 xyz":   mabOnly(mft2XYZMatrix(srgbD50, srgbDecode)),
		"mft1 lab":   labProfile(mft1Lab(17, srgbModel)),
		"mft2 lab":   labProfile(mft2Lab(33, srgbModel)),
	}
	target := mustTransform(t, ref, p3, Options{})
	for name, data := range variants {
		lp := mustParse(t, data)
		tf := mustTransform(t, lp, p3, Options{})
		tol := 1
		switch name {
		case "mft1 lab":
			tol = 8
		case "mft2 lab":
			tol = 3
		}
		img := randomImage(96, 64, 5)
		want := image.NewNRGBA(img.Rect)
		copy(want.Pix, img.Pix)
		target.Apply(want)
		tf.Apply(img)
		if d := maxDiff(want, img); d > tol {
			t.Errorf("%s: differs from the matrix profile by %d levels", name, d)
		}
	}
}

func TestLUTDestinationB2A(t *testing.T) {
	disp := mustParse(t, lutDisplay())
	p3 := mustParse(t, displayP3())
	matrixDisplay := mustParse(t, srgbV4())
	tf := mustTransform(t, p3, disp, Options{})
	want := mustTransform(t, p3, matrixDisplay, Options{})
	img := randomImage(80, 80, 21)
	for i := range img.Pix {
		if i%4 != 3 {
			img.Pix[i] = uint8(128 + (int(img.Pix[i])-128)/3)
		}
	}
	ref := image.NewNRGBA(img.Rect)
	copy(ref.Pix, img.Pix)
	tf.Apply(img)
	want.Apply(ref)
	if d := maxDiff(ref, img); d > 2 {
		t.Fatalf("B2A destination differs by %d", d)
	}
}

func TestPerceptualUsesA2B0(t *testing.T) {
	p := mustParse(t, perceptualPair())
	rel := mustTransform(t, p, SRGB(), Options{Intent: RelativeColorimetric})
	per := mustTransform(t, p, SRGB(), Options{Intent: Perceptual})
	a, b := one(200, 200, 200, 255), one(200, 200, 200, 255)
	rel.Apply(a)
	per.Apply(b)
	if absDiff(a.Pix[0], 200) > 1 {
		t.Fatalf("relative colorimetric must use A2B1 (identity here), got %v", a.Pix)
	}
	if b.Pix[0] >= a.Pix[0]-20 {
		t.Fatalf("perceptual must use the darker A2B0: rel %v per %v", a.Pix, b.Pix)
	}
}

func TestCMYKProfile(t *testing.T) {
	p := mustParse(t, cmykProfile(9, true, false))
	tf := mustTransform(t, p, SRGB(), Options{})
	img := image.NewCMYK(image.Rect(0, 0, 4, 1))
	copy(img.Pix, []uint8{0, 0, 0, 0, 255, 0, 0, 0, 0, 0, 0, 255, 128, 128, 128, 0})
	out := image.NewNRGBA(img.Rect)
	if err := tf.ConvertCMYK(out, img); err != nil {
		t.Fatal(err)
	}
	if out.Pix[0] < 250 || out.Pix[1] < 250 || out.Pix[2] < 250 {
		t.Errorf("paper white became %v", out.Pix[:4])
	}
	if out.Pix[4] > 40 || out.Pix[6] < 200 {
		t.Errorf("cyan became %v", out.Pix[4:8])
	}
	if out.Pix[8] > 6 || out.Pix[9] > 6 || out.Pix[10] > 6 {
		t.Errorf("K black must be mapped to display black by BPC, got %v", out.Pix[8:12])
	}
	for i := 3; i < 16; i += 4 {
		if out.Pix[i] != 255 {
			t.Errorf("alpha %d", out.Pix[i])
		}
	}
	for i := 0; i < 256; i += 17 {
		x := image.NewCMYK(image.Rect(0, 0, 1, 1))
		x.Pix[3] = uint8(i)
		o := image.NewNRGBA(x.Rect)
		tf.ConvertCMYK(o, x)
		y := image.NewCMYK(image.Rect(0, 0, 1, 1))
		y.Pix[3] = uint8(min(i+17, 255))
		o2 := image.NewNRGBA(y.Rect)
		tf.ConvertCMYK(o2, y)
		if o2.Pix[0] > o.Pix[0] {
			t.Fatalf("K ramp not monotone at %d: %d then %d", i, o.Pix[0], o2.Pix[0])
		}
	}
	mid := image.NewCMYK(image.Rect(0, 0, 1, 1))
	copy(mid.Pix, []uint8{90, 160, 40, 70})
	mo := image.NewNRGBA(mid.Rect)
	tf.ConvertCMYK(mo, mid)
	naive := [3]uint8{uint8(255 * (255 - 90) * (255 - 70) / 255 / 255), uint8(255 * (255 - 160) * (255 - 70) / 255 / 255), uint8(255 * (255 - 40) * (255 - 70) / 255 / 255)}
	if absDiff(mo.Pix[0], naive[0]) < 3 && absDiff(mo.Pix[1], naive[1]) < 3 && absDiff(mo.Pix[2], naive[2]) < 3 {
		t.Errorf("profile conversion equals the naive one %v", naive)
	}
	c, m, y, k := 90/255.0, 160/255.0, 40/255.0, 70/255.0
	bs := cmykModel(0, 0, 0, 1)
	x := cmykModel(c, m, y, k)
	ref := Vec3{}
	for i := 0; i < 3; i++ {
		a := (d50[i]) / (d50[i] - bs[i])
		ref[i] = a*x[i] - a*bs[i]
	}
	inv, _ := srgbD50.matrix().Inverse()
	lin := inv.Apply(ref)
	for i := 0; i < 3; i++ {
		want := srgbEncode(clamp01(lin[i])) * 255
		if math.Abs(float64(mo.Pix[i])-want) > 5 {
			t.Errorf("cmyk channel %d = %d want %.1f", i, mo.Pix[i], want)
		}
	}
}

func TestConvertDeepMatchesEightBitPath(t *testing.T) {
	p3 := mustParse(t, displayP3())
	tf := mustTransform(t, p3, SRGB(), Options{NoDither: true})
	img := randomImage(90, 70, 33)
	deep := image.NewNRGBA64(img.Rect)
	for i := 0; i < len(img.Pix); i += 4 {
		for c := 0; c < 4; c++ {
			v := uint16(img.Pix[i+c]) * 257
			deep.Pix[2*i+2*c], deep.Pix[2*i+2*c+1] = byte(v>>8), byte(v)
		}
	}
	out := image.NewNRGBA(img.Rect)
	if err := tf.Convert(out, deep); err != nil {
		t.Fatal(err)
	}
	tf.Apply(img)
	if d := maxDiff(img, out); d > 1 {
		t.Fatalf("deep and 8-bit paths differ by %d", d)
	}
}

func TestDeepIdentityRounding(t *testing.T) {
	tf := mustTransform(t, SRGB(), SRGB(), Options{NoDither: true})
	deep := image.NewNRGBA64(image.Rect(0, 0, 65536/16, 1))
	for x := 0; x < deep.Rect.Dx(); x++ {
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
	for x := 0; x < out.Rect.Dx(); x++ {
		want := uint8((uint32(x*16)*255 + 32767) / 65535)
		if out.Pix[4*x] != want || out.Pix[4*x+3] != 255 {
			t.Fatalf("x=%d got %d want %d", x, out.Pix[4*x], want)
		}
	}
	if out.Pix[0] != 0 || out.Pix[4*(out.Rect.Dx()-1)] != 255 {
		t.Fatal("ends")
	}
}

func TestDeepDitherKeepsMean(t *testing.T) {
	tf := mustTransform(t, SRGB(), SRGB(), Options{})
	const w = 64
	deep := image.NewNRGBA64(image.Rect(0, 0, w, w))
	v := uint16(0x8000 + 0x40)
	for i := 0; i < w*w; i++ {
		for c := 0; c < 3; c++ {
			deep.Pix[8*i+2*c], deep.Pix[8*i+2*c+1] = byte(v>>8), byte(v)
		}
		deep.Pix[8*i+6], deep.Pix[8*i+7] = 0xff, 0xff
	}
	out := image.NewNRGBA(deep.Rect)
	tf.Convert(out, deep)
	sum := 0
	seen := map[uint8]bool{}
	for i := 0; i < w*w; i++ {
		sum += int(out.Pix[4*i])
		seen[out.Pix[4*i]] = true
	}
	mean := float64(sum) / (w * w)
	want := float64(v) * 255 / 65535
	if math.Abs(mean-want) > 0.1 || len(seen) != 2 {
		t.Fatalf("mean %.3f want %.3f, levels %v", mean, want, seen)
	}
}

func TestTransformCache(t *testing.T) {
	a := mustTransform(t, mustParse(t, displayP3()), SRGB(), Options{})
	b := mustTransform(t, mustParse(t, displayP3()), SRGB(), Options{})
	if a != b {
		t.Fatal("same profiles must hit the cache")
	}
	c := mustTransform(t, mustParse(t, displayP3()), SRGB(), Options{SoftClip: true})
	if a == c {
		t.Fatal("different options must not share an entry")
	}
}

func TestFromCICPAgreesWithICC(t *testing.T) {
	cicp, err := FromCICP(codec.CICP{Primaries: 12, Transfer: 13, FullRange: true})
	if err != nil {
		t.Fatal(err)
	}
	a := mustTransform(t, cicp, SRGB(), Options{})
	b := mustTransform(t, mustParse(t, displayP3()), SRGB(), Options{})
	img := randomImage(64, 64, 9)
	other := image.NewNRGBA(img.Rect)
	copy(other.Pix, img.Pix)
	a.Apply(img)
	b.Apply(other)
	if d := maxDiff(img, other); d > 2 {
		t.Fatalf("CICP Display P3 differs from the ICC one by %d", d)
	}
}

func TestCICPCodePoints(t *testing.T) {
	for _, c := range []codec.CICP{
		{Primaries: 1, Transfer: 13}, {Primaries: 1, Transfer: 1}, {Primaries: 5, Transfer: 6}, {Primaries: 6, Transfer: 6},
		{Primaries: 9, Transfer: 14}, {Primaries: 9, Transfer: 16}, {Primaries: 9, Transfer: 18}, {Primaries: 11, Transfer: 17},
		{Primaries: 12, Transfer: 13}, {Primaries: 12, Transfer: 8}, {Primaries: 1, Transfer: 4}, {Primaries: 1, Transfer: 5},
		{Primaries: 2, Transfer: 2},
	} {
		p, err := FromCICP(c)
		if err != nil {
			t.Errorf("%+v: %v", c, err)
			continue
		}
		if _, err := NewTransform(p, SRGB(), Options{}); err != nil {
			t.Errorf("%+v: transform: %v", c, err)
		}
	}
	if _, err := FromCICP(codec.CICP{Primaries: 10, Transfer: 13}); err == nil {
		t.Error("XYZ primaries must be unsupported")
	}
	if _, err := FromCICP(codec.CICP{Primaries: 1, Transfer: 99}); err == nil {
		t.Error("unknown transfer must be unsupported")
	}
	bt709, _ := FromCICP(codec.CICP{Primaries: 1, Transfer: 1})
	tf := mustTransform(t, bt709, SRGB(), Options{})
	if tf.Identity() {
		t.Error("BT.709 transfer differs from sRGB in the shadows and must not be identity")
	}
}

func TestLinearTransferGivesBrighterDisplay(t *testing.T) {
	lin, _ := FromCICP(codec.CICP{Primaries: 1, Transfer: 8})
	tf := mustTransform(t, lin, SRGB(), Options{})
	img := one(128, 128, 128, 255)
	tf.Apply(img)
	want := srgbEncode(128.0/255) * 255
	if math.Abs(float64(img.Pix[0])-want) > 1 {
		t.Fatalf("linear 0.5 -> %d want %.1f", img.Pix[0], want)
	}
}

func TestParallelApplyIsDeterministic(t *testing.T) {
	tf := mustTransform(t, mustParse(t, adobeRGB()), SRGB(), Options{})
	a := randomImage(700, 300, 4)
	b := image.NewNRGBA(a.Rect)
	copy(b.Pix, a.Pix)
	tf.Apply(a)
	for y := 0; y < 300; y++ {
		row := image.NewNRGBA(image.Rect(0, 0, 700, 1))
		copy(row.Pix, b.Pix[y*b.Stride:(y+1)*b.Stride])
		tf.Apply(row)
		if !bytes.Equal(row.Pix, a.Pix[y*a.Stride:(y+1)*a.Stride]) {
			t.Fatalf("row %d differs between banded and single-row application", y)
		}
	}
}

func TestSubImageApply(t *testing.T) {
	tf := mustTransform(t, mustParse(t, displayP3()), SRGB(), Options{})
	full := randomImage(50, 40, 6)
	sub := full.SubImage(image.Rect(10, 5, 30, 25)).(*image.NRGBA)
	before := bytes.Clone(full.Pix)
	tf.Apply(sub)
	for y := 0; y < 40; y++ {
		for x := 0; x < 50; x++ {
			o := full.PixOffset(x, y)
			inside := x >= 10 && x < 30 && y >= 5 && y < 25
			same := bytes.Equal(full.Pix[o:o+4], before[o:o+4])
			if !inside && !same {
				t.Fatalf("pixel outside the sub image changed at %d,%d", x, y)
			}
		}
	}
	if bytes.Equal(full.Pix, before) {
		t.Fatal("nothing converted")
	}
}
