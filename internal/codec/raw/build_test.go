package raw

import (
	"bytes"
	"context"
	"encoding/binary"
	"image"
	"image/color"
	stdjpeg "image/jpeg"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/testimg"
)

type byteOrder interface {
	binary.ByteOrder
	binary.AppendByteOrder
}

const (
	typeByte      = 1
	typeASCIIType = 2
	typeShort     = 3
	typeLong      = 4
	typeUndefined = 7
	tagOrient     = 0x0112
)

var (
	tomato  = color.NRGBA{R: 230, G: 60, B: 40, A: 255}
	navy    = color.NRGBA{R: 20, G: 40, B: 160, A: 255}
	forest  = color.NRGBA{R: 30, G: 140, B: 60, A: 255}
	mustard = color.NRGBA{R: 220, G: 190, B: 30, A: 255}
	little  = byteOrder(binary.LittleEndian)
	big     = byteOrder(binary.BigEndian)
)

func jpegOf(tb testing.TB, w, h int, c color.NRGBA) []byte {
	tb.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, 255
	}
	var buf bytes.Buffer
	if err := stdjpeg.Encode(&buf, img, &stdjpeg.Options{Quality: 95}); err != nil {
		tb.Fatal(err)
	}
	return buf.Bytes()
}

func grayJPEG(tb testing.TB, w, h int) []byte {
	tb.Helper()
	img := image.NewGray(image.Rect(0, 0, w, h))
	for i := range img.Pix {
		img.Pix[i] = uint8(i)
	}
	var buf bytes.Buffer
	if err := stdjpeg.Encode(&buf, img, nil); err != nil {
		tb.Fatal(err)
	}
	return buf.Bytes()
}

func withExifOrientation(jpegData []byte, value uint16, bigEndian bool) []byte {
	payload := append([]byte("Exif\x00\x00"), testimg.OrientationTIFF(bigEndian, value)...)
	segment := []byte{0xff, 0xe1, byte((len(payload) + 2) >> 8), byte(len(payload) + 2)}
	segment = append(segment, payload...)
	out := append([]byte(nil), jpegData[:2]...)
	out = append(out, segment...)
	return append(out, jpegData[2:]...)
}

func losslessJPEG(w, h int) []byte {
	out := []byte{0xff, 0xd8, 0xff, 0xc3, 0, 11, 8, byte(h >> 8), byte(h), byte(w >> 8), byte(w), 1, 1, 0x11, 0}
	out = append(out, 0xff, 0xda, 0, 8, 1, 1, 0, 1, 0, 0)
	return append(out, bytes.Repeat([]byte{0x55}, 64)...)
}

func frameJPEG(marker byte, precision byte, w, h int, components byte) []byte {
	length := 8 + 3*int(components)
	out := []byte{0xff, 0xd8, 0xff, marker, byte(length >> 8), byte(length), precision, byte(h >> 8), byte(h), byte(w >> 8), byte(w), components}
	for i := 0; i < int(components); i++ {
		out = append(out, byte(i+1), 0x11, 0)
	}
	return append(out, 0xff, 0xda, 0, 2, 0xff, 0xd9)
}

type blobRef struct {
	data   []byte
	offset uint32
}

type entry struct {
	tag    uint16
	typ    uint16
	count  uint32
	value  uint32
	inline []byte
	ref    *blobRef
}

type patch struct {
	at  int
	ref *blobRef
}

type tiffFile struct {
	order   byteOrder
	buf     []byte
	blobs   []*blobRef
	patches []patch
}

func newTIFF(order byteOrder, header [4]byte) *tiffFile {
	t := &tiffFile{order: order}
	t.buf = append(t.buf, header[:]...)
	t.buf = append(t.buf, 0, 0, 0, 0)
	return t
}

func standardTIFF(order byteOrder) *tiffFile {
	if order == little {
		return newTIFF(order, [4]byte{'I', 'I', 42, 0})
	}
	return newTIFF(order, [4]byte{'M', 'M', 0, 42})
}

