package settings

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"path/filepath"
	"slices"
	"sort"
	"testing"

	"github.com/oops1/PixGo/internal/testutil"
)

func maxChannelDiff(a, b color.RGBA) float64 {
	d := func(x, y uint8) float64 { return math.Abs(float64(x) - float64(y)) }
	return math.Max(d(a.R, b.R), math.Max(d(a.G, b.G), d(a.B, b.B)))
}

func borderMedian(img *image.RGBA, r image.Rectangle) color.RGBA {
	var rs, gs, bs []int
	add := func(x, y int) {
		c := img.RGBAAt(x, y)
		rs, gs, bs = append(rs, int(c.R)), append(gs, int(c.G)), append(bs, int(c.B))
	}
	for x := r.Min.X; x < r.Max.X; x++ {
		add(x, r.Min.Y)
		add(x, r.Max.Y-1)
	}
	for y := r.Min.Y + 1; y < r.Max.Y-1; y++ {
		add(r.Min.X, y)
		add(r.Max.X-1, y)
	}
	med := func(v []int) uint8 {
		sort.Ints(v)
		return uint8(v[len(v)/2])
	}
	return color.RGBA{med(rs), med(gs), med(bs), 255}
}

func inkBoxIn(img *image.RGBA, r image.Rectangle) image.Rectangle {
	r = r.Intersect(img.Bounds())
	bg := borderMedian(img, r)
	var peak float64
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			peak = math.Max(peak, maxChannelDiff(img.RGBAAt(x, y), bg))
		}
	}
	th := peak * 0.4
	box := image.Rectangle{Min: image.Pt(1<<30, 1<<30), Max: image.Pt(-1, -1)}
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if maxChannelDiff(img.RGBAAt(x, y), bg) >= th {
				box.Min.X, box.Min.Y = min(box.Min.X, x), min(box.Min.Y, y)
				box.Max.X, box.Max.Y = max(box.Max.X, x+1), max(box.Max.Y, y+1)
			}
		}
	}
	return box
}

type probe struct {
	name string
	ink  image.Rectangle
	tol  int
}

func ib(x0, y0, x1, y1 int) image.Rectangle { return image.Rect(x0, y0, x1+1, y1+1) }

var headerProbes = []probe{
	{"back", ib(16, 13, 26, 22), 1},
	{"title", ib(40, 14, 90, 23), 2},
}

var navProbes = []probe{
	{"icon general", ib(15, 50, 26, 61), 1},
	{"icon view", ib(15, 80, 26, 89), 1},
	{"icon formats", ib(16, 109, 26, 120), 1},
	{"icon slideshow", ib(15, 139, 26, 150), 1},
	{"icon performance", ib(15, 170, 26, 179), 1},
	{"icon language", ib(16, 199, 26, 210), 1},
	{"icon about", ib(15, 228, 26, 239), 1},
	{"text general", ib(40, 52, 72, 60), 2},
	{"text view", ib(40, 82, 86, 91), 2},
	{"text formats", ib(40, 111, 82, 120), 2},
	{"text slideshow", ib(40, 141, 91, 150), 2},
	{"text performance", ib(40, 171, 138, 180), 3},
	{"text language", ib(40, 201, 62, 208), 2},
	{"text about", ib(40, 231, 102, 240), 2},
}

var contentProbes = []probe{
	{"title", ib(169, 48, 227, 64), 2},
	{"heading theme", ib(169, 80, 252, 89), 2},
	{"label system", ib(185, 150, 234, 157), 2},
	{"label light", ib(287, 150, 322, 157), 2},
	{"label dark", ib(384, 150, 416, 157), 2},
	{"heading language", ib(169, 179, 250, 188), 2},
	{"dropdown text", ib(178, 205, 277, 214), 2},
	{"dropdown chevron", ib(427, 207, 431, 209), 1},
	{"box 1", ib(168, 235, 181, 248), 1},
	{"box 2", ib(168, 257, 181, 270), 1},
	{"box 3", ib(168, 280, 181, 292), 1},
	{"box 4", ib(168, 302, 181, 314), 1},
	{"check 1", ib(190, 238, 372, 247), 2},
	{"check 2", ib(190, 260, 448, 269), 2},
	{"check 3", ib(190, 282, 342, 292), 2},
	{"check 4", ib(189, 305, 405, 314), 2},
	{"heading background", ib(168, 336, 335, 345), 2},
	{"bg label system", ib(174, 398, 220, 404), 2},
	{"bg label white", ib(256, 398, 281, 404), 2},
	{"bg label black", ib(324, 398, 355, 406), 2},
	{"bg label checker", ib(387, 398, 435, 404), 2},
}

