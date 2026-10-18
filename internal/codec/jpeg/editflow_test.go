package jpeg

import (
	"bytes"
	"context"
	"image"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/testimg"
	"github.com/oops1/PixGo/internal/edit"
	"github.com/oops1/PixGo/internal/meta"
)

func TestSaveFlowKeepsMetadataAndFixesWhatEditingChanged(t *testing.T) {
	original := testimg.Photo(120, 80)
	source := codec.Metadata{EXIF: testimg.SampleEXIFOriented(120, 80, 6), ICC: testimg.SampleICC(), XMP: []byte(testimg.SampleXMP)}
	var file bytes.Buffer
	if err := (Encoder{}).Encode(&file, original, codec.EncodeOptions{Quality: 92, Metadata: source}); err != nil {
		t.Fatal(err)
	}
	r := bytes.NewReader(file.Bytes())
	decoded, err := Decoder{}.Decode(context.Background(), r, r.Size(), codec.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Info.Orientation != codec.OrientRotate90 {
		t.Fatalf("orientation %d", decoded.Info.Orientation)
	}
	plan := edit.Plan{Orientation: decoded.Info.Orientation, Crop: image.Rect(10, 5, 70, 100), Size: image.Pt(30, 48)}
	result, err := edit.Apply(context.Background(), decoded.First(), plan)
	if err != nil {
		t.Fatal(err)
	}
	blocks, err := meta.ReadBlocks(r, r.Size(), codec.JPEG)
	if err != nil {
		t.Fatal(err)
	}
	w, h := result.Bounds().Dx(), result.Bounds().Dy()
	fresh, err := meta.RewriteFor(blocks, meta.RewriteOptions{Width: w, Height: h, StripGPS: true}, codec.JPEG)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := (Encoder{}).Encode(&out, result, codec.EncodeOptions{Quality: 90, Metadata: fresh}); err != nil {
		t.Fatal(err)
	}
	got, err := meta.Read(context.Background(), bytes.NewReader(out.Bytes()), int64(out.Len()), codec.JPEG)
	if err != nil {
		t.Fatal(err)
	}
	if got.Width != 30 || got.Height != 48 {
		t.Fatalf("size %dx%d", got.Width, got.Height)
	}
	if got.Exif == nil || got.Exif.Orientation != codec.OrientNormal || got.Exif.PixelWidth != 30 || got.Exif.PixelHeight != 48 || got.Exif.Make != "PixGo" {
		t.Fatalf("exif %+v", got.Exif)
	}
	if got.ICC == nil || got.XMP == nil || got.XMP.Title != "Sample & test" {
		t.Fatalf("icc %+v xmp %+v", got.ICC, got.XMP)
	}
	again := bytes.NewReader(out.Bytes())
	info, err := Decoder{}.Probe(context.Background(), again, again.Size(), codec.Limits{})
	if err != nil || info.Orientation != codec.OrientNormal || info.Width != 30 || info.Height != 48 {
		t.Fatalf("%+v %v", info, err)
	}
}
