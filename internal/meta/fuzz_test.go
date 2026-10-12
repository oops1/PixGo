package meta

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/oops1/PixGo/internal/codec"
)

var fuzzIDs = []codec.ID{codec.JPEG, codec.PNG, codec.WebP, codec.TIFF, codec.APNG, codec.GIF}

func checkInvariants(t *testing.T, m *Metadata) {
	t.Helper()
	if m == nil {
		t.Fatal("nil metadata")
	}
	if m.Width < 0 || m.Height < 0 || m.Width > maxDimension || m.Height > maxDimension {
		t.Fatalf("size %dx%d", m.Width, m.Height)
	}
	_ = AspectRatio(m.Width, m.Height)
	if x := m.Exif; x != nil {
		if x.ISO < 0 || x.ISO > maxISO || x.FocalLength35 < 0 || x.PixelWidth < 0 || x.PixelHeight < 0 {
			t.Fatalf("exif %+v", *x)
		}
		if x.Orientation != 0 && !x.Orientation.Valid() {
			t.Fatalf("orientation %d", x.Orientation)
		}
		if x.Exposure.Den == 0 && x.Exposure.Num != 0 {
			t.Fatalf("exposure %+v", x.Exposure)
		}
		_ = x.Exposure.String() + x.FNumber.String() + x.FNumber.FNumberString() + x.FocalLength.String()
		if len(x.Make) > maxTextBytes || len(x.Model) > maxTextBytes || len(x.Lens) > maxTextBytes || len(x.Software) > maxTextBytes {
			t.Fatal("unbounded text")
		}
	}
	if g := m.GPS; g != nil {
		if g.Lat < -90 || g.Lat > 90 || g.Lon < -180 || g.Lon > 180 || g.DMS() == "" || g.Decimal() == "" {
			t.Fatalf("gps %+v", *g)
		}
	}
	if c := m.ICC; c != nil {
		if c.Size < 128 || c.Size > MaxICCBytes || len(c.Description) > iccMaxDescBytes {
			t.Fatalf("icc %+v", *c)
		}
	}
	if x := m.XMP; x != nil {
		if x.Rating < -1 || x.Rating > 5 || len(x.Creator) > xmpMaxCreators || len(x.Title) > xmpMaxText || len(x.Description) > xmpMaxText {
			t.Fatalf("xmp %+v", *x)
		}
	}
}

func FuzzRead(f *testing.F) {
	for _, e := range []enc{le, be} {
		j, _ := fullJPEG(f, e, false)
		p, _ := fullPNG(f, e, true)
		w, _ := fullWebP(f, e, false)
		tf, _ := fullTIFF(f, e, true)
		f.Add(j, uint8(0))
		f.Add(p, uint8(1))
		f.Add(w, uint8(2))
		f.Add(tf, uint8(3))
		f.Add(p, uint8(4))
		f.Add(tf, uint8(5))
	}
	f.Add(loopTIFF(le), uint8(3))
	f.Add([]byte(xmpSample), uint8(0))
	f.Add(sRGBProfile(), uint8(3))
	f.Add([]byte{}, uint8(0))
	f.Fuzz(func(t *testing.T, data []byte, sel uint8) {
		id := fuzzIDs[int(sel)%len(fuzzIDs)]
		start := time.Now()
		m, _ := Read(context.Background(), bytes.NewReader(data), int64(len(data)), id)
		if elapsed := time.Since(start); elapsed > 5*time.Second {
			t.Fatalf("Read took %v", elapsed)
		}
		checkInvariants(t, m)
		checkInvariants(t, &Metadata{Format: id, XMP: parseXMP(data), ICC: parseICC(data)})
	})
}
