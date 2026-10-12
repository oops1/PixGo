package meta

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oops1/PixGo/internal/codec"
)

func within(t *testing.T, limit time.Duration, f func()) {
	t.Helper()
	done := make(chan any, 1)
	go func() {
		defer func() { done <- recover() }()
		f()
	}()
	select {
	case r := <-done:
		if r != nil {
			t.Fatalf("panic: %v", r)
		}
	case <-time.After(limit):
		t.Fatalf("not finished within %v", limit)
	}
}

func isPanic(err error) bool {
	return err != nil && strings.HasPrefix(err.Error(), "panic:")
}

var allIDs = []codec.ID{codec.JPEG, codec.PNG, codec.APNG, codec.WebP, codec.TIFF, codec.GIF}

type sample struct {
	name string
	id   codec.ID
	data []byte
}

func smallSamples(t *testing.T) []sample {
	t.Helper()
	var out []sample
	for _, o := range orders {
		e := enc{o.order}
		j, _ := fullJPEG(t, e, false)
		p, _ := fullPNG(t, e, true)
		w, _ := fullWebP(t, e, false)
		tf, _ := fullTIFF(t, e, true)
		out = append(out,
			sample{"jpeg/" + o.name, codec.JPEG, j},
			sample{"png/" + o.name, codec.PNG, p},
			sample{"webp/" + o.name, codec.WebP, w},
			sample{"tiff/" + o.name, codec.TIFF, tf},
		)
	}
	return out
}

func TestEveryTruncationIsSafe(t *testing.T) {
	for _, s := range smallSamples(t) {
		within(t, 10*time.Second, func() {
			full, err := readAny(s.data, s.id)
			if err != nil || full == nil || full.Exif == nil {
				t.Fatalf("%s: complete sample must parse: %v", s.name, err)
			}
			for n := 0; n <= len(s.data); n++ {
				prefix := s.data[:n]
				m, err := readAny(prefix, s.id)
				if m == nil || isPanic(err) {
					t.Fatalf("%s/%d: m=%v err=%v", s.name, n, m, err)
				}
				if n%11 == 0 {
					for _, other := range allIDs {
						if _, err := readAny(prefix, other); isPanic(err) {
							t.Fatalf("%s/%d as %s: %v", s.name, n, other, err)
						}
					}
				}
			}
		})
	}
}

func TestTruncationNeverInventsData(t *testing.T) {
	data, _ := fullJPEG(t, le, false)
	full := mustRead(t, data, codec.JPEG)
	for n := 0; n < len(data); n++ {
		m, _ := readAny(data[:n], codec.JPEG)
		if m == nil {
			t.Fatalf("%d: nil", n)
		}
		if m.Exif != nil && m.Exif.Model != full.Exif.Model {
			t.Fatalf("%d: model %q", n, m.Exif.Model)
		}
		if m.ICC != nil && m.ICC.Size != full.ICC.Size {
			t.Fatalf("%d: partial ICC accepted (%d bytes)", n, m.ICC.Size)
		}
	}
}

func loopTIFF(e enc) []byte {
	buf := e.header()
	return e.writeIFD(buf, []tentry{
		e.short(0x0112, 3),
		e.long(0x8769, 8),
		e.long(0x8825, 8),
	}, 8)
}

func TestIFDLoopsDoNotHang(t *testing.T) {
	for _, e := range []enc{le, be} {
		tiff := loopTIFF(e)
		cases := []sample{
			{"tiff", codec.TIFF, tiff},
			{"jpeg", codec.JPEG, buildJPEG(4, 4, jpegExifSeg(tiff))},
			{"png", codec.PNG, buildPNG(t, 1, 1, [][]byte{pngChunk("eXIf", tiff)}, nil)},
			{"webp", codec.WebP, buildWebP(vp8lChunk(2, 2), riffChunk("EXIF", tiff))},
		}
		within(t, 10*time.Second, func() {
			for _, c := range cases {
				m := mustRead(t, c.data, c.id)
				if m.Exif == nil || m.Exif.Orientation != codec.OrientRotate180 || m.GPS != nil {
					t.Fatalf("%s: %+v %+v", c.name, m.Exif, m.GPS)
				}
			}
		})
	}
}

