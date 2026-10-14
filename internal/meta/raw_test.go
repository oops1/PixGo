package meta

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"testing"
	"time"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/isobmff/isotest"
)

func TestReadRAWTIFFVariants(t *testing.T) {
	cases := []struct {
		name  string
		magic uint16
	}{
		{"CR2 NEF ARW DNG PEF", 42},
		{"ORF", 0x4f52},
		{"ORF second kind", 0x5352},
		{"RW2", 0x0055},
	}
	for _, c := range cases {
		for _, o := range orders {
			for _, below := range []bool{false, true} {
				name := c.name + "/" + o.name
				if below {
					name += "/below"
				}
				t.Run(name, func(t *testing.T) {
					e := enc{o.order}
					data := rawTIFF(e, c.magic, rawParts(t, e, below))
					m := mustRead(t, data, codec.RAW)
					if m.Format != codec.RAW {
						t.Fatalf("format %q", m.Format)
					}
					checkPhoto(t, m, below)
					checkICC(t, m.ICC, 0)
					checkXMPSample(t, m.XMP)
					if m.Width != 0 || m.Height != 0 {
						t.Fatalf("the size of IFD0 must not be reported for RAW: %dx%d", m.Width, m.Height)
					}
				})
			}
		}
	}
}

func TestRAWTIFFWithoutExtrasOnlyHasTheCameraBlock(t *testing.T) {
	parts := photoParts(le, false)
	m := mustRead(t, rawTIFF(le, 42, parts), codec.RAW)
	checkPhoto(t, m, false)
	if m.ICC != nil || m.XMP != nil {
		t.Fatalf("%+v %+v", m.ICC, m.XMP)
	}
	m = mustRead(t, rawTIFF(be, 42, tiffParts{ifd0: []tentry{be.ascii(0x010f, "NIKON CORPORATION"), be.ascii(0x0110, "NIKON D850")}}), codec.RAW)
	if m.Exif == nil || m.Exif.Make != "NIKON CORPORATION" || m.Exif.Model != "NIKON D850" || m.GPS != nil {
		t.Fatalf("%+v", m.Exif)
	}
}

func TestRAWTIFFWithNoGPSOrExifBlocks(t *testing.T) {
	parts := photoParts(le, false)
	parts.gps = nil
	m := mustRead(t, rawTIFF(le, 0x4f52, parts), codec.RAW)
	if m.GPS != nil || m.Exif == nil || m.Exif.ISO != 100 {
		t.Fatalf("%+v %+v", m.GPS, m.Exif)
	}
	parts.exif = nil
	m = mustRead(t, rawTIFF(le, 0x55, parts), codec.RAW)
	if m.Exif == nil || m.Exif.Make != "Canon" || m.Exif.ISO != 0 {
		t.Fatalf("%+v", m.Exif)
	}
}

func TestRAWTIFFWithAnUnknownMagicHasNothingToRead(t *testing.T) {
	for _, magic := range []uint16{0, 1, 43, 0x1234, 0x4f53, 0x0054, 0xffff} {
		data := rawTIFF(le, magic, photoParts(le, false))
		m, err := readAny(data, codec.RAW)
		if err != nil || m == nil || m.Exif != nil || m.GPS != nil {
			t.Fatalf("magic %#x: %+v %v", magic, m, err)
		}
	}
}

func TestRAWUnknownSignaturesHaveNothingToRead(t *testing.T) {
	cases := map[string][]byte{
		"empty":                nil,
		"one byte":             {'I'},
		"a JPEG":               buildJPEG(4, 4, jpegExifSeg(le.tiff(photoParts(le, false)))),
		"text":                 []byte("this is definitely not a raw photo file"),
		"short TIFF header":    {'I', 'I', 42, 0, 8, 0, 0},
		"ftyp of another kind": concat(isotest.Box("ftyp", []byte("heic"), isotest.U32(0), []byte("heic")), isotest.Box("free", make([]byte, 20))),
		"FUJI without a dash":  []byte("FUJIFILMCCD RAW 0201"),
	}
	for name, data := range cases {
		m, err := readAny(data, codec.RAW)
		if err != nil || m == nil || m.Format != codec.RAW || m.Exif != nil || m.XMP != nil || m.ICC != nil {
			t.Fatalf("%s: %+v %v", name, m, err)
		}
	}
}

