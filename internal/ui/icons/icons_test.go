package icons

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/testutil"
	"github.com/oops1/PixGo/internal/ui/gfx"
)

type canvas struct {
	widget.DrawContext
	img  *image.RGBA
	clip image.Rectangle
}

func newCanvas(w, h int, bg color.RGBA) *canvas {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), image.NewUniform(bg), image.Point{}, draw.Src)
	return &canvas{img: img, clip: img.Bounds()}
}

func (c *canvas) FillRect(x, y, w, h int, col color.RGBA) {
	r := image.Rect(x, y, x+w, y+h).Intersect(c.clip)
	draw.Draw(c.img, r, image.NewUniform(col), image.Point{}, draw.Src)
}

func (c *canvas) FillRectAlpha(x, y, w, h int, col color.RGBA) {
	r := image.Rect(x, y, x+w, y+h).Intersect(c.clip)
	draw.Draw(c.img, r, image.NewUniform(col), image.Point{}, draw.Over)
}

func (c *canvas) Clip() image.Rectangle     { return c.clip }
func (c *canvas) SetClip(r image.Rectangle) { c.clip = r }

var specs = map[string]*image.RGBA{}

func spec(t *testing.T, file string) *image.RGBA {
	if m, ok := specs[file]; ok {
		return m
	}
	m, err := testutil.LoadRef(file)
	if err != nil {
		t.Fatal(err)
	}
	specs[file] = m
	return m
}

var (
	white = color.RGBA{255, 255, 255, 255}
	dim   = color.RGBA{130, 134, 140, 255}
	mid   = color.RGBA{180, 184, 190, 255}
)

type tcase struct {
	name   string
	file   string
	x, y   int
	fg     color.RGBA
	fg2    color.RGBA
	bgAt   image.Point
	margin int
	maxErr float64
	box    int
}

const (
	m1 = "01_main_viewer.png"
	m2 = "02_exif_panel.png"
	m3 = "03_gallery.png"
	m4 = "04_settings.png"
	m5 = "05_fullscreen_slideshow.png"
)

