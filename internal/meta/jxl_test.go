package meta

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/isobmff/isotest"
)

func TestReadJXLContainerWithExifAndXMP(t *testing.T) {
	for _, o := range orders {
		for _, below := range []bool{false, true} {
			name := o.name
			if below {
				name += "/below"
			}
			t.Run(name, func(t *testing.T) {
				m := mustRead(t, fullJXL(t, enc{o.order}, below), codec.JXL)
				if m.Format != codec.JXL {
					t.Fatalf("format %q", m.Format)
				}
				checkPhoto(t, m, below)
				checkXMPSample(t, m.XMP)
				if m.ICC != nil || m.Width != 0 || m.Height != 0 {
					t.Fatalf("the codestream is not parsed: %+v", m)
				}
			})
		}
	}
}

func TestJXLExifBoxLayouts(t *testing.T) {
	tiff := le.tiff(photoParts(le, false))
	cases := []struct {
		name string
		box  []byte
		want bool
	}{
		{"offset 0", isotest.Box("Exif", heifExifData(tiff, 0, false)), true},
		{"offset 6 and an Exif header", isotest.Box("Exif", heifExifData(tiff, 6, true)), true},
		{"large box", isotest.LargeBox("Exif", heifExifData(tiff, 0, false)), true},
		{"open ended box", isotest.OpenEndedBox("Exif", heifExifData(tiff, 0, false)), true},
		{"garbage payload", isotest.Box("Exif", []byte("definitely not exif data")), false},
		{"empty box", isotest.Box("Exif"), false},
		{"offset field only", isotest.Box("Exif", isotest.U32(0)), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := mustRead(t, jxlContainer(c.box), codec.JXL)
			if got := m.Exif != nil && m.Exif.Model == "Canon EOS R5"; got != c.want {
				t.Fatalf("exif %+v, want data %v", m.Exif, c.want)
			}
		})
	}
}

func TestJXLFirstMetadataBoxWins(t *testing.T) {
	canon := isotest.Box("Exif", heifExifData(le.tiff(photoParts(le, false)), 0, false))
	nikon := isotest.Box("Exif", heifExifData(le.tiff(tiffParts{ifd0: []tentry{le.ascii(0x010f, "Nikon")}}), 0, false))
	other := isotest.Box("xml ", []byte(`<x:xmpmeta xmlns:x="adobe:ns:meta/"/>`))
	m := mustRead(t, jxlContainer(canon, nikon, isotest.Box("xml ", []byte(xmpSample)), other), codec.JXL)
	if m.Exif == nil || m.Exif.Make != "Canon" {
		t.Fatalf("exif %+v", m.Exif)
	}
	checkXMPSample(t, m.XMP)
}

func TestJXLCompressedAndUnrelatedBoxesAreSkipped(t *testing.T) {
	tiff := heifExifData(le.tiff(photoParts(le, false)), 0, false)
	data := jxlContainer(
		isotest.Box("jumb", []byte("JUMBF superbox")),
		isotest.Box("brob", []byte("Exif"), tiff),
		isotest.Box("brob", []byte("xml "), []byte(xmpSample)),
		isotest.Box("jbrd", tiff),
		isotest.Box("free", make([]byte, 40)),
		jxlCodestream(),
	)
	m, err := readAny(data, codec.JXL)
	if err != nil || m == nil || m.Exif != nil || m.XMP != nil || m.ICC != nil || m.GPS != nil {
		t.Fatalf("%+v %v", m, err)
	}
}

func TestJXLWithoutContainerHasNothingToRead(t *testing.T) {
	cases := map[string][]byte{
		"bare codestream":  {0xff, 0x0a, 0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b},
		"empty":            nil,
		"shorter than box": jxlSignature[:8],
		"other container":  buildJPEG(4, 4),
		"a HEIC file":      heifFile(heifOptions{exif: heifExifData(le.tiff(photoParts(le, false)), 6, true)}),
	}
	for name, data := range cases {
		m, err := readAny(data, codec.JXL)
		if err != nil || m == nil || m.Format != codec.JXL || m.Exif != nil || m.XMP != nil || m.Width != 0 {
			t.Fatalf("%s: %+v %v", name, m, err)
		}
	}
}