func TestPointersOutsideTheBlock(t *testing.T) {
	for _, off := range []uint32{0, 1, 7, 9, 0xffff, 0x7fffffff, 0xfffffffe, 0xffffffff} {
		buf := le.header()
		buf = le.writeIFD(buf, []tentry{
			le.short(0x0112, 6),
			le.long(0x8769, off),
			le.long(0x8825, off),
			le.ascii(0x010f, "Canon"),
		}, off)
		m := mustRead(t, buf, codec.TIFF)
		if m.Exif == nil || m.Exif.Orientation != codec.OrientRotate90 || m.GPS != nil {
			t.Fatalf("offset %#x: %+v", off, m.Exif)
		}
	}
	for _, first := range []uint32{0, 2, 0xfffffff0, 0xffffffff} {
		buf := le.header()
		binary.LittleEndian.PutUint32(buf[4:], first)
		buf = le.writeIFD(buf, []tentry{le.short(0x0112, 6)}, 0)
		m, err := readAny(buf, codec.TIFF)
		if m == nil || isPanic(err) {
			t.Fatalf("first IFD %#x: %v", first, err)
		}
	}
}

func TestHugeDeclaredCounts(t *testing.T) {
	huge := func(tag uint16, typ uint16, off uint32) tentry {
		return tentry{tag: tag, typ: typ, count: 0xffffffff, data: le.o.AppendUint32(nil, off)}
	}
	tiff := le.tiff(tiffParts{
		ifd0: []tentry{
			huge(0x010f, 2, 8),
			huge(0x0110, 2, 0),
			huge(0x0131, 1, 0xfffffff0),
			huge(0x8773, 7, 8),
			huge(0x02bc, 1, 8),
			le.short(0x0112, 8),
		},
		exif: []tentry{
			huge(0x829a, 5, 8),
			huge(0x829d, 10, 8),
			huge(0x8827, 3, 8),
			huge(0x9003, 2, 8),
			huge(0xa434, 2, 8),
			le.short(0x9207, 2),
		},
		gps: []tentry{
			huge(0x0002, 5, 8),
			huge(0x0004, 5, 8),
			huge(0x0001, 2, 8),
		},
	})
	m := mustRead(t, tiff, codec.TIFF)
	if m.Exif == nil || m.Exif.Orientation != codec.OrientRotate270 || m.Exif.MeteringMode != 2 {
		t.Fatalf("%+v", m.Exif)
	}
	if m.Exif.Make != "" || m.Exif.Exposure.Valid() || m.Exif.ISO != 0 || m.GPS != nil || m.ICC != nil || m.XMP != nil {
		t.Fatalf("%+v %+v %+v %+v", m.Exif, m.GPS, m.ICC, m.XMP)
	}
	jpeg := buildJPEG(4, 4, jpegExifSeg(tiff))
	if m := mustRead(t, jpeg, codec.JPEG); m.Exif == nil || m.Exif.Orientation != codec.OrientRotate270 {
		t.Fatalf("%+v", m.Exif)
	}
}

func TestIFDWithThousandsOfEntries(t *testing.T) {
	buf := le.header()
	buf = binary.LittleEndian.AppendUint16(buf, 65535)
	buf = append(buf, make([]byte, 65535*12+4)...)
	m, err := readAny(buf, codec.TIFF)
	if m == nil || m.Exif != nil || isPanic(err) {
		t.Fatalf("%+v %v", m, err)
	}
	buf = le.header()
	buf = binary.LittleEndian.AppendUint16(buf, 1024)
	for i := 0; i < 1024; i++ {
		buf = le.o.AppendUint16(buf, 0xf000+uint16(i))
		buf = le.o.AppendUint16(buf, 3)
		buf = le.o.AppendUint32(buf, 1)
		buf = append(buf, 1, 0, 0, 0)
	}
	buf = append(buf, 0, 0, 0, 0)
	if m := mustRead(t, buf, codec.TIFF); m == nil {
		t.Fatal("1024 entries must be accepted")
	}
}

