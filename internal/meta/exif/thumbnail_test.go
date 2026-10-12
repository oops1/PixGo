package exif

import (
	"bytes"
	"encoding/binary"
	"testing"
)

type thumbSpec struct {
	bigEndian   bool
	compression int
	data        []byte
	offsetShift int
	lengthShift int
	noIFD1      bool
	noLength    bool
}

func buildThumbTIFF(spec thumbSpec) []byte {
	var order binary.AppendByteOrder = binary.LittleEndian
	out := []byte{'I', 'I'}
	if spec.bigEndian {
		order = binary.BigEndian
		out = []byte{'M', 'M'}
	}
	out = order.AppendUint16(out, 42)
	out = order.AppendUint32(out, 8)
	out = order.AppendUint16(out, 1)
	out = order.AppendUint16(out, TagOrientation)
	out = order.AppendUint16(out, uint16(TypeShort))
	out = order.AppendUint32(out, 1)
	out = order.AppendUint16(out, 1)
	out = order.AppendUint16(out, 0)
	if spec.noIFD1 {
		return order.AppendUint32(out, 0)
	}
	out = order.AppendUint32(out, uint32(len(out)+4))
	entries := 2
	if spec.compression >= 0 {
		entries++
	}
	if spec.noLength {
		entries--
	}
	ifdStart := len(out)
	dataStart := ifdStart + 2 + entries*12 + 4
	out = order.AppendUint16(out, uint16(entries))
	if spec.compression >= 0 {
		out = order.AppendUint16(out, thumbCompressionTag)
		out = order.AppendUint16(out, uint16(TypeShort))
		out = order.AppendUint32(out, 1)
		out = order.AppendUint16(out, uint16(spec.compression))
		out = order.AppendUint16(out, 0)
	}
	out = order.AppendUint16(out, thumbOffsetTag)
	out = order.AppendUint16(out, uint16(TypeLong))
	out = order.AppendUint32(out, 1)
	out = order.AppendUint32(out, uint32(dataStart+spec.offsetShift))
	if !spec.noLength {
		out = order.AppendUint16(out, thumbLengthTag)
		out = order.AppendUint16(out, uint16(TypeLong))
		out = order.AppendUint32(out, 1)
		out = order.AppendUint32(out, uint32(len(spec.data)+spec.lengthShift))
	}
	out = order.AppendUint32(out, 0)
	return append(out, spec.data...)
}

func fakeJPEG(n int) []byte {
	data := make([]byte, n)
	data[0], data[1] = 0xff, 0xd8
	for i := 2; i < n; i++ {
		data[i] = byte(i)
	}
	return data
}

func TestJPEGThumbnail(t *testing.T) {
	payload := fakeJPEG(300)
	cases := []struct {
		name string
		spec thumbSpec
		ok   bool
	}{
		{"little endian with compression 6", thumbSpec{compression: 6, data: payload}, true},
		{"big endian with compression 6", thumbSpec{bigEndian: true, compression: 6, data: payload}, true},
		{"compression 7", thumbSpec{compression: 7, data: payload}, true},
		{"no compression tag", thumbSpec{compression: -1, data: payload}, true},
		{"uncompressed thumbnail", thumbSpec{compression: 1, data: payload}, false},
		{"no IFD1", thumbSpec{compression: 6, data: payload, noIFD1: true}, false},
		{"missing length", thumbSpec{compression: 6, data: payload, noLength: true}, false},
		{"length past the end", thumbSpec{compression: 6, data: payload, lengthShift: 1}, false},
		{"offset past the end", thumbSpec{compression: 6, data: payload, offsetShift: 1000}, false},
		{"not a JPEG", thumbSpec{compression: 6, data: bytes.Repeat([]byte{0x11}, 64)}, false},
		{"too short", thumbSpec{compression: 6, data: []byte{0xff, 0xd8}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tiff := buildThumbTIFF(tc.spec)
			got, ok := JPEGThumbnail(tiff)
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v", ok, tc.ok)
			}
			if tc.ok && !bytes.Equal(got, tc.spec.data) {
				t.Fatalf("thumbnail bytes differ: got %d bytes, want %d", len(got), len(tc.spec.data))
			}
		})
	}
}

func TestJPEGThumbnailRejectsGarbage(t *testing.T) {
	for _, in := range [][]byte{nil, {}, []byte("II*"), bytes.Repeat([]byte{0xff}, 64)} {
		if _, ok := JPEGThumbnail(in); ok {
			t.Fatalf("garbage %q accepted", in)
		}
	}
}

func TestJPEGThumbnailResultIsBounded(t *testing.T) {
	tiff := buildThumbTIFF(thumbSpec{compression: 6, data: fakeJPEG(100)})
	got, ok := JPEGThumbnail(tiff)
	if !ok {
		t.Fatal("thumbnail not found")
	}
	if cap(got) != len(got) {
		t.Fatalf("cap %d != len %d", cap(got), len(got))
	}
}
