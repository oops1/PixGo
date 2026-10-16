package gfx

import (
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/testutil"
	"github.com/oops1/PixGo/internal/ui/theme"
)

var (
	black = color.RGBA{0, 0, 0, 255}
	white = color.RGBA{255, 255, 255, 255}
)

type probe struct {
	widget.Base
	draw func(c widget.DrawContext)
}

func (p *probe) Draw(c widget.DrawContext) {
	b := p.Bounds()
	c.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), black)
	p.draw(c)
}

func render(t *testing.T, w, h int, k float64, draw func(c widget.DrawContext)) *image.RGBA {
	t.Helper()
	eng := testutil.NewEngineScaled(w, h, k)
	eng.SetRootFullCanvas(&probe{draw: draw})
	eng.Flush()
	return eng.RenderOnce()
}

func coverageSum(img *image.RGBA, r image.Rectangle) float64 {
	var sum float64
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			sum += float64(img.RGBAAt(x, y).R) / 255
		}
	}
	return sum
}

var scales = []float64{1.25, 1.5, 1.75, 2}

func TestFilledRoundRectKeepsCrispEdgesOnThePhysicalGrid(t *testing.T) {
	logical := image.Rect(10, 8, 50, 28)
	for _, k := range scales {
		img := render(t, 80, 40, k, func(c widget.DrawContext) { FillRound(c, logical, 4, white) })
		p := PhysRect(logical, k)
		midY := (p.Min.Y + p.Max.Y) / 2
		for _, probe := range []struct {
			x    int
			want color.RGBA
		}{{p.Min.X - 1, black}, {p.Min.X, white}, {p.Max.X - 1, white}, {p.Max.X, black}} {
			if got := img.RGBAAt(probe.x, midY); got != probe.want {
				t.Errorf("k=%v: pixel (%d,%d) = %v, want %v", k, probe.x, midY, got, probe.want)
			}
		}
		midX := (p.Min.X + p.Max.X) / 2
		for _, probe := range []struct {
			y    int
			want color.RGBA
		}{{p.Min.Y - 1, black}, {p.Min.Y, white}, {p.Max.Y - 1, white}, {p.Max.Y, black}} {
			if got := img.RGBAAt(midX, probe.y); got != probe.want {
				t.Errorf("k=%v: pixel (%d,%d) = %v, want %v", k, midX, probe.y, got, probe.want)
			}
		}
	}
}

func TestRoundedCornersAreSmoothAtPhysicalResolution(t *testing.T) {
	logical := image.Rect(10, 8, 50, 28)
	for _, k := range scales {
		img := render(t, 80, 40, k, func(c widget.DrawContext) { FillRound(c, logical, 6, white) })
		p := PhysRect(logical, k)
		partial := map[uint8]bool{}
		for y := p.Min.Y; y < p.Min.Y+int(6*k)+1; y++ {
			for x := p.Min.X; x < p.Min.X+int(6*k)+1; x++ {
				if v := img.RGBAAt(x, y).R; v > 0 && v < 255 {
					partial[v] = true
				}
			}
		}
		if len(partial) < 4 {
			t.Errorf("k=%v: only %d distinct edge coverages in the corner", k, len(partial))
		}
		if img.RGBAAt(p.Min.X, p.Min.Y) != black {
			t.Errorf("k=%v: the very corner pixel is filled", k)
		}
	}
}

func TestRoundedRectAreaMatchesTheGeometry(t *testing.T) {
	logical := image.Rect(10, 8, 50, 28)
	for _, k := range scales {
		img := render(t, 80, 40, k, func(c widget.DrawContext) { FillRound(c, logical, 6, white) })
		p := PhysRect(logical, k)
		radius := 6 * k
		want := float64(p.Dx()*p.Dy()) - (4-math.Pi)*radius*radius
		if got := coverageSum(img, p.Inset(-2)); math.Abs(got-want) > want*0.01 {
			t.Errorf("k=%v: area %.1f, want %.1f", k, got, want)
		}
	}
}

