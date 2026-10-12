package exif

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
)

type rawEntry struct {
	tag   uint16
	typ   Type
	count uint32
	value []byte
}

type tiffBuilder struct {
	order testOrder
	buf   []byte
}

func newTIFFBuilder(order testOrder) *tiffBuilder {
	b := &tiffBuilder{order: order}
	if order == binary.LittleEndian {
		b.buf = append(b.buf, 'I', 'I')
	} else {
		b.buf = append(b.buf, 'M', 'M')
	}
	b.buf = order.AppendUint16(b.buf, 42)
	b.buf = order.AppendUint32(b.buf, 8)
	return b
}

func (b *tiffBuilder) short(values ...uint16) []byte {
	var out []byte
	for _, v := range values {
		out = b.order.AppendUint16(out, v)
	}
	return out
}

func (b *tiffBuilder) long(values ...uint32) []byte {
	var out []byte
	for _, v := range values {
		out = b.order.AppendUint32(out, v)
	}
	return out
}

func (b *tiffBuilder) ifd(entries []rawEntry, next uint32) uint32 {
	start := uint32(len(b.buf))
	tableSize := 2 + len(entries)*12 + 4
	b.buf = b.order.AppendUint16(b.buf, uint16(len(entries)))
	var extra []byte
	for _, e := range entries {
		b.buf = b.order.AppendUint16(b.buf, e.tag)
		b.buf = b.order.AppendUint16(b.buf, uint16(e.typ))
		b.buf = b.order.AppendUint32(b.buf, e.count)
		if len(e.value) <= 4 {
			slot := make([]byte, 4)
			copy(slot, e.value)
			b.buf = append(b.buf, slot...)
		} else {
			b.buf = b.order.AppendUint32(b.buf, start+uint32(tableSize)+uint32(len(extra)))
			extra = append(extra, e.value...)
			if len(extra)%2 == 1 {
				extra = append(extra, 0)
			}
		}
	}
	b.buf = b.order.AppendUint32(b.buf, next)
	b.buf = append(b.buf, extra...)
	return start
}

func orientationTIFF(order testOrder, value uint16) []byte {
	b := newTIFFBuilder(order)
	b.ifd([]rawEntry{{tag: TagOrientation, typ: TypeShort, count: 1, value: b.short(value)}}, 0)
	return b.buf
}

func TestOrientationAllValuesBothByteOrders(t *testing.T) {
	for _, order := range []testOrder{binary.LittleEndian, binary.BigEndian} {
		for v := uint16(1); v <= 8; v++ {
			got, ok := Orientation(orientationTIFF(order, v))
			if !ok || got != codec.Orientation(v) {
				t.Fatalf("%v value %d: got %d ok=%v", order, v, got, ok)
			}
		}
	}
}

func TestOrientationRejectsOutOfRangeAndWrongType(t *testing.T) {
	for _, v := range []uint16{0, 9, 255, 65535} {
		if got, ok := Orientation(orientationTIFF(binary.LittleEndian, v)); ok || got != codec.OrientNormal {
			t.Fatalf("value %d accepted as %d", v, got)
		}
	}
	b := newTIFFBuilder(binary.LittleEndian)
	b.ifd([]rawEntry{{tag: TagOrientation, typ: TypeLong, count: 1, value: b.long(6)}}, 0)
	if _, ok := Orientation(b.buf); ok {
		t.Fatal("LONG typed orientation must be rejected")
	}
}

func TestOrientationMissing(t *testing.T) {
	b := newTIFFBuilder(binary.BigEndian)
	b.ifd([]rawEntry{{tag: 0x010f, typ: TypeASCII, count: 6, value: []byte("Canon\x00")}}, 0)
	if got, ok := Orientation(b.buf); ok || got != codec.OrientNormal {
		t.Fatalf("got %d ok=%v", got, ok)
	}
}

func TestOrientationBadHeaders(t *testing.T) {
	for name, data := range map[string][]byte{
		"nil":         nil,
		"short":       {'I', 'I', 42, 0},
		"bad order":   {'X', 'X', 42, 0, 8, 0, 0, 0},
		"bad magic":   {'I', 'I', 43, 0, 8, 0, 0, 0},
		"ifd outside": {'I', 'I', 42, 0, 200, 0, 0, 0},
		"ifd at end":  {'I', 'I', 42, 0, 8, 0, 0, 0},
	} {
		if _, ok := Orientation(data); ok {
			t.Fatalf("%s: orientation reported", name)
		}
	}
}

