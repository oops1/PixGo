package print

import (
	"bytes"
	"context"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/jpeg"
	"image/png"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
)

func tiffWithResolution(xr, yr uint32, unit uint16) []byte {
	var b bytes.Buffer
	le := binary.LittleEndian
	b.WriteString("II")
	binary.Write(&b, le, uint16(42))
	binary.Write(&b, le, uint32(8))
	binary.Write(&b, le, uint16(3))
	entry := func(tag, typ uint16, count uint32, value uint32) {
		binary.Write(&b, le, tag)
		binary.Write(&b, le, typ)
		binary.Write(&b, le, count)
		binary.Write(&b, le, value)
	}
	entry(0x011a, 5, 1, 50)
	entry(0x011b, 5, 1, 58)
	entry(0x0128, 3, 1, uint32(unit))
	binary.Write(&b, le, uint32(0))
	binary.Write(&b, le, xr)
	binary.Write(&b, le, uint32(1))
	binary.Write(&b, le, yr)
	binary.Write(&b, le, uint32(1))
	return b.Bytes()
}

func segment(marker byte, payload []byte) []byte {
	out := []byte{0xff, marker, 0, 0}
	binary.BigEndian.PutUint16(out[2:], uint16(len(payload)+2))
	return append(out, payload...)
}