func TestHeaderOnlyContainers(t *testing.T) {
	cases := map[codec.ID][]byte{
		codec.JPEG: {0xff, 0xd8},
		codec.PNG:  {0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'},
		codec.WebP: []byte("RIFF\x04\x00\x00\x00WEBP"),
		codec.TIFF: {'I', 'I', 42, 0, 8, 0, 0, 0},
	}
	for id, data := range cases {
		m, err := readAny(data, id)
		if m == nil || m.Exif != nil || m.ICC != nil || m.XMP != nil || isPanic(err) {
			t.Fatalf("%s: %+v %v", id, m, err)
		}
	}
}

type countingReader struct {
	data  []byte
	calls atomic.Int64
}

func (c *countingReader) ReadAt(p []byte, off int64) (int, error) {
	c.calls.Add(1)
	if off >= int64(len(c.data)) {
		return 0, io.EOF
	}
	n := copy(p, c.data[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func TestReadCallsStayBounded(t *testing.T) {
	for _, s := range smallSamples(t) {
		r := &countingReader{data: s.data}
		if _, err := Read(context.Background(), r, int64(len(s.data)), s.id); err != nil {
			t.Fatalf("%s: %v", s.name, err)
		}
		if calls := r.calls.Load(); calls > 80 {
			t.Fatalf("%s: %d ReadAt calls", s.name, calls)
		}
	}
}

type brokenReader struct {
	data      []byte
	failAfter int64
	short     bool
	calls     int64
}

func (b *brokenReader) ReadAt(p []byte, off int64) (int, error) {
	b.calls++
	if b.calls > b.failAfter {
		return 0, errors.New("disk on fire")
	}
	if b.short && len(p) > 1 {
		return copy(p[:len(p)-1], b.data[off:]), nil
	}
	return copy(p, b.data[off:]), nil
}

func TestFailingAndMisbehavingReaders(t *testing.T) {
	for _, s := range smallSamples(t) {
		for failAfter := int64(0); failAfter < 12; failAfter++ {
			r := &brokenReader{data: s.data, failAfter: failAfter}
			within(t, 10*time.Second, func() {
				m, _ := Read(context.Background(), r, int64(len(s.data)), s.id)
				if m == nil {
					t.Fatalf("%s: nil metadata", s.name)
				}
			})
		}
		short := &brokenReader{data: s.data, failAfter: 1 << 30, short: true}
		m, err := Read(context.Background(), short, int64(len(s.data)), s.id)
		if m == nil {
			t.Fatalf("%s: nil metadata", s.name)
		}
		if err == nil && m.Exif != nil {
			t.Fatalf("%s: short reads must not yield data", s.name)
		}
		m, _ = Read(context.Background(), bytes.NewReader(s.data), int64(len(s.data))+5000, s.id)
		if m == nil {
			t.Fatalf("%s: nil metadata with inflated size", s.name)
		}
	}
}

func TestHostileContainerLoopsFinishQuickly(t *testing.T) {
	fills := append([]byte{0xff, 0xd8}, bytes.Repeat([]byte{0xff}, 1<<20)...)
	segs := []byte{0xff, 0xd8}
	for i := 0; i < 40000; i++ {
		segs = append(segs, jpegSeg(0xef, []byte{1})...)
	}
	chunks := append([]byte(nil), pngSignature...)
	for i := 0; i < 300000; i++ {
		chunks = append(chunks, pngChunk("tEXt", nil)...)
	}
	var riff []byte
	for i := 0; i < 300000; i++ {
		riff = append(riff, riffChunk("JUNK", nil)...)
	}
	hugePNG := append(append([]byte(nil), pngSignature...), 0xff, 0xff, 0xff, 0xff, 'i', 'C', 'C', 'P')
	hugeWebP := []byte("RIFF\xff\xff\xff\xffWEBPEXIF\xff\xff\xff\xff")
	cases := []sample{
		{"fill bytes", codec.JPEG, fills},
		{"many segments", codec.JPEG, segs},
		{"many chunks", codec.PNG, chunks},
		{"many riff", codec.WebP, buildWebP(riff)},
		{"giant png", codec.PNG, hugePNG},
		{"giant webp", codec.WebP, hugeWebP},
		{"zero length", codec.JPEG, []byte{0xff, 0xd8, 0xff, 0xe1, 0, 0}},
		{"length one", codec.JPEG, []byte{0xff, 0xd8, 0xff, 0xe1, 0, 1}},
	}
	for _, c := range cases {
		within(t, 10*time.Second, func() {
			m, err := readAny(c.data, c.id)
			if m == nil || isPanic(err) {
				t.Fatalf("%s: m=%v err=%v", c.name, m, err)
			}
			if !errors.Is(err, codec.ErrCorrupt) {
				t.Fatalf("%s: %v does not wrap ErrCorrupt", c.name, err)
			}
		})
	}
}

type zeroTail struct {
	head []byte
}

func (z zeroTail) ReadAt(p []byte, off int64) (int, error) {
	for i := range p {
		pos := off + int64(i)
		if pos < int64(len(z.head)) {
			p[i] = z.head[pos]
		} else {
			p[i] = 0
		}
	}
	return len(p), nil
}

func TestEnormousVirtualFiles(t *testing.T) {
	const size = int64(1) << 40
	huge := func(tag uint16, typ uint16, count uint32) tentry {
		return tentry{tag: tag, typ: typ, count: count, data: le.o.AppendUint32(nil, 4096)}
	}
	tiff := le.tiff(tiffParts{ifd0: []tentry{
		huge(0x8773, 7, MaxICCBytes),
		huge(0x02bc, 1, MaxXMPBytes),
		huge(0x010f, 2, 1<<20),
		le.long(0x0100, 100000),
		le.long(0x0101, 50000),
	}})
	png := append(append([]byte(nil), pngSignature...), pngChunk("IHDR", []byte{0, 0, 0, 1, 0, 0, 0, 1, 8, 6, 0, 0, 0})...)
	png = append(png, 0x40, 0x00, 0x00, 0x00, 'i', 'T', 'X', 't')
	cases := []sample{
		{"tiff", codec.TIFF, tiff},
		{"png", codec.PNG, png},
		{"jpeg", codec.JPEG, []byte{0xff, 0xd8, 0xff, 0xe1, 0xff, 0xff}},
	}
	for _, c := range cases {
		within(t, 30*time.Second, func() {
			m, _ := Read(context.Background(), zeroTail{head: c.data}, size, c.id)
			if m == nil {
				t.Fatalf("%s: nil", c.name)
			}
			if c.name == "tiff" && (m.Width != 100000 || m.Height != 50000 || m.ICC != nil) {
				t.Fatalf("%+v", m)
			}
		})
	}
}

func TestStressRandomBitFlips(t *testing.T) {
	seed := uint32(12345)
	next := func() uint32 {
		seed = seed*1664525 + 1013904223
		return seed
	}
	for _, s := range smallSamples(t) {
		within(t, 30*time.Second, func() {
			for round := 0; round < 400; round++ {
				mutated := append([]byte(nil), s.data...)
				for k := 0; k < 1+int(next()%6); k++ {
					mutated[int(next()%uint32(len(mutated)))] ^= byte(next() >> 8)
				}
				if m, err := readAny(mutated, s.id); m == nil || isPanic(err) {
					t.Fatalf("%s round %d: %v", s.name, round, err)
				}
			}
		})
	}
}
