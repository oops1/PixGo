package tiff

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"image"
	"io"
	"math/rand"
	"runtime"
	"runtime/debug"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/testimg"
)

type hookReader struct {
	r      io.ReaderAt
	reads  atomic.Int64
	onRead func(count int64)
}

func (h *hookReader) ReadAt(p []byte, off int64) (int, error) {
	count := h.reads.Add(1)
	if h.onRead != nil {
		h.onRead(count)
	}
	return h.r.ReadAt(p, off)
}

func limitsWith(maxPixels, maxTiled int64) codec.Limits {
	lim := codec.DefaultLimits()
	lim.MaxPixels = maxPixels
	lim.MaxTiledPixels = maxTiled
	return lim
}

func allocatedBy(fn func()) uint64 {
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	fn()
	runtime.ReadMemStats(&after)
	return after.TotalAlloc - before.TotalAlloc
}

func settledGoroutines(limit int) int {
	for i := 0; i < 100; i++ {
		if n := runtime.NumGoroutine(); n <= limit {
			return n
		}
		time.Sleep(10 * time.Millisecond)
	}
	return runtime.NumGoroutine()
}

func TestProbeReportsInfoWithTooLargeForTiledSizedPictures(t *testing.T) {
	f := kinds[3].fixture(2000, 50, 1)
	f.claimH = 1500
	f.rowsPerStrip = 50
	f.alias = true
	data := f.bytes()
	ctx := context.Background()
	d := Decoder{}
	info, err := d.Probe(ctx, bytes.NewReader(data), int64(len(data)), limitsWith(1_000_000, 0))
	if !errors.Is(err, codec.ErrTooLarge) || info.Width != 2000 || info.Height != 1500 || info.Format != codec.TIFF || info.BitDepth != 8 {
		t.Fatalf("%+v %v", info, err)
	}
	info, err = d.Probe(ctx, bytes.NewReader(data), int64(len(data)), limitsWith(1_000_000, 2_000_000))
	if !errors.Is(err, codec.ErrTooLarge) || info != (codec.Info{}) {
		t.Fatalf("beyond the tiled limit: %+v %v", info, err)
	}
	info, err = d.Probe(ctx, bytes.NewReader(data), int64(len(data)), limitsWith(4_000_000, 0))
	if err != nil || info.Width != 2000 {
		t.Fatalf("within limits: %+v %v", info, err)
	}
	if _, err := d.Decode(ctx, bytes.NewReader(data), int64(len(data)), codec.Options{Limits: limitsWith(1_000_000, 0)}); !errors.Is(err, codec.ErrTooLarge) {
		t.Fatalf("full decode above MaxPixels: %v", err)
	}
	img, err := d.Decode(ctx, bytes.NewReader(data), int64(len(data)), codec.Options{Limits: limitsWith(1_000_000, 0), Scale: 8})
	if err != nil {
		t.Fatalf("scaled decode above MaxPixels: %v", err)
	}
	if img.Scale != 8 || img.First().Rect.Dx() != 250 || img.First().Rect.Dy() != 188 || img.Info.Width != 2000 {
		t.Fatalf("scale %d size %v info %+v", img.Scale, img.First().Rect, img.Info)
	}
	region := image.Rect(100, 100, 400, 300)
	img, err = d.Decode(ctx, bytes.NewReader(data), int64(len(data)), codec.Options{Limits: limitsWith(1_000_000, 0), Region: &region})
	if err != nil || img.First().Rect.Dx() != 300 {
		t.Fatalf("region decode above MaxPixels: %v", err)
	}
	if _, err := d.Decode(ctx, bytes.NewReader(data), int64(len(data)), codec.Options{Limits: limitsWith(1_000_000, 2_000_000), Scale: 8}); !errors.Is(err, codec.ErrTooLarge) {
		t.Fatalf("input beyond MaxTiledPixels: %v", err)
	}
}

func peakHeap(fn func()) uint64 {
	previous := debug.SetGCPercent(20)
	defer debug.SetGCPercent(previous)
	runtime.GC()
	var baseline runtime.MemStats
	runtime.ReadMemStats(&baseline)
	var peak atomic.Uint64
	stop, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		var stats runtime.MemStats
		for {
			runtime.ReadMemStats(&stats)
			if stats.HeapAlloc > peak.Load() {
				peak.Store(stats.HeapAlloc)
			}
			select {
			case <-stop:
				return
			case <-time.After(2 * time.Millisecond):
			}
		}
	}()
	fn()
	close(stop)
	<-done
	if peak.Load() < baseline.HeapAlloc {
		return 0
	}
	return peak.Load() - baseline.HeapAlloc
}