func plainJPEG(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := jpeg.Encode(&b, image.NewRGBA(image.Rect(0, 0, 8, 8)), nil); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func withSegments(t *testing.T, segs ...[]byte) []byte {
	t.Helper()
	base := plainJPEG(t)
	out := append([]byte{}, base[:2]...)
	for _, s := range segs {
		out = append(out, s...)
	}
	return append(out, base[2:]...)
}

func jfif(units byte, x, y uint16) []byte {
	p := []byte("JFIF\x00\x01\x01")
	p = append(p, units, 0, 0, 0, 0, 0, 0)
	binary.BigEndian.PutUint16(p[8:], x)
	binary.BigEndian.PutUint16(p[10:], y)
	return segment(0xe0, p)
}

func exifSegment(tiff []byte) []byte {
	return segment(0xe1, append([]byte("Exif\x00\x00"), tiff...))
}

func read(t *testing.T, data []byte, id codec.ID) (float64, bool) {
	t.Helper()
	return ReadDPI(context.Background(), bytes.NewReader(data), int64(len(data)), id)
}

func TestReadDPIFromJFIF(t *testing.T) {
	for _, c := range []struct {
		name  string
		units byte
		x, y  uint16
		want  float64
		ok    bool
	}{
		{"dpi", 1, 300, 300, 300, true},
		{"dots per cm", 2, 118, 118, 299.72, true},
		{"aspect only", 0, 1, 1, 0, false},
		{"absurd", 1, 2, 2, 0, false},
	} {
		got, ok := read(t, withSegments(t, jfif(c.units, c.x, c.y)), codec.JPEG)
		if ok != c.ok || (ok && !near(got, c.want, 0.01)) {
			t.Errorf("%s: %v %v, want %v %v", c.name, got, ok, c.want, c.ok)
		}
	}
}

func TestReadDPIFromEXIF(t *testing.T) {
	for _, c := range []struct {
		name string
		x, y uint32
		unit uint16
		want float64
		ok   bool
	}{
		{"inches", 240, 240, 2, 240, true},
		{"centimetres", 118, 118, 3, 299.72, true},
		{"no unit", 240, 240, 1, 0, false},
		{"zero", 0, 0, 2, 0, false},
		{"only y valid", 0, 180, 2, 180, true},
	} {
		got, ok := read(t, withSegments(t, exifSegment(tiffWithResolution(c.x, c.y, c.unit))), codec.JPEG)
		if ok != c.ok || (ok && !near(got, c.want, 0.01)) {
			t.Errorf("%s: %v %v, want %v %v", c.name, got, ok, c.want, c.ok)
		}
	}
}

func TestEXIFWinsOverJFIF(t *testing.T) {
	data := withSegments(t, jfif(1, 72, 72), exifSegment(tiffWithResolution(350, 350, 2)))
	if got, ok := read(t, data, codec.JPEG); !ok || got != 350 {
		t.Errorf("%v %v", got, ok)
	}
}

func TestReadDPIFromPNGPhysChunk(t *testing.T) {
	var plain bytes.Buffer
	if err := png.Encode(&plain, image.NewRGBA(image.Rect(0, 0, 4, 4))); err != nil {
		t.Fatal(err)
	}
	data := plain.Bytes()
	chunk := func(unit byte, x, y uint32) []byte {
		body := make([]byte, 9)
		binary.BigEndian.PutUint32(body[0:], x)
		binary.BigEndian.PutUint32(body[4:], y)
		body[8] = unit
		out := make([]byte, 0, 21)
		out = binary.BigEndian.AppendUint32(out, 9)
		out = append(out, "pHYs"...)
		out = append(out, body...)
		return binary.BigEndian.AppendUint32(out, crc32.ChecksumIEEE(append([]byte("pHYs"), body...)))
	}
	insert := func(c []byte) []byte {
		at := 8 + 25
		return append(append(append([]byte{}, data[:at]...), c...), data[at:]...)
	}
	if got, ok := read(t, insert(chunk(1, 11811, 11811)), codec.PNG); !ok || !near(got, 300, 0.1) {
		t.Errorf("300 dpi: %v %v", got, ok)
	}
	if got, ok := read(t, insert(chunk(1, 2835, 2835)), codec.PNG); !ok || !near(got, 72, 0.1) {
		t.Errorf("72 dpi: %v %v", got, ok)
	}
	if _, ok := read(t, insert(chunk(0, 1, 1)), codec.PNG); ok {
		t.Error("a ratio-only pHYs has no resolution")
	}
	if _, ok := read(t, data, codec.PNG); ok {
		t.Error("a PNG without pHYs has no resolution")
	}
}

func TestReadDPIFromBMPHeader(t *testing.T) {
	h := make([]byte, 54)
	h[0], h[1] = 'B', 'M'
	binary.LittleEndian.PutUint32(h[14:], 40)
	binary.LittleEndian.PutUint32(h[38:], 3780)
	binary.LittleEndian.PutUint32(h[42:], 3780)
	if got, ok := read(t, h, codec.BMP); !ok || !near(got, 96, 0.1) {
		t.Errorf("%v %v", got, ok)
	}
	binary.LittleEndian.PutUint32(h[38:], 0)
	binary.LittleEndian.PutUint32(h[42:], 0)
	if _, ok := read(t, h, codec.BMP); ok {
		t.Error("zero pixels per metre")
	}
}

func TestReadDPIIsQuietOnHostileInput(t *testing.T) {
	for _, id := range []codec.ID{codec.JPEG, codec.PNG, codec.BMP, codec.WebP, codec.TIFF, codec.GIF, codec.SVG, codec.RAW} {
		for _, data := range [][]byte{nil, {0xff}, {0xff, 0xd8, 0xff}, bytes.Repeat([]byte{0xff, 0xe0, 0xff, 0xff}, 64), []byte("BM"), []byte("\x89PNG\r\n\x1a\n")} {
			if v, ok := read(t, data, id); ok {
				t.Errorf("%s %q: %v", id, data, v)
			}
		}
	}
	if _, ok := ReadDPI(nil, nil, 0, codec.JPEG); ok {
		t.Error("nil reader")
	}
}

func FuzzReadDPI(f *testing.F) {
	f.Add([]byte("\xff\xd8\xff\xe0\x00\x10JFIF\x00\x01\x01\x01\x01,\x01,\x00\x00"), uint8(0))
	f.Add([]byte("\x89PNG\r\n\x1a\n\x00\x00\x00\x09pHYs\x00\x00\x2e\x23\x00\x00\x2e\x23\x01\x00\x00\x00\x00"), uint8(1))
	f.Add([]byte("BM\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x28\x00\x00\x00"), uint8(2))
	ids := []codec.ID{codec.JPEG, codec.PNG, codec.BMP, codec.WebP, codec.TIFF}
	f.Fuzz(func(t *testing.T, data []byte, which uint8) {
		v, ok := ReadDPI(context.Background(), bytes.NewReader(data), int64(len(data)), ids[int(which)%len(ids)])
		if ok && !validDPI(v) {
			t.Fatalf("invalid density %v", v)
		}
	})
}
