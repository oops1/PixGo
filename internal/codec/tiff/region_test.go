package tiff

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"io"
	"math/rand"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	xtiff "golang.org/x/image/tiff"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/convert"
	"github.com/oops1/PixGo/internal/codec/internal/reduce"
)

func cropOf(src *image.NRGBA, r image.Rectangle) *image.NRGBA {
	r = r.Canon().Intersect(src.Rect)
	if r.Empty() {
		return image.NewNRGBA(image.Rectangle{})
	}
	dst := image.NewNRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	for y := 0; y < r.Dy(); y++ {
		from := src.PixOffset(r.Min.X, r.Min.Y+y)
		copy(dst.Pix[y*dst.Stride:], src.Pix[from:from+r.Dx()*4])
	}
	return dst
}

func randomRect(rng *rand.Rand, w, h int) image.Rectangle {
	x0, y0 := rng.Intn(w+6)-3, rng.Intn(h+6)-3
	return image.Rect(x0, y0, x0+rng.Intn(w+4), y0+rng.Intn(h+4))
}

func levelsOf(base *image.NRGBA, count int) []*image.NRGBA {
	levels := []*image.NRGBA{base}
	for i := 1; i <= count; i++ {
		levels = append(levels, reduce.Halve(levels[i-1]))
	}
	return levels
}

type regionCase struct {
	name        string
	kind        kind
	variant     variant
	compression int
	predictor   int
}

func regionCases() []regionCase {
	var cases []regionCase
	picks := []struct {
		kind        int
		variant     int
		compression int
		predictor   int
	}{
		{12, 5, 1, 0}, {12, 5, 8, 2}, {12, 8, 5, 2}, {12, 0, 8, 0}, {12, 2, 5, 0}, {12, 3, 32773, 2}, {12, 1, 1, 0}, {12, 4, 8, 0},
		{14, 5, 8, 2}, {14, 6, 1, 0}, {14, 2, 32773, 0}, {15, 7, 5, 0}, {15, 3, 1, 0},
		{3, 5, 8, 2}, {3, 2, 5, 0}, {4, 6, 1, 0}, {0, 5, 8, 0}, {0, 1, 1, 0}, {2, 7, 32773, 0},
		{11, 6, 5, 0}, {10, 2, 8, 0}, {13, 5, 8, 2}, {16, 7, 8, 2}, {17, 5, 1, 0},
	}
	for _, p := range picks {
		k, v := kinds[p.kind], variants[p.variant]
		cases = append(cases, regionCase{
			name: fmt.Sprintf("%s/%s/comp%d/pred%d", k.name, v.name, p.compression, p.predictor),
			kind: k, variant: v, compression: p.compression, predictor: p.predictor,
		})
	}
	return cases
}

func (c regionCase) fixture(width, height int) fixture {
	f := c.kind.fixture(width, height, 77)
	f.compression, f.predictor = c.compression, c.predictor
	c.variant.apply(&f)
	return f
}

func TestRegionEqualsCropOfOverview(t *testing.T) {
	ctx := context.Background()
	for _, c := range regionCases() {
		t.Run(c.name, func(t *testing.T) {
			f := c.fixture(211, 157)
			data := f.bytes()
			levels := levelsOf(f.expected(), 7)
			for _, cacheBytes := range []int64{0, 3000} {
				rd, err := Decoder{CacheBytes: cacheBytes}.OpenTiled(ctx, bytes.NewReader(data), int64(len(data)), codec.DefaultLimits())
				if err != nil {
					t.Fatal(err)
				}
				rng := rand.New(rand.NewSource(int64(len(c.name)) + cacheBytes))
				for shift := 0; shift <= 7; shift++ {
					overview, err := rd.Overview(ctx, shift)
					if err != nil {
						t.Fatal(err)
					}
					requireIdentical(t, overview, levels[shift], fmt.Sprintf("Overview(%d)", shift))
					lw, lh := codec.LevelSize(f.width, f.height, shift)
					rects := []image.Rectangle{image.Rect(0, 0, lw, lh), image.Rect(0, 0, 1, 1), image.Rect(lw-1, lh-1, lw, lh), image.Rect(-5, -5, lw+5, lh+5), image.Rect(3, 3, 3, 9), image.Rect(lw, lh, lw+4, lh+4)}
					for i := 0; i < 8; i++ {
						rects = append(rects, randomRect(rng, lw, lh))
					}
					for _, r := range rects {
						region, err := rd.Region(ctx, shift, r)
						if err != nil {
							t.Fatalf("Region(%d, %v): %v", shift, r, err)
						}
						requireIdentical(t, region, cropOf(overview, r), fmt.Sprintf("cache %d Region(%d, %v)", cacheBytes, shift, r))
					}
				}
				rd.Close()
			}
		})
	}
}

