//go:build !nojxl

package all

import (
	"bytes"
	"context"
	"encoding/binary"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
)

func containerOf(codestream []byte) []byte {
	signature := []byte("\x00\x00\x00\x0cJXL \x0d\x0a\x87\x0a")
	ftyp := []byte("\x00\x00\x00\x14ftypjxl \x00\x00\x00\x00jxl ")
	box := binary.BigEndian.AppendUint32(nil, uint32(8+len(codestream)))
	box = append(box, "jxlc"...)
	out := append(append(append([]byte{}, signature...), ftyp...), box...)
	return append(out, codestream...)
}

func TestJPEGXLIsRegisteredAndDecodedThroughTheRegistry(t *testing.T) {
	r := newRegistry()
	bare := readSample(t, "jxl", "testdata", "dct_small.jxl")
	for label, data := range map[string][]byte{"bare codestream": bare, "container": containerOf(bare)} {
		for _, name := range []string{"photo.jxl", "photo.JXL", "photo.png", "photo.avif", "photo", "photo.dat"} {
			d, ok := r.Detect(head(data), name)
			if !ok || d.ID() != codec.JXL {
				t.Fatalf("%s as %s: detected as %v", label, name, d)
			}
			requireDecoded(t, r, name, data, codec.JXL, 129, 77)
		}
	}
	if !r.Supports("movie.jxl") {
		t.Fatal(".jxl is not supported")
	}
}

func TestJPEGXLRejectsTheOtherFormatsOfTheSuite(t *testing.T) {
	d := decoderOf(t, newRegistry(), codec.JXL)
	for _, s := range samples(t) {
		if d.Sniff(head(s.data)) {
			t.Fatalf("%s sniffed as JPEG XL", s.id)
		}
	}
}

func TestJPEGXLProbedThroughTheRegistryHonoursTheLimits(t *testing.T) {
	r := newRegistry()
	data := readSample(t, "jxl", "testdata", "tree_plain.jxl")
	src := &codec.Source{Path: "a.jxl", R: bytes.NewReader(data), Size: int64(len(data)), Head: head(data)}
	d, info, err := r.Probe(context.Background(), src, codec.DefaultLimits())
	if err != nil || d.ID() != codec.JXL || info.Width != 2000 || info.Height != 1200 {
		t.Fatalf("%v %+v %v", d, info, err)
	}
	small := codec.DefaultLimits()
	small.MaxPixels = 1 << 20
	if _, _, err := r.Probe(context.Background(), src, small); err == nil {
		t.Fatal("a 2.4 megapixel file passed a 1 megapixel limit")
	}
}
