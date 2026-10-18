package jxl

import (
	"image"
	"image/color"
	"testing"

	gjxl "github.com/gen2brain/jxl"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/testimg"
)

func deepOptions() codec.Options {
	return codec.Options{Limits: codec.DefaultLimits(), Deep: true}
}

func wideImage(w, h int, alpha func(x, y int) uint16) *image.NRGBA64 {
	img := image.NewNRGBA64(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA64(x, y, color.NRGBA64{R: uint16(x*2600 + y), G: uint16(y*4000 + x), B: uint16(x*y*100 + 77), A: alpha(x, y)})
		}
	}
	return img
}

func TestDeepReturnsSixteenBitSamplesExactly(t *testing.T) {
	src := wideImage(24, 16, func(x, y int) uint16 { return 0xffff })
	data := encode(t, src, gjxl.EncodeOptions{Lossless: true})
	img, err := decode(data, deepOptions())
	if err != nil {
		t.Fatal(err)
	}
	frame := img.Frames[0]
	if frame.Deep == nil || frame.Pix != nil {
		t.Fatalf("deep %v pix %v", frame.Deep != nil, frame.Pix != nil)
	}
	if img.Info.Width != 24 || img.Info.Height != 16 {
		t.Fatalf("%+v", img.Info)
	}
	for y := 0; y < 16; y++ {
		for x := 0; x < 24; x++ {
			if got, want := frame.Deep.NRGBA64At(x, y), src.NRGBA64At(x, y); got != want {
				t.Fatalf("(%d,%d) = %v, want %v", x, y, got, want)
			}
		}
	}
}

func TestDeepKeepsSixteenBitAlpha(t *testing.T) {
	src := wideImage(20, 12, func(x, y int) uint16 { return uint16(0x4000 + x*0x800 + y) })
	data := encode(t, src, gjxl.EncodeOptions{Lossless: true})
	img, err := decode(data, deepOptions())
	if err != nil {
		t.Fatal(err)
	}
	frame := img.Frames[0]
	if frame.Deep == nil || !img.Info.HasAlpha {
		t.Fatal("no deep picture with alpha")
	}
	for y := 0; y < 12; y++ {
		for x := 0; x < 20; x++ {
			if got, want := frame.Deep.NRGBA64At(x, y), src.NRGBA64At(x, y); got != want {
				t.Fatalf("(%d,%d) = %v, want %v", x, y, got, want)
			}
		}
	}
}

func TestDeepExpandsSixteenBitGrey(t *testing.T) {
	src := image.NewGray16(image.Rect(0, 0, 19, 11))
	for i := range src.Pix {
		src.Pix[i] = byte(i*29 + 5)
	}
	data := encode(t, src, gjxl.EncodeOptions{Lossless: true})
	img, err := decode(data, deepOptions())
	if err != nil {
		t.Fatal(err)
	}
	frame := img.Frames[0]
	if frame.Deep == nil {
		t.Fatal("grey 16 bit is not deep")
	}
	for y := 0; y < 11; y++ {
		for x := 0; x < 19; x++ {
			v := src.Gray16At(x, y).Y
			if got := frame.Deep.NRGBA64At(x, y); got.R != v || got.G != v || got.B != v || got.A != 0xffff {
				t.Fatalf("(%d,%d) = %v, want %d", x, y, got, v)
			}
		}
	}
}

func TestWithoutDeepTheResultIsEightBitAsBefore(t *testing.T) {
	src := wideImage(24, 16, func(x, y int) uint16 { return 0xffff })
	data := encode(t, src, gjxl.EncodeOptions{Lossless: true})
	img, err := decodeAll(data)
	if err != nil {
		t.Fatal(err)
	}
	if img.Frames[0].Deep != nil || img.Frames[0].Pix == nil {
		t.Fatal("Deep leaked into the default path")
	}
}

func TestEightBitPicturesStayEightBitWithDeep(t *testing.T) {
	data := encode(t, testimg.Photo(40, 30), gjxl.EncodeOptions{Lossless: true})
	img, err := decode(data, deepOptions())
	if err != nil {
		t.Fatal(err)
	}
	if img.Frames[0].Deep != nil || img.Frames[0].Pix == nil {
		t.Fatal("8 bit picture became deep")
	}
	if img.Color.CICP == nil || *img.Color.CICP != (codec.CICP{Primaries: 1, Transfer: 13, FullRange: true}) || img.Color.ICC != nil {
		t.Fatalf("%+v", img.Color)
	}
}

func TestStreamsWithoutAColourEncodingAreSRGB(t *testing.T) {
	data := buildStill(t, 16, 16, 1)
	img, err := decode(data, deepOptions())
	if err != nil {
		t.Fatal(err)
	}
	if img.Color.CICP == nil || img.Color.CICP.Primaries != 1 || img.Color.CICP.Transfer != 13 {
		t.Fatalf("%+v", img.Color.CICP)
	}
}

func colourSpec(change func(*headerSpec)) headerSpec {
	s := baseHeader(100, 80)
	s.colorAllDefault = false
	change(&s)
	return s
}

