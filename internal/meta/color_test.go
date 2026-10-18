package meta

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/binary"
	"hash/crc32"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
)

func colorProfile(space string, padding int) []byte {
	p := make([]byte, 128+4+12+padding)
	binary.BigEndian.PutUint32(p[0:], uint32(len(p)))
	p[8], p[9] = 4, 0x30
	copy(p[12:], "mntr")
	copy(p[16:], space)
	copy(p[20:], "XYZ ")
	copy(p[36:], "acsp")
	binary.BigEndian.PutUint32(p[128:], 1)
	copy(p[132:], "wtpt")
	for i := 0; i < padding; i++ {
		p[144+i] = byte(i*7 + 3)
	}
	return p
}

func colorChunk(kind string, body []byte) []byte {
	out := binary.BigEndian.AppendUint32(nil, uint32(len(body)))
	start := len(out)
	out = append(out, kind...)
	out = append(out, body...)
	return binary.BigEndian.AppendUint32(out, crc32.ChecksumIEEE(out[start:]))
}

func colorDeflate(data []byte) []byte {
	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	zw.Write(data)
	zw.Close()
	return buf.Bytes()
}

func colorPNG(chunks ...[]byte) []byte {
	out := append([]byte(nil), pngSignature...)
	header := binary.BigEndian.AppendUint32(nil, 4)
	header = binary.BigEndian.AppendUint32(header, 4)
	header = append(header, 16, 2, 0, 0, 0)
	out = append(out, colorChunk("IHDR", header)...)
	for _, c := range chunks {
		out = append(out, c...)
	}
	out = append(out, colorChunk("IDAT", colorDeflate(make([]byte, 100)))...)
	return append(out, colorChunk("IEND", nil)...)
}

func iccChunkBody(profile []byte) []byte {
	return append(append([]byte("sRGB"), 0, 0), colorDeflate(profile)...)
}

func readColor(t *testing.T, data []byte, id codec.ID) codec.ColorInfo {
	t.Helper()
	return ReadColor(context.Background(), bytes.NewReader(data), int64(len(data)), id)
}

func TestColorFromPNGChunks(t *testing.T) {
	profile := colorProfile("RGB ", 40)
	data := colorPNG(colorChunk("cICP", []byte{9, 16, 0, 1}), colorChunk("iCCP", iccChunkBody(profile)))
	got := readColor(t, data, codec.PNG)
	if !bytes.Equal(got.ICC, profile) {
		t.Fatalf("profile %d bytes, want %d", len(got.ICC), len(profile))
	}
	if got.CICP == nil || *got.CICP != (codec.CICP{Primaries: 9, Transfer: 16, Matrix: 0, FullRange: true}) {
		t.Fatalf("cicp %+v", got.CICP)
	}
	if !got.HDR() {
		t.Fatal("PQ is not reported as HDR")
	}
	if apng := readColor(t, data, codec.APNG); !bytes.Equal(apng.ICC, profile) || apng.CICP == nil {
		t.Fatal("APNG reads the same chunks")
	}
}

func TestColorFromPNGWithoutChunksIsEmpty(t *testing.T) {
	if got := readColor(t, colorPNG(), codec.PNG); !got.Empty() {
		t.Fatalf("%+v", got)
	}
}

func TestColorChunksAfterPixelDataAreIgnored(t *testing.T) {
	out := colorPNG()
	end := len(out) - 12
	late := append(append([]byte(nil), out[:end]...), colorChunk("cICP", []byte{1, 13, 0, 1})...)
	late = append(late, colorChunk("iCCP", iccChunkBody(colorProfile("RGB ", 8)))...)
	late = append(late, out[end:]...)
	if got := readColor(t, late, codec.PNG); !got.Empty() {
		t.Fatalf("%+v", got)
	}
}

func TestMalformedPNGColorChunksAreIgnored(t *testing.T) {
	cases := map[string][]byte{
		"short cICP":          colorPNG(colorChunk("cICP", []byte{1, 13, 0})),
		"long cICP":           colorPNG(colorChunk("cICP", []byte{1, 13, 0, 1, 0})),
		"profile not zlib":    colorPNG(colorChunk("iCCP", append([]byte("x\x00\x00"), 1, 2, 3, 4))),
		"profile name only":   colorPNG(colorChunk("iCCP", []byte("sRGB"))),
		"profile too short":   colorPNG(colorChunk("iCCP", iccChunkBody([]byte("acsp")))),
		"profile wrong magic": colorPNG(colorChunk("iCCP", iccChunkBody(make([]byte, 200)))),
	}
	for name, data := range cases {
		if got := readColor(t, data, codec.PNG); !got.Empty() {
			t.Fatalf("%s: %+v", name, got)
		}
	}
}

