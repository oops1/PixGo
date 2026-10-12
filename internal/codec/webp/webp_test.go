package webp

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/png"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	xwebp "golang.org/x/image/webp"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/testimg"
)

func readTestdata(t testing.TB, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func chunk(id string, payload []byte) []byte {
	out := append([]byte(id), binary.LittleEndian.AppendUint32(nil, uint32(len(payload)))...)
	out = append(out, payload...)
	if len(payload)%2 == 1 {
		out = append(out, 0)
	}
	return out
}

func riff(chunks ...[]byte) []byte {
	var body []byte
	for _, c := range chunks {
		body = append(body, c...)
	}
	out := []byte("RIFF")
	out = binary.LittleEndian.AppendUint32(out, uint32(4+len(body)))
	out = append(out, "WEBP"...)
	return append(out, body...)
}

func vp8x(flags byte, width, height int) []byte {
	payload := []byte{flags, 0, 0, 0}
	payload = append(payload, byte(width-1), byte((width-1)>>8), byte((width-1)>>16))
	payload = append(payload, byte(height-1), byte((height-1)>>8), byte((height-1)>>16))
	return chunk("VP8X", payload)
}

func vp8(width, height int) []byte {
	payload := []byte{0x10, 0, 0, 0x9d, 0x01, 0x2a}
	payload = binary.LittleEndian.AppendUint16(payload, uint16(width))
	payload = binary.LittleEndian.AppendUint16(payload, uint16(height))
	return chunk("VP8 ", payload)
}

func vp8l(width, height int, alpha bool) []byte {
	bitsWord := uint32(width-1) | uint32(height-1)<<14
	if alpha {
		bitsWord |= 1 << 28
	}
	payload := []byte{0x2f}
	payload = binary.LittleEndian.AppendUint32(payload, bitsWord)
	return chunk("VP8L", payload)
}

func exifChunk(tiff []byte, prefixed bool) []byte {
	if prefixed {
		tiff = append([]byte("Exif\x00\x00"), tiff...)
	}
	return chunk("EXIF", tiff)
}

func probe(data []byte) (codec.Info, error) {
	return Decoder{}.Probe(context.Background(), bytes.NewReader(data), int64(len(data)), codec.DefaultLimits())
}

func decode(data []byte) (*codec.Image, error) {
	return Decoder{}.Decode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Options{Limits: codec.DefaultLimits()})
}

func TestSniffAndExtensions(t *testing.T) {
	d := Decoder{}
	if !d.Sniff(riff(vp8(1, 1))) {
		t.Fatal("WebP not recognised")
	}
	for _, head := range [][]byte{nil, []byte("RIFF"), []byte("RIFF\x00\x00\x00\x00WAVE"), []byte("RIFX\x00\x00\x00\x00WEBP"), []byte("GIF89a")} {
		if d.Sniff(head) {
			t.Fatalf("%q sniffed as WebP", head)
		}
	}
	if d.ID() != codec.WebP || d.Name() != "WebP" || len(d.Extensions()) != 1 || d.Extensions()[0] != ".webp" {
		t.Fatal("identity")
	}
}

func TestRealFilesMatchStandardLibrary(t *testing.T) {
	for _, name := range []string{"blue-purple-pink.lossy.webp", "gopher-doc.1bpp.lossless.webp", "gopher-doc.with-alpha.lossless.webp", "yellow_rose.lossy-with-alpha.webp"} {
		data := readTestdata(t, name)
		config, err := xwebp.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		info, err := probe(data)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if info.Width != config.Width || info.Height != config.Height || info.Format != codec.WebP || info.FormatName != "WebP" || info.Frames != 1 || info.Animated || info.Orientation != codec.OrientNormal {
			t.Fatalf("%s: %+v vs %v", name, info, config)
		}
		reference, err := xwebp.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		img, err := decode(data)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		got := img.First()
		for y := 0; y < info.Height; y++ {
			for x := 0; x < info.Width; x++ {
				want := color.NRGBAModel.Convert(reference.At(x, y)).(color.NRGBA)
				if have := got.NRGBAAt(x, y); have != want {
					t.Fatalf("%s: (%d,%d) = %v want %v", name, x, y, have, want)
				}
			}
		}
	}
}