func TestScaledDecodeOfHugePictureNeverBuildsTheFullResolutionImage(t *testing.T) {
	f := kinds[3].fixture(8000, 16, 2)
	f.claimH = 8000
	f.rowsPerStrip = 8
	f.alias = true
	for _, tiled := range []bool{false, true} {
		if tiled {
			f.rowsPerStrip, f.tileW, f.tileH = 0, 256, 256
		}
		data := f.bytes()
		var img *codec.Image
		var err error
		peak := peakHeap(func() {
			img, err = Decoder{}.Decode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Options{Limits: limitsWith(20_000_000, 0), Scale: 8})
		})
		if err != nil {
			t.Fatal(err)
		}
		if img.First().Rect.Dx() != 1000 || img.Scale != 8 {
			t.Fatalf("%v scale %d", img.First().Rect, img.Scale)
		}
		t.Logf("tiled=%v: peak heap %d MiB", tiled, peak>>20)
		if peak > 40<<20 {
			t.Fatalf("tiled=%v: decoding a 64 Mpx picture at scale 8 peaked at %d MiB of heap", tiled, peak>>20)
		}
	}
}

func TestOversizedResultIsRefusedBeforeAllocating(t *testing.T) {
	f := kinds[3].fixture(30000, 16, 3)
	f.claimH = 30000
	f.rowsPerStrip = 8
	f.alias = true
	data := f.bytes()
	lim := limitsWith(0, 2_000_000_000)
	rd, err := Decoder{}.OpenTiled(context.Background(), bytes.NewReader(data), int64(len(data)), lim)
	if err != nil {
		t.Fatal(err)
	}
	defer rd.Close()
	allocated := allocatedBy(func() {
		if _, err = rd.Overview(context.Background(), 0); !errors.Is(err, codec.ErrTooLarge) {
			t.Errorf("Overview(0): %v", err)
		}
		if _, err = rd.Region(context.Background(), 0, image.Rect(0, 0, 30000, 30000)); !errors.Is(err, codec.ErrTooLarge) {
			t.Errorf("Region: %v", err)
		}
	})
	if allocated > 64<<20 {
		t.Fatalf("allocated %d MiB", allocated>>20)
	}
}

func TestOpenTiledLimits(t *testing.T) {
	f := kinds[12].fixture(100, 100, 4)
	data := f.bytes()
	ctx := context.Background()
	d := Decoder{}
	if _, err := d.OpenTiled(ctx, bytes.NewReader(data), int64(len(data)), limitsWith(0, 9999)); !errors.Is(err, codec.ErrTooLarge) {
		t.Fatalf("pixels: %v", err)
	}
	lim := codec.DefaultLimits()
	lim.MaxFileBytes = 100
	if _, err := d.OpenTiled(ctx, bytes.NewReader(data), int64(len(data)), lim); !errors.Is(err, codec.ErrTooLarge) {
		t.Fatalf("file size: %v", err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := d.OpenTiled(canceled, bytes.NewReader(data), int64(len(data)), codec.DefaultLimits()); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled: %v", err)
	}
	if rd, err := d.OpenTiled(ctx, bytes.NewReader(data), int64(len(data)), limitsWith(10, 0)); err != nil {
		t.Fatalf("OpenTiled must ignore MaxPixels: %v", err)
	} else {
		rd.Close()
	}
}

func TestOpenTiledRejectsUnsupportedLayouts(t *testing.T) {
	cases := map[string]func(f *fixture){
		"planar": func(f *fixture) { f.planar = 2 },
		"jpeg":   func(f *fixture) { f.compression = 7 },
		"ccitt": func(f *fixture) {
			f.compression = 4
			f.bits = 1
			f.samples = 1
			f.photometric = 0
			f.values = make([]uint16, f.width*f.height)
		},
		"cmyk":        func(f *fixture) { f.photometric = 5; f.samples = 4; f.values = make([]uint16, f.width*f.height*4) },
		"lab":         func(f *fixture) { f.photometric = 8 },
		"ycbcr":       func(f *fixture) { f.photometric = 6 },
		"float":       func(f *fixture) { f.sampleFormat = 3 },
		"predictor 3": func(f *fixture) { f.predictor = 3 },
		"odd tiles":   func(f *fixture) { f.tileW, f.tileH = 24, 24 },
		"extra":       func(f *fixture) { f.samples, f.extra = 5, 2; f.values = make([]uint16, f.width*f.height*5) },
		"32 bit":      func(f *fixture) { f.bits = 32 },
		"mixed depth": func(f *fixture) { f.custom = []rawTag{{0x102, 3, []uint64{8, 8, 16}}} },
		"unspecified": func(f *fixture) { f.samples, f.extra = 4, 0; f.values = make([]uint16, f.width*f.height*4) },
		"fill order": func(f *fixture) {
			f.bits, f.samples, f.photometric, f.fillOrder = 1, 1, 1, 2
			f.values = make([]uint16, f.width*f.height)
		},
		"pal16": func(f *fixture) {
			f.bits, f.samples, f.photometric = 16, 1, 3
			f.values = make([]uint16, f.width*f.height)
		},
		"samples": func(f *fixture) { f.custom = []rawTag{{0x115, 3, []uint64{4}}} },
	}
	for name, mutate := range cases {
		f := kinds[12].fixture(20, 20, 5)
		mutate(&f)
		data := f.bytes()
		_, err := Decoder{}.OpenTiled(context.Background(), bytes.NewReader(data), int64(len(data)), codec.DefaultLimits())
		if !errors.Is(err, codec.ErrUnsupported) {
			t.Errorf("%s: OpenTiled %v", name, err)
		}
		lim := limitsWith(100, 0)
		if _, err := (Decoder{}).Probe(context.Background(), bytes.NewReader(data), int64(len(data)), lim); !errors.Is(err, codec.ErrUnsupported) {
			t.Errorf("%s: Probe above MaxPixels %v", name, err)
		}
		if _, err := (Decoder{}).Decode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Options{Limits: lim}); !errors.Is(err, codec.ErrUnsupported) {
			t.Errorf("%s: Decode above MaxPixels %v", name, err)
		}
	}
}

