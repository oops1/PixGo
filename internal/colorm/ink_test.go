package colorm

import (
	"bytes"
	"context"
	"image"
	"image/color"
	stdjpeg "image/jpeg"
	"os"
	"path/filepath"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
	cjpeg "github.com/oops1/PixGo/internal/codec/jpeg"
)

func inkCorpus(t testing.TB) map[string][]byte {
	t.Helper()
	src, err := os.ReadFile(filepath.Join("..", "jpegdec", "testdata", "video-001.cmyk.jpeg"))
	if err != nil {
		t.Fatal(err)
	}
	at := bytes.Index(src, []byte("Adobe"))
	if at < 0 {
		t.Fatal("no Adobe marker")
	}
	ycck := append([]byte(nil), src...)
	ycck[at+11] = 2
	cmyk := append([]byte(nil), src...)
	cmyk[at+11] = 0
	return map[string][]byte{"cmyk": cmyk, "ycck": ycck}
}

func embedICC(data, profile []byte, chunk int) []byte {
	count := (len(profile) + chunk - 1) / chunk
	out := append([]byte(nil), data[:2]...)
	for i := 0; i < count; i++ {
		part := profile[i*chunk : min(len(profile), (i+1)*chunk)]
		body := append([]byte("ICC_PROFILE\x00"), byte(i+1), byte(count))
		body = append(body, part...)
		out = append(out, 0xff, 0xe2, byte((len(body)+2)>>8), byte(len(body)+2))
		out = append(out, body...)
	}
	return append(out, data[2:]...)
}

func stdInk(t testing.TB, data []byte) *image.CMYK {
	t.Helper()
	img, err := stdjpeg.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	return img.(*image.CMYK)
}

func decodeInkFile(t testing.TB, data []byte, deep bool) *codec.Image {
	t.Helper()
	img, err := cjpeg.Decoder{}.Decode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Options{Limits: codec.DefaultLimits(), Deep: deep})
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func samePix(a, b *image.NRGBA) bool {
	return a.Bounds() == b.Bounds() && bytes.Equal(a.Pix, b.Pix)
}

