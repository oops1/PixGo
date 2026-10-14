package raw

import (
	"bytes"
	"context"
	"errors"
	"testing"
)

func standardJPEG(t *testing.T) []byte {
	t.Helper()
	return jpegOf(t, 40, 30, tomato)
}

func TestProbeJPEGHeaders(t *testing.T) {
	segment := func(marker byte, body ...byte) []byte {
		n := len(body) + 2
		return append([]byte{0xff, marker, byte(n >> 8), byte(n)}, body...)
	}
	frame := func(marker, precision byte, w, h int, components byte) []byte {
		body := []byte{precision, byte(h >> 8), byte(h), byte(w >> 8), byte(w), components}
		for i := 0; i < int(components); i++ {
			body = append(body, byte(i+1), 0x11, 0)
		}
		return segment(marker, body...)
	}
	soi := []byte{0xff, 0xd8}
	build := func(parts ...[]byte) []byte { return append(append([]byte(nil), soi...), bytes.Join(parts, nil)...) }
	cases := []struct {
		name   string
		data   []byte
		status jpegStatus
		w, h   int
		comps  int
	}{
		{"baseline", build(frame(0xc0, 8, 640, 480, 3)), jpegOK, 640, 480, 3},
		{"extended sequential", build(frame(0xc1, 8, 100, 50, 1)), jpegOK, 100, 50, 1},
		{"progressive", build(frame(0xc2, 8, 4000, 3000, 3)), jpegOK, 4000, 3000, 3},
		{"cmyk", build(frame(0xc0, 8, 10, 10, 4)), jpegOK, 10, 10, 4},
		{"after application segments", build(segment(0xe0, make([]byte, 14)...), segment(0xe1, make([]byte, 500)...), segment(0xdb, make([]byte, 65)...), frame(0xc0, 8, 33, 22, 3)), jpegOK, 33, 22, 3},
		{"fill bytes before markers", build([]byte{0xff, 0xff, 0xff}, frame(0xc0, 8, 33, 22, 3)), jpegOK, 33, 22, 3},
		{"standalone markers are skipped", build([]byte{0xff, 0x01, 0xff, 0xd3}, frame(0xc0, 8, 33, 22, 3)), jpegOK, 33, 22, 3},
		{"twelve bit precision", build(frame(0xc1, 12, 100, 100, 3)), jpegUnsupported, 0, 0, 0},
		{"two components", build(frame(0xc0, 8, 100, 100, 2)), jpegUnsupported, 0, 0, 0},
		{"no components", build(frame(0xc0, 8, 100, 100, 0)), jpegUnsupported, 0, 0, 0},
		{"lossless", build(frame(0xc3, 8, 100, 100, 1)), jpegUnsupported, 0, 0, 0},
		{"differential sequential", build(frame(0xc5, 8, 100, 100, 3)), jpegUnsupported, 0, 0, 0},
		{"differential progressive", build(frame(0xc6, 8, 100, 100, 3)), jpegUnsupported, 0, 0, 0},
		{"differential lossless", build(frame(0xc7, 8, 100, 100, 3)), jpegUnsupported, 0, 0, 0},
		{"arithmetic sequential", build(frame(0xc9, 8, 100, 100, 3)), jpegUnsupported, 0, 0, 0},
		{"arithmetic progressive", build(frame(0xca, 8, 100, 100, 3)), jpegUnsupported, 0, 0, 0},
		{"arithmetic lossless", build(frame(0xcb, 8, 100, 100, 3)), jpegUnsupported, 0, 0, 0},
		{"differential arithmetic", build(frame(0xcd, 8, 100, 100, 3), frame(0xce, 8, 1, 1, 1), frame(0xcf, 8, 1, 1, 1)), jpegUnsupported, 0, 0, 0},
		{"Huffman table is not a frame", build(segment(0xc4, make([]byte, 20)...), frame(0xc0, 8, 5, 5, 1)), jpegOK, 5, 5, 1},
		{"no width", build(frame(0xc0, 8, 0, 480, 3)), jpegNotJPEG, 0, 0, 0},
		{"no height", build(frame(0xc0, 8, 640, 0, 3)), jpegNotJPEG, 0, 0, 0},
		{"scan before frame", build(segment(0xda, 1, 1, 0, 0, 0x3f, 0)), jpegNotJPEG, 0, 0, 0},
		{"end before frame", build([]byte{0xff, 0xd9}), jpegNotJPEG, 0, 0, 0},
		{"segment shorter than its own length", build([]byte{0xff, 0xe0, 0, 1}), jpegNotJPEG, 0, 0, 0},
		{"frame segment too short", build(segment(0xc0, 8, 0, 5)), jpegNotJPEG, 0, 0, 0},
		{"garbage instead of a marker", build([]byte{0x12, 0x34, 0x56, 0x78}), jpegNotJPEG, 0, 0, 0},
		{"missing third signature byte", []byte{0xff, 0xd8, 0xaa, 0xc0, 0, 17}, jpegNotJPEG, 0, 0, 0},
		{"not a JPEG", []byte("GIF89a....."), jpegNotJPEG, 0, 0, 0},
		{"only the signature", []byte{0xff, 0xd8, 0xff}, jpegOutOfRange, 0, 0, 0},
		{"cut inside a segment header", []byte{0xff, 0xd8, 0xff, 0xe0, 0}, jpegOutOfRange, 0, 0, 0},
		{"cut inside the frame header", build(frame(0xc0, 8, 640, 480, 3)[:8]), jpegOutOfRange, 0, 0, 0},
		{"too short for a signature", []byte{0xff, 0xd8}, jpegOutOfRange, 0, 0, 0},
	}
	for _, c := range cases {
		head, status := probeJPEG(bytes.NewReader(c.data), int64(len(c.data)), 0, 0)
		if status != c.status || head.width != c.w || head.height != c.h || head.components != c.comps {
			t.Errorf("%s: %+v status %d, want %dx%d/%d status %d", c.name, head, status, c.w, c.h, c.comps, c.status)
		}
	}
}