func (t *tiffFile) later(data []byte) *blobRef {
	ref := &blobRef{data: data}
	t.blobs = append(t.blobs, ref)
	return ref
}

func (t *tiffFile) align() {
	if len(t.buf)%2 == 1 {
		t.buf = append(t.buf, 0)
	}
}

func (t *tiffFile) now(data []byte) uint32 {
	t.align()
	at := uint32(len(t.buf))
	t.buf = append(t.buf, data...)
	return at
}

func (t *tiffFile) ifd(entries ...entry) (offset uint32, nextAt int) {
	offset, nextAt, _ = t.ifdAt(entries...)
	return offset, nextAt
}

func (t *tiffFile) ifdAt(entries ...entry) (offset uint32, nextAt int, valueAt map[uint16]int) {
	sortEntries(entries)
	t.align()
	offset = uint32(len(t.buf))
	valueAt = map[uint16]int{}
	t.buf = t.order.AppendUint16(t.buf, uint16(len(entries)))
	for _, e := range entries {
		t.buf = t.order.AppendUint16(t.buf, e.tag)
		t.buf = t.order.AppendUint16(t.buf, e.typ)
		t.buf = t.order.AppendUint32(t.buf, e.count)
		at := len(t.buf)
		valueAt[e.tag] = at
		switch {
		case e.ref != nil:
			t.buf = append(t.buf, 0, 0, 0, 0)
			t.patches = append(t.patches, patch{at: at, ref: e.ref})
		case e.inline != nil:
			field := make([]byte, 4)
			copy(field, e.inline)
			t.buf = append(t.buf, field...)
		case e.typ == typeShort:
			t.buf = t.order.AppendUint16(t.buf, uint16(e.value))
			t.buf = append(t.buf, 0, 0)
		default:
			t.buf = t.order.AppendUint32(t.buf, e.value)
		}
	}
	nextAt = len(t.buf)
	t.buf = append(t.buf, 0, 0, 0, 0)
	return offset, nextAt, valueAt
}

func (t *tiffFile) link(nextAt int, offset uint32) {
	t.order.PutUint32(t.buf[nextAt:], offset)
}

func (t *tiffFile) setFirst(offset uint32) { t.link(4, offset) }

func (t *tiffFile) data() []byte {
	for _, ref := range t.blobs {
		t.align()
		ref.offset = uint32(len(t.buf))
		t.buf = append(t.buf, ref.data...)
	}
	for _, p := range t.patches {
		t.order.PutUint32(t.buf[p.at:], p.ref.offset)
	}
	t.patches = nil
	return t.buf
}

func sortEntries(entries []entry) {
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0 && entries[j].tag < entries[j-1].tag; j-- {
			entries[j], entries[j-1] = entries[j-1], entries[j]
		}
	}
}

func shortEntry(tag uint16, v uint16) entry {
	return entry{tag: tag, typ: typeShort, count: 1, value: uint32(v)}
}

func longEntry(tag uint16, v uint32) entry {
	return entry{tag: tag, typ: typeLong, count: 1, value: v}
}

func refEntry(tag uint16, ref *blobRef) entry {
	return entry{tag: tag, typ: typeLong, count: 1, ref: ref}
}

func undefinedEntry(tag uint16, ref *blobRef) entry {
	return entry{tag: tag, typ: typeUndefined, count: uint32(len(ref.data)), ref: ref}
}

func (t *tiffFile) asciiEntry(tag uint16, text string) entry {
	data := append([]byte(text), 0)
	if len(data) <= 4 {
		return entry{tag: tag, typ: typeASCIIType, count: uint32(len(data)), inline: data}
	}
	return entry{tag: tag, typ: typeASCIIType, count: uint32(len(data)), value: t.now(data)}
}

func (t *tiffFile) longsEntry(tag uint16, values ...uint32) entry {
	if len(values) == 1 {
		return longEntry(tag, values[0])
	}
	var data []byte
	for _, v := range values {
		data = t.order.AppendUint32(data, v)
	}
	return entry{tag: tag, typ: typeLong, count: uint32(len(values)), ref: t.later(data)}
}