func TestUnsupportedLayoutsWithinLimitsFallBackToXImage(t *testing.T) {
	f := kinds[12].fixture(40, 30, 6)
	f.tileW, f.tileH = 24, 24
	data := f.bytes()
	info, err := probe(data)
	if err != nil || info.Width != 40 || info.BitDepth != 24 {
		t.Fatalf("%+v %v", info, err)
	}
	img, err := decode(data)
	if err != nil {
		t.Fatal(err)
	}
	requireIdentical(t, img.First(), f.expected(), "fallback")
	f = kinds[12].fixture(8, 8, 6)
	f.photometric = 5
	f.samples = 4
	f.values = make([]uint16, 8*8*4)
	if _, err := decode(f.bytes()); !errors.Is(err, codec.ErrUnsupported) {
		t.Fatalf("CMYK: %v", err)
	}
}

func TestTileDimensionsLargerThanThePictureAreRefused(t *testing.T) {
	f := kinds[3].fixture(20, 20, 7)
	f.tileW, f.tileH = 4096, 4096
	data := f.bytes()
	if _, err := (Decoder{}).OpenTiled(context.Background(), bytes.NewReader(data), int64(len(data)), codec.DefaultLimits()); !errors.Is(err, codec.ErrCorrupt) {
		t.Fatalf("%v", err)
	}
	f.tileW, f.tileH = 1024, 1024
	rd := openTiled(t, f.bytes())
	got, err := rd.Overview(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	requireIdentical(t, got, f.expected(), "single oversized tile")
}

func TestChunkTablesAreValidatedWithoutReadingPixels(t *testing.T) {
	reads := func(f fixture) (*hookReader, []byte) {
		data := f.bytes()
		return &hookReader{r: bytes.NewReader(data)}, data
	}
	f := kinds[12].fixture(64, 64, 8)
	f.compression = 8
	f.tileW, f.tileH = 16, 16
	source, data := reads(f)
	rd, err := Decoder{}.OpenTiled(context.Background(), source, int64(len(data)), codec.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	rd.Close()
	if n := source.reads.Load(); n > 12 {
		t.Fatalf("OpenTiled made %d reads", n)
	}
	for _, mutate := range []func(f *fixture){
		func(f *fixture) { f.omit = []uint16{0x145} },
		func(f *fixture) { f.omit = []uint16{0x144} },
		func(f *fixture) { f.custom = []rawTag{{0x144, 4, []uint64{8, 8}}} },
		func(f *fixture) { f.custom = []rawTag{{0x145, 4, []uint64{4, 4, 4}}} },
	} {
		g := f
		mutate(&g)
		data := g.bytes()
		if _, err := (Decoder{}).OpenTiled(context.Background(), bytes.NewReader(data), int64(len(data)), codec.DefaultLimits()); !errors.Is(err, codec.ErrCorrupt) {
			t.Fatalf("inconsistent tables accepted: %v", err)
		}
	}
	g := f
	g.compression = 1
	data = g.bytes()
	patched := append([]byte(nil), data...)
	o := g.byteOrder()
	ifd := int(o.Uint32(patched[4:]))
	entries := int(o.Uint16(patched[ifd:]))
	for i := 0; i < entries; i++ {
		at := ifd + 2 + 12*i
		if o.Uint16(patched[at:]) == 0x145 {
			table := int(o.Uint32(patched[at+8:]))
			o.PutUint32(patched[table:], 10)
		}
	}
	if _, err := (Decoder{}).OpenTiled(context.Background(), bytes.NewReader(patched), int64(len(patched)), codec.DefaultLimits()); !errors.Is(err, codec.ErrCorrupt) {
		t.Fatalf("short uncompressed chunk accepted: %v", err)
	}
}

func patchTag(t *testing.T, data []byte, o order, tag uint16, value uint32) {
	t.Helper()
	ifd := int(o.Uint32(data[4:]))
	entries := int(o.Uint16(data[ifd:]))
	for i := 0; i < entries; i++ {
		at := ifd + 2 + 12*i
		if o.Uint16(data[at:]) != tag {
			continue
		}
		if o.Uint16(data[at+2:]) == 3 {
			o.PutUint16(data[at+8:], uint16(value))
			return
		}
		o.PutUint32(data[at+8:], value)
		return
	}
	t.Fatalf("tag %#x not found", tag)
}

func TestImpossibleExpansionIsRejectedWhenOpening(t *testing.T) {
	for _, compression := range []int{1, 5, 8, 32773} {
		f := kinds[12].fixture(64, 64, 40)
		f.compression = compression
		f.tileW, f.tileH = 16, 16
		data := f.bytes()
		patchTag(t, data, f.byteOrder(), 0x100, 16384)
		patchTag(t, data, f.byteOrder(), 0x101, 16384)
		patchTag(t, data, f.byteOrder(), 0x142, 4096)
		patchTag(t, data, f.byteOrder(), 0x143, 4096)
		allocated := allocatedBy(func() {
			rd, err := Decoder{}.OpenTiled(context.Background(), bytes.NewReader(data), int64(len(data)), codec.DefaultLimits())
			if err == nil {
				rd.Close()
				t.Errorf("compression %d: bomb accepted", compression)
				return
			}
			if !errors.Is(err, codec.ErrCorrupt) {
				t.Errorf("compression %d: %v", compression, err)
			}
		})
		if allocated > 16<<20 {
			t.Errorf("compression %d: allocated %d MiB", compression, allocated>>20)
		}
	}
}

func TestStreamsLongerThanTheChunkAreNotExpanded(t *testing.T) {
	zeros := make([]byte, 20<<20)
	for _, compression := range []int{1, 5, 8, 32773} {
		f := fixture{width: 10, height: 10, bits: 8, samples: 1, photometric: 1, compression: compression}
		f.fill(1)
		f.rawOverride = zeros
		data := f.bytes()
		var err error
		var img *codec.Image
		allocated := allocatedBy(func() {
			img, err = Decoder{}.Decode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Options{Limits: codec.DefaultLimits()})
		})
		if err != nil {
			t.Fatalf("compression %d: %v", compression, err)
		}
		if img.First().NRGBAAt(3, 3).R != 0 {
			t.Fatalf("compression %d: wrong pixels", compression)
		}
		if allocated > 8<<20 {
			t.Fatalf("compression %d: allocated %d MiB for a 100 pixel picture", compression, allocated>>20)
		}
	}
}

func TestHeaderClaimsAreCheckedBeforeAllocating(t *testing.T) {
	cases := map[string]func(t *testing.T, data []byte, o order){
		"huge dimensions": func(t *testing.T, data []byte, o order) {
			patchTag(t, data, o, 0x100, 1<<30)
			patchTag(t, data, o, 0x101, 1<<30)
		},
		"wide strips": func(t *testing.T, data []byte, o order) {
			patchTag(t, data, o, 0x100, 1<<29)
			patchTag(t, data, o, 0x101, 1)
		},
		"too many strips": func(t *testing.T, data []byte, o order) {
			patchTag(t, data, o, 0x100, 1)
			patchTag(t, data, o, 0x101, 1<<29)
			patchTag(t, data, o, 0x116, 1)
		},
		"one billion pixels in one row": func(t *testing.T, data []byte, o order) {
			patchTag(t, data, o, 0x100, 1_000_000_000)
			patchTag(t, data, o, 0x101, 1)
		},
		"dimension beyond int32": func(t *testing.T, data []byte, o order) {
			patchTag(t, data, o, 0x100, 1<<31)
			patchTag(t, data, o, 0x101, 1)
		},
		"huge strip table": func(t *testing.T, data []byte, o order) {
			patchTag(t, data, o, 0x101, 1<<20)
			patchTag(t, data, o, 0x116, 1)
		},
	}
	for name, mutate := range cases {
		f := kinds[12].fixture(8, 8, 9)
		data := f.bytes()
		mutate(t, data, f.byteOrder())
		allocated := allocatedBy(func() {
			testimg.Within(t, 10*time.Second, func() {
				if rd, err := (Decoder{}).OpenTiled(context.Background(), bytes.NewReader(data), int64(len(data)), codec.DefaultLimits()); err == nil {
					rd.Close()
					t.Errorf("%s: accepted", name)
				} else {
					testimg.RequireCodecError(t, err)
				}
				if _, err := (Decoder{}).Probe(context.Background(), bytes.NewReader(data), int64(len(data)), codec.DefaultLimits()); err != nil {
					testimg.RequireCodecError(t, err)
				}
				if _, err := (Decoder{}).Decode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Options{Limits: codec.DefaultLimits()}); err == nil {
					t.Errorf("%s: decoded", name)
				} else {
					testimg.RequireCodecError(t, err)
				}
			})
		})
		if allocated > 64<<20 {
			t.Errorf("%s: allocated %d MiB", name, allocated>>20)
		}
	}
}