func TestHairlineBorderIsOnePhysicalPixelWhereTheScaleAllows(t *testing.T) {
	logical := image.Rect(10, 8, 50, 28)
	for _, k := range []float64{1.25, 1.75} {
		img := render(t, 80, 40, k, func(c widget.DrawContext) { StrokeRound(c, logical, 4, 1, white) })
		p := PhysRect(logical, k)
		midY := (p.Min.Y + p.Max.Y) / 2
		want := int(math.Round(k))
		var lit int
		for x := p.Min.X - 2; x < p.Min.X+want+3; x++ {
			if img.RGBAAt(x, midY) == white {
				lit++
			} else if v := img.RGBAAt(x, midY).R; v != 0 {
				t.Errorf("k=%v: blurred border pixel %d at x=%d", k, v, x)
			}
		}
		if lit != want {
			t.Errorf("k=%v: border is %d physical pixels wide, want %d", k, lit, want)
		}
	}
}

func TestStrokeSitsInsideTheSnappedRectangle(t *testing.T) {
	logical := image.Rect(10, 8, 50, 28)
	img := render(t, 80, 40, 2, func(c widget.DrawContext) { StrokeRound(c, logical, 4, 1, white) })
	p := PhysRect(logical, 2)
	midY := (p.Min.Y + p.Max.Y) / 2
	if img.RGBAAt(p.Min.X-1, midY) != black || img.RGBAAt(p.Min.X, midY) != white || img.RGBAAt(p.Min.X+1, midY) != white || img.RGBAAt(p.Min.X+2, midY) != black {
		t.Errorf("a 1 px stroke at 2x is not two physical pixels inside the left edge: %v %v %v %v",
			img.RGBAAt(p.Min.X-1, midY), img.RGBAAt(p.Min.X, midY), img.RGBAAt(p.Min.X+1, midY), img.RGBAAt(p.Min.X+2, midY))
	}
}

func TestCircleAreaScalesWithTheDisplay(t *testing.T) {
	for _, k := range scales {
		img := render(t, 60, 60, k, func(c widget.DrawContext) { FillCircle(c, 30, 30, 10, white) })
		want := math.Pi * 100 * k * k
		got := coverageSum(img, img.Bounds())
		if math.Abs(got-want) > want*0.01 {
			t.Errorf("k=%v: circle area %.1f, want %.1f", k, got, want)
		}
	}
}

func TestStrokeCircleRingIsSmooth(t *testing.T) {
	img := render(t, 60, 60, 2, func(c widget.DrawContext) { StrokeCircle(c, 30, 30, 12, 2, white) })
	partial := map[uint8]bool{}
	for y := 0; y < img.Bounds().Dy(); y++ {
		for x := 0; x < img.Bounds().Dx(); x++ {
			if v := img.RGBAAt(x, y).R; v > 0 && v < 255 {
				partial[v] = true
			}
		}
	}
	if len(partial) < 20 {
		t.Errorf("only %d distinct coverage values on a ring drawn at 2x", len(partial))
	}
}

func TestShapesAreClippedToTheContextClip(t *testing.T) {
	img := render(t, 80, 40, 1.5, func(c widget.DrawContext) {
		c.SetClip(image.Rect(0, 0, 30, 40))
		FillRound(c, image.Rect(10, 8, 50, 28), 4, white)
	})
	if got := img.RGBAAt(PhysEdgeX(40, 1.5), 30); got != black {
		t.Errorf("shape leaked past the clip: %v", got)
	}
	if got := img.RGBAAt(PhysEdgeX(20, 1.5), 30); got != white {
		t.Errorf("shape missing inside the clip: %v", got)
	}
}

func PhysEdgeX(x int, k float64) int { return physEdge(x, k) }

func TestSemiTransparentFillBlendsOnce(t *testing.T) {
	half := Alpha(white, .5)
	img := render(t, 40, 40, 2, func(c widget.DrawContext) { FillRound(c, image.Rect(5, 5, 35, 35), 4, half) })
	got := img.RGBAAt(40, 40)
	if got.R < 126 || got.R > 130 {
		t.Errorf("half-transparent white over black = %v", got)
	}
}

func TestScaleOneStillUsesThePerPixelPath(t *testing.T) {
	logical := image.Rect(10, 8, 50, 28)
	img := render(t, 80, 40, 1, func(c widget.DrawContext) { FillRound(c, logical, 4, white) })
	if img.RGBAAt(30, 18) != white || img.RGBAAt(9, 18) != black || img.RGBAAt(50, 18) != black {
		t.Error("scale 1 rendering changed")
	}
}

