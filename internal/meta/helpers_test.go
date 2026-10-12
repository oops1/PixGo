package meta

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"image"
	"image/png"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/oops1/PixGo/internal/codec"
)

type byteOrder interface {
	binary.ByteOrder
	binary.AppendByteOrder
}

type tentry struct {
	tag   uint16
	typ   uint16
	count uint32
	data  []byte
}

type enc struct{ o byteOrder }

var orders = []struct {
	name  string
	order byteOrder
}{
	{"little", binary.LittleEndian},
	{"big", binary.BigEndian},
}

func (e enc) ascii(tag uint16, s string) tentry {
	data := append([]byte(s), 0)
	return tentry{tag: tag, typ: 2, count: uint32(len(data)), data: data}
}

func (e enc) asciiRaw(tag uint16, raw []byte) tentry {
	return tentry{tag: tag, typ: 2, count: uint32(len(raw)), data: raw}
}

func (e enc) bytesVal(tag uint16, v ...byte) tentry {
	return tentry{tag: tag, typ: 1, count: uint32(len(v)), data: v}
}

func (e enc) undefined(tag uint16, data []byte) tentry {
	return tentry{tag: tag, typ: 7, count: uint32(len(data)), data: data}
}

func (e enc) short(tag uint16, v ...uint16) tentry {
	var data []byte
	for _, x := range v {
		data = e.o.AppendUint16(data, x)
	}
	return tentry{tag: tag, typ: 3, count: uint32(len(v)), data: data}
}

func (e enc) long(tag uint16, v ...uint32) tentry {
	var data []byte
	for _, x := range v {
		data = e.o.AppendUint32(data, x)
	}
	return tentry{tag: tag, typ: 4, count: uint32(len(v)), data: data}
}

func (e enc) rational(tag uint16, v ...uint32) tentry {
	var data []byte
	for _, x := range v {
		data = e.o.AppendUint32(data, x)
	}
	return tentry{tag: tag, typ: 5, count: uint32(len(v) / 2), data: data}
}

func (e enc) srational(tag uint16, v ...int32) tentry {
	var data []byte
	for _, x := range v {
		data = e.o.AppendUint32(data, uint32(x))
	}
	return tentry{tag: tag, typ: 10, count: uint32(len(v) / 2), data: data}
}

type tiffParts struct {
	ifd0, exif, gps []tentry
}

func ifdSize(entries []tentry) int {
	n := 2 + 12*len(entries) + 4
	for _, e := range entries {
		if len(e.data) > 4 {
			n += len(e.data) + len(e.data)&1
		}
	}
	return n
}

func (e enc) writeIFD(buf []byte, entries []tentry, next uint32) []byte {
	start := len(buf)
	buf = e.o.AppendUint16(buf, uint16(len(entries)))
	extra := start + 2 + 12*len(entries) + 4
	var tail []byte
	for _, en := range entries {
		buf = e.o.AppendUint16(buf, en.tag)
		buf = e.o.AppendUint16(buf, en.typ)
		buf = e.o.AppendUint32(buf, en.count)
		if len(en.data) <= 4 {
			slot := make([]byte, 4)
			copy(slot, en.data)
			buf = append(buf, slot...)
			continue
		}
		buf = e.o.AppendUint32(buf, uint32(extra+len(tail)))
		tail = append(tail, en.data...)
		if len(en.data)&1 == 1 {
			tail = append(tail, 0)
		}
	}
	buf = e.o.AppendUint32(buf, next)
	return append(buf, tail...)
}

func (e enc) header() []byte {
	var buf []byte
	if e.o == binary.LittleEndian {
		buf = append(buf, 'I', 'I')
	} else {
		buf = append(buf, 'M', 'M')
	}
	buf = e.o.AppendUint16(buf, 42)
	return e.o.AppendUint32(buf, 8)
}

