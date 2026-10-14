package jxl

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"math/rand"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
)

func openBytes(data []byte) (*codestream, error) {
	return openCodestream(context.Background(), bytes.NewReader(data), int64(len(data)))
}

func readAll(t *testing.T, s *codestream) []byte {
	t.Helper()
	out, err := s.read(0, s.size)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestNakedCodestreamIsReadAsOneSegment(t *testing.T) {
	data := encodeLossless(t, 20, 10, 1)
	s, err := openBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	if s.size != int64(len(data)) || s.level != supportedLevel || !bytes.Equal(readAll(t, s), data) {
		t.Fatalf("%+v", s)
	}
}

func TestContainersDeliverTheCodestreamWhicheverWayItIsBoxed(t *testing.T) {
	stream := encodeLossless(t, 30, 20, 2)
	shuffled := partial(stream, 4)
	shuffled[0], shuffled[2] = shuffled[2], shuffled[0]
	tests := []struct {
		name string
		data []byte
	}{
		{"single box", contained(stream)},
		{"large box header", boxed(containerSignature, ftypBox(), largeBox("jxlc", stream))},
		{"open ended box", boxed(containerSignature, ftypBox(), openEndedBox("jxlc", stream))},
		{"metadata around the codestream", boxed(containerSignature, ftypBox(), box("Exif", []byte("exif")), box("jxll", []byte{5}), box("jxlc", stream), box("xml ", []byte("<x/>")), box("jumb", []byte("j")), box("brob", []byte("Exifzz")))},
		{"two parts", boxed(append([][]byte{containerSignature, ftypBox()}, partial(stream, 2)...)...)},
		{"seven parts", boxed(append([][]byte{containerSignature, ftypBox()}, partial(stream, 7)...)...)},
		{"parts out of order", boxed(append([][]byte{containerSignature, ftypBox()}, shuffled...)...)},
		{"parts split across other boxes", boxed(containerSignature, ftypBox(), partial(stream, 3)[0], box("Exif", []byte("abc")), partial(stream, 3)[1], partial(stream, 3)[2])},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := openBytes(tt.data)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(readAll(t, s), stream) {
				t.Fatal("codestream differs")
			}
			for _, window := range [][2]int64{{0, 2}, {1, 5}, {int64(len(stream)) / 3, 40}, {int64(len(stream)) - 3, 3}, {int64(len(stream)), 0}} {
				got, err := s.read(window[0], window[1])
				if err != nil || !bytes.Equal(got, stream[window[0]:window[0]+window[1]]) {
					t.Fatalf("window %v: %v", window, err)
				}
			}
		})
	}
}

func TestCodestreamReadsOutsideTheStreamAreRefused(t *testing.T) {
	s, err := openBytes(contained(encodeLossless(t, 8, 8, 3)))
	if err != nil {
		t.Fatal(err)
	}
	for _, window := range [][2]int64{{-1, 1}, {0, s.size + 1}, {s.size + 1, 0}, {5, -1}} {
		if _, err := s.read(window[0], window[1]); !errors.Is(err, codec.ErrCorrupt) {
			t.Fatalf("%v: %v", window, err)
		}
	}
}

func TestContainerLevelsAreRead(t *testing.T) {
	stream := encodeLossless(t, 8, 8, 4)
	for _, level := range []byte{5, 10} {
		s, err := openBytes(boxed(containerSignature, ftypBox(), box("jxll", []byte{level}), box("jxlc", stream)))
		if err != nil || s.level != int(level) {
			t.Fatalf("level %d: %+v %v", level, s, err)
		}
	}
}