func TestProfilesThatCannotDescribeRGBOutputAreDropped(t *testing.T) {
	for _, space := range []string{"CMYK", "Lab ", "XYZ ", "YCbr", "HSV "} {
		data := colorPNG(colorChunk("iCCP", iccChunkBody(colorProfile(space, 0))))
		if got := readColor(t, data, codec.PNG); got.ICC != nil {
			t.Fatalf("%s profile was kept", space)
		}
	}
	for _, space := range []string{"RGB ", "GRAY"} {
		data := colorPNG(colorChunk("iCCP", iccChunkBody(colorProfile(space, 0))))
		if got := readColor(t, data, codec.PNG); got.ICC == nil {
			t.Fatalf("%s profile was dropped", space)
		}
	}
}

func colorJPEG(segments ...[]byte) []byte {
	out := []byte{0xff, 0xd8}
	for _, s := range segments {
		out = append(out, s...)
	}
	return append(out, 0xff, 0xda, 0, 2, 0xff, 0xd9)
}

func iccSegment(seq, count int, payload []byte) []byte {
	body := append([]byte("ICC_PROFILE\x00"), byte(seq), byte(count))
	body = append(body, payload...)
	out := []byte{0xff, 0xe2, byte((len(body) + 2) >> 8), byte(len(body) + 2)}
	return append(out, body...)
}

func TestColorFromJPEGReassemblesChunksInSequenceOrder(t *testing.T) {
	profile := colorProfile("RGB ", 300)
	a, b, c := profile[:100], profile[100:250], profile[250:]
	data := colorJPEG(iccSegment(2, 3, b), iccSegment(1, 3, a), iccSegment(3, 3, c))
	got := readColor(t, data, codec.JPEG)
	if !bytes.Equal(got.ICC, profile) || got.CICP != nil {
		t.Fatalf("%d bytes, cicp %+v", len(got.ICC), got.CICP)
	}
}

func TestColorFromJPEGWithMissingOrDuplicatedChunksIsEmpty(t *testing.T) {
	profile := colorProfile("RGB ", 300)
	a, b := profile[:100], profile[100:]
	cases := map[string][]byte{
		"missing second": colorJPEG(iccSegment(1, 2, a)),
		"duplicate":      colorJPEG(iccSegment(1, 2, a), iccSegment(1, 2, b)),
		"count mismatch": colorJPEG(iccSegment(1, 2, a), iccSegment(2, 3, b)),
		"zero sequence":  colorJPEG(iccSegment(0, 1, profile)),
		"cmyk":           colorJPEG(iccSegment(1, 1, colorProfile("CMYK", 20))),
	}
	for name, data := range cases {
		if got := readColor(t, data, codec.JPEG); !got.Empty() {
			t.Fatalf("%s: %d bytes", name, len(got.ICC))
		}
	}
	if got := readColor(t, colorJPEG(iccSegment(1, 1, profile)), codec.JPEG); !bytes.Equal(got.ICC, profile) {
		t.Fatal("single chunk")
	}
}

func colorWebP(chunks ...[]byte) []byte {
	var body []byte
	for _, c := range chunks {
		body = append(body, c...)
	}
	out := append([]byte("RIFF"), 0, 0, 0, 0)
	out = append(out, "WEBP"...)
	out = append(out, body...)
	binary.LittleEndian.PutUint32(out[4:], uint32(len(out)-8))
	return out
}

func webpChunk(kind string, body []byte) []byte {
	out := append([]byte(kind), 0, 0, 0, 0)
	binary.LittleEndian.PutUint32(out[4:], uint32(len(body)))
	out = append(out, body...)
	if len(body)&1 == 1 {
		out = append(out, 0)
	}
	return out
}

func TestColorFromWebP(t *testing.T) {
	profile := colorProfile("RGB ", 41)
	vp8x := webpChunk("VP8X", []byte{0x20, 0, 0, 0, 1, 0, 0, 1, 0, 0})
	data := colorWebP(vp8x, webpChunk("ICCP", profile), webpChunk("VP8L", []byte{0x2f, 0, 0, 0, 0}))
	if got := readColor(t, data, codec.WebP); !bytes.Equal(got.ICC, profile) {
		t.Fatalf("%d bytes", len(got.ICC))
	}
	if got := readColor(t, colorWebP(vp8x), codec.WebP); !got.Empty() {
		t.Fatalf("%+v", got)
	}
}

