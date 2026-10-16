package jpegxform

import (
	"bytes"
	"encoding/binary"
	"math/rand"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/meta/exif"
)

type exifSpec struct {
	order       binary.ByteOrder
	orientation uint16
	width       uint32
	height      uint32
	pixelX      uint32
	pixelY      uint32
	pixelShort  bool
	resX, resY  uint32
	thumbnail   []byte
	noIFD0Dims  bool
}

func putEntry(order binary.ByteOrder, b []byte, tag, typ uint16, count uint32, value uint32, short bool) {
	order.PutUint16(b[0:], tag)
	order.PutUint16(b[2:], typ)
	order.PutUint32(b[4:], count)
	if short {
		order.PutUint16(b[8:], uint16(value))
		return
	}
	order.PutUint32(b[8:], value)
}

func buildTIFF(s exifSpec) []byte {
	o := s.order
	const ifd0At = 8
	n0 := 4
	if !s.noIFD0Dims {
		n0 += 2
	}
	ifd0Size := 2 + n0*12 + 4
	resAt := ifd0At + ifd0Size
	exifAt := resAt + 16
	exifSize := 2 + 2*12 + 4
	ifd1At := exifAt + exifSize
	ifd1Size := 2 + 2*12 + 4
	thumbAt := ifd1At + ifd1Size
	out := make([]byte, thumbAt+len(s.thumbnail))
	copy(out, []byte{'I', 'I'})
	if o == binary.BigEndian {
		copy(out, []byte{'M', 'M'})
	}
	o.PutUint16(out[2:], 42)
	o.PutUint32(out[4:], ifd0At)

	p := ifd0At
	o.PutUint16(out[p:], uint16(n0))
	p += 2
	if !s.noIFD0Dims {
		putEntry(o, out[p:], tagImageWidth, typeLong, 1, s.width, false)
		p += 12
		putEntry(o, out[p:], tagImageLength, typeLong, 1, s.height, false)
		p += 12
	}
	putEntry(o, out[p:], tagOrientation, typeShort, 1, uint32(s.orientation), true)
	p += 12
	putEntry(o, out[p:], tagXResolution, typeRational, 1, uint32(resAt), false)
	p += 12
	putEntry(o, out[p:], tagYResolution, typeRational, 1, uint32(resAt+8), false)
	p += 12
	putEntry(o, out[p:], tagExifPointer, typeLong, 1, uint32(exifAt), false)
	p += 12
	if len(s.thumbnail) > 0 {
		o.PutUint32(out[p:], uint32(ifd1At))
	}
	o.PutUint32(out[resAt:], s.resX)
	o.PutUint32(out[resAt+4:], 1)
	o.PutUint32(out[resAt+8:], s.resY)
	o.PutUint32(out[resAt+12:], 1)

	p = exifAt
	o.PutUint16(out[p:], 2)
	p += 2
	putEntry(o, out[p:], tagPixelXDim, map[bool]uint16{true: typeShort, false: typeLong}[s.pixelShort], 1, s.pixelX, s.pixelShort)
	p += 12
	putEntry(o, out[p:], tagPixelYDim, map[bool]uint16{true: typeShort, false: typeLong}[s.pixelShort], 1, s.pixelY, s.pixelShort)

	if len(s.thumbnail) > 0 {
		p = ifd1At
		o.PutUint16(out[p:], 2)
		p += 2
		putEntry(o, out[p:], 0x201, typeLong, 1, uint32(thumbAt), false)
		p += 12
		putEntry(o, out[p:], 0x202, typeLong, 1, uint32(len(s.thumbnail)), false)
		copy(out[thumbAt:], s.thumbnail)
	}
	return out
}

func exifSegmentOf(tiff []byte) segment {
	return segment{marker: markerAPP0 + 1, payload: append([]byte{'E', 'x', 'i', 'f', 0, 0}, tiff...)}
}

func exifFixture(t *testing.T, s exifSpec, w, h int) ([]byte, buildSpec) {
	t.Helper()
	spec := samplingSpec(w, h, [2]int{2, 2}, [2]int{1, 1}, [2]int{1, 1})
	spec.segments = []segment{exifSegmentOf(buildTIFF(s))}
	return buildJPEG(t, spec, randomWall(rand.New(rand.NewSource(12)), spec)), spec
}

