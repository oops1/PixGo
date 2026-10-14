package meta

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/isobmff/isotest"
)

func TestReadHEIFAndAVIFFullPhoto(t *testing.T) {
	cases := []struct {
		id       codec.ID
		itemType string
	}{
		{codec.HEIC, "hvc1"},
		{codec.AVIF, "av01"},
	}
	for _, c := range cases {
		for _, o := range orders {
			for _, below := range []bool{false, true} {
				name := string(c.id) + "/" + o.name
				if below {
					name += "/below"
				}
				t.Run(name, func(t *testing.T) {
					data, profile := fullHEIF(t, enc{o.order}, below, c.itemType)
					m := mustRead(t, data, c.id)
					if m.Format != c.id || m.Width != 6000 || m.Height != 4000 {
						t.Fatalf("format %q %dx%d", m.Format, m.Width, m.Height)
					}
					checkPhoto(t, m, below)
					checkICC(t, m.ICC, len(profile))
					checkXMPSample(t, m.XMP)
				})
			}
		}
	}
}

func TestHEIFExifOffsetLayouts(t *testing.T) {
	tiff := le.tiff(photoParts(le, false))
	cases := []struct {
		name string
		data []byte
		want bool
	}{
		{"offset 6 after the Exif header", heifExifData(tiff, 6, true), true},
		{"offset 0 and no header", heifExifData(tiff, 0, false), true},
		{"offset 0 but the Exif header is present", heifExifData(tiff, 0, true), true},
		{"offset points into padding", append(isotest.U32(4), append([]byte{9, 9, 9, 9}, tiff...)...), true},
		{"offset beyond the data falls back to the start", heifExifData(tiff, 0xffffffff, false), true},
		{"offset in the middle of the TIFF falls back to the start", heifExifData(tiff, 10, false), true},
		{"no TIFF header at all", heifExifData([]byte("not a tiff block, just text"), 0, false), false},
		{"payload shorter than a TIFF header", append(isotest.U32(0), 'I', 'I', 42), false},
		{"only the offset field", isotest.U32(0), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := mustRead(t, heifFile(heifOptions{exif: c.data}), codec.HEIC)
			if got := m.Exif != nil && m.Exif.Model == "Canon EOS R5"; got != c.want {
				t.Fatalf("exif %+v, want data %v", m.Exif, c.want)
			}
		})
	}
}

func TestHEIFColorProfiles(t *testing.T) {
	profile := sRGBProfile()
	cases := []struct {
		name string
		opts heifOptions
		want bool
	}{
		{"prof", heifOptions{profile: profile}, true},
		{"rICC", heifOptions{profile: profile, kind: "rICC"}, true},
		{"nclx only", heifOptions{props: []isotest.Prop{isotest.Colr()}}, false},
		{"unknown colour type", heifOptions{profile: profile, kind: "abcd"}, false},
		{"header without a profile", heifOptions{profile: []byte{}}, false},
		{"profile that is not an ICC profile", heifOptions{profile: bytes.Repeat([]byte{1}, 400)}, false},
		{"profile after an nclx box", heifOptions{props: []isotest.Prop{isotest.Colr(), isotest.ColrProfile("prof", profile)}}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := mustRead(t, heifFile(c.opts), codec.HEIC)
			if (m.ICC != nil) != c.want {
				t.Fatalf("ICC %+v, want present=%v", m.ICC, c.want)
			}
			if c.want {
				checkICC(t, m.ICC, len(profile))
			}
		})
	}
}

