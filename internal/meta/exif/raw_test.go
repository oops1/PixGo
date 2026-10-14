package exif

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
)

func headerWithMagic(order testOrder, mark string, magic uint16) []byte {
	out := []byte(mark)
	out = order.AppendUint16(out, magic)
	return order.AppendUint32(out, 8)
}

func TestCameraReaderAcceptsTheRawFileHeaders(t *testing.T) {
	cases := []struct {
		name  string
		mark  string
		order testOrder
		magic uint16
		ok    bool
	}{
		{"classic little endian", "II", binary.LittleEndian, 42, true},
		{"classic big endian", "MM", binary.BigEndian, 42, true},
		{"Olympus IIRO", "II", binary.LittleEndian, MagicOlympus, true},
		{"Olympus IIRS", "II", binary.LittleEndian, MagicOlympusAlt, true},
		{"Olympus MMOR", "MM", binary.BigEndian, MagicOlympus, true},
		{"Panasonic", "II", binary.LittleEndian, MagicPanasonic, true},
		{"BigTIFF", "II", binary.LittleEndian, 43, false},
		{"unknown magic", "II", binary.LittleEndian, 0x1234, false},
		{"bad byte order mark", "XX", binary.LittleEndian, 42, false},
	}
	for _, c := range cases {
		header := append(headerWithMagic(c.order, c.mark, c.magic), 0, 0, 0, 0, 0, 0)
		r, err := NewReaderAtCamera(bytes.NewReader(header), 0, int64(len(header)))
		if (err == nil) != c.ok {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if c.ok && (r.FirstOffset() != 8 || r.ByteOrder() != c.order) {
			t.Errorf("%s: first %d", c.name, r.FirstOffset())
		}
		if _, err := NewReader(header); (err == nil) != (c.ok && c.magic == 42) {
			t.Errorf("%s: the strict reader must stay strict: %v", c.name, err)
		}
	}
	if _, err := NewReaderAtCamera(nil, 0, 10); err == nil {
		t.Error("nil reader accepted")
	}
	if _, err := NewReaderAtCamera(bytes.NewReader(nil), -1, 10); err == nil {
		t.Error("negative base accepted")
	}
	if _, err := NewReaderAtCamera(bytes.NewReader(nil), 0, -1); err == nil {
		t.Error("negative length accepted")
	}
	if _, err := NewReaderAtCamera(bytes.NewReader(make([]byte, 4)), 0, 4); err == nil {
		t.Error("short header accepted")
	}
}

func TestOffsetReaderBuildsDirectoriesWithoutAHeader(t *testing.T) {
	b := newTIFFBuilder(binary.BigEndian)
	b.buf = b.buf[:0]
	b.ifd([]rawEntry{{tag: TagOrientation, typ: TypeShort, count: 1, value: b.short(6)}}, 0)
	file := append(bytes.Repeat([]byte{0xcc}, 40), b.buf...)
	r, err := NewReaderAtOffset(bytes.NewReader(file), 40, int64(len(file)-40), binary.BigEndian, 0)
	if err != nil {
		t.Fatal(err)
	}
	dir, err := r.Directory(0)
	if err != nil || len(dir.Entries) != 1 {
		t.Fatalf("%v %+v", err, dir)
	}
	if o, ok := r.Orientation(); !ok || o != codec.OrientRotate90 {
		t.Fatalf("orientation %d %v", o, ok)
	}
	for name, call := range map[string]func() (*Reader, error){
		"nil reader":      func() (*Reader, error) { return NewReaderAtOffset(nil, 0, 1, binary.BigEndian, 0) },
		"negative base":   func() (*Reader, error) { return NewReaderAtOffset(bytes.NewReader(file), -1, 1, binary.BigEndian, 0) },
		"negative length": func() (*Reader, error) { return NewReaderAtOffset(bytes.NewReader(file), 0, -1, binary.BigEndian, 0) },
		"no byte order":   func() (*Reader, error) { return NewReaderAtOffset(bytes.NewReader(file), 0, 1, nil, 0) },
	} {
		if _, err := call(); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestOrientationOfAReaderWithoutTheTag(t *testing.T) {
	b := newTIFFBuilder(binary.LittleEndian)
	b.ifd([]rawEntry{{tag: 0x0100, typ: TypeShort, count: 1, value: b.short(5)}}, 0)
	r, err := NewReader(b.buf)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := r.Orientation(); ok {
		t.Fatal("orientation invented")
	}
}

func TestEntryLocationIsOnlyDefinedForOutOfLineValues(t *testing.T) {
	b := newTIFFBuilder(binary.LittleEndian)
	long := bytes.Repeat([]byte{7}, 30)
	b.ifd([]rawEntry{
		{tag: 0x0100, typ: TypeShort, count: 1, value: b.short(5)},
		{tag: 0x0101, typ: TypeShort, count: 2, value: b.short(1, 2)},
		{tag: 0x0102, typ: TypeUndefined, count: 30, value: long},
		{tag: 0x0103, typ: TypeLong, count: 5, value: b.long(1, 2, 3, 4, 5)},
	}, 0)
	r, err := NewReader(b.buf)
	if err != nil {
		t.Fatal(err)
	}
	dir, err := r.Directory(r.FirstOffset())
	if err != nil {
		t.Fatal(err)
	}
	inline, _ := dir.Find(0x0100)
	if _, _, ok := inline.Location(); ok {
		t.Fatal("inline value has a location")
	}
	pair, _ := dir.Find(0x0101)
	if _, _, ok := pair.Location(); ok {
		t.Fatal("four inline bytes have a location")
	}
	blob, _ := dir.Find(0x0102)
	offset, length, ok := blob.Location()
	if !ok || length != 30 || !bytes.Equal(b.buf[offset:offset+length], long) {
		t.Fatalf("%d %d %v", offset, length, ok)
	}
	array, _ := dir.Find(0x0103)
	if _, length, ok := array.Location(); !ok || length != 20 {
		t.Fatalf("%d %v", length, ok)
	}
	if _, _, ok := (Entry{}).Location(); ok {
		t.Fatal("zero entry has a location")
	}
	huge := Entry{Tag: 1, Type: TypeLong, Count: 100, reader: r}
	if _, _, ok := huge.Location(); !ok {
		t.Fatal("a large array still has a location")
	}
}
