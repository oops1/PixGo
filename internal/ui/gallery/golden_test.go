package gallery

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
	{"back", ib(20, 17, 30, 26), 1},
	{"forward", ib(47, 17, 56, 26), 1},
	{"up", ib(73, 17, 80, 26), 1},
	{"folder", ib(101, 15, 114, 27), 1},
	{"address", ib(126, 18, 212, 27), 2},
	{"filter", ib(340, 17, 349, 25), 1},
	{"go", ib(365, 19, 371, 24), 1},
	{"viewList", ib(400, 17, 410, 26), 1},
	{"viewGrid", ib(428, 16, 440, 27), 1},
	{"thumbs", ib(460, 17, 470, 26), 1},
}

var sidebarProbes = []probe{
	{"icon images", ib(22, 59, 33, 70), 1},
	{"icon recent", ib(22, 81, 34, 93), 1},
	{"icon favorites", ib(22, 105, 33, 115), 1},
	{"icon desktop", ib(25, 159, 37, 170), 1},
	{"icon pictures", ib(25, 181, 37, 192), 1},
	{"icon video", ib(25, 202, 37, 214), 1},
	{"icon documents", ib(26, 224, 36, 236), 1},
	{"icon download", ib(26, 246, 35, 258), 1},
	{"icon system drive", ib(25, 302, 37, 312), 1},
	{"icon drive", ib(25, 326, 37, 334), 1},
	{"icon network", ib(23, 350, 37, 362), 1},
	{"text all", ib(43, 61, 125, 71), 2},
	{"text recent", ib(43, 84, 88, 93), 2},
	{"text favorites", ib(43, 107, 95, 116), 2},
	{"head folders", ib(21, 137, 51, 145), 2},
	{"text desktop", ib(45, 161, 107, 168), 2},
	{"text pictures", ib(45, 183, 109, 192), 2},
	{"text videos", ib(45, 205, 72, 213), 2},
	{"text documents", ib(44, 226, 97, 236), 2},
	{"text downloads", ib(44, 248, 85, 258), 2},
	{"head computer", ib(21, 279, 103, 289), 2},
	{"text system", ib(44, 304, 103, 313), 2},
	{"text data", ib(45, 327, 84, 336), 2},
	{"text network", ib(44, 353, 65, 360), 2},
}

var gridProbes = []probe{
	{"label lake", ib(299, 135, 332, 144), 2},
	{"label forest", ib(401, 135, 441, 144), 2},
	{"label flower", ib(505, 135, 548, 144), 2},
	{"label city", ib(195, 237, 226, 246), 2},
	{"label autumm", ib(290, 237, 341, 246), 2},
	{"label beach", ib(400, 237, 443, 246), 2},
	{"label night", ib(508, 237, 546, 246), 2},
	{"label animal", ib(188, 339, 233, 348), 2},
	{"label macro", ib(294, 339, 337, 348), 2},
	{"label desert", ib(399, 339, 443, 349), 2},
	{"label snow", ib(508, 339, 546, 348), 2},
}

var statusProbes = []probe{
	{"status selected", ib(23, 404, 125, 415), 2},
	{"status size", ib(142, 405, 182, 414), 2},
	{"status dimensions", ib(200, 405, 262, 412), 3},
	{"button label", ib(472, 395, 520, 406), 2},
	{"button chevron", ib(555, 397, 563, 401), 1},
}

var photoColumns = []int{164, 269, 374, 480}
var photoRows = []int{57, 159, 261}

func photoMask() []image.Rectangle {
	var out []image.Rectangle
	for _, y := range photoRows {
		for _, x := range photoColumns {
			out = append(out, image.Rect(x-2, y-2, x+97, y+73))
		}
	}
	return out
}

func renderMock(t *testing.T) (*rig, *image.RGBA) {
	t.Helper()
	r := newRig(t, mockSource(), mockW, mockH)
	r.src.selected = 0
	r.page.Refresh()
	hx, hy := r.tileCenter(7)
	r.page.OnMouseMove(hx, hy)
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
	mask   []image.Rectangle
	maxErr float64
}

func TestMockupRegions(t *testing.T) {
	ref, err := testutil.LoadRef("03_gallery.png")
	if err != nil {
		t.Skip(err)
	}
	_, got := renderMock(t)
	if out := testutil.OutDir(); out != "" {
		testutil.SavePNG(filepath.Join(out, "gallery_golden.png"), got)
		testutil.SaveComparison(filepath.Join(out, "gallery_compare.png"), got, ref)
	}
	areas := []areaCheck{
		{"header", image.Rect(2, 1, 488, 43), nil, 5.5},
		{"sidebar", image.Rect(2, 43, 152, 389), nil, 7.5},
		{"page", image.Rect(152, 43, 588, 389), photoMask(), 7.5},
		{"status", image.Rect(2, 389, 448, 429), nil, 5.5},
		{"buttons", image.Rect(448, 380, 590, 420), nil, 8.8},
	}
	for _, a := range areas {
		e := testutil.MeanError(got, ref, a.rect, a.mask...)
		t.Logf("%-8s mean error %.2f", a.name, e)
		if e > a.maxErr {
			t.Errorf("%s mean error %.2f > %.2f", a.name, e, a.maxErr)
		}
	}
	var bad []string
	for _, ps := range [][]probe{headerProbes, sidebarProbes, gridProbes, statusProbes} {
		bad = append(bad, logProbes(t, ref, got, ps)...)
	}
	for _, b := range bad {
		t.Error(b)
	}
}

func TestMockupFlatColours(t *testing.T) {
	ref, err := testutil.LoadRef("03_gallery.png")
	if err != nil {
		t.Skip(err)
	}
	_, got := renderMock(t)
	for _, c := range []struct {
		name string
		r    image.Rectangle
		tol  int
	}{
		{"header", image.Rect(215, 2, 330, 7), 3},
		{"address field", image.Rect(215, 12, 330, 31), 2},
		{"sidebar", image.Rect(4, 44, 148, 56), 2},
		{"sidebar bottom", image.Rect(4, 364, 148, 388), 2},
		{"selected row", image.Rect(100, 322, 140, 340), 3},
		{"divider", image.Rect(151, 100, 152, 380), 3},
		{"page", image.Rect(160, 346, 580, 380), 2},
		{"selection fill", image.Rect(168, 130, 200, 133), 2},
		{"status", image.Rect(160, 393, 440, 427), 2},
		{"status rule", image.Rect(160, 389, 440, 390), 2},
		{"open button", image.Rect(455, 386, 468, 412), 2},
		{"more button", image.Rect(546, 386, 553, 412), 2},
	} {
		want, have := meanOf(ref, c.r), meanOf(got, c.r)
		if maxChannelDiff(want, have) > float64(c.tol) {
			t.Errorf("%s: mockup %v, ours %v (tolerance %d)", c.name, want, have, c.tol)
		}
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