func TestMalformedContainersAreRefused(t *testing.T) {
	stream := encodeLossless(t, 8, 8, 5)
	parts := partial(stream, 3)
	tests := []struct {
		name string
		data []byte
	}{
		{"nothing after the signature", append([]byte{}, containerSignature...)},
		{"first box is not ftyp", boxed(containerSignature, box("jxlc", stream))},
		{"wrong brand", boxed(containerSignature, box("ftyp", []byte("avif"), binary.BigEndian.AppendUint32(nil, 0), []byte("jxl ")), box("jxlc", stream))},
		{"future version", boxed(containerSignature, box("ftyp", []byte("jxl "), binary.BigEndian.AppendUint32(nil, 2), []byte("jxl ")), box("jxlc", stream))},
		{"short ftyp", boxed(containerSignature, box("ftyp", []byte("jxl ")), box("jxlc", stream))},
		{"second ftyp", boxed(containerSignature, ftypBox(), ftypBox(), box("jxlc", stream))},
		{"no codestream box", boxed(containerSignature, ftypBox(), box("Exif", []byte("exif")))},
		{"two codestream boxes", boxed(containerSignature, ftypBox(), box("jxlc", stream), box("jxlc", stream))},
		{"whole codestream after parts", boxed(containerSignature, ftypBox(), parts[0], box("jxlc", stream))},
		{"parts after the whole codestream", boxed(containerSignature, ftypBox(), box("jxlc", stream), parts[0])},
		{"missing part", boxed(containerSignature, ftypBox(), parts[0], parts[2])},
		{"repeated part", boxed(containerSignature, ftypBox(), parts[0], parts[0], parts[1], parts[2])},
		{"part without an index", boxed(containerSignature, ftypBox(), box("jxlp", []byte{0, 0}))},
		{"empty level box", boxed(containerSignature, ftypBox(), box("jxll"), box("jxlc", stream))},
		{"box larger than the file", boxed(containerSignature, ftypBox(), binary.BigEndian.AppendUint32(nil, 9999), []byte("jxlc"), stream)},
		{"large box larger than the file", boxed(containerSignature, ftypBox(), binary.BigEndian.AppendUint32(nil, 1), []byte("jxlc"), binary.BigEndian.AppendUint64(nil, 1<<40))},
		{"large box header cut", boxed(containerSignature, ftypBox(), binary.BigEndian.AppendUint32(nil, 1), []byte("jxlc"))},
		{"box smaller than its header", boxed(containerSignature, ftypBox(), binary.BigEndian.AppendUint32(nil, 4), []byte("jxlc"), stream)},
		{"header cut", boxed(containerSignature, ftypBox(), []byte{0, 0, 0})},
		{"no signature", []byte("not a jxl file at all")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := openBytes(tt.data); !errors.Is(err, codec.ErrCorrupt) {
				t.Fatalf("%v", err)
			}
		})
	}
}

func TestContainerWalkIsBounded(t *testing.T) {
	stream := encodeLossless(t, 8, 8, 6)
	var filler []byte
	for i := 0; i < maxContainerBoxes+1; i++ {
		filler = append(filler, box("free")...)
	}
	if _, err := openBytes(boxed(containerSignature, ftypBox(), filler, box("jxlc", stream))); !errors.Is(err, codec.ErrCorrupt) {
		t.Fatalf("boxes: %v", err)
	}
}

func TestContainerWalkStopsOnCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	data := contained(encodeLossless(t, 8, 8, 7))
	if _, err := openCodestream(ctx, bytes.NewReader(data), int64(len(data))); !errors.Is(err, context.Canceled) {
		t.Fatalf("%v", err)
	}
}

func TestTruncatedContainersNeverPanic(t *testing.T) {
	random := rand.New(rand.NewSource(1))
	data := boxed(append([][]byte{containerSignature, ftypBox()}, partial(encodeLossless(t, 16, 16, 8), 3)...)...)
	for cut := 0; cut < len(data); cut++ {
		if _, err := openBytes(data[:cut]); err == nil && cut < len(data)-1 {
			t.Fatalf("cut %d of %d opened", cut, len(data))
		}
		damaged := append([]byte(nil), data...)
		damaged[random.Intn(len(damaged))] ^= 1 << uint(random.Intn(8))
		openBytes(damaged)
	}
}
