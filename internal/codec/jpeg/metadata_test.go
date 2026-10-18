package jpeg

import (
	"bytes"
	"context"
	"image"
	stdjpeg "image/jpeg"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/testimg"
	"github.com/oops1/PixGo/internal/meta"
)

func encodeWithMetadata(t testing.TB, img *image.NRGBA, m codec.Metadata) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := (Encoder{}).Encode(&buf, img, codec.EncodeOptions{Quality: 90, Metadata: m}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestEncodeWritesMetadataBlocksVerbatim(t *testing.T) {
	img := testimg.Photo(64, 48)
	m := testimg.SampleMetadata(64, 48)
	data := encodeWithMetadata(t, img, m)
	got, err := meta.ReadBlocks(bytes.NewReader(data), int64(len(data)), codec.JPEG)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.EXIF, m.EXIF) || !bytes.Equal(got.ICC, m.ICC) || !bytes.Equal(got.XMP, m.XMP) {
		t.Fatalf("blocks differ: exif %d/%d icc %d/%d xmp %d/%d", len(got.EXIF), len(m.EXIF), len(got.ICC), len(m.ICC), len(got.XMP), len(m.XMP))
	}
	read, err := meta.Read(context.Background(), bytes.NewReader(data), int64(len(data)), codec.JPEG)
	if err != nil {
		t.Fatal(err)
	}
	if read.Exif == nil || read.Exif.Make != "PixGo" || read.Exif.PixelWidth != 64 || read.Exif.PixelHeight != 48 {
		t.Fatalf("exif %+v", read.Exif)
	}
	if read.ICC == nil || read.ICC.Class != "Display" || read.ICC.ColorSpace != "RGB" {
		t.Fatalf("icc %+v", read.ICC)
	}
	if read.XMP == nil || read.XMP.Title != "Sample & test" || read.XMP.Rating != 3 {
		t.Fatalf("xmp %+v", read.XMP)
	}
}

func TestEncodedFileWithMetadataDecodesEverywhere(t *testing.T) {
	img := testimg.Photo(70, 50)
	plain := encodeWithMetadata(t, img, codec.Metadata{})
	rich := encodeWithMetadata(t, img, testimg.SampleMetadata(70, 50))
	if !bytes.HasPrefix(rich, []byte{0xff, 0xd8, 0xff, 0xe1}) {
		t.Fatalf("segments must follow SOI: % x", rich[:6])
	}
	stdPlain, err := stdjpeg.Decode(bytes.NewReader(plain))
	if err != nil {
		t.Fatal(err)
	}
	stdRich, err := stdjpeg.Decode(bytes.NewReader(rich))
	if err != nil {
		t.Fatal(err)
	}
	if !samePixels(rgbaFrom(stdPlain), rgbaFrom(stdRich)) {
		t.Fatal("metadata changed the pixels")
	}
	ours, err := decodeWith(t, rich, codec.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if ours.Info.Width != 70 || ours.Info.Height != 50 {
		t.Fatalf("info %+v", ours.Info)
	}
	if ours.Info.Orientation != codec.OrientNormal {
		t.Fatalf("orientation %d", ours.Info.Orientation)
	}
}

func rgbaFrom(img image.Image) *image.NRGBA {
	b := img.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			r, g, bl, a := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
			i := out.PixOffset(x, y)
			out.Pix[i], out.Pix[i+1], out.Pix[i+2], out.Pix[i+3] = uint8(r>>8), uint8(g>>8), uint8(bl>>8), uint8(a>>8)
		}
	}
	return out
}

func TestLargeICCProfileIsChunked(t *testing.T) {
	profile := testimg.SampleICC()
	profile = append(profile, bytes.Repeat([]byte{7}, 150_000)...)
	data := encodeWithMetadata(t, testimg.Photo(16, 16), codec.Metadata{ICC: profile})
	got, err := meta.ReadBlocks(bytes.NewReader(data), int64(len(data)), codec.JPEG)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.ICC, profile) {
		t.Fatalf("profile of %d bytes came back as %d", len(profile), len(got.ICC))
	}
	if _, err := stdjpeg.Decode(bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}
}

func TestMetadataThatCannotFitIsDroppedNotWritten(t *testing.T) {
	m := testimg.SampleMetadata(8, 8)
	m.EXIF = append(m.EXIF, bytes.Repeat([]byte{1}, 70_000)...)
	m.XMP = bytes.Repeat([]byte{'x'}, 70_000)
	data := encodeWithMetadata(t, testimg.Photo(8, 8), m)
	got, err := meta.ReadBlocks(bytes.NewReader(data), int64(len(data)), codec.JPEG)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.EXIF) != 0 || len(got.XMP) != 0 || !bytes.Equal(got.ICC, m.ICC) {
		t.Fatalf("oversized blocks must be dropped and the profile kept: %d %d %d", len(got.EXIF), len(got.XMP), len(got.ICC))
	}
	if _, err := stdjpeg.Decode(bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}
}

func TestNoMetadataKeepsTheOutputIdentical(t *testing.T) {
	img := testimg.Photo(33, 21)
	var want bytes.Buffer
	if err := stdjpeg.Encode(&want, rgbaOf(img), &stdjpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	if got := encodeWithMetadata(t, img, codec.Metadata{}); !bytes.Equal(got, want.Bytes()) {
		t.Fatal("an empty Metadata must not change the encoder output")
	}
}

func TestMetadataSegmentsAreWellFormed(t *testing.T) {
	m := testimg.SampleMetadata(10, 10)
	segs := metadataSegments(m)
	pos, markers := 0, []byte{}
	for pos < len(segs) {
		if segs[pos] != 0xff {
			t.Fatalf("no marker at %d", pos)
		}
		n := int(segs[pos+2])<<8 | int(segs[pos+3])
		markers = append(markers, segs[pos+1])
		pos += 2 + n
	}
	if pos != len(segs) || !bytes.Equal(markers, []byte{0xe1, 0xe1, 0xe2}) {
		t.Fatalf("pos %d of %d, markers % x", pos, len(segs), markers)
	}
}
