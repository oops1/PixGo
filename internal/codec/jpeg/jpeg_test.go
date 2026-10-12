package jpeg

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	stdjpeg "image/jpeg"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/testimg"
)

func encodeStd(t testing.TB, img image.Image, quality int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := stdjpeg.Encode(&buf, img, &stdjpeg.Options{Quality: quality}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func rgbaOf(img *image.NRGBA) *image.RGBA {
	out := image.NewRGBA(img.Bounds())
	for i := 0; i < len(img.Pix); i += 4 {
		out.Pix[i], out.Pix[i+1], out.Pix[i+2], out.Pix[i+3] = img.Pix[i], img.Pix[i+1], img.Pix[i+2], 0xff
	}
	return out
}

func withExifOrientation(data []byte, bigEndian bool, orientation uint16) []byte {
	payload := append([]byte("Exif\x00\x00"), testimg.OrientationTIFF(bigEndian, orientation)...)
	length := len(payload) + 2
	segment := []byte{0xff, 0xe1, byte(length >> 8), byte(length)}
	segment = append(segment, payload...)
	out := append([]byte(nil), data[:2]...)
	out = append(out, segment...)
	return append(out, data[2:]...)
}

func decodeBytes(t *testing.T, data []byte) (*codec.Image, error) {
	t.Helper()
	return Decoder{}.Decode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Options{Limits: codec.DefaultLimits()})
}

func TestSniffAndExtensions(t *testing.T) {
	d := Decoder{}
	if !d.Sniff([]byte{0xff, 0xd8, 0xff, 0xe0}) || !d.Sniff([]byte{0xff, 0xd8, 0xff}) {
		t.Fatal("JPEG signature not recognised")
	}
	for _, head := range [][]byte{nil, {0xff}, {0xff, 0xd8}, {0xff, 0xd8, 0x00}, []byte("\x89PNG\r\n\x1a\n"), []byte("GIF89a")} {
		if d.Sniff(head) {
			t.Fatalf("%x sniffed as JPEG", head)
		}
	}
	if d.ID() != codec.JPEG || d.Name() != "JPEG" {
		t.Fatalf("identity %q %q", d.ID(), d.Name())
	}
	for _, e := range d.Extensions() {
		if e != "."+e[1:] || e[0] != '.' {
			t.Fatalf("extension %q", e)
		}
	}
}

func TestProbeColorImage(t *testing.T) {
	src := testimg.Photo(67, 45)
	data := encodeStd(t, rgbaOf(src), 90)
	info, err := Decoder{}.Probe(context.Background(), bytes.NewReader(data), int64(len(data)), codec.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	want := codec.Info{Format: codec.JPEG, FormatName: "JPEG", Width: 67, Height: 45, Frames: 1, Orientation: codec.OrientNormal, BitDepth: 24}
	if info != want {
		t.Fatalf("got %+v, want %+v", info, want)
	}
}

func TestProbeGrayImage(t *testing.T) {
	gray := image.NewGray(image.Rect(0, 0, 20, 10))
	data := encodeStd(t, gray, 80)
	info, err := Decoder{}.Probe(context.Background(), bytes.NewReader(data), int64(len(data)), codec.DefaultLimits())
	if err != nil || info.BitDepth != 8 || info.Width != 20 || info.Height != 10 {
		t.Fatalf("info %+v err %v", info, err)
	}
}

func TestDecodeMatchesStandardLibraryConversion(t *testing.T) {
	src := testimg.Photo(130, 77)
	data := encodeStd(t, rgbaOf(src), 85)
	reference, err := stdjpeg.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	img, err := decodeBytes(t, data)
	if err != nil {
		t.Fatal(err)
	}
	if img.Scale != 1 || len(img.Frames) != 1 || img.Loop != 0 {
		t.Fatalf("image %+v", img)
	}
	got := img.First()
	if got.Bounds() != image.Rect(0, 0, 130, 77) {
		t.Fatalf("bounds %v", got.Bounds())
	}
	for y := 0; y < 77; y++ {
		for x := 0; x < 130; x++ {
			want := color.NRGBAModel.Convert(reference.At(x, y)).(color.NRGBA)
			if have := got.NRGBAAt(x, y); have != want {
				t.Fatalf("pixel (%d,%d) = %v, want %v", x, y, have, want)
			}
		}
	}
	if d := testimg.MaxDelta(got, src); d > 40 {
		t.Fatalf("decoded image differs from the source by %d", d)
	}
	if img.Info.Width != 130 || img.Info.Height != 77 {
		t.Fatalf("info %+v", img.Info)
	}
}

func TestOrientationFromExifBothByteOrders(t *testing.T) {
	base := encodeStd(t, rgbaOf(testimg.Photo(32, 24)), 90)
	for _, bigEndian := range []bool{false, true} {
		for v := uint16(1); v <= 8; v++ {
			data := withExifOrientation(base, bigEndian, v)
			info, err := Decoder{}.Probe(context.Background(), bytes.NewReader(data), int64(len(data)), codec.DefaultLimits())
			if err != nil {
				t.Fatalf("orientation %d: %v", v, err)
			}
			if info.Orientation != codec.Orientation(v) {
				t.Fatalf("bigEndian=%v: got orientation %d, want %d", bigEndian, info.Orientation, v)
			}
			img, err := decodeBytes(t, data)
			if err != nil || img.Info.Orientation != codec.Orientation(v) {
				t.Fatalf("decode orientation %d: %v %+v", v, err, img)
			}
		}
	}
}

func TestInvalidOrientationFallsBackToNormal(t *testing.T) {
	base := encodeStd(t, rgbaOf(testimg.Photo(32, 24)), 90)
	for _, v := range []uint16{0, 9, 300} {
		data := withExifOrientation(base, false, v)
		info, err := Decoder{}.Probe(context.Background(), bytes.NewReader(data), int64(len(data)), codec.DefaultLimits())
		if err != nil || info.Orientation != codec.OrientNormal {
			t.Fatalf("value %d: %+v %v", v, info, err)
		}
	}
}

func TestEveryTruncatedPrefix(t *testing.T) {
	data := withExifOrientation(encodeStd(t, rgbaOf(testimg.Photo(24, 16)), 90), false, 6)
	for n := 0; n < len(data); n++ {
		prefix := data[:n]
		_, probeErr := Decoder{}.Probe(context.Background(), bytes.NewReader(prefix), int64(n), codec.DefaultLimits())
		_, decodeErr := decodeBytes(t, prefix)
		if decodeErr != nil {
			testimg.RequireCodecError(t, decodeErr)
		}
		if probeErr != nil {
			testimg.RequireCodecError(t, probeErr)
		}
		if n < len(data)/2 && decodeErr == nil {
			t.Fatalf("prefix of %d/%d bytes decoded without error", n, len(data))
		}
	}
}

func hugeHeader(width, height int) []byte {
	return []byte{
		0xff, 0xd8,
		0xff, 0xc0, 0x00, 0x11, 0x08,
		byte(height >> 8), byte(height), byte(width >> 8), byte(width),
		0x03, 0x01, 0x22, 0x00, 0x02, 0x11, 0x01, 0x03, 0x11, 0x01,
		0xff, 0xda, 0x00, 0x02,
		0xff, 0xd9,
	}
}

func TestHugeDimensionsAreRejectedQuickly(t *testing.T) {
	data := hugeHeader(60000, 60000)
	testimg.Within(t, 10*time.Second, func() {
		_, err := decodeBytes(t, data)
		if !errors.Is(err, codec.ErrTooLarge) {
			t.Errorf("decode: got %v", err)
		}
		_, err = Decoder{}.Probe(context.Background(), bytes.NewReader(data), int64(len(data)), codec.DefaultLimits())
		if !errors.Is(err, codec.ErrTooLarge) {
			t.Errorf("probe: got %v", err)
		}
	})
}

func TestZeroDimensionsAreCorrupt(t *testing.T) {
	data := hugeHeader(0, 10)
	_, err := Decoder{}.Probe(context.Background(), bytes.NewReader(data), int64(len(data)), codec.DefaultLimits())
	testimg.RequireCodecError(t, err)
}

func TestLimitsAreEnforced(t *testing.T) {
	data := encodeStd(t, rgbaOf(testimg.Photo(100, 100)), 90)
	lim := codec.DefaultLimits()
	lim.MaxPixels = 5000
	if _, err := (Decoder{}).Decode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Options{Limits: lim}); !errors.Is(err, codec.ErrTooLarge) {
		t.Fatalf("pixel limit: %v", err)
	}
	lim = codec.DefaultLimits()
	lim.MaxFileBytes = int64(len(data)) - 1
	if _, err := (Decoder{}).Probe(context.Background(), bytes.NewReader(data), int64(len(data)), lim); !errors.Is(err, codec.ErrTooLarge) {
		t.Fatalf("file limit: %v", err)
	}
}

func TestZeroLimitsMeanDefaults(t *testing.T) {
	data := encodeStd(t, rgbaOf(testimg.Photo(40, 30)), 90)
	if _, err := (Decoder{}).Decode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Options{}); err != nil {
		t.Fatal(err)
	}
}