var cases = []tcase{
	{name: "back", file: m1, x: 22, y: 17},
	{name: "info", file: m1, x: 493, y: 15},
	{name: "heart", file: m1, x: 538, y: 15, maxErr: 18},
	{name: "more-title", file: m1, x: 585, y: 17},
	{name: "caption-min", file: m1, x: 640, y: 19},
	{name: "caption-max", file: m1, x: 686, y: 18, maxErr: 16},
	{name: "caption-close", file: m1, x: 735, y: 18},
	{name: "gallery", file: m1, x: 138, y: 457, maxErr: 17},
	{name: "delete", file: m1, x: 196, y: 457},
	{name: "rotate-left", file: m1, x: 263, y: 457, maxErr: 19},
	{name: "zoom-out", file: m1, x: 320, y: 457},
	{name: "zoom-in", file: m1, x: 428, y: 457},
	{name: "rotate-right", file: m1, x: 493, y: 457, maxErr: 21},
	{name: "more", file: m1, x: 553, y: 463, maxErr: 16},
	{name: "fullscreen", file: m1, x: 609, y: 457},
	{name: "chevron-left", file: m1, x: 26, y: 249},
	{name: "chevron-right", file: m1, x: 724, y: 249},
	{name: "close", file: m2, x: 683, y: 64},
	{name: "copy", file: m2, x: 681, y: 266, maxErr: 17},
	{name: "arrow-left", file: m3, x: 17, y: 14},
	{name: "arrow-right", file: m3, x: 44, y: 14, fg: dim},
	{name: "arrow-right-short", file: m3, x: 68, y: 14, fg: mid, maxErr: 20},
	{name: "filter", file: m3, x: 337, y: 14, maxErr: 22},
	{name: "sort", file: m3, x: 362, y: 16, fg: mid, maxErr: 20},
	{name: "view-details", file: m3, x: 398, y: 14, maxErr: 29},
	{name: "view-tiles", file: m3, x: 426, y: 14, maxErr: 27},
	{name: "view-grid", file: m3, x: 457, y: 14, maxErr: 20},
	{name: "folder", file: m3, x: 100, y: 14},
	{name: "images", file: m3, x: 20, y: 57, maxErr: 34},
	{name: "recent", file: m3, x: 20, y: 80, maxErr: 23},
	{name: "heart-small", file: m3, x: 20, y: 102, maxErr: 24},
	{name: "desktop", file: m3, x: 23, y: 158, maxErr: 16},
	{name: "pictures", file: m3, x: 23, y: 179, maxErr: 17},
	{name: "video", file: m3, x: 23, y: 200, maxErr: 17},
	{name: "documents", file: m3, x: 23, y: 222, maxErr: 22},
	{name: "download", file: m3, x: 23, y: 244},
	{name: "drive-system", file: m3, x: 23, y: 299, maxErr: 26},
	{name: "drive", file: m3, x: 23, y: 322, maxErr: 25},
	{name: "network", file: m3, x: 22, y: 348},
	{name: "expander", file: m3, x: 12, y: 327, fg: dim},
	{name: "chevron-down", file: m3, x: 553, y: 393},
	{name: "settings", file: m4, x: 13, y: 48, maxErr: 33},
	{name: "view", file: m4, x: 13, y: 77, maxErr: 29},
	{name: "formats", file: m4, x: 13, y: 106, maxErr: 31},
	{name: "slideshow", file: m4, x: 13, y: 136, maxErr: 19},
	{name: "performance", file: m4, x: 13, y: 165, maxErr: 50},
	{name: "language", file: m4, x: 13, y: 195, maxErr: 27},
	{name: "about", file: m4, x: 13, y: 225, maxErr: 20},
	{name: "checkmark", file: m4, x: 169, y: 236, bgAt: image.Pt(172, 238), margin: -1},
	{name: "chevron-down-sm", file: m4, x: 424, y: 202},
	{name: "caption-min-sm", file: m3, x: 500, y: 16},
	{name: "caption-max-sm", file: m3, x: 533, y: 15},
	{name: "caption-close-sm", file: m3, x: 567, y: 15, maxErr: 24},
	{name: "arrow-left", file: m4, x: 13, y: 10, maxErr: 16},
	{name: "pause", file: m5, x: 33, y: 300},
	{name: "settings-bar", file: m5, x: 235, y: 300, maxErr: 32},
	{name: "exit-fullscreen", file: m5, x: 275, y: 300, maxErr: 23},
	{name: "close", file: m5, x: 301, y: 14},
	{name: "chevron-left-sm", file: m5, x: 17, y: 158},
	{name: "chevron-right-sm", file: m5, x: 296, y: 158},
}

func chdiff(a, b color.RGBA) float64 {
	d := func(x, y uint8) float64 { return math.Abs(float64(x) - float64(y)) }
	return (d(a.R, b.R) + d(a.G, b.G) + d(a.B, b.B)) / 3
}

func maxdiff(a, b color.RGBA) float64 {
	d := func(x, y uint8) float64 { return math.Abs(float64(x) - float64(y)) }
	return math.Max(d(a.R, b.R), math.Max(d(a.G, b.G), d(a.B, b.B)))
}

func median(v []int) uint8 {
	sort.Ints(v)
	return uint8(v[len(v)/2])
}

func estimateBg(m *image.RGBA, r image.Rectangle) color.RGBA {
	var rs, gs, bs []int
	add := func(x, y int) {
		c := m.RGBAAt(x, y)
		rs = append(rs, int(c.R))
		gs = append(gs, int(c.G))
		bs = append(bs, int(c.B))
	}
	for x := r.Min.X; x < r.Max.X; x++ {
		add(x, r.Min.Y)
		add(x, r.Max.Y-1)
	}
	for y := r.Min.Y + 1; y < r.Max.Y-1; y++ {
		add(r.Min.X, y)
		add(r.Max.X-1, y)
	}
	return color.RGBA{median(rs), median(gs), median(bs), 255}
}

func inkBox(m *image.RGBA, r image.Rectangle, bg color.RGBA, th float64) image.Rectangle {
	box := image.Rectangle{Min: image.Pt(1<<30, 1<<30), Max: image.Pt(-1, -1)}
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if maxdiff(m.RGBAAt(x, y), bg) >= th {
				box.Min.X = min(box.Min.X, x)
				box.Min.Y = min(box.Min.Y, y)
				box.Max.X = max(box.Max.X, x+1)
				box.Max.Y = max(box.Max.Y, y+1)
			}
		}
	}
	return box
}