func TestEveryTruncationIsSafe(t *testing.T) {
	b := newTIFFBuilder(binary.LittleEndian)
	b.ifd([]rawEntry{
		{tag: 0x010f, typ: TypeASCII, count: 6, value: []byte("Canon\x00")},
		{tag: TagOrientation, typ: TypeShort, count: 1, value: b.short(6)},
		{tag: 0x829a, typ: TypeRational, count: 1, value: b.long(1, 250)},
	}, 0)
	for n := 0; n <= len(b.buf); n++ {
		prefix := b.buf[:n]
		Orientation(prefix)
		if r, err := NewReader(prefix); err == nil {
			r.Tree()
			r.Chain()
		}
	}
	if _, ok := Orientation(b.buf); !ok {
		t.Fatal("complete sample must parse")
	}
}

func TestTruncatedTableStillYieldsPresentEntries(t *testing.T) {
	b := newTIFFBuilder(binary.LittleEndian)
	b.ifd([]rawEntry{
		{tag: TagOrientation, typ: TypeShort, count: 1, value: b.short(3)},
		{tag: 0x0100, typ: TypeLong, count: 1, value: b.long(10)},
		{tag: 0x0101, typ: TypeLong, count: 1, value: b.long(10)},
	}, 0)
	cut := b.buf[:8+2+12+5]
	if got, ok := Orientation(cut); !ok || got != codec.OrientRotate180 {
		t.Fatalf("got %d ok=%v", got, ok)
	}
}

func TestHugeEntryCountIsRejected(t *testing.T) {
	data := []byte{'I', 'I', 42, 0, 8, 0, 0, 0, 0xff, 0xff}
	data = append(data, make([]byte, 65535*12+4)...)
	r, err := NewReader(data)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Directory(r.FirstOffset()); !errors.Is(err, codec.ErrCorrupt) {
		t.Fatalf("got %v", err)
	}
	if _, ok := Orientation(data); ok {
		t.Fatal("hostile count must not produce an orientation")
	}
}

func TestValueOffsetOutsideDataIsRejected(t *testing.T) {
	b := newTIFFBuilder(binary.LittleEndian)
	b.ifd([]rawEntry{{tag: 0x829a, typ: TypeRational, count: 1, value: b.long(1, 2)}}, 0)
	data := append([]byte(nil), b.buf...)
	binary.LittleEndian.PutUint32(data[8+2+8:], 0xfffffff0)
	r, err := NewReader(data)
	if err != nil {
		t.Fatal(err)
	}
	dir, err := r.Directory(r.FirstOffset())
	if err != nil {
		t.Fatal(err)
	}
	entry, _ := dir.Find(0x829a)
	if _, _, ok := entry.Rational(0); ok {
		t.Fatal("rational with out-of-range offset must not resolve")
	}
}

func TestHugeCountDoesNotAllocate(t *testing.T) {
	b := newTIFFBuilder(binary.LittleEndian)
	b.ifd([]rawEntry{{tag: 0x0111, typ: TypeLong, count: 0xffffffff, value: b.long(8)}}, 0)
	r, err := NewReader(b.buf)
	if err != nil {
		t.Fatal(err)
	}
	dir, err := r.Directory(r.FirstOffset())
	if err != nil {
		t.Fatal(err)
	}
	entry, _ := dir.Find(0x0111)
	if _, ok := entry.Raw(); ok {
		t.Fatal("4G entries must not resolve")
	}
	if _, ok := entry.Uint(0); ok {
		t.Fatal("Uint must fail for oversized value")
	}
}

func TestIFDLoopsAreDetected(t *testing.T) {
	b := newTIFFBuilder(binary.LittleEndian)
	b.ifd([]rawEntry{
		{tag: TagOrientation, typ: TypeShort, count: 1, value: b.short(2)},
		{tag: TagExifPointer, typ: TypeLong, count: 1, value: b.long(8)},
		{tag: TagGPSPointer, typ: TypeLong, count: 1, value: b.long(8)},
	}, 8)
	r, err := NewReader(b.buf)
	if err != nil {
		t.Fatal(err)
	}
	chain, err := r.Chain()
	if err != nil || len(chain) != 1 {
		t.Fatalf("chain %d err %v", len(chain), err)
	}
	tree, err := r.Tree()
	if err != nil || tree.IFD0 == nil || tree.Exif != nil || tree.GPS != nil || tree.IFD1 != nil {
		t.Fatalf("tree %+v err %v", tree, err)
	}
}

