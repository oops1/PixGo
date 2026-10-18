package heic

import (
	"bytes"
	"image"
	"testing"

	gheic "github.com/gen2brain/h265/heic"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/testimg"
	"github.com/oops1/PixGo/internal/isobmff/isotest"
)

func deepOptions() codec.Options {
	return codec.Options{Limits: codec.DefaultLimits(), Deep: true}
}

func withColr(t testing.TB, raw []byte, props ...isotest.Prop) []byte {
	t.Helper()
	parsed, err := isotest.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	for i := range parsed.Items {
		var kept []isotest.Prop
		for _, p := range parsed.Items[i].Props {
			if len(p.Box) >= 8 && string(p.Box[4:8]) == "colr" {
				continue
			}
			kept = append(kept, p)
		}
		if i == 0 {
			kept = append(kept, props...)
		}
		parsed.Items[i].Props = kept
	}
	return parsed.Bytes()
}

func nclx(primaries, transfer, matrix uint16, full bool) isotest.Prop {
	flag := byte(0)
	if full {
		flag = 0x80
	}
	return isotest.Prop{Box: isotest.Box("colr", []byte("nclx"), isotest.U16(primaries), isotest.U16(transfer), isotest.U16(matrix), []byte{flag})}
}

func TestDeepReturnsTenAndTwelveBitPictures(t *testing.T) {
	src := smoothImage(48, 32, false)
	for _, depth := range []int{10, 12} {
		data := encode(t, src, gheic.EncodeOptions{Quality: 90, BitDepth: depth})
		deep, err := decode(data, deepOptions())
		if err != nil {
			t.Fatalf("%d bit: %v", depth, err)
		}
		plain, err := decode(data, codec.Options{Limits: codec.DefaultLimits()})
		if err != nil {
			t.Fatal(err)
		}
		frame := deep.Frames[0]
		if frame.Deep == nil || frame.Pix != nil {
			t.Fatalf("%d bit: deep %v pix %v", depth, frame.Deep != nil, frame.Pix != nil)
		}
		if plain.Frames[0].Deep != nil || plain.Frames[0].Pix == nil {
			t.Fatalf("%d bit: Deep leaked into the default path", depth)
		}
		pix := plain.Frames[0].Pix
		lowBits := map[uint16]bool{}
		for y := 0; y < pix.Rect.Dy(); y++ {
			for x := 0; x < pix.Rect.Dx(); x++ {
				d, p := frame.Deep.NRGBA64At(x, y), pix.NRGBAAt(x, y)
				if uint8(d.R>>8) != p.R || uint8(d.G>>8) != p.G || uint8(d.B>>8) != p.B {
					t.Fatalf("%d bit (%d,%d): deep %v, 8 bit %v", depth, x, y, d, p)
				}
				lowBits[d.G&0xff] = true
			}
		}
		if len(lowBits) < 8 {
			t.Fatalf("%d bit: only %d distinct low bytes", depth, len(lowBits))
		}
	}
}

func TestEightBitPicturesStayEightBitWithDeep(t *testing.T) {
	for _, name := range []string{"basic.heic", "alpha.heic", "chroma422.heic", "lossless.heic"} {
		img, err := decode(readTestdata(t, name), deepOptions())
		if err != nil {
			t.Fatal(err)
		}
		if img.Frames[0].Deep != nil || img.Frames[0].Pix == nil {
			t.Fatalf("%s became deep", name)
		}
	}
}

func TestEmbeddedProfileIsReported(t *testing.T) {
	profile := testimg.SampleICCSpace("RGB ", 321)
	data := encode(t, smoothImage(32, 32, false), gheic.EncodeOptions{Quality: 80, ICC: profile})
	for _, o := range []codec.Options{deepOptions(), {Limits: codec.DefaultLimits()}} {
		img, err := decode(data, o)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(img.Color.ICC, profile) {
			t.Fatalf("%d bytes, want %d", len(img.Color.ICC), len(profile))
		}
		if img.Color.CICP == nil || img.Color.CICP.Primaries != 1 || img.Color.CICP.Transfer != 13 {
			t.Fatalf("the nclx description next to the profile: %+v", img.Color.CICP)
		}
	}
	plain := encode(t, smoothImage(32, 32, false), gheic.EncodeOptions{Quality: 80})
	img, err := decode(plain, deepOptions())
	if err != nil {
		t.Fatal(err)
	}
	if img.Color.ICC != nil {
		t.Fatal("profile appeared from nowhere")
	}
}