func TestColourEncodingBecomesCICP(t *testing.T) {
	cases := []struct {
		name string
		spec headerSpec
		want *codec.CICP
	}{
		{"all default", baseHeader(100, 80), &codec.CICP{Primaries: 1, Transfer: 13, FullRange: true}},
		{"sRGB", colourSpec(func(s *headerSpec) {}), &codec.CICP{Primaries: 1, Transfer: 13, FullRange: true}},
		{"BT.2100 PQ", colourSpec(func(s *headerSpec) { s.primariesEnum, s.transferEnum = 9, 16 }), &codec.CICP{Primaries: 9, Transfer: 16, FullRange: true}},
		{"BT.2100 HLG", colourSpec(func(s *headerSpec) { s.primariesEnum, s.transferEnum = 9, 18 }), &codec.CICP{Primaries: 9, Transfer: 18, FullRange: true}},
		{"Display P3", colourSpec(func(s *headerSpec) { s.primariesEnum = 11 }), &codec.CICP{Primaries: 12, Transfer: 13, FullRange: true}},
		{"DCI P3", colourSpec(func(s *headerSpec) { s.primariesEnum, s.whiteEnum = 11, 11 }), &codec.CICP{Primaries: 11, Transfer: 13, FullRange: true}},
		{"P3 linear", colourSpec(func(s *headerSpec) { s.primariesEnum, s.transferEnum = 11, 8 }), &codec.CICP{Primaries: 12, Transfer: 8, FullRange: true}},
		{"BT.709 transfer", colourSpec(func(s *headerSpec) { s.transferEnum = 1 }), &codec.CICP{Primaries: 1, Transfer: 1, FullRange: true}},
		{"gamma 2.2", colourSpec(func(s *headerSpec) { s.gamma, s.gammaBits = true, 4545455 }), &codec.CICP{Primaries: 1, Transfer: 4, FullRange: true}},
		{"gamma 2.8", colourSpec(func(s *headerSpec) { s.gamma, s.gammaBits = true, 3571429 }), &codec.CICP{Primaries: 1, Transfer: 5, FullRange: true}},
		{"gamma 1", colourSpec(func(s *headerSpec) { s.gamma, s.gammaBits = true, 10000000 }), &codec.CICP{Primaries: 1, Transfer: 8, FullRange: true}},
		{"odd gamma", colourSpec(func(s *headerSpec) { s.gamma, s.gammaBits = true, 5500000 }), nil},
		{"DCI transfer", colourSpec(func(s *headerSpec) { s.transferEnum = 17 }), nil},
		{"unknown transfer", colourSpec(func(s *headerSpec) { s.transferEnum = 2 }), nil},
		{"custom primaries", colourSpec(func(s *headerSpec) { s.primariesCustom = true }), nil},
		{"custom white point", colourSpec(func(s *headerSpec) { s.whiteCustom = true }), nil},
		{"E white point", colourSpec(func(s *headerSpec) { s.whiteEnum = 10 }), nil},
		{"grey", colourSpec(func(s *headerSpec) { s.colorSpace = colorSpaceGray }), &codec.CICP{Primaries: 1, Transfer: 13, FullRange: true}},
		{"grey gamma", colourSpec(func(s *headerSpec) { s.colorSpace, s.gamma, s.gammaBits = colorSpaceGray, true, 4545455 }), &codec.CICP{Primaries: 1, Transfer: 4, FullRange: true}},
		{"embedded profile", colourSpec(func(s *headerSpec) { s.embeddedProfile = true }), nil},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			data, _ := tt.spec.write()
			header, err := parseImageHeader(data)
			if err != nil {
				t.Fatal(err)
			}
			got := header.cicp()
			switch {
			case tt.want == nil && got != nil:
				t.Fatalf("got %+v, want none", got)
			case tt.want != nil && (got == nil || *got != *tt.want):
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestEmbeddedProfileIsReportedAsICCOnly(t *testing.T) {
	spec := colourSpec(func(s *headerSpec) { s.embeddedProfile = true })
	data, _ := spec.write()
	header, err := parseImageHeader(data)
	if err != nil {
		t.Fatal(err)
	}
	profile := testimg.SampleICCSpace("RGB ", 50)
	got := header.color(profile)
	if string(got.ICC) != string(profile) || got.CICP != nil {
		t.Fatalf("%+v", got)
	}
	if got := header.color(testimg.SampleICCSpace("CMYK", 0)); !got.Empty() {
		t.Fatal("CMYK profile accepted")
	}
	if got := header.color(nil); !got.Empty() {
		t.Fatal("missing profile accepted")
	}
}

func TestColourFieldsDoNotChangeWhatFollowsInTheHeader(t *testing.T) {
	for _, mod := range []func(*headerSpec){
		func(s *headerSpec) { s.primariesEnum, s.transferEnum = 9, 16 },
		func(s *headerSpec) { s.gamma, s.gammaBits = true, 4545455 },
		func(s *headerSpec) { s.primariesCustom = true },
		func(s *headerSpec) { s.colorSpace = colorSpaceGray },
	} {
		spec := colourSpec(mod)
		spec.toneMapping = true
		spec.orientation = 3
		data, end := spec.write()
		header, err := parseImageHeader(data)
		if err != nil || header.endBit != end || header.orientation != 3 {
			t.Fatalf("%+v %v end %d/%d", header, err, header.endBit, end)
		}
	}
}

func frameBounds(f codec.Frame) (image.Rectangle, bool) {
	switch {
	case f.Pix != nil && f.Deep == nil:
		return f.Pix.Bounds(), true
	case f.Deep != nil && f.Pix == nil:
		return f.Deep.Bounds(), true
	}
	return image.Rectangle{}, false
}