func (e enc) tiff(p tiffParts) []byte {
	ifd0 := append([]tentry(nil), p.ifd0...)
	exifAt := len(ifd0)
	if p.exif != nil {
		ifd0 = append(ifd0, e.long(0x8769, 0))
	}
	gpsAt := len(ifd0)
	if p.gps != nil {
		ifd0 = append(ifd0, e.long(0x8825, 0))
	}
	exifOffset := 8 + ifdSize(ifd0)
	gpsOffset := exifOffset
	if p.exif != nil {
		gpsOffset += ifdSize(p.exif)
	}
	if p.exif != nil {
		ifd0[exifAt] = e.long(0x8769, uint32(exifOffset))
	}
	if p.gps != nil {
		ifd0[gpsAt] = e.long(0x8825, uint32(gpsOffset))
	}
	buf := e.header()
	buf = e.writeIFD(buf, ifd0, 0)
	if p.exif != nil {
		buf = e.writeIFD(buf, p.exif, 0)
	}
	if p.gps != nil {
		buf = e.writeIFD(buf, p.gps, 0)
	}
	return buf
}

func photoParts(e enc, below bool) tiffParts {
	altRef := byte(0)
	if below {
		altRef = 1
	}
	return tiffParts{
		ifd0: []tentry{
			e.ascii(0x010f, "Canon"),
			e.ascii(0x0110, "Canon EOS R5"),
			e.short(0x0112, 6),
			e.ascii(0x0131, "Firmware Version 1.8.1"),
			e.ascii(0x0132, "2024:09:14 18:27:09"),
		},
		exif: []tentry{
			e.rational(0x829a, 1, 250),
			e.rational(0x829d, 40, 10),
			e.short(0x8822, 3),
			e.short(0x8827, 100),
			e.ascii(0x9003, "2024:09:14 18:27:03"),
			e.ascii(0x9011, "+03:00"),
			e.short(0x9207, 5),
			e.short(0x9209, 0x19),
			e.rational(0x920a, 24, 1),
			e.ascii(0x9291, "45"),
			e.short(0xa001, 1),
			e.long(0xa002, 6000),
			e.long(0xa003, 4000),
			e.short(0xa403, 1),
			e.short(0xa405, 24),
			e.ascii(0xa434, "RF 24-70mm F2.8 L"),
		},
		gps: []tentry{
			e.bytesVal(0x0000, 2, 3, 0, 0),
			e.ascii(0x0001, "N"),
			e.rational(0x0002, 55, 1, 45, 1, 211, 10),
			e.ascii(0x0003, "E"),
			e.rational(0x0004, 37, 1, 37, 1, 25, 10),
			e.bytesVal(0x0005, altRef),
			e.rational(0x0006, 156, 1),
			e.rational(0x0007, 15, 1, 27, 1, 3, 1),
			e.ascii(0x001d, "2024:09:14"),
		},
	}
}

func jpegSeg(marker byte, payload []byte) []byte {
	n := len(payload) + 2
	out := []byte{0xff, marker, byte(n >> 8), byte(n)}
	return append(out, payload...)
}

func jpegExifSeg(tiff []byte) []byte {
	return jpegSeg(0xe1, append([]byte("Exif\x00\x00"), tiff...))
}

func jpegXMPSeg(xmp string) []byte {
	return jpegSeg(0xe1, append([]byte("http://ns.adobe.com/xap/1.0/\x00"), xmp...))
}

func jpegICCSegs(profile []byte, chunk int) [][]byte {
	var parts [][]byte
	for off := 0; off < len(profile); off += chunk {
		end := off + chunk
		if end > len(profile) {
			end = len(profile)
		}
		parts = append(parts, profile[off:end])
	}
	var segs [][]byte
	for i, part := range parts {
		payload := append([]byte("ICC_PROFILE\x00"), byte(i+1), byte(len(parts)))
		segs = append(segs, jpegSeg(0xe2, append(payload, part...)))
	}
	return segs
}

