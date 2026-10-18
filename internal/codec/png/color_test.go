package png

import (
	"bytes"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/testimg"
)

func TestDecodeReportsTheEmbeddedProfile(t *testing.T) {
	profile := testimg.SampleICCSpace("RGB ", 900)
	data := encodeWithMetadata(t, testimg.Photo(30, 20), codec.Metadata{ICC: profile})
	img := decodeBytes(t, data, codec.Options{Limits: codec.DefaultLimits()})
	if !bytes.Equal(img.Color.ICC, profile) || img.Color.CICP != nil {
		t.Fatalf("%d bytes, cicp %+v", len(img.Color.ICC), img.Color.CICP)
	}
}

func TestDecodeWithoutColorChunksHasNoColor(t *testing.T) {
	data := encodeWithMetadata(t, testimg.Photo(30, 20), codec.Metadata{})
	if got := decodeBytes(t, data, codec.Options{Limits: codec.DefaultLimits()}).Color; !got.Empty() {
		t.Fatalf("%+v", got)
	}
}

func TestCICPAloneIsReported(t *testing.T) {
	raw := encodeWithMetadata(t, testimg.Photo(8, 8), codec.Metadata{})
	at := 8 + 8 + 13 + 4
	data := append(append(append([]byte(nil), raw[:at]...), pngChunk("cICP", []byte{1, 18, 0, 0})...), raw[at:]...)
	got := decodeBytes(t, data, codec.Options{Limits: codec.DefaultLimits()}).Color
	if got.ICC != nil || got.CICP == nil || *got.CICP != (codec.CICP{Primaries: 1, Transfer: 18}) || !got.HDR() {
		t.Fatalf("%+v", got)
	}
}