func TestIFDTwoNodeLoop(t *testing.T) {
	b := newTIFFBuilder(binary.BigEndian)
	first := b.ifd([]rawEntry{{tag: TagOrientation, typ: TypeShort, count: 1, value: b.short(8)}}, 0)
	second := b.ifd([]rawEntry{{tag: TagOrientation, typ: TypeShort, count: 1, value: b.short(1)}}, first)
	binary.BigEndian.PutUint32(b.buf[first+2+12:], second)
	r, err := NewReader(b.buf)
	if err != nil {
		t.Fatal(err)
	}
	chain, err := r.Chain()
	if err != nil || len(chain) != 2 {
		t.Fatalf("chain %d err %v", len(chain), err)
	}
}

func TestTreeReadsExifGPSAndThumbnailDirectories(t *testing.T) {
	b := newTIFFBuilder(binary.BigEndian)
	ifd0Offset := uint32(len(b.buf))
	pointerEntries := func(exif, gps uint32) []rawEntry {
		return []rawEntry{
			{tag: 0x010f, typ: TypeASCII, count: 6, value: []byte("Canon\x00")},
			{tag: TagOrientation, typ: TypeShort, count: 1, value: b.short(6)},
			{tag: TagExifPointer, typ: TypeLong, count: 1, value: b.long(exif)},
			{tag: TagGPSPointer, typ: TypeLong, count: 1, value: b.long(gps)},
		}
	}
	ifd0Size := uint32(2 + 4*12 + 4 + 6)
	exifOffset := ifd0Offset + ifd0Size
	exifEntries := []rawEntry{
		{tag: 0x829a, typ: TypeRational, count: 1, value: b.long(1, 250)},
		{tag: 0x8827, typ: TypeShort, count: 1, value: b.short(100)},
	}
	exifSize := uint32(2 + 2*12 + 4 + 8)
	gpsOffset := exifOffset + exifSize
	gpsEntries := []rawEntry{
		{tag: 0x0001, typ: TypeASCII, count: 2, value: []byte("N\x00")},
		{tag: 0x0002, typ: TypeRational, count: 3, value: b.long(55, 1, 45, 1, 300, 100)},
	}
	gpsSize := uint32(2 + 2*12 + 4 + 24)
	thumbOffset := gpsOffset + gpsSize
	if got := b.ifd(pointerEntries(exifOffset, gpsOffset), thumbOffset); got != ifd0Offset {
		t.Fatalf("ifd0 at %d", got)
	}
	if got := b.ifd(exifEntries, 0); got != exifOffset {
		t.Fatalf("exif at %d, want %d", got, exifOffset)
	}
	if got := b.ifd(gpsEntries, 0); got != gpsOffset {
		t.Fatalf("gps at %d, want %d", got, gpsOffset)
	}
	b.ifd([]rawEntry{{tag: 0x0103, typ: TypeShort, count: 1, value: b.short(6)}}, 0)

	r, err := NewReader(b.buf)
	if err != nil {
		t.Fatal(err)
	}
	tree, err := r.Tree()
	if err != nil {
		t.Fatal(err)
	}
	if tree.IFD0 == nil || tree.Exif == nil || tree.GPS == nil || tree.IFD1 == nil {
		t.Fatalf("missing directories: %+v", tree)
	}
	makeEntry, _ := tree.IFD0.Find(0x010f)
	if text, ok := makeEntry.Text(); !ok || text != "Canon" {
		t.Fatalf("make string %q ok=%v", text, ok)
	}
	exposure, _ := tree.Exif.Find(0x829a)
	if n, d, ok := exposure.Rational(0); !ok || n != 1 || d != 250 {
		t.Fatalf("exposure %d/%d ok=%v", n, d, ok)
	}
	iso, _ := tree.Exif.Find(0x8827)
	if v, ok := iso.Uint(0); !ok || v != 100 {
		t.Fatalf("iso %d ok=%v", v, ok)
	}
	if _, ok := iso.Uint(1); ok {
		t.Fatal("index past count must fail")
	}
	latitude, _ := tree.GPS.Find(0x0002)
	if n, d, ok := latitude.Rational(2); !ok || n != 300 || d != 100 {
		t.Fatalf("seconds %d/%d ok=%v", n, d, ok)
	}
	if e, ok := tree.IFD1.Find(0x0103); !ok || e.Tag != 0x0103 {
		t.Fatal("thumbnail directory not read")
	}
	if got, ok := Orientation(b.buf); !ok || got != codec.OrientRotate90 {
		t.Fatalf("orientation %d ok=%v", got, ok)
	}
}