func TestDistanceBasedShapesFollowTheScale(t *testing.T) {
	d := CapsuleDistanceForTest(10, 50, 20, 3)
	img := render(t, 60, 40, 1.5, func(c widget.DrawContext) { FillDistance(c, image.Rect(5, 10, 55, 30), d, white) })
	want := (math.Pi*9 + 34*6) * 1.5 * 1.5
	if got := coverageSum(img, img.Bounds()); math.Abs(got-want) > want*0.02 {
		t.Errorf("capsule area %.1f, want %.1f", got, want)
	}
}

func CapsuleDistanceForTest(x0, x1, y, radius float64) Distance {
	a, b := x0+radius, x1-radius
	return func(px, py float64) float64 {
		cx := math.Max(a, math.Min(b, px))
		return math.Hypot(px-cx, py-y) - radius
	}
}

func themeRing(contrast bool) Ring {
	r := Ring{Color: color.RGBA{255, 230, 0, 255}, Inner: color.RGBA{0, 0, 0, 255}}
	r.Contrast = contrast
	return r
}

func TestRingBandsFollowThePhysicalRectangleAtEveryScale(t *testing.T) {
	logical := image.Rect(20, 12, 60, 32)
	ring := themeRing(false)
	for _, k := range []float64{1, 1.25, 1.5, 1.75, 2} {
		img := render(t, 90, 50, k, func(c widget.DrawContext) { ring.Rect(c, logical, 4) })
		p := PhysRect(logical, k)
		width := int(math.Round(RingWidth * k))
		if k == 1 {
			width = RingWidth
		}
		midY := (p.Min.Y + p.Max.Y) / 2
		for i := 1; i <= width; i++ {
			if got := img.RGBAAt(p.Min.X-i, midY); got != ring.Color {
				t.Errorf("k=%v: pixel %d left of the control is %v", k, i, got)
			}
			if got := img.RGBAAt(p.Max.X+i-1, midY); got != ring.Color {
				t.Errorf("k=%v: pixel %d right of the control is %v", k, i, got)
			}
		}
		if got := img.RGBAAt(p.Min.X-width-1, midY); got != black {
			t.Errorf("k=%v: ring is wider than %d pixels: %v", k, width, got)
		}
		if got := img.RGBAAt(p.Min.X, midY); got != black {
			t.Errorf("k=%v: ring paints over the control: %v", k, got)
		}
	}
}

func TestHighContrastRingHasAnInnerLineAtEveryScale(t *testing.T) {
	logical := image.Rect(20, 12, 60, 32)
	ring := themeRing(true)
	for _, k := range []float64{1, 1.5, 2} {
		img := render(t, 90, 50, k, func(c widget.DrawContext) { ring.Rect(c, logical, 4) })
		p := PhysRect(logical, k)
		midY := (p.Min.Y + p.Max.Y) / 2
		wp := int(math.Round(RingWidth * k))
		inner := color.RGBA{0, 0, 0, 255}
		if got := img.RGBAAt(p.Min.X-1, midY); got != inner {
			t.Errorf("k=%v: inner line %v", k, got)
		}
		for i := 2; i < 2+wp; i++ {
			if got := img.RGBAAt(p.Min.X-i, midY); got != ring.Color {
				t.Errorf("k=%v: pixel %d left of the control is %v, want the ring colour", k, i, got)
			}
		}
	}
}

func TestCircleRingEnclosesTheCircleAtEveryScale(t *testing.T) {
	ring := themeRing(false)
	for _, k := range []float64{1, 1.5, 2} {
		img := render(t, 60, 60, k, func(c widget.DrawContext) { ring.Circle(c, 30, 30, 10) })
		cx := int(30 * k)
		y := int(30 * k)
		inside := img.RGBAAt(cx, y)
		if inside != black {
			t.Errorf("k=%v: the ring paints over the centre: %v", k, inside)
		}
		hit := false
		for x := cx + int(10*k); x < cx+int(13*k); x++ {
			if img.RGBAAt(x, y) == ring.Color {
				hit = true
			}
		}
		if !hit {
			t.Errorf("k=%v: no ring pixel just outside the radius", k)
		}
	}
}

