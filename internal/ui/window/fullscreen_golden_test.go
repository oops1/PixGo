package window

import (
	"image"
	"image/color"
	"math"
	"path/filepath"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/testutil"
	"github.com/oops1/PixGo/internal/ui/theme"
)

const (
	mockupW = 327
	mockupH = 343
)

type photoWidget struct {
	widget.Base
	img *image.RGBA
}

func (p *photoWidget) Draw(c widget.DrawContext) {
	b := p.Bounds()
	c.DrawImage(p.img, b.Min.X, b.Min.Y)
}

func (p *photoWidget) HasImage() bool { return true }

type goldenRig struct {
	eng  *engine.Engine
	root *Root
	host *fakeHost
}

func newGoldenRig(t testing.TB, w, h int, photo *image.RGBA, mutate func(*fakeHost)) *goldenRig {
	t.Helper()
	i18n.SetLang("ru")
	host := newHost()
	host.fullscreen = true
	if photo != nil {
		host.contentOverride = &photoWidget{img: photo}
	}
	if mutate != nil {
		mutate(host)
	}
	eng := testutil.NewEngine(w, h)
	root := New(host, eng, WindowOps{})
	root.Theme = theme.Dark()
	eng.SetRootFullCanvas(root)
	eng.Flush()
	return &goldenRig{eng: eng, root: root, host: host}
}

func (g *goldenRig) render() *image.RGBA {
	g.root.Refresh()
	g.eng.Flush()
	return g.eng.RenderOnce()
}

func slideshowMockupView() SlideshowView {
	return SlideshowView{Active: true, Playing: true, Index: 5, Total: 24, Progress: 0.4236, Interval: 5 * time.Second}
}

func mockupLayout() Layout {
	return Compute(mockupW, mockupH, Options{Fullscreen: true, Toolbar: true, Arrows: true, CounterDigits: 2})
}

func slideContent(l Layout) []image.Rectangle {
	sl := l.Slide
	var out []image.Rectangle
	for _, z := range []image.Rectangle{sl.Play, sl.Settings, sl.Exit} {
		cx, cy := (z.Min.X+z.Max.X)/2, (z.Min.Y+z.Max.Y)/2
		out = append(out, image.Rect(cx-11, cy-11, cx+11, cy+11))
	}
	return append(out, image.Rect(sl.CounterX-2, sl.CounterBase-12, sl.CounterX+36, sl.CounterBase+4), sl.Damage.Inset(-1))
}

func mockupPhoto(mock *image.RGBA, l Layout, th *theme.Theme) *image.RGBA {
	out := image.NewRGBA(mock.Bounds())
	copy(out.Pix, mock.Pix)
	fill := th.SlideBar
	k := 1 - float64(fill.A)/255
	unblend := func(c color.RGBA) color.RGBA {
		f := func(v, p uint8) uint8 {
			return uint8(min(max((float64(v)-float64(p))/k, 0), 255) + .5)
		}
		return color.RGBA{f(c.R, fill.R), f(c.G, fill.G), f(c.B, fill.B), 255}
	}
	panel := l.Slide.Panel
	core := panel.Inset(3)
	for y := panel.Min.Y; y < panel.Max.Y; y++ {
		for x := panel.Min.X; x < panel.Max.X; x++ {
			sx, sy := min(max(x, core.Min.X), core.Max.X-1), min(max(y, core.Min.Y), core.Max.Y-1)
			out.SetRGBA(x, y, unblend(mock.RGBAAt(sx, sy)))
		}
	}
	for _, c := range slideContent(l) {
		for y := c.Min.Y; y < c.Max.Y; y++ {
			for x := c.Min.X; x < c.Max.X; x++ {
				out.SetRGBA(x, y, out.RGBAAt(x, c.Min.Y-2))
			}
		}
	}
	for _, a := range []image.Rectangle{l.Prev, l.Next} {
		cx, cy := float64(a.Min.X+a.Max.X)/2, float64(a.Min.Y+a.Max.Y)/2+fullArrowShiftY
		inner, outer := fullArrowRadius-3, fullArrowRadius+2.2
		for y := a.Min.Y - 3; y < a.Max.Y+3; y++ {
			for x := a.Min.X - 3; x < a.Max.X+3; x++ {
				dx, dy := float64(x)+.5-cx, float64(y)+.5-cy
				d := math.Hypot(dx, dy)
				switch {
				case d > fullArrowRadius+1.6:
				case d > fullArrowRadius:
					out.SetRGBA(x, y, mock.RGBAAt(int(math.Floor(cx+dx*outer/d)), int(math.Floor(cy+dy*outer/d))))
				case d > inner:
					out.SetRGBA(x, y, unblend(mock.RGBAAt(int(math.Floor(cx+dx*inner/d)), int(math.Floor(cy+dy*inner/d)))))
				default:
					out.SetRGBA(x, y, unblend(mock.RGBAAt(x, y)))
				}
			}
		}
	}
	return out
}