func TestCorruptBytesNeverPanicOrEscapeTheErrorTaxonomy(t *testing.T) {
	rng := rand.New(rand.NewSource(21))
	picks := []struct{ kind, variant, compression int }{{12, 5, 8}, {12, 2, 5}, {14, 3, 32773}, {3, 7, 1}, {10, 6, 8}, {16, 5, 5}}
	for _, p := range picks {
		f := kinds[p.kind].fixture(50, 40, 22)
		f.compression = p.compression
		variants[p.variant].apply(&f)
		if p.kind != 10 && kinds[p.kind].bits >= 8 {
			f.predictor = 2
		}
		for _, bigTIFF := range []bool{false, true} {
			f.bigTIFF = bigTIFF
			good := f.bytes()
			for i := 0; i < 100; i++ {
				data := append([]byte(nil), good...)
				for k := 0; k < 1+rng.Intn(5); k++ {
					data[rng.Intn(len(data))] = byte(rng.Intn(256))
				}
				exerciseAll(t, data)
			}
		}
	}
}

func exerciseAll(t testing.TB, data []byte) {
	t.Helper()
	ctx := context.Background()
	lim := testimg.SmallLimits()
	lim.MaxTiledPixels = 1 << 24
	d := Decoder{}
	d.Sniff(data)
	if _, err := d.Probe(ctx, bytes.NewReader(data), int64(len(data)), lim); err != nil && !isTaxonomy(err) {
		t.Fatalf("Probe: %v", err)
	}
	region := image.Rect(3, 2, 70, 41)
	for _, o := range []codec.Options{{Limits: lim}, {Limits: lim, Scale: 8}, {Limits: lim, Scale: 2, Region: &region}} {
		img, err := d.Decode(ctx, bytes.NewReader(data), int64(len(data)), o)
		if err != nil && !isTaxonomy(err) {
			t.Fatalf("Decode: %v", err)
		}
		if err == nil && (img == nil || img.First() == nil) {
			t.Fatal("Decode succeeded without pixels")
		}
		o.Deep = true
		img, err = d.Decode(ctx, bytes.NewReader(data), int64(len(data)), o)
		if err != nil && !isTaxonomy(err) {
			t.Fatalf("Decode deep: %v", err)
		}
		if err == nil && (img == nil || len(img.Frames) == 0 || img.Frames[0].Pix == nil && img.Frames[0].Deep == nil) {
			t.Fatal("deep Decode succeeded without pixels")
		}
	}
	rd, err := d.OpenTiled(ctx, bytes.NewReader(data), int64(len(data)), lim)
	if err != nil {
		if !isTaxonomy(err) {
			t.Fatalf("OpenTiled: %v", err)
		}
		return
	}
	defer rd.Close()
	info := rd.Info()
	for _, shift := range []int{0, 1, 3, 5} {
		lw, lh := codec.LevelSize(info.Width, info.Height, shift)
		if int64(lw)*int64(lh) > 1<<22 {
			continue
		}
		if _, err := rd.Overview(ctx, shift); err != nil && !isTaxonomy(err) {
			t.Fatalf("Overview(%d): %v", shift, err)
		}
		if _, err := rd.Region(ctx, shift, image.Rect(lw/3, lh/4, lw/3+40, lh/4+30)); err != nil && !isTaxonomy(err) {
			t.Fatalf("Region(%d): %v", shift, err)
		}
		if deep, ok := rd.(codec.DeepTileReader); ok && deep.Deep() {
			if _, err := deep.OverviewDeep(ctx, shift); err != nil && !isTaxonomy(err) {
				t.Fatalf("OverviewDeep(%d): %v", shift, err)
			}
			if _, err := deep.RegionDeep(ctx, shift, image.Rect(lw/3, lh/4, lw/3+40, lh/4+30)); err != nil && !isTaxonomy(err) {
				t.Fatalf("RegionDeep(%d): %v", shift, err)
			}
		}
	}
}

