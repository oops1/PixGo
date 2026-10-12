package app

import (
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	model "github.com/oops1/PixGo/internal/gallery"
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

func fillVisible(t *testing.T, a *App, count int) time.Duration {
	t.Helper()
	g := a.gal
	g.actions().Visible(0, count)
	start := time.Now()
	deadline := start.Add(60 * time.Second)
	for time.Now().Before(deadline) {
		a.Eng.Flush()
		ready := 0
		for i := 0; i < count && i < len(g.Items()); i++ {
			if _, ok := g.Thumb(g.Items()[i].Path, g.ThumbSize(), true); ok {
				ready++
			}
		}
		if ready == count {
			return time.Since(start)
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("thumbnails did not fill in time")
	return 0
}

func TestGalleryFillsAScreenOfLargeJPEGs(t *testing.T) {
	if testing.Short() || os.Getenv("PIXGO_PERF") == "" {
		t.Skip("set PIXGO_PERF to measure")
	}
	root := t.TempDir()
	seed := filepath.Join(root, "seed.jpg")
	writeLargeJPEG(t, seed, 6000, 4000)
	data, err := os.ReadFile(seed)
	if err != nil {
		t.Fatal(err)
	}
	const files = 500
	for i := 0; i < files; i++ {
		if err := os.WriteFile(filepath.Join(root, fmt.Sprintf("photo%03d.jpg", i)), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	os.Remove(seed)
	const screen = 24

	cold, _ := goldenApp(t, 1100, 700, nil)
	t.Cleanup(cold.Shutdown)
	listStart := time.Now()
	cold.OpenGalleryAt(model.Folder(root), "")
	for cold.gal.loading {
		cold.Eng.Flush()
		time.Sleep(time.Millisecond)
	}
	listed := time.Since(listStart)
	first := fillVisible(t, cold, screen)
	t.Logf("%d files listed in %v, first %d thumbnails after %v", files, listed.Round(time.Millisecond), screen, first.Round(time.Millisecond))
	cold.Shutdown()

	warm, _ := goldenAppSharingState(t, cold)
	t.Cleanup(warm.Shutdown)
	warm.OpenGalleryAt(model.Folder(root), "")
	for warm.gal.loading {
		warm.Eng.Flush()
		time.Sleep(time.Millisecond)
	}
	second := fillVisible(t, warm, screen)
	t.Logf("second visit: %d thumbnails after %v", screen, second.Round(time.Millisecond))
	if second > 500*time.Millisecond {
		t.Errorf("a revisited folder took %v to fill, the target is instant", second)
	}
}