func TestJXLSignatureOnly(t *testing.T) {
	m, err := readAny(jxlSignature, codec.JXL)
	if err != nil || m == nil || m.Exif != nil {
		t.Fatalf("%+v %v", m, err)
	}
}

func TestJXLBrokenBoxesKeepWhatWasRead(t *testing.T) {
	exif := isotest.Box("Exif", heifExifData(le.tiff(photoParts(le, false)), 0, false))
	cases := []struct {
		name    string
		data    []byte
		corrupt bool
	}{
		{"box larger than the file", jxlContainer(exif, isotest.U32(1<<20), []byte("xml "), []byte("short")), true},
		{"cut inside a large box header", jxlContainer(exif, isotest.U32(1), []byte("xml "), []byte{0, 0}), true},
		{"box shorter than its header", jxlContainer(exif, isotest.U32(3), []byte("xml "), []byte("padding")), true},
		{"tail shorter than a header", jxlContainer(exif, []byte{0, 0, 0}), false},
	}
	for _, c := range cases {
		m, err := readAny(c.data, codec.JXL)
		if m == nil || m.Exif == nil || m.Exif.Make != "Canon" {
			t.Fatalf("%s: metadata before the break must survive: %+v", c.name, m)
		}
		if got := errors.Is(err, codec.ErrCorrupt); got != c.corrupt {
			t.Fatalf("%s: corrupt=%v, error %v", c.name, got, err)
		}
	}
}

func TestJXLTruncatedMetadataBox(t *testing.T) {
	data := fullJXL(t, le, false)
	cut := data[:bytes.Index(data, []byte("Exif"))+40]
	m, err := readAny(cut, codec.JXL)
	if m == nil || m.Exif != nil || !errors.Is(err, codec.ErrCorrupt) {
		t.Fatalf("%+v %v", m, err)
	}
}

func TestJXLOversizedBlocksAreSkipped(t *testing.T) {
	tiff := heifExifData(le.tiff(photoParts(le, false)), 0, false)
	data := jxlContainer(
		isotest.Box("Exif", tiff, make([]byte, maxExifBlock)),
		isotest.Box("xml ", []byte(xmpSample), make([]byte, MaxXMPBytes)),
	)
	m, err := readAny(data, codec.JXL)
	if err != nil || m == nil || m.Exif != nil || m.XMP != nil {
		t.Fatalf("%+v %v", m, err)
	}
	exactly := jxlContainer(isotest.Box("xml ", append([]byte(xmpSample), make([]byte, MaxXMPBytes-len(xmpSample))...)))
	if m := mustRead(t, exactly, codec.JXL); m.XMP == nil {
		t.Fatal("a packet of exactly MaxXMPBytes must be read")
	}
}

func TestJXLManyBoxesFinishQuickly(t *testing.T) {
	var boxes []byte
	for i := 0; i < maxContainerOps+10; i++ {
		boxes = append(boxes, isotest.Box("free")...)
	}
	data := jxlContainer(boxes)
	within(t, 20*time.Second, func() {
		m, err := readAny(data, codec.JXL)
		if m == nil || !errors.Is(err, codec.ErrCorrupt) {
			t.Fatalf("%+v %v", m, err)
		}
	})
}

func TestJXLCancelledContext(t *testing.T) {
	data := fullJXL(t, le, false)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	m, err := Read(ctx, bytes.NewReader(data), int64(len(data)), codec.JXL)
	if !errors.Is(err, context.Canceled) || m == nil || m.Exif != nil {
		t.Fatalf("%+v %v", m, err)
	}
	ctx, cancel = context.WithCancel(context.Background())
	reader := &cancelAfter{data: data, cancel: cancel, after: 3}
	m, err = Read(ctx, reader, int64(len(data)), codec.JXL)
	if !errors.Is(err, context.Canceled) || m == nil || m.Exif != nil {
		t.Fatalf("cancelled in the middle: %+v %v", m, err)
	}
}

func TestJXLReadCallsStayBounded(t *testing.T) {
	data := fullJXL(t, be, true)
	r := &countingReader{data: data}
	if _, err := Read(context.Background(), r, int64(len(data)), codec.JXL); err != nil {
		t.Fatal(err)
	}
	if calls := r.calls.Load(); calls > 40 {
		t.Fatalf("%d ReadAt calls", calls)
	}
}