func TestHEIFColorProfileOfATileOfAGrid(t *testing.T) {
	profile := sRGBProfile()
	f := isotest.File{
		Major:      "heic",
		Compatible: []string{"mif1", "heic"},
		Primary:    1,
		Items: []isotest.Item{
			{ID: 1, Type: "grid", Data: isotest.GridDescriptor(1, 2, 64, 32, false), Props: []isotest.Prop{isotest.Ispe(64, 32)}},
			{ID: 2, Type: "hvc1", Data: []byte("tile"), Props: []isotest.Prop{isotest.Ispe(32, 32), isotest.ColrProfile("prof", profile)}},
			{ID: 3, Type: "hvc1", Data: []byte("tile"), Props: []isotest.Prop{isotest.Ispe(32, 32)}},
		},
		Refs: []isotest.Ref{{Type: "dimg", From: 1, To: []uint32{2, 3}}},
	}
	m := mustRead(t, f.Bytes(), codec.HEIC)
	if m.Width != 64 || m.Height != 32 {
		t.Fatalf("size %dx%d", m.Width, m.Height)
	}
	checkICC(t, m.ICC, len(profile))
	f.Items[1].Props = f.Items[1].Props[:1]
	if m := mustRead(t, f.Bytes(), codec.HEIC); m.ICC != nil {
		t.Fatalf("ICC from nowhere: %+v", m.ICC)
	}
}

func TestHEIFPrefersMetadataThatDescribesThePrimaryItem(t *testing.T) {
	canon := heifExifData(le.tiff(photoParts(le, false)), 6, true)
	nikon := heifExifData(le.tiff(tiffParts{ifd0: []tentry{le.ascii(0x010f, "Nikon")}}), 6, true)
	otherXMP := `<x:xmpmeta xmlns:x="adobe:ns:meta/"><rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"><rdf:Description xmlns:xmp="http://ns.adobe.com/xap/1.0/" xmp:Rating="1"/></rdf:RDF></x:xmpmeta>`
	f := isotest.File{
		Major:      "heic",
		Compatible: []string{"heic"},
		Primary:    1,
		Items: []isotest.Item{
			{ID: 1, Type: "hvc1", Data: []byte("pixels"), Props: []isotest.Prop{isotest.Ispe(8, 8)}},
			{ID: 2, Type: "hvc1", Data: []byte("pixels"), Props: []isotest.Prop{isotest.Ispe(8, 8)}},
			{ID: 3, Type: "Exif", Data: nikon},
			{ID: 4, Type: "mime", ContentType: "Application/RDF+XML", Data: []byte(otherXMP)},
			{ID: 5, Type: "Exif", Data: canon},
			{ID: 6, Type: "mime", ContentType: heifXMPContentType, Data: []byte(xmpSample)},
			{ID: 7, Type: "mime", ContentType: "text/plain", Data: []byte("note")},
		},
		Refs: []isotest.Ref{
			{Type: "cdsc", From: 3, To: []uint32{2}},
			{Type: "cdsc", From: 4, To: []uint32{2}},
			{Type: "cdsc", From: 5, To: []uint32{1}},
			{Type: "cdsc", From: 6, To: []uint32{1, 2}},
		},
	}
	m := mustRead(t, f.Bytes(), codec.HEIC)
	if m.Exif == nil || m.Exif.Make != "Canon" {
		t.Fatalf("exif %+v", m.Exif)
	}
	checkXMPSample(t, m.XMP)

	f.Refs = nil
	m = mustRead(t, f.Bytes(), codec.HEIC)
	if m.Exif == nil || m.Exif.Make != "Nikon" || m.XMP == nil || m.XMP.Rating != 1 {
		t.Fatalf("without links the first items win: %+v %+v", m.Exif, m.XMP)
	}
}

func TestHEIFWithoutMetadataOrPrimaryItem(t *testing.T) {
	m := mustRead(t, heifFile(heifOptions{}), codec.AVIF)
	if m.Width != 6000 || m.Height != 4000 || m.Exif != nil || m.GPS != nil || m.ICC != nil || m.XMP != nil {
		t.Fatalf("%+v", m)
	}
	f := isotest.File{
		Major:      "heic",
		Compatible: []string{"heic"},
		Items:      []isotest.Item{{ID: 1, Type: "Exif", Data: heifExifData(le.tiff(photoParts(le, false)), 6, true)}},
	}
	m = mustRead(t, f.Bytes(), codec.HEIC)
	if m.Width != 0 || m.Exif == nil || m.Exif.Model != "Canon EOS R5" {
		t.Fatalf("a file without pitm still has its Exif: %+v", m)
	}
}