func zoom(dst *image.RGBA, src *image.RGBA, sr image.Rectangle, ox, oy, k int) {
	for y := 0; y < sr.Dy()*k; y++ {
		for x := 0; x < sr.Dx()*k; x++ {
			dst.SetRGBA(ox+x, oy+y, src.RGBAAt(sr.Min.X+x/k, sr.Min.Y+y/k))
		}
	}
}

type result struct {
	name    string
	err     float64
	ink     image.Rectangle
	mine    image.Rectangle
	sheet   *image.RGBA
	box     image.Rectangle
	bg      color.RGBA
	overall float64
	soft    float64
}

func run(t *testing.T, tc tcase) result {
	m := spec(t, tc.file)
	d, ok := defs[tc.name]
	if !ok {
		t.Fatalf("%s: no such icon", tc.name)
	}
	box := image.Rect(tc.x, tc.y, tc.x+d.size.W, tc.y+d.size.H)
	mg := 3
	if tc.margin != 0 {
		mg = tc.margin
	}
	view := box.Inset(-mg).Intersect(m.Bounds())
	bg := estimateBg(m, view)
	if tc.bgAt != (image.Point{}) {
		bg = m.RGBAAt(tc.bgAt.X, tc.bgAt.Y)
	}
	fg := tc.fg
	if fg.A == 0 {
		fg = white
	}
	fg2 := tc.fg2
	if fg2.A == 0 {
		fg2 = fg
	}
	cv := newCanvas(m.Bounds().Dx(), m.Bounds().Dy(), bg)
	DrawTwoTone(cv, tc.name, tc.x, tc.y, fg, fg2)
	var peak float64
	for y := view.Min.Y; y < view.Max.Y; y++ {
		for x := view.Min.X; x < view.Max.X; x++ {
			peak = math.Max(peak, maxdiff(m.RGBAAt(x, y), bg))
		}
	}
	th := peak * 0.4
	ink := inkBox(m, view, bg, th)
	mine := inkBox(cv.img, view, bg, th)
	reg := ink.Inset(-1).Intersect(m.Bounds())
	var sum float64
	n := 0
	for y := reg.Min.Y; y < reg.Max.Y; y++ {
		for x := reg.Min.X; x < reg.Max.X; x++ {
			sum += chdiff(m.RGBAAt(x, y), cv.img.RGBAAt(x, y))
			n++
		}
	}
	e := 0.0
	if n > 0 {
		e = sum / float64(n)
	}
	k := 12
	for view.Dx()*k > 420 {
		k--
	}
	sh := image.NewRGBA(image.Rect(0, 0, view.Dx()*k*3+8, view.Dy()*k))
	draw.Draw(sh, sh.Bounds(), image.NewUniform(color.RGBA{128, 0, 128, 255}), image.Point{}, draw.Src)
	zoom(sh, m, view, 0, 0, k)
	zoom(sh, cv.img, view, view.Dx()*k+4, 0, k)
	diff := image.NewRGBA(view.Sub(view.Min))
	for y := view.Min.Y; y < view.Max.Y; y++ {
		for x := view.Min.X; x < view.Max.X; x++ {
			a, b := m.RGBAAt(x, y), cv.img.RGBAAt(x, y)
			v := uint8(math.Min(255, chdiff(a, b)*2))
			diff.SetRGBA(x-view.Min.X, y-view.Min.Y, color.RGBA{v, v, v, 255})
		}
	}
	zoom(sh, diff, diff.Bounds(), view.Dx()*k*2+8, 0, k)
	return result{tc.name, e, ink, mine, sh, box, bg, peak, softError(m, cv.img, reg)}
}

