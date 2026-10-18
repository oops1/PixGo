package jpeg

import (
	"bytes"
	"context"
	"image"
	"image/color"
	stdjpeg "image/jpeg"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/testimg"
)

func withICC(t testing.TB, jpegData, profile []byte, chunk int) []byte {
	t.Helper()
	if len(profile) == 0 {
		return jpegData
	}
	count := (len(profile) + chunk - 1) / chunk
	var segments []byte
	for i := 0; i < count; i++ {
		part := profile[i*chunk : min(len(profile), (i+1)*chunk)]
		segments = appendSegment(segments, 0xe2, iccHeader, []byte{byte(i + 1), byte(count)}, part)
	}
	out := append([]byte(nil), jpegData[:2]...)
	out = append(out, segments...)
	return append(out, jpegData[2:]...)
}

func setAdobeTransform(t testing.TB, data []byte, transform byte) []byte {
	t.Helper()
	at := bytes.Index(data, []byte("Adobe"))
	if at < 0 {
		t.Fatal("no Adobe marker")
	}
	out := append([]byte(nil), data...)
	out[at+11] = transform
	return out
}

func cmykFiles(t testing.TB) map[string][]byte {
	files, _ := corpus(t)
	src := files["video-001.cmyk.jpeg"]
	if src == nil {
		t.Fatal("no CMYK file in the corpus")
	}
	return map[string][]byte{
		"cmyk": setAdobeTransform(t, src, 0),
		"ycck": setAdobeTransform(t, src, 2),
	}
}

func standardInk(t testing.TB, data []byte) *image.CMYK {
	t.Helper()
	img, err := stdjpeg.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	ink, ok := img.(*image.CMYK)
	if !ok {
		t.Fatalf("standard library returned %T", img)
	}
	return ink
}

func inkAsRGB(src *image.CMYK) *image.NRGBA {
	b := src.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			p := src.CMYKAt(b.Min.X+x, b.Min.Y+y)
			r, g, bl := color.CMYKToRGB(p.C, p.M, p.Y, p.K)
			out.SetNRGBA(x, y, color.NRGBA{R: r, G: g, B: bl, A: 255})
		}
	}
	return out
}

func sameInk(a, b *image.CMYK) bool {
	if a.Bounds() != b.Bounds() {
		return false
	}
	w := a.Bounds().Dx() * 4
	for y := 0; y < a.Bounds().Dy(); y++ {
		if !bytes.Equal(a.Pix[y*a.Stride:y*a.Stride+w], b.Pix[y*b.Stride:y*b.Stride+w]) {
			return false
		}
	}
	return true
}

func deepOptions() codec.Options {
	return codec.Options{Limits: codec.DefaultLimits(), Deep: true}
}

func TestCMYKWithAProfileAndDeepReturnsTheInkSamples(t *testing.T) {
	profile := testimg.SampleICCSpace("CMYK", 3000)
	for name, base := range cmykFiles(t) {
		for _, chunk := range []int{len(profile), 700} {
			data := withICC(t, base, profile, chunk)
			img, err := decodeWith(t, data, deepOptions())
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			frame := img.Frames[0]
			if frame.CMYK == nil || frame.Pix != nil || frame.Deep != nil {
				t.Fatalf("%s: frame %v %v %v", name, frame.Pix != nil, frame.Deep != nil, frame.CMYK != nil)
			}
			if !bytes.Equal(img.Color.ICC, profile) || img.Color.CICP != nil {
				t.Fatalf("%s: colour %d bytes", name, len(img.Color.ICC))
			}
			if !sameInk(frame.CMYK, standardInk(t, data)) {
				t.Fatalf("%s: ink differs from the standard library", name)
			}
			if img.Info.Width != frame.CMYK.Bounds().Dx() || img.Info.Height != frame.CMYK.Bounds().Dy() || img.Info.BitDepth != 32 || img.Scale != 1 {
				t.Fatalf("%s: %+v scale %d", name, img.Info, img.Scale)
			}
		}
	}
}

func TestCMYKWithoutDeepKeepsTheNaiveConversion(t *testing.T) {
	profile := testimg.SampleICCSpace("CMYK", 3000)
	for name, base := range cmykFiles(t) {
		plain, err := decodeWith(t, base, codec.Options{Limits: codec.DefaultLimits()})
		if err != nil {
			t.Fatal(err)
		}
		withProfile, err := decodeWith(t, withICC(t, base, profile, len(profile)), codec.Options{Limits: codec.DefaultLimits()})
		if err != nil {
			t.Fatal(err)
		}
		frame := withProfile.Frames[0]
		if frame.CMYK != nil || frame.Deep != nil || frame.Pix == nil || !withProfile.Color.Empty() {
			t.Fatalf("%s: a profile changed the colour management free path", name)
		}
		if !samePixels(frame.Pix, plain.Frames[0].Pix) {
			t.Fatalf("%s: pixels differ with a profile present", name)
		}
		if !samePixels(frame.Pix, inkAsRGB(standardInk(t, base))) {
			t.Fatalf("%s: naive conversion changed", name)
		}
	}
}