func inkWhere(img *image.RGBA, r image.Rectangle, pred func(c color.RGBA) bool) image.Rectangle {
	out := image.Rectangle{Min: image.Pt(1<<30, 1<<30), Max: image.Pt(-1, -1)}
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if pred(img.RGBAAt(x, y)) {
				out.Min.X, out.Min.Y = min(out.Min.X, x), min(out.Min.Y, y)
				out.Max.X, out.Max.Y = max(out.Max.X, x+1), max(out.Max.Y, y+1)
			}
		}
	}
	return out
}

func lumOf(c color.RGBA) int { return (int(c.R)*299 + int(c.G)*587 + int(c.B)*114) / 1000 }

func lightAbove(th int) func(color.RGBA) bool {
	return func(c color.RGBA) bool { return lumOf(c) > th }
}

func blueish(c color.RGBA) bool { return int(c.B)-int(c.R) > 110 }

func greyTrack(c color.RGBA) bool { return lumOf(c) > 85 && !blueish(c) }

func renderSlideshowMockup(t *testing.T) (got, mock *image.RGBA) {
	t.Helper()
	mock, err := testutil.LoadRef("05_fullscreen_slideshow.png")
	if err != nil {
		t.Skip(err)
	}
	photo := mockupPhoto(mock, mockupLayout(), theme.Dark())
	g := newGoldenRig(t, mockupW, mockupH, photo, func(h *fakeHost) { h.slide = slideshowMockupView() })
	got = g.render()
	if out := testutil.OutDir(); out != "" {
		testutil.SavePNG(filepath.Join(out, "fullscreen.png"), got)
		testutil.SaveComparison(filepath.Join(out, "fullscreen_compare.png"), got, mock)
	}
	return got, mock
}

func TestFullscreenChromeInkMatchesMockup(t *testing.T) {
	got, mock := renderSlideshowMockup(t)
	probes := []struct {
		name string
		r    image.Rectangle
		pred func(color.RGBA) bool
		tol  int
	}{
		{"close", image.Rect(285, 5, 320, 40), lightAbove(120), 1},
		{"prev glyph", image.Rect(12, 150, 42, 182), lightAbove(200), 1},
		{"next glyph", image.Rect(285, 150, 318, 182), lightAbove(200), 1},
		{"pause", image.Rect(30, 295, 60, 326), lightAbove(160), 1},
		{"counter", image.Rect(65, 295, 112, 326), lightAbove(170), 1},
		{"gear", image.Rect(225, 292, 262, 328), lightAbove(140), 1},
		{"exit", image.Rect(268, 292, 305, 328), lightAbove(140), 1},
		{"progress fill", image.Rect(100, 295, 230, 325), blueish, 0},
		{"track", image.Rect(160, 300, 230, 320), greyTrack, 1},
	}
	for _, p := range probes {
		want, have := inkWhere(mock, p.r, p.pred), inkWhere(got, p.r, p.pred)
		d := [4]int{have.Min.X - want.Min.X, have.Min.Y - want.Min.Y, have.Max.X - want.Max.X, have.Max.Y - want.Max.Y}
		t.Logf("%-14s mockup %v ours %v d=%v", p.name, want, have, d)
		for _, v := range d {
			if v < -p.tol || v > p.tol {
				t.Errorf("%s: ink box %v is off the mockup %v by more than %d px", p.name, have, want, p.tol)
				break
			}
		}
	}
}