func TestOtherProfileKindsAndSpacesAreHandled(t *testing.T) {
	raw := readTestdata(t, "basic.heic")
	profile := testimg.SampleICCSpace("RGB ", 77)
	for _, kind := range []string{"prof", "rICC"} {
		data := withColr(t, raw, isotest.ColrProfile(kind, profile))
		img, err := decode(data, deepOptions())
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		if !bytes.Equal(img.Color.ICC, profile) {
			t.Fatalf("%s: %d bytes", kind, len(img.Color.ICC))
		}
	}
	data := withColr(t, raw, nclx(12, 13, 6, true), isotest.ColrProfile("prof", profile))
	img, err := decode(data, deepOptions())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(img.Color.ICC, profile) || img.Color.CICP == nil || img.Color.CICP.Primaries != 12 || !img.Color.CICP.FullRange {
		t.Fatalf("nclx before prof: %d bytes, %+v", len(img.Color.ICC), img.Color.CICP)
	}
	data = withColr(t, raw, isotest.ColrProfile("prof", testimg.SampleICCSpace("CMYK", 8)))
	img, err = decode(data, deepOptions())
	if err != nil {
		t.Fatal(err)
	}
	if img.Color.ICC != nil {
		t.Fatal("CMYK profile reported for RGB output")
	}
}

func TestCraftedNCLXBoxesAreReported(t *testing.T) {
	raw := readTestdata(t, "basic.heic")
	cases := []struct {
		name                       string
		primaries, transfer, matrx uint16
		full                       bool
	}{
		{"Display P3", 12, 13, 6, false},
		{"BT.2020 PQ", 9, 16, 9, false},
		{"BT.2020 HLG", 9, 18, 9, true},
		{"BT.601", 6, 6, 6, false},
	}
	for _, c := range cases {
		data := withColr(t, raw, nclx(c.primaries, c.transfer, c.matrx, c.full))
		img, err := decode(data, deepOptions())
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		want := codec.CICP{Primaries: uint8(c.primaries), Transfer: uint8(c.transfer), Matrix: uint8(c.matrx), FullRange: c.full}
		if img.Color.CICP == nil || *img.Color.CICP != want {
			t.Fatalf("%s: %+v, want %+v", c.name, img.Color.CICP, want)
		}
	}
}

func TestEightBitHDRPictureIsWidenedOnlyForDeep(t *testing.T) {
	raw := readTestdata(t, "basic.heic")
	data := withColr(t, raw, nclx(9, 16, 9, false))
	deep, err := decode(data, deepOptions())
	if err != nil {
		t.Fatal(err)
	}
	plain, err := decode(data, codec.Options{Limits: codec.DefaultLimits()})
	if err != nil {
		t.Fatal(err)
	}
	if deep.Frames[0].Deep == nil || deep.Frames[0].Pix != nil || !deep.Color.HDR() {
		t.Fatal("PQ picture is not deep")
	}
	if plain.Frames[0].Pix == nil || plain.Frames[0].Deep != nil {
		t.Fatal("default path changed")
	}
	for y := 0; y < plain.Info.Height; y += 7 {
		for x := 0; x < plain.Info.Width; x += 7 {
			d, p := deep.Frames[0].Deep.NRGBA64At(x, y), plain.Frames[0].Pix.NRGBAAt(x, y)
			if d.R != uint16(p.R)*257 || d.G != uint16(p.G)*257 || d.B != uint16(p.B)*257 || d.A != uint16(p.A)*257 {
				t.Fatalf("(%d,%d): %v vs %v", x, y, d, p)
			}
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