func tinyJPEGThumbnail(t *testing.T) []byte {
	t.Helper()
	spec := samplingSpec(16, 16, [2]int{1, 1})
	return buildJPEG(t, spec, randomWall(rand.New(rand.NewSource(13)), spec))
}

func readBackExif(t *testing.T, data []byte) (tiff []byte, tree exif.Tree) {
	t.Helper()
	payload, err := exif.FromJPEG(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("no EXIF in the output: %v", err)
	}
	r, err := exif.NewReader(payload)
	if err != nil {
		t.Fatal(err)
	}
	tree, err = r.Tree()
	if err != nil {
		t.Fatal(err)
	}
	return payload, tree
}

func uintOf(t *testing.T, d *exif.Directory, tag uint16) uint32 {
	t.Helper()
	e, ok := d.Find(tag)
	if !ok {
		t.Fatalf("tag %#x is missing", tag)
	}
	v, ok := e.Uint(0)
	if !ok {
		t.Fatalf("tag %#x has no integer", tag)
	}
	return v
}

func rationalOf(t *testing.T, d *exif.Directory, tag uint16) uint32 {
	t.Helper()
	e, ok := d.Find(tag)
	if !ok {
		t.Fatalf("tag %#x is missing", tag)
	}
	n, den, ok := e.Rational(0)
	if !ok || den != 1 {
		t.Fatalf("tag %#x: %d/%d", tag, n, den)
	}
	return n
}

func TestRotationMakesTheEXIFOrientationNormalAndFollowsTheNewSize(t *testing.T) {
	for _, order := range []binary.ByteOrder{binary.LittleEndian, binary.BigEndian} {
		for _, short := range []bool{true, false} {
			spec := exifSpec{order: order, orientation: 6, width: 96, height: 64, pixelX: 96, pixelY: 64, pixelShort: short, resX: 300, resY: 72}
			data, _ := exifFixture(t, spec, 96, 64)

			out := mustTransform(t, data, Rotate90, Options{})

			_, tree := readBackExif(t, out)
			if got := uintOf(t, tree.IFD0, tagOrientation); got != 1 {
				t.Errorf("orientation %d", got)
			}
			if w, h := uintOf(t, tree.IFD0, tagImageWidth), uintOf(t, tree.IFD0, tagImageLength); w != 64 || h != 96 {
				t.Errorf("IFD0 size %dx%d", w, h)
			}
			if x, y := uintOf(t, tree.Exif, tagPixelXDim), uintOf(t, tree.Exif, tagPixelYDim); x != 64 || y != 96 {
				t.Errorf("pixel dimensions %dx%d", x, y)
			}
			if x, y := rationalOf(t, tree.IFD0, tagXResolution), rationalOf(t, tree.IFD0, tagYResolution); x != 72 || y != 300 {
				t.Errorf("resolution %d x %d", x, y)
			}
		}
	}
}

func TestFlippingKeepsTheResolutionOrder(t *testing.T) {
	spec := exifSpec{order: binary.LittleEndian, orientation: 2, width: 96, height: 64, pixelX: 96, pixelY: 64, resX: 300, resY: 72}
	data, _ := exifFixture(t, spec, 96, 64)

	out := mustTransform(t, data, FlipH, Options{})

	_, tree := readBackExif(t, out)
	if x, y := rationalOf(t, tree.IFD0, tagXResolution), rationalOf(t, tree.IFD0, tagYResolution); x != 300 || y != 72 {
		t.Errorf("resolution %d x %d", x, y)
	}
	if w := uintOf(t, tree.IFD0, tagImageWidth); w != 96 {
		t.Errorf("width %d", w)
	}
}

func TestTrimmedOutputReportsItsTrimmedSizeInEXIF(t *testing.T) {
	spec := exifSpec{order: binary.BigEndian, orientation: 1, width: 101, height: 75, pixelX: 101, pixelY: 75, resX: 72, resY: 72}
	data, _ := exifFixture(t, spec, 101, 75)

	out := mustTransform(t, data, Rotate90, Options{Trim: true})

	_, tree := readBackExif(t, out)
	if x, y := uintOf(t, tree.Exif, tagPixelXDim), uintOf(t, tree.Exif, tagPixelYDim); x != 64 || y != 101 {
		t.Errorf("pixel dimensions %dx%d, want 64x101", x, y)
	}
}

