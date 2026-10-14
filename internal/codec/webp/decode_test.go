package webp

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"testing"

	vpx "github.com/gen2brain/vpx/webp"
	xwebp "golang.org/x/image/webp"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/testimg"
)

func clip8(v float64) uint8 {
	return uint8(math.Max(0, math.Min(255, math.Round(v))))
}

func studioRange(src *image.YCbCr, alpha *image.NYCbCrA) *image.NRGBA {
	bounds := src.Bounds()
	out := image.NewNRGBA(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			luma := 1.164383 * (float64(src.Y[src.YOffset(x, y)]) - 16)
			index := src.COffset(x, y)
			cb, cr := float64(src.Cb[index])-128, float64(src.Cr[index])-128
			a := uint8(255)
			if alpha != nil {
				a = alpha.A[alpha.AOffset(x, y)]
			}
			out.SetNRGBA(x, y, color.NRGBA{
				R: clip8(luma + 1.596027*cr),
				G: clip8(luma - 0.391762*cb - 0.812968*cr),
				B: clip8(luma + 2.017232*cb),
				A: a,
			})
		}
	}
	return out
}

func standardLibraryReference(t testing.TB, data []byte) *image.NRGBA {
	t.Helper()
	decoded, err := xwebp.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	switch v := decoded.(type) {
	case *image.NRGBA:
		return v
	case *image.YCbCr:
		return studioRange(v, nil)
	case *image.NYCbCrA:
		return studioRange(&v.YCbCr, v)
	}
	t.Fatalf("unexpected reference type %T", decoded)
	return nil
}

func TestLosslessRealFilesMatchStandardLibraryExactly(t *testing.T) {
	for _, name := range []string{"gopher-doc.1bpp.lossless.webp", "gopher-doc.with-alpha.lossless.webp"} {
		data := readTestdata(t, name)
		img, err := decode(data)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if reference := standardLibraryReference(t, data); !testimg.Identical(img.First(), reference) {
			t.Fatalf("%s: differs from the standard library by %d", name, testimg.MaxDelta(img.First(), reference))
		}
	}
}

func TestLossyRealFilesStayCloseToStandardLibraryReference(t *testing.T) {
	for _, name := range []string{"blue-purple-pink.lossy.webp", "yellow_rose.lossy-with-alpha.webp"} {
		data := readTestdata(t, name)
		img, err := decode(data)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		reference := standardLibraryReference(t, data)
		if !alphaIdentical(img.First(), reference) {
			t.Fatalf("%s: alpha plane differs", name)
		}
		if quality := psnr(img.First(), reference, 200); quality < 38 {
			t.Fatalf("%s: PSNR against the reference is %.1f dB", name, quality)
		}
	}
}

func TestFlatColoursUseStudioRangeConversion(t *testing.T) {
	for _, c := range []color.NRGBA{
		{0, 0, 0, 255}, {255, 255, 255, 255}, {255, 0, 0, 255}, {0, 255, 0, 255}, {0, 0, 255, 255},
		{128, 128, 128, 255}, {200, 30, 90, 255}, {16, 16, 16, 255}, {235, 235, 235, 255}, {12, 140, 220, 255},
	} {
		data := vpxEncode(t, solid(32, 32, c), vpx.EncodeOptions{Quality: 100, Method: 4})
		img, err := decode(data)
		if err != nil {
			t.Fatal(err)
		}
		reference := standardLibraryReference(t, data)
		for _, p := range [][2]int{{0, 0}, {15, 15}, {31, 31}, {7, 24}} {
			got, want := img.First().NRGBAAt(p[0], p[1]), reference.NRGBAAt(p[0], p[1])
			for _, d := range [3]int{int(got.R) - int(want.R), int(got.G) - int(want.G), int(got.B) - int(want.B)} {
				if d < -2 || d > 2 {
					t.Fatalf("%v at %v: got %v, reference %v", c, p, got, want)
				}
			}
			for _, d := range [3]int{int(got.R) - int(c.R), int(got.G) - int(c.G), int(got.B) - int(c.B)} {
				if d < -6 || d > 6 {
					t.Fatalf("%v at %v: got %v", c, p, got)
				}
			}
		}
	}
}