func isTaxonomy(err error) bool {
	return errors.Is(err, codec.ErrCorrupt) || errors.Is(err, codec.ErrUnsupported) || errors.Is(err, codec.ErrTooLarge) || errors.Is(err, context.Canceled)
}

func TestEveryTruncatedPrefixOfLayoutsFails(t *testing.T) {
	picks := []struct{ kind, variant, compression int }{{12, 5, 8}, {14, 2, 5}, {3, 3, 32773}, {0, 0, 1}}
	for _, p := range picks {
		for _, bigTIFF := range []bool{false, true} {
			f := kinds[p.kind].fixture(21, 17, 30)
			f.compression, f.bigTIFF = p.compression, bigTIFF
			variants[p.variant].apply(&f)
			data := f.bytes()
			nextPointer := 4
			if bigTIFF {
				nextPointer = 8
			}
			for n := 0; n < len(data)-nextPointer; n++ {
				prefix := data[:n]
				if _, err := (Decoder{}).Decode(context.Background(), bytes.NewReader(prefix), int64(n), codec.Options{Limits: codec.DefaultLimits()}); err == nil {
					t.Fatalf("prefix %d/%d of %+v decoded", n, len(data), p)
				} else {
					testimg.RequireCodecError(t, err)
				}
				if rd, err := (Decoder{}).OpenTiled(context.Background(), bytes.NewReader(prefix), int64(n), codec.DefaultLimits()); err == nil {
					rd.Close()
					t.Fatalf("prefix %d/%d of %+v opened", n, len(data), p)
				}
			}
			img, err := decode(data[:len(data)-nextPointer])
			if err != nil {
				t.Fatalf("a file without the next-directory pointer must decode: %v", err)
			}
			requireIdentical(t, img.First(), f.expected(), "without the trailing pointer")
		}
	}
}