func TestHEIFSizeFromPrimaryPicture(t *testing.T) {
	for _, c := range []struct{ w, h uint32 }{{1, 1}, {4032, 3024}, {1 << 20, 3}} {
		f := isotest.File{
			Major: "avif", Compatible: []string{"avif"}, Primary: 1,
			Items: []isotest.Item{{ID: 1, Type: "av01", Data: []byte("x"), Props: []isotest.Prop{isotest.Ispe(c.w, c.h)}}},
		}
		got := mustRead(t, f.Bytes(), codec.AVIF)
		if got.Width != int(c.w) || got.Height != int(c.h) {
			t.Fatalf("%dx%d: %dx%d", c.w, c.h, got.Width, got.Height)
		}
	}
}

func TestHEIFRealSamplesCarryNoMetadata(t *testing.T) {
	cases := []struct {
		glob string
		id   codec.ID
	}{
		{"../codec/avif/testdata/*.avif", codec.AVIF},
		{"../codec/heic/testdata/*.heic", codec.HEIC},
		{"../codec/jxl/testdata/*.jxl", codec.JXL},
	}
	for _, c := range cases {
		files, err := filepath.Glob(c.glob)
		if err != nil || len(files) == 0 {
			t.Fatalf("%s: %v %d files", c.glob, err, len(files))
		}
		for _, name := range files {
			data, err := os.ReadFile(name)
			if err != nil {
				t.Fatal(err)
			}
			m := mustRead(t, data, c.id)
			if m.Exif != nil || m.GPS != nil || m.ICC != nil || m.XMP != nil {
				t.Fatalf("%s: %+v", name, m)
			}
			if c.id != codec.JXL && (m.Width <= 0 || m.Height <= 0) {
				t.Fatalf("%s: size %dx%d", name, m.Width, m.Height)
			}
		}
	}
}

func TestHEIFMetadataAddedToRealPhotos(t *testing.T) {
	cases := []struct {
		name string
		id   codec.ID
	}{
		{"../codec/heic/testdata/basic.heic", codec.HEIC},
		{"../codec/avif/testdata/grid-2x2.avif", codec.AVIF},
		{"../codec/avif/testdata/yuv420-8.avif", codec.AVIF},
	}
	for _, c := range cases {
		t.Run(filepath.Base(c.name), func(t *testing.T) {
			raw, err := os.ReadFile(c.name)
			if err != nil {
				t.Fatal(err)
			}
			base := mustRead(t, raw, c.id)
			parsed, err := isotest.Parse(raw)
			if err != nil {
				t.Fatal(err)
			}
			profile := sRGBProfile()
			next := uint32(len(parsed.Items) + 1)
			parsed.Items[0].Props = append(parsed.Items[0].Props, isotest.ColrProfile("prof", profile))
			parsed.Items = append(parsed.Items,
				isotest.Item{ID: next, Type: "Exif", Hidden: true, Data: heifExifData(be.tiff(photoParts(be, false)), 6, true)},
				isotest.Item{ID: next + 1, Type: "mime", ContentType: heifXMPContentType, Hidden: true, Data: []byte(xmpSample)},
			)
			parsed.Refs = append(parsed.Refs,
				isotest.Ref{Type: "cdsc", From: next, To: []uint32{parsed.Primary}},
				isotest.Ref{Type: "cdsc", From: next + 1, To: []uint32{parsed.Primary}},
			)
			m := mustRead(t, parsed.Bytes(), c.id)
			if m.Width != base.Width || m.Height != base.Height {
				t.Fatalf("size %dx%d, was %dx%d", m.Width, m.Height, base.Width, base.Height)
			}
			checkPhoto(t, m, false)
			checkICC(t, m.ICC, len(profile))
			checkXMPSample(t, m.XMP)
		})
	}
}

