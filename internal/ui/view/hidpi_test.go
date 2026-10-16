package view

import (
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/imaging"
	"github.com/oops1/PixGo/internal/render"
	"github.com/oops1/PixGo/internal/testutil"
	"github.com/oops1/PixGo/internal/viewer"
)

func checkerImage(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			v := uint8(0)
			if (x+y)%2 == 0 {
				v = 255
			}
			img.SetNRGBA(x, y, color.NRGBA{v, v, v, 255})
		}
	}
	return img
}

func scaledRig(t *testing.T, k float64) (*engine.Engine, *Canvas) {
	t.Helper()
	eng := testutil.NewEngineScaled(areaW, areaH, k)
	c := New(eng)
	c.Background = render.Background{Canvas: color.RGBA{10, 20, 30, 255}, Kind: render.BgSolid, Fill: color.RGBA{10, 20, 30, 255}}
	eng.SetRootFullCanvas(c)
	return eng, c
}

func TestCanvasBufferMatchesThePhysicalSizeAtEveryScale(t *testing.T) {
	for _, k := range []float64{1, 1.25, 1.5, 1.75, 2} {
		eng, c := scaledRig(t, k)
		c.SetImage(render.Source{Levels: imaging.BuildPyramid(checkerImage(64, 64), 64), Width: 64, Height: 64}, codec.OrientNormal, viewer.ZoomFit)
		eng.Flush()
		eng.RenderOnce()
		want := image.Pt(int(math.Round(areaW*k)), int(math.Round(areaH*k)))
		if got := c.buf.Bounds().Size(); got != want {
			t.Errorf("k=%v: buffer %v, want the physical size %v", k, got, want)
		}
		if c.areaW != want.X || c.areaH != want.Y {
			t.Errorf("k=%v: viewport %dx%d, want %v", k, c.areaW, c.areaH, want)
		}
	}
}

func TestActualSizeShowsOneImagePixelPerPhysicalPixel(t *testing.T) {
	for _, k := range []float64{1.25, 1.5, 1.75, 2} {
		eng, c := scaledRig(t, k)
		c.SetImage(render.Source{Levels: imaging.BuildPyramid(checkerImage(256, 256), 64), Width: 256, Height: 256}, codec.OrientNormal, viewer.ZoomActual)
		eng.Flush()
		frame := eng.RenderOnce()
		r := c.view.ImageRect(c.Frame())
		origin := image.Pt(physical(c.Bounds().Min.X, k), physical(c.Bounds().Min.Y, k))
		for y := 20; y < 36; y++ {
			for x := 20; x < 36; x++ {
				want := uint8(0)
				if (x+y)%2 == 0 {
					want = 255
				}
				p := image.Pt(origin.X+r.Min.X+x, origin.Y+r.Min.Y+y)
				if got := frame.RGBAAt(p.X, p.Y); got.R != want || got.G != want {
					t.Fatalf("k=%v: image pixel (%d,%d) shows %v, want %d", k, x, y, got, want)
				}
			}
		}
	}
}

func physical(v int, k float64) int { return int(math.Round(float64(v) * k)) }

func TestImageRectLogicalCoversThePhysicalPicture(t *testing.T) {
	for _, k := range []float64{1.25, 1.5, 2} {
		_, c := scaledRig(t, k)
		c.SetImage(render.Source{Levels: imaging.BuildPyramid(checkerImage(200, 100), 64), Width: 200, Height: 100}, codec.OrientNormal, viewer.ZoomActual)
		c.eng.(*engine.Engine).Flush()
		c.eng.(*engine.Engine).RenderOnce()
		logical := c.ImageRectLogical()
		phys := c.view.ImageRect(c.Frame())
		if logical.Dx()*int(k*100) < phys.Dx()*100-int(k*100)*2 {
			t.Errorf("k=%v: logical rect %v is smaller than the physical picture %v", k, logical, phys)
		}
	}
}
