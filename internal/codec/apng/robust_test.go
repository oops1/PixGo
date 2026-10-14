package apng

import (
	"bytes"
	"context"
	"errors"
	"math/rand"
	"strings"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/testimg"
)

func TestHeaderValidation(t *testing.T) {
	cases := []struct {
		name                                       string
		depth, colourType, compression, filter, il byte
		ok                                         bool
	}{
		{"gray 1", 1, 0, 0, 0, 0, true},
		{"gray 2", 2, 0, 0, 0, 0, true},
		{"gray 4", 4, 0, 0, 0, 0, true},
		{"gray 16", 16, 0, 0, 0, 0, true},
		{"rgb 16 interlaced", 16, 2, 0, 0, 1, true},
		{"indexed 8", 8, 3, 0, 0, 0, true},
		{"indexed 16", 16, 3, 0, 0, 0, false},
		{"gray with alpha 8", 8, 4, 0, 0, 0, true},
		{"gray with alpha 4", 4, 4, 0, 0, 0, false},
		{"rgb 4", 4, 2, 0, 0, 0, false},
		{"rgba 1", 1, 6, 0, 0, 0, false},
		{"colour type 1", 8, 1, 0, 0, 0, false},
		{"colour type 5", 8, 5, 0, 0, 0, false},
		{"colour type 7", 8, 7, 0, 0, 0, false},
		{"depth 3", 3, 0, 0, 0, 0, false},
		{"compression 1", 8, 6, 1, 0, 0, false},
		{"filter 1", 8, 6, 0, 1, 0, false},
		{"interlace 2", 8, 6, 0, 0, 2, false},
	}
	for _, c := range cases {
		if got := validHeader(c.depth, c.colourType, c.compression, c.filter, c.il); got != c.ok {
			t.Errorf("%s: validHeader = %v", c.name, got)
		}
		header := ihdr(4, 4, c.depth, c.colourType, c.il)
		header[10], header[11] = c.compression, c.filter
		data := chunkList{
			{kind: "IHDR", body: header},
			{kind: "acTL", body: animationControl(1, 0)},
			{kind: "IEND"},
		}.bytes()
		_, err := probe(data)
		if invalid := err != nil && strings.Contains(err.Error(), "invalid IHDR"); invalid == c.ok {
			t.Errorf("%s: probe: %v", c.name, err)
		}
	}
}

func TestGrayscaleWithAlphaFrames(t *testing.T) {
	for _, depth := range []byte{8, 16} {
		rowBytes := 4 * 2 * int(depth) / 8
		raw := make([]byte, 4*(1+rowBytes))
		for i := range raw {
			raw[i] = byte(i * 7)
		}
		for y := 0; y < 4; y++ {
			raw[y*(1+rowBytes)] = 0
		}
		data := chunkList{
			{kind: "IHDR", body: ihdr(4, 4, depth, 4, 0)},
			{kind: "acTL", body: animationControl(2, 0)},
			{kind: "fcTL", body: frameControl(0, frameSpec{num: 1, den: 10}, 4, 4)},
			{kind: "IDAT", body: deflate(raw)},
			{kind: "fcTL", body: frameControl(1, frameSpec{num: 1, den: 10, blend: 1}, 4, 4)},
			{kind: "fdAT", body: append([]byte{0, 0, 0, 2}, deflate(raw)...)},
			{kind: "IEND"},
		}.bytes()
		info, err := probe(data)
		if err != nil || info.BitDepth != 2*int(depth) || !info.HasAlpha {
			t.Fatalf("depth %d: %+v %v", depth, info, err)
		}
		if img := mustDecodeAll(t, data); len(img.Frames) != 2 {
			t.Fatalf("depth %d: %d frames", depth, len(img.Frames))
		}
	}
}

type flakyReader struct {
	r     *bytes.Reader
	after int
	reads int
	err   error
}

