package webp

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"testing"

	vpx "github.com/gen2brain/vpx/webp"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/testimg"
)

func encodeWith(t testing.TB, img *image.NRGBA, o codec.EncodeOptions) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := (Encoder{}).Encode(&buf, img, o); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func transparentColour(width, height int) *image.NRGBA {
	img := withAlpha(smooth(width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width/8; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(30 + x), G: uint8(200 - y), B: 77, A: 0})
		}
	}
	return img
}

func TestEncoderIdentity(t *testing.T) {
	var e codec.Encoder = Encoder{}
	if e.ID() != codec.WebP || e.Name() != "WebP" || e.Extension() != ".webp" {
		t.Fatalf("%s %s %s", e.ID(), e.Name(), e.Extension())
	}
}

func TestClampQuality(t *testing.T) {
	for _, tc := range []struct{ in, want int }{
		{-100, MinQuality}, {-1, MinQuality}, {0, DefaultQuality}, {1, 1}, {2, 2}, {50, 50}, {80, 80}, {99, 99}, {100, 100}, {101, MaxQuality}, {5000, MaxQuality},
	} {
		if got := ClampQuality(tc.in); got != tc.want {
			t.Fatalf("ClampQuality(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
	if DefaultQuality != 80 {
		t.Fatalf("default quality %d", DefaultQuality)
	}
}

func TestLosslessRoundTripIsExact(t *testing.T) {
	for name, img := range map[string]*image.NRGBA{
		"single pixel":            solid(1, 1, color.NRGBA{R: 9, G: 99, B: 199, A: 255}),
		"single row":              smooth(37, 1),
		"single column":           smooth(1, 37),
		"odd size":                smooth(33, 17),
		"photo":                   testimg.Photo(200, 120),
		"noise with alpha":        testimg.Noise(99, 61, 5, true),
		"gradient alpha":          withAlpha(smooth(64, 48)),
		"transparent keeps rgb":   transparentColour(64, 48),
		"larger than one MB row":  smooth(700, 40),
		"fully transparent black": solid(16, 16, color.NRGBA{}),
		"fully transparent tint":  solid(16, 16, color.NRGBA{R: 200, G: 100, B: 50}),
	} {
		data := encodeWith(t, img, codec.EncodeOptions{Lossless: true})
		decoded, err := decode(data)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !testimg.Identical(decoded.First(), img) {
			t.Fatalf("%s: round trip differs by %d", name, testimg.MaxDelta(decoded.First(), img))
		}
	}
}

func TestLosslessStaysExactAtAnyQuality(t *testing.T) {
	img := withAlpha(testimg.Photo(48, 32))
	for _, quality := range []int{-5, 0, 1, 50, 100, 400} {
		decoded, err := decode(encodeWith(t, img, codec.EncodeOptions{Lossless: true, Quality: quality}))
		if err != nil {
			t.Fatalf("quality %d: %v", quality, err)
		}
		if !testimg.Identical(decoded.First(), img) {
			t.Fatalf("quality %d changed the pixels by %d", quality, testimg.MaxDelta(decoded.First(), img))
		}
	}
}

func TestLosslessOutputDoesNotDependOnQuality(t *testing.T) {
	img := withAlpha(smooth(48, 32))
	reference := encodeWith(t, img, codec.EncodeOptions{Lossless: true})
	for _, quality := range []int{-5, 1, 50, 100, 400} {
		if !bytes.Equal(encodeWith(t, img, codec.EncodeOptions{Lossless: true, Quality: quality}), reference) {
			t.Fatalf("quality %d changed a lossless file", quality)
		}
	}
}

func TestLossyQualityTradesSizeForFidelity(t *testing.T) {
	img := testimg.Photo(240, 160)
	var sizes []int
	var qualities []float64
	for _, quality := range []int{5, 40, 75, 95} {
		data := encodeWith(t, img, codec.EncodeOptions{Quality: quality})
		decoded, err := decode(data)
		if err != nil {
			t.Fatal(err)
		}
		sizes = append(sizes, len(data))
		qualities = append(qualities, psnr(decoded.First(), img, 1))
	}
	for i := 1; i < len(sizes); i++ {
		if sizes[i] <= sizes[i-1] || qualities[i] <= qualities[i-1] {
			t.Fatalf("quality steps are not monotonic: sizes %v PSNR %v", sizes, qualities)
		}
	}
	if qualities[len(qualities)-1] < 33 {
		t.Fatalf("PSNR at quality 95 is %.1f dB", qualities[len(qualities)-1])
	}
}

func TestSmoothImagesSurviveLossyCoding(t *testing.T) {
	img := smooth(160, 120)
	for quality, floor := range map[int]float64{30: 33, 60: 36, 90: 40} {
		decoded, err := decode(encodeWith(t, img, codec.EncodeOptions{Quality: quality}))
		if err != nil {
			t.Fatal(err)
		}
		if got := psnr(decoded.First(), img, 1); got < floor {
			t.Fatalf("quality %d: PSNR %.1f dB, want at least %.0f", quality, got, floor)
		}
	}
}

func TestDefaultAndOutOfRangeQualityMatchTheirClamps(t *testing.T) {
	img := testimg.Photo(64, 48)
	encode := func(q int) []byte { return encodeWith(t, img, codec.EncodeOptions{Quality: q}) }
	if !bytes.Equal(encode(0), encode(80)) {
		t.Fatal("quality 0 is not 80")
	}
	if !bytes.Equal(encode(-7), encode(1)) {
		t.Fatal("negative quality is not 1")
	}
	if !bytes.Equal(encode(1000), encode(100)) {
		t.Fatal("quality 1000 is not 100")
	}
	if bytes.Equal(encode(0), encode(81)) {
		t.Fatal("quality steps have no effect")
	}
}

func TestLossyKeepsTheAlphaPlaneExactly(t *testing.T) {
	img := withAlpha(smooth(96, 64))
	data := encodeWith(t, img, codec.EncodeOptions{Quality: 90})
	decoded, err := decode(data)
	if err != nil {
		t.Fatal(err)
	}
	info, _ := probe(data)
	if !info.HasAlpha || info.BitDepth != 32 {
		t.Fatalf("%+v", info)
	}
	if !alphaIdentical(decoded.First(), img) {
		t.Fatal("alpha plane changed")
	}
	if quality := psnr(decoded.First(), img, 250); quality < 36 {
		t.Fatalf("opaque area PSNR %.1f dB", quality)
	}
}

func TestOpaqueImagesProduceSimpleFiles(t *testing.T) {
	img := smooth(40, 30)
	if ids := chunkIDs(t, encodeWith(t, img, codec.EncodeOptions{Quality: 70})); len(ids) != 1 || ids[0] != "VP8 " {
		t.Fatalf("lossy chunks %v", ids)
	}
	if ids := chunkIDs(t, encodeWith(t, img, codec.EncodeOptions{Lossless: true})); len(ids) != 1 || ids[0] != "VP8L" {
		t.Fatalf("lossless chunks %v", ids)
	}
	if ids := chunkIDs(t, encodeWith(t, withAlpha(img), codec.EncodeOptions{Quality: 70})); len(ids) != 3 || ids[0] != "VP8X" || ids[1] != "ALPH" || ids[2] != "VP8 " {
		t.Fatalf("lossy alpha chunks %v", ids)
	}
}

func TestEncoderWritesNoMetadata(t *testing.T) {
	img := withAlpha(smooth(40, 30))
	for _, o := range []codec.EncodeOptions{{}, {Quality: 10}, {Lossless: true}, {Lossless: true, Quality: 99}} {
		for _, id := range chunkIDs(t, encodeWith(t, img, o)) {
			switch id {
			case "ICCP", "EXIF", "XMP ", "ANIM", "ANMF":
				t.Fatalf("%+v wrote a %q chunk", o, id)
			}
		}
		info, err := probe(encodeWith(t, img, o))
		if err != nil || info.Orientation != codec.OrientNormal || info.Animated || info.Frames != 1 {
			t.Fatalf("%+v: %+v %v", o, info, err)
		}
	}
}

func TestEncodedFilesAreReadableByTheStandardLibrary(t *testing.T) {
	source := withAlpha(testimg.Photo(120, 80))
	lossless := encodeWith(t, source, codec.EncodeOptions{Lossless: true})
	if reference := standardLibraryReference(t, lossless); !testimg.Identical(reference, source) {
		t.Fatalf("lossless file read back differently by %d", testimg.MaxDelta(reference, source))
	}
	smoothSource := withAlpha(smooth(120, 80))
	lossy := encodeWith(t, smoothSource, codec.EncodeOptions{Quality: 85})
	reference := standardLibraryReference(t, lossy)
	if !alphaIdentical(reference, smoothSource) {
		t.Fatal("standard library reads another alpha plane")
	}
	if quality := psnr(reference, smoothSource, 250); quality < 33 {
		t.Fatalf("lossy file read back at %.1f dB", quality)
	}
}

func TestEncodedFilesAreReadableByTheLibraryItself(t *testing.T) {
	source := withAlpha(smooth(50, 40))
	for _, o := range []codec.EncodeOptions{{Quality: 60}, {Lossless: true}} {
		decoded, err := vpx.Decode(bytes.NewReader(encodeWith(t, source, o)), vpx.Options{ToRGBA: true})
		if err != nil {
			t.Fatalf("%+v: %v", o, err)
		}
		if decoded.Bounds() != source.Bounds() {
			t.Fatalf("%+v: %v", o, decoded.Bounds())
		}
	}
}

func TestEncodeRejectsImagesTheFormatCannotHold(t *testing.T) {
	var buf bytes.Buffer
	for name, img := range map[string]*image.NRGBA{
		"nil":          nil,
		"empty":        image.NewNRGBA(image.Rectangle{}),
		"too wide":     image.NewNRGBA(image.Rect(0, 0, MaxDimension+1, 1)),
		"too tall":     image.NewNRGBA(image.Rect(0, 0, 1, MaxDimension+1)),
		"zero height":  image.NewNRGBA(image.Rect(0, 0, 10, 0)),
		"offset empty": image.NewNRGBA(image.Rect(5, 5, 5, 9)),
	} {
		for _, lossless := range []bool{false, true} {
			buf.Reset()
			if err := (Encoder{}).Encode(&buf, img, codec.EncodeOptions{Lossless: lossless}); err == nil {
				t.Fatalf("%s lossless=%v: encoded", name, lossless)
			}
			if buf.Len() != 0 {
				t.Fatalf("%s: wrote %d bytes before failing", name, buf.Len())
			}
		}
	}
}

func TestEncodeAcceptsTheLargestSide(t *testing.T) {
	for _, size := range []image.Rectangle{image.Rect(0, 0, MaxDimension, 1), image.Rect(0, 0, 1, MaxDimension)} {
		img := image.NewNRGBA(size)
		for i := range img.Pix {
			img.Pix[i] = uint8(i)
		}
		for _, lossless := range []bool{false, true} {
			data := encodeWith(t, img, codec.EncodeOptions{Lossless: lossless})
			info, err := probe(data)
			if err != nil || info.Width != size.Dx() || info.Height != size.Dy() {
				t.Fatalf("%v lossless=%v: %+v %v", size, lossless, info, err)
			}
		}
	}
}

func TestEncodeUsesOnlyTheVisibleWindowOfASubImage(t *testing.T) {
	whole := smooth(80, 60)
	window := image.Rect(13, 9, 52, 41)
	view := whole.SubImage(window).(*image.NRGBA)
	decoded, err := decode(encodeWith(t, view, codec.EncodeOptions{Lossless: true}))
	if err != nil {
		t.Fatal(err)
	}
	got := decoded.First()
	if got.Bounds() != image.Rect(0, 0, window.Dx(), window.Dy()) {
		t.Fatalf("%v", got.Bounds())
	}
	for y := 0; y < window.Dy(); y++ {
		for x := 0; x < window.Dx(); x++ {
			if got.NRGBAAt(x, y) != whole.NRGBAAt(window.Min.X+x, window.Min.Y+y) {
				t.Fatalf("(%d,%d) differs", x, y)
			}
		}
	}
}

type failingWriter struct {
	budget int
	err    error
}

func (w *failingWriter) Write(p []byte) (int, error) {
	if len(p) > w.budget {
		return 0, w.err
	}
	w.budget -= len(p)
	return len(p), nil
}

func TestEncodePropagatesWriterErrors(t *testing.T) {
	boom := errors.New("disk full")
	img := withAlpha(smooth(40, 30))
	for _, o := range []codec.EncodeOptions{{Quality: 60}, {Lossless: true}} {
		full := encodeWith(t, img, o)
		for _, budget := range []int{0, 5, 12, 20, len(full) - 1} {
			err := (Encoder{}).Encode(&failingWriter{budget: budget, err: boom}, img, o)
			if !errors.Is(err, boom) {
				t.Fatalf("%+v budget %d: %v", o, budget, err)
			}
		}
	}
}

func TestEncodingIsDeterministic(t *testing.T) {
	img := testimg.Photo(130, 90)
	for _, o := range []codec.EncodeOptions{{Quality: 77}, {Lossless: true}} {
		if !bytes.Equal(encodeWith(t, img, o), encodeWith(t, img, o)) {
			t.Fatalf("%+v: output changes between runs", o)
		}
	}
}
