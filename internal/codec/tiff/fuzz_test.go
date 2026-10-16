package tiff

import (
	"os"
	"path/filepath"
	"testing"
)

func FuzzTiledReader(f *testing.F) {
	picks := []struct{ kind, variant, compression, predictor int }{
		{12, 0, 1, 0}, {12, 5, 8, 2}, {12, 2, 5, 2}, {14, 3, 32773, 0}, {3, 7, 1, 0}, {10, 6, 8, 0},
		{16, 5, 5, 2}, {0, 1, 32773, 0}, {2, 5, 8, 0}, {13, 4, 1, 2}, {8, 0, 5, 0}, {11, 8, 8, 0},
	}
	for _, p := range picks {
		for _, bigTIFF := range []bool{false, true} {
			fix := kinds[p.kind].fixture(37, 29, 61)
			fix.compression, fix.predictor, fix.bigTIFF = p.compression, p.predictor, bigTIFF
			variants[p.variant].apply(&fix)
			f.Add(fix.bytes())
		}
	}
	samples, _ := filepath.Glob("testdata/*.tiff")
	for _, name := range samples {
		if raw, err := os.ReadFile(name); err == nil && len(raw) < 60_000 {
			f.Add(raw)
		}
	}
	f.Add([]byte("II*\x00\x08\x00\x00\x00"))
	f.Add([]byte("MM\x00+\x00\x08\x00\x00\x00\x00\x00\x00\x00\x00\x10"))
	f.Fuzz(func(t *testing.T, data []byte) {
		exerciseAll(t, data)
	})
}