func dngVersion() entry {
	return entry{tag: tagDNGVersion, typ: typeByte, count: 4, inline: []byte{1, 4, 0, 0}}
}

func jpegTags(ref *blobRef) []entry {
	return []entry{refEntry(tagJPEGOffset, ref), longEntry(tagJPEGLength, uint32(len(ref.data)))}
}

func stripTags(ref *blobRef, compression uint16) []entry {
	return []entry{
		shortEntry(tagCompression, compression),
		refEntry(tagStripOffsets, ref),
		longEntry(tagStripCounts, uint32(len(ref.data))),
	}
}

func join(groups ...[]entry) []entry {
	var out []entry
	for _, g := range groups {
		out = append(out, g...)
	}
	return out
}

func orientationTag(v uint16) []entry {
	if v == 0 {
		return nil
	}
	return []entry{shortEntry(tagOrient, v)}
}

type previews struct {
	large, medium, small, thumb []byte
}

func makePreviews(tb testing.TB) previews {
	return previews{
		large:  jpegOf(tb, 96, 64, tomato),
		medium: jpegOf(tb, 48, 32, navy),
		small:  jpegOf(tb, 24, 16, forest),
		thumb:  jpegOf(tb, 16, 12, mustard),
	}
}

func cr2File(p previews, orientation uint16) []byte {
	t := newTIFF(little, [4]byte{'I', 'I', 42, 0})
	t.buf = append(t.buf, 'C', 'R', 2, 0, 0, 0, 0, 0)
	raw := t.later(losslessJPEG(120, 80))
	preview, thumb := t.later(p.large), t.later(p.thumb)
	rawIFD, _ := t.ifd(join(stripTags(raw, compressionOldJPEG), []entry{longEntry(0x0100, 120)})...)
	ifd1, next1 := t.ifd(jpegTags(thumb)...)
	t.link(next1, rawIFD)
	ifd0, next0 := t.ifd(join(stripTags(preview, compressionOldJPEG), []entry{t.asciiEntry(tagMake, "Canon")}, orientationTag(orientation))...)
	t.link(next0, ifd1)
	t.setFirst(ifd0)
	return t.data()
}

func nefFile(order byteOrder, p previews, orientation uint16) []byte {
	t := standardTIFF(order)
	large, medium, thumb := t.later(p.large), t.later(p.medium), t.later(p.thumb)
	raw := t.later(losslessJPEG(200, 100))
	rawSub, _ := t.ifd(stripTags(raw, compressionJPEG)...)
	previewSub, _ := t.ifd(join(jpegTags(large), []entry{shortEntry(tagCompression, compressionJPEG)})...)
	smallSub, _ := t.ifd(jpegTags(medium)...)
	ifd1, _ := t.ifd(jpegTags(thumb)...)
	ifd0, next0 := t.ifd(join([]entry{t.asciiEntry(tagMake, "NIKON CORPORATION"), t.longsEntry(tagSubIFDs, smallSub, previewSub, rawSub)}, orientationTag(orientation))...)
	t.link(next0, ifd1)
	t.setFirst(ifd0)
	return t.data()
}

func arwFile(p previews) []byte {
	t := standardTIFF(little)
	large, thumb := t.later(p.large), t.later(p.thumb)
	ifd1, _ := t.ifd(jpegTags(thumb)...)
	ifd0, next0 := t.ifd(join(jpegTags(large), []entry{t.asciiEntry(tagMake, "SONY")})...)
	t.link(next0, ifd1)
	t.setFirst(ifd0)
	return t.data()
}

func dngFile(order byteOrder, p previews, orientation uint16) []byte {
	t := standardTIFF(order)
	large := t.later(p.large)
	raw := t.later(losslessJPEG(300, 200))
	rawSub, _ := t.ifd(join(stripTags(raw, compressionJPEG), []entry{longEntry(0x0100, 300)})...)
	previewSub, _ := t.ifd(join(stripTags(large, compressionJPEG), []entry{longEntry(0x00fe, 1)})...)
	ifd0, _ := t.ifd(join([]entry{dngVersion(), t.longsEntry(tagSubIFDs, previewSub, rawSub)}, orientationTag(orientation))...)
	t.setFirst(ifd0)
	return t.data()
}