func blurAt(m *image.RGBA, x, y int) [3]float64 {
	var s [3]float64
	n := 0.0
	for j := -1; j <= 1; j++ {
		for i := -1; i <= 1; i++ {
			p := image.Pt(x+i, y+j)
			if !p.In(m.Bounds()) {
				continue
			}
			c := m.RGBAAt(p.X, p.Y)
			s[0] += float64(c.R)
			s[1] += float64(c.G)
			s[2] += float64(c.B)
			n++
		}
	}
	return [3]float64{s[0] / n, s[1] / n, s[2] / n}
}

func softError(a, b *image.RGBA, r image.Rectangle) float64 {
	var sum float64
	n := 0
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			p, q := blurAt(a, x, y), blurAt(b, x, y)
			sum += (math.Abs(p[0]-q[0]) + math.Abs(p[1]-q[1]) + math.Abs(p[2]-q[2])) / 3
			n++
		}
	}
	if n == 0 {
		return 0
	}
	return sum / float64(n)
}

func writePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

func TestIcons(t *testing.T) {
	dir := os.Getenv("PIXGO_SNAPSHOTS")
	only := os.Getenv("PIXGO_ICON")
	if dir != "" {
		os.MkdirAll(dir, 0o755)
	}
	for _, tc := range cases {
		if only != "" && only != tc.name {
			continue
		}
		tc := tc
		t.Run(tc.name+"@"+tc.file[:2], func(t *testing.T) {
			r := run(t, tc)
			dx0, dy0 := r.mine.Min.X-r.ink.Min.X, r.mine.Min.Y-r.ink.Min.Y
			dx1, dy1 := r.mine.Max.X-r.ink.Max.X, r.mine.Max.Y-r.ink.Max.Y
			t.Logf("%s@%s x=%d y=%d err %.1f soft %.1f  ink %v mine %v  d=(%d,%d,%d,%d)", tc.name, tc.file[:2], tc.x, tc.y, r.err, r.soft, r.ink, r.mine, dx0, dy0, dx1, dy1)
			if dir != "" {
				writePNG(filepath.Join(dir, tc.name+"_"+tc.file[:2]+".png"), r.sheet)
			}
			lim := tc.maxErr
			if lim == 0 {
				lim = 14
			}
			if r.err > lim {
				t.Errorf("mean error %.1f > %.1f", r.err, lim)
			}
			for _, d := range []int{dx0, dy0, dx1, dy1} {
				if d < -1 || d > 1 {
					t.Errorf("ink box off by more than 1px: mine %v ink %v", r.mine, r.ink)
					break
				}
			}
		})
	}
}

func wrapped(s func() gfx.Shape, dx, dy, sx, sy, cx, cy, grow float64) func() gfx.Shape {
	return func() gfx.Shape {
		f := grown(s(), grow)
		return func(x, y float64) bool { return f((x-cx-dx)/sx+cx, (y-cy-dy)/sy+cy) }
	}
}

func TestFit(t *testing.T) {
	name := os.Getenv("PIXGO_FIT")
	if name == "" {
		t.Skip()
	}
	for _, tc := range cases {
		if tc.name != name {
			continue
		}
		orig := defs[name]
		reset := func() {
			for i := range orig.layers {
				cache.Lock()
				delete(cache.m, name+"#"+string(rune('0'+i)))
				cache.Unlock()
			}
		}
		base := run(t, tc).err
		bestErr, best := base, [5]float64{0, 0, 1, 1, 0}
		cx, cy := float64(orig.size.W)/2, float64(orig.size.H)/2
		try := func(dx, dy, sx, sy, g float64) float64 {
			layers := make([]layer, len(orig.layers))
			for i, l := range orig.layers {
				l.shape = wrapped(l.shape, dx, dy, sx, sy, cx, cy, g)
				layers[i] = l
			}
			defs[name] = def{orig.size, layers}
			reset()
			return run(t, tc).err
		}
		for dx := -1.0; dx <= 1.01; dx += .25 {
			for dy := -1.0; dy <= 1.01; dy += .25 {
				for _, sx := range []float64{.9, .95, 1, 1.05, 1.1} {
					for _, sy := range []float64{.9, .95, 1, 1.05, 1.1} {
						if e := try(dx, dy, sx, sy, 0); e < bestErr {
							bestErr, best = e, [5]float64{dx, dy, sx, sy, 0}
						}
					}
				}
			}
		}
		bx, by, bsx, bsy := best[0], best[1], best[2], best[3]
		for _, g := range []float64{-.4, -.3, -.2, -.1, .1, .2, .3, .4, .5} {
			if e := try(bx, by, bsx, bsy, g); e < bestErr {
				bestErr, best[4] = e, g
			}
		}
		defs[name] = orig
		reset()
		t.Logf("%s@%s: base %.1f best %.1f at dx=%.2f dy=%.2f sx=%.2f sy=%.2f grow=%.1f", name, tc.file[:2], base, bestErr, best[0], best[1], best[2], best[3], best[4])
	}
}