func TestEXIFWithOtherDimensionsIsLeftAlone(t *testing.T) {
	spec := exifSpec{order: binary.LittleEndian, orientation: 6, width: 4000, height: 3000, pixelX: 4000, pixelY: 3000, resX: 72, resY: 72}
	data, _ := exifFixture(t, spec, 96, 64)

	out := mustTransform(t, data, Rotate90, Options{})

	_, tree := readBackExif(t, out)
	if w, x := uintOf(t, tree.IFD0, tagImageWidth), uintOf(t, tree.Exif, tagPixelXDim); w != 4000 || x != 4000 {
		t.Errorf("foreign dimensions were rewritten: %d, %d", w, x)
	}
}

func TestThumbnailIsDroppedWhenThePictureChanges(t *testing.T) {
	thumb := tinyJPEGThumbnail(t)
	tests := []struct {
		name        string
		orientation uint16
		op          Op
		dropped     bool
	}{
		{"rotated", 1, Rotate90, true},
		{"upright by the transform", 6, Rotate90, true},
		{"orientation reset only", 6, Identity, true},
		{"nothing changes", 1, Identity, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			spec := exifSpec{order: binary.LittleEndian, orientation: tc.orientation, width: 96, height: 64, pixelX: 96, pixelY: 64, resX: 72, resY: 72, thumbnail: thumb}
			data, _ := exifFixture(t, spec, 96, 64)
			if _, ok := exif.JPEGThumbnail(mustEXIF(t, data)); !ok {
				t.Fatal("the fixture has no thumbnail")
			}

			out := mustTransform(t, data, tc.op, Options{})

			_, has := exif.JPEGThumbnail(mustEXIF(t, out))
			if has == tc.dropped {
				t.Fatalf("thumbnail present: %v", has)
			}
		})
	}
}

