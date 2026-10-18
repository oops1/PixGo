package png

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	stdpng "image/png"
	"math/rand"
	"testing"
	"time"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/testimg"
)

func encodeStd(t testing.TB, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := stdpng.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func chunk(kind string, data []byte) []byte {
	out := binary.BigEndian.AppendUint32(nil, uint32(len(data)))
	out = append(out, kind...)
	out = append(out, data...)
	checksum := crc32.NewIEEE()
	checksum.Write([]byte(kind))
	checksum.Write(data)
	return binary.BigEndian.AppendUint32(out, checksum.Sum32())
}

const afterHeader = 8 + 25

func insertAfterHeader(data []byte, chunks ...[]byte) []byte {
	out := append([]byte(nil), data[:afterHeader]...)
	for _, c := range chunks {
		out = append(out, c...)
	}
	return append(out, data[afterHeader:]...)
}

func insertBeforeEnd(data []byte, chunks ...[]byte) []byte {
	end := len(data) - 12
	out := append([]byte(nil), data[:end]...)
	for _, c := range chunks {
		out = append(out, c...)
	}
	return append(out, data[end:]...)
}

func actl(frames, plays uint32) []byte {
	data := binary.BigEndian.AppendUint32(nil, frames)
	return chunk("acTL", binary.BigEndian.AppendUint32(data, plays))
}

func probe(t testing.TB, data []byte) (codec.Info, error) {
	t.Helper()
	return Decoder{}.Probe(context.Background(), bytes.NewReader(data), int64(len(data)), codec.DefaultLimits())
}

func decode(data []byte) (*codec.Image, error) {
	return Decoder{}.Decode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Options{Limits: codec.DefaultLimits()})
}

func TestSniffAndExtensions(t *testing.T) {
	d := Decoder{}
	if !d.Sniff([]byte("\x89PNG\r\n\x1a\nxxxx")) || !d.Sniff(signature) {
		t.Fatal("signature not recognised")
	}
	for _, head := range [][]byte{nil, []byte("\x89PNG"), []byte("\x89PNG\r\n\x1a\x0b"), []byte("GIF89a"), {0xff, 0xd8, 0xff}} {
		if d.Sniff(head) {
			t.Fatalf("%q sniffed as PNG", head)
		}
	}
	if d.ID() != codec.PNG || d.Name() != "PNG" || len(d.Extensions()) != 1 || d.Extensions()[0] != ".png" {
		t.Fatal("identity")
	}
}

func TestLosslessRoundTrip(t *testing.T) {
	for _, withAlpha := range []bool{false, true} {
		src := testimg.Noise(53, 31, 7, withAlpha)
		var buf bytes.Buffer
		if err := (Encoder{}).Encode(&buf, src, codec.EncodeOptions{}); err != nil {
			t.Fatal(err)
		}
		img, err := decode(buf.Bytes())
		if err != nil {
			t.Fatal(err)
		}
		if !testimg.Identical(img.First(), src) {
			t.Fatalf("alpha=%v: pixels differ after round trip", withAlpha)
		}
		info, err := probe(t, buf.Bytes())
		if err != nil || info.HasAlpha != withAlpha || info.Width != 53 || info.Height != 31 {
			t.Fatalf("alpha=%v: %+v %v", withAlpha, info, err)
		}
		wantDepth := 24
		if withAlpha {
			wantDepth = 32
		}
		if info.BitDepth != wantDepth || info.Frames != 1 || info.Animated || info.Orientation != codec.OrientNormal {
			t.Fatalf("alpha=%v: %+v", withAlpha, info)
		}
	}
}

