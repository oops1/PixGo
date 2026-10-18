package colorm

import (
	"image"
	"testing"
	"time"

	"github.com/oops1/PixGo/internal/codec"
)

func newManager(cfg Config, provider DisplayProvider) *Manager {
	m := NewManager(provider)
	m.Configure(cfg)
	return m
}

func TestManagerDisabledLeavesEightBitAlone(t *testing.T) {
	m := newManager(Config{}, nil)
	if m.Enabled() {
		t.Fatal("zero config must be disabled")
	}
	conv := m.Resolve(codec.ColorInfo{ICC: displayP3()})
	if conv.Needed() || conv.Report.Managed {
		t.Fatalf("disabled manager planned a conversion: %+v", conv.Report)
	}
	img := randomImage(20, 20, 2)
	want := append([]byte(nil), img.Pix...)
	conv.Tile(img)
	for i := range want {
		if want[i] != img.Pix[i] {
			t.Fatal("pixels changed")
		}
	}
}

func TestManagerResolvesEmbeddedProfile(t *testing.T) {
	m := newManager(Config{Enabled: true}, nil)
	conv := m.Resolve(codec.ColorInfo{ICC: displayP3()})
	r := conv.Report
	if !r.Managed || r.Profile != "Display P3" || r.Origin != OriginEmbedded || !r.Converted || r.HDR != "" {
		t.Fatalf("report %+v", r)
	}
	srgb := m.Resolve(codec.ColorInfo{ICC: srgbV2()})
	if srgb.Needed() || srgb.Report.Profile != "sRGB IEC61966-2.1" || srgb.Report.Converted {
		t.Fatalf("sRGB profile report %+v", srgb.Report)
	}
	none := m.Resolve(codec.ColorInfo{})
	if none.Needed() || none.Report.Origin != OriginNone || none.Report.Profile != "" {
		t.Fatalf("untagged report %+v", none.Report)
	}
}

func TestManagerICCWinsOverCICPAndFallsBack(t *testing.T) {
	m := newManager(Config{Enabled: true}, nil)
	both := m.Resolve(codec.ColorInfo{ICC: srgbV4(), CICP: &codec.CICP{Primaries: 12, Transfer: 13}})
	if both.Report.Origin != OriginEmbedded || both.Needed() {
		t.Fatalf("ICC must win: %+v", both.Report)
	}
	bad := m.Resolve(codec.ColorInfo{ICC: []byte("garbage"), CICP: &codec.CICP{Primaries: 12, Transfer: 13}})
	if bad.Report.Origin != OriginCICP || !bad.Needed() || bad.Report.Profile != "Display P3 / sRGB" {
		t.Fatalf("broken ICC must fall back to CICP: %+v", bad.Report)
	}
	unknown := m.Resolve(codec.ColorInfo{CICP: &codec.CICP{Primaries: 10, Transfer: 13}})
	if unknown.Needed() || unknown.Report.Origin != OriginNone {
		t.Fatalf("unsupported CICP must read as sRGB: %+v", unknown.Report)
	}
}

func TestManagerHDRReport(t *testing.T) {
	m := newManager(Config{Enabled: true}, nil)
	for transfer, name := range map[uint8]string{16: "PQ", 18: "HLG"} {
		c := m.Resolve(codec.ColorInfo{CICP: &codec.CICP{Primaries: 9, Transfer: transfer}})
		if c.Report.HDR != name || !c.Report.ToneMapped || !c.Needed() {
			t.Fatalf("%s: %+v", name, c.Report)
		}
	}
}

func TestManagerConvertsDeepFramesAndDropsThem(t *testing.T) {
	m := newManager(Config{Enabled: true}, nil)
	img := &codec.Image{
		Color: codec.ColorInfo{CICP: &codec.CICP{Primaries: 9, Transfer: 16}},
		Frames: []codec.Frame{
			{Deep: deepPixel(0.5806, 0.5806, 0.5806)},
			{Pix: one(10, 20, 30, 255)},
		},
	}
	rep, err := m.ConvertImage(img)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Deep || img.Frames[0].Deep != nil || img.Frames[0].Pix == nil {
		t.Fatalf("frame 0 not converted: %+v", img.Frames[0])
	}
	if img.Frames[0].Pix.Pix[0] < 250 {
		t.Fatalf("203 nits became %v", img.Frames[0].Pix.Pix)
	}
	if img.Frames[1].Pix.Pix[0] == 10 && img.Frames[1].Pix.Pix[1] == 20 {
		t.Fatalf("8-bit PQ frame untouched: %v", img.Frames[1].Pix.Pix)
	}
}

func TestDisabledManagerStillConvertsStrayDeepFrames(t *testing.T) {
	m := newManager(Config{}, nil)
	img := &codec.Image{Frames: []codec.Frame{{Deep: deepPixel(1, 0.5, 0)}}}
	if _, err := m.ConvertImage(img); err != nil {
		t.Fatal(err)
	}
	f := img.Frames[0]
	if f.Deep != nil || f.Pix == nil || f.Pix.Pix[0] != 255 || f.Pix.Pix[2] != 0 || f.Pix.Pix[3] != 255 {
		t.Fatalf("%+v", f)
	}
	var nilManager *Manager
	if nilManager.Enabled() || nilManager.Stamp() != 0 {
		t.Fatal("nil manager")
	}
	if c := nilManager.Resolve(codec.ColorInfo{}); c.Needed() {
		t.Fatal("nil manager planned work")
	}
}

