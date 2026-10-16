package icons

import (
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/testutil"
)

type paintProbe struct {
	widget.Base
	draw func(c widget.DrawContext)
}

func (p *paintProbe) Draw(c widget.DrawContext) {
	b := p.Bounds()
	c.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), color.RGBA{0, 0, 0, 255})
	p.draw(c)
}

func paint(t *testing.T, k float64, draw func(c widget.DrawContext)) *image.RGBA {
	t.Helper()
	eng := testutil.NewEngineScaled(64, 64, k)
	eng.SetRootFullCanvas(&paintProbe{draw: draw})
	eng.Flush()
	return eng.RenderOnce()
}

func inkBounds(img *image.RGBA) image.Rectangle {
	out := image.Rectangle{Min: image.Pt(1<<30, 1<<30), Max: image.Pt(-1, -1)}
	for y := 0; y < img.Bounds().Dy(); y++ {
		for x := 0; x < img.Bounds().Dx(); x++ {
			if img.RGBAAt(x, y).R > 40 {
				out.Min.X, out.Min.Y = min(out.Min.X, x), min(out.Min.Y, y)
				out.Max.X, out.Max.Y = max(out.Max.X, x+1), max(out.Max.Y, y+1)
			}
		}
	}
	return out
}

func distinctLevels(img *image.RGBA) int {
	seen := map[uint8]bool{}
	for y := 0; y < img.Bounds().Dy(); y++ {
		for x := 0; x < img.Bounds().Dx(); x++ {
			seen[img.RGBAAt(x, y).R] = true
		}
	}
	return len(seen)
}

func TestIconsAreRasterisedAtThePhysicalSize(t *testing.T) {
	white := color.RGBA{255, 255, 255, 255}
	for _, name := range []string{"delete", "heart", "gallery", "rotate-left", "info", "chevron-left", "play", "settings", "fullscreen"} {
		base := paint(t, 1, func(c widget.DrawContext) { Draw(c, name, 20, 20, white) })
		want := inkBounds(base)
		for _, k := range []float64{1.25, 1.5, 1.75, 2} {
			img := paint(t, k, func(c widget.DrawContext) { Draw(c, name, 20, 20, white) })
			got := inkBounds(img)
			tol := 3
			if math.Abs(float64(got.Dx())-float64(want.Dx())*k) > float64(tol) || math.Abs(float64(got.Dy())-float64(want.Dy())*k) > float64(tol) {
				t.Errorf("%s at %v: ink %v is not %v scaled", name, k, got, want)
			}
			if levels := distinctLevels(img); levels < 8 {
				t.Errorf("%s at %v: only %d grey levels, the edges are not anti-aliased at the physical size", name, k, levels)
			}
		}
	}
}

func TestIconsAreNotPixelDoubledAtTwoTimes(t *testing.T) {
	white := color.RGBA{255, 255, 255, 255}
	img := paint(t, 2, func(c widget.DrawContext) { Draw(c, "heart", 20, 20, white) })
	blocks, total := 0, 0
	for y := 0; y+1 < img.Bounds().Dy(); y += 2 {
		for x := 0; x+1 < img.Bounds().Dx(); x += 2 {
			a := img.RGBAAt(x, y)
			if a.R == 0 {
				continue
			}
			total++
			if img.RGBAAt(x+1, y) == a && img.RGBAAt(x, y+1) == a && img.RGBAAt(x+1, y+1) == a {
				blocks++
			}
		}
	}
	if total == 0 || blocks*2 > total {
		t.Errorf("%d of %d inked 2x2 cells are flat: the icon was drawn at 1x and doubled", blocks, total)
	}
}

func TestTwoToneAndRampLayersFollowTheScale(t *testing.T) {
	for name, d := range defs {
		for _, l := range d.layers {
			if l.tone == fixed {
				img := paint(t, 1.5, func(c widget.DrawContext) { Draw(c, name, 10, 10, color.RGBA{255, 255, 255, 255}) })
				if inkBounds(img).Empty() {
					t.Errorf("%s: coloured icon vanished at 1.5x", name)
				}
				return
			}
		}
	}
	t.Skip("no coloured icons")
}

func TestHiMasksAreCached(t *testing.T) {
	white := color.RGBA{255, 255, 255, 255}
	paint(t, 1.5, func(c widget.DrawContext) { Draw(c, "delete", 10, 10, white) })
	hiCache.Lock()
	before := len(hiCache.m)
	hiCache.Unlock()
	paint(t, 1.5, func(c widget.DrawContext) { Draw(c, "delete", 10, 10, white) })
	hiCache.Lock()
	after := len(hiCache.m)
	hiCache.Unlock()
	if before == 0 || before != after {
		t.Errorf("cache entries %d then %d", before, after)
	}
}

func TestUnknownIconsDrawNothingAtAnyScale(t *testing.T) {
	img := paint(t, 2, func(c widget.DrawContext) { Draw(c, "no-such-icon", 10, 10, color.RGBA{255, 255, 255, 255}) })
	if !inkBounds(img).Empty() {
		t.Error("an unknown icon drew something")
	}
}
