package testutil

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"runtime"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/fonts"
)

func RefDir() string {
	_, file, _, _ := runtime.Caller(0)
	dir := filepath.Join(filepath.Dir(file), "..", "..", "docs", "spec")
	if _, err := os.Stat(dir); err == nil {
		return dir
	}
	if wd, err := os.Getwd(); err == nil {
		for d := wd; ; d = filepath.Dir(d) {
			if cand := filepath.Join(d, "docs", "spec"); dirExists(cand) {
				return cand
			}
			if filepath.Dir(d) == d {
				break
			}
		}
	}
	return dir
}

func dirExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

func LoadRef(name string) (*image.RGBA, error) { return LoadPNG(filepath.Join(RefDir(), name)) }

func LoadPNG(path string) (*image.RGBA, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	src, err := png.Decode(f)
	if err != nil {
		return nil, err
	}
	b := src.Bounds()
	out := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(out, out.Bounds(), src, b.Min, draw.Src)
	return out, nil
}

func LoadRefCrop(name string, r image.Rectangle) (*image.RGBA, error) {
	full, err := LoadRef(name)
	if err != nil {
		return nil, err
	}
	r = r.Intersect(full.Bounds())
	if r.Empty() {
		return nil, fmt.Errorf("testutil: crop %v is outside %s", r, name)
	}
	out := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	draw.Draw(out, out.Bounds(), full, r.Min, draw.Src)
	return out, nil
}

func MeanError(a, b *image.RGBA, r image.Rectangle, mask ...image.Rectangle) float64 {
	r = r.Intersect(a.Bounds()).Intersect(b.Bounds())
	var sum, n float64
	for y := r.Min.Y; y < r.Max.Y; y++ {
	px:
		for x := r.Min.X; x < r.Max.X; x++ {
			for _, m := range mask {
				if image.Pt(x, y).In(m) {
					continue px
				}
			}
			ca, cb := a.RGBAAt(x, y), b.RGBAAt(x, y)
			sum += absDiff(ca.R, cb.R) + absDiff(ca.G, cb.G) + absDiff(ca.B, cb.B)
			n += 3
		}
	}
	if n == 0 {
		return 0
	}
	return sum / n
}

func absDiff(a, b uint8) float64 {
	if a > b {
		return float64(a - b)
	}
	return float64(b - a)
}

func SaveComparison(path string, got, want *image.RGBA) error {
	w, h := got.Bounds().Dx(), got.Bounds().Dy()
	out := image.NewRGBA(image.Rect(0, 0, w, h*3))
	draw.Draw(out, image.Rect(0, 0, w, h), got, got.Bounds().Min, draw.Src)
	draw.Draw(out, image.Rect(0, h, w, 2*h), want, want.Bounds().Min, draw.Src)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			a, b := got.RGBAAt(got.Bounds().Min.X+x, got.Bounds().Min.Y+y), want.RGBAAt(want.Bounds().Min.X+x, want.Bounds().Min.Y+y)
			d := (absDiff(a.R, b.R) + absDiff(a.G, b.G) + absDiff(a.B, b.B)) * 2
			if d > 255 {
				d = 255
			}
			v := uint8(255 - d)
			out.SetRGBA(x, 2*h+y, color.RGBA{255, v, v, 255})
		}
	}
	return SavePNG(path, out)
}

func SavePNG(path string, img image.Image) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func OutDir() string { return os.Getenv("PIXGO_SNAPSHOTS") }

func NewEngine(w, h int) *engine.Engine {
	widget.UseMemoryClipboard()
	eng := engine.New(w, h, 30)
	if err := fonts.Register(eng); err != nil {
		panic(fmt.Sprintf("testutil: register fonts: %v", err))
	}
	return eng
}
