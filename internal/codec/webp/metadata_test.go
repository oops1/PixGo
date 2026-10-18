package webp

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/testimg"
	"github.com/oops1/PixGo/internal/meta"
)

type encodeCase struct {
	name     string
	lossless bool
	alpha    bool
}

var encodeCases = []encodeCase{
	{"lossy", false, false},
	{"lossy alpha", false, true},
	{"lossless", true, false},
	{"lossless alpha", true, true},
}

func chunkList(t testing.TB, data []byte) []riffChunk {
	t.Helper()
	if int(binary.LittleEndian.Uint32(data[4:]))+8 != len(data) {
		t.Fatalf("RIFF size %d does not match the file of %d bytes", binary.LittleEndian.Uint32(data[4:]), len(data))
	}
	chunks, err := readChunks(data)
	if err != nil {
		t.Fatal(err)
	}
	return chunks
}

func TestMetadataBlocksAreWrittenVerbatim(t *testing.T) {
	for _, c := range encodeCases {
		t.Run(c.name, func(t *testing.T) {
			img := smooth(41, 29)
			if c.alpha {
				img = withAlpha(img)
			}
			m := testimg.SampleMetadata(41, 29)
			data := encodeWith(t, img, codec.EncodeOptions{Quality: 80, Lossless: c.lossless, Metadata: m})
			got, err := meta.ReadBlocks(bytes.NewReader(data), int64(len(data)), codec.WebP)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got.EXIF, m.EXIF) || !bytes.Equal(got.ICC, m.ICC) || !bytes.Equal(got.XMP, m.XMP) {
				t.Fatalf("blocks differ: exif %d/%d icc %d/%d xmp %d/%d", len(got.EXIF), len(m.EXIF), len(got.ICC), len(m.ICC), len(got.XMP), len(m.XMP))
			}
			read, err := meta.Read(context.Background(), bytes.NewReader(data), int64(len(data)), codec.WebP)
			if err != nil {
				t.Fatal(err)
			}
			if read.Exif == nil || read.Exif.Make != "PixGo" || read.ICC == nil || read.XMP == nil || read.Width != 41 || read.Height != 29 {
				t.Fatalf("%+v %+v %+v %dx%d", read.Exif, read.ICC, read.XMP, read.Width, read.Height)
			}
		})
	}
}

func TestExtendedHeaderCarriesTheRightFlags(t *testing.T) {
	m := testimg.SampleMetadata(33, 21)
	for _, c := range encodeCases {
		for mask := 1; mask < 8; mask++ {
			var sub codec.Metadata
			var want byte
			if mask&1 != 0 {
				sub.ICC, want = m.ICC, want|flagICC
			}
			if mask&2 != 0 {
				sub.EXIF, want = m.EXIF, want|flagEXIF
			}
			if mask&4 != 0 {
				sub.XMP, want = m.XMP, want|flagXMP
			}
			if c.alpha {
				want |= alphaBit
			}
			t.Run(fmt.Sprintf("%s/%d", c.name, mask), func(t *testing.T) {
				img := smooth(33, 21)
				if c.alpha {
					img = withAlpha(img)
				}
				data := encodeWith(t, img, codec.EncodeOptions{Quality: 70, Lossless: c.lossless, Metadata: sub})
				chunks := chunkList(t, data)
				if chunks[0].kind != "VP8X" || len(chunks[0].body) != 10 {
					t.Fatalf("first chunk %q", chunks[0].kind)
				}
				if chunks[0].body[0] != want {
					t.Fatalf("flags %#x, want %#x", chunks[0].body[0], want)
				}
				w := 1 + int(chunks[0].body[4]) | int(chunks[0].body[5])<<8 | int(chunks[0].body[6])<<16
				h := 1 + int(chunks[0].body[7]) | int(chunks[0].body[8])<<8 | int(chunks[0].body[9])<<16
				if w != 33 || h != 21 {
					t.Fatalf("canvas %dx%d", w, h)
				}
				var kinds []string
				for _, ch := range chunks {
					kinds = append(kinds, ch.kind)
				}
				order := map[string]int{"VP8X": 0, "ICCP": 1, "ALPH": 2, "VP8 ": 2, "VP8L": 2, "EXIF": 3, "XMP ": 4}
				for i := 1; i < len(kinds); i++ {
					if order[kinds[i]] < order[kinds[i-1]] {
						t.Fatalf("chunk order %v", kinds)
					}
				}
				info, err := probe(data)
				if err != nil {
					t.Fatal(err)
				}
				if info.Width != 33 || info.Height != 21 || info.HasAlpha != c.alpha {
					t.Fatalf("info %+v", info)
				}
			})
		}
	}
}

