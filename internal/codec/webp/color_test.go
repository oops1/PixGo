package webp

import (
	"bytes"
	"context"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/testimg"
)

func TestDecodeReportsTheEmbeddedProfile(t *testing.T) {
	for _, c := range encodeCases {
		t.Run(c.name, func(t *testing.T) {
			img := smooth(41, 29)
			if c.alpha {
				img = withAlpha(img)
			}
			profile := testimg.SampleICCSpace("RGB ", 211)
			data := encodeWith(t, img, codec.EncodeOptions{Quality: 80, Lossless: c.lossless, Metadata: codec.Metadata{ICC: profile}})
			got, err := Decoder{}.Decode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Options{Limits: codec.DefaultLimits()})
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got.Color.ICC, profile) || got.Color.CICP != nil {
				t.Fatalf("%d bytes, cicp %+v", len(got.Color.ICC), got.Color.CICP)
			}
		})
	}
}

func TestDecodeWithoutProfileHasNoColor(t *testing.T) {
	data := encodeWith(t, smooth(20, 20), codec.EncodeOptions{Quality: 80})
	got, err := Decoder{}.Decode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Options{Limits: codec.DefaultLimits()})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Color.Empty() {
		t.Fatalf("%+v", got.Color)
	}
}
