package jxl

import (
	"bytes"
	"image"
	"math"
	"testing"

	gjxl "github.com/gen2brain/jxl"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/testimg"
	"github.com/oops1/PixGo/internal/colorm"
)

func xybStill(t testing.TB, original []byte, mod func(*headerSpec)) []byte {
	t.Helper()
	header, err := parseImageHeader(original)
	if err != nil {
		t.Fatal(err)
	}
	spec := baseHeader(header.width, header.height)
	spec.xyb = true
	spec.colorAllDefault = false
	for range header.extraChannels {
		spec.extra = append(spec.extra, extraSpec{kind: extraChannelAlpha})
	}
	if mod != nil {
		mod(&spec)
	}
	bits, end := spec.write()
	out := &bitWriter{data: append([]byte(nil), bits...), position: end}
	out.align()
	return append(out.data, original[(header.endBit+7)/8:]...)
}

func lossyPhoto(t testing.TB, w, h int) []byte {
	t.Helper()
	return encode(t, testimg.Photo(w, h), gjxl.EncodeOptions{Quality: 95})
}

func tagged(primaries, transfer uint32, target float64) func(*headerSpec) {
	return func(s *headerSpec) {
		s.primariesEnum, s.transferEnum = primaries, transfer
		if target != 0 {
			s.toneMapping = true
			s.intensityBits = halfBits(target)
		}
	}
}