func TestTruncatedChunkDataWithIntactDirectory(t *testing.T) {
	for _, compression := range []int{1, 5, 8, 32773} {
		f := kinds[12].fixture(40, 40, 31)
		f.compression = compression
		f.tileW, f.tileH = 16, 16
		data := f.bytes()
		o := f.byteOrder()
		ifd := int(o.Uint32(data[4:]))
		entries := int(o.Uint16(data[ifd:]))
		var offsets, counts int
		for i := 0; i < entries; i++ {
			at := ifd + 2 + 12*i
			switch o.Uint16(data[at:]) {
			case 0x144:
				offsets = int(o.Uint32(data[at+8:]))
			case 0x145:
				counts = int(o.Uint32(data[at+8:]))
			}
		}
		for chunk := 0; chunk < 9; chunk++ {
			mutated := append([]byte(nil), data...)
			start := int(o.Uint32(mutated[offsets+4*chunk:]))
			length := int(o.Uint32(mutated[counts+4*chunk:]))
			for i := start + length/2; i < start+length; i++ {
				mutated[i] = 0xa5
			}
			rd, err := Decoder{}.OpenTiled(context.Background(), bytes.NewReader(mutated), int64(len(mutated)), codec.DefaultLimits())
			if err != nil {
				t.Fatal(err)
			}
			_, err = rd.Overview(context.Background(), 0)
			if compression == 1 {
				if err != nil {
					t.Fatalf("uncompressed garbage must decode: %v", err)
				}
			} else if err != nil {
				testimg.RequireCodecError(t, err)
			}
			rd.Close()
		}
	}
}