func jpegSOF(w, h int) []byte {
	return jpegSeg(0xc0, []byte{8, byte(h >> 8), byte(h), byte(w >> 8), byte(w), 3, 1, 0x22, 0, 2, 0x11, 1, 3, 0x11, 1})
}

func buildJPEG(w, h int, segs ...[]byte) []byte {
	out := []byte{0xff, 0xd8}
	for _, s := range segs {
		out = append(out, s...)
	}
	out = append(out, jpegSOF(w, h)...)
	out = append(out, jpegSeg(0xda, []byte{3, 1, 0, 2, 0x11, 3, 0x11, 0, 0x3f, 0})...)
	out = append(out, 0x12, 0x34, 0xff, 0x00, 0x56, 0xff, 0xd9)
	return out
}

func pngChunk(kind string, data []byte) []byte {
	out := binary.BigEndian.AppendUint32(nil, uint32(len(data)))
	out = append(out, kind...)
	out = append(out, data...)
	return binary.BigEndian.AppendUint32(out, crc32.ChecksumIEEE(append([]byte(kind), data...)))
}

func splitPNG(tb testing.TB, data []byte) [][]byte {
	tb.Helper()
	var chunks [][]byte
	pos := 8
	for pos < len(data) {
		n := int(binary.BigEndian.Uint32(data[pos:]))
		end := pos + 8 + n + 4
		chunks = append(chunks, data[pos:end])
		pos = end
	}
	return chunks
}

func buildPNG(tb testing.TB, w, h int, before, after [][]byte) []byte {
	tb.Helper()
	var enc bytes.Buffer
	if err := png.Encode(&enc, image.NewNRGBA(image.Rect(0, 0, w, h))); err != nil {
		tb.Fatal(err)
	}
	chunks := splitPNG(tb, enc.Bytes())
	out := append([]byte(nil), enc.Bytes()[:8]...)
	out = append(out, chunks[0]...)
	for _, c := range before {
		out = append(out, c...)
	}
	for _, c := range chunks[1 : len(chunks)-1] {
		out = append(out, c...)
	}
	for _, c := range after {
		out = append(out, c...)
	}
	return append(out, chunks[len(chunks)-1]...)
}

func riffChunk(kind string, data []byte) []byte {
	out := binary.LittleEndian.AppendUint32([]byte(kind), uint32(len(data)))
	out = append(out, data...)
	if len(data)&1 == 1 {
		out = append(out, 0)
	}
	return out
}

func buildWebP(chunks ...[]byte) []byte {
	body := []byte("WEBP")
	for _, c := range chunks {
		body = append(body, c...)
	}
	out := binary.LittleEndian.AppendUint32([]byte("RIFF"), uint32(len(body)))
	return append(out, body...)
}

func vp8xChunk(flags byte, w, h int) []byte {
	data := []byte{flags, 0, 0, 0, byte(w - 1), byte((w - 1) >> 8), byte((w - 1) >> 16), byte(h - 1), byte((h - 1) >> 8), byte((h - 1) >> 16)}
	return riffChunk("VP8X", data)
}

func vp8lChunk(w, h int) []byte {
	data := []byte{0x2f}
	data = binary.LittleEndian.AppendUint32(data, uint32(w-1)|uint32(h-1)<<14)
	return riffChunk("VP8L", append(data, 0, 0, 0, 0, 0))
}

func vp8Chunk(w, h int) []byte {
	data := []byte{0x10, 0x02, 0x00, 0x9d, 0x01, 0x2a}
	data = binary.LittleEndian.AppendUint16(data, uint16(w))
	data = binary.LittleEndian.AppendUint16(data, uint16(h))
	return riffChunk("VP8 ", append(data, 0, 0, 0, 0))
}

func descASCII(s string) []byte {
	out := []byte("desc\x00\x00\x00\x00")
	out = binary.BigEndian.AppendUint32(out, uint32(len(s)+1))
	out = append(out, s...)
	out = append(out, 0)
	return append(out, make([]byte, 78)...)
}

type mlucRecord struct{ lang, country, text string }