func TestCMYKWithDeepButNoCMYKProfileKeepsTheNaiveConversion(t *testing.T) {
	for name, base := range cmykFiles(t) {
		for label, profile := range map[string][]byte{
			"none": nil,
			"rgb":  testimg.SampleICCSpace("RGB ", 500),
			"gray": testimg.SampleICCSpace("GRAY", 500),
		} {
			data := withICC(t, base, profile, 1<<15)
			img, err := decodeWith(t, data, deepOptions())
			if err != nil {
				t.Fatalf("%s/%s: %v", name, label, err)
			}
			frame := img.Frames[0]
			if frame.CMYK != nil || frame.Deep != nil || frame.Pix == nil {
				t.Fatalf("%s/%s: not a plain picture", name, label)
			}
			if !samePixels(frame.Pix, inkAsRGB(standardInk(t, base))) {
				t.Fatalf("%s/%s: naive conversion changed", name, label)
			}
			if label == "none" && !img.Color.Empty() {
				t.Fatalf("%s: %+v", name, img.Color)
			}
		}
	}
}

func TestOtherJPEGsAreNotAffectedByDeep(t *testing.T) {
	files, names := corpus(t)
	for _, name := range names {
		if name == "video-001.cmyk.jpeg" || len(files[name]) == 0 {
			continue
		}
		plain, err := decodeWith(t, files[name], codec.Options{Limits: codec.DefaultLimits()})
		if err != nil {
			continue
		}
		deep, err := decodeWith(t, files[name], deepOptions())
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if deep.Frames[0].CMYK != nil || deep.Frames[0].Deep != nil || !samePixels(deep.Frames[0].Pix, plain.Frames[0].Pix) {
			t.Fatalf("%s: Deep changed an RGB or grey picture", name)
		}
	}
}

func TestCMYKScaledAndRegionDecodesFollowTheNaiveOnes(t *testing.T) {
	profile := testimg.SampleICCSpace("CMYK", 200)
	for name, base := range cmykFiles(t) {
		data := withICC(t, base, profile, len(profile))
		for _, scale := range []int{2, 4, 8} {
			naive, err := decodeWith(t, base, codec.Options{Limits: codec.DefaultLimits(), Scale: scale})
			if err != nil {
				t.Fatal(err)
			}
			o := deepOptions()
			o.Scale = scale
			ink, err := decodeWith(t, data, o)
			if err != nil {
				t.Fatal(err)
			}
			if ink.Scale != naive.Scale || ink.Frames[0].CMYK == nil {
				t.Fatalf("%s scale %d: got scale %d, ink %v", name, scale, ink.Scale, ink.Frames[0].CMYK != nil)
			}
			if !samePixels(inkAsRGB(ink.Frames[0].CMYK), naive.Frames[0].Pix) {
				t.Fatalf("%s scale %d: ink and naive decodes disagree", name, scale)
			}
		}
		whole, err := decodeWith(t, data, deepOptions())
		if err != nil {
			t.Fatal(err)
		}
		b := whole.Frames[0].CMYK.Bounds()
		region := image.Rect(b.Dx()/4, b.Dy()/3, b.Dx()*3/4, b.Dy()*2/3)
		o := deepOptions()
		o.Region = &region
		part, err := decodeWith(t, data, o)
		if err != nil {
			t.Fatal(err)
		}
		got := part.Frames[0].CMYK
		if got == nil || got.Bounds() != image.Rect(0, 0, region.Dx(), region.Dy()) {
			t.Fatalf("%s: region frame %v", name, got)
		}
		for y := 0; y < region.Dy(); y++ {
			for x := 0; x < region.Dx(); x++ {
				if got.CMYKAt(x, y) != whole.Frames[0].CMYK.CMYKAt(region.Min.X+x, region.Min.Y+y) {
					t.Fatalf("%s: region pixel (%d,%d)", name, x, y)
				}
			}
		}
	}
}

func openTiles(t testing.TB, data []byte) codec.TileReader {
	t.Helper()
	tr, err := Decoder{}.OpenTiled(context.Background(), bytes.NewReader(data), int64(len(data)), codec.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { tr.Close() })
	return tr
}

