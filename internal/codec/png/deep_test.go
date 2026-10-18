package png

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	stdpng "image/png"
	"runtime"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/testimg"
)

type adam7Pass struct{ x0, y0, dx, dy int }

var adam7 = []adam7Pass{{0, 0, 8, 8}, {4, 0, 8, 8}, {0, 4, 4, 8}, {2, 0, 4, 4}, {0, 2, 2, 4}, {1, 0, 2, 2}, {0, 1, 1, 2}}

func pngChunk(kind string, body []byte) []byte {
	out := binary.BigEndian.AppendUint32(nil, uint32(len(body)))
	start := len(out)
	out = append(out, kind...)
	out = append(out, body...)
	return binary.BigEndian.AppendUint32(out, crc32.ChecksumIEEE(out[start:]))
}

func zlibBytes(data []byte) []byte {
	var buf bytes.Buffer
	zw, _ := zlib.NewWriterLevel(&buf, zlib.BestSpeed)
	zw.Write(data)
	zw.Close()
	return buf.Bytes()
}

type sample16 func(x, y, channel int) uint16

func channelsOf(colorType byte) int {
	switch colorType {
	case 0:
		return 1
	case 2:
		return 3
	case 4:
		return 2
	}
	return 4
}

func rawRows(w, h int, colorType byte, interlace bool, at sample16) []byte {
	channels := channelsOf(colorType)
	passes := []adam7Pass{{0, 0, 1, 1}}
	if interlace {
		passes = adam7
	}
	var raw []byte
	for _, p := range passes {
		for y := p.y0; y < h; y += p.dy {
			if p.x0 >= w {
				continue
			}
			raw = append(raw, 0)
			for x := p.x0; x < w; x += p.dx {
				for c := 0; c < channels; c++ {
					raw = binary.BigEndian.AppendUint16(raw, at(x, y, c))
				}
			}
		}
	}
	return raw
}

func build16(w, h int, colorType byte, interlace bool, at sample16, extra ...[]byte) []byte {
	out := append([]byte(nil), signature...)
	header := binary.BigEndian.AppendUint32(nil, uint32(w))
	header = binary.BigEndian.AppendUint32(header, uint32(h))
	flag := byte(0)
	if interlace {
		flag = 1
	}
	header = append(header, 16, colorType, 0, 0, flag)
	out = append(out, pngChunk("IHDR", header)...)
	for _, e := range extra {
		out = append(out, e...)
	}
	out = append(out, pngChunk("IDAT", zlibBytes(rawRows(w, h, colorType, interlace, at)))...)
	return append(out, pngChunk("IEND", nil)...)
}

func scatter(x, y, c int) uint16 {
	return uint16((x*2371+y*4099+c*17317+x*y*13)%65536) | uint16(c&1)
}

func deepOptions() codec.Options {
	return codec.Options{Limits: codec.DefaultLimits(), Deep: true}
}