func TestHEIFSignatureDecidesWhetherThereIsAnythingToRead(t *testing.T) {
	cases := map[string][]byte{
		"empty":             nil,
		"shorter than ftyp": []byte("\x00\x00\x00\x08ftyp"),
		"a JPEG":            buildJPEG(4, 4),
		"ftyp not first":    append(isotest.Box("free", []byte("12345678")), isotest.Box("ftyp", []byte("heic"))...),
	}
	for name, data := range cases {
		for _, id := range []codec.ID{codec.HEIC, codec.AVIF} {
			m, err := readAny(data, id)
			if err != nil || m == nil || m.Exif != nil || m.Width != 0 {
				t.Fatalf("%s as %s: %+v %v", name, id, m, err)
			}
		}
	}
}

func TestHEIFBrokenContainersReportCorruption(t *testing.T) {
	good, _ := fullHEIF(t, le, false, "hvc1")
	ftyp := isotest.Box("ftyp", []byte("heic"), isotest.U32(0), []byte("heic"))
	cases := map[string][]byte{
		"meta cut near its start":        good[:len(ftyp)+20],
		"meta cut in the middle":         good[:len(ftyp)+len(good)/8],
		"meta box larger than the file":  concat(ftyp, isotest.U32(1<<20), []byte("meta"), isotest.U32(0)),
		"large meta box beyond the file": concat(ftyp, isotest.U32(1), []byte("meta"), isotest.U64(64<<20)),
	}
	for name, data := range cases {
		m, err := readAny(data, codec.HEIC)
		if m == nil || !errors.Is(err, codec.ErrCorrupt) || m.Exif != nil {
			t.Fatalf("%s: %+v %v", name, m, err)
		}
	}
	m, err := readAny(ftyp, codec.HEIC)
	if m == nil || err != nil || m.Exif != nil {
		t.Fatalf("a file with only ftyp has nothing to read: %+v %v", m, err)
	}
}

func TestHEIFBrokenExifKeepsTheRest(t *testing.T) {
	profile := sRGBProfile()
	f := isotest.File{
		Major:      "heic",
		Compatible: []string{"heic"},
		Primary:    1,
		Items: []isotest.Item{
			{ID: 1, Type: "hvc1", Data: []byte("pixels"), Props: []isotest.Prop{isotest.Ispe(10, 20), isotest.ColrProfile("prof", profile)}},
			{ID: 2, Type: "Exif", NoData: true},
			{ID: 3, Type: "mime", ContentType: heifXMPContentType, Data: []byte(xmpSample)},
		},
		Refs: []isotest.Ref{{Type: "cdsc", From: 2, To: []uint32{1}}},
	}
	m, err := readAny(f.Bytes(), codec.HEIC)
	if !errors.Is(err, codec.ErrCorrupt) || m == nil || m.Exif != nil {
		t.Fatalf("%+v %v", m, err)
	}
	checkICC(t, m.ICC, len(profile))
	checkXMPSample(t, m.XMP)
	if m.Width != 10 || m.Height != 20 {
		t.Fatalf("size %dx%d", m.Width, m.Height)
	}

	f.Items[2].NoData = true
	m, err = readAny(f.Bytes(), codec.HEIC)
	if !errors.Is(err, codec.ErrCorrupt) || m == nil || m.XMP != nil || m.ICC == nil {
		t.Fatalf("broken XMP: %+v %v", m, err)
	}
}

func TestHEIFOversizedBlocksAreSkippedNotCut(t *testing.T) {
	tiff := le.tiff(photoParts(le, false))
	hugeExif := append(heifExifData(tiff, 0, false), make([]byte, maxExifBlock)...)
	hugeXMP := xmpSample + string(make([]byte, MaxXMPBytes))
	hugeProfile := append(sRGBProfile(), make([]byte, MaxICCBytes)...)
	data := heifFile(heifOptions{exif: hugeExif, xmp: hugeXMP, profile: hugeProfile})
	m, err := readAny(data, codec.HEIC)
	if err != nil || m == nil || m.Exif != nil || m.XMP != nil || m.ICC != nil {
		t.Fatalf("%+v %v", m, err)
	}
	if m.Width != 6000 {
		t.Fatalf("size %dx%d", m.Width, m.Height)
	}
}