func (f *flakyReader) ReadAt(p []byte, off int64) (int, error) {
	f.reads++
	if f.after > 0 && f.reads >= f.after {
		return 0, f.err
	}
	return f.r.ReadAt(p, off)
}

func TestReadErrorsAreReportedAsCorruptData(t *testing.T) {
	data := animation{pieces: 3, frames: []frameSpec{canvasFrame(red), rect(green, 2, 2, 1, 1, 0, 0), canvasFrame(blue)}}.bytes(t)
	counter := &flakyReader{r: bytes.NewReader(data)}
	if _, err := (Decoder{}).Decode(context.Background(), counter, int64(len(data)), codec.Options{Frames: codec.AllFrames}); err != nil {
		t.Fatal(err)
	}
	failed := 0
	for after := 1; after <= counter.reads; after++ {
		reader := &flakyReader{r: bytes.NewReader(data), after: after, err: errors.New("device not ready")}
		_, err := Decoder{}.Decode(context.Background(), reader, int64(len(data)), codec.Options{Frames: codec.AllFrames})
		if err != nil {
			testimg.RequireCodecError(t, err)
			failed++
		}
		reader = &flakyReader{r: bytes.NewReader(data), after: after, err: errors.New("device not ready")}
		if _, err := (Decoder{}).Probe(context.Background(), reader, int64(len(data)), codec.DefaultLimits()); err != nil {
			testimg.RequireCodecError(t, err)
		}
	}
	if failed == 0 {
		t.Fatal("read errors were swallowed")
	}
}

type shiftingReader struct {
	first, second []byte
	switchAfter   int
	reads         int
}

func (s *shiftingReader) ReadAt(p []byte, off int64) (int, error) {
	s.reads++
	source := s.first
	if s.reads > s.switchAfter {
		source = s.second
	}
	if off >= int64(len(source)) {
		return 0, errors.New("EOF")
	}
	n := copy(p, source[off:])
	if n < len(p) {
		return n, errors.New("short read")
	}
	return n, nil
}

func TestFileChangingBetweenPassesNeverPanics(t *testing.T) {
	base := animation{pieces: 3, frames: []frameSpec{canvasFrame(red), rect(green, 2, 2, 1, 1, 0, 0), canvasFrame(blue)}}.bytes(t)
	rng := rand.New(rand.NewSource(11))
	variants := [][]byte{base[:len(base)/2], base[:len(base)-20], nil}
	for i := 0; i < 80; i++ {
		mutated := append([]byte(nil), base...)
		for k := 0; k < 1+rng.Intn(6); k++ {
			mutated[rng.Intn(len(mutated))] = byte(rng.Intn(256))
		}
		variants = append(variants, mutated)
	}
	for _, other := range variants {
		for switchAfter := 1; switchAfter <= 6; switchAfter++ {
			reader := &shiftingReader{first: base, second: other, switchAfter: switchAfter}
			if _, err := (Decoder{}).Decode(context.Background(), reader, int64(len(base)), codec.Options{Frames: codec.AllFrames}); err != nil {
				testimg.RequireCodecError(t, err)
			}
		}
	}
}

func TestFrameDataLengthsAreRechecked(t *testing.T) {
	base := animation{frames: []frameSpec{canvasFrame(red), canvasFrame(green)}}.chunks(t)
	small := base.clone().edit("fdAT", 0, func(ch *rawChunk) { ch.body = ch.body[:2] }).bytes()
	huge := uint32(0xfffffff0)
	oversized := base.clone().edit("fdAT", 0, func(ch *rawChunk) { ch.badSize = &huge }).bytes()
	data := base.bytes()
	for _, other := range [][]byte{small, oversized, data[:len(data)-30]} {
		reader := &shiftingReader{first: data, second: other, switchAfter: 1}
		_, err := Decoder{}.Decode(context.Background(), reader, int64(len(data)), codec.Options{Frames: codec.AllFrames})
		testimg.RequireCodecError(t, err)
	}
}