func naiveOf(src *image.CMYK) *image.NRGBA {
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

func displays() map[string]DisplayProvider {
	return map[string]DisplayProvider{
		"srgb": nil,
		"p3":   func() ([]byte, string) { return displayP3(), "Display P3 monitor" },
	}
}

func TestLoadedCMYKJPEGIsConvertedWithItsProfile(t *testing.T) {
	profile := cmykProfile(9, true, false)
	for name, base := range inkCorpus(t) {
		for display, provider := range displays() {
			data := embedICC(base, profile, 20000)
			m := newManager(Config{Enabled: true, SystemProfile: provider != nil}, provider)
			img := decodeInkFile(t, data, true)
			if img.Frames[0].CMYK == nil {
				t.Fatalf("%s: no ink frame", name)
			}
			rep, err := m.ConvertImage(img)
			if err != nil {
				t.Fatal(err)
			}
			frame := img.Frames[0]
			if frame.Pix == nil || frame.CMYK != nil || frame.Deep != nil {
				t.Fatalf("%s/%s: frame not converted", name, display)
			}
			tf, ok := m.ResolveCMYK(codec.ColorInfo{ICC: profile})
			if !ok {
				t.Fatal("no CMYK transform")
			}
			want := image.NewNRGBA(image.Rect(0, 0, frame.Pix.Bounds().Dx(), frame.Pix.Bounds().Dy()))
			if err := tf.ConvertCMYK(want, stdInk(t, data)); err != nil {
				t.Fatal(err)
			}
			if !samePix(frame.Pix, want) {
				t.Fatalf("%s/%s: output differs from ConvertCMYK of the standard library samples", name, display)
			}
			naive := naiveOf(stdInk(t, data))
			if samePix(frame.Pix, naive) {
				t.Fatalf("%s/%s: profile had no effect", name, display)
			}
			if !rep.Managed || !rep.CMYK || !rep.Converted || rep.Profile != "Test CMYK" || rep.Origin != OriginEmbedded {
				t.Fatalf("%s/%s: %+v", name, display, rep)
			}
			if display == "p3" && rep.Display != "Display P3 monitor" {
				t.Fatalf("display %q", rep.Display)
			}
			if display == "srgb" && rep.Display != "" {
				t.Fatalf("display %q", rep.Display)
			}
		}
	}
}

func TestCMYKFrameWithoutAUsableTransformFallsBackToTheNaiveConversion(t *testing.T) {
	for name, base := range inkCorpus(t) {
		want := naiveOf(stdInk(t, base))
		disabled := newManager(Config{}, nil)
		var nilManager *Manager
		for label, m := range map[string]*Manager{"disabled": disabled, "nil": nilManager} {
			img := decodeInkFile(t, embedICC(base, cmykProfile(5, true, false), 1<<15), true)
			rep, err := m.ConvertImage(img)
			if err != nil {
				t.Fatalf("%s/%s: %v", name, label, err)
			}
			if !samePix(img.Frames[0].Pix, want) || img.Frames[0].CMYK != nil {
				t.Fatalf("%s/%s: not the naive conversion", name, label)
			}
			if rep.Converted || rep.Managed {
				t.Fatalf("%s/%s: %+v", name, label, rep)
			}
		}
		enabled := newManager(Config{Enabled: true}, nil)
		img := decodeInkFile(t, embedICC(base, cmykProfile(5, true, false), 1<<15), true)
		img.Color = codec.ColorInfo{ICC: displayP3()}
		rep, err := enabled.ConvertImage(img)
		if err != nil || !samePix(img.Frames[0].Pix, want) || rep.Converted {
			t.Fatalf("%s: unusable profile: %v %+v", name, err, rep)
		}
	}
}

func TestPlainConvFrameConvertsStrayInkNaively(t *testing.T) {
	ink := stdInk(t, inkCorpus(t)["cmyk"])
	f := codec.Frame{CMYK: ink}
	c := &Conv{}
	if err := c.Frame(&f); err != nil || f.CMYK != nil || !samePix(f.Pix, naiveOf(ink)) {
		t.Fatalf("%v", err)
	}
}

func TestCMYKTileReaderIsConvertedWithTheProfile(t *testing.T) {
	profile := cmykProfile(9, true, false)
	for name, base := range inkCorpus(t) {
		data := embedICC(base, profile, 30000)
		m := newManager(Config{Enabled: true}, nil)
		tr, err := cjpeg.Decoder{}.OpenTiled(context.Background(), bytes.NewReader(data), int64(len(data)), codec.DefaultLimits())
		if err != nil {
			t.Fatal(err)
		}
		wrapped, rep := m.WrapTiles(tr)
		if wrapped == tr || !rep.CMYK || !rep.Converted || rep.Profile != "Test CMYK" {
			t.Fatalf("%s: tiles not wrapped: %+v", name, rep)
		}
		tf, ok := m.ResolveCMYK(codec.ColorInfo{ICC: profile})
		if !ok {
			t.Fatal("no CMYK transform")
		}
		ink := stdInk(t, data)
		want := image.NewNRGBA(image.Rect(0, 0, ink.Bounds().Dx(), ink.Bounds().Dy()))
		if err := tf.ConvertCMYK(want, ink); err != nil {
			t.Fatal(err)
		}
		got, err := wrapped.Overview(context.Background(), 0)
		if err != nil || !samePix(got, want) {
			t.Fatalf("%s: overview differs: %v", name, err)
		}
		r := image.Rect(3, 5, ink.Bounds().Dx()-7, ink.Bounds().Dy()-2)
		part, err := wrapped.Region(context.Background(), 0, r)
		if err != nil {
			t.Fatal(err)
		}
		if !samePix(part, croppedNRGBA(want, r)) {
			t.Fatalf("%s: region differs", name)
		}
		none, err := wrapped.Region(context.Background(), 0, image.Rect(ink.Bounds().Dx()+4, 0, ink.Bounds().Dx()+8, 4))
		if err != nil || !none.Bounds().Empty() {
			t.Fatalf("%s: region outside: %v", name, err)
		}
		tr.Close()

		noProfile, err := cjpeg.Decoder{}.OpenTiled(context.Background(), bytes.NewReader(base), int64(len(base)), codec.DefaultLimits())
		if err != nil {
			t.Fatal(err)
		}
		if again, _ := m.WrapTiles(noProfile); again != noProfile {
			t.Fatalf("%s: a file without a profile was wrapped", name)
		}
		noProfile.Close()

		off := newManager(Config{}, nil)
		plain, err := cjpeg.Decoder{}.OpenTiled(context.Background(), bytes.NewReader(data), int64(len(data)), codec.DefaultLimits())
		if err != nil {
			t.Fatal(err)
		}
		if again, _ := off.WrapTiles(plain); again != plain {
			t.Fatalf("%s: wrapped while disabled", name)
		}
		naive, err := plain.Overview(context.Background(), 0)
		if err != nil || !samePix(naive, naiveOf(ink)) {
			t.Fatalf("%s: disabled overview is not naive: %v", name, err)
		}
		plain.Close()
	}
}

func croppedNRGBA(src *image.NRGBA, r image.Rectangle) *image.NRGBA {
	out := image.NewNRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	for y := 0; y < r.Dy(); y++ {
		copy(out.Pix[y*out.Stride:y*out.Stride+r.Dx()*4], src.Pix[src.PixOffset(r.Min.X, r.Min.Y+y):][:r.Dx()*4])
	}
	return out
}
