package png

import (
	"bytes"
	"context"
	"encoding/binary"
	"hash/crc32"
	"image"
	stdpng "image/png"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/testimg"
	"github.com/oops1/PixGo/internal/meta"
)

func encodeWithMetadata(t testing.TB, img *image.NRGBA, m codec.Metadata) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := (Encoder{}).Encode(&buf, img, codec.EncodeOptions{Metadata: m}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func chunkKinds(t testing.TB, data []byte) []string {
	t.Helper()
	var kinds []string
	pos := 8
	for pos < len(data) {
		n := int(binary.BigEndian.Uint32(data[pos:]))
		kind := string(data[pos+4 : pos+8])
		body := data[pos+8 : pos+8+n]
		sum := binary.BigEndian.Uint32(data[pos+8+n:])
		if crc32.ChecksumIEEE(append([]byte(kind), body...)) != sum {
			t.Fatalf("chunk %s has a bad checksum", kind)
		}
		kinds = append(kinds, kind)
		pos += 12 + n
	}
	return kinds
}

func TestEncodeWritesMetadataBlocksVerbatim(t *testing.T) {
	img := testimg.Photo(40, 30)
	m := testimg.SampleMetadata(40, 30)
	data := encodeWithMetadata(t, img, m)
	got, err := meta.ReadBlocks(bytes.NewReader(data), int64(len(data)), codec.PNG)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.EXIF, m.EXIF) || !bytes.Equal(got.ICC, m.ICC) || !bytes.Equal(got.XMP, m.XMP) {
		t.Fatalf("blocks differ: exif %d/%d icc %d/%d xmp %d/%d", len(got.EXIF), len(m.EXIF), len(got.ICC), len(m.ICC), len(got.XMP), len(m.XMP))
	}
	read, err := meta.Read(context.Background(), bytes.NewReader(data), int64(len(data)), codec.PNG)
	if err != nil {
		t.Fatal(err)
	}
	if read.Exif == nil || read.Exif.Make != "PixGo" || read.ICC == nil || read.ICC.Class != "Display" || read.XMP == nil || read.XMP.Title != "Sample & test" {
		t.Fatalf("%+v %+v %+v", read.Exif, read.ICC, read.XMP)
	}
	if read.Width != 40 || read.Height != 30 {
		t.Fatalf("size %dx%d", read.Width, read.Height)
	}
}

func TestChunkOrderAndChecksums(t *testing.T) {
	data := encodeWithMetadata(t, testimg.Photo(20, 20), testimg.SampleMetadata(20, 20))
	kinds := chunkKinds(t, data)
	if kinds[0] != "IHDR" || kinds[1] != "iCCP" || kinds[2] != "eXIf" || kinds[3] != "iTXt" {
		t.Fatalf("order %v", kinds)
	}
	if kinds[len(kinds)-1] != "IEND" {
		t.Fatalf("order %v", kinds)
	}
}

func TestFileWithMetadataStillDecodesLosslessly(t *testing.T) {
	img := testimg.Noise(37, 29, 3, true)
	data := encodeWithMetadata(t, img, testimg.SampleMetadata(37, 29))
	std, err := stdpng.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if b := std.Bounds(); b.Dx() != 37 || b.Dy() != 29 {
		t.Fatalf("bounds %v", b)
	}
	ours, err := decode(data)
	if err != nil {
		t.Fatal(err)
	}
	got := ours.First()
	for i := range img.Pix {
		if img.Pix[i] != got.Pix[i] {
			t.Fatalf("pixel data differs at byte %d", i)
		}
	}
	if ours.Info.Orientation != codec.OrientNormal {
		t.Fatalf("orientation %d", ours.Info.Orientation)
	}
}

func TestNoMetadataKeepsTheOutputIdentical(t *testing.T) {
	img := testimg.Noise(20, 20, 1, true)
	var want bytes.Buffer
	std := stdpng.Encoder{CompressionLevel: stdpng.DefaultCompression}
	if err := std.Encode(&want, img); err != nil {
		t.Fatal(err)
	}
	if got := encodeWithMetadata(t, img, codec.Metadata{}); !bytes.Equal(got, want.Bytes()) {
		t.Fatal("an empty Metadata must not change the encoder output")
	}
}

func TestOnlyWhatWasGivenIsWritten(t *testing.T) {
	data := encodeWithMetadata(t, testimg.Photo(8, 8), codec.Metadata{XMP: []byte(testimg.SampleXMP)})
	kinds := chunkKinds(t, data)
	for _, k := range kinds {
		if k == "iCCP" || k == "eXIf" {
			t.Fatalf("unexpected %s in %v", k, kinds)
		}
	}
	got, err := meta.ReadBlocks(bytes.NewReader(data), int64(len(data)), codec.PNG)
	if err != nil || len(got.XMP) == 0 || len(got.EXIF) != 0 || len(got.ICC) != 0 {
		t.Fatalf("%v %+v", err, got)
	}
}

func TestInvalidExifBlockIsNotWritten(t *testing.T) {
	data := encodeWithMetadata(t, testimg.Photo(8, 8), codec.Metadata{EXIF: []byte("definitely not tiff")})
	for _, k := range chunkKinds(t, data) {
		if k == "eXIf" {
			t.Fatal("garbage was written as eXIf")
		}
	}
}