func TestMetadataDoesNotChangeThePixels(t *testing.T) {
	for _, c := range encodeCases {
		t.Run(c.name, func(t *testing.T) {
			img := smooth(48, 36)
			if c.alpha {
				img = withAlpha(img)
			}
			plain := encodeWith(t, img, codec.EncodeOptions{Quality: 75, Lossless: c.lossless})
			rich := encodeWith(t, img, codec.EncodeOptions{Quality: 75, Lossless: c.lossless, Metadata: testimg.SampleMetadata(48, 36)})
			a, err := decode(plain)
			if err != nil {
				t.Fatal(err)
			}
			b, err := decode(rich)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(a.First().Pix, b.First().Pix) {
				t.Fatal("the pixels differ")
			}
			if c.lossless && !bytes.Equal(b.First().Pix, img.Pix) {
				t.Fatalf("lossless output is not exact")
			}
			if b.Info.Orientation != codec.OrientNormal {
				t.Fatalf("orientation %d", b.Info.Orientation)
			}
		})
	}
}

func TestNoMetadataKeepsTheOutputIdentical(t *testing.T) {
	img := smooth(30, 30)
	a := encodeWith(t, img, codec.EncodeOptions{Quality: 60})
	b := encodeWith(t, img, codec.EncodeOptions{Quality: 60, Metadata: codec.Metadata{}})
	if !bytes.Equal(a, b) {
		t.Fatal("an empty Metadata must not change the output")
	}
	if chunkList(t, a)[0].kind == "VP8X" && !bytes.Contains(a, []byte("ALPH")) {
		t.Fatal("an opaque picture without metadata needs no extended header")
	}
}

func TestOddSizedChunksArePadded(t *testing.T) {
	m := codec.Metadata{EXIF: append(testimg.SampleEXIF(5, 5), 0), XMP: []byte(testimg.SampleXMP + " ")}
	for len(m.EXIF)%2 == 0 {
		m.EXIF = append(m.EXIF, 0)
	}
	data := encodeWith(t, smooth(16, 16), codec.EncodeOptions{Quality: 50, Metadata: m})
	chunkList(t, data)
	got, err := meta.ReadBlocks(bytes.NewReader(data), int64(len(data)), codec.WebP)
	if err != nil || !bytes.Equal(got.EXIF, m.EXIF) || !bytes.Equal(got.XMP, m.XMP) {
		t.Fatalf("%v %d %d", err, len(got.EXIF), len(got.XMP))
	}
}

func TestOversizedBlocksAreDroppedNotFatal(t *testing.T) {
	m := codec.Metadata{EXIF: []byte("not a tiff block at all")}
	data := encodeWith(t, smooth(16, 16), codec.EncodeOptions{Quality: 50, Metadata: m})
	got, err := meta.ReadBlocks(bytes.NewReader(data), int64(len(data)), codec.WebP)
	if err != nil || len(got.EXIF) != 0 {
		t.Fatalf("%v %d", err, len(got.EXIF))
	}
}

func TestAddMetadataRejectsABrokenEncoderOutput(t *testing.T) {
	m := testimg.SampleMetadata(2, 2)
	for _, bad := range [][]byte{nil, []byte("RIFF"), []byte("RIFX\x04\x00\x00\x00WEBP"), append([]byte("RIFF\x10\x00\x00\x00WEBPVP8 \xff\xff\xff\x7f"), 1, 2, 3)} {
		if _, err := addMetadata(bad, m, 2, 2); err == nil {
			t.Fatalf("accepted %q", bad)
		}
	}
}
