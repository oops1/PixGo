package jpegxform

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	stdjpeg "image/jpeg"
	"math/rand"
	"testing"
	"time"

	"github.com/oops1/PixGo/internal/codec"
)

const fuzzMaxPixels = 1 << 22

func addTransformSeeds(f *testing.F) {
	for _, name := range corpusNames(f) {
		f.Add(readCorpus(f, name), uint8(3), true)
	}
	rng := rand.New(rand.NewSource(40))
	for i, sc := range samplingCases {
		s := samplingSpec(40+i, 24+i, sc.factors...)
		s.style = tableStyle(i % 2)
		s.restart = i % 3
		f.Add(buildJPEG(f, s, randomWall(rng, s)), uint8(i), i%2 == 0)
	}
	s := samplingSpec(48, 32, [2]int{2, 2}, [2]int{1, 1}, [2]int{1, 1})
	tiff := buildTIFF(exifSpec{order: binary.LittleEndian, orientation: 6, width: 48, height: 32, pixelX: 48, pixelY: 32, resX: 72, resY: 72, thumbnail: []byte{0xFF, 0xD8, 1, 2, 3, 4}})
	s.segments = []segment{exifSegmentOf(tiff), xmpSegment(`<x tiff:Orientation="6"/>`)}
	f.Add(buildJPEG(f, s, randomWall(rng, s)), uint8(5), true)
}

func classified(err error) bool {
	for _, target := range []error{codec.ErrCorrupt, codec.ErrUnsupported, codec.ErrTooLarge, ErrImperfect} {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}

func FuzzTransform(f *testing.F) {
	addTransformSeeds(f)
	f.Fuzz(func(t *testing.T, data []byte, opSeed uint8, trim bool) {
		op := Op(opSeed % uint8(opCount))
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		var out bytes.Buffer
		err := TransformContext(ctx, &out, bytes.NewReader(data), int64(len(data)), op, Options{Trim: trim, MaxPixels: fuzzMaxPixels})
		perfect, perr := Perfect(bytes.NewReader(data), int64(len(data)), op)
		if err != nil {
			if !classified(err) {
				t.Fatalf("unclassified error: %v", err)
			}
			return
		}
		if perr != nil {
			t.Fatalf("Transform succeeded but Perfect failed: %v", perr)
		}
		if !perfect && !trim {
			t.Fatal("an imperfect transform succeeded without Trim")
		}
		got := out.Bytes()
		if len(got) < 4 || got[0] != 0xFF || got[1] != markerSOI || got[len(got)-2] != 0xFF || got[len(got)-1] != markerEOI {
			t.Fatal("output is not framed by SOI and EOI")
		}
		cfg, cerr := stdjpeg.DecodeConfig(bytes.NewReader(got))
		if cerr != nil {
			if isStdUnsupported(cerr) {
				return
			}
			t.Fatalf("the output has no readable header: %v", cerr)
		}
		hin, herr := parseHeaderOnly(data)
		if herr != nil {
			t.Fatalf("input header: %v", herr)
		}
		cw, ch, _ := edgeCrop(&hin.fr, op)
		wantW, wantH := cw, ch
		if op.Transposes() {
			wantW, wantH = ch, cw
		}
		if cfg.Width != wantW || cfg.Height != wantH {
			t.Fatalf("output is %dx%d, want %dx%d", cfg.Width, cfg.Height, wantW, wantH)
		}
		if _, derr := stdjpeg.Decode(bytes.NewReader(got)); derr != nil && !isStdUnsupported(derr) {
			t.Fatalf("the output does not decode: %v", derr)
		}
		if op == Identity {
			_, a := parseCoefficients(t, data)
			_, b := parseCoefficients(t, got)
			if !gridsEqual(a, b) {
				t.Fatal("identity changed coefficients")
			}
		}
	})
}