func descMLUC(records ...mlucRecord) []byte {
	out := []byte("mluc\x00\x00\x00\x00")
	out = binary.BigEndian.AppendUint32(out, uint32(len(records)))
	out = binary.BigEndian.AppendUint32(out, 12)
	offset := 16 + 12*len(records)
	var tail []byte
	for _, r := range records {
		units := utf16.Encode([]rune(r.text))
		var raw []byte
		for _, u := range units {
			raw = binary.BigEndian.AppendUint16(raw, u)
		}
		out = append(out, r.lang...)
		out = append(out, r.country...)
		out = binary.BigEndian.AppendUint32(out, uint32(len(raw)))
		out = binary.BigEndian.AppendUint32(out, uint32(offset+len(tail)))
		tail = append(tail, raw...)
	}
	return append(out, tail...)
}

func iccProfile(major, minor byte, class, space string, desc []byte, padding int) []byte {
	header := make([]byte, 128)
	header[8], header[9] = major, minor
	copy(header[12:], class)
	copy(header[16:], space)
	copy(header[20:], "XYZ ")
	copy(header[36:], "acsp")
	out := append(header, 0, 0, 0, 1)
	out = append(out, "desc"...)
	out = binary.BigEndian.AppendUint32(out, uint32(len(out)+8))
	out = binary.BigEndian.AppendUint32(out, uint32(len(desc)))
	out = append(out, desc...)
	out = append(out, bytes.Repeat([]byte{0xa5}, padding)...)
	binary.BigEndian.PutUint32(out[0:4], uint32(len(out)))
	return out
}

func sRGBProfile() []byte {
	return iccProfile(4, 0x30, "mntr", "RGB ", descMLUC(
		mlucRecord{"de", "DE", "sRGB Profil"},
		mlucRecord{"en", "US", "sRGB IEC61966-2.1"},
	), 600)
}

const xmpSample = `<?xpacket begin="` + string(rune(0xfeff)) + `" id="W5M0MpCehiHzreSzNTczkc9d"?>
<x:xmpmeta xmlns:x="adobe:ns:meta/">
 <rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">
  <rdf:Description rdf:about=""
    xmlns:dc="http://purl.org/dc/elements/1.1/"
    xmlns:xmp="http://ns.adobe.com/xap/1.0/"
    xmlns:tiff="http://ns.adobe.com/tiff/1.0/"
    xmp:Rating="4">
   <dc:title><rdf:Alt><rdf:li xml:lang="ru">Закат</rdf:li><rdf:li xml:lang="x-default">Sunset over Moscow</rdf:li></rdf:Alt></dc:title>
   <dc:description><rdf:Alt><rdf:li xml:lang="x-default">Evening light &amp; river</rdf:li></rdf:Alt></dc:description>
   <dc:creator><rdf:Seq><rdf:li>Ivan Petrov</rdf:li><rdf:li>Anna</rdf:li></rdf:Seq></dc:creator>
  </rdf:Description>
 </rdf:RDF>
</x:xmpmeta>
<?xpacket end="w"?>`

func checkXMPSample(t *testing.T, x *XMP) {
	t.Helper()
	if x == nil {
		t.Fatal("XMP missing")
	}
	if x.Title != "Sunset over Moscow" || x.Description != "Evening light & river" || x.Rating != 4 {
		t.Fatalf("XMP %+v", *x)
	}
	if len(x.Creator) != 2 || x.Creator[0] != "Ivan Petrov" || x.Creator[1] != "Anna" {
		t.Fatalf("creators %q", x.Creator)
	}
}

func approx(a, b float64) bool {
	d := a - b
	return d < 1e-9 && d > -1e-9
}

