package all

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
)

func readSample(t *testing.T, parts ...string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(append([]string{".."}, parts...)...))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func requireDecoded(t *testing.T, r *codec.Registry, name string, data []byte, id codec.ID, width, height int) {
	t.Helper()
	src := &codec.Source{Path: name, R: bytes.NewReader(data), Size: int64(len(data)), Head: head(data)}
	d, info, err := r.Probe(context.Background(), src, codec.DefaultLimits())
	if err != nil || d.ID() != id || info.Format != id || info.Width != width || info.Height != height {
		t.Fatalf("%s: %v %+v %v", name, d, info, err)
	}
	img, err := r.Decode(context.Background(), d, src, codec.Options{Limits: codec.DefaultLimits(), Frames: codec.AllFrames})
	if err != nil || img.First().Bounds().Dx() != info.Width || img.First().Bounds().Dy() != info.Height || img.Info.Format != id {
		t.Fatalf("%s: %v", name, err)
	}
}

func decoderOf(t *testing.T, r *codec.Registry, id codec.ID) codec.Decoder {
	t.Helper()
	for _, d := range r.Decoders() {
		if d.ID() == id {
			return d
		}
	}
	t.Fatalf("%s decoder is not registered", id)
	return nil
}