func TestCMYKTileReaderOffersInkOnlyWithACMYKProfile(t *testing.T) {
	profile := testimg.SampleICCSpace("CMYK", 3000)
	for name, base := range cmykFiles(t) {
		data := withICC(t, base, profile, 900)
		tr := openTiles(t, data)
		ik, ok := tr.(codec.InkTileReader)
		if !ok || !ik.Ink() {
			t.Fatalf("%s: no ink tiles", name)
		}
		if got := tr.(codec.ColorTileReader).Color(); !bytes.Equal(got.ICC, profile) {
			t.Fatalf("%s: tile colour %d bytes", name, len(got.ICC))
		}
		want := standardInk(t, data)
		whole, err := ik.OverviewInk(context.Background(), 0)
		if err != nil || !sameInk(whole, want) {
			t.Fatalf("%s: overview %v", name, err)
		}
		naive, err := tr.Overview(context.Background(), 0)
		if err != nil || !samePixels(naive, inkAsRGB(want)) {
			t.Fatalf("%s: naive overview %v", name, err)
		}
		half, err := ik.OverviewInk(context.Background(), 1)
		if err != nil {
			t.Fatal(err)
		}
		naiveHalf, err := tr.Overview(context.Background(), 1)
		if err != nil || !samePixels(inkAsRGB(half), naiveHalf) {
			t.Fatalf("%s: level 1 differs: %v", name, err)
		}
		b := want.Bounds()
		r := image.Rect(5, 7, b.Dx()-9, b.Dy()-4)
		region, err := ik.RegionInk(context.Background(), 0, r)
		if err != nil {
			t.Fatal(err)
		}
		for y := 0; y < r.Dy(); y++ {
			for x := 0; x < r.Dx(); x++ {
				if region.CMYKAt(x, y) != want.CMYKAt(r.Min.X+x, r.Min.Y+y) {
					t.Fatalf("%s: region (%d,%d)", name, x, y)
				}
			}
		}
		deepLevel := jpegLevel(t, ik, b)
		if deepLevel.Dx() <= 0 {
			t.Fatalf("%s: level beyond the native ones is empty", name)
		}
		if empty, err := ik.RegionInk(context.Background(), 0, image.Rect(b.Dx()+5, 0, b.Dx()+9, 4)); err != nil || !empty.Bounds().Empty() {
			t.Fatalf("%s: region outside: %v %v", name, empty, err)
		}
		if _, err := ik.OverviewInk(context.Background(), -1); err == nil {
			t.Fatalf("%s: negative level accepted", name)
		}
		if _, err := ik.RegionInk(context.Background(), -1, r); err == nil {
			t.Fatalf("%s: negative level accepted for a region", name)
		}
	}
}

func jpegLevel(t testing.TB, ik codec.InkTileReader, b image.Rectangle) image.Rectangle {
	t.Helper()
	const shift = 5
	ov, err := ik.OverviewInk(context.Background(), shift)
	if err != nil {
		t.Fatal(err)
	}
	w, h := codec.LevelSize(b.Dx(), b.Dy(), shift)
	if ov.Bounds() != image.Rect(0, 0, w, h) {
		t.Fatalf("level %d is %v, want %dx%d", shift, ov.Bounds(), w, h)
	}
	part, err := ik.RegionInk(context.Background(), shift, image.Rect(0, 0, w, h))
	if err != nil {
		t.Fatal(err)
	}
	if part.Bounds() != ov.Bounds() {
		t.Fatalf("region of level %d is %v", shift, part.Bounds())
	}
	return ov.Bounds()
}

func TestTileReadersWithoutAProfileOrInkOfferNoInk(t *testing.T) {
	for name, base := range cmykFiles(t) {
		tr := openTiles(t, base)
		ik, ok := tr.(codec.InkTileReader)
		if !ok || ik.Ink() {
			t.Fatalf("%s: ink offered without a profile", name)
		}
		if _, err := ik.OverviewInk(context.Background(), 0); err == nil {
			t.Fatalf("%s: OverviewInk worked without a profile", name)
		}
		if _, err := ik.RegionInk(context.Background(), 0, image.Rect(0, 0, 4, 4)); err == nil {
			t.Fatalf("%s: RegionInk worked without a profile", name)
		}
		rgbProfile := withICC(t, base, testimg.SampleICCSpace("RGB ", 500), 1<<15)
		if openTiles(t, rgbProfile).(codec.InkTileReader).Ink() {
			t.Fatalf("%s: RGB profile on CMYK data gave ink", name)
		}
	}
	files, _ := corpus(t)
	rgb := withICC(t, files["video-001.jpeg"], testimg.SampleICCSpace("CMYK", 500), 1<<15)
	if openTiles(t, rgb).(codec.InkTileReader).Ink() {
		t.Fatal("RGB picture offered ink")
	}
}
