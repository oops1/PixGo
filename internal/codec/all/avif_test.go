//go:build !noavif

package all

import (
	"bytes"
	"context"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
)

func TestAVIFIsRegisteredAndDecodedThroughTheRegistry(t *testing.T) {
	r := newRegistry()
	data := readSample(t, "avif", "testdata", "yuv420-8.avif")
	for _, name := range []string{"photo.avif", "photo.AVIF", "photo.png", "photo.heic", "photo.jxl", "photo", "photo.dat"} {
		d, ok := r.Detect(head(data), name)
		if !ok || d.ID() != codec.AVIF {
			t.Fatalf("%s: detected as %v", name, d)
		}
		requireDecoded(t, r, name, data, codec.AVIF, 160, 120)
	}
	if !r.Supports("movie.avif") {
		t.Fatal(".avif is not supported")
	}
}

func TestAnimatedAVIFThroughTheRegistry(t *testing.T) {
	r := newRegistry()
	data := readSample(t, "avif", "testdata", "anim.avif")
	src := &codec.Source{Path: "a.avif", R: bytes.NewReader(data), Size: int64(len(data)), Head: head(data)}
	d, info, err := r.Probe(context.Background(), src, codec.DefaultLimits())
	if err != nil || d.ID() != codec.AVIF || info.Frames != 5 || !info.Animated {
		t.Fatalf("%v %+v %v", d, info, err)
	}
	img, err := r.Decode(context.Background(), d, src, codec.Options{Limits: codec.DefaultLimits(), Frames: codec.AllFrames})
	if err != nil || len(img.Frames) != 5 {
		t.Fatalf("%v", err)
	}
}

func TestAVIFRejectsTheOtherFormatsOfTheSuite(t *testing.T) {
	d := decoderOf(t, newRegistry(), codec.AVIF)
	for _, s := range samples(t) {
		if d.Sniff(head(s.data)) {
			t.Fatalf("%s sniffed as AVIF", s.id)
		}
	}
}