func TestRegionBeyondTheNativeLevels(t *testing.T) {
	ctx := context.Background()
	f := kinds[12].fixture(130, 70, 5)
	f.compression = 8
	f.tileW, f.tileH = 32, 32
	rd := openTiled(t, f.bytes())
	levels := levelsOf(f.expected(), 12)
	for shift := 4; shift <= 12; shift++ {
		overview, err := rd.Overview(ctx, shift)
		if err != nil {
			t.Fatal(err)
		}
		requireIdentical(t, overview, levels[shift], fmt.Sprintf("Overview(%d)", shift))
		if overview.Rect.Dx() != 1 && shift >= 8 {
			t.Fatalf("level %d is %v", shift, overview.Rect)
		}
		region, err := rd.Region(ctx, shift, image.Rect(0, 0, 100, 100))
		if err != nil {
			t.Fatal(err)
		}
		requireIdentical(t, region, overview, fmt.Sprintf("Region(%d)", shift))
	}
	if _, err := rd.Overview(ctx, maxShift+1); err == nil {
		t.Fatal("level beyond the supported range accepted")
	}
	if _, err := rd.Overview(ctx, -1); err == nil {
		t.Fatal("negative level accepted")
	}
}

func TestConcurrentRegionsAgree(t *testing.T) {
	ctx := context.Background()
	for _, c := range regionCases()[:6] {
		f := c.fixture(300, 200)
		data := f.bytes()
		levels := levelsOf(f.expected(), 4)
		rd, err := Decoder{CacheBytes: 200 << 10}.OpenTiled(ctx, bytes.NewReader(data), int64(len(data)), codec.DefaultLimits())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := rd.Overview(ctx, 3); err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		errs := make(chan error, 16)
		for g := 0; g < 8; g++ {
			wg.Add(1)
			go func(seed int64) {
				defer wg.Done()
				rng := rand.New(rand.NewSource(seed))
				for i := 0; i < 25; i++ {
					shift := rng.Intn(4)
					lw, lh := codec.LevelSize(f.width, f.height, shift)
					r := randomRect(rng, lw, lh)
					got, err := rd.Region(ctx, shift, r)
					if err != nil {
						errs <- err
						return
					}
					want := cropOf(levels[shift], r)
					if got.Bounds() != want.Bounds() || !bytes.Equal(got.Pix[:got.Rect.Dy()*got.Stride], want.Pix[:want.Rect.Dy()*want.Stride]) {
						errs <- fmt.Errorf("%s: region %v at shift %d differs", c.name, r, shift)
						return
					}
				}
			}(int64(g) + 1)
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			t.Fatal(err)
		}
		rd.Close()
	}
}

func TestDecodeRegionAndScale(t *testing.T) {
	ctx := context.Background()
	f := kinds[13].fixture(97, 61, 11)
	f.compression = 5
	f.tileW, f.tileH = 32, 16
	data := f.bytes()
	levels := levelsOf(f.expected(), 3)
	decodeWith := func(o codec.Options) *codec.Image {
		o.Limits = codec.DefaultLimits()
		img, err := Decoder{}.Decode(ctx, bytes.NewReader(data), int64(len(data)), o)
		if err != nil {
			t.Fatal(err)
		}
		return img
	}
	for _, scale := range []int{0, 1, 2, 3, 4, 5, 7, 8, 16, 100} {
		shift := scaleShift(scale)
		img := decodeWith(codec.Options{Scale: scale})
		if img.Scale != 1<<uint(shift) {
			t.Fatalf("scale %d reported %d", scale, img.Scale)
		}
		requireIdentical(t, img.First(), levels[shift], fmt.Sprintf("scale %d", scale))
		if img.Info.Width != 97 || img.Info.Height != 61 {
			t.Fatalf("info reports %dx%d", img.Info.Width, img.Info.Height)
		}
	}
	region := image.Rect(10, 5, 70, 40)
	img := decodeWith(codec.Options{Region: &region})
	requireIdentical(t, img.First(), cropOf(levels[0], region), "region at scale 1")
	img = decodeWith(codec.Options{Region: &region, Scale: 4})
	want := cropOf(levels[2], region)
	requireIdentical(t, img.First(), want, "region at scale 4")
	if img.Scale != 4 {
		t.Fatalf("scale %d", img.Scale)
	}
	outside := image.Rect(500, 500, 600, 600)
	if _, err := (Decoder{}).Decode(ctx, bytes.NewReader(data), int64(len(data)), codec.Options{Limits: codec.DefaultLimits(), Region: &outside}); !errors.Is(err, codec.ErrCorrupt) {
		t.Fatalf("region outside the picture: %v", err)
	}
	inverted := image.Rect(70, 40, 10, 5)
	img = decodeWith(codec.Options{Region: &inverted})
	requireIdentical(t, img.First(), cropOf(levels[0], region), "inverted region")
}