func TestColorOfOtherFormatsAndBrokenInputIsEmpty(t *testing.T) {
	data := colorPNG(colorChunk("cICP", []byte{1, 13, 0, 1}))
	if got := readColor(t, data, codec.GIF); !got.Empty() {
		t.Fatal("GIF has no colour description")
	}
	for cut := 0; cut < len(data); cut += 7 {
		ReadColor(context.Background(), bytes.NewReader(data[:cut]), int64(cut), codec.PNG)
	}
	if got := ReadColor(context.Background(), nil, 0, codec.PNG); !got.Empty() {
		t.Fatal("nil reader")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got := ReadColor(ctx, bytes.NewReader(data), int64(len(data)), codec.PNG); !got.Empty() {
		t.Fatal("cancelled context")
	}
}

func TestNewColorCopiesItsInputs(t *testing.T) {
	profile := colorProfile("RGB ", 12)
	cicp := &codec.CICP{Primaries: 12, Transfer: 13}
	got := NewColor(profile, cicp)
	profile[130] ^= 0xff
	cicp.Primaries = 1
	if got.ICC[130] == profile[130] || got.CICP.Primaries != 12 {
		t.Fatal("NewColor aliases its arguments")
	}
}

func FuzzReadColor(f *testing.F) {
	profile := colorProfile("RGB ", 60)
	f.Add(colorPNG(colorChunk("cICP", []byte{9, 16, 0, 1}), colorChunk("iCCP", iccChunkBody(profile))), uint8(0))
	f.Add(colorJPEG(iccSegment(1, 2, profile[:80]), iccSegment(2, 2, profile[80:])), uint8(1))
	f.Add(colorWebP(webpChunk("VP8X", []byte{0x20, 0, 0, 0, 1, 0, 0, 1, 0, 0}), webpChunk("ICCP", profile)), uint8(2))
	f.Add(profile, uint8(0))
	f.Add([]byte{}, uint8(1))
	ids := []codec.ID{codec.PNG, codec.JPEG, codec.WebP, codec.APNG, codec.TIFF, codec.GIF}
	f.Fuzz(func(t *testing.T, data []byte, sel uint8) {
		got := ReadColor(context.Background(), bytes.NewReader(data), int64(len(data)), ids[int(sel)%len(ids)])
		if len(got.ICC) > 0 && (len(got.ICC) < iccHeaderSize || len(got.ICC) > MaxICCBytes || ColorProfile(got.ICC) == nil) {
			t.Fatalf("unusable profile of %d bytes", len(got.ICC))
		}
		if got.CICP != nil && got.Empty() {
			t.Fatal("empty with a description")
		}
	})
}

func TestCMYKColorOfAJPEGKeepsOnlyCMYKProfiles(t *testing.T) {
	cmyk := colorProfile("CMYK", 300)
	rgb := colorProfile("RGB ", 300)
	read := func(data []byte) codec.ColorInfo {
		return ReadCMYKColor(context.Background(), bytes.NewReader(data), int64(len(data)))
	}
	got := read(colorJPEG(iccSegment(2, 2, cmyk[150:]), iccSegment(1, 2, cmyk[:150])))
	if !bytes.Equal(got.ICC, cmyk) || got.CICP != nil {
		t.Fatalf("%d bytes, cicp %+v", len(got.ICC), got.CICP)
	}
	if got := read(colorJPEG(iccSegment(1, 1, rgb))); !got.Empty() {
		t.Fatal("RGB profile accepted for CMYK data")
	}
	if got := read(colorJPEG(iccSegment(1, 2, cmyk[:150]))); !got.Empty() {
		t.Fatal("incomplete profile accepted")
	}
	if got := read(colorJPEG()); !got.Empty() {
		t.Fatal("profile invented")
	}
	if got := readColor(t, colorJPEG(iccSegment(1, 1, cmyk)), codec.JPEG); !got.Empty() {
		t.Fatal("ReadColor must keep ignoring CMYK profiles")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	data := colorJPEG(iccSegment(1, 1, cmyk))
	if got := ReadCMYKColor(ctx, bytes.NewReader(data), int64(len(data))); !got.Empty() {
		t.Fatal("cancelled context")
	}
	if got := ReadCMYKColor(nil, nil, 0); !got.Empty() {
		t.Fatal("nil reader")
	}
	if CMYKProfile(rgb) != nil || CMYKProfile(cmyk) == nil || ColorProfile(cmyk) != nil {
		t.Fatal("profile selection")
	}
}