func rw2File(p previews) []byte {
	t := newTIFF(little, [4]byte{'I', 'I', 0x55, 0})
	large := t.later(p.large)
	ifd0, _ := t.ifd(undefinedEntry(tagJpgFromRaw, large), t.asciiEntry(tagMake, "Panasonic"))
	t.setFirst(ifd0)
	return t.data()
}

func olympusNote(order byteOrder, newStyle bool, shift uint32, p previews) []byte {
	header := []byte("OLYMP\x00\x01\x00")
	if newStyle {
		header = []byte("OLYMPUS\x00II\x03\x00")
		if order == big {
			header = []byte("OLYMPUS\x00MM\x00\x03")
		}
	}
	mainAt := uint32(len(header))
	settingsAt := mainAt + 2 + 2*12 + 4
	thumbAt := settingsAt + 2 + 2*12 + 4
	bigAt := thumbAt + uint32(len(p.thumb))
	note := append([]byte(nil), header...)
	put16 := func(v uint16) { note = order.AppendUint16(note, v) }
	put32 := func(v uint32) { note = order.AppendUint32(note, v) }
	put16(2)
	put16(tagOlympusThumb)
	put16(typeUndefined)
	put32(uint32(len(p.thumb)))
	put32(shift + thumbAt)
	put16(tagOlympusCamera)
	put16(typeLong)
	put32(1)
	put32(shift + settingsAt)
	put32(0)
	put16(2)
	put16(tagOlympusStart)
	put16(typeLong)
	put32(1)
	put32(shift + bigAt)
	put16(tagOlympusLength)
	put16(typeLong)
	put32(1)
	put32(uint32(len(p.large)))
	put32(0)
	note = append(note, p.thumb...)
	return append(note, p.large...)
}

func nikonNote(p previews) []byte {
	note := []byte("Nikon\x00\x02\x10\x00\x00MM\x00\x2a\x00\x00\x00\x08")
	put16 := func(v uint16) { note = big.AppendUint16(note, v) }
	put32 := func(v uint32) { note = big.AppendUint32(note, v) }
	previewAt := uint32(8 + 2 + 12 + 4)
	bigAt := previewAt + 2 + 2*12 + 4
	put16(1)
	put16(tagNikonPreview)
	put16(typeLong)
	put32(1)
	put32(previewAt)
	put32(0)
	put16(2)
	put16(tagJPEGOffset)
	put16(typeLong)
	put32(1)
	put32(bigAt)
	put16(tagJPEGLength)
	put16(typeLong)
	put32(1)
	put32(uint32(len(p.large)))
	put32(0)
	return append(note, p.large...)
}

func makerNoteFile(order byteOrder, header [4]byte, makeName string, build func(t *tiffFile) []byte, p previews) []byte {
	t := newTIFF(order, header)
	thumb := t.later(p.thumb)
	ifd0, next0, at0 := t.ifdAt(longEntry(tagExifIFD, 0), t.asciiEntry(tagMake, makeName))
	t.align()
	note := build(t)
	noteAt := t.now(note)
	exifIFD, _ := t.ifd(entry{tag: tagMakerNote, typ: typeUndefined, count: uint32(len(note)), value: noteAt})
	ifd1, _ := t.ifd(jpegTags(thumb)...)
	t.link(at0[tagExifIFD], exifIFD)
	t.link(next0, ifd1)
	t.setFirst(ifd0)
	return t.data()
}

func orfFile(p previews, style string) []byte {
	build := func(t *tiffFile) []byte {
		switch style {
		case "old":
			t.align()
			return olympusNote(little, false, uint32(len(t.buf)), p)
		}
		return olympusNote(little, true, 0, p)
	}
	return makerNoteFile(little, [4]byte{'I', 'I', 'R', 'O'}, "OLYMPUS CORPORATION", build, p)
}