func TestOtherColourTypes(t *testing.T) {
	gray := image.NewGray(image.Rect(0, 0, 9, 7))
	gray16 := image.NewGray16(image.Rect(0, 0, 9, 7))
	rgba64 := image.NewNRGBA64(image.Rect(0, 0, 9, 7))
	rng := rand.New(rand.NewSource(3))
	rng.Read(gray.Pix)
	rng.Read(gray16.Pix)
	rng.Read(rgba64.Pix)
	palette := color.Palette{color.NRGBA{255, 0, 0, 255}, color.NRGBA{0, 255, 0, 128}, color.NRGBA{0, 0, 0, 0}}
	paletted := image.NewPaletted(image.Rect(0, 0, 9, 7), palette)
	for i := range paletted.Pix {
		paletted.Pix[i] = uint8(i % 3)
	}
	cases := []struct {
		name  string
		img   image.Image
		depth int
		alpha bool
	}{
		{"gray", gray, 8, false},
		{"gray16", gray16, 16, false},
		{"nrgba64", rgba64, 64, true},
		{"paletted", paletted, 2, true},
	}
	for _, c := range cases {
		data := encodeStd(t, c.img)
		info, err := probe(t, data)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if info.BitDepth != c.depth || info.HasAlpha != c.alpha {
			t.Fatalf("%s: %+v", c.name, info)
		}
		img, err := decode(data)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		reference, err := stdpng.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		bounds := reference.Bounds()
		for y := 0; y < bounds.Dy(); y++ {
			for x := 0; x < bounds.Dx(); x++ {
				want := color.NRGBAModel.Convert(reference.At(x, y)).(color.NRGBA)
				if c.name == "nrgba64" {
					p := c.img.(*image.NRGBA64).NRGBA64At(x, y)
					want = color.NRGBA{R: uint8(p.R >> 8), G: uint8(p.G >> 8), B: uint8(p.B >> 8), A: uint8(p.A >> 8)}
				}
				if have := img.First().NRGBAAt(x, y); have != want {
					t.Fatalf("%s: pixel (%d,%d) = %v, want %v", c.name, x, y, have, want)
				}
			}
		}
	}
}

