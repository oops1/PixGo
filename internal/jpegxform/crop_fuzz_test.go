package jpegxform

import (
	"bytes"
	"context"
	"errors"
	"image"
	stdjpeg "image/jpeg"
	"math/rand"
	"testing"
	"time"
)

func FuzzTransformCrop(f *testing.F) {
	for _, name := range corpusNames(f) {
		data := readCorpus(f, name)
		f.Add(data, uint8(0), true, int16(10), int16(10), int16(100), int16(80))
		f.Add(data, uint8(3), false, int16(0), int16(0), int16(7), int16(9))
		f.Add(data, uint8(6), true, int16(-5), int16(-5), int16(20000), int16(20000))
	}
	rng := rand.New(rand.NewSource(41))
	for i, sc := range samplingCases {
		s := samplingSpec(70+i, 50+i, sc.factors...)
		f.Add(buildJPEG(f, s, randomWall(rng, s)), uint8(i), i%2 == 0, int16(i), int16(3), int16(60), int16(45))
	}
	f.Fuzz(func(t *testing.T, data []byte, opSeed uint8, trim bool, x0, y0, x1, y1 int16) {
		op := Op(opSeed % uint8(opCount))
		want := image.Rect(int(x0), int(y0), int(x1), int(y1))
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		var applied image.Rectangle
		var out bytes.Buffer
		err := TransformContext(ctx, &out, bytes.NewReader(data), int64(len(data)), op, Options{Trim: trim, MaxPixels: fuzzMaxPixels, Crop: want, Applied: &applied, StripGPS: x0%2 == 0})
		snapped, trimmed, serr := CropForTrim(bytes.NewReader(data), int64(len(data)), op, want)
		if err != nil {
			if !classified(err) && !errors.Is(err, ErrCrop) {
				t.Fatalf("unclassified error: %v", err)
			}
			return
		}
		if want.Empty() {
			return
		}
		if serr != nil {
			t.Fatalf("Transform succeeded but CropForTrim failed: %v", serr)
		}
		if trimmed && !trim {
			t.Fatal("a trimmed crop succeeded without Trim")
		}
		if snapped != applied {
			t.Fatalf("CropForTrim %v, Transform %v", snapped, applied)
		}
		clamped := want.Canon()
		if applied.Empty() || applied.Max.X > clamped.Max.X || applied.Max.Y > clamped.Max.Y {
			t.Fatalf("applied %v for %v", applied, want)
		}
		cfg, cerr := stdjpeg.DecodeConfig(bytes.NewReader(out.Bytes()))
		if cerr != nil {
			if isStdUnsupported(cerr) {
				return
			}
			t.Fatalf("the output has no readable header: %v", cerr)
		}
		if cfg.Width != applied.Dx() || cfg.Height != applied.Dy() {
			t.Fatalf("output is %dx%d, applied %v", cfg.Width, cfg.Height, applied)
		}
		if _, derr := stdjpeg.Decode(bytes.NewReader(out.Bytes())); derr != nil && !isStdUnsupported(derr) {
			t.Fatalf("the output does not decode: %v", derr)
		}
	})
}