func mustEXIF(t *testing.T, data []byte) []byte {
	t.Helper()
	payload, err := exif.FromJPEG(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func TestUntouchedEXIFIsCopiedByteForByte(t *testing.T) {
	thumb := tinyJPEGThumbnail(t)
	spec := exifSpec{order: binary.BigEndian, orientation: 1, width: 96, height: 64, pixelX: 96, pixelY: 64, resX: 72, resY: 72, thumbnail: thumb}
	data, _ := exifFixture(t, spec, 96, 64)

	out := mustTransform(t, data, Identity, Options{})

	if !bytes.Equal(mustEXIF(t, out), mustEXIF(t, data)) {
		t.Fatal("the EXIF block changed although the picture did not")
	}
}

func TestEXIFOrientationReadByTheViewerIsNormalAfterEveryOperation(t *testing.T) {
	for o := codec.OrientNormal; o <= codec.OrientRotate270; o++ {
		spec := exifSpec{order: binary.LittleEndian, orientation: uint16(o), width: 96, height: 64, pixelX: 96, pixelY: 64, resX: 72, resY: 72}
		data, _ := exifFixture(t, spec, 96, 64)

		out := mustTransform(t, data, FromOrientation(o), Options{})

		got, ok := exif.Orientation(mustEXIF(t, out))
		if !ok || got != codec.OrientNormal {
			t.Errorf("orientation %d -> %d, %v", o, got, ok)
		}
	}
}

func TestDamagedEXIFIsCopiedWithoutFailing(t *testing.T) {
	good := buildTIFF(exifSpec{order: binary.LittleEndian, orientation: 6, width: 96, height: 64, pixelX: 96, pixelY: 64, resX: 72, resY: 72})
	variants := map[string][]byte{
		"empty":            {},
		"short":            good[:5],
		"bad order":        append([]byte{'X', 'X'}, good[2:]...),
		"bad magic":        append(append([]byte{}, good[:2]...), append([]byte{0, 0}, good[4:]...)...),
		"cut inside IFD0":  good[:30],
		"cut inside value": good[:90],
		"IFD0 far away":    append(append(append([]byte{}, good[:4]...), 0xFF, 0xFF, 0xFF, 0x7F), good[8:]...),
	}
	for name, tiff := range variants {
		t.Run(name, func(t *testing.T) {
			spec := samplingSpec(96, 64, [2]int{1, 1}, [2]int{1, 1}, [2]int{1, 1})
			spec.segments = []segment{exifSegmentOf(tiff), {marker: markerAPP0 + 1, payload: []byte{'E', 'x', 'i', 'f', 0}}}
			data := buildJPEG(t, spec, randomWall(rand.New(rand.NewSource(14)), spec))

			if _, err := runTransform(data, Rotate90, Options{}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func xmpSegment(body string) segment {
	return segment{marker: markerAPP0 + 1, payload: append([]byte(xmpPrefix), body...)}
}

func TestXMPOrientationBecomesNormalInPlace(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"attribute", `<rdf:Description tiff:Orientation="6" x="1"/>`, `<rdf:Description tiff:Orientation="1" x="1"/>`},
		{"attribute single quotes", `<rdf:Description tiff:Orientation='8'/>`, `<rdf:Description tiff:Orientation='1'/>`},
		{"element", `<tiff:Orientation>3</tiff:Orientation>`, `<tiff:Orientation>1</tiff:Orientation>`},
		{"two places", `a tiff:Orientation="5" b <tiff:Orientation>7</tiff:Orientation>`, `a tiff:Orientation="1" b <tiff:Orientation>1</tiff:Orientation>`},
		{"unrelated digit stays", `<x:Other>6</x:Other> tiff:OrientationX="6"`, `<x:Other>6</x:Other> tiff:OrientationX="6"`},
		{"not a digit", `tiff:Orientation="x"`, `tiff:Orientation="x"`},
		{"cut off", `tiff:Orientation="`, `tiff:Orientation="`},
		{"cut off element", `<tiff:Orientation>`, `<tiff:Orientation>`},
		{"two digits stay", `tiff:Orientation="61"`, `tiff:Orientation="61"`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			spec := samplingSpec(96, 64, [2]int{1, 1}, [2]int{1, 1}, [2]int{1, 1})
			spec.segments = []segment{xmpSegment(tc.in)}
			data := buildJPEG(t, spec, randomWall(rand.New(rand.NewSource(15)), spec))

			out := mustTransform(t, data, Rotate90, Options{})

			h, _ := parseCoefficients(t, out)
			got := string(h.segments[0].payload[len(xmpPrefix):])
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestMetadataSegmentsKeepTheirOrderAndMultiPictureDataIsDropped(t *testing.T) {
	spec := samplingSpec(96, 64, [2]int{2, 2}, [2]int{1, 1}, [2]int{1, 1})
	icc := append([]byte("ICC_PROFILE\x00\x01\x01"), bytes.Repeat([]byte{7}, 300)...)
	spec.segments = []segment{
		{marker: markerAPP0, payload: []byte{'J', 'F', 'I', 'F', 0, 1, 1, 0, 0, 1, 0, 1, 0, 0}},
		{marker: markerCOM, payload: []byte("first comment")},
		{marker: markerAPP0 + 2, payload: icc},
		{marker: markerAPP0 + 2, payload: []byte{'M', 'P', 'F', 0, 1, 2, 3}},
		{marker: markerAPP0 + 14, payload: []byte{'A', 'd', 'o', 'b', 'e', 0, 100, 0, 0, 0, 0, 1}},
		{marker: markerCOM, payload: []byte("second comment")},
	}
	data := buildJPEG(t, spec, randomWall(rand.New(rand.NewSource(16)), spec))

	out := mustTransform(t, data, Rotate180, Options{})

	h, _ := parseCoefficients(t, out)
	want := []segment{spec.segments[0], spec.segments[1], spec.segments[2], spec.segments[4], spec.segments[5]}
	if len(h.segments) != len(want) {
		t.Fatalf("%d segments, want %d", len(h.segments), len(want))
	}
	for i, s := range h.segments {
		if s.marker != want[i].marker || !bytes.Equal(s.payload, want[i].payload) {
			t.Errorf("segment %d is %#x %q", i, s.marker, s.payload)
		}
	}
}