func TestAPNGDetection(t *testing.T) {
	base := encodeStd(t, testimg.Noise(10, 8, 1, true))
	data := insertAfterHeader(base, actl(5, 0))
	data = insertBeforeEnd(data, chunk("fcTL", make([]byte, 26)), chunk("fdAT", make([]byte, 20)))
	info, err := probe(t, data)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Animated || info.Frames != 5 || info.Format != codec.PNG || info.FormatName != "PNG" {
		t.Fatalf("%+v", info)
	}
	img, err := decode(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(img.Frames) != 1 || !testimg.Identical(img.First(), testimg.Noise(10, 8, 1, true)) {
		t.Fatalf("default image not decoded: %d frames", len(img.Frames))
	}
	if !img.Info.Animated || img.Info.Frames != 5 {
		t.Fatalf("decode info %+v", img.Info)
	}
}

func TestAPNGWithSingleFrameIsStatic(t *testing.T) {
	info, err := probe(t, insertAfterHeader(encodeStd(t, testimg.Noise(4, 4, 1, false)), actl(1, 0)))
	if err != nil || info.Animated || info.Frames != 1 {
		t.Fatalf("%+v %v", info, err)
	}
}

func TestAnimationControlAfterImageDataIsIgnored(t *testing.T) {
	data := insertBeforeEnd(encodeStd(t, testimg.Noise(4, 4, 1, false)), actl(3, 0))
	info, err := probe(t, data)
	if err != nil || info.Animated || info.Frames != 1 {
		t.Fatalf("%+v %v", info, err)
	}
}

func TestAnimationControlWithBadLengthOrZeroFrames(t *testing.T) {
	base := encodeStd(t, testimg.Noise(4, 4, 1, false))
	for name, c := range map[string][]byte{
		"zero frames": actl(0, 0),
		"short":       chunk("acTL", []byte{0, 0, 0, 3}),
	} {
		info, err := probe(t, insertAfterHeader(base, c))
		if err != nil || info.Animated || info.Frames != 1 {
			t.Fatalf("%s: %+v %v", name, info, err)
		}
	}
}

func TestOrientationFromExifChunk(t *testing.T) {
	base := encodeStd(t, testimg.Noise(6, 6, 2, false))
	for _, bigEndian := range []bool{false, true} {
		for v := uint16(1); v <= 8; v++ {
			exifChunk := chunk("eXIf", testimg.OrientationTIFF(bigEndian, v))
			for name, data := range map[string][]byte{
				"before data": insertAfterHeader(base, exifChunk),
				"after data":  insertBeforeEnd(base, exifChunk),
			} {
				info, err := probe(t, data)
				if err != nil || info.Orientation != codec.Orientation(v) {
					t.Fatalf("%s v=%d: %+v %v", name, v, info, err)
				}
				img, err := decode(data)
				if err != nil || img.Info.Orientation != codec.Orientation(v) {
					t.Fatalf("%s v=%d decode: %v", name, v, err)
				}
			}
		}
	}
}

func TestExifChunkWithBadLengthIsIgnored(t *testing.T) {
	base := encodeStd(t, testimg.Noise(6, 6, 2, false))
	bad := chunk("eXIf", testimg.OrientationTIFF(false, 6))
	binary.BigEndian.PutUint32(bad[0:4], 0x7fffffff)
	data := insertAfterHeader(base, bad)
	info, err := probe(t, data)
	if err != nil || info.Orientation != codec.OrientNormal {
		t.Fatalf("%+v %v", info, err)
	}
	if _, err := decode(data); err == nil {
		t.Fatal("corrupt chunk must fail the full decode")
	}
}

func TestTransparencyChunkMeansAlpha(t *testing.T) {
	rgb := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for i := 3; i < len(rgb.Pix); i += 4 {
		rgb.Pix[i] = 255
	}
	base := encodeStd(t, rgb)
	info, err := probe(t, base)
	if err != nil || info.HasAlpha {
		t.Fatalf("%+v %v", info, err)
	}
	info, err = probe(t, insertAfterHeader(base, chunk("tRNS", []byte{0, 1, 0, 2, 0, 3})))
	if err != nil || !info.HasAlpha {
		t.Fatalf("%+v %v", info, err)
	}
}

func headerOnly(width, height uint32, depth, colourType byte) []byte {
	ihdr := binary.BigEndian.AppendUint32(nil, width)
	ihdr = binary.BigEndian.AppendUint32(ihdr, height)
	ihdr = append(ihdr, depth, colourType, 0, 0, 0)
	out := append([]byte(nil), signature...)
	out = append(out, chunk("IHDR", ihdr)...)
	return append(out, chunk("IEND", nil)...)
}

func TestHugeDimensionsRejectedQuickly(t *testing.T) {
	data := headerOnly(1<<30, 1<<30, 8, 6)
	testimg.Within(t, 10*time.Second, func() {
		if _, err := probe(t, data); !errors.Is(err, codec.ErrTooLarge) {
			t.Errorf("probe: %v", err)
		}
		if _, err := decode(data); !errors.Is(err, codec.ErrTooLarge) {
			t.Errorf("decode: %v", err)
		}
	})
	if _, err := probe(t, headerOnly(0xffffffff, 1, 8, 6)); !errors.Is(err, codec.ErrCorrupt) {
		t.Fatalf("beyond int32: %v", err)
	}
}

func TestInvalidHeadersAreCorrupt(t *testing.T) {
	cases := map[string][]byte{
		"zero width":    headerOnly(0, 5, 8, 6),
		"zero height":   headerOnly(5, 0, 8, 6),
		"colour type 1": headerOnly(5, 5, 8, 1),
		"depth 3":       headerOnly(5, 5, 3, 0),
		"depth 4 rgb":   headerOnly(5, 5, 4, 2),
		"depth 16 pal":  headerOnly(5, 5, 16, 3),
		"empty":         nil,
		"bad signature": append([]byte("\x89PNG\r\n\x1a\x0b"), headerOnly(5, 5, 8, 6)[8:]...),
	}
	for name, data := range cases {
		_, err := probe(t, data)
		if !errors.Is(err, codec.ErrCorrupt) {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

func TestIHDRMustComeFirstWithRightLength(t *testing.T) {
	good := headerOnly(5, 5, 8, 6)
	notFirst := append([]byte(nil), signature...)
	notFirst = append(notFirst, chunk("tEXt", []byte("a\x00b"))...)
	notFirst = append(notFirst, good[8:]...)
	if _, err := probe(t, notFirst); !errors.Is(err, codec.ErrCorrupt) {
		t.Fatalf("IHDR not first: %v", err)
	}
	wrongLength := append([]byte(nil), good...)
	binary.BigEndian.PutUint32(wrongLength[8:12], 14)
	if _, err := probe(t, wrongLength); !errors.Is(err, codec.ErrCorrupt) {
		t.Fatalf("IHDR length: %v", err)
	}
}

func TestEveryTruncatedPrefix(t *testing.T) {
	base := encodeStd(t, testimg.Noise(12, 9, 4, true))
	data := insertAfterHeader(base, actl(2, 0), chunk("eXIf", testimg.OrientationTIFF(false, 3)))
	for n := 0; n < len(data); n++ {
		prefix := data[:n]
		if _, err := probe(t, prefix); err != nil {
			testimg.RequireCodecError(t, err)
		}
		if _, err := decode(prefix); err == nil {
			t.Fatalf("prefix of %d/%d bytes decoded without error", n, len(data))
		} else {
			testimg.RequireCodecError(t, err)
		}
	}
	if _, err := decode(data); err != nil {
		t.Fatal(err)
	}
}

func TestChunkLengthsLargerThanFile(t *testing.T) {
	base := encodeStd(t, testimg.Noise(6, 6, 2, false))
	data := append([]byte(nil), base...)
	binary.BigEndian.PutUint32(data[afterHeader:afterHeader+4], 0xfffffff0)
	if _, err := decode(data); err == nil {
		t.Fatal("oversized chunk accepted")
	} else {
		testimg.RequireCodecError(t, err)
	}
	if _, err := probe(t, data); err != nil {
		testimg.RequireCodecError(t, err)
	}
}

func TestManyTinyChunksFinishQuickly(t *testing.T) {
	base := encodeStd(t, testimg.Noise(4, 4, 1, false))
	var many []byte
	empty := chunk("tEXt", nil)
	for i := 0; i < 400_000; i++ {
		many = append(many, empty...)
	}
	data := insertAfterHeader(base, many)
	testimg.Within(t, 10*time.Second, func() {
		if _, err := probe(t, data); err != nil {
			t.Errorf("probe: %v", err)
		}
	})
}

func TestLimitsAndCancellation(t *testing.T) {
	data := encodeStd(t, testimg.Noise(100, 100, 1, false))
	lim := codec.DefaultLimits()
	lim.MaxPixels = 9999
	if _, err := (Decoder{}).Decode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Options{Limits: lim}); !errors.Is(err, codec.ErrTooLarge) {
		t.Fatalf("pixels: %v", err)
	}
	lim = codec.DefaultLimits()
	lim.MaxFileBytes = 10
	if _, err := (Decoder{}).Probe(context.Background(), bytes.NewReader(data), int64(len(data)), lim); !errors.Is(err, codec.ErrTooLarge) {
		t.Fatalf("file size: %v", err)
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
	data := encodeStd(t, testimg.Noise(24, 16, 5, true))
	rng := rand.New(rand.NewSource(2))
	for i := 0; i < 400; i++ {
		mutated := append([]byte(nil), data...)
		for k := 0; k < 1+rng.Intn(4); k++ {
			mutated[rng.Intn(len(mutated))] = byte(rng.Intn(256))
		}
		if _, err := decode(mutated); err != nil {
			testimg.RequireCodecError(t, err)
		}
	}
}

func TestEncoderRejectsEmptyImage(t *testing.T) {
	if err := (Encoder{}).Encode(&bytes.Buffer{}, nil, codec.EncodeOptions{}); err == nil {
		t.Fatal("nil accepted")
	}
	if err := (Encoder{}).Encode(&bytes.Buffer{}, image.NewNRGBA(image.Rectangle{}), codec.EncodeOptions{}); err == nil {
		t.Fatal("empty accepted")
	}
	e := Encoder{}
	if e.ID() != codec.PNG || e.Name() != "PNG" || e.Extension() != ".png" {
		t.Fatal("identity")
	}
}

func FuzzDecode(f *testing.F) {
	base := encodeStd(f, testimg.Noise(5, 4, 9, true))
	f.Add(base)
	f.Add(insertAfterHeader(base, actl(2, 1), chunk("eXIf", testimg.OrientationTIFF(true, 6))))
	f.Add(headerOnly(7, 7, 8, 2))
	f.Add(insertBeforeEnd(base, chunk("eXIf", testimg.OrientationTIFF(false, 8))))
	f.Fuzz(func(t *testing.T, data []byte) {
		lim := testimg.SmallLimits()
		ctx := context.Background()
		Decoder{}.Sniff(data)
		Decoder{}.Probe(ctx, bytes.NewReader(data), int64(len(data)), lim)
		img, err := Decoder{}.Decode(ctx, bytes.NewReader(data), int64(len(data)), codec.Options{Limits: lim, Deep: len(data)&1 == 1})
		if err == nil && (img == nil || len(img.Frames) == 0 || img.Frames[0].Pix == nil && img.Frames[0].Deep == nil) {
			t.Fatal("success without pixels")
		}
		if err != nil {
			testimg.RequireCodecError(t, err)
		}
	})
}
