package jpegdec

import (
	"bytes"
	"context"
	"os"
	"runtime"
	"testing"
	"time"
)

func TestTallRestartFilesMatchTheStandardLibrary(t *testing.T) {
	for _, spec := range []encSpec{
		{width: 301, height: 16, comps: ycbcrComps(2, 2, 1, 1, 1, 1), seed: 3},
		{width: 100, height: 8, comps: ycbcrComps(1, 1, 1, 1, 1, 1), seed: 4, mixedCodes: true},
		{width: 77, height: 8, comps: []encComp{{1, 1, 1, 0}}, seed: 5},
	} {
		data := tallJPEG(spec, 9)
		want, err := referenceNRGBA(t, data)
		if err != nil {
			t.Fatal(err)
		}
		for _, workers := range []int{1, 3} {
			got := decodeAt(t, data, 0, Options{Workers: workers})
			if !samePixels(got, want) {
				t.Fatalf("%+v workers %d: %s", spec.comps, workers, firstDifference(got, want))
			}
		}
	}
}

func TestGiantFileOverview(t *testing.T) {
	if os.Getenv("PIXGO_GIANT") == "" {
		t.Skip("set PIXGO_GIANT=1")
	}
	spec := encSpec{width: 16000, height: 16, comps: ycbcrComps(2, 2, 1, 1, 1, 1), seed: 9}
	data := tallJPEG(spec, 781)
	t.Logf("file %d bytes, %dx%d", len(data), 16000, 781*16)
	for _, shift := range []int{2, 3} {
		runtime.GC()
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		d, err := Open(context.Background(), bytes.NewReader(data), int64(len(data)), Options{})
		if err != nil {
			t.Fatal(err)
		}
		start := time.Now()
		img, err := d.Decode(context.Background(), shift)
		if err != nil {
			t.Fatal(err)
		}
		elapsed := time.Since(start)
		runtime.ReadMemStats(&after)
		t.Logf("shift %d: %v, %v, index %d bytes, heap in use %d MB, total allocated %d MB", shift, img.Bounds(), elapsed, d.IndexBytes(), after.HeapInuse>>20, (after.TotalAlloc-before.TotalAlloc)>>20)
		start = time.Now()
		for i := 0; i < 20; i++ {
			x, y := (i*977)%15000, (i*613)%12000
			r := rect(x, y, x+256, y+256)
			if _, err := d.Region(context.Background(), 0, r); err != nil {
				t.Fatal(err)
			}
		}
		t.Logf("20 tiles of 256x256 at full resolution: %v", time.Since(start))
		d.Close()
	}
}