func decodeBytes(t testing.TB, data []byte, o codec.Options) *codec.Image {
	t.Helper()
	img, err := Decoder{}.Decode(context.Background(), bytes.NewReader(data), int64(len(data)), o)
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func word(pix []byte, offset int) uint16 { return binary.BigEndian.Uint16(pix[offset:]) }

func expectPixel(colorType byte, at sample16, x, y int) [4]uint16 {
	switch colorType {
	case 0:
		v := at(x, y, 0)
		return [4]uint16{v, v, v, 0xffff}
	case 2:
		return [4]uint16{at(x, y, 0), at(x, y, 1), at(x, y, 2), 0xffff}
	case 4:
		v := at(x, y, 0)
		return [4]uint16{v, v, v, at(x, y, 1)}
	}
	return [4]uint16{at(x, y, 0), at(x, y, 1), at(x, y, 2), at(x, y, 3)}
}

func TestDeepReturnsEverySixteenBitSampleExactly(t *testing.T) {
	for _, colorType := range []byte{0, 2, 4, 6} {
		for _, interlace := range []bool{false, true} {
			for _, size := range [][2]int{{1, 1}, {7, 5}, {33, 21}, {64, 3}} {
				w, h := size[0], size[1]
				data := build16(w, h, colorType, interlace, scatter)
				img := decodeBytes(t, data, deepOptions())
				frame := img.Frames[0]
				if frame.Deep == nil || frame.Pix != nil {
					t.Fatalf("type %d interlace %v %dx%d: deep %v pix %v", colorType, interlace, w, h, frame.Deep != nil, frame.Pix != nil)
				}
				if frame.Deep.Rect != image.Rect(0, 0, w, h) {
					t.Fatalf("bounds %v", frame.Deep.Rect)
				}
				for y := 0; y < h; y++ {
					for x := 0; x < w; x++ {
						want := expectPixel(colorType, scatter, x, y)
						o := frame.Deep.PixOffset(x, y)
						got := [4]uint16{word(frame.Deep.Pix, o), word(frame.Deep.Pix, o+2), word(frame.Deep.Pix, o+4), word(frame.Deep.Pix, o+6)}
						if got != want {
							t.Fatalf("type %d interlace %v %dx%d at %d,%d: %v, want %v", colorType, interlace, w, h, x, y, got, want)
						}
					}
				}
			}
		}
	}
}

func TestWithoutDeepSixteenBitPNGIsUnchanged(t *testing.T) {
	for _, colorType := range []byte{0, 2, 4, 6} {
		data := build16(19, 11, colorType, false, scatter)
		img := decodeBytes(t, data, codec.Options{Limits: codec.DefaultLimits()})
		frame := img.Frames[0]
		if frame.Deep != nil || frame.Pix == nil {
			t.Fatalf("type %d: deep %v pix %v", colorType, frame.Deep != nil, frame.Pix != nil)
		}
		want := expectPixel(colorType, scatter, 5, 3)
		got := frame.Pix.NRGBAAt(5, 3)
		if uint16(got.R) != want[0]>>8 || uint16(got.G) != want[1]>>8 || uint16(got.B) != want[2]>>8 || uint16(got.A) != want[3]>>8 {
			t.Fatalf("type %d: %v, want %v", colorType, got, want)
		}
	}
}

func TestEightBitPNGKeepsPixEvenWithDeep(t *testing.T) {
	var buf bytes.Buffer
	if err := stdpng.Encode(&buf, testimg.Photo(24, 16)); err != nil {
		t.Fatal(err)
	}
	img := decodeBytes(t, buf.Bytes(), deepOptions())
	if img.Frames[0].Deep != nil || img.Frames[0].Pix == nil {
		t.Fatal("8 bit picture became deep")
	}
}

func TestStandardEncoderSixteenBitRoundTrip(t *testing.T) {
	src := image.NewNRGBA64(image.Rect(0, 0, 30, 20))
	for y := 0; y < 20; y++ {
		for x := 0; x < 30; x++ {
			src.SetNRGBA64(x, y, color.NRGBA64{R: scatter(x, y, 0), G: scatter(x, y, 1), B: scatter(x, y, 2), A: scatter(x, y, 3) | 1})
		}
	}
	var buf bytes.Buffer
	if err := stdpng.Encode(&buf, src); err != nil {
		t.Fatal(err)
	}
	img := decodeBytes(t, buf.Bytes(), deepOptions())
	if !bytes.Equal(img.Frames[0].Deep.Pix, src.Pix) {
		t.Fatal("pixels differ")
	}
	gray := image.NewGray16(image.Rect(0, 0, 17, 9))
	for i := range gray.Pix {
		gray.Pix[i] = byte(i * 37)
	}
	buf.Reset()
	if err := stdpng.Encode(&buf, gray); err != nil {
		t.Fatal(err)
	}
	deep := decodeBytes(t, buf.Bytes(), deepOptions()).Frames[0].Deep
	for y := 0; y < 9; y++ {
		for x := 0; x < 17; x++ {
			v := binary.BigEndian.Uint16(gray.Pix[gray.PixOffset(x, y):])
			o := deep.PixOffset(x, y)
			if word(deep.Pix, o) != v || word(deep.Pix, o+2) != v || word(deep.Pix, o+4) != v || word(deep.Pix, o+6) != 0xffff {
				t.Fatalf("gray16 at %d,%d", x, y)
			}
		}
	}
}

func TestSixteenBitTransparencyKeyBecomesAlpha(t *testing.T) {
	at := func(x, y, c int) uint16 {
		if x == 2 && y == 1 {
			return uint16(0x1234 + c*0x1111)
		}
		return scatter(x, y, c)
	}
	key := []byte{0x12, 0x34, 0x23, 0x45, 0x34, 0x56}
	data := build16(5, 4, 2, false, at, pngChunk("tRNS", key))
	frame := decodeBytes(t, data, deepOptions()).Frames[0]
	if frame.Deep == nil {
		t.Fatal("not deep")
	}
	o := frame.Deep.PixOffset(2, 1)
	if word(frame.Deep.Pix, o+6) != 0 {
		t.Fatalf("keyed pixel alpha %#x", word(frame.Deep.Pix, o+6))
	}
	o = frame.Deep.PixOffset(3, 1)
	if word(frame.Deep.Pix, o+6) != 0xffff || word(frame.Deep.Pix, o) != scatter(3, 1, 0) {
		t.Fatal("other pixel changed")
	}
}

func TestDeepCarriesColorAndLeavesSamplesUnconverted(t *testing.T) {
	profile := testimg.SampleICCSpace("RGB ", 30)
	extra := [][]byte{pngChunk("cICP", []byte{12, 13, 0, 1}), pngChunk("iCCP", append(append([]byte("p"), 0, 0), zlibBytes(profile)...))}
	data := build16(9, 6, 6, false, scatter, extra...)
	img := decodeBytes(t, data, deepOptions())
	if !bytes.Equal(img.Color.ICC, profile) {
		t.Fatalf("icc %d bytes", len(img.Color.ICC))
	}
	if img.Color.CICP == nil || *img.Color.CICP != (codec.CICP{Primaries: 12, Transfer: 13, FullRange: true}) {
		t.Fatalf("cicp %+v", img.Color.CICP)
	}
	plain := decodeBytes(t, data, codec.Options{Limits: codec.DefaultLimits()})
	if !bytes.Equal(plain.Color.ICC, profile) || plain.Color.CICP == nil {
		t.Fatal("colour is reported without Deep too")
	}
	want := expectPixel(6, scatter, 4, 2)
	o := img.Frames[0].Deep.PixOffset(4, 2)
	if word(img.Frames[0].Deep.Pix, o) != want[0] {
		t.Fatal("samples were converted")
	}
}

func TestEightBitHDRPictureBecomesDeepOnlyWhenAsked(t *testing.T) {
	var buf bytes.Buffer
	src := testimg.Photo(20, 12)
	if err := stdpng.Encode(&buf, src); err != nil {
		t.Fatal(err)
	}
	raw := buf.Bytes()
	ihdrEnd := 8 + 8 + 13 + 4
	data := append(append(append([]byte(nil), raw[:ihdrEnd]...), pngChunk("cICP", []byte{9, 16, 0, 1})...), raw[ihdrEnd:]...)
	img := decodeBytes(t, data, deepOptions())
	frame := img.Frames[0]
	if frame.Deep == nil || frame.Pix != nil {
		t.Fatal("PQ picture is not deep")
	}
	px := src.NRGBAAt(3, 4)
	o := frame.Deep.PixOffset(3, 4)
	if word(frame.Deep.Pix, o) != uint16(px.R)*257 || word(frame.Deep.Pix, o+6) != 0xffff {
		t.Fatal("8 bit samples are not widened exactly")
	}
	plain := decodeBytes(t, data, codec.Options{Limits: codec.DefaultLimits()})
	if plain.Frames[0].Pix == nil || plain.Frames[0].Deep != nil {
		t.Fatal("without Deep the picture must stay 8 bit")
	}
}

func TestDeepDecodeAllocatesAboutTheSixteenBitPicture(t *testing.T) {
	if testing.Short() {
		t.Skip("allocates a large picture")
	}
	const w, h = 6000, 4000
	src := image.NewNRGBA64(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		row := src.Pix[y*src.Stride : (y+1)*src.Stride]
		for x := 0; x < w; x++ {
			v := uint16(x*11 + y*7)
			binary.BigEndian.PutUint16(row[x*8:], v)
			binary.BigEndian.PutUint16(row[x*8+2:], v>>1)
			binary.BigEndian.PutUint16(row[x*8+4:], ^v)
			binary.BigEndian.PutUint16(row[x*8+6:], 0xffff)
		}
	}
	var buf bytes.Buffer
	enc := stdpng.Encoder{CompressionLevel: stdpng.BestSpeed}
	if err := enc.Encode(&buf, src); err != nil {
		t.Fatal(err)
	}
	src = nil
	data := buf.Bytes()
	lim := codec.DefaultLimits()
	lim.MaxPixels = 100_000_000
	measure := func(deep bool) uint64 {
		runtime.GC()
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		img, err := Decoder{}.Decode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Options{Limits: lim, Deep: deep})
		if err != nil {
			t.Fatal(err)
		}
		runtime.ReadMemStats(&after)
		runtime.KeepAlive(img)
		return after.TotalAlloc - before.TotalAlloc
	}
	picture := uint64(w) * h * 8
	deep := measure(true)
	if deep > picture+picture/4 {
		t.Fatalf("deep decode allocated %d MiB for a %d MiB picture", deep>>20, picture>>20)
	}
	t.Logf("deep decode allocated %d MiB for a %d MiB 16-bit picture", deep>>20, picture>>20)
}
