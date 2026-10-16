package jpegdec

import (
	"bytes"
	"errors"
	"image"
	"math/rand"
	"sync"
	"testing"
)

func randomRect(rng *rand.Rand, w, h int) image.Rectangle {
	x0, y0 := rng.Intn(w), rng.Intn(h)
	x1, y1 := x0+1+rng.Intn(w), y0+1+rng.Intn(h)
	if rng.Intn(4) == 0 {
		x1, y1 = x0+1+rng.Intn(24), y0+1+rng.Intn(24)
	}
	if rng.Intn(8) == 0 {
		x0, y0 = -rng.Intn(8), -rng.Intn(8)
	}
	return image.Rect(x0, y0, min(x1, w+rng.Intn(6)), min(y1, h+rng.Intn(6)))
}

func TestRegionEqualsCropOfFullDecode(t *testing.T) {
	files := corpus(t)
	rng := rand.New(rand.NewSource(7))
	for _, name := range sortedNames(files) {
		data := files[name]
		for _, every := range []int{1, 3, 16} {
			d := open(t, data, Options{CheckpointEvery: every, Workers: 1})
			for shift := 0; shift <= MaxShift; shift++ {
				full, err := d.Decode(t.Context(), shift)
				if err != nil {
					d.Close()
					t.Fatalf("%s shift %d: %v", name, shift, err)
				}
				if !d.CanRegion() && shift < d.MinShift() {
					continue
				}
				b := full.Bounds()
				for i := 0; i < 25; i++ {
					r := randomRect(rng, b.Dx(), b.Dy())
					got, err := d.Region(t.Context(), shift, r)
					if err != nil {
						t.Fatalf("%s shift %d rect %v: %v", name, shift, r, err)
					}
					want := crop(full, r)
					if !samePixels(got, want) {
						t.Fatalf("%s every %d shift %d rect %v: %s", name, every, shift, r, firstDifference(got, want))
					}
				}
			}
			d.Close()
		}
	}
}

func TestRegionOfSyntheticFilesEqualsCrop(t *testing.T) {
	files := syntheticFiles()
	rng := rand.New(rand.NewSource(21))
	for _, name := range sortedNames(files) {
		data := files[name]
		for _, every := range []int{1, 2, 5, 16} {
			d := open(t, data, Options{CheckpointEvery: every, Workers: 2})
			for shift := 0; shift <= MaxShift; shift++ {
				full, err := d.Decode(t.Context(), shift)
				if err != nil {
					t.Fatalf("%s: %v", name, err)
				}
				if shift < d.MinShift() {
					continue
				}
				b := full.Bounds()
				for i := 0; i < 8; i++ {
					r := randomRect(rng, b.Dx(), b.Dy())
					got, err := d.Region(t.Context(), shift, r)
					if err != nil {
						t.Fatalf("%s shift %d rect %v: %v", name, shift, r, err)
					}
					if !samePixels(got, crop(full, r)) {
						t.Fatalf("%s every %d shift %d rect %v: %s", name, every, shift, r, firstDifference(got, crop(full, r)))
					}
				}
			}
			d.Close()
		}
	}
}

func TestRegionOutsideTheLevelIsEmpty(t *testing.T) {
	d := open(t, corpus(t)["video-001.q50.420.jpeg"], Options{})
	defer d.Close()
	for _, r := range []image.Rectangle{rect(500, 500, 600, 600), rect(-50, -50, 0, 0), rect(10, 10, 10, 20), {}} {
		img, err := d.Region(t.Context(), 0, r)
		if err != nil || !img.Bounds().Empty() {
			t.Fatalf("%v: %v %v", r, img.Bounds(), err)
		}
	}
	img, err := d.Region(t.Context(), 0, rect(-20, -20, 4000, 4000))
	if err != nil || img.Bounds() != rect(0, 0, 150, 103) {
		t.Fatalf("clipped region %v %v", img.Bounds(), err)
	}
}

func TestRegionBuildsTheIndexOnDemand(t *testing.T) {
	d := open(t, corpus(t)["video-001.q50.420.jpeg"], Options{})
	defer d.Close()
	if d.IndexBytes() != 0 {
		t.Fatal("index exists before any decode")
	}
	if _, err := d.Region(t.Context(), 1, rect(3, 3, 40, 30)); err != nil {
		t.Fatal(err)
	}
	if d.IndexBytes() == 0 {
		t.Fatal("no index after a region request")
	}
}

func TestIndexBuiltByDecodeEqualsExplicitIndex(t *testing.T) {
	files := syntheticFiles()
	for _, name := range []string{"420-restart-150x103", "444-restart1-65x31", "411-150x103", "gray-150x103", "ycck-2x2"} {
		data := files[name]
		a := open(t, data, Options{CheckpointEvery: 3})
		if _, err := a.Decode(t.Context(), 1); err != nil {
			t.Fatal(err)
		}
		b := open(t, data, Options{CheckpointEvery: 3})
		if err := b.BuildIndex(t.Context()); err != nil {
			t.Fatal(err)
		}
		x, y := a.index.Load(), b.index.Load()
		if x == nil || y == nil || x.every != y.every || !bytes.Equal(x.bit, y.bit) || !equalUint32(x.off, y.off) || !equalInt16(x.dc, y.dc) {
			t.Fatalf("%s: indexes differ", name)
		}
		a.Close()
		b.Close()
	}
}

