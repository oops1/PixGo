package tiff

import (
	"bytes"
	"compress/bzip2"
	"context"
	"errors"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	xtiff "golang.org/x/image/tiff"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/convert"
)

func readSample(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasSuffix(name, ".bz2") {
		raw, err = io.ReadAll(bzip2.NewReader(bytes.NewReader(raw)))
		if err != nil {
			t.Fatal(err)
		}
	}
	return raw
}

func sampleNames(t *testing.T) []string {
	t.Helper()
	names, err := filepath.Glob("testdata/*")
	if err != nil || len(names) < 20 {
		t.Fatalf("test samples are missing: %v", err)
	}
	for i := range names {
		names[i] = filepath.Base(names[i])
	}
	return names
}

func TestRealFilesDecodeLikeXImage(t *testing.T) {
	ctx := context.Background()
	handled, rejected := 0, 0
	for _, name := range sampleNames(t) {
		data := readSample(t, name)
		reference, referenceErr := xtiff.Decode(bytes.NewReader(data))
		img, err := Decoder{}.Decode(ctx, bytes.NewReader(data), int64(len(data)), codec.Options{Limits: codec.DefaultLimits()})
		if referenceErr != nil {
			if err == nil {
				t.Errorf("%s: x/image fails (%v) but Decode succeeds", name, referenceErr)
				continue
			}
			rejected++
			continue
		}
		if err != nil {
			t.Errorf("%s: x/image decodes, Decode fails: %v", name, err)
			continue
		}
		want := convert.NRGBA(reference)
		requireIdentical(t, img.First(), want, name)
		handled++
		_, _, why, perr := inspect(ctx, bytes.NewReader(data), int64(len(data)))
		if perr != nil || why != "" {
			continue
		}
		rd := openTiled(t, data)
		overview, err := rd.Overview(ctx, 0)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		requireIdentical(t, overview, want, name+" Overview")
		levels := levelsOf(want, 4)
		rng := rand.New(rand.NewSource(int64(len(data))))
		for shift := 1; shift <= 4; shift++ {
			level, err := rd.Overview(ctx, shift)
			if err != nil {
				t.Fatal(err)
			}
			requireIdentical(t, level, levels[shift], name+" level")
			lw, lh := codec.LevelSize(want.Rect.Dx(), want.Rect.Dy(), shift)
			for i := 0; i < 6; i++ {
				r := randomRect(rng, lw, lh)
				region, err := rd.Region(ctx, shift, r)
				if err != nil {
					t.Fatal(err)
				}
				requireIdentical(t, region, cropOf(level, r), name+" region")
			}
		}
	}
	if handled < 20 || rejected < 3 {
		t.Fatalf("handled %d files, rejected %d", handled, rejected)
	}
}

func TestRealFilesReportTheNewReaderWhereItApplies(t *testing.T) {
	ctx := context.Background()
	supported := 0
	for _, name := range sampleNames(t) {
		data := readSample(t, name)
		_, lay, why, err := inspect(ctx, bytes.NewReader(data), int64(len(data)))
		if err != nil || why != "" {
			continue
		}
		supported++
		info, err := Decoder{}.Probe(ctx, bytes.NewReader(data), int64(len(data)), codec.DefaultLimits())
		if err != nil || info.Width != lay.width || info.Height != lay.height {
			t.Errorf("%s: %+v %v", name, info, err)
		}
	}
	if supported < 20 {
		t.Fatalf("only %d samples are handled by the new reader", supported)
	}
}

func TestRealFilesWithBrokenPalette(t *testing.T) {
	data := readSample(t, "invalid-palette-ref.tiff")
	if _, err := decode(data); !errors.Is(err, codec.ErrCorrupt) {
		t.Fatalf("%v", err)
	}
	if rd, err := (Decoder{}).OpenTiled(context.Background(), bytes.NewReader(data), int64(len(data)), codec.DefaultLimits()); err == nil {
		defer rd.Close()
		if _, err := rd.Overview(context.Background(), 0); !errors.Is(err, codec.ErrCorrupt) {
			t.Fatalf("%v", err)
		}
	}
}

func TestRealFilesSurviveCorruption(t *testing.T) {
	rng := rand.New(rand.NewSource(91))
	for _, name := range sampleNames(t) {
		good := readSample(t, name)
		if len(good) > 80_000 {
			continue
		}
		for i := 0; i < 25; i++ {
			data := append([]byte(nil), good...)
			for k := 0; k < 1+rng.Intn(4); k++ {
				data[rng.Intn(len(data))] = byte(rng.Intn(256))
			}
			exerciseAll(t, data)
		}
	}
}
