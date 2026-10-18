package jpeg

import (
	"bytes"
	"context"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/testimg"
)

func decodeColor(t testing.TB, data []byte) *codec.Image {
	t.Helper()
	img, err := Decoder{}.Decode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Options{Limits: codec.DefaultLimits()})
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func TestDecodeReportsTheEmbeddedProfile(t *testing.T) {
	profile := testimg.SampleICCSpace("RGB ", 3000)
	data := encodeWithMetadata(t, testimg.Photo(48, 32), codec.Metadata{ICC: profile})
	img := decodeColor(t, data)
	if !bytes.Equal(img.Color.ICC, profile) || img.Color.CICP != nil {
		t.Fatalf("%d bytes, cicp %+v", len(img.Color.ICC), img.Color.CICP)
	}
}

func TestDecodeReportsAProfileSplitOverSeveralSegments(t *testing.T) {
	profile := testimg.SampleICCSpace("RGB ", 150_000)
	data := encodeWithMetadata(t, testimg.Photo(16, 16), codec.Metadata{ICC: profile})
	if bytes.Count(data, []byte("ICC_PROFILE\x00")) < 3 {
		t.Fatal("profile was not split")
	}
	if got := decodeColor(t, data).Color.ICC; !bytes.Equal(got, profile) {
		t.Fatalf("%d bytes, want %d", len(got), len(profile))
	}
}

func TestPictureWithoutProfileHasNoColor(t *testing.T) {
	data := encodeWithMetadata(t, testimg.Photo(16, 16), codec.Metadata{})
	if got := decodeColor(t, data).Color; !got.Empty() {
		t.Fatalf("%+v", got)
	}
}

func TestProfilesOfOtherColourSpacesAreNotReportedForRGBOutput(t *testing.T) {
	data := encodeWithMetadata(t, testimg.Photo(16, 16), codec.Metadata{ICC: testimg.SampleICCSpace("CMYK", 10)})
	if got := decodeColor(t, data).Color; !got.Empty() {
		t.Fatalf("CMYK profile reported for RGB pixels: %d bytes", len(got.ICC))
	}
}

func TestTileReaderReportsTheProfile(t *testing.T) {
	profile := testimg.SampleICCSpace("RGB ", 500)
	data := encodeWithMetadata(t, testimg.Photo(64, 48), codec.Metadata{ICC: profile})
	tr, err := Decoder{}.OpenTiled(context.Background(), bytes.NewReader(data), int64(len(data)), codec.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Close()
	colored, ok := tr.(codec.ColorTileReader)
	if !ok {
		t.Fatal("tile reader does not implement ColorTileReader")
	}
	if !bytes.Equal(colored.Color().ICC, profile) {
		t.Fatal("profile differs")
	}
}