func TestRingOfTheThemeUsesItsFocusColour(t *testing.T) {
	r := RingOf(themeFor("dark"))
	if r.Contrast || r.Color == (color.RGBA{}) {
		t.Errorf("dark ring %+v", r)
	}
	if hc := RingOf(themeFor("contrast")); !hc.Contrast {
		t.Error("the high-contrast theme must get the inner line")
	}
}

func TestFloatRectanglesSnapToThePhysicalGrid(t *testing.T) {
	r := RectF{10.5, 8.5, 50.5, 28.5}
	for _, k := range []float64{1.25, 1.5, 1.75, 2} {
		img := render(t, 80, 40, k, func(c widget.DrawContext) { FillRoundF(c, r, 4, white) })
		p := r.physical(k)
		midY := (p.Min.Y + p.Max.Y) / 2
		if img.RGBAAt(p.Min.X-1, midY) != black || img.RGBAAt(p.Min.X, midY) != white || img.RGBAAt(p.Max.X-1, midY) != white || img.RGBAAt(p.Max.X, midY) != black {
			t.Errorf("k=%v: edges of %v are not crisp", k, p)
		}
		img = render(t, 80, 40, k, func(c widget.DrawContext) { StrokeRoundF(c, r, 4, 1, white) })
		if img.RGBAAt(p.Min.X, midY) != white {
			t.Errorf("k=%v: the stroke misses the snapped edge", k)
		}
	}
}

func TestFloatRectangleRingMatchesTheIntegerOneOnWholePixels(t *testing.T) {
	ring := themeRing(false)
	logical := image.Rect(20, 12, 60, 32)
	for _, k := range []float64{1, 1.5, 2} {
		a := render(t, 90, 50, k, func(c widget.DrawContext) { ring.Rect(c, logical, 4) })
		b := render(t, 90, 50, k, func(c widget.DrawContext) {
			ring.RectF(c, RectF{20, 12, 60, 32}, 4)
		})
		for y := 0; y < a.Bounds().Dy(); y++ {
			for x := 0; x < a.Bounds().Dx(); x++ {
				if a.RGBAAt(x, y) != b.RGBAAt(x, y) {
					t.Fatalf("k=%v: rings differ at (%d,%d): %v vs %v", k, x, y, a.RGBAAt(x, y), b.RGBAAt(x, y))
				}
			}
		}
	}
}

func TestMaskedPaintingKeepsTheShapeAreaAndTint(t *testing.T) {
	shape := Circle(5, 5, 4)
	for _, k := range []float64{1.5, 2} {
		pw, ph := HiPhysSize(10, 10, 10, 10, k)
		mask := MaskHi(shape, pw, ph, k)
		img := render(t, 40, 40, k, func(c widget.DrawContext) {
			PaintMaskHi(c, mask, 10, 10, 10, 10, k, func(row, rows int) color.RGBA { return white })
		})
		want := math.Pi * 16 * k * k
		if got := coverageSum(img, img.Bounds()); math.Abs(got-want) > want*0.04 {
			t.Errorf("k=%v: painted area %.1f, want %.1f", k, got, want)
		}
	}
}

func TestMaskedPaintingBlendsTranslucentTintsOnce(t *testing.T) {
	mask := make([]uint8, 20*20)
	for i := range mask {
		mask[i] = 255
	}
	pw, ph := HiPhysSize(5, 5, 10, 10, 2)
	if pw != 20 || ph != 20 {
		t.Fatalf("physical size %dx%d", pw, ph)
	}
	half := color.RGBA{255, 255, 255, 128}
	img := render(t, 30, 30, 2, func(c widget.DrawContext) {
		PaintMaskHi(c, mask, 5, 5, 10, 10, 2, func(int, int) color.RGBA { return half })
	})
	if got := img.RGBAAt(20, 20).R; got < 126 || got > 130 {
		t.Errorf("a half-transparent tint over black gives %d, want about 128", got)
	}
}

func themeFor(name string) *theme.Theme { return theme.ByName(name) }