func TestLosslessPixelsMatchReferencePNG(t *testing.T) {
	file, err := os.Open(filepath.Join("testdata", "gopher-doc.1bpp.png"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	reference, err := png.Decode(file)
	if err != nil {
		t.Fatal(err)
	}
	img, err := decode(readTestdata(t, "gopher-doc.1bpp.lossless.webp"))
	if err != nil {
		t.Fatal(err)
	}
	bounds := reference.Bounds()
	if img.First().Bounds().Dx() != bounds.Dx() || img.First().Bounds().Dy() != bounds.Dy() {
		t.Fatalf("size %v vs %v", img.First().Bounds(), bounds)
	}
	for y := 0; y < bounds.Dy(); y++ {
		for x := 0; x < bounds.Dx(); x++ {
			want := color.NRGBAModel.Convert(reference.At(bounds.Min.X+x, bounds.Min.Y+y)).(color.NRGBA)
			if have := img.First().NRGBAAt(x, y); have != want {
				t.Fatalf("(%d,%d) = %v want %v", x, y, have, want)
			}
		}
	}
}

func TestDecodedImageShape(t *testing.T) {
	for _, name := range realFiles {
		for _, frames := range []codec.FrameSel{codec.FirstFrame, codec.AllFrames} {
			data := readTestdata(t, name)
			img, err := decodeWith(data, codec.Options{Limits: codec.DefaultLimits(), Frames: frames})
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			info, _ := probe(data)
			pix := img.First()
			if img.Scale != 1 || len(img.Frames) != 1 || img.Info != info || img.Loop != 0 || img.Frames[0].Delay != 0 {
				t.Fatalf("%s: %+v", name, img)
			}
			if pix.Rect != image.Rect(0, 0, info.Width, info.Height) || pix.Stride != info.Width*4 || len(pix.Pix) != info.Width*info.Height*4 {
				t.Fatalf("%s: rect %v stride %d", name, pix.Rect, pix.Stride)
			}
		}
	}
}

func TestOpaqueLossyStillIsFullyOpaque(t *testing.T) {
	img, err := decode(readTestdata(t, "blue-purple-pink.lossy.webp"))
	if err != nil {
		t.Fatal(err)
	}
	pix := img.First().Pix
	for i := 3; i < len(pix); i += 4 {
		if pix[i] != 0xff {
			t.Fatalf("pixel %d has alpha %d", i/4, pix[i])
		}
	}
}

func TestStillsAreDecodedWithoutExtraBufferCopies(t *testing.T) {
	source := smooth(1600, 1000)
	for name, o := range map[string]vpx.EncodeOptions{"lossy": {Quality: 80, Method: 2}, "lossless": optionsLossless()} {
		data := vpxEncode(t, source, o)
		own := func() {
			if _, err := decode(data); err != nil {
				t.Fatal(err)
			}
		}
		library := func() {
			if _, err := vpx.Decode(bytes.NewReader(data), vpx.Options{ToRGBA: name == "lossy", Threads: decodeThreads}); err != nil {
				t.Fatal(err)
			}
		}
		allocated := func(fn func()) uint64 {
			defer debug.SetGCPercent(debug.SetGCPercent(-1))
			defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(1))
			least := uint64(math.MaxUint64)
			for i := 0; i < 6; i++ {
				var before, after runtime.MemStats
				runtime.ReadMemStats(&before)
				fn()
				runtime.ReadMemStats(&after)
				least = min(least, after.TotalAlloc-before.TotalAlloc)
			}
			return least
		}
		mine, theirs := allocated(own), allocated(library)
		if float64(mine) > float64(theirs)*1.02+(256<<10) {
			t.Fatalf("%s: %d bytes allocated against %d by the library alone", name, mine, theirs)
		}
	}
}

func TestStraightAlphaInvertsThePremultiplicationOfTheLibrary(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 256, 256))
	for alpha := 0; alpha < 256; alpha++ {
		for value := 0; value < 256; value++ {
			stored := value
			if alpha > 0 && alpha < 255 {
				stored = int(premultiplied(uint32(value), uint32(alpha)))
			}
			i := src.PixOffset(value, alpha)
			src.Pix[i], src.Pix[i+1], src.Pix[i+2], src.Pix[i+3] = uint8(stored), uint8(stored), uint8(stored), uint8(alpha)
		}
	}
	out := straightAlpha(src)
	for alpha := 0; alpha < 256; alpha++ {
		previous := -1
		for value := 0; value < 256; value++ {
			got := out.NRGBAAt(value, alpha)
			switch {
			case alpha == 0:
				if got != (color.NRGBA{}) {
					t.Fatalf("transparent pixel became %v", got)
				}
				continue
			case alpha == 255:
				if int(got.R) != value || got.A != 255 {
					t.Fatalf("opaque pixel %d became %v", value, got)
				}
				continue
			}
			if got.A != uint8(alpha) || got.R != got.G || got.G != got.B {
				t.Fatalf("alpha %d value %d: %v", alpha, value, got)
			}
			if (value == 0 || value == 255) && int(got.R) != value {
				t.Fatalf("alpha %d: saturated %d became %d", alpha, value, got.R)
			}
			if bound := 255/alpha + 1; int(got.R) > value+bound || int(got.R) < value-bound {
				t.Fatalf("alpha %d: %d became %d", alpha, value, got.R)
			}
			if int(got.R) < previous {
				t.Fatalf("alpha %d: not monotonic at %d", alpha, value)
			}
			previous = int(got.R)
		}
	}
}

func TestStraightAlphaHandlesEmptyAndOffsetImages(t *testing.T) {
	if out := straightAlpha(image.NewRGBA(image.Rectangle{})); !out.Bounds().Empty() {
		t.Fatalf("%v", out.Bounds())
	}
	src := image.NewRGBA(image.Rect(5, 7, 9, 10))
	for i := 0; i < len(src.Pix); i += 4 {
		src.Pix[i], src.Pix[i+1], src.Pix[i+2], src.Pix[i+3] = 100, 50, 25, 200
	}
	out := straightAlpha(src)
	if out.Rect != src.Rect || &out.Pix[0] != &src.Pix[0] {
		t.Fatal("conversion is not in place")
	}
	want := out.NRGBAAt(5, 7)
	if want.A != 200 || want.R < 125 || want.R > 130 {
		t.Fatalf("%v", want)
	}
	if out.NRGBAAt(8, 9) != want {
		t.Fatal("rows differ")
	}
}

func TestLosslessStillWithAlphaRoundTripsExactly(t *testing.T) {
	source := withAlpha(smooth(70, 50))
	data := vpxEncode(t, source, optionsLossless())
	img, err := Decoder{}.Decode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !testimg.Identical(img.First(), source) {
		t.Fatalf("lossless round trip differs by %d", testimg.MaxDelta(img.First(), source))
	}
}
