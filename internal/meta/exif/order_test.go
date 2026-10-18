package exif

import (
	"encoding/binary"
	"testing"
)

func tinyTIFF(order binary.AppendByteOrder, big bool, makeAt uint32) []byte {
	out := []byte{'I', 'I'}
	if big {
		out = []byte{'M', 'M'}
	}
	out = order.AppendUint16(out, 42)
	out = order.AppendUint32(out, 8)
	out = order.AppendUint16(out, 2)
	out = order.AppendUint16(out, 0x010f)
	out = order.AppendUint16(out, 2)
	out = order.AppendUint32(out, 12)
	out = order.AppendUint32(out, makeAt)
	out = order.AppendUint16(out, 0x0112)
	out = order.AppendUint16(out, 3)
	out = order.AppendUint32(out, 1)
	out = order.AppendUint16(out, 6)
	out = append(out, 0, 0)
	out = order.AppendUint32(out, 0)
	return append(out, []byte("Canon Camera\x00")...)
}

func TestEntryByteOrderAndInlineOffset(t *testing.T) {
	for _, big := range []bool{false, true} {
		var data []byte
		var want binary.ByteOrder
		if big {
			data, want = tinyTIFF(binary.BigEndian, true, 8+2+24+4), binary.BigEndian
		} else {
			data, want = tinyTIFF(binary.LittleEndian, false, 8+2+24+4), binary.LittleEndian
		}
		r, err := NewReader(data)
		if err != nil {
			t.Fatal(err)
		}
		dir, err := r.Directory(r.FirstOffset())
		if err != nil {
			t.Fatal(err)
		}
		make, _ := dir.Find(0x010f)
		orient, _ := dir.Find(0x0112)
		if make.ByteOrder() != want || orient.ByteOrder() != want {
			t.Fatalf("byte order %v", make.ByteOrder())
		}
		if off, ok := make.InlineOffset(); !ok || off != 8+2+24+4 {
			t.Fatalf("offset %d %v", off, ok)
		}
		if _, ok := orient.InlineOffset(); ok {
			t.Fatal("a short value has no offset")
		}
	}
	if (Entry{}).ByteOrder() != binary.LittleEndian {
		t.Fatal("zero entry")
	}
	if _, ok := (Entry{}).InlineOffset(); ok {
		t.Fatal("zero entry has no offset")
	}
}
