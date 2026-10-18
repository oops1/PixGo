package avif

import (
	"bytes"
	"image"
	"testing"

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

func TestDeepReturnsHighBitDepthPicturesWithoutConversion(t *testing.T) {
	for _, name := range []string{"yuv420-10.avif", "yuv444-10.avif", "yuv422-12.avif", "mono-12.avif", "bt2020-422.avif", "clap-10.avif"} {
		t.Run(name, func(t *testing.T) {
			data := readTestdata(t, name)
			deep, err := decode(data, deepOptions())
			if err != nil {
				t.Fatal(err)
			}
			plain, err := decode(data, codec.Options{Limits: codec.DefaultLimits()})
			if err != nil {
				t.Fatal(err)
			}
			frame := deep.Frames[0]
			if frame.Deep == nil || frame.Pix != nil {
				t.Fatalf("deep %v pix %v", frame.Deep != nil, frame.Pix != nil)
			}
			if plain.Frames[0].Deep != nil || plain.Frames[0].Pix == nil {
				t.Fatal("Deep leaked into the default path")
			}
			if deep.Info != plain.Info {
				t.Fatalf("info %+v vs %+v", deep.Info, plain.Info)
			}
			pix := plain.Frames[0].Pix
			if frame.Deep.Rect != pix.Rect {
				t.Fatalf("bounds %v vs %v", frame.Deep.Rect, pix.Rect)
			}
			distinct := map[uint16]bool{}
			for y := 0; y < pix.Rect.Dy(); y++ {
				for x := 0; x < pix.Rect.Dx(); x++ {
					d, p := frame.Deep.NRGBA64At(x, y), pix.NRGBAAt(x, y)
					if uint8(d.R>>8) != p.R || uint8(d.G>>8) != p.G || uint8(d.B>>8) != p.B || uint8(d.A>>8) != p.A {
						t.Fatalf("(%d,%d): deep %v, 8 bit %v", x, y, d, p)
					}
					distinct[d.R&0xff] = true
				}
			}
			if len(distinct) < 8 {
				t.Fatalf("only %d distinct low bytes: the extra bits are missing", len(distinct))
			}
		})
	}
}

func TestEightBitPicturesStayEightBitWithDeep(t *testing.T) {
	for _, name := range []string{"yuv420-8.avif", "yuv444-8.avif", "alpha-444.avif", "mono-8.avif", "grid-2x2.avif", "unspec-420.avif"} {
		img, err := decode(readTestdata(t, name), deepOptions())
		if err != nil {
			t.Fatal(err)
		}
		if img.Frames[0].Deep != nil || img.Frames[0].Pix == nil {
			t.Fatalf("%s became deep", name)
		}
	}
}

func TestColorFollowsTheColourDescription(t *testing.T) {
	cases := []struct {
		file string
		want *codec.CICP
		hdr  bool
	}{
		{"yuv420-8.avif", &codec.CICP{Primaries: 1, Transfer: 13, Matrix: 6}, false},
		{"bt709-full.avif", &codec.CICP{Primaries: 1, Transfer: 13, Matrix: 1, FullRange: true}, false},
		{"bt2020-422.avif", &codec.CICP{Primaries: 9, Transfer: 16, Matrix: 9}, true},
		{"unspec-420.avif", nil, false},
	}
	for _, c := range cases {
		img, err := decode(readTestdata(t, c.file), deepOptions())
		if err != nil {
			t.Fatal(err)
		}
		switch {
		case c.want == nil && img.Color.CICP != nil:
			t.Fatalf("%s: %+v", c.file, img.Color.CICP)
		case c.want != nil && (img.Color.CICP == nil || *img.Color.CICP != *c.want):
			t.Fatalf("%s: %+v, want %+v", c.file, img.Color.CICP, c.want)
		}
		if img.Color.HDR() != c.hdr || img.Color.ICC != nil {
			t.Fatalf("%s: HDR %v icc %d", c.file, img.Color.HDR(), len(img.Color.ICC))
		}
	}
}

func TestCraftedNCLXBoxesAreReported(t *testing.T) {
	raw := readTestdata(t, "yuv420-8.avif")
	cases := []struct {
		name                       string
		primaries, transfer, matrx uint16
		full                       bool
	}{
		{"Display P3", 12, 13, 6, false},
		{"BT.2020 HLG", 9, 18, 9, true},
		{"gamma 2.2", 1, 4, 1, false},
		{"linear", 1, 8, 6, true},
		{"SMPTE 431", 11, 13, 6, false},
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
	raw := readTestdata(t, "yuv444-8.avif")
	for _, transfer := range []uint16{16, 18} {
		data := withColr(t, raw, nclx(9, transfer, 9, false))
		deep, err := decode(data, deepOptions())
		if err != nil {
			t.Fatal(err)
		}
		plain, err := decode(data, codec.Options{Limits: codec.DefaultLimits()})
		if err != nil {
			t.Fatal(err)
		}
		if deep.Frames[0].Deep == nil || deep.Frames[0].Pix != nil || !deep.Color.HDR() {
			t.Fatalf("transfer %d: not deep", transfer)
		}
		if plain.Frames[0].Pix == nil || plain.Frames[0].Deep != nil {
			t.Fatalf("transfer %d: default path changed", transfer)
		}
		for y := 0; y < plain.Info.Height; y += 5 {
			for x := 0; x < plain.Info.Width; x += 5 {
				d, p := deep.Frames[0].Deep.NRGBA64At(x, y), plain.Frames[0].Pix.NRGBAAt(x, y)
				if d.R != uint16(p.R)*257 || d.G != uint16(p.G)*257 || d.B != uint16(p.B)*257 || d.A != uint16(p.A)*257 {
					t.Fatalf("(%d,%d): %v vs %v", x, y, d, p)
				}
			}
		}
	}
}

func TestEmbeddedProfilesAreReported(t *testing.T) {
	raw := readTestdata(t, "yuv420-10.avif")
	profile := testimg.SampleICCSpace("RGB ", 123)
	for _, kind := range []string{"prof", "rICC"} {
		data := withColr(t, raw, nclx(1, 13, 6, false), isotest.ColrProfile(kind, profile))
		for _, o := range []codec.Options{deepOptions(), {Limits: codec.DefaultLimits()}} {
			img, err := decode(data, o)
			if err != nil {
				t.Fatalf("%s: %v", kind, err)
			}
			if !bytes.Equal(img.Color.ICC, profile) {
				t.Fatalf("%s: %d bytes", kind, len(img.Color.ICC))
			}
			if img.Color.CICP == nil {
				t.Fatalf("%s: the nclx description next to the profile is lost", kind)
			}
		}
	}
	data := withColr(t, raw, isotest.ColrProfile("prof", testimg.SampleICCSpace("CMYK", 20)))
	img, err := decode(data, deepOptions())
	if err != nil {
		t.Fatal(err)
	}
	if img.Color.ICC != nil {
		t.Fatal("CMYK profile reported for RGB output")
	}
}

func TestDeepAnimationFramesStayEightBit(t *testing.T) {
	img, err := decode(readTestdata(t, "anim.avif"), codec.Options{Limits: codec.DefaultLimits(), Frames: codec.AllFrames, Deep: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(img.Frames) < 2 {
		t.Fatalf("%d frames", len(img.Frames))
	}
	for _, f := range img.Frames {
		if f.Deep != nil || f.Pix == nil {
			t.Fatal("8 bit animation became deep")
		}
	}
	if img.Color.CICP == nil {
		t.Fatal("animation lost its colour description")
	}
}

func TestDeepAnimationOfAnHDRPictureWidensEveryFrame(t *testing.T) {
	data := readTestdata(t, "anim.avif")
	at := bytes.Index(data, []byte("colrnclx"))
	if at < 0 {
		t.Fatal("no nclx box in the sample")
	}
	data[at+8], data[at+9], data[at+10], data[at+11], data[at+12], data[at+13] = 0, 9, 0, 16, 0, 9
	img, err := decode(data, codec.Options{Limits: codec.DefaultLimits(), Frames: codec.AllFrames, Deep: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(img.Frames) < 2 {
		t.Fatalf("%d frames", len(img.Frames))
	}
	for i, f := range img.Frames {
		if f.Deep == nil || f.Pix != nil {
			t.Fatalf("frame %d is not deep", i)
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
