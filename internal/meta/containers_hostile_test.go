package meta

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/oops1/PixGo/internal/codec"
)

var newIDs = []codec.ID{codec.HEIC, codec.AVIF, codec.JXL, codec.RAW}

func TestNewContainerSamplesAreComplete(t *testing.T) {
	samples := newContainerSamples(t)
	if len(samples) != 16 {
		t.Fatalf("%d samples", len(samples))
	}
	for _, s := range samples {
		m := mustRead(t, s.data, s.id)
		if m.Exif == nil || m.Exif.Model != "Canon EOS R5" || m.XMP == nil {
			t.Fatalf("%s: %+v", s.name, m)
		}
	}
}

func TestEveryTruncationOfNewContainersIsSafe(t *testing.T) {
	for _, s := range newContainerSamples(t) {
		within(t, 20*time.Second, func() {
			for n := 0; n <= len(s.data); n++ {
				prefix := s.data[:n]
				m, err := readAny(prefix, s.id)
				if m == nil || isPanic(err) {
					t.Fatalf("%s/%d: m=%v err=%v", s.name, n, m, err)
				}
				if m.Exif != nil && m.Exif.Model != "" && m.Exif.Model != "Canon EOS R5" {
					t.Fatalf("%s/%d: model %q", s.name, n, m.Exif.Model)
				}
				if n%7 == 0 {
					for _, other := range append(append([]codec.ID(nil), allIDs...), newIDs...) {
						if _, err := readAny(prefix, other); isPanic(err) {
							t.Fatalf("%s/%d as %s: %v", s.name, n, other, err)
						}
					}
				}
			}
		})
	}
}

func TestNewContainersReadAsEveryOtherFormat(t *testing.T) {
	for _, s := range newContainerSamples(t) {
		for _, id := range append(append([]codec.ID(nil), allIDs...), newIDs...) {
			if id == s.id {
				continue
			}
			if m, err := readAny(s.data, id); m == nil || isPanic(err) {
				t.Fatalf("%s as %s: %v", s.name, id, err)
			}
		}
	}
}

func TestFailingReadersOnNewContainers(t *testing.T) {
	for _, s := range newContainerSamples(t) {
		for failAfter := int64(0); failAfter < 16; failAfter++ {
			r := &brokenReader{data: s.data, failAfter: failAfter}
			within(t, 10*time.Second, func() {
				m, _ := Read(context.Background(), r, int64(len(s.data)), s.id)
				if m == nil {
					t.Fatalf("%s: nil metadata", s.name)
				}
			})
		}
		short := &brokenReader{data: s.data, failAfter: 1 << 30, short: true}
		m, err := Read(context.Background(), short, int64(len(s.data)), s.id)
		if m == nil {
			t.Fatalf("%s: nil metadata", s.name)
		}
		if err == nil && m.Exif != nil {
			t.Fatalf("%s: short reads must not yield data", s.name)
		}
		m, _ = Read(context.Background(), bytes.NewReader(s.data), int64(len(s.data))+5000, s.id)
		if m == nil {
			t.Fatalf("%s: nil metadata with inflated size", s.name)
		}
	}
}

func TestRandomBitFlipsInNewContainers(t *testing.T) {
	seed := uint32(987654)
	next := func() uint32 {
		seed = seed*1664525 + 1013904223
		return seed
	}
	for _, s := range newContainerSamples(t) {
		within(t, 60*time.Second, func() {
			for round := 0; round < 600; round++ {
				mutated := append([]byte(nil), s.data...)
				for k := 0; k < 1+int(next()%6); k++ {
					mutated[int(next()%uint32(len(mutated)))] ^= byte(next() >> 8)
				}
				m, err := readAny(mutated, s.id)
				if m == nil || isPanic(err) {
					t.Fatalf("%s round %d: %v", s.name, round, err)
				}
				checkInvariants(t, m)
			}
		})
	}
}

func TestNewContainersWithGarbageTailsAndHeads(t *testing.T) {
	for _, s := range newContainerSamples(t) {
		garbage := bytes.Repeat([]byte{0xa5, 0x5a, 0xff, 0x00}, 64)
		for name, data := range map[string][]byte{
			"garbage tail": append(append([]byte(nil), s.data...), garbage...),
			"garbage head": append(append([]byte(nil), garbage...), s.data...),
			"zeroed":       make([]byte, len(s.data)),
			"all ones":     bytes.Repeat([]byte{0xff}, len(s.data)),
		} {
			if m, err := readAny(data, s.id); m == nil || isPanic(err) {
				t.Fatalf("%s %s: %v", s.name, name, err)
			}
		}
	}
}