func TestManagerSystemProfileAndStamp(t *testing.T) {
	now := time.Unix(1000, 0)
	icc := adobeRGB()
	name := "Monitor"
	m := NewManager(func() ([]byte, string) { return icc, name })
	m.now = func() time.Time { return now }
	m.Configure(Config{Enabled: true, SystemProfile: true})
	untagged := m.Resolve(codec.ColorInfo{})
	if !untagged.Needed() || untagged.Report.Display != "Monitor" {
		t.Fatalf("untagged sRGB on an Adobe RGB display must convert: %+v", untagged.Report)
	}
	s1 := m.Stamp()
	icc = displayP3()
	if m.Stamp() != s1 {
		t.Fatal("stamp must not change before the refresh interval")
	}
	now = now.Add(2 * displayRefresh)
	s2 := m.Stamp()
	if s2 == s1 {
		t.Fatal("new display profile must change the stamp")
	}
	if p, _ := m.Display(); p.Description != "Display P3" {
		t.Fatalf("display %q", p.Description)
	}
	m.Configure(Config{Enabled: true, SystemProfile: false})
	if m.Stamp() == s2 {
		t.Fatal("switching to sRGB must change the stamp")
	}
	if p, n := m.Display(); p != SRGB() || n != "" {
		t.Fatal("sRGB display expected")
	}
	if m.Resolve(codec.ColorInfo{}).Needed() {
		t.Fatal("untagged on an sRGB display needs nothing")
	}
}

func TestManagerBrokenDisplayProfileFallsBackToSRGB(t *testing.T) {
	m := NewManager(func() ([]byte, string) { return []byte("not a profile"), "Broken" })
	m.Configure(Config{Enabled: true, SystemProfile: true})
	if p, n := m.Display(); p != SRGB() || n != "" {
		t.Fatal("expected sRGB")
	}
	cmykDisplay := NewManager(func() ([]byte, string) { return cmykProfile(5, true, false), "CMYK" })
	cmykDisplay.Configure(Config{Enabled: true, SystemProfile: true})
	if p, _ := cmykDisplay.Display(); p != SRGB() {
		t.Fatal("a CMYK display profile must be ignored")
	}
}

func TestManagerCMYKProfileIsNotAppliedToRGBPixels(t *testing.T) {
	m := newManager(Config{Enabled: true}, nil)
	ci := codec.ColorInfo{ICC: cmykProfile(5, true, false)}
	conv := m.Resolve(ci)
	if conv.Needed() || !conv.Report.CMYK {
		t.Fatalf("%+v", conv.Report)
	}
	tf, ok := m.ResolveCMYK(ci)
	if !ok || tf == nil {
		t.Fatal("CMYK transform expected")
	}
	img := image.NewCMYK(image.Rect(0, 0, 2, 2))
	out := image.NewNRGBA(img.Rect)
	if err := tf.ConvertCMYK(out, img); err != nil || out.Pix[0] < 250 {
		t.Fatalf("%v %v", err, out.Pix)
	}
	if _, ok := m.ResolveCMYK(codec.ColorInfo{ICC: displayP3()}); ok {
		t.Fatal("RGB profile is not a CMYK source")
	}
}

func TestChannelMismatchErrors(t *testing.T) {
	tf := mustTransform(t, mustParse(t, cmykProfile(5, true, false)), SRGB(), Options{})
	if err := tf.Convert(image.NewNRGBA(image.Rect(0, 0, 1, 1)), image.NewNRGBA64(image.Rect(0, 0, 1, 1))); err == nil {
		t.Fatal("CMYK transform must refuse RGB input")
	}
	rgb := mustTransform(t, mustParse(t, displayP3()), SRGB(), Options{})
	if err := rgb.ConvertCMYK(image.NewNRGBA(image.Rect(0, 0, 1, 1)), image.NewCMYK(image.Rect(0, 0, 1, 1))); err == nil {
		t.Fatal("RGB transform must refuse CMYK input")
	}
	if err := rgb.Convert(image.NewNRGBA(image.Rect(0, 0, 2, 2)), image.NewNRGBA64(image.Rect(0, 0, 1, 1))); err == nil {
		t.Fatal("size mismatch must fail")
	}
}

func TestDisplayICCIsHandedOutOnlyForAConvertingDisplay(t *testing.T) {
	icc := displayP3()
	m := NewManager(func() ([]byte, string) { return icc, "Display P3" })
	m.Configure(Config{Enabled: true, SystemProfile: true})
	got := m.DisplayICC()
	if len(got) != len(icc) || string(got) != string(icc) {
		t.Fatalf("display profile bytes %d, want %d", len(got), len(icc))
	}
	got[0] ^= 0xff
	if again := m.DisplayICC(); again[0] == got[0] {
		t.Fatal("the bytes must be a copy")
	}
	m.Configure(Config{Enabled: true, SystemProfile: false})
	if m.DisplayICC() != nil {
		t.Fatal("an sRGB display needs no profile in saved files")
	}
	m.Configure(Config{Enabled: false, SystemProfile: true})
	if m.DisplayICC() != nil {
		t.Fatal("without colour management nothing is converted, so nothing is tagged")
	}
}
