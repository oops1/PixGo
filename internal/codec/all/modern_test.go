//go:build !noavif && !noheic && !nojxl

package all

import (
	"bytes"
	"context"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/testimg"
	"github.com/oops1/PixGo/internal/isobmff/isotest"
)

func modernSamples(t *testing.T) map[codec.ID][]byte {
	t.Helper()
	return map[codec.ID][]byte{
		codec.AVIF: readSample(t, "avif", "testdata", "yuv420-8.avif"),
		codec.HEIC: readSample(t, "heic", "testdata", "basic.heic"),
		codec.JXL:  readSample(t, "jxl", "testdata", "dct_small.jxl"),
	}
}

func TestModernFormatsAreNeverTakenForEachOther(t *testing.T) {
	r := newRegistry()
	names := []string{"x.avif", "x.heic", "x.heif", "x.hif", "x.jxl", "x.png", "x.jpg", "x", "x.avifs"}
	for id, data := range modernSamples(t) {
		for _, name := range names {
			d, ok := r.Detect(head(data), name)
			if !ok || d.ID() != id {
				t.Fatalf("%s as %q detected as %v", id, name, d)
			}
		}
	}
	extensions := map[string]bool{}
	for _, e := range r.Extensions() {
		extensions[e] = true
	}
	for _, e := range []string{".avif", ".heic", ".heif", ".hif", ".jxl"} {
		if !extensions[e] {
			t.Fatalf("%s is not in the catalogue", e)
		}
	}
}

func TestModernDecodersRefuseEachOthersFilesCleanly(t *testing.T) {
	r := newRegistry()
	files := modernSamples(t)
	for _, id := range []codec.ID{codec.AVIF, codec.HEIC, codec.JXL} {
		d := decoderOf(t, r, id)
		for owner, data := range files {
			_, probeErr := d.Probe(context.Background(), bytes.NewReader(data), int64(len(data)), testimg.SmallLimits())
			_, decodeErr := d.Decode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Options{Limits: testimg.SmallLimits()})
			if owner == id {
				if probeErr != nil || decodeErr != nil {
					t.Fatalf("%s rejected its own file: %v %v", id, probeErr, decodeErr)
				}
				continue
			}
			for _, err := range []error{probeErr, decodeErr} {
				testimg.RequireCodecError(t, err)
			}
		}
	}
}

func TestUndeterminedHEIFBrandsAreSettledByTheExtension(t *testing.T) {
	r := newRegistry()
	head := isotest.File{Major: "mif1", Compatible: []string{"miaf"}}.Bytes()
	for name, want := range map[string]codec.ID{"x.avif": codec.AVIF, "x.AVIF": codec.AVIF, "x.heic": codec.HEIC, "x.heif": codec.HEIC, "x.hif": codec.HEIC} {
		d, ok := r.Detect(head, name)
		if !ok || d.ID() != want {
			t.Fatalf("%s detected as %v", name, d)
		}
	}
	if _, ok := r.Detect(head, "x.dat"); !ok {
		t.Fatal("a generic HEIF file must still find a decoder, which then reports what it holds")
	}
}
