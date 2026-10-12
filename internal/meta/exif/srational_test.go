package exif

import (
	"encoding/binary"
	"testing"
)

func TestSRationalBothByteOrders(t *testing.T) {
	for _, order := range []testOrder{binary.LittleEndian, binary.BigEndian} {
		b := newTIFFBuilder(order)
		values := b.long(0xffffffff, 3, 5, 0, 7, 0xfffffffe)
		b.ifd([]rawEntry{{tag: 0x9204, typ: TypeSRational, count: 3, value: values}}, 0)
		r, err := NewReader(b.buf)
		if err != nil {
			t.Fatal(err)
		}
		dir, _ := r.Directory(r.FirstOffset())
		e, _ := dir.Find(0x9204)
		want := [3][2]int32{{-1, 3}, {5, 0}, {7, -2}}
		for i, w := range want {
			n, d, ok := e.SRational(i)
			if !ok || n != w[0] || d != w[1] {
				t.Fatalf("%v value %d: %d/%d ok=%v", order, i, n, d, ok)
			}
		}
		if _, _, ok := e.SRational(3); ok {
			t.Fatal("index past count must fail")
		}
		if _, _, ok := e.SRational(-1); ok {
			t.Fatal("negative index must fail")
		}
		if _, _, ok := e.Rational(0); ok {
			t.Fatal("Rational must reject SRATIONAL")
		}
	}
}

func TestSRationalRejectsOtherTypesAndBadOffsets(t *testing.T) {
	b := newTIFFBuilder(binary.LittleEndian)
	b.ifd([]rawEntry{
		{tag: 0x829a, typ: TypeRational, count: 1, value: b.long(1, 2)},
		{tag: 0x9204, typ: TypeSRational, count: 1, value: b.long(1, 2)},
	}, 0)
	data := append([]byte(nil), b.buf...)
	r, _ := NewReader(data)
	dir, _ := r.Directory(r.FirstOffset())
	rational, _ := dir.Find(0x829a)
	if _, _, ok := rational.SRational(0); ok {
		t.Fatal("SRational must reject RATIONAL")
	}
	binary.LittleEndian.PutUint32(data[8+2+12+8:], 0xfffffff0)
	r, _ = NewReader(data)
	dir, _ = r.Directory(r.FirstOffset())
	signed, _ := dir.Find(0x9204)
	if _, _, ok := signed.SRational(0); ok {
		t.Fatal("out-of-range offset must not resolve")
	}
}

func TestRawMaxHonoursItsLimit(t *testing.T) {
	b := newTIFFBuilder(binary.LittleEndian)
	payload := make([]byte, 3000)
	for i := range payload {
		payload[i] = byte(i)
	}
	b.ifd([]rawEntry{
		{tag: 0x8773, typ: TypeUndefined, count: uint32(len(payload)), value: payload},
		{tag: 0x0100, typ: TypeByte, count: 2, value: []byte{7, 8}},
		{tag: 0x0101, typ: TypeLong, count: 0xffffffff, value: b.long(8)},
	}, 0)
	r, err := NewReader(b.buf)
	if err != nil {
		t.Fatal(err)
	}
	dir, _ := r.Directory(r.FirstOffset())
	big, _ := dir.Find(0x8773)
	if got, ok := big.RawMax(4096); !ok || len(got) != 3000 || got[2999] != byte(2999%256) {
		t.Fatalf("RawMax(4096): %d bytes ok=%v", len(got), ok)
	}
	if _, ok := big.RawMax(2999); ok {
		t.Fatal("RawMax must refuse values above the limit")
	}
	if got, ok := big.Raw(); !ok || len(got) != 3000 {
		t.Fatal("Raw keeps working for values below its own cap")
	}
	small, _ := dir.Find(0x0100)
	if got, ok := small.RawMax(2); !ok || len(got) != 2 || got[0] != 7 {
		t.Fatalf("inline RawMax: %v ok=%v", got, ok)
	}
	if _, ok := small.RawMax(1); ok {
		t.Fatal("inline value above limit accepted")
	}
	hostile, _ := dir.Find(0x0101)
	if _, ok := hostile.RawMax(1 << 40); ok {
		t.Fatal("4G entries must not resolve even with a huge limit")
	}
}
