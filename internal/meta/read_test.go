package meta

import (
	"bytes"
	"compress/zlib"
	"context"
	"errors"
	"image/png"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
)

func zlibBytes(t testing.TB, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	if _, err := zw.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func iTXtXMP(t testing.TB, xmp string, compress bool) []byte {
	t.Helper()
	data := []byte("XML:com.adobe.xmp\x00")
	if compress {
		data = append(data, 1, 0, 0, 0)
		return append(data, zlibBytes(t, []byte(xmp))...)
	}
	data = append(data, 0, 0, 0, 0)
	return append(data, xmp...)
}

func iCCPChunk(t testing.TB, profile []byte) []byte {
	t.Helper()
	data := append([]byte("sRGB\x00\x00"), zlibBytes(t, profile)...)
	return pngChunk("iCCP", data)
}

func fullJPEG(t testing.TB, e enc, below bool) ([]byte, []byte) {
	profile := sRGBProfile()
	segs := [][]byte{jpegSeg(0xe0, []byte("JFIF\x00\x01\x01\x00\x00\x01\x00\x01\x00\x00"))}
	segs = append(segs, jpegExifSeg(e.tiff(photoParts(e, below))))
	segs = append(segs, jpegICCSegs(profile, 300)...)
	segs = append(segs, jpegXMPSeg(xmpSample))
	return buildJPEG(6000, 4000, segs...), profile
}

func fullPNG(t testing.TB, e enc, below bool) ([]byte, []byte) {
	profile := sRGBProfile()
	before := [][]byte{iCCPChunk(t, profile), pngChunk("iTXt", iTXtXMP(t, xmpSample, false))}
	after := [][]byte{pngChunk("eXIf", e.tiff(photoParts(e, below)))}
	return buildPNG(t, 3, 2, before, after), profile
}

func fullWebP(t testing.TB, e enc, below bool) ([]byte, []byte) {
	profile := sRGBProfile()
	data := buildWebP(
		vp8xChunk(0x2c, 6000, 4000),
		riffChunk("ICCP", profile),
		vp8lChunk(6000, 4000),
		riffChunk("EXIF", append([]byte("Exif\x00\x00"), e.tiff(photoParts(e, below))...)),
		riffChunk("XMP ", []byte(xmpSample)),
	)
	return data, profile
}

func fullTIFF(t testing.TB, e enc, below bool) ([]byte, []byte) {
	profile := sRGBProfile()
	parts := photoParts(e, below)
	parts.ifd0 = append(parts.ifd0,
		e.long(0x0100, 6000),
		e.long(0x0101, 4000),
		e.undefined(0x8773, profile),
		e.bytesVal(0x02bc, []byte(xmpSample)...),
	)
	return e.tiff(parts), profile
}

func TestReadFullPhotoAllContainersBothByteOrders(t *testing.T) {
	cases := []struct {
		id    codec.ID
		build func(testing.TB, enc, bool) ([]byte, []byte)
		w, h  int
	}{
		{codec.JPEG, fullJPEG, 6000, 4000},
		{codec.PNG, fullPNG, 3, 2},
		{codec.WebP, fullWebP, 6000, 4000},
		{codec.TIFF, fullTIFF, 6000, 4000},
	}
	for _, c := range cases {
		for _, o := range orders {
			for _, below := range []bool{false, true} {
				name := string(c.id) + "/" + o.name
				if below {
					name += "/below"
				}
				t.Run(name, func(t *testing.T) {
					data, profile := c.build(t, enc{o.order}, below)
					m := mustRead(t, data, c.id)
					if m.Format != c.id || m.Width != c.w || m.Height != c.h {
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

func TestReadAPNGUsesPNGWalker(t *testing.T) {
	data, _ := fullPNG(t, enc{orders[0].order}, false)
	m := mustRead(t, data, codec.APNG)
	if m.Format != codec.APNG || m.Exif == nil || m.Width != 3 {
		t.Fatalf("%+v", m)
	}
}

func TestPNGWithMetadataStillDecodes(t *testing.T) {
	data, _ := fullPNG(t, enc{orders[1].order}, false)
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("chunk CRCs or layout are wrong: %v", err)
	}
	if b := img.Bounds(); b.Dx() != 3 || b.Dy() != 2 {
		t.Fatalf("bounds %v", b)
	}
}

func TestPNGExifBeforeIDATAndCompressedXMP(t *testing.T) {
	e := enc{orders[0].order}
	before := [][]byte{
		pngChunk("eXIf", append([]byte("Exif\x00\x00"), e.tiff(photoParts(e, false))...)),
		pngChunk("iTXt", iTXtXMP(t, xmpSample, true)),
	}
	m := mustRead(t, buildPNG(t, 2, 2, before, nil), codec.PNG)
	checkPhoto(t, m, false)
	checkXMPSample(t, m.XMP)
	if m.ICC != nil {
		t.Fatal("unexpected ICC")
	}
}

func TestPNGIgnoresOtherITXtKeywords(t *testing.T) {
	other := append([]byte("Comment\x00"), 0, 0, 0, 0)
	other = append(other, xmpSample...)
	m := mustRead(t, buildPNG(t, 1, 1, [][]byte{pngChunk("iTXt", other)}, nil), codec.PNG)
	if m.XMP != nil {
		t.Fatalf("XMP from a foreign keyword: %+v", m.XMP)
	}
}

func TestWebPBareVariants(t *testing.T) {
	m := mustRead(t, buildWebP(vp8lChunk(100, 50)), codec.WebP)
	if m.Width != 100 || m.Height != 50 {
		t.Fatalf("VP8L %dx%d", m.Width, m.Height)
	}
	m = mustRead(t, buildWebP(vp8Chunk(64, 48)), codec.WebP)
	if m.Width != 64 || m.Height != 48 {
		t.Fatalf("VP8 %dx%d", m.Width, m.Height)
	}
	m = mustRead(t, buildWebP(vp8xChunk(0, 16384, 1), vp8lChunk(1, 1)), codec.WebP)
	if m.Width != 16384 || m.Height != 1 {
		t.Fatalf("VP8X canvas %dx%d", m.Width, m.Height)
	}
}

func TestWebPExifWithAndWithoutPrefix(t *testing.T) {
	e := enc{orders[1].order}
	tiff := e.tiff(photoParts(e, false))
	for name, payload := range map[string][]byte{
		"prefixed": append([]byte("Exif\x00\x00"), tiff...),
		"bare":     tiff,
	} {
		m := mustRead(t, buildWebP(vp8lChunk(8, 8), riffChunk("EXIF", payload)), codec.WebP)
		if m.Exif == nil || m.Exif.Model != "Canon EOS R5" {
			t.Fatalf("%s: %+v", name, m.Exif)
		}
	}
}

func TestJPEGSegmentOrderAndDuplicates(t *testing.T) {
	e := enc{orders[0].order}
	first := e.tiff(photoParts(e, false))
	second := e.tiff(tiffParts{ifd0: []tentry{e.ascii(0x010f, "Nikon")}})
	data := buildJPEG(10, 20,
		jpegSeg(0xe1, append([]byte("http://ns.adobe.com/xmp/extension/\x00"), "ignored"...)),
		jpegXMPSeg(xmpSample),
		jpegXMPSeg(`<x:xmpmeta xmlns:x="adobe:ns:meta/"/>`),
		jpegExifSeg(first),
		jpegExifSeg(second),
	)
	m := mustRead(t, data, codec.JPEG)
	if m.Exif == nil || m.Exif.Make != "Canon" {
		t.Fatalf("first Exif must win: %+v", m.Exif)
	}
	checkXMPSample(t, m.XMP)
	if m.Width != 10 || m.Height != 20 {
		t.Fatalf("size %dx%d", m.Width, m.Height)
	}
}

func TestJPEGWithoutMetadataOnlyHasSize(t *testing.T) {
	m := mustRead(t, buildJPEG(640, 480), codec.JPEG)
	if m.Width != 640 || m.Height != 480 || m.Exif != nil || m.GPS != nil || m.ICC != nil || m.XMP != nil {
		t.Fatalf("%+v", m)
	}
}

func TestJPEGProgressiveAndOtherSOFMarkers(t *testing.T) {
	for _, marker := range []byte{0xc1, 0xc2, 0xc3, 0xc5, 0xc6, 0xc7, 0xc9, 0xca, 0xcb, 0xcd, 0xce, 0xcf} {
		sof := jpegSeg(marker, []byte{8, 0x01, 0x00, 0x02, 0x00, 1, 1, 0x11, 0})
		data := []byte{0xff, 0xd8}
		data = append(data, sof...)
		data = append(data, 0xff, 0xd9)
		m := mustRead(t, data, codec.JPEG)
		if m.Width != 512 || m.Height != 256 {
			t.Fatalf("marker %#x: %dx%d", marker, m.Width, m.Height)
		}
	}
	for _, marker := range []byte{0xc4, 0xc8, 0xcc} {
		data := []byte{0xff, 0xd8}
		data = append(data, jpegSeg(marker, []byte{8, 0x01, 0x00, 0x02, 0x00})...)
		data = append(data, 0xff, 0xd9)
		if m := mustRead(t, data, codec.JPEG); m.Width != 0 {
			t.Fatalf("marker %#x treated as SOF", marker)
		}
	}
}

func TestJPEGFillBytesAndRestartMarkers(t *testing.T) {
	e := enc{orders[0].order}
	data := []byte{0xff, 0xd8, 0xff, 0xff, 0xff, 0xd0, 0xff, 0x01}
	data = append(data, jpegExifSeg(e.tiff(photoParts(e, false)))...)
	data = append(data, 0xff, 0xd9)
	if m := mustRead(t, data, codec.JPEG); m.Exif == nil || m.Exif.Make != "Canon" {
		t.Fatalf("%+v", m.Exif)
	}
}

func TestOtherFormatsReturnFormatOnly(t *testing.T) {
	for _, id := range []codec.ID{codec.GIF, codec.BMP, codec.ICO, codec.CUR, codec.SVG, codec.AVIF, codec.HEIC, codec.JXL, codec.RAW, "nonsense", ""} {
		m, err := Read(context.Background(), bytes.NewReader([]byte("GIF89a....")), 10, id)
		if err != nil || m == nil || m.Format != id || m.Width != 0 || m.Height != 0 || m.Exif != nil || m.ICC != nil || m.XMP != nil || m.GPS != nil {
			t.Fatalf("%q: %+v err=%v", id, m, err)
		}
	}
}

func TestWrongContainerForIDIsCorrupt(t *testing.T) {
	data, _ := fullPNG(t, enc{orders[0].order}, false)
	for _, id := range []codec.ID{codec.JPEG, codec.WebP, codec.TIFF} {
		m, err := Read(context.Background(), bytes.NewReader(data), int64(len(data)), id)
		if m == nil || !errors.Is(err, codec.ErrCorrupt) {
			t.Fatalf("%s: m=%v err=%v", id, m, err)
		}
	}
	jpegData, _ := fullJPEG(t, enc{orders[0].order}, false)
	if _, err := Read(context.Background(), bytes.NewReader(jpegData), int64(len(jpegData)), codec.PNG); !errors.Is(err, codec.ErrCorrupt) {
		t.Fatalf("PNG over JPEG: %v", err)
	}
}

func TestBigTIFFIsAcceptedWithoutData(t *testing.T) {
	data := []byte{'I', 'I', 43, 0, 8, 0, 0, 0, 16, 0, 0, 0, 0, 0, 0, 0}
	m, err := Read(context.Background(), bytes.NewReader(data), int64(len(data)), codec.TIFF)
	if err != nil || m.Exif != nil || m.Width != 0 {
		t.Fatalf("%+v %v", m, err)
	}
}

func TestCancelledContext(t *testing.T) {
	data, _ := fullJPEG(t, enc{orders[0].order}, false)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	m, err := Read(ctx, bytes.NewReader(data), int64(len(data)), codec.JPEG)
	if !errors.Is(err, context.Canceled) || m == nil || m.Format != codec.JPEG {
		t.Fatalf("m=%v err=%v", m, err)
	}
}

func TestNilReaderAndNegativeSize(t *testing.T) {
	if m, err := Read(context.Background(), nil, 10, codec.JPEG); m == nil || err == nil {
		t.Fatalf("nil reader: %v %v", m, err)
	}
	if m, err := Read(context.Background(), bytes.NewReader(nil), -1, codec.JPEG); m == nil || err == nil {
		t.Fatalf("negative size: %v %v", m, err)
	}
	if m, err := Read(nil, bytes.NewReader(nil), 0, codec.GIF); m == nil || err != nil {
		t.Fatalf("nil context: %v %v", m, err)
	}
}

func TestPartialResultsSurviveTruncatedTail(t *testing.T) {
	e := enc{orders[0].order}
	data, _ := fullJPEG(t, e, false)
	cut := len(data) - 30
	m, err := Read(context.Background(), bytes.NewReader(data[:cut]), int64(cut), codec.JPEG)
	if m == nil || m.Exif == nil || !errors.Is(err, codec.ErrCorrupt) {
		t.Fatalf("metadata before the cut must survive: %+v err=%v", m, err)
	}
	checkPhoto(t, m, false)
}