func TestReadCR3(t *testing.T) {
	for _, o := range orders {
		for _, below := range []bool{false, true} {
			name := o.name
			if below {
				name += "/below"
			}
			t.Run(name, func(t *testing.T) {
				m := mustRead(t, cr3File(enc{o.order}, below, cr3Options{}), codec.RAW)
				checkPhoto(t, m, below)
				checkXMPSample(t, m.XMP)
				if m.ICC != nil || m.Width != 0 {
					t.Fatalf("%+v", m)
				}
			})
		}
	}
}

func TestCR3MissingBlocks(t *testing.T) {
	cases := []struct {
		name  string
		opts  cr3Options
		check func(*testing.T, *Metadata)
	}{
		{"no CMT1", cr3Options{skipCMT1: true}, func(t *testing.T, m *Metadata) {
			if m.Exif == nil || m.Exif.Make != "" || m.Exif.ISO != 100 || m.Exif.Exposure.String() != "1/250" || m.GPS == nil {
				t.Fatalf("%+v %+v", m.Exif, m.GPS)
			}
		}},
		{"no CMT2", cr3Options{skipCMT2: true}, func(t *testing.T, m *Metadata) {
			if m.Exif == nil || m.Exif.Model != "Canon EOS R5" || m.Exif.ISO != 0 || m.GPS == nil {
				t.Fatalf("%+v %+v", m.Exif, m.GPS)
			}
		}},
		{"no CMT4", cr3Options{skipCMT4: true}, func(t *testing.T, m *Metadata) {
			if m.Exif == nil || m.Exif.ISO != 100 || m.GPS != nil {
				t.Fatalf("%+v %+v", m.Exif, m.GPS)
			}
		}},
		{"CMT2 is not a TIFF block", cr3Options{badCMT2: true}, func(t *testing.T, m *Metadata) {
			if m.Exif == nil || m.Exif.Make != "Canon" || m.Exif.ISO != 0 || m.GPS == nil {
				t.Fatalf("%+v %+v", m.Exif, m.GPS)
			}
		}},
		{"no Canon box", cr3Options{noCanon: true}, func(t *testing.T, m *Metadata) {
			if m.Exif != nil || m.GPS != nil || m.XMP == nil {
				t.Fatalf("%+v %+v %+v", m.Exif, m.GPS, m.XMP)
			}
		}},
		{"no XMP", cr3Options{noXMP: true}, func(t *testing.T, m *Metadata) {
			if m.XMP != nil || m.Exif == nil {
				t.Fatalf("%+v %+v", m.XMP, m.Exif)
			}
		}},
		{"only GPS", cr3Options{skipCMT1: true, skipCMT2: true}, func(t *testing.T, m *Metadata) {
			if m.GPS == nil || m.Exif != nil && m.Exif.Make != "" {
				t.Fatalf("%+v %+v", m.GPS, m.Exif)
			}
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m, err := readAny(cr3File(le, false, c.opts), codec.RAW)
			if err != nil || m == nil {
				t.Fatalf("%+v %v", m, err)
			}
			c.check(t, m)
		})
	}
}

func TestCR3FirstBlocksWin(t *testing.T) {
	canon := cr3File(le, false, cr3Options{noXMP: true})
	second := isotest.Box("uuid", concat(cr3CanonUUID, isotest.Box("CMT1", le.tiff(tiffParts{ifd0: []tentry{le.ascii(0x010f, "Nikon")}}))))
	nested := concat(
		isotest.Box("moov", isotest.Box("mvhd", make([]byte, 100)), second),
		isotest.Box("uuid", cr3XMPUUID, []byte(xmpSample)),
		isotest.Box("uuid", cr3XMPUUID, []byte(`<x:xmpmeta xmlns:x="adobe:ns:meta/"/>`)),
	)
	data := concat(canon, nested)
	m := mustRead(t, data, codec.RAW)
	if m.Exif == nil || m.Exif.Make != "Canon" {
		t.Fatalf("%+v", m.Exif)
	}
	checkXMPSample(t, m.XMP)
}