func digits(m *image.RGBA, r image.Rectangle, bg color.RGBA, lo, hi float64) []string {
	var out []string
	for y := r.Min.Y; y < r.Max.Y; y++ {
		row := ""
		for x := r.Min.X; x < r.Max.X; x++ {
			c := m.RGBAAt(x, y)
			l := (float64(c.R)*.299 + float64(c.G)*.587 + float64(c.B)*.114)
			b := (float64(bg.R)*.299 + float64(bg.G)*.587 + float64(bg.B)*.114)
			f := (l - b) / (hi - b)
			_ = lo
			ch := " ."
			switch {
			case f < -.15:
				ch = " -"
			case f >= .05:
				d := int(f*10 + .5)
				if d > 9 {
					ch = " #"
				} else {
					ch = fmt.Sprintf(" %d", d)
				}
			}
			row += ch
		}
		out = append(out, row)
	}
	return out
}

func TestDump(t *testing.T) {
	name := os.Getenv("PIXGO_DUMP")
	if name == "" {
		t.Skip()
	}
	for _, tc := range cases {
		if tc.name != name {
			continue
		}
		m := spec(t, tc.file)
		d := defs[name]
		box := image.Rect(tc.x, tc.y, tc.x+d.size.W, tc.y+d.size.H)
		view := box.Inset(-2).Intersect(m.Bounds())
		bg := estimateBg(m, box.Inset(-3).Intersect(m.Bounds()))
		if tc.bgAt != (image.Point{}) {
			bg = m.RGBAAt(tc.bgAt.X, tc.bgAt.Y)
		}
		fg := tc.fg
		if fg.A == 0 {
			fg = white
		}
		cv := newCanvas(m.Bounds().Dx(), m.Bounds().Dy(), bg)
		DrawTwoTone(cv, name, tc.x, tc.y, fg, fg)
		hi := (float64(fg.R)*.299 + float64(fg.G)*.587 + float64(fg.B)*.114)
		a := digits(m, view, bg, 0, hi)
		b := digits(cv.img, view, bg, 0, hi)
		for i := range a {
			fmt.Println(view.Min.Y+i, a[i], "  |", b[i])
		}
	}
}

func TestSheet(t *testing.T) {
	dir := os.Getenv("PIXGO_SNAPSHOTS")
	if dir == "" {
		t.Skip()
	}
	var names []string
	for n := range defs {
		names = append(names, n)
	}
	sort.Strings(names)
	const k, cell, cols = 6, 28, 8
	rows := (len(names) + cols - 1) / cols
	bg := color.RGBA{30, 36, 44, 255}
	cv := newCanvas(cols*cell*k, rows*cell*k, bg)
	for i, n := range names {
		d := defs[n]
		small := newCanvas(cell, cell, bg)
		DrawCentered(small, n, cell/2, cell/2, white)
		_ = d
		ox, oy := (i%cols)*cell*k, (i/cols)*cell*k
		zoom(cv.img, small.img, small.img.Bounds(), ox, oy, k)
	}
	writePNG(filepath.Join(dir, "sheet.png"), cv.img)
	t.Log(strings.Join(names, " "))
}