func floatsOf(t testing.TB, data []byte) *gjxl.Float {
	t.Helper()
	f, err := gjxl.DecodeFloat(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func nitsOfCode(code uint16) float64 {
	e := math.Pow(float64(code)/65535, 1/pqM2)
	return pqPeakNits * math.Pow(math.Max(e-pqC1, 0)/(pqC2-pqC3*e), 1/pqM1)
}

func peak(f *gjxl.Float) float32 {
	var m float32
	for _, v := range f.Frames[0] {
		m = max(m, v)
	}
	return m
}

func TestTheHeaderBuilderReproducesTheLibraryPicture(t *testing.T) {
	original := lossyPhoto(t, 64, 48)
	rebuilt := xybStill(t, original, nil)
	want, err := gjxl.Decode(bytes.NewReader(original))
	if err != nil {
		t.Fatal(err)
	}
	got, err := gjxl.Decode(bytes.NewReader(rebuilt))
	if err != nil {
		t.Fatal(err)
	}
	if !testimg.Identical(want.(*image.NRGBA), got.(*image.NRGBA)) {
		t.Fatal("rebuilt stream decodes differently")
	}
	header, err := parseImageHeader(rebuilt)
	if err != nil || !header.xybEncoded || header.intensityTarget != defaultIntensityTarget {
		t.Fatalf("%+v %v", header, err)
	}
}

func TestHeaderReadsTheIntensityTarget(t *testing.T) {
	original := lossyPhoto(t, 24, 16)
	for _, target := range []float64{100, 255, 1000, 4000, 10000, 65504} {
		header, err := parseImageHeader(xybStill(t, original, tagged(9, 16, target)))
		if err != nil || header.intensityTarget != target {
			t.Fatalf("target %v: %v %v", target, header.intensityTarget, err)
		}
	}
	header, err := parseImageHeader(xybStill(t, original, nil))
	if err != nil || header.intensityTarget != defaultIntensityTarget {
		t.Fatalf("default: %v %v", header.intensityTarget, err)
	}
	broken := xybStill(t, original, func(s *headerSpec) {
		tagged(9, 16, 1000)(s)
		s.intensityBits = 0x7c00
	})
	if _, err := parseImageHeader(broken); err == nil {
		t.Fatal("infinite intensity target accepted")
	}
}

func decodeHDRStill(t testing.TB, data []byte) *codec.Image {
	t.Helper()
	img, err := decode(data, deepOptions())
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func requireNear(t testing.TB, label string, got, want uint16, tolerance int) {
	t.Helper()
	if d := int(got) - int(want); d > tolerance || d < -tolerance {
		t.Fatalf("%s: code %d, want %d", label, got, want)
	}
}

func TestPQAndHLGStreamsComeBackAsPQCodesOfTheirAbsoluteLuminance(t *testing.T) {
	original := lossyPhoto(t, 64, 48)
	cases := []struct {
		name       string
		primaries  uint32
		transfer   uint32
		target     float64
		wantCICP   codec.CICP
		scale      float64
		isRelative bool
	}{
		{"PQ 1000", 9, 16, 1000, codec.CICP{Primaries: 9, Transfer: 16, FullRange: true}, 1000, false},
		{"PQ 10000", 9, 16, 10000, codec.CICP{Primaries: 9, Transfer: 16, FullRange: true}, 10000, false},
		{"HLG 1000", 9, 18, 1000, codec.CICP{Primaries: 9, Transfer: 16, FullRange: true}, 1000, false},
		{"PQ P3", 11, 16, 4000, codec.CICP{Primaries: 12, Transfer: 16, FullRange: true}, 4000, false},
		{"linear 4000", 1, 8, 4000, codec.CICP{Primaries: 1, Transfer: 16, FullRange: true}, 4000, true},
		{"linear default", 1, 8, 0, codec.CICP{Primaries: 1, Transfer: 16, FullRange: true}, 255 * referenceWhiteNits / sdrIntensityTarget, true},
		{"linear 100", 9, 8, 100, codec.CICP{Primaries: 9, Transfer: 16, FullRange: true}, 100 * referenceWhiteNits / sdrIntensityTarget, true},
	}
	for _, tt := range cases {
		data := xybStill(t, original, tagged(tt.primaries, tt.transfer, tt.target))
		img := decodeHDRStill(t, data)
		frame := img.Frames[0]
		if frame.Deep == nil || frame.Pix != nil {
			t.Fatalf("%s: not a deep picture", tt.name)
		}
		if img.Color.CICP == nil || *img.Color.CICP != tt.wantCICP || len(img.Color.ICC) != 0 {
			t.Fatalf("%s: colour %+v", tt.name, img.Color.CICP)
		}
		if img.Info.Width != 64 || img.Info.Height != 48 || img.Scale != 1 || len(img.Frames) != 1 {
			t.Fatalf("%s: %+v", tt.name, img.Info)
		}
		floats := floatsOf(t, data)
		for y := 0; y < 48; y += 3 {
			for x := 0; x < 64; x += 3 {
				px := floats.Frames[0][(y*64+x)*floats.Channels:]
				c := frame.Deep.NRGBA64At(x, y)
				for i, got := range []uint16{c.R, c.G, c.B} {
					l := math.Max(float64(px[i]), 0)
					want := uint16(math.Round(math.Min(pqEncode(math.Min(l*tt.scale/pqPeakNits, 1)), 1) * 65535))
					requireNear(t, tt.name, got, want, 1)
				}
				if c.A != 0xffff {
					t.Fatalf("%s: alpha %d", tt.name, c.A)
				}
			}
		}
	}
}

func TestLinearValuesAboveOneSurviveAsHighlights(t *testing.T) {
	original := lossyPhoto(t, 64, 48)
	data := xybStill(t, original, tagged(1, 8, 100))
	floats := floatsOf(t, data)
	if peak(floats) <= 2 {
		t.Fatalf("the test picture does not exceed 1.0: peak %v", peak(floats))
	}
	clamped, err := gjxl.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	img := decodeHDRStill(t, data)
	scale := 100 * referenceWhiteNits / sdrIntensityTarget
	var checked int
	for y := 0; y < 48; y++ {
		for x := 0; x < 64; x++ {
			l := float64(floats.Frames[0][(y*64+x)*floats.Channels])
			if l <= 1.2 {
				continue
			}
			checked++
			got := nitsOfCode(img.Frames[0].Deep.NRGBA64At(x, y).R)
			want := l * scale
			if math.Abs(got-want) > want*0.01 {
				t.Fatalf("(%d,%d): %.2f nits, want %.2f (value %.3f)", x, y, got, want, l)
			}
			if got <= scale*1.1 {
				t.Fatalf("(%d,%d): %.2f nits is not above the reference white", x, y, got)
			}
			if c := clamped.(*image.NRGBA).NRGBAAt(x, y); c.R == 255 && nitsOfCode(img.Frames[0].Deep.NRGBA64At(x, y).R) < scale {
				t.Fatalf("(%d,%d): clipped", x, y)
			}
		}
	}
	if checked < 50 {
		t.Fatalf("only %d samples above 1.0", checked)
	}
}

func TestPQStreamMatchesTheSRGBRenderingOfTheSameData(t *testing.T) {
	original := lossyPhoto(t, 64, 48)
	srgb, err := gjxl.Decode(bytes.NewReader(xybStill(t, original, func(s *headerSpec) {
		s.toneMapping = true
		s.intensityBits = halfBits(1000)
	})))
	if err != nil {
		t.Fatal(err)
	}
	pq := decodeHDRStill(t, xybStill(t, original, tagged(1, 16, 1000)))
	for y := 0; y < 48; y += 2 {
		for x := 0; x < 64; x += 2 {
			s := srgb.(*image.NRGBA).NRGBAAt(x, y)
			c := pq.Frames[0].Deep.NRGBA64At(x, y)
			for i, pair := range [][2]float64{{float64(s.R), nitsOfCode(c.R)}, {float64(s.G), nitsOfCode(c.G)}, {float64(s.B), nitsOfCode(c.B)}} {
				v := pair[0] / 255
				lin := v / 12.92
				if v > 0.04045 {
					lin = math.Pow((v+0.055)/1.055, 2.4)
				}
				want := lin * 1000
				if math.Abs(pair[1]-want) > want*0.02+1.5 {
					t.Fatalf("(%d,%d) channel %d: %.2f nits, sRGB rendering says %.2f", x, y, i, pair[1], want)
				}
			}
		}
	}
}

func TestStreamsThatAreNotHDRKeepTheirPath(t *testing.T) {
	original := lossyPhoto(t, 48, 32)
	for name, data := range map[string][]byte{
		"sRGB xyb":        xybStill(t, original, nil),
		"sRGB xyb target": xybStill(t, original, func(s *headerSpec) { s.toneMapping = true; s.intensityBits = halfBits(1000) }),
		"gamma xyb":       xybStill(t, original, func(s *headerSpec) { s.gamma, s.gammaBits = true, 4545455 }),
		"P3 sRGB xyb":     xybStill(t, original, func(s *headerSpec) { s.primariesEnum = 11 }),
	} {
		img := decodeHDRStill(t, data)
		if img.Frames[0].Deep != nil || img.Frames[0].Pix == nil {
			t.Fatalf("%s: left the 8 bit path", name)
		}
		plain, err := decodeAll(data)
		if err != nil || !testimg.Identical(plain.First(), img.First()) {
			t.Fatalf("%s: Deep changed the picture", name)
		}
	}
	lossless := encode(t, wideImage(24, 16, func(x, y int) uint16 { return 0xffff }), gjxl.EncodeOptions{Lossless: true})
	img := decodeHDRStill(t, lossless)
	if img.Frames[0].Deep == nil {
		t.Fatal("16 bit modular lost its deep path")
	}
}

func TestHDRStreamsWithoutDeepAreDecodedAsBefore(t *testing.T) {
	original := lossyPhoto(t, 48, 32)
	data := xybStill(t, original, tagged(9, 16, 1000))
	img, err := decode(data, codec.Options{Limits: codec.DefaultLimits()})
	if err != nil {
		t.Fatal(err)
	}
	want, err := gjxl.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if img.Frames[0].Deep != nil || !testimg.Identical(img.First(), want.(*image.NRGBA)) {
		t.Fatal("colour management off must keep the old result")
	}
	if img.Color.CICP == nil || img.Color.CICP.Transfer != 16 {
		t.Fatalf("%+v", img.Color.CICP)
	}
}

func TestHDRStreamKeepsItsAlpha(t *testing.T) {
	src := testimg.Noise(40, 24, 7, true)
	for i := 3; i < len(src.Pix); i += 4 {
		src.Pix[i] = uint8(30 + i%200)
	}
	original := encode(t, src, gjxl.EncodeOptions{Quality: 95})
	header, err := parseImageHeader(original)
	if err != nil || !header.hasAlpha() || !header.xybEncoded {
		t.Fatalf("test input: %+v %v", header, err)
	}
	data := xybStill(t, original, tagged(9, 16, 1000))
	img := decodeHDRStill(t, data)
	frame := img.Frames[0].Deep
	if frame == nil || !img.Info.HasAlpha {
		t.Fatal("no deep picture with alpha")
	}
	plain, err := gjxl.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	for y := 0; y < 24; y++ {
		for x := 0; x < 40; x++ {
			want := uint16(plain.(*image.NRGBA).NRGBAAt(x, y).A) * 257
			requireNear(t, "alpha", frame.NRGBA64At(x, y).A, want, 0)
		}
	}
}

func TestHDRPlanSelection(t *testing.T) {
	base := func(mod func(*imageHeader)) imageHeader {
		h := imageHeader{width: 8, height: 8, orientation: 1, bitsPerSample: 8, xybEncoded: true, intensityTarget: 1000, encoding: colorEncoding{whitePoint: 1, primaries: 9, transfer: 16}}
		if mod != nil {
			mod(&h)
		}
		return h
	}
	cases := []struct {
		name string
		h    imageHeader
		want bool
	}{
		{"xyb PQ", base(nil), true},
		{"xyb HLG", base(func(h *imageHeader) { h.encoding.transfer = 18 }), true},
		{"xyb linear", base(func(h *imageHeader) { h.encoding.transfer = 8 }), true},
		{"xyb sRGB", base(func(h *imageHeader) { h.encoding.transfer = 13 }), false},
		{"xyb gamma 2.2", base(func(h *imageHeader) { h.encoding.haveGamma, h.encoding.gamma = true, 4545455 }), false},
		{"modular integer PQ", base(func(h *imageHeader) { h.xybEncoded = false }), false},
		{"modular float PQ", base(func(h *imageHeader) { h.xybEncoded, h.floatSample, h.bitsPerSample = false, true, 32 }), false},
		{"modular float linear", base(func(h *imageHeader) {
			h.xybEncoded, h.floatSample, h.bitsPerSample, h.encoding.transfer = false, true, 32, 8
		}), true},
		{"modular float sRGB", base(func(h *imageHeader) {
			h.xybEncoded, h.floatSample, h.bitsPerSample, h.encoding.transfer = false, true, 32, 13
		}), true},
		{"animation", base(func(h *imageHeader) { h.animated = true }), false},
		{"embedded profile", base(func(h *imageHeader) { h.embeddedProfile = true }), false},
		{"spot colour", base(func(h *imageHeader) { h.extraChannels = []extraChannel{{kind: extraChannelSpot}} }), false},
		{"custom primaries", base(func(h *imageHeader) { h.encoding.primaries = 2 }), false},
	}
	for _, tt := range cases {
		if _, got := tt.h.hdrPlan(); got != tt.want {
			t.Errorf("%s: %v", tt.name, got)
		}
	}
	alpha := base(func(h *imageHeader) {
		h.extraChannels = []extraChannel{{kind: 4}, {kind: extraChannelAlpha}}
	})
	if plan, ok := alpha.hdrPlan(); !ok || plan.alpha != 4 || plan.color != 3 {
		t.Fatalf("%+v %v", plan, ok)
	}
	grey := base(func(h *imageHeader) { h.colorSpace = colorSpaceGray })
	if plan, ok := grey.hdrPlan(); !ok || plan.color != 1 {
		t.Fatalf("%+v %v", plan, ok)
	}
}

func TestPQLookupMatchesTheFormula(t *testing.T) {
	table := pqLookup()
	worst := 0
	for nits := 1e-6; nits < pqPeakNits*1.2; nits *= 1.0007 {
		want := math.Round(math.Min(pqEncode(math.Min(nits/pqPeakNits, 1)), 1) * 65535)
		got := pqCode(table, nits)
		if d := int(math.Abs(float64(got) - want)); d > worst {
			worst = d
		}
	}
	if worst > 1 {
		t.Fatalf("lookup is %d codes away from the formula", worst)
	}
	for _, nits := range []float64{0, -3, math.NaN(), 1e-30, 1e-12} {
		if pqCode(table, nits) != 0 {
			t.Fatalf("%v nits is not black", nits)
		}
	}
	for _, nits := range []float64{pqPeakNits, 2e4, math.Inf(1)} {
		if pqCode(table, nits) != 0xffff {
			t.Fatalf("%v nits is not the peak", nits)
		}
	}
	if code := pqCode(table, referenceWhiteNits); code < 38000 || code > 38100 {
		t.Fatalf("203 nits is code %d", code)
	}
}

func floatPicture(w, h, channels int, fill func(x, y, c int) float32) *gjxl.Float {
	f := &gjxl.Float{Width: w, Height: h, Channels: channels, Frames: [][]float32{make([]float32, w*h*channels)}}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			for c := 0; c < channels; c++ {
				f.Frames[0][(y*w+x)*channels+c] = fill(x, y, c)
			}
		}
	}
	return f
}

func TestFloatSamplesAreMappedByTheirTransfer(t *testing.T) {
	values := []float32{0, 0.002, 0.04, 0.18, 0.5, 1, 1.5, 4, 16, 100}
	pic := floatPicture(len(values), 1, 3, func(x, y, c int) float32 { return values[x] * float32(c+1) / 3 })
	srgb := func(v float64) float64 {
		if v <= 0.04045 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	bt709 := func(v float64) float64 {
		if v < 0.081 {
			return v / 4.5
		}
		return math.Pow((v+0.099)/1.099, 1/0.45)
	}
	for _, tt := range []struct {
		name     string
		transfer uint8
		linear   func(float64) float64
	}{
		{"linear", transferLinear, func(v float64) float64 { return v }},
		{"sRGB", transferSRGB, srgb},
		{"BT.709", transferBT709Code, bt709},
		{"gamma 2.2", transferGamma22, func(v float64) float64 { return math.Pow(v, 2.2) }},
		{"gamma 2.8", transferGamma28, func(v float64) float64 { return math.Pow(v, 2.8) }},
	} {
		plan := hdrPlan{transfer: tt.transfer, scale: referenceWhiteNits, color: 3, alpha: -1}
		out := plan.frame(pic, pic.Frames[0])
		for x, v := range values {
			c := out.NRGBA64At(x, 0)
			for i, got := range []uint16{c.R, c.G, c.B} {
				in := float64(v) * float64(i+1) / 3
				want := uint16(math.Round(math.Min(pqEncode(math.Min(tt.linear(float64(float32(in)))*referenceWhiteNits/pqPeakNits, 1)), 1) * 65535))
				requireNear(t, tt.name, got, want, 1)
			}
			if c.A != 0xffff {
				t.Fatalf("%s: alpha", tt.name)
			}
		}
	}
}

func TestFloatReferenceWhiteSitsAtTheSDRWhiteCode(t *testing.T) {
	pic := floatPicture(2, 1, 3, func(x, y, c int) float32 { return float32(1 + x*3) })
	plan := hdrPlan{transfer: transferLinear, scale: referenceWhiteNits, color: 3, alpha: -1}
	out := plan.frame(pic, pic.Frames[0])
	white, bright := out.NRGBA64At(0, 0).R, out.NRGBA64At(1, 0).R
	if math.Abs(nitsOfCode(white)-referenceWhiteNits) > 1 {
		t.Fatalf("1.0 is %.2f nits", nitsOfCode(white))
	}
	if math.Abs(nitsOfCode(bright)-4*referenceWhiteNits) > 8 {
		t.Fatalf("4.0 is %.2f nits", nitsOfCode(bright))
	}
}

func TestFloatPicturesWithAlphaGreyAndHostileValues(t *testing.T) {
	nan, inf := float32(math.NaN()), float32(math.Inf(1))
	grey := floatPicture(5, 1, 2, func(x, y, c int) float32 {
		if c == 1 {
			return float32(x) / 4
		}
		return []float32{0.5, nan, -2, inf, 3}[x]
	})
	plan := hdrPlan{transfer: transferLinear, scale: referenceWhiteNits, color: 1, alpha: 1}
	out := plan.frame(grey, grey.Frames[0])
	for x := 0; x < 5; x++ {
		c := out.NRGBA64At(x, 0)
		if c.R != c.G || c.G != c.B {
			t.Fatalf("grey pixel %d is not grey: %v", x, c)
		}
		if want := uint16(math.Round(float64(x) / 4 * 65535)); c.A != want {
			t.Fatalf("alpha %d = %d, want %d", x, c.A, want)
		}
	}
	if out.NRGBA64At(1, 0).R != 0 || out.NRGBA64At(2, 0).R != 0 || out.NRGBA64At(3, 0).R != 0xffff {
		t.Fatal("NaN and negatives must be black and infinity white")
	}
	rgba := floatPicture(2, 1, 4, func(x, y, c int) float32 { return []float32{1, 1, 1, 7}[c] })
	plan = hdrPlan{transfer: transferLinear, scale: referenceWhiteNits, color: 3, alpha: 3}
	if got := plan.frame(rgba, rgba.Frames[0]).NRGBA64At(0, 0).A; got != 0xffff {
		t.Fatalf("alpha above one: %d", got)
	}
}

func TestHDRDecodeIsDeterministicAndShareable(t *testing.T) {
	data := xybStill(t, lossyPhoto(t, 96, 64), tagged(9, 16, 1000))
	first := decodeHDRStill(t, data)
	done := make(chan *codec.Image, 4)
	for i := 0; i < 4; i++ {
		go func() {
			img, err := decode(data, deepOptions())
			if err != nil {
				done <- nil
				return
			}
			done <- img
		}()
	}
	for i := 0; i < 4; i++ {
		img := <-done
		if img == nil || !bytes.Equal(img.Frames[0].Deep.Pix, first.Frames[0].Deep.Pix) {
			t.Fatal("parallel decodes disagree")
		}
	}
}

func TestTruncatedHDRStreamsFailCleanly(t *testing.T) {
	data := xybStill(t, lossyPhoto(t, 32, 32), tagged(9, 16, 1000))
	for _, n := range []int{len(data) / 2, len(data) - 3, 20} {
		if _, err := decode(data[:n], deepOptions()); err == nil {
			t.Fatalf("stream cut at %d accepted", n)
		}
	}
}

func BenchmarkHDRFrame(b *testing.B) {
	pic := floatPicture(1000, 1000, 3, func(x, y, c int) float32 { return float32(x+y+c) / 400 })
	plan := hdrPlan{transfer: transferLinear, scale: referenceWhiteNits, color: 3, alpha: -1}
	b.SetBytes(1000 * 1000 * 3 * 4)
	for i := 0; i < b.N; i++ {
		plan.frame(pic, pic.Frames[0])
	}
}

func TestDecodedHDRStreamsReachTheDisplayThroughTheColourManager(t *testing.T) {
	original := lossyPhoto(t, 64, 48)
	plain, err := gjxl.Decode(bytes.NewReader(xybStill(t, original, nil)))
	if err != nil {
		t.Fatal(err)
	}
	reference := plain.(*image.NRGBA)
	for name, tt := range map[string]struct {
		mod     func(*headerSpec)
		likeSDR bool
	}{
		"PQ Rec.2020":     {mod: tagged(9, 16, 1000)},
		"HLG P3":          {mod: tagged(11, 18, 1000)},
		"linear 4000":     {mod: tagged(1, 8, 4000)},
		"linear sRGB SDR": {mod: tagged(1, 8, 0), likeSDR: true},
	} {
		img := decodeHDRStill(t, xybStill(t, original, tt.mod))
		m := colorm.NewManager(nil)
		m.Configure(colorm.Config{Enabled: true})
		rep, err := m.ConvertImage(img)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		pix := img.Frames[0].Pix
		if pix == nil || img.Frames[0].Deep != nil || rep.HDR != "PQ" || !rep.ToneMapped {
			t.Fatalf("%s: %+v", name, rep)
		}
		var total, count int
		for i := 0; i < len(pix.Pix); i += 4 {
			for c := 0; c < 3; c++ {
				d := int(pix.Pix[i+c]) - int(reference.Pix[i+c])
				total += max(d, -d)
				count++
			}
			if pix.Pix[i] == 0 && pix.Pix[i+1] == 0 && pix.Pix[i+2] == 0 || pix.Pix[i] == 255 && pix.Pix[i+1] == 255 && pix.Pix[i+2] == 255 {
				t.Fatalf("%s: clipped pixel at %d", name, i/4)
			}
		}
		if mean := float64(total) / float64(count); tt.likeSDR && mean > 3 {
			t.Fatalf("%s: SDR white linear data drifts %.2f levels from the sRGB rendering", name, mean)
		}
	}
}