func TestHEIFExtentOutsideTheFile(t *testing.T) {
	data := heifFile(heifOptions{exif: heifExifData(le.tiff(photoParts(le, false)), 6, true), xmp: xmpSample})
	parsed, err := isotest.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	parsed.Items[1].InIdat = true
	m := mustRead(t, parsed.Bytes(), codec.HEIC)
	if m.Exif == nil || m.Exif.Model != "Canon EOS R5" {
		t.Fatalf("Exif in idat: %+v", m.Exif)
	}
	openEnded := append([]byte(nil), data...)
	copy(openEnded[bytes.LastIndex(openEnded, []byte("mdat"))-4:], isotest.U32(0))
	cut := openEnded[:len(openEnded)-len(xmpSample)/2]
	m, err = readAny(cut, codec.HEIC)
	if m == nil || !errors.Is(err, codec.ErrCorrupt) || m.Exif == nil || m.XMP != nil {
		t.Fatalf("cut tail: %+v %v", m, err)
	}
}

func TestHEIFCancelledContext(t *testing.T) {
	data, _ := fullHEIF(t, le, false, "hvc1")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, id := range []codec.ID{codec.HEIC, codec.AVIF} {
		m, err := Read(ctx, bytes.NewReader(data), int64(len(data)), id)
		if !errors.Is(err, context.Canceled) || m == nil || m.Format != id || m.Exif != nil {
			t.Fatalf("%s: %+v %v", id, m, err)
		}
	}
}

func TestHEIFCancelledWhileReading(t *testing.T) {
	data, _ := fullHEIF(t, le, false, "hvc1")
	ctx, cancel := context.WithCancel(context.Background())
	reader := &cancelAfter{data: data, cancel: cancel, after: 3}
	m, err := Read(ctx, reader, int64(len(data)), codec.HEIC)
	if !errors.Is(err, context.Canceled) || m == nil || m.Exif != nil {
		t.Fatalf("%+v %v", m, err)
	}
}

type cancelAfter struct {
	data   []byte
	cancel context.CancelFunc
	after  int
	calls  int
}

func (c *cancelAfter) ReadAt(p []byte, off int64) (int, error) {
	c.calls++
	if c.calls >= c.after {
		c.cancel()
	}
	if off >= int64(len(c.data)) {
		return 0, errors.New("EOF")
	}
	n := copy(p, c.data[off:])
	if n < len(p) {
		return n, errors.New("EOF")
	}
	return n, nil
}

func TestHEIFReadCallsStayBounded(t *testing.T) {
	data, _ := fullHEIF(t, le, false, "hvc1")
	r := &countingReader{data: data}
	if _, err := Read(context.Background(), r, int64(len(data)), codec.HEIC); err != nil {
		t.Fatal(err)
	}
	if calls := r.calls.Load(); calls > 40 {
		t.Fatalf("%d ReadAt calls", calls)
	}
}

func TestHEIFWithManyItemsFinishesQuickly(t *testing.T) {
	f := isotest.File{Major: "heic", Compatible: []string{"heic"}, Primary: 1}
	f.Items = append(f.Items, isotest.Item{ID: 1, Type: "hvc1", Data: []byte("p"), Props: []isotest.Prop{isotest.Ispe(2, 2)}})
	for i := 2; i < 3000; i++ {
		f.Items = append(f.Items, isotest.Item{ID: uint32(i), Type: "Exif", Data: heifExifData(le.tiff(tiffParts{ifd0: []tentry{le.ascii(0x010f, "Nikon")}}), 6, true)})
	}
	data := f.Bytes()
	within(t, 10*time.Second, func() {
		m := mustRead(t, data, codec.HEIC)
		if m.Exif == nil || m.Exif.Make != "Nikon" {
			t.Fatalf("%+v", m.Exif)
		}
	})
}