func TestAppIcon(t *testing.T) {
	sizes := []int{16, 20, 24, 32, 48, 64, 128, 256}
	total := 0
	for _, s := range sizes {
		total += s + 8
	}
	sheet := newCanvas(total, 256, color.RGBA{44, 48, 56, 255})
	x := 0
	for _, s := range sizes {
		img := AppIcon(s)
		if img.Bounds().Dx() != s || img.Bounds().Dy() != s {
			t.Fatalf("size %d: got %v", s, img.Bounds())
		}
		if a := img.RGBAAt(0, 0).A; a != 0 {
			t.Errorf("size %d: corner alpha %d", s, a)
		}
		if c := img.RGBAAt(s/2, s/2); c.A != 255 {
			t.Errorf("size %d: centre alpha %d", s, c.A)
		}
		var blue, white, light int
		for i := 0; i < len(img.Pix); i += 4 {
			r, g, b, a := img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3]
			if a == 255 && b > 180 && r < 120 {
				blue++
			}
			if a == 255 && r > 170 && g > 170 && b > 170 {
				light++
			}
			if a == 255 && r > 235 && g > 235 && b > 235 {
				white++
			}
		}
		minWhite := s * s / 24
		if s < 32 {
			minWhite = 0
		}
		if blue < s*s/8 || white < minWhite || light < s*s/40 {
			t.Errorf("size %d: blue %d white %d light %d", s, blue, white, light)
		}
		draw.Draw(sheet.img, image.Rect(x, 0, x+s, s), img, image.Point{}, draw.Over)
		x += s + 8
	}
	if dir := os.Getenv("PIXGO_SNAPSHOTS"); dir != "" {
		os.MkdirAll(dir, 0o755)
		writePNG(filepath.Join(dir, "appicon.png"), sheet.img)
		big := newCanvas(512, 512, color.RGBA{44, 48, 56, 255})
		draw.Draw(big.img, big.img.Bounds(), AppIcon(256), image.Point{}, draw.Over)
		zoom(big.img, AppIcon(16), image.Rect(0, 0, 16, 16), 270, 0, 8)
		writePNG(filepath.Join(dir, "appicon_big.png"), big.img)
	}
}

func TestAPI(t *testing.T) {
	if Has("no-such-icon") || SizeOf("no-such-icon") != (Size{}) {
		t.Fatal("unknown icon reported as present")
	}
	cv := newCanvas(40, 40, color.RGBA{10, 10, 10, 255})
	Draw(cv, "no-such-icon", 5, 5, white)
	DrawCentered(cv, "no-such-icon", 20, 20, white)
	for _, p := range cv.img.Pix {
		if p != 10 && p != 255 {
			t.Fatal("unknown icon painted")
		}
	}
	for name, d := range defs {
		if !Has(name) || SizeOf(name) != d.size || d.size.W <= 0 || d.size.H <= 0 {
			t.Errorf("%s: bad registration %v", name, d.size)
		}
		a := newCanvas(64, 64, color.RGBA{0, 0, 0, 255})
		Draw(a, name, 10, 10, white)
		inside, outside := 0, 0
		for y := 0; y < 64; y++ {
			for x := 0; x < 64; x++ {
				if a.img.RGBAAt(x, y).R == 0 && a.img.RGBAAt(x, y).B == 0 && a.img.RGBAAt(x, y).G == 0 {
					continue
				}
				if image.Pt(x, y).In(image.Rect(10, 10, 10+d.size.W, 10+d.size.H)) {
					inside++
				} else {
					outside++
				}
			}
		}
		if inside == 0 || outside != 0 {
			t.Errorf("%s: %d pixels inside the box, %d outside", name, inside, outside)
		}
		b := newCanvas(64, 64, color.RGBA{0, 0, 0, 255})
		DrawCentered(b, name, 32, 32, white)
		want := newCanvas(64, 64, color.RGBA{0, 0, 0, 255})
		Draw(want, name, 32-d.size.W/2, 32-d.size.H/2, white)
		if string(b.img.Pix) != string(want.img.Pix) {
			t.Errorf("%s: DrawCentered differs from Draw at the centred origin", name)
		}
	}
	one := newCanvas(30, 30, color.RGBA{0, 0, 0, 255})
	two := newCanvas(30, 30, color.RGBA{0, 0, 0, 255})
	Draw(one, "exit-fullscreen", 5, 5, white)
	DrawTwoTone(two, "exit-fullscreen", 5, 5, white, color.RGBA{255, 0, 0, 255})
	if string(one.img.Pix) == string(two.img.Pix) {
		t.Error("second tone ignored")
	}
}