func TestProbeJPEGRespectsOffsetAndLength(t *testing.T) {
	data := standardJPEG(t)
	padded := append(bytes.Repeat([]byte{9}, 100), data...)
	padded = append(padded, bytes.Repeat([]byte{8}, 50)...)
	size := int64(len(padded))
	reader := bytes.NewReader(padded)
	cases := []struct {
		name           string
		offset, length int64
		status         jpegStatus
	}{
		{"exact range", 100, int64(len(data)), jpegOK},
		{"unknown length", 100, 0, jpegOK},
		{"length beyond the file", 100, size, jpegOutOfRange},
		{"length of two bytes", 100, 2, jpegOutOfRange},
		{"offset at the end", size, 0, jpegOutOfRange},
		{"offset beyond the end", size + 10, 0, jpegOutOfRange},
		{"negative offset", -1, 0, jpegOutOfRange},
		{"not at a signature", 101, 0, jpegNotJPEG},
		{"offset beyond the end with a length", size + 10, 5, jpegOutOfRange},
	}
	for _, c := range cases {
		if _, status := probeJPEG(reader, size, c.offset, c.length); status != c.status {
			t.Errorf("%s: status %d, want %d", c.name, status, c.status)
		}
	}
}

func TestProbeJPEGGivesUpOnEndlessSegments(t *testing.T) {
	data := []byte{0xff, 0xd8}
	for i := 0; i < maxJPEGSegments+10; i++ {
		data = append(data, 0xff, 0xe0, 0, 2)
	}
	data = append(data, frameJPEG(0xc0, 8, 5, 5, 1)[2:]...)
	if _, status := probeJPEG(bytes.NewReader(data), int64(len(data)), 0, 0); status != jpegNotJPEG {
		t.Fatalf("status %d", status)
	}
}

type failingReader struct{ err error }

func (f failingReader) ReadAt([]byte, int64) (int, error) { return 0, f.err }

func TestProbeJPEGReadFailureIsOutOfRange(t *testing.T) {
	if _, status := probeJPEG(failingReader{errors.New("boom")}, 1000, 0, 0); status != jpegOutOfRange {
		t.Fatalf("status %d", status)
	}
}

func TestCarveFindsEverySignatureOnceAcrossBlockBoundaries(t *testing.T) {
	size := 3*carveBlock + 100
	positions := []int{0, 5, carveBlock - 5, carveBlock - 2, carveBlock + 2, 2*carveBlock - 1, 2*carveBlock + 4, 3*carveBlock + 97}
	data := make([]byte, size)
	for _, p := range positions {
		copy(data[p:], jpegStart)
	}
	var got []int64
	err := carve(context.Background(), bytes.NewReader(data), int64(size), 0, int64(size), func(at int64) bool {
		got = append(got, at)
		return true
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(positions) {
		t.Fatalf("found %v, want %v", got, positions)
	}
	for i, p := range positions {
		if got[i] != int64(p) {
			t.Fatalf("found %v, want %v", got, positions)
		}
	}
}

func TestCarveHonoursItsBoundsAndCallbacks(t *testing.T) {
	size := maxCarveBytes + 4096
	data := make([]byte, size)
	copy(data[100:], jpegStart)
	copy(data[200:], jpegStart)
	copy(data[maxCarveBytes+100:], jpegStart)
	count := func(from, to int64, stopAfter int) []int64 {
		var got []int64
		err := carve(context.Background(), bytes.NewReader(data), int64(size), from, to, func(at int64) bool {
			got = append(got, at)
			return stopAfter == 0 || len(got) < stopAfter
		})
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	if got := count(0, int64(size), 0); len(got) != 2 {
		t.Fatalf("scan is limited to %d bytes: %v", maxCarveBytes, got)
	}
	if got := count(0, int64(size)*2, 0); len(got) != 2 {
		t.Fatalf("end beyond the file: %v", got)
	}
	if got := count(150, 1000, 0); len(got) != 1 || got[0] != 200 {
		t.Fatalf("sub-range: %v", got)
	}
	if got := count(int64(maxCarveBytes), int64(size), 0); len(got) != 1 {
		t.Fatalf("a later range starts a new window: %v", got)
	}
	if got := count(0, int64(size), 1); len(got) != 1 {
		t.Fatalf("callback stop: %v", got)
	}
	if got := count(300, 300, 0); len(got) != 0 {
		t.Fatalf("empty range: %v", got)
	}
}

func TestCarveStopsAfterTooManyHits(t *testing.T) {
	data := bytes.Repeat(append([]byte(nil), jpegStart...), 5000)
	calls := 0
	err := carve(context.Background(), bytes.NewReader(data), int64(len(data)), 0, int64(len(data)), func(int64) bool {
		calls++
		return true
	})
	if err != nil || calls != maxCarveHits {
		t.Fatalf("%d calls, %v", calls, err)
	}
}

func TestCarveReportsCancellationAndIgnoresReadFailures(t *testing.T) {
	data := make([]byte, 2*carveBlock)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := carve(ctx, bytes.NewReader(data), int64(len(data)), 0, int64(len(data)), func(int64) bool { return true }); !errors.Is(err, context.Canceled) {
		t.Fatalf("%v", err)
	}
	if err := carve(context.Background(), failingReader{errors.New("boom")}, 1000, 0, 1000, func(int64) bool { return true }); err != nil {
		t.Fatalf("%v", err)
	}
}
