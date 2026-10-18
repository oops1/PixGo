package meta

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/meta/exif"
)

func FuzzReadBlocks(f *testing.F) {
	for _, e := range []enc{le, be} {
		j, _ := fullJPEG(f, e, false)
		p, _ := fullPNG(f, e, true)
		w, _ := fullWebP(f, e, false)
		tf, _ := fullTIFF(f, e, true)
		f.Add(j, uint8(0))
		f.Add(p, uint8(1))
		f.Add(w, uint8(2))
		f.Add(tf, uint8(3))
	}
	for _, s := range newContainerSamples(f) {
		f.Add(s.data, fuzzSelector(s.id))
	}
	f.Add(loopTIFF(le), uint8(3))
	f.Add([]byte{}, uint8(0))
	f.Fuzz(func(t *testing.T, data []byte, sel uint8) {
		id := fuzzIDs[int(sel)%len(fuzzIDs)]
		start := time.Now()
		m, _ := ReadBlocksContext(context.Background(), bytes.NewReader(data), int64(len(data)), id)
		if elapsed := time.Since(start); elapsed > 5*time.Second {
			t.Fatalf("ReadBlocks took %v", elapsed)
		}
		if len(m.EXIF) > maxExifBlock+1<<20 || len(m.ICC) > MaxICCBytes || len(m.XMP) > MaxXMPBytes {
			t.Fatalf("block sizes %d %d %d", len(m.EXIF), len(m.ICC), len(m.XMP))
		}
		if len(m.EXIF) > 0 {
			if _, err := exif.NewReader(stripExifHeader(m.EXIF)); err == nil {
				checkRewriteInvariants(t, m)
			}
		}
	})
}

func checkRewriteInvariants(t *testing.T, m codec.Metadata) {
	t.Helper()
	opts := RewriteOptions{Width: 77, Height: 55, StripGPS: len(m.EXIF)%2 == 0, KeepThumbnail: len(m.EXIF)%3 == 0}
	once := Rewrite(m, opts)
	if len(once.EXIF) > 0 {
		doc, err := parseTIFF(once.EXIF)
		if err != nil {
			t.Fatalf("rewritten EXIF does not parse: %v", err)
		}
		if i := doc.ifd0.index(tagOrientation); i >= 0 && doc.order.Uint16(doc.ifd0.entries[i].data) != 1 {
			t.Fatal("orientation was not reset")
		}
		if opts.StripGPS && doc.gps != nil {
			t.Fatal("GPS survived")
		}
		if !opts.KeepThumbnail && doc.ifd1 != nil {
			t.Fatal("thumbnail survived")
		}
		twice := Rewrite(once, opts)
		if !bytes.Equal(once.EXIF, twice.EXIF) {
			t.Fatal("rewriting is not idempotent")
		}
	}
}

func FuzzRewrite(f *testing.F) {
	for _, e := range []enc{le, be} {
		parts := richParts(e)
		parts.exif = append(parts.exif, e.undefined(0x927c, canonLikeNote(200)))
		f.Add(withThumbnail(e, e.tiff(parts), testThumbnail), []byte(xmpGPSSample), uint16(640), uint16(480), uint8(0))
		f.Add(e.tiff(photoParts(e, true)), []byte(xmpSample), uint16(1), uint16(1), uint8(3))
	}
	f.Add([]byte("II*\x00\x08\x00\x00\x00\x00\x00\x00\x00\x00\x00"), []byte("<exif:GPSLatitude>"), uint16(0), uint16(0), uint8(1))
	f.Add([]byte{}, []byte{}, uint16(0), uint16(0), uint8(0))
	f.Fuzz(func(t *testing.T, tiff, xmp []byte, w, h uint16, flags uint8) {
		opts := RewriteOptions{Width: int(w), Height: int(h), StripGPS: flags&1 != 0, KeepThumbnail: flags&2 != 0}
		start := time.Now()
		out, err := RewriteFor(codec.Metadata{EXIF: tiff, XMP: xmp}, opts, codec.JPEG)
		if elapsed := time.Since(start); elapsed > 5*time.Second {
			t.Fatalf("rewrite took %v", elapsed)
		}
		if len(out.EXIF) > MaxJPEGExifBytes || len(out.XMP) > MaxJPEGXMPBytes {
			t.Fatalf("block exceeds a JPEG segment: %d %d (%v)", len(out.EXIF), len(out.XMP), err)
		}
		if len(out.EXIF) > 0 {
			checkRewriteInvariants(t, codec.Metadata{EXIF: out.EXIF})
			if !validTIFFHeader(out.EXIF) {
				t.Fatal("not a TIFF block")
			}
		}
		if opts.StripGPS && bytes.Contains(out.XMP, []byte("<exif:GPS")) {
			t.Fatal("GPS element survived in XMP")
		}
		_ = parseXMP(out.XMP)
	})
}