func nikonMakerNoteFile(p previews) []byte {
	return makerNoteFile(big, [4]byte{'M', 'M', 0, 42}, "NIKON CORPORATION", func(*tiffFile) []byte { return nikonNote(p) }, p)
}

func carvedFile(p previews, prefix int) []byte {
	t := standardTIFF(little)
	thumb := t.later(p.thumb)
	ifd0, _ := t.ifd(join(jpegTags(thumb), []entry{t.asciiEntry(tagMake, "PENTAX Corporation")})...)
	t.setFirst(ifd0)
	data := t.data()
	data = append(data, bytes.Repeat([]byte{0x11}, prefix)...)
	data = append(data, p.large...)
	return append(data, bytes.Repeat([]byte{0x22}, 40)...)
}

func mp4Box(kind string, parts ...[]byte) []byte {
	payload := bytes.Join(parts, nil)
	out := binary.BigEndian.AppendUint32(nil, uint32(8+len(payload)))
	out = append(out, kind...)
	return append(out, payload...)
}

var previewUUID = []byte{0xea, 0xf4, 0x2b, 0x5e, 0x1c, 0x98, 0x4b, 0x88, 0xb9, 0xfb, 0xb7, 0xdc, 0x40, 0x6e, 0x4d, 0x16}

func cr3File(p previews, orientation uint16) []byte {
	var cmt1 []byte
	if orientation != 0 {
		cmt1 = mp4Box("CMT1", testimg.OrientationTIFF(false, orientation))
	}
	thumbHeader := make([]byte, 16)
	thumbHeader[0] = 1
	canon := mp4Box("uuid", canonUUID, mp4Box("CNCV", []byte("CanonCR3_001/00.09.00/00.00.00")), cmt1, mp4Box("THMB", thumbHeader, p.thumb))
	moov := mp4Box("moov", mp4Box("mvhd", make([]byte, 100)), canon)
	prvw := mp4Box("uuid", previewUUID, make([]byte, 8), mp4Box("PRVW", make([]byte, 12), p.medium))
	mdat := mp4Box("mdat", bytes.Repeat([]byte{0x33}, 200), p.large, bytes.Repeat([]byte{0x44}, 200))
	ftyp := mp4Box("ftyp", []byte("crx "), []byte{0, 1, 0, 0}, []byte("crx isom"))
	return bytes.Join([][]byte{ftyp, moov, prvw, mdat}, nil)
}

func rafFile(jpegData []byte) []byte {
	header := make([]byte, 160)
	copy(header, "FUJIFILMCCD-RAW 0201FF129502X-T20")
	binary.BigEndian.PutUint32(header[84:], uint32(len(header)))
	binary.BigEndian.PutUint32(header[88:], uint32(len(jpegData)))
	out := append(header, jpegData...)
	return append(out, bytes.Repeat([]byte{0x77}, 300)...)
}

func probeData(data []byte) (codec.Info, error) {
	return Decoder{}.Probe(context.Background(), bytes.NewReader(data), int64(len(data)), codec.DefaultLimits())
}

func decodeData(data []byte) (*codec.Image, error) {
	return Decoder{}.Decode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Options{Limits: codec.DefaultLimits()})
}

func mustProbe(tb testing.TB, data []byte) codec.Info {
	tb.Helper()
	info, err := probeData(data)
	if err != nil {
		tb.Fatal(err)
	}
	return info
}

func mustDecode(tb testing.TB, data []byte) *codec.Image {
	tb.Helper()
	img, err := decodeData(data)
	if err != nil {
		tb.Fatal(err)
	}
	return img
}

func closeTo(a, b color.NRGBA, tolerance int) bool {
	diff := func(x, y uint8) int {
		if x > y {
			return int(x - y)
		}
		return int(y - x)
	}
	return diff(a.R, b.R) <= tolerance && diff(a.G, b.G) <= tolerance && diff(a.B, b.B) <= tolerance
}