func TestFullscreenChromeRegionsMatchMockup(t *testing.T) {
	got, mock := renderSlideshowMockup(t)
	for _, r := range []struct {
		name string
		rc   image.Rectangle
		max  float64
	}{
		{"panel", image.Rect(16, 285, 312, 335), 5.5},
		{"track", image.Rect(112, 302, 224, 318), 6},
		{"prev arrow", image.Rect(6, 144, 48, 186), 4.3},
		{"next arrow", image.Rect(281, 144, 323, 186), 5},
		{"close", image.Rect(285, 5, 320, 40), 2},
	} {
		e := testutil.MeanError(got, mock, r.rc)
		t.Logf("%-10s mean error vs mockup: %.2f", r.name, e)
		if e > r.max {
			t.Errorf("%s: mean error %.2f exceeds %.1f", r.name, e, r.max)
		}
	}
}

func TestFullscreenPanelEdgesMatchMockup(t *testing.T) {
	got, mock := renderSlideshowMockup(t)
	l := mockupLayout()
	if l.Slide.Panel != image.Rect(18, 287, 310, 333) {
		t.Fatalf("panel = %v, want the mockup rectangle (18,287)-(310,333)", l.Slide.Panel)
	}
	cases := []struct {
		name string
		p    image.Point
		tol  int
	}{
		{"left border", image.Pt(18, 304), 12},
		{"left border", image.Pt(18, 310), 12},
		{"left border", image.Pt(18, 316), 12},
		{"left outside", image.Pt(17, 310), 0},
		{"top border", image.Pt(100, 287), 14},
		{"bottom border", image.Pt(100, 332), 14},
		{"bottom border", image.Pt(60, 332), 14},
		{"bottom outside", image.Pt(60, 333), 0},
		{"top outside", image.Pt(100, 286), 0},
	}
	for _, c := range cases {
		a, b := got.RGBAAt(c.p.X, c.p.Y), mock.RGBAAt(c.p.X, c.p.Y)
		if d := lumOf(a) - lumOf(b); d < -c.tol || d > c.tol {
			t.Errorf("%s %v: ours %v mockup %v", c.name, c.p, a, b)
		}
	}
}

func TestFullscreenWithoutSlideshowShowsPlayAndAnEmptyTrack(t *testing.T) {
	mock, err := testutil.LoadRef("05_fullscreen_slideshow.png")
	if err != nil {
		t.Skip(err)
	}
	photo := mockupPhoto(mock, mockupLayout(), theme.Dark())
	idle := newGoldenRig(t, mockupW, mockupH, photo, func(h *fakeHost) {
		h.slide = SlideshowView{Index: 5, Total: 24, Interval: 5 * time.Second}
	}).render()
	running := newGoldenRig(t, mockupW, mockupH, photo, func(h *fakeHost) { h.slide = slideshowMockupView() }).render()

	if box := inkWhere(idle, image.Rect(100, 295, 230, 325), blueish); !box.Empty() {
		t.Errorf("an idle panel must not show progress, found %v", box)
	}
	if box := inkWhere(idle, image.Rect(100, 300, 230, 320), greyTrack); box.Dx() < 100 {
		t.Errorf("the idle track must still be visible, got %v", box)
	}
	button := image.Rect(30, 295, 60, 326)
	if inkWhere(idle, button, lightAbove(160)).Empty() || testutil.MeanError(idle, running, button) < 3 {
		t.Error("play and pause must both be drawn and look different")
	}
	if e := testutil.MeanError(idle, running, image.Rect(220, 285, 312, 335)); e > .5 {
		t.Errorf("the settings and exit buttons must not depend on the slideshow state, difference %.2f", e)
	}
}
