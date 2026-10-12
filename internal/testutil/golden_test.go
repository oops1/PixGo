package testutil

import (
	"image"
	"image/color"
	"path/filepath"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

func solid(w, h int, c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, c.A
	}
	return img
}

func TestLoadRef(t *testing.T) {
	img, err := LoadRef("01_main_viewer.png")
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 770 || img.Bounds().Dy() != 504 {
		t.Fatalf("size %v", img.Bounds())
	}
	crop, err := LoadRefCrop("01_main_viewer.png", image.Rect(10, 10, 60, 40))
	if err != nil {
		t.Fatal(err)
	}
	if crop.Bounds() != image.Rect(0, 0, 50, 30) || crop.RGBAAt(0, 0) != img.RGBAAt(10, 10) {
		t.Fatalf("crop %v", crop.Bounds())
	}
	if _, err := LoadRef("missing.png"); err == nil {
		t.Fatal("missing reference accepted")
	}
}

func TestMeanError(t *testing.T) {
	a := solid(10, 10, color.RGBA{10, 10, 10, 255})
	b := solid(10, 10, color.RGBA{20, 20, 20, 255})
	if got := MeanError(a, b, a.Bounds()); got != 10 {
		t.Fatalf("got %v", got)
	}
	if got := MeanError(a, b, image.Rect(0, 0, 10, 10), image.Rect(0, 0, 10, 10)); got != 0 {
		t.Fatalf("masked: %v", got)
	}
	if got := MeanError(a, b, image.Rect(20, 20, 30, 30)); got != 0 {
		t.Fatalf("outside: %v", got)
	}
}

func TestSaveComparison(t *testing.T) {
	a := solid(8, 6, color.RGBA{0, 0, 0, 255})
	b := solid(8, 6, color.RGBA{255, 255, 255, 255})
	p := filepath.Join(t.TempDir(), "x", "cmp.png")
	if err := SaveComparison(p, a, b); err != nil {
		t.Fatal(err)
	}
	back, err := LoadPNG(p)
	if err != nil {
		t.Fatal(err)
	}
	if back.Bounds().Dx() != 8 || back.Bounds().Dy() != 18 {
		t.Fatalf("size %v", back.Bounds())
	}
}

func TestNewEngineRenders(t *testing.T) {
	eng := NewEngine(120, 80)
	p := widget.NewPanel(color.RGBA{30, 40, 50, 255})
	eng.SetRootFullCanvas(p)
	img := eng.RenderOnce()
	if img.Bounds().Dx() != 120 || img.Bounds().Dy() != 80 {
		t.Fatalf("size %v", img.Bounds())
	}
	if c := img.RGBAAt(60, 40); c.R != 30 || c.G != 40 || c.B != 50 {
		t.Fatalf("pixel %v", c)
	}
}