func TestDecodeRegionOnFallbackLayouts(t *testing.T) {
	f := kinds[12].fixture(40, 30, 12)
	f.tileW, f.tileH = 24, 24
	data := f.bytes()
	if _, _, why, err := inspect(context.Background(), bytes.NewReader(data), int64(len(data))); err != nil || why == "" {
		t.Fatalf("tiles of 24 should be handled by the fallback: %q %v", why, err)
	}
	region := image.Rect(1, 1, 4, 3)
	img, err := Decoder{}.Decode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Options{Region: &region, Scale: 8})
	if err != nil {
		t.Fatal(err)
	}
	if img.Scale != 1 {
		t.Fatalf("fallback reported scale %d", img.Scale)
	}
	requireIdentical(t, img.First(), cropOf(f.expected(), image.Rect(8, 8, 32, 24)), "fallback region")
	outside := image.Rect(50, 50, 60, 60)
	if _, err := (Decoder{}).Decode(context.Background(), bytes.NewReader(data), int64(len(data)), codec.Options{Region: &outside}); !errors.Is(err, codec.ErrCorrupt) {
		t.Fatalf("fallback region outside the picture: %v", err)
	}
}

func withBandLimit(t *testing.T, limit int) {
	previous := maxBandBytes
	maxBandBytes = limit
	t.Cleanup(func() { maxBandBytes = previous })
}