func checkPhoto(t *testing.T, m *Metadata, below bool) {
	t.Helper()
	x := m.Exif
	if x == nil {
		t.Fatal("Exif missing")
	}
	if x.Make != "Canon" || x.Model != "Canon EOS R5" || x.Lens != "RF 24-70mm F2.8 L" || x.Software != "Firmware Version 1.8.1" {
		t.Fatalf("strings %q %q %q %q", x.Make, x.Model, x.Lens, x.Software)
	}
	if x.Exposure != (Rational{1, 250}) || x.Exposure.String() != "1/250" {
		t.Fatalf("exposure %+v %q", x.Exposure, x.Exposure.String())
	}
	if x.FNumber.Float() != 4 || x.FNumber.FNumberString() != "4.0" {
		t.Fatalf("fnumber %+v %q", x.FNumber, x.FNumber.FNumberString())
	}
	if x.ISO != 100 || x.FocalLength.Float() != 24 || x.FocalLength.String() != "24" || x.FocalLength35 != 24 {
		t.Fatalf("iso %d focal %+v 35mm %d", x.ISO, x.FocalLength, x.FocalLength35)
	}
	if x.Flash == nil || !x.Flash.Fired || x.Flash.Mode != FlashModeAuto || x.Flash.Return != FlashReturnNone || x.Flash.Code != 0x19 {
		t.Fatalf("flash %+v", x.Flash)
	}
	want := time.Date(2024, 9, 14, 18, 27, 3, 450_000_000, time.FixedZone("", 3*3600))
	if !x.TakenAt.Equal(want) || !x.HasTakenZone {
		t.Fatalf("taken %v zone=%v", x.TakenAt, x.HasTakenZone)
	}
	if _, off := x.TakenAt.Zone(); off != 3*3600 {
		t.Fatalf("zone offset %d", off)
	}
	if x.Orientation != codec.OrientRotate90 || x.ExposureProgram != 3 || x.MeteringMode != 5 || x.WhiteBalance != 1 || x.ColorSpace != 1 {
		t.Fatalf("codes %+v", *x)
	}
	if x.PixelWidth != 6000 || x.PixelHeight != 4000 {
		t.Fatalf("pixels %dx%d", x.PixelWidth, x.PixelHeight)
	}
	g := m.GPS
	if g == nil {
		t.Fatal("GPS missing")
	}
	if !approx(g.Lat, 55+45.0/60+21.1/3600) || !approx(g.Lon, 37+37.0/60+2.5/3600) {
		t.Fatalf("coordinates %v %v", g.Lat, g.Lon)
	}
	wantAlt := 156.0
	if below {
		wantAlt = -156
	}
	if !g.HasAlt || g.Alt != wantAlt {
		t.Fatalf("altitude %v has=%v", g.Alt, g.HasAlt)
	}
	if !g.Time.Equal(time.Date(2024, 9, 14, 15, 27, 3, 0, time.UTC)) {
		t.Fatalf("gps time %v", g.Time)
	}
	if g.DMS() != "55°45′21.1″ N, 37°37′02.5″ E" {
		t.Fatalf("DMS %q", g.DMS())
	}
	if g.Decimal() != "55.755861, 37.617361" {
		t.Fatalf("decimal %q", g.Decimal())
	}
}

func checkICC(t *testing.T, c *ICC, size int) {
	t.Helper()
	if c == nil {
		t.Fatal("ICC missing")
	}
	if c.Description != "sRGB IEC61966-2.1" || c.ColorSpace != "RGB" || c.Class != "Display" || c.Version != "4.3" {
		t.Fatalf("ICC %+v", *c)
	}
	if size > 0 && c.Size != size {
		t.Fatalf("ICC size %d, want %d", c.Size, size)
	}
}

func mustRead(t testing.TB, data []byte, id codec.ID) *Metadata {
	t.Helper()
	m, err := Read(t.Context(), bytes.NewReader(data), int64(len(data)), id)
	if err != nil {
		t.Fatalf("Read(%s): %v", id, err)
	}
	if m == nil {
		t.Fatal("nil metadata")
	}
	return m
}

func readAny(data []byte, id codec.ID) (m *Metadata, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	return Read(context.Background(), bytes.NewReader(data), int64(len(data)), id)
}