func renderMock(t *testing.T) (*rig, *image.RGBA) {
	t.Helper()
	r := newRig(t, mockW, mockH)
	return r, r.render()
}

func logProbes(t *testing.T, ref, got *image.RGBA, probes []probe) []string {
	t.Helper()
	var bad []string
	for _, p := range probes {
		region := p.ink.Inset(-4)
		want, have := inkBoxIn(ref, region), inkBoxIn(got, region)
		d := [4]int{have.Min.X - want.Min.X, have.Min.Y - want.Min.Y, have.Max.X - want.Max.X, have.Max.Y - want.Max.Y}
		t.Logf("%-18s mock %v ours %v d=%v", p.name, want, have, d)
		if slices.ContainsFunc(d[:], func(v int) bool { return v < -p.tol || v > p.tol }) {
			bad = append(bad, fmt.Sprintf("%s: ink %v vs mock %v d=%v", p.name, have, want, d))
		}
	}
	return bad
}

type areaCheck struct {
	name   string
	rect   image.Rectangle
	maxErr float64
}

func TestMockupRegions(t *testing.T) {
	ref, err := testutil.LoadRef("04_settings.png")
	if err != nil {
		t.Skip(err)
	}
	_, got := renderMock(t)
	if out := testutil.OutDir(); out != "" {
		testutil.SavePNG(filepath.Join(out, "settings_golden.png"), got)
		testutil.SaveComparison(filepath.Join(out, "settings_compare.png"), got, ref)
	}
	areas := []areaCheck{
		{"header", image.Rect(3, 2, 350, 33), 3.6},
		{"nav", image.Rect(3, 34, 154, 427), 4.7},
		{"title", image.Rect(160, 40, 440, 75), 4.1},
		{"theme cards", image.Rect(160, 90, 450, 147), 11.5},
		{"language", image.Rect(160, 170, 450, 225), 5.7},
		{"checks", image.Rect(160, 228, 450, 322), 16.5},
		{"background cards", image.Rect(160, 330, 450, 410), 12.5},
	}
	for _, a := range areas {
		e := testutil.MeanError(got, ref, a.rect)
		t.Logf("%-18s mean error %.2f", a.name, e)
		if e > a.maxErr {
			t.Errorf("%s mean error %.2f > %.2f", a.name, e, a.maxErr)
		}
	}
	var bad []string
	for _, ps := range [][]probe{headerProbes, navProbes, contentProbes} {
		bad = append(bad, logProbes(t, ref, got, ps)...)
	}
	for _, b := range bad {
		t.Error(b)
	}
}

func meanOf(img *image.RGBA, r image.Rectangle) color.RGBA {
	var sr, sg, sb, n int
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			c := img.RGBAAt(x, y)
			sr, sg, sb, n = sr+int(c.R), sg+int(c.G), sb+int(c.B), n+1
		}
	}
	return color.RGBA{uint8((sr + n/2) / n), uint8((sg + n/2) / n), uint8((sb + n/2) / n), 255}
}

func TestMockupFlatColours(t *testing.T) {
	ref, err := testutil.LoadRef("04_settings.png")
	if err != nil {
		t.Skip(err)
	}
	_, got := renderMock(t)
	for _, c := range []struct {
		name string
		r    image.Rectangle
		tol  int
	}{
		{"header", image.Rect(200, 4, 330, 30), 3},
		{"header rule", image.Rect(160, 33, 440, 34), 4},
		{"nav", image.Rect(10, 300, 150, 420), 3},
		{"selected row", image.Rect(100, 45, 146, 67), 4},
		{"nav indicator", image.Rect(8, 50, 9, 60), 8},
		{"divider", image.Rect(155, 100, 156, 400), 4},
		{"page", image.Rect(300, 36, 440, 44), 3},
		{"page below", image.Rect(170, 62, 440, 66), 3},
		{"field", image.Rect(330, 200, 420, 216), 3},
		{"checked box", image.Rect(170, 237, 172, 247), 10},
		{"white card", image.Rect(255, 360, 285, 384), 5},
		{"black card", image.Rect(325, 360, 355, 384), 6},
	} {
		want, have := meanOf(ref, c.r), meanOf(got, c.r)
		t.Logf("%-14s mock %v ours %v", c.name, want, have)
		if maxChannelDiff(want, have) > float64(c.tol) {
			t.Errorf("%s: mockup %v, ours %v (tolerance %d)", c.name, want, have, c.tol)
		}
	}
}