func TestSignedAccessors(t *testing.T) {
	b := newTIFFBuilder(binary.LittleEndian)
	b.ifd([]rawEntry{
		{tag: 0x9204, typ: TypeSShort, count: 2, value: b.short(0xfffe, 5)},
		{tag: 0x9205, typ: TypeSByte, count: 1, value: []byte{0xff}},
		{tag: 0x9206, typ: TypeSLong, count: 1, value: b.long(0xfffffff0)},
	}, 0)
	r, err := NewReader(b.buf)
	if err != nil {
		t.Fatal(err)
	}
	dir, _ := r.Directory(r.FirstOffset())
	e, _ := dir.Find(0x9204)
	if v, ok := e.Int(0); !ok || v != -2 {
		t.Fatalf("sshort %d", v)
	}
	if v, ok := e.Int(1); !ok || v != 5 {
		t.Fatalf("sshort %d", v)
	}
	e, _ = dir.Find(0x9205)
	if v, ok := e.Int(0); !ok || v != -1 {
		t.Fatalf("sbyte %d", v)
	}
	e, _ = dir.Find(0x9206)
	if v, ok := e.Int(0); !ok || v != -16 {
		t.Fatalf("slong %d", v)
	}
	if _, ok := e.Uint(0); ok {
		t.Fatal("Uint must reject signed types")
	}
}

func TestReaderAtWithBaseOffset(t *testing.T) {
	tiff := orientationTIFF(binary.BigEndian, 7)
	file := append(bytes.Repeat([]byte{0xaa}, 100), tiff...)
	file = append(file, bytes.Repeat([]byte{0xbb}, 50)...)
	got, ok := OrientationAt(bytes.NewReader(file), 100, int64(len(tiff)))
	if !ok || got != codec.OrientTransverse {
		t.Fatalf("got %d ok=%v", got, ok)
	}
	if _, ok := OrientationAt(bytes.NewReader(file), 100, 5); ok {
		t.Fatal("window too small must fail")
	}
	if _, ok := OrientationAt(bytes.NewReader(file), -1, 10); ok {
		t.Fatal("negative base must fail")
	}
	if _, ok := OrientationAt(bytes.NewReader(file[:120]), 100, int64(len(tiff))); ok {
		t.Fatal("window beyond the reader must fail")
	}
}

func TestStripExifHeader(t *testing.T) {
	tiff := orientationTIFF(binary.LittleEndian, 3)
	prefixed := append([]byte("Exif\x00\x00"), tiff...)
	if !bytes.Equal(StripExifHeader(prefixed), tiff) {
		t.Fatal("prefix not removed")
	}
	if !bytes.Equal(StripExifHeader(tiff), tiff) {
		t.Fatal("bare TIFF must be unchanged")
	}
	if len(StripExifHeader([]byte("Exif"))) != 4 {
		t.Fatal("short input must be unchanged")
	}
}

func jpegWithSegments(segments ...[]byte) []byte {
	out := []byte{0xff, 0xd8}
	for _, s := range segments {
		out = append(out, s...)
	}
	return out
}

func segment(marker byte, payload []byte) []byte {
	out := []byte{0xff, marker, byte((len(payload) + 2) >> 8), byte(len(payload) + 2)}
	return append(out, payload...)
}

func exifSegment(tiff []byte) []byte {
	return segment(0xe1, append([]byte("Exif\x00\x00"), tiff...))
}

func TestFromJPEGFindsExifAfterOtherSegments(t *testing.T) {
	tiff := orientationTIFF(binary.LittleEndian, 6)
	data := jpegWithSegments(
		segment(0xe0, []byte("JFIF\x00\x01\x01\x00\x00\x01\x00\x01\x00\x00")),
		segment(0xe1, []byte("http://ns.adobe.com/xap/1.0/\x00<x/>")),
		exifSegment(tiff),
		segment(0xdb, make([]byte, 65)),
		[]byte{0xff, 0xda, 0, 2},
	)
	got, err := FromJPEG(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, tiff) {
		t.Fatalf("payload %x, want %x", got, tiff)
	}
	if o, ok := Orientation(got); !ok || o != codec.OrientRotate90 {
		t.Fatalf("orientation %d ok=%v", o, ok)
	}
}

func TestFromJPEGReturnsFirstExifSegment(t *testing.T) {
	first := orientationTIFF(binary.LittleEndian, 2)
	second := orientationTIFF(binary.LittleEndian, 5)
	data := jpegWithSegments(exifSegment(first), exifSegment(second))
	got, err := FromJPEG(bytes.NewReader(data), int64(len(data)))
	if err != nil || !bytes.Equal(got, first) {
		t.Fatalf("got %x err %v", got, err)
	}
}