func TestAlphaDetectionOnRealFiles(t *testing.T) {
	for name, want := range map[string]bool{
		"blue-purple-pink.lossy.webp":         false,
		"gopher-doc.1bpp.lossless.webp":       false,
		"gopher-doc.with-alpha.lossless.webp": true,
		"yellow_rose.lossy-with-alpha.webp":   true,
	} {
		info, err := probe(readTestdata(t, name))
		if err != nil || info.HasAlpha != want {
			t.Fatalf("%s: %+v %v", name, info, err)
		}
		wantDepth := 24
		if want {
			wantDepth = 32
		}
		if info.BitDepth != wantDepth {
			t.Fatalf("%s: depth %d", name, info.BitDepth)
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

func TestBareBitstreamHeaders(t *testing.T) {
	info, err := probe(riff(vp8(640, 480)))
	if err != nil || info.Width != 640 || info.Height != 480 || info.HasAlpha || info.BitDepth != 24 {
		t.Fatalf("lossy: %+v %v", info, err)
	}
	info, err = probe(riff(vp8l(1000, 20, true)))
	if err != nil || info.Width != 1000 || info.Height != 20 || !info.HasAlpha || info.BitDepth != 32 {
		t.Fatalf("lossless: %+v %v", info, err)
	}
	info, err = probe(riff(vp8l(10000, 9999, false)))
	if err != nil || info.Width != 10000 || info.Height != 9999 || info.HasAlpha {
		t.Fatalf("largest lossless: %+v %v", info, err)
	}
}

func TestExtendedHeaderStillImage(t *testing.T) {
	info, err := probe(riff(vp8x(flagAlpha, 300, 200), chunk("ALPH", []byte{0, 1, 2}), vp8(300, 200)))
	if err != nil || info.Width != 300 || info.Height != 200 || !info.HasAlpha || info.Animated || info.Frames != 1 {
		t.Fatalf("%+v %v", info, err)
	}
	info, err = probe(riff(vp8x(0, 300, 200), vp8(300, 200)))
	if err != nil || info.HasAlpha {
		t.Fatalf("%+v %v", info, err)
	}
	if _, err := probe(riff(vp8x(0, 300, 200), vp8(301, 200))); !errors.Is(err, codec.ErrCorrupt) {
		t.Fatalf("canvas mismatch: %v", err)
	}
}

func TestAnimatedHeaderIsReportedButNotDecoded(t *testing.T) {
	anim := chunk("ANIM", []byte{0, 0, 0, 0, 0, 0})
	frame := chunk("ANMF", make([]byte, 16+20))
	data := riff(vp8x(flagAnimation|flagAlpha, 500, 400), anim, frame, frame, frame)
	info, err := probe(data)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Animated || info.Frames != 3 || info.Width != 500 || info.Height != 400 || !info.HasAlpha {
		t.Fatalf("%+v", info)
	}
	if _, err := decode(data); !errors.Is(err, codec.ErrUnsupported) {
		t.Fatalf("decode: %v", err)
	}
	noFrames := riff(vp8x(flagAnimation, 5, 5), anim)
	if info, err := probe(noFrames); err != nil || !info.Animated || info.Frames != 1 {
		t.Fatalf("no ANMF chunks: %+v %v", info, err)
	}
}

func TestOrientationFromExifChunk(t *testing.T) {
	for _, bigEndian := range []bool{false, true} {
		for v := uint16(1); v <= 8; v++ {
			tiff := testimg.OrientationTIFF(bigEndian, v)
			for name, data := range map[string][]byte{
				"after image":     riff(vp8x(0x08, 64, 32), vp8(64, 32), exifChunk(tiff, false)),
				"before image":    riff(vp8x(0x08, 64, 32), exifChunk(tiff, false), vp8(64, 32)),
				"with Exif\\0\\0": riff(vp8x(0x08, 64, 32), vp8(64, 32), exifChunk(tiff, true)),
			} {
				info, err := probe(data)
				if err != nil || info.Orientation != codec.Orientation(v) {
					t.Fatalf("%s bigEndian=%v v=%d: %+v %v", name, bigEndian, v, info, err)
				}
			}
		}
	}
	info, err := probe(riff(vp8x(0x08, 64, 32), vp8(64, 32), exifChunk(testimg.OrientationTIFF(false, 99), false)))
	if err != nil || info.Orientation != codec.OrientNormal {
		t.Fatalf("invalid value: %+v %v", info, err)
	}
	info, err = probe(riff(vp8x(0x08, 64, 32), vp8(64, 32), chunk("EXIF", []byte("junk"))))
	if err != nil || info.Orientation != codec.OrientNormal {
		t.Fatalf("junk exif: %+v %v", info, err)
	}
}

func TestRIFFSizeVariants(t *testing.T) {
	data := riff(vp8(77, 33))
	withTrailing := append(append([]byte(nil), data...), bytes.Repeat([]byte{0xee}, 40)...)
	if info, err := probe(withTrailing); err != nil || info.Width != 77 {
		t.Fatalf("trailing data: %+v %v", info, err)
	}
	oversize := append([]byte(nil), data...)
	binary.LittleEndian.PutUint32(oversize[4:], 1<<30)
	if info, err := probe(oversize); err != nil || info.Width != 77 {
		t.Fatalf("oversized RIFF length: %+v %v", info, err)
	}
	tiny := append([]byte(nil), data...)
	binary.LittleEndian.PutUint32(tiny[4:], 2)
	if _, err := probe(tiny); !errors.Is(err, codec.ErrCorrupt) {
		t.Fatalf("RIFF length 2: %v", err)
	}
}

func TestMalformedContainers(t *testing.T) {
	good := riff(vp8(10, 10))
	cases := map[string][]byte{
		"empty":               nil,
		"header only":         good[:12],
		"not riff":            append([]byte("RIFX"), good[4:]...),
		"wave":                append(append([]byte(nil), good[:8]...), append([]byte("WAVE"), good[12:]...)...),
		"no chunks":           riff(),
		"unknown chunks only": riff(chunk("JUNK", []byte{1, 2, 3})),
		"chunk beyond file":   append(append([]byte(nil), good[:12]...), 'V', 'P', '8', ' ', 0xff, 0xff, 0xff, 0x7f, 1, 2, 3),
		"bad VP8 start code":  riff(chunk("VP8 ", []byte{0x10, 0, 0, 1, 2, 3, 10, 0, 10, 0})),
		"VP8 inter frame":     riff(chunk("VP8 ", []byte{0x11, 0, 0, 0x9d, 1, 0x2a, 10, 0, 10, 0})),
		"VP8 too short":       riff(chunk("VP8 ", []byte{0x10, 0, 0})),
		"bad VP8L signature":  riff(chunk("VP8L", []byte{0x2e, 1, 2, 3, 4})),
		"VP8L too short":      riff(chunk("VP8L", []byte{0x2f, 1})),
		"VP8X too short":      riff(chunk("VP8X", []byte{0, 0, 0})),
		"zero VP8 dimensions": riff(vp8(0, 10)),
	}
	for name, data := range cases {
		_, probeErr := probe(data)
		_, decodeErr := decode(data)
		if probeErr == nil || decodeErr == nil {
			t.Fatalf("%s: probe=%v decode=%v", name, probeErr, decodeErr)
		}
		testimg.RequireCodecError(t, probeErr)
		testimg.RequireCodecError(t, decodeErr)
	}
}

func TestHugeCanvasRejectedQuickly(t *testing.T) {
	data := riff(vp8x(0, 1<<24, 1<<24), vp8(10, 10))
	testimg.Within(t, 10*time.Second, func() {
		if _, err := probe(data); !errors.Is(err, codec.ErrTooLarge) && !errors.Is(err, codec.ErrCorrupt) {
			t.Errorf("probe: %v", err)
		}
		animated := riff(vp8x(flagAnimation, 1<<24, 1<<24), chunk("ANMF", make([]byte, 16)))
		if _, err := decode(animated); !errors.Is(err, codec.ErrTooLarge) {
			t.Errorf("animated decode: %v", err)
		}
		lossless := riff(vp8l(16384, 16384, true))
		lim := codec.DefaultLimits()
		lim.MaxPixels = 1_000_000
		if _, err := (Decoder{}).Decode(context.Background(), bytes.NewReader(lossless), int64(len(lossless)), codec.Options{Limits: lim}); !errors.Is(err, codec.ErrTooLarge) {
			t.Errorf("lossless decode: %v", err)
		}
	})
}

func TestManyTinyChunksFinishQuickly(t *testing.T) {
	var many [][]byte
	many = append(many, vp8(5, 5))
	junk := chunk("JUNK", nil)
	for i := 0; i < 300_000; i++ {
		many = append(many, junk)
	}
	data := riff(many...)
	testimg.Within(t, 10*time.Second, func() {
		if info, err := probe(data); err != nil || info.Width != 5 {
			t.Errorf("%+v %v", info, err)
		}
	})
}

func TestEveryTruncatedPrefix(t *testing.T) {
	data := readTestdata(t, "gopher-doc.1bpp.lossless.webp")
	for n := 0; n < len(data); n++ {
		prefix := data[:n]
		if _, err := probe(prefix); err != nil {
			testimg.RequireCodecError(t, err)
		}
		_, err := decode(prefix)
		if err == nil && n < len(data)-1 {
			t.Fatalf("prefix %d/%d decoded", n, len(data))
		}
		if err != nil {
			testimg.RequireCodecError(t, err)
		}
	}
	lossy := readTestdata(t, "blue-purple-pink.lossy.webp")
	for n := 0; n < len(lossy); n += 7 {
		if _, err := decode(lossy[:n]); err != nil {
			testimg.RequireCodecError(t, err)
		}
	}
}

func TestLimitsAndCancellation(t *testing.T) {
	data := readTestdata(t, "blue-purple-pink.lossy.webp")
	lim := codec.DefaultLimits()
	lim.MaxPixels = 10
	if _, err := (Decoder{}).Decode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Options{Limits: lim}); !errors.Is(err, codec.ErrTooLarge) {
		t.Fatalf("pixels: %v", err)
	}
	lim = codec.DefaultLimits()
	lim.MaxFileBytes = 100
	if _, err := (Decoder{}).Probe(context.Background(), bytes.NewReader(data), int64(len(data)), lim); !errors.Is(err, codec.ErrTooLarge) {
		t.Fatalf("file: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (Decoder{}).Decode(ctx, bytes.NewReader(data), int64(len(data)), codec.Options{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
	if _, err := (Decoder{}).Decode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Options{}); err != nil {
		t.Fatalf("zero limits: %v", err)
	}
}

func TestCorruptedBytesNeverPanic(t *testing.T) {
	rng := rand.New(rand.NewSource(11))
	for _, name := range []string{"gopher-doc.1bpp.lossless.webp", "blue-purple-pink.lossy.webp", "yellow_rose.lossy-with-alpha.webp"} {
		data := readTestdata(t, name)
		for i := 0; i < 150; i++ {
			mutated := append([]byte(nil), data...)
			for k := 0; k < 1+rng.Intn(4); k++ {
				mutated[rng.Intn(len(mutated))] = byte(rng.Intn(256))
			}
			if _, err := (Decoder{}).Decode(context.Background(), bytes.NewReader(mutated), int64(len(mutated)), codec.Options{Limits: testimg.SmallLimits()}); err != nil {
				testimg.RequireCodecError(t, err)
			}
		}
	}
}

func TestImageType(t *testing.T) {
	img, err := decode(readTestdata(t, "gopher-doc.with-alpha.lossless.webp"))
	if err != nil {
		t.Fatal(err)
	}
	var _ *image.NRGBA = img.First()
	if img.Scale != 1 || len(img.Frames) != 1 || img.Info.Format != codec.WebP {
		t.Fatalf("%+v", img)
	}
}

func FuzzDecode(f *testing.F) {
	f.Add(riff(vp8(10, 10)))
	f.Add(riff(vp8x(flagAnimation|flagAlpha, 40, 30), chunk("ANMF", make([]byte, 20)), exifChunk(testimg.OrientationTIFF(false, 6), true)))
	f.Add(riff(vp8l(7, 7, true)))
	f.Add(riff(vp8x(0x08, 64, 32), vp8(64, 32), exifChunk(testimg.OrientationTIFF(true, 3), false)))
	for _, name := range []string{"gopher-doc.1bpp.lossless.webp", "blue-purple-pink.lossy.webp"} {
		f.Add(readTestdata(f, name))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		lim := testimg.SmallLimits()
		ctx := context.Background()
		Decoder{}.Sniff(data)
		walkChunks(ctx, bytes.NewReader(data), int64(len(data)))
		Decoder{}.Probe(ctx, bytes.NewReader(data), int64(len(data)), lim)
		img, err := Decoder{}.Decode(ctx, bytes.NewReader(data), int64(len(data)), codec.Options{Limits: lim})
		if err == nil && (img == nil || img.First() == nil) {
			t.Fatal("success without pixels")
		}
		if err != nil {
			testimg.RequireCodecError(t, err)
		}
	})
}