func TestCR3BrokenBoxesKeepWhatWasRead(t *testing.T) {
	good := cr3File(le, false, cr3Options{})
	moovEnd := bytes.Index(good, []byte("mvhd")) + 100
	cases := []struct {
		name string
		data []byte
	}{
		{"cut in the middle of the file", good[:len(good)-40]},
		{"huge box after the metadata", concat(good[:len(good)-72], isotest.U32(1<<30), []byte("mdat"), make([]byte, 8))},
	}
	for _, c := range cases {
		m, err := readAny(c.data, codec.RAW)
		if m == nil || m.Exif == nil || m.Exif.Make != "Canon" || !errors.Is(err, codec.ErrCorrupt) {
			t.Fatalf("%s: %+v %v", c.name, m, err)
		}
	}
	for _, cut := range []int{moovEnd, moovEnd + 30} {
		m, err := readAny(good[:cut], codec.RAW)
		if m == nil || m.Exif != nil || !errors.Is(err, codec.ErrCorrupt) {
			t.Fatalf("cut inside moov at %d: %+v %v", cut, m, err)
		}
	}
}

func TestCR3CorruptCanonBoxStillAllowsXMP(t *testing.T) {
	canon := concat(cr3CanonUUID, isotest.Box("CMT1", le.tiff(photoParts(le, false))), isotest.U32(1<<30), []byte("CMT2"))
	data := concat(
		[]byte(cr3Header),
		isotest.Box("moov", isotest.Box("uuid", canon)),
		isotest.Box("uuid", cr3XMPUUID, []byte(xmpSample)),
	)
	m, err := readAny(data, codec.RAW)
	if m == nil || !errors.Is(err, codec.ErrCorrupt) || m.Exif == nil || m.Exif.Make != "Canon" {
		t.Fatalf("%+v %v", m, err)
	}
	checkXMPSample(t, m.XMP)
}

func TestCR3OversizedBlocksAreSkipped(t *testing.T) {
	big := le.tiff(tiffParts{ifd0: []tentry{le.ascii(0x010f, "Canon"), le.undefined(0xf000, make([]byte, maxCMTBytes))}})
	canon := concat(cr3CanonUUID, isotest.Box("CMT1", big))
	data := concat(
		[]byte(cr3Header),
		isotest.Box("moov", isotest.Box("uuid", canon)),
		isotest.Box("uuid", cr3XMPUUID, []byte(xmpSample), make([]byte, MaxXMPBytes)),
	)
	m, err := readAny(data, codec.RAW)
	if err != nil || m == nil || m.Exif != nil || m.XMP != nil {
		t.Fatalf("%+v %v", m, err)
	}
}

func TestCR3ShortUUIDBoxesAreIgnored(t *testing.T) {
	data := concat(
		[]byte(cr3Header),
		isotest.Box("moov", isotest.Box("uuid", []byte("short"))),
		isotest.Box("uuid", []byte("tiny")),
	)
	m, err := readAny(data, codec.RAW)
	if err != nil || m == nil || m.Exif != nil || m.XMP != nil {
		t.Fatalf("%+v %v", m, err)
	}
}

func TestReadRAF(t *testing.T) {
	for _, o := range orders {
		for _, below := range []bool{false, true} {
			name := o.name
			if below {
				name += "/below"
			}
			t.Run(name, func(t *testing.T) {
				data, profile := fullRAF(t, enc{o.order}, below)
				m := mustRead(t, data, codec.RAW)
				checkPhoto(t, m, below)
				checkICC(t, m.ICC, len(profile))
				checkXMPSample(t, m.XMP)
				if m.Width != 0 || m.Height != 0 {
					t.Fatalf("preview size leaked: %dx%d", m.Width, m.Height)
				}
			})
		}
	}
}