func TestFromJPEGWithoutExif(t *testing.T) {
	for name, data := range map[string][]byte{
		"sos first":   jpegWithSegments([]byte{0xff, 0xda, 0, 2}),
		"eoi":         jpegWithSegments([]byte{0xff, 0xd9}),
		"no segments": {0xff, 0xd8},
		"only app0":   jpegWithSegments(segment(0xe0, []byte("JFIF\x00"))),
	} {
		if _, err := FromJPEG(bytes.NewReader(data), int64(len(data))); !errors.Is(err, ErrNoExif) {
			t.Fatalf("%s: got %v", name, err)
		}
	}
}

func TestFromJPEGSkipsFillBytesAndStandaloneMarkers(t *testing.T) {
	tiff := orientationTIFF(binary.BigEndian, 4)
	data := jpegWithSegments([]byte{0xff, 0xff, 0xff, 0xd0, 0xff, 0x01}, exifSegment(tiff))
	got, err := FromJPEG(bytes.NewReader(data), int64(len(data)))
	if err != nil || !bytes.Equal(got, tiff) {
		t.Fatalf("got %x err %v", got, err)
	}
}

func TestFromJPEGHostile(t *testing.T) {
	for name, data := range map[string][]byte{
		"not jpeg":         []byte("GIF89a"),
		"empty":            nil,
		"garbage marker":   {0xff, 0xd8, 0x12, 0x34, 0x56, 0x78},
		"length too small": {0xff, 0xd8, 0xff, 0xe1, 0x00, 0x01},
		"length past end":  {0xff, 0xd8, 0xff, 0xe1, 0xff, 0xff, 'E', 'x'},
		"cut header":       {0xff, 0xd8, 0xff, 0xe1, 0x00},
	} {
		if _, err := FromJPEG(bytes.NewReader(data), int64(len(data))); err == nil {
			t.Fatalf("%s: expected an error", name)
		}
	}
	endless := append([]byte{0xff, 0xd8}, bytes.Repeat([]byte{0xff}, 1<<20)...)
	if _, err := FromJPEG(bytes.NewReader(endless), int64(len(endless))); err == nil {
		t.Fatal("endless fill bytes must stop")
	}
	tiny := append([]byte{0xff, 0xd8}, bytes.Repeat([]byte{0xff, 0xe2, 0, 2}, 100000)...)
	if _, err := FromJPEG(bytes.NewReader(tiny), int64(len(tiny))); !errors.Is(err, ErrNoExif) {
		t.Fatalf("many tiny segments: got %v", err)
	}
}

func TestFromJPEGEveryTruncation(t *testing.T) {
	tiff := orientationTIFF(binary.LittleEndian, 6)
	data := jpegWithSegments(segment(0xe0, []byte("JFIF\x00")), exifSegment(tiff), []byte{0xff, 0xda, 0, 2})
	for n := 0; n <= len(data); n++ {
		FromJPEG(bytes.NewReader(data[:n]), int64(n))
	}
}

func FuzzDecode(f *testing.F) {
	f.Add(orientationTIFF(binary.LittleEndian, 6))
	f.Add(orientationTIFF(binary.BigEndian, 3))
	f.Add(jpegWithSegments(exifSegment(orientationTIFF(binary.BigEndian, 8)), []byte{0xff, 0xda, 0, 2}))
	f.Add([]byte{'I', 'I', 42, 0, 8, 0, 0, 0, 1, 0, 0x12, 0x01, 3, 0, 1, 0, 0, 0, 6, 0, 0, 0, 8, 0, 0, 0})
	f.Fuzz(func(t *testing.T, data []byte) {
		Orientation(data)
		Orientation(StripExifHeader(data))
		OrientationAt(bytes.NewReader(data), 0, int64(len(data)))
		if r, err := NewReader(data); err == nil {
			if tree, err := r.Tree(); err == nil {
				for _, dir := range []*Directory{tree.IFD0, tree.Exif, tree.GPS, tree.IFD1} {
					if dir == nil {
						continue
					}
					for _, e := range dir.Entries {
						e.Raw()
						e.Uint(0)
						e.Int(0)
						e.Rational(0)
						e.Text()
					}
				}
			}
			r.Chain()
		}
		if payload, err := FromJPEG(bytes.NewReader(data), int64(len(data))); err == nil {
			Orientation(payload)
		}
	})
}

type testOrder interface {
	binary.ByteOrder
	binary.AppendByteOrder
}