func TestCancelledContext(t *testing.T) {
	data := encodeStd(t, rgbaOf(testimg.Photo(40, 30)), 90)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (Decoder{}).Decode(ctx, bytes.NewReader(data), int64(len(data)), codec.Options{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
	if _, err := (Decoder{}).Probe(ctx, bytes.NewReader(data), int64(len(data)), codec.Limits{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("probe got %v", err)
	}
}

func TestCorruptedBytesNeverPanic(t *testing.T) {
	data := encodeStd(t, rgbaOf(testimg.Photo(48, 32)), 90)
	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 400; i++ {
		mutated := append([]byte(nil), data...)
		for k := 0; k < 1+rng.Intn(4); k++ {
			mutated[rng.Intn(len(mutated))] = byte(rng.Intn(256))
		}
		if _, err := decodeBytes(t, mutated); err != nil {
			testimg.RequireCodecError(t, err)
		}
	}
}

func TestHeaderSegmentLongerThanFile(t *testing.T) {
	data := []byte{0xff, 0xd8, 0xff, 0xe1, 0xff, 0xff, 'E', 'x', 'i', 'f'}
	_, err := decodeBytes(t, data)
	testimg.RequireCodecError(t, err)
}

func TestEncoderRoundTripAndQuality(t *testing.T) {
	src := testimg.Photo(64, 48)
	encode := func(quality int) []byte {
		var buf bytes.Buffer
		if err := (Encoder{}).Encode(&buf, src, codec.EncodeOptions{Quality: quality}); err != nil {
			t.Fatal(err)
		}
		return buf.Bytes()
	}
	high, low := encode(95), encode(10)
	if len(low) >= len(high) {
		t.Fatalf("quality 10 produced %d bytes, quality 95 produced %d", len(low), len(high))
	}
	if !bytes.Equal(encode(0), encode(DefaultQuality)) {
		t.Fatal("quality 0 must select the default")
	}
	if !bytes.Equal(encode(1000), encode(MaxQuality)) {
		t.Fatal("quality above 100 must clamp to 100")
	}
	if !bytes.Equal(encode(-7), encode(MinQuality)) {
		t.Fatal("negative quality must clamp to 1")
	}
	img, err := decodeBytes(t, high)
	if err != nil {
		t.Fatal(err)
	}
	if d := testimg.MaxDelta(img.First(), src); d > 40 {
		t.Fatalf("round trip drifted by %d", d)
	}
}

func TestEncoderFlattensTransparencyOnWhite(t *testing.T) {
	src := testimg.Solid(16, 16, color.NRGBA{R: 255, G: 0, B: 0, A: 0})
	var buf bytes.Buffer
	if err := (Encoder{}).Encode(&buf, src, codec.EncodeOptions{Quality: 100}); err != nil {
		t.Fatal(err)
	}
	img, err := decodeBytes(t, buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	c := img.First().NRGBAAt(8, 8)
	if c.R < 250 || c.G < 250 || c.B < 250 {
		t.Fatalf("transparent area became %v", c)
	}
}

func TestEncoderRejectsEmptyImage(t *testing.T) {
	if err := (Encoder{}).Encode(&bytes.Buffer{}, nil, codec.EncodeOptions{}); err == nil {
		t.Fatal("nil image accepted")
	}
	if err := (Encoder{}).Encode(&bytes.Buffer{}, image.NewNRGBA(image.Rectangle{}), codec.EncodeOptions{}); err == nil {
		t.Fatal("empty image accepted")
	}
	e := Encoder{}
	if e.ID() != codec.JPEG || e.Extension() != ".jpg" || e.Name() != "JPEG" {
		t.Fatal("encoder identity")
	}
}

var (
	benchmarkData     []byte
	benchmarkDataOnce sync.Once
)

func BenchmarkDecode6000x4000(b *testing.B) {
	benchmarkDataOnce.Do(func() {
		benchmarkData = encodeStd(b, rgbaOf(testimg.Photo(6000, 4000)), 90)
	})
	b.SetBytes(int64(len(benchmarkData)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		img, err := Decoder{}.Decode(context.Background(), bytes.NewReader(benchmarkData), int64(len(benchmarkData)), codec.Options{Limits: codec.DefaultLimits()})
		if err != nil || img.First().Bounds().Dx() != 6000 {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncode6000x4000(b *testing.B) {
	src := testimg.Photo(6000, 4000)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := (Encoder{}).Encode(&bytes.Buffer{}, src, codec.EncodeOptions{Quality: 90}); err != nil {
			b.Fatal(err)
		}
	}
}

func FuzzDecode(f *testing.F) {
	base := encodeStd(f, rgbaOf(testimg.Photo(16, 12)), 80)
	f.Add(base)
	f.Add(withExifOrientation(base, true, 6))
	f.Add(hugeHeader(300, 200))
	f.Fuzz(func(t *testing.T, data []byte) {
		lim := testimg.SmallLimits()
		Decoder{}.Sniff(data)
		Decoder{}.Probe(context.Background(), bytes.NewReader(data), int64(len(data)), lim)
		img, err := Decoder{}.Decode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Options{Limits: lim})
		if err == nil && (img == nil || img.First() == nil) {
			t.Fatal("success without pixels")
		}
		if err != nil {
			testimg.RequireCodecError(t, err)
		}
	})
}