func TestRAFPreviewWindow(t *testing.T) {
	jpeg, _ := fullJPEG(t, le, false)
	n := uint32(len(jpeg))
	cases := []struct {
		name    string
		data    []byte
		corrupt bool
		exif    bool
	}{
		{"offset inside the header", rafFile(jpeg, 90, n), true, false},
		{"offset beyond the file", rafFile(jpeg, 1<<30, n), true, false},
		{"length beyond the file", rafFile(jpeg, 160, n+1000), true, false},
		{"length of one byte", rafFile(jpeg, 160, 1), true, false},
		{"zero length", rafFile(jpeg, 160, 0), true, false},
		{"huge offset and length", rafFile(jpeg, 0xffffffff, 0xffffffff), true, false},
		{"preview is not a JPEG", rafFile(bytes.Repeat([]byte{7}, 400), 160, 400), true, false},
		{"preview cut inside a segment", rafFile(jpeg[:200], 160, 200), true, false},
		{"preview shorter than declared but inside the file", rafFile(jpeg, 160, n-30), true, true},
		{"header cut", rafFile(nil, 0, 0)[:80], true, false},
	}
	for _, c := range cases {
		m, err := readAny(c.data, codec.RAW)
		if m == nil || isPanic(err) {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got := errors.Is(err, codec.ErrCorrupt); got != c.corrupt {
			t.Fatalf("%s: corrupt=%v %v", c.name, got, err)
		}
		if got := m.Exif != nil; got != c.exif {
			t.Fatalf("%s: exif present=%v", c.name, got)
		}
	}
}

func TestRAFPreviewMayLieAnywhere(t *testing.T) {
	jpeg, _ := fullJPEG(t, be, false)
	head := rafFile(nil, 0, 0)[:160]
	pad := make([]byte, 5000)
	data := concat(head, pad, jpeg, pad)
	binary.BigEndian.PutUint32(data[84:], uint32(len(head)+len(pad)))
	binary.BigEndian.PutUint32(data[88:], uint32(len(jpeg)))
	m := mustRead(t, data, codec.RAW)
	checkPhoto(t, m, false)
}

func TestRAWIFDLoopsDoNotHang(t *testing.T) {
	for _, e := range []enc{le, be} {
		for _, magic := range []uint16{42, 0x4f52, 0x55} {
			tiff := patchMagic(e, loopTIFF(e), magic)
			within(t, 10*time.Second, func() {
				m := mustRead(t, tiff, codec.RAW)
				if m.Exif == nil || m.Exif.Orientation != codec.OrientRotate180 || m.GPS != nil {
					t.Fatalf("magic %#x: %+v %+v", magic, m.Exif, m.GPS)
				}
			})
		}
	}
}

func TestRAWHugeCountsAndWildPointers(t *testing.T) {
	huge := func(tag uint16, typ uint16, off uint32) tentry {
		return tentry{tag: tag, typ: typ, count: 0xffffffff, data: le.o.AppendUint32(nil, off)}
	}
	tiff := rawTIFF(le, 0x4f52, tiffParts{
		ifd0: []tentry{
			huge(0x010f, 2, 8),
			huge(0x8773, 7, 8),
			huge(0x02bc, 1, 0xfffffff0),
			le.short(0x0112, 8),
		},
		exif: []tentry{huge(0x829a, 5, 8), huge(0x9003, 2, 8), le.short(0x9207, 2)},
		gps:  []tentry{huge(0x0002, 5, 8), huge(0x0004, 5, 8)},
	})
	m := mustRead(t, tiff, codec.RAW)
	if m.Exif == nil || m.Exif.Orientation != codec.OrientRotate270 || m.Exif.MeteringMode != 2 || m.GPS != nil || m.ICC != nil || m.XMP != nil {
		t.Fatalf("%+v", m)
	}
	for _, off := range []uint32{0, 1, 7, 9, 0xffff, 0x7fffffff, 0xffffffff} {
		buf := le.header()
		buf = le.writeIFD(buf, []tentry{le.short(0x0112, 6), le.long(0x8769, off), le.long(0x8825, off)}, off)
		if m, err := readAny(patchMagic(le, buf, 0x55), codec.RAW); m == nil || isPanic(err) || m.GPS != nil {
			t.Fatalf("offset %#x: %v", off, err)
		}
	}
}

func TestRAWEnormousVirtualFile(t *testing.T) {
	const size = int64(1) << 40
	huge := func(tag uint16, typ uint16, count uint32) tentry {
		return tentry{tag: tag, typ: typ, count: count, data: le.o.AppendUint32(nil, 4096)}
	}
	tiff := rawTIFF(le, 42, tiffParts{ifd0: []tentry{
		huge(0x8773, 7, MaxICCBytes),
		huge(0x02bc, 1, MaxXMPBytes),
		huge(0x010f, 2, 1<<20),
		le.long(0x0100, 100000),
		le.long(0x0101, 50000),
	}})
	within(t, 30*time.Second, func() {
		m, _ := Read(context.Background(), zeroTail{head: tiff}, size, codec.RAW)
		if m == nil || m.Width != 0 || m.Height != 0 || m.ICC != nil {
			t.Fatalf("%+v", m)
		}
	})
	cr3 := concat([]byte(cr3Header), isotest.U32(0), []byte("mdat"))
	within(t, 30*time.Second, func() {
		if m, err := Read(context.Background(), zeroTail{head: cr3}, size, codec.RAW); m == nil || isPanic(err) {
			t.Fatalf("%+v %v", m, err)
		}
	})
}

func TestRAWManyBoxesFinishQuickly(t *testing.T) {
	var boxes []byte
	for i := 0; i < maxCR3Boxes+10; i++ {
		boxes = append(boxes, isotest.Box("free")...)
	}
	cases := map[string][]byte{
		"top level": concat([]byte(cr3Header), boxes),
		"in moov":   concat([]byte(cr3Header), isotest.Box("moov", boxes)),
		"in Canon":  concat([]byte(cr3Header), isotest.Box("moov", isotest.Box("uuid", cr3CanonUUID, boxes))),
	}
	for name, data := range cases {
		within(t, 10*time.Second, func() {
			m, err := readAny(data, codec.RAW)
			if m == nil || !errors.Is(err, codec.ErrCorrupt) {
				t.Fatalf("%s: %+v %v", name, m, err)
			}
		})
	}
}

func TestRAWCancelledContext(t *testing.T) {
	e := le
	raf, _ := fullRAF(t, e, false)
	samples := []sample{
		{"tiff", codec.RAW, rawTIFF(e, 42, rawParts(t, e, false))},
		{"cr3", codec.RAW, cr3File(e, false, cr3Options{})},
		{"raf", codec.RAW, raf},
	}
	for _, s := range samples {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		m, err := Read(ctx, bytes.NewReader(s.data), int64(len(s.data)), s.id)
		if !errors.Is(err, context.Canceled) || m == nil || m.Exif != nil {
			t.Fatalf("%s: %+v %v", s.name, m, err)
		}
		ctx, cancel = context.WithCancel(context.Background())
		reader := &cancelAfter{data: s.data, cancel: cancel, after: 2}
		m, err = Read(ctx, reader, int64(len(s.data)), s.id)
		if !errors.Is(err, context.Canceled) || m == nil {
			t.Fatalf("%s cancelled while reading: %+v %v", s.name, m, err)
		}
	}
}

func TestRAWReadCallsStayBounded(t *testing.T) {
	for _, s := range newContainerSamples(t) {
		r := &countingReader{data: s.data}
		if _, err := Read(context.Background(), r, int64(len(s.data)), s.id); err != nil {
			t.Fatalf("%s: %v", s.name, err)
		}
		if calls := r.calls.Load(); calls > 120 {
			t.Fatalf("%s: %d ReadAt calls", s.name, calls)
		}
	}
}