func checkBandedFixture(t *testing.T, name string, f fixture, workersCache int64) {
	t.Helper()
	ctx := context.Background()
	data := f.bytes()
	rd, err := Decoder{CacheBytes: workersCache}.OpenTiled(ctx, bytes.NewReader(data), int64(len(data)), codec.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	defer rd.Close()
	lay := rd.(*reader).lay
	if lay.bandRows() >= lay.chunkH {
		t.Fatalf("%s: chunks of %d rows are not split (band rows %d)", name, lay.chunkH, lay.bandRows())
	}
	levels := levelsOf(f.expected(), 3)
	rng := rand.New(rand.NewSource(int64(f.compression)))
	for shift := 0; shift <= 3; shift++ {
		overview, err := rd.Overview(ctx, shift)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		requireIdentical(t, overview, levels[shift], fmt.Sprintf("%s Overview(%d)", name, shift))
		lw, lh := codec.LevelSize(f.width, f.height, shift)
		for i := 0; i < 8; i++ {
			r := randomRect(rng, lw, lh)
			region, err := rd.Region(ctx, shift, r)
			if err != nil {
				t.Fatal(err)
			}
			requireIdentical(t, region, cropOf(overview, r), fmt.Sprintf("%s Region(%d, %v)", name, shift, r))
		}
	}
}

func TestLargeChunksAreSplitIntoBands(t *testing.T) {
	for _, limit := range []int{3000, 500} {
		t.Run(fmt.Sprintf("limit%d", limit), func(t *testing.T) {
			withBandLimit(t, limit)
			checkAllBandedCases(t)
		})
	}
}

func cachesFor(limit int) []int64 {
	if limit < 1000 {
		return []int64{-1}
	}
	return []int64{-1, 1 << 20}
}

func checkAllBandedCases(t *testing.T) {
	cases := []struct {
		name  string
		kind  kind
		apply func(f *fixture)
	}{
		{"tiles-64", kinds[3], func(f *fixture) { f.tileW, f.tileH = 64, 64 }},
		{"tiles-48x80", kinds[3], func(f *fixture) { f.tileW, f.tileH = 48, 80 }},
		{"tiles-32-rgb", kinds[12], func(f *fixture) { f.tileW, f.tileH = 32, 64 }},
		{"strips-37", kinds[3], func(f *fixture) { f.rowsPerStrip = 37 }},
		{"single-strip", kinds[3], func(f *fixture) {}},
		{"strips-rgb-20", kinds[12], func(f *fixture) { f.rowsPerStrip = 20 }},
		{"palette4-tiles", kinds[10], func(f *fixture) { f.tileW, f.tileH = 32, 32 }},
		{"rgba16-strips", kinds[16], func(f *fixture) { f.rowsPerStrip = 13 }},
	}
	for _, c := range cases {
		for _, compression := range []int{1, 8, 5, 32773} {
			f := c.kind.fixture(97, 113, 51)
			f.compression = compression
			c.apply(&f)
			for _, cache := range cachesFor(maxBandBytes) {
				checkBandedFixture(t, fmt.Sprintf("%s/comp%d/cache%d", c.name, compression, cache), f, cache)
			}
		}
	}
}

func TestProductionBandLimitSplitsWideStrips(t *testing.T) {
	f := kinds[3].fixture(8200, 70, 52)
	f.compression = 8
	checkBandedFixture(t, "wide strip", f, 1<<20)
}

func TestFailedBandsAreNotCached(t *testing.T) {
	ctx := context.Background()
	f := kinds[12].fixture(64, 64, 52)
	f.compression = 8
	f.tileW, f.tileH = 16, 16
	data := f.bytes()
	o := f.byteOrder()
	ifd := int(o.Uint32(data[4:]))
	entries := int(o.Uint16(data[ifd:]))
	for i := 0; i < entries; i++ {
		at := ifd + 2 + 12*i
		if o.Uint16(data[at:]) == 0x144 {
			table := int(o.Uint32(data[at+8:]))
			start := int(o.Uint32(data[table:]))
			for k := 0; k < 12; k++ {
				data[start+2+k] = 0xff
			}
		}
	}
	rd, err := Decoder{}.OpenTiled(ctx, bytes.NewReader(data), int64(len(data)), codec.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	defer rd.Close()
	for attempt := 0; attempt < 3; attempt++ {
		if _, err := rd.Region(ctx, 0, image.Rect(0, 0, 16, 16)); !errors.Is(err, codec.ErrCorrupt) {
			t.Fatalf("attempt %d: %v", attempt, err)
		}
	}
	good, err := rd.Region(ctx, 0, image.Rect(16, 16, 32, 32))
	if err != nil {
		t.Fatal(err)
	}
	requireIdentical(t, good, cropOf(f.expected(), image.Rect(16, 16, 32, 32)), "neighbouring tile")
}

func TestSharedBandSurvivesACancelledLeader(t *testing.T) {
	f := kinds[12].fixture(300, 300, 53)
	f.compression = 8
	f.tileW, f.tileH = 256, 256
	data := f.bytes()
	source := &hookReader{r: bytes.NewReader(data)}
	rd, err := Decoder{}.OpenTiled(context.Background(), source, int64(len(data)), codec.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	defer rd.Close()
	leader, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	source.onRead = func(int64) {
		once.Do(func() {
			close(started)
			<-release
		})
	}
	leaderDone := make(chan error, 1)
	go func() {
		_, err := rd.Region(leader, 0, image.Rect(0, 0, 100, 100))
		leaderDone <- err
	}()
	<-started
	followerDone := make(chan error, 1)
	var follower *image.NRGBA
	go func() {
		var err error
		follower, err = rd.Region(context.Background(), 0, image.Rect(10, 10, 90, 90))
		followerDone <- err
	}()
	time.Sleep(50 * time.Millisecond)
	cancel()
	close(release)
	if err := <-leaderDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("leader: %v", err)
	}
	if err := <-followerDone; err != nil {
		t.Fatalf("follower: %v", err)
	}
	requireIdentical(t, follower, cropOf(f.expected(), image.Rect(10, 10, 90, 90)), "follower")
}

type flakyReader struct {
	r        io.ReaderAt
	failures atomic.Int64
}

func (f *flakyReader) ReadAt(p []byte, off int64) (int, error) {
	if f.failures.Load() > 0 && f.failures.Add(-1) >= 0 {
		return 0, errors.New("transient failure")
	}
	return f.r.ReadAt(p, off)
}

func TestTransientFailuresAreRetried(t *testing.T) {
	ctx := context.Background()
	f := kinds[12].fixture(64, 64, 54)
	f.compression = 8
	f.tileW, f.tileH = 16, 16
	data := f.bytes()
	source := &flakyReader{r: bytes.NewReader(data)}
	rd, err := Decoder{}.OpenTiled(ctx, source, int64(len(data)), codec.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	defer rd.Close()
	source.failures.Store(1)
	if _, err := rd.Region(ctx, 0, image.Rect(0, 0, 16, 16)); err == nil {
		t.Fatal("transient failure ignored")
	}
	region, err := rd.Region(ctx, 0, image.Rect(0, 0, 16, 16))
	if err != nil {
		t.Fatalf("the failure was cached: %v", err)
	}
	requireIdentical(t, region, cropOf(f.expected(), image.Rect(0, 0, 16, 16)), "retry")
}

func TestOverviewsAndRegionsRunTogether(t *testing.T) {
	ctx := context.Background()
	f := kinds[12].fixture(400, 300, 55)
	f.compression = 8
	f.predictor = 2
	f.rowsPerStrip = 9
	data := f.bytes()
	levels := levelsOf(f.expected(), 4)
	rd, err := Decoder{CacheBytes: 300 << 10}.OpenTiled(ctx, bytes.NewReader(data), int64(len(data)), codec.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	defer rd.Close()
	var wg sync.WaitGroup
	errs := make(chan error, 32)
	for g := 0; g < 6; g++ {
		wg.Add(1)
		go func(seed int64) {
			defer wg.Done()
			rng := rand.New(rand.NewSource(seed))
			for i := 0; i < 8; i++ {
				shift := rng.Intn(5)
				if seed%2 == 0 {
					got, err := rd.Overview(ctx, shift)
					if err != nil {
						errs <- err
						return
					}
					if !bytes.Equal(got.Pix, levels[shift].Pix) {
						errs <- fmt.Errorf("overview %d differs", shift)
						return
					}
					continue
				}
				lw, lh := codec.LevelSize(f.width, f.height, shift)
				r := randomRect(rng, lw, lh)
				got, err := rd.Region(ctx, shift, r)
				if err != nil {
					errs <- err
					return
				}
				want := cropOf(levels[shift], r)
				if got.Bounds() != want.Bounds() || !bytes.Equal(got.Pix[:got.Rect.Dy()*got.Stride], want.Pix[:want.Rect.Dy()*want.Stride]) {
					errs <- fmt.Errorf("region %v at %d differs", r, shift)
					return
				}
			}
		}(int64(g))
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}

func TestFailuresDoNotLeakGoroutines(t *testing.T) {
	f := kinds[12].fixture(512, 512, 56)
	f.compression = 8
	f.tileW, f.tileH = 32, 32
	data := f.bytes()
	o := f.byteOrder()
	ifd := int(o.Uint32(data[4:]))
	entries := int(o.Uint16(data[ifd:]))
	for i := 0; i < entries; i++ {
		at := ifd + 2 + 12*i
		if o.Uint16(data[at:]) == 0x144 {
			table := int(o.Uint32(data[at+8:]))
			start := int(o.Uint32(data[table+4*100:]))
			for k := 0; k < 8; k++ {
				data[start+2+k] = 0xff
			}
		}
	}
	base := runtime.NumGoroutine()
	for i := 0; i < 20; i++ {
		rd, err := Decoder{}.OpenTiled(context.Background(), bytes.NewReader(data), int64(len(data)), codec.DefaultLimits())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := rd.Overview(context.Background(), 0); !errors.Is(err, codec.ErrCorrupt) {
			t.Fatalf("%v", err)
		}
		rd.Close()
	}
	if n := settledGoroutines(base + 1); n > base+1 {
		t.Fatalf("goroutines grew from %d to %d", base, n)
	}
}

func TestMissingPhotometricAndDepthFollowXImage(t *testing.T) {
	f := kinds[0].fixture(40, 31, 57)
	f.omit = []uint16{0x106, 0x102}
	data := f.bytes()
	rd := openTiled(t, data)
	got, err := rd.Overview(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	reference, err := xtiff.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	requireIdentical(t, got, convert.NRGBA(reference), "defaults")
	f.photometric = 0
	if !bytes.Equal(got.Pix, f.expected().Pix) {
		t.Fatal("a missing photometric tag must mean WhiteIsZero")
	}
}