func TestPaletteIndexBeyondTheColorMapIsCorrupt(t *testing.T) {
	f := kinds[11].fixture(16, 16, 32)
	f.palette = f.palette[:0]
	words := makePalette(8, 1)
	colors := 100
	f.palette = append(append(append([]uint16(nil), words[:colors]...), words[256:256+colors]...), words[512:512+colors]...)
	for i := range f.values {
		f.values[i] %= 100
	}
	f.values[37] = 150
	data := f.bytes()
	rd := openTiled(t, data)
	if _, err := rd.Overview(context.Background(), 0); !errors.Is(err, codec.ErrCorrupt) {
		t.Fatalf("overview: %v", err)
	}
	if _, err := decode(data); !errors.Is(err, codec.ErrCorrupt) {
		t.Fatalf("decode: %v", err)
	}
	f.values[37] = 20
	rd = openTiled(t, f.bytes())
	if _, err := rd.Overview(context.Background(), 0); err != nil {
		t.Fatal(err)
	}
	f.palette = nil
	if _, err := (Decoder{}).OpenTiled(context.Background(), bytes.NewReader(f.bytes()), int64(len(f.bytes())), codec.DefaultLimits()); !errors.Is(err, codec.ErrCorrupt) {
		t.Fatalf("missing palette: %v", err)
	}
}

func TestCancellationStopsLongDecodesWithoutLeaks(t *testing.T) {
	f := kinds[12].fixture(1500, 1500, 33)
	f.compression = 8
	f.tileW, f.tileH = 64, 64
	data := f.bytes()
	base := runtime.NumGoroutine()
	for _, cancelAt := range []int64{3, 40, 200} {
		ctx, cancel := context.WithCancel(context.Background())
		source := &hookReader{r: bytes.NewReader(data)}
		rd, err := Decoder{}.OpenTiled(ctx, source, int64(len(data)), codec.DefaultLimits())
		if err != nil {
			t.Fatal(err)
		}
		start := source.reads.Load()
		source.onRead = func(count int64) {
			if count-start == cancelAt {
				cancel()
			}
		}
		_, err = rd.Overview(ctx, 0)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancel at %d: %v", cancelAt, err)
		}
		rd.Close()
		cancel()
	}
	if n := settledGoroutines(base + 2); n > base+2 {
		t.Fatalf("goroutines grew from %d to %d", base, n)
	}
}