func equalUint32(a, b []uint32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func equalInt16(a, b []int16) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestRegionSurvivesMissingCheckpoints(t *testing.T) {
	data := syntheticFiles()["420-150x103"]
	d := open(t, data, Options{CheckpointEvery: 2})
	defer d.Close()
	full, err := d.Decode(t.Context(), 0)
	if err != nil {
		t.Fatal(err)
	}
	ix := d.index.Load()
	for slot := 1; slot < len(ix.off); slot++ {
		if slot%3 != 0 {
			ix.off[slot] = invalidSlot
		}
	}
	rng := rand.New(rand.NewSource(4))
	for i := 0; i < 40; i++ {
		r := randomRect(rng, 150, 103)
		got, err := d.Region(t.Context(), 0, r)
		if err != nil || !samePixels(got, crop(full, r)) {
			t.Fatalf("rect %v: %v", r, err)
		}
	}
}

func TestParallelRegionEqualsSerialRegion(t *testing.T) {
	data := mediumJPEG(t)
	serial := open(t, data, Options{Workers: 1})
	defer serial.Close()
	parallel := open(t, data, Options{Workers: 6})
	defer parallel.Close()
	for _, r := range []image.Rectangle{rect(100, 90, 2100, 1500), rect(0, 0, 2400, 1600), rect(1000, 700, 1300, 900)} {
		for shift := 0; shift <= 2; shift++ {
			rr := image.Rect(r.Min.X>>uint(shift), r.Min.Y>>uint(shift), r.Max.X>>uint(shift), r.Max.Y>>uint(shift))
			a, err := serial.Region(t.Context(), shift, rr)
			if err != nil {
				t.Fatal(err)
			}
			b, err := parallel.Region(t.Context(), shift, rr)
			if err != nil {
				t.Fatal(err)
			}
			if !samePixels(a, b) {
				t.Fatalf("shift %d rect %v: %s", shift, rr, firstDifference(a, b))
			}
		}
	}
}

func TestConcurrentRegionRequests(t *testing.T) {
	data := mediumJPEG(t)
	d := open(t, data, Options{Workers: 1})
	defer d.Close()
	full, err := d.Decode(t.Context(), 1)
	if err != nil {
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
				r := randomRect(rng, full.Bounds().Dx(), full.Bounds().Dy())
				if r.Dx() > 400 {
					r.Max.X = r.Min.X + 400
				}
				got, err := d.Region(t.Context(), 1, r)
				if err != nil {
					errs <- err
					return
				}
				if !samePixels(got, crop(full, r)) {
					errs <- errors.New("concurrent region differs from crop " + r.String())
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

func TestWorkerCountsProduceIdenticalImages(t *testing.T) {
	data := mediumJPEG(t)
	reference := decodeAt(t, data, 0, Options{Workers: 1})
	for _, workers := range []int{2, 3, 5, 8, 16} {
		for _, shift := range []int{0, 2} {
			got := decodeAt(t, data, shift, Options{Workers: workers})
			want := decodeAt(t, data, shift, Options{Workers: 1})
			if !samePixels(got, want) {
				t.Fatalf("workers %d shift %d: %s", workers, shift, firstDifference(got, want))
			}
		}
	}
	_ = reference
}

func TestIndexStaysSmallForGiantImages(t *testing.T) {
	cases := []struct {
		name         string
		width        int
		height       int
		comps        []encComp
		maxIndexSize int
	}{
		{"444 200 Mpx", 16000, 12500, ycbcrComps(1, 1, 1, 1, 1, 1), 2 << 20},
		{"420 200 Mpx", 16000, 12500, ycbcrComps(2, 2, 1, 1, 1, 1), 2 << 20},
		{"gray 200 Mpx", 16000, 12500, []encComp{{1, 1, 1, 0}}, 2 << 20},
		{"cmyk 200 Mpx", 16000, 12500, []encComp{{1, 1, 1, 0}, {2, 1, 1, 1}, {3, 1, 1, 1}, {4, 1, 1, 0}}, 2 << 20},
	}
	for _, tc := range cases {
		f := &frame{width: tc.width, height: tc.height, nComp: len(tc.comps)}
		for i, c := range tc.comps {
			f.comp[i] = component{h: c.h, v: c.v}
			f.maxH, f.maxV = max(f.maxH, c.h), max(f.maxV, c.v)
		}
		g := f.geometry(0, 0)
		rec := newIndexRecorder(&Decoder{f: f}, g)
		if size := rec.Bytes(); size >= tc.maxIndexSize {
			t.Errorf("%s: index of %d bytes", tc.name, size)
		}
	}
}

func TestNoiseFilesCarryManyStuffedBytes(t *testing.T) {
	files := syntheticFiles()
	for _, name := range []string{"ones-420", "ones-444-restart", "ones-gray"} {
		if n := bytes.Count(files[name], []byte{0xff, 0x00}); n < 100 {
			t.Fatalf("%s has only %d stuffed bytes", name, n)
		}
	}
}
