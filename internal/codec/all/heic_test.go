//go:build !noheic

package all

import (
	"bytes"
	"context"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
)

func TestHEICIsRegisteredAndDecodedThroughTheRegistry(t *testing.T) {
	r := newRegistry()
	data := readSample(t, "heic", "testdata", "basic.heic")
	for _, name := range []string{"photo.heic", "photo.HEIF", "photo.hif", "photo.png", "photo.avif", "photo", "photo.dat"} {
		d, ok := r.Detect(head(data), name)
		if !ok || d.ID() != codec.HEIC {
			t.Fatalf("%s: detected as %v", name, d)
		}
		requireDecoded(t, r, name, data, codec.HEIC, 320, 240)
	}
	for _, name := range []string{"a.heic", "a.heif", "a.hif"} {
		if !r.Supports(name) {
			t.Fatalf("%s is not supported", name)
		}
	}
}

func TestHEICAlphaThroughTheRegistry(t *testing.T) {
	r := newRegistry()
	data := readSample(t, "heic", "testdata", "alpha.heic")
	src := &codec.Source{Path: "a.heic", R: bytes.NewReader(data), Size: int64(len(data)), Head: head(data)}
	d, info, err := r.Probe(context.Background(), src, codec.DefaultLimits())
	if err != nil || d.ID() != codec.HEIC || !info.HasAlpha {
		t.Fatalf("%v %+v %v", d, info, err)
	}
}

func TestHEICRejectsTheOtherFormatsOfTheSuite(t *testing.T) {
	d := decoderOf(t, newRegistry(), codec.HEIC)
	for _, s := range samples(t) {
		if d.Sniff(head(s.data)) {
			t.Fatalf("%s sniffed as HEIC", s.id)
		}
	}
}