func TestReaderOutlivesTheOpeningContext(t *testing.T) {
	f := kinds[12].fixture(100, 100, 34)
	f.compression = 5
	f.tileW, f.tileH = 32, 32
	data := f.bytes()
	ctx, cancel := context.WithCancel(context.Background())
	rd, err := Decoder{}.OpenTiled(ctx, bytes.NewReader(data), int64(len(data)), codec.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	region, err := rd.Region(context.Background(), 1, image.Rect(0, 0, 40, 40))
	if err != nil {
		t.Fatal(err)
	}
	if region.Rect.Dx() != 40 {
		t.Fatalf("%v", region.Rect)
	}
	canceled, stop := context.WithCancel(context.Background())
	stop()
	if _, err := rd.Region(canceled, 1, image.Rect(0, 0, 40, 40)); !errors.Is(err, context.Canceled) {
		t.Fatalf("%v", err)
	}
}

func TestClosedReaderRefusesWorkAndCloseIsRepeatable(t *testing.T) {
	f := kinds[12].fixture(50, 50, 35)
	data := f.bytes()
	rd, err := Decoder{}.OpenTiled(context.Background(), bytes.NewReader(data), int64(len(data)), codec.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rd.Overview(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	if err := rd.Close(); err != nil {
		t.Fatal(err)
	}
	if err := rd.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := rd.Region(context.Background(), 0, image.Rect(0, 0, 4, 4)); err == nil {
		t.Fatal("closed reader served a region")
	}
	if _, err := rd.Overview(context.Background(), 0); err == nil {
		t.Fatal("closed reader served an overview")
	}
	if rd.MinShift() != 0 {
		t.Fatalf("MinShift %d", rd.MinShift())
	}
}

func TestCloseDuringWorkStopsIt(t *testing.T) {
	f := kinds[12].fixture(1200, 1200, 36)
	f.compression = 8
	f.tileW, f.tileH = 32, 32
	data := f.bytes()
	source := &hookReader{r: bytes.NewReader(data)}
	rd, err := Decoder{}.OpenTiled(context.Background(), source, int64(len(data)), codec.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	start := source.reads.Load()
	source.onRead = func(count int64) {
		if count-start == 30 {
			rd.Close()
		}
	}
	if _, err := rd.Overview(context.Background(), 0); err == nil {
		t.Fatal("overview finished on a closed reader")
	}
}

func TestPanickingSourceIsContained(t *testing.T) {
	f := kinds[12].fixture(100, 100, 37)
	f.tileW, f.tileH = 32, 32
	data := f.bytes()
	source := &hookReader{r: bytes.NewReader(data)}
	rd, err := Decoder{}.OpenTiled(context.Background(), source, int64(len(data)), codec.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	source.onRead = func(int64) { panic("disk on fire") }
	if _, err := rd.Overview(context.Background(), 0); !errors.Is(err, codec.ErrCorrupt) {
		t.Fatalf("%v", err)
	}
	if _, err := rd.Region(context.Background(), 0, image.Rect(0, 0, 50, 50)); !errors.Is(err, codec.ErrCorrupt) {
		t.Fatalf("%v", err)
	}
}

func TestSourceFailuresAreClassified(t *testing.T) {
	f := kinds[12].fixture(100, 100, 38)
	f.compression = 8
	f.tileW, f.tileH = 32, 32
	data := f.bytes()
	counter := &hookReader{r: bytes.NewReader(data)}
	rd, err := Decoder{}.OpenTiled(context.Background(), counter, int64(len(data)), codec.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rd.Overview(context.Background(), 0); err != nil {
		t.Fatal(err)
	}
	rd.Close()
	total := counter.reads.Load()
	failing := errors.New("device removed")
	for failAfter := int64(0); failAfter < total; failAfter++ {
		source := &brokenReader{r: bytes.NewReader(data), failAfter: failAfter, err: failing}
		rd, err := Decoder{}.OpenTiled(context.Background(), source, int64(len(data)), codec.DefaultLimits())
		if err == nil {
			_, err = rd.Overview(context.Background(), 0)
			rd.Close()
		}
		if err == nil {
			t.Fatalf("failure after %d of %d reads ignored", failAfter, total)
		}
		testimg.RequireCodecError(t, err)
	}
}

type brokenReader struct {
	r         io.ReaderAt
	reads     atomic.Int64
	failAfter int64
	err       error
}

func (b *brokenReader) ReadAt(p []byte, off int64) (int, error) {
	if b.reads.Add(1) > b.failAfter {
		return 0, b.err
	}
	return b.r.ReadAt(p, off)
}

func TestHeaderByteOrderAndMagicVariants(t *testing.T) {
	f := kinds[3].fixture(8, 8, 39)
	data := f.bytes()
	for name, mutate := range map[string]func(b []byte){
		"bad order": func(b []byte) { b[0], b[1] = 'X', 'X' },
		"bad magic": func(b []byte) { binary.LittleEndian.PutUint16(b[2:], 44) },
		"ifd at 0":  func(b []byte) { binary.LittleEndian.PutUint32(b[4:], 0) },
		"ifd out":   func(b []byte) { binary.LittleEndian.PutUint32(b[4:], 0xfffffff0) },
	} {
		mutated := append([]byte(nil), data...)
		mutate(mutated)
		if _, err := decode(mutated); err == nil {
			t.Errorf("%s: decoded", name)
		} else {
			testimg.RequireCodecError(t, err)
		}
		if _, err := probe(mutated); err == nil {
			t.Errorf("%s: probed", name)
		}
	}
	big := f
	big.bigTIFF = true
	bigData := big.bytes()
	for name, mutate := range map[string]func(b []byte){
		"offset size": func(b []byte) { binary.LittleEndian.PutUint16(b[4:], 4) },
		"reserved":    func(b []byte) { binary.LittleEndian.PutUint16(b[6:], 1) },
		"ifd out":     func(b []byte) { binary.LittleEndian.PutUint64(b[8:], 1<<40) },
		"short file":  func(b []byte) {},
	} {
		mutated := append([]byte(nil), bigData...)
		mutate(mutated)
		if name == "short file" {
			mutated = mutated[:12]
		}
		if _, err := decode(mutated); err == nil {
			t.Errorf("%s: decoded", name)
		} else {
			testimg.RequireCodecError(t, err)
		}
	}
}
