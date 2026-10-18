package meta

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/meta/exif"
)

func readBlocks(t testing.TB, data []byte, id codec.ID) codec.Metadata {
	t.Helper()
	m, err := ReadBlocks(bytes.NewReader(data), int64(len(data)), id)
	if err != nil {
		t.Fatalf("ReadBlocks(%s): %v", id, err)
	}
	return m
}

func summary(m *Metadata) string {
	var e Exif
	var f Flash
	if m.Exif != nil {
		e = *m.Exif
		if e.Flash != nil {
			f = *e.Flash
		}
		e.Flash = nil
	}
	return fmt.Sprintf("%+v|%+v|%+v", e, f, m.GPS)
}

func wrapEXIF(tiff []byte) []byte {
	return buildJPEG(10, 10, jpegExifSeg(tiff))
}

func TestReadBlocksReturnsStoredBytesVerbatim(t *testing.T) {
	for _, o := range orders {
		e := enc{o.order}
		want := e.tiff(photoParts(e, false))
		cases := []struct {
			id   codec.ID
			data func() ([]byte, []byte)
		}{
			{codec.JPEG, func() ([]byte, []byte) { return fullJPEG(t, e, false) }},
			{codec.PNG, func() ([]byte, []byte) { return fullPNG(t, e, false) }},
			{codec.WebP, func() ([]byte, []byte) { return fullWebP(t, e, false) }},
		}
		for _, c := range cases {
			t.Run(string(c.id)+"/"+o.name, func(t *testing.T) {
				data, profile := c.data()
				m := readBlocks(t, data, c.id)
				if !bytes.Equal(m.EXIF, want) {
					t.Fatalf("EXIF differs: %d bytes, want %d", len(m.EXIF), len(want))
				}
				if !bytes.Equal(m.ICC, profile) {
					t.Fatalf("ICC differs: %d bytes, want %d", len(m.ICC), len(profile))
				}
				if string(m.XMP) != xmpSample {
					t.Fatalf("XMP differs")
				}
			})
		}
	}
}

func TestReadBlocksAgreesWithReadAcrossContainers(t *testing.T) {
	var samples []sample
	for _, o := range orders {
		e := enc{o.order}
		tiff, _ := fullTIFF(t, e, false)
		samples = append(samples, sample{"tiff/" + o.name, codec.TIFF, tiff})
	}
	samples = append(samples, newContainerSamples(t)...)
	for _, s := range samples {
		t.Run(s.name, func(t *testing.T) {
			m := readBlocks(t, s.data, s.id)
			if len(m.EXIF) == 0 {
				t.Fatal("no EXIF block")
			}
			if !validTIFFHeader(m.EXIF) {
				t.Fatalf("EXIF block is not a TIFF structure: % x", m.EXIF[:8])
			}
			want := mustRead(t, s.data, s.id)
			got := mustRead(t, wrapEXIF(m.EXIF), codec.JPEG)
			if got.Exif == nil || got.GPS == nil {
				t.Fatalf("rebuilt EXIF lost its content: %+v %+v", got.Exif, got.GPS)
			}
			if want.Exif != nil {
				want.Exif.PixelWidth, want.Exif.PixelHeight = got.Exif.PixelWidth, got.Exif.PixelHeight
			}
			if summary(got) != summary(want) {
				t.Fatalf("rebuilt EXIF reads differently:\n got %s\nwant %s", summary(got), summary(want))
			}
			if want.ICC != nil {
				if len(m.ICC) == 0 {
					t.Fatal("ICC profile missing")
				}
				if parsed := parseICC(m.ICC); parsed == nil || parsed.Description != want.ICC.Description {
					t.Fatalf("ICC block %+v", parsed)
				}
			}
			if want.XMP != nil {
				if len(m.XMP) == 0 {
					t.Fatal("XMP packet missing")
				}
				if x := parseXMP(m.XMP); x == nil || x.Title != want.XMP.Title {
					t.Fatalf("XMP block %+v", x)
				}
			}
		})
	}
}

func TestReadBlocksTIFFFiltersImageStructure(t *testing.T) {
	e := enc{orders[0].order}
	parts := photoParts(e, false)
	parts.ifd0 = append(parts.ifd0, e.long(0x0100, 6000), e.long(0x0101, 4000), e.long(0x0111, 1234), e.long(0x0117, 5678), e.short(0x0103, 5))
	data := e.tiff(parts)
	m := readBlocks(t, data, codec.TIFF)
	doc, err := parseTIFF(m.EXIF)
	if err != nil {
		t.Fatal(err)
	}
	for _, tag := range []uint16{0x0100, 0x0101, 0x0111, 0x0117, 0x0103} {
		if doc.ifd0.index(tag) >= 0 {
			t.Fatalf("image structure tag %#x leaked into the EXIF block", tag)
		}
	}
	if doc.ifd0.index(0x010f) < 0 || doc.ifd0.index(tagOrientation) < 0 {
		t.Fatal("descriptive tags were lost")
	}
}

func TestReadBlocksEmptyAndUnknown(t *testing.T) {
	data := buildJPEG(4, 4)
	if m := readBlocks(t, data, codec.JPEG); !m.Empty() {
		t.Fatalf("a bare JPEG has no metadata, got %d/%d/%d bytes", len(m.EXIF), len(m.ICC), len(m.XMP))
	}
	if m, err := ReadBlocks(bytes.NewReader(data), int64(len(data)), codec.GIF); err != nil || !m.Empty() {
		t.Fatalf("unsupported format: %v %v", m, err)
	}
	if _, err := ReadBlocks(nil, 0, codec.JPEG); err == nil {
		t.Fatal("nil reader must fail")
	}
	if _, err := ReadBlocks(bytes.NewReader([]byte("nope")), 4, codec.JPEG); !errors.Is(err, codec.ErrCorrupt) {
		t.Fatalf("garbage: %v", err)
	}
}

func TestReadBlocksKeepsPartialResultOnDamage(t *testing.T) {
	e := enc{orders[0].order}
	data, _ := fullJPEG(t, e, false)
	cut := data[:len(data)/2]
	m, err := ReadBlocks(bytes.NewReader(cut), int64(len(cut)), codec.JPEG)
	if err == nil {
		t.Fatal("a cut file must report an error")
	}
	if len(m.EXIF) == 0 {
		t.Fatal("blocks found before the damage must survive")
	}
}

func TestReadBlocksContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	data := buildJPEG(4, 4)
	if _, err := ReadBlocksContext(ctx, bytes.NewReader(data), int64(len(data)), codec.JPEG); !errors.Is(err, context.Canceled) {
		t.Fatalf("err %v", err)
	}
}

func TestReadBlocksCR3KeepsTagsFromEveryDirectory(t *testing.T) {
	for _, o := range orders {
		e := enc{o.order}
		m := readBlocks(t, cr3File(e, false, cr3Options{}), codec.RAW)
		r, err := exif.NewReader(m.EXIF)
		if err != nil {
			t.Fatal(err)
		}
		tree, err := r.Tree()
		if err != nil || tree.Exif == nil || tree.GPS == nil {
			t.Fatalf("tree %+v err %v", tree, err)
		}
		if _, ok := tree.IFD0.Find(0x010f); !ok {
			t.Fatal("Make missing")
		}
	}
}
