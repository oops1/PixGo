package app

import (
	"image"
	"image/color"
	"image/jpeg"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeLargeJPEG(tb testing.TB, path string, w, h int) {
	tb.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	r := rand.New(rand.NewSource(1))
	for y := 0; y < h; y++ {
		row := img.Pix[y*img.Stride : y*img.Stride+w*4]
		for x := 0; x < w; x++ {
			n := uint8(r.Intn(24))
			row[x*4] = uint8(x*255/w) + n
			row[x*4+1] = uint8(y*255/h) + n
			row[x*4+2] = uint8((x+y)*255/(w+h)) + n
			row[x*4+3] = 255
		}
	}
	f, err := os.Create(path)
	if err != nil {
		tb.Fatal(err)
	}
	defer f.Close()
	if err := jpeg.Encode(f, img, &jpeg.Options{Quality: 90}); err != nil {
		tb.Fatal(err)
	}
	_ = color.RGBA{}
}

func TestFirstShowOfLargeJPEG(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	path := filepath.Join(t.TempDir(), "large.jpg")
	writeLargeJPEG(t, path, 6000, 4000)
	for i := 0; i < 3; i++ {
		a, _ := goldenApp(t, 1100, 700, nil)
		start := time.Now()
		if err := a.Ctl.Open(path); err != nil {
			t.Fatal(err)
		}
		waitReady(t, a)
		ready := time.Since(start)
		a.Root.Refresh()
		a.Eng.Flush()
		a.Eng.RenderOnce()
		shown := time.Since(start)
		t.Logf("run %d: ready %v, first frame %v", i+1, ready.Round(time.Millisecond), shown.Round(time.Millisecond))
		if os.Getenv("PIXGO_PERF") != "" && shown > 500*time.Millisecond {
			t.Errorf("first show took %v, the target is 500 ms", shown)
		}
	}
}
