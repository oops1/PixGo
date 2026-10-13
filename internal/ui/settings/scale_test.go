package settings

import (
	"fmt"
	"image"
	"math"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/fonts"
	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/testutil"
	"github.com/oops1/PixGo/internal/ui/theme"
)

var scales = []float64{1.2, 1.33, 1.6}

var scaleLangs = []string{"ru", "en"}

func withScale(t testing.TB, k float64) {
	t.Helper()
	old := fonts.TextScale()
	fonts.SetTextScale(k)
	t.Cleanup(func() {
		fonts.SetTextScale(old)
		i18n.SetLang("ru")
		syncDims()
	})
}

func scaledSize(k float64, w, h int) image.Point {
	return image.Pt(int(math.Round(float64(w)*k)), int(math.Round(float64(h)*k)))
}

func eachScale(t *testing.T, f func(t *testing.T, k float64, lang string)) {
	for _, k := range scales {
		for _, lang := range scaleLangs {
			t.Run(fmt.Sprintf("k%.2f/%s", k, lang), func(t *testing.T) {
				withScale(t, k)
				f(t, k, lang)
			})
		}
	}
}

func newScaledRig(t testing.TB, lang string, w, h int) *rig {
	t.Helper()
	r := newRig(t, w, h)
	i18n.SetLang(lang)
	r.page.Refresh()
	return r
}

func TestScaleOneKeepsTheMockMetrics(t *testing.T) {
	for _, lang := range scaleLangs {
		r := newScaledRig(t, lang, mockW, mockH)
		_ = r
		want := mock
		want.navCenters = dim.navCenters
		if !reflect.DeepEqual(dim, want) {
			t.Fatalf("%s: at scale 1 the metrics must be exactly the mockup ones:\n got %+v\nwant %+v", lang, dim, want)
		}
		for s := Section(0); s < sectionCount; s++ {
			if got, want := dim.navCenters[s], float64(mock.navCapTops[s])+3.65; got != want {
				t.Errorf("%s: row %d centre %v, want %v", lang, s, got, want)
			}
		}
	}
}

func TestSoftScaleFollowsHalfOfTheExcess(t *testing.T) {
	withScale(t, 1.6)
	newScaledRig(t, "ru", mockW, mockH)
	if math.Abs(dim.ks-1.3) > 1e-9 {
		t.Fatalf("ks = %v, want 1.3", dim.ks)
	}
	if math.Abs(dim.checkBoxSize-mock.checkBoxSize*1.3) > 1e-9 || math.Abs(dim.sliderThumb-14*1.3) > 1e-9 {
		t.Fatalf("control geometry must follow the soft scale: box %v thumb %v", dim.checkBoxSize, dim.sliderThumb)
	}
	if dim.rowHeight != int(math.Round(22*1.6)) || dim.dropdownHeight != int(math.Round(25*1.6)) || dim.buttonHeight != int(math.Round(25*1.6)) {
		t.Fatalf("rows must follow the full scale: row %d dropdown %d button %d", dim.rowHeight, dim.dropdownHeight, dim.buttonHeight)
	}
	if dim.headerBase <= mock.headerBase {
		t.Fatalf("the header baseline follows the bigger text: %d", dim.headerBase)
	}
}

func TestNavigationLabelsFitAtEveryScale(t *testing.T) {
	eachScale(t, func(t *testing.T, k float64, lang string) {
		size := scaledSize(k, 420, 320)
		r := newScaledRig(t, lang, size.X, size.Y)
		for s := Section(0); s < sectionCount; s++ {
			label := i18n.T(sectionKeys[s])
			if w := measure(navFont, label); w > dim.navTextWidth {
				t.Errorf("%q is %d wide, the column offers %d", label, w, dim.navTextWidth)
			}
			if got := navFont.Fit(nil, label, dim.navTextWidth); got != label {
				t.Errorf("%q would be cut to %q", label, got)
			}
			row := navRowRect(s)
			if row.Max.X > dim.navW || row.Min.Y < contentTop || row.Max.Y > size.Y {
				t.Errorf("row %d %v does not fit the navigation %d x %d", s, row, dim.navW, size.Y)
			}
		}
		if dim.navW > size.X*55/100 {
			t.Errorf("the navigation takes %d of %d px", dim.navW, size.X)
		}
		if r.page.geo.nav.Max.X != dim.navW {
			t.Errorf("nav %v", r.page.geo.nav)
		}
	})
}

func hasEllipsis(s string) bool { return strings.HasSuffix(s, "…") }

func TestNothingIsCutShortAtEveryScale(t *testing.T) {
	eachScale(t, func(t *testing.T, k float64, lang string) {
		for _, size := range []image.Point{scaledSize(k, 420, 320), scaledSize(k, 457, 430), {1100, 800}} {
			r := newScaledRig(t, lang, size.X, size.Y)
			for s := Section(0); s < sectionCount; s++ {
				r.page.SetSection(s)
				for i := range r.page.items {
					it := &r.page.items[i]
					switch it.kind {
					case kindTitle, kindHeading, kindNote, kindValue, kindCheck, kindRadio, kindButton:
						for _, l := range it.lines {
							if hasEllipsis(l) {
								t.Errorf("%v/%s/%s: line %q is cut", size, sectionNames[s], it.id, l)
							}
						}
					case kindDropdown, kindFormat:
						if hasEllipsis(it.text) || hasEllipsis(it.name) {
							t.Errorf("%v/%s/%s: %q %q is cut", size, sectionNames[s], it.id, it.text, it.name)
						}
					case kindLink:
						if hasEllipsis(it.text) {
							t.Errorf("%v/%s/%s: %q is cut", size, sectionNames[s], it.id, it.text)
						}
					case kindCard:
						font := smallCapFont
						if isTheme(it.pic) {
							font = captionFont
						}
						if w := measure(font, it.text); w > it.r.Dx() {
							t.Errorf("%v/%s/%s: label %q is %d wide in a %d wide item", size, sectionNames[s], it.id, it.text, w, it.r.Dx())
						}
					}
				}
			}
		}
	})
}

func TestLayoutInvariantsAtEveryScale(t *testing.T) {
	eachScale(t, func(t *testing.T, k float64, lang string) {
		for _, size := range []image.Point{scaledSize(k, 420, 320), scaledSize(k, 457, 430), {1100, 800}} {
			r := newScaledRig(t, lang, size.X, size.Y)
			for s := Section(0); s < sectionCount; s++ {
				r.page.SetSection(s)
				assertLayout(t, r, fmt.Sprintf("%v/%s", size, sectionNames[s]))
			}
		}
	})
}

func assertLayout(t *testing.T, r *rig, name string) {
	t.Helper()
	g := r.page.geo
	prevTop := 0
	for i := range r.page.items {
		it := &r.page.items[i]
		if it.r.Min.X < g.x0 || it.r.Max.X > g.xt {
			t.Errorf("%s: %s %q: rect %v leaves the column %d..%d", name, it.id, it.text, it.r, g.x0, g.xt)
		}
		if it.r.Min.Y < prevTop {
			t.Errorf("%s: %s: items must run top to bottom", name, it.id)
		}
		prevTop = it.r.Min.Y
		for j := 0; j < i; j++ {
			o := &r.page.items[j]
			if it.r.Overlaps(o.r) && !(it.kind == kindCard && o.kind == kindCard && it.r.Min.X >= o.r.Max.X) {
				t.Errorf("%s: %s overlaps %s: %v %v", name, it.id, o.id, it.r, o.r)
			}
		}
		switch it.kind {
		case kindCheck, kindRadio:
			for _, l := range it.lines {
				if w := measure(bodyFont, l); w > g.xt-g.x0-dim.checkLabelX {
					t.Errorf("%s: %s line %q is %d wide", name, it.id, l, w)
				}
			}
		case kindTitle:
			for _, l := range it.lines {
				if w := measure(titleFont, l); w > g.x1-g.x0 {
					t.Errorf("%s: title line %q is %d wide", name, l, w)
				}
			}
		case kindHeading:
			for _, l := range it.lines {
				if w := measure(headingFont, l); it.name != "big" && w > g.x1-g.x0 {
					t.Errorf("%s: heading line %q is %d wide", name, l, w)
				}
			}
		case kindSlider:
			x0, x1, _ := sliderSpan(it)
			if x1-x0 < dim.sliderMinLen || x1 > it.r.Max.X {
				t.Errorf("%s: slider %s track %d..%d in %v", name, it.id, x0, x1, it.r)
			}
			if !it.stacked && x1+dim.sliderInset+dim.sliderLabelGap+it.labelW > it.r.Max.X {
				t.Errorf("%s: slider %s label leaves its row", name, it.id)
			}
		case kindButton:
			for _, l := range it.lines {
				if w := measure(bodyFont, l); w > it.r.Dx()-2*dim.buttonPadding {
					t.Errorf("%s: button line %q is %d wide in %v", name, l, w, it.r)
				}
			}
		}
	}
}

func TestNothingIsDrawnOutsideTheColumnAtEveryScale(t *testing.T) {
	eachScale(t, func(t *testing.T, k float64, lang string) {
		for _, size := range []image.Point{scaledSize(k, 420, 320), scaledSize(k, 457, 430)} {
			for name, mk := range map[string]func() *theme.Theme{"dark": theme.Dark, "light": theme.Light} {
				r := newScaledRig(t, lang, size.X, size.Y)
				r.th = mk()
				for s := Section(0); s < sectionCount; s++ {
					r.page.SetSection(s)
					r.tab(4)
					img := r.render()
					g := r.page.geo
					bg := r.th.Page
					for y := g.view.Min.Y + 1; y < g.view.Max.Y; y++ {
						for x := g.xt + 3; x < g.view.Max.X-dim.barWide-barRightInset-2; x++ {
							if c := img.RGBAAt(x, y); c != bg {
								t.Fatalf("%s/%v/%s: pixel (%d,%d) = %v right of the column", name, size, sectionNames[s], x, y, c)
							}
						}
						for x := g.view.Min.X + 1; x < g.x0-6; x++ {
							if c := img.RGBAAt(x, y); c != bg {
								t.Fatalf("%s/%v/%s: pixel (%d,%d) = %v left of the column", name, size, sectionNames[s], x, y, c)
							}
						}
					}
				}
			}
		}
	})
}

func TestNavigationDoesNotLeakIntoTheContentAtEveryScale(t *testing.T) {
	eachScale(t, func(t *testing.T, k float64, lang string) {
		size := scaledSize(k, 420, 320)
		r := newScaledRig(t, lang, size.X, size.Y)
		for s := Section(0); s < sectionCount; s++ {
			r.page.SetSection(s)
			img := r.render()
			for y := contentTop; y < size.Y; y++ {
				for x := dim.navW + 1; x < dim.navW+4; x++ {
					if c := img.RGBAAt(x, y); c != r.th.Page {
						t.Fatalf("section %d: pixel (%d,%d) = %v just right of the divider", s, x, y, c)
					}
				}
			}
		}
	})
}

func TestHitTestingScrollingAndFocusWorkAtEveryScale(t *testing.T) {
	eachScale(t, func(t *testing.T, k float64, lang string) {
		size := scaledSize(k, 457, 430)
		r := newScaledRig(t, lang, size.X, size.Y)
		for s := Section(0); s < sectionCount; s++ {
			r.page.SetSection(s)
			row := navRowRect(s)
			r.page.SetSection((s + 1) % sectionCount)
			r.click((row.Min.X+row.Max.X)/2, (row.Min.Y+row.Max.Y)/2)
			if r.page.Section() != s {
				t.Fatalf("clicking the navigation row %d selected %d", s, r.page.Section())
			}
			for i := range r.page.items {
				it := &r.page.items[i]
				if !it.interactive() {
					continue
				}
				r.page.reveal(it.r)
				h := it.hitRect().Add(image.Pt(0, -r.page.scroll))
				p := image.Pt((h.Min.X+h.Max.X)/2, (h.Min.Y+h.Max.Y)/2)
				if !p.In(r.page.geo.view) {
					t.Errorf("%s/%s: centre %v is outside the view %v", sectionNames[s], it.id, p, r.page.geo.view)
					continue
				}
				got := r.page.hitAt(p.X, p.Y)
				if got.kind != hitItem || r.page.items[got.index].id != it.id {
					t.Errorf("%s/%s: hit at %v = %+v", sectionNames[s], it.id, p, got)
				}
			}
			r.page.setScroll(0)
			if max := r.page.maxScroll(); max > 0 {
				r.page.setScroll(max)
				last := r.page.items[len(r.page.items)-1]
				if bottom := last.r.Max.Y - r.page.scroll; bottom > r.page.geo.view.Max.Y {
					t.Errorf("%s: the last item ends at %d below the view %d at the end of the scroll", sectionNames[s], bottom, r.page.geo.view.Max.Y)
				}
				_, thumb, ok := r.page.barRects()
				if !ok || thumb.Dx() != dim.barThin {
					t.Errorf("%s: bar %v ok %v, thin width must be %d", sectionNames[s], thumb, ok, dim.barThin)
				}
			}
			r.page.setScroll(0)
			r.page.focus, r.page.keyNav = "", false
			for range r.page.stops() {
				r.key(widget.KeyTab, 0)
				if i := r.page.indexOf(r.page.focus); i >= 0 {
					it := &r.page.items[i]
					top, bottom := it.r.Min.Y-r.page.scroll, it.r.Max.Y-r.page.scroll
					if it.r.Dy() <= r.page.geo.view.Dy() && (top < r.page.geo.view.Min.Y || bottom > r.page.geo.view.Max.Y) {
						t.Errorf("%s/%s: focused control %d..%d is outside the view %v", sectionNames[s], it.id, top, bottom, r.page.geo.view)
					}
				}
			}
		}
	})
}

func TestSlidersStillWorkWhenTheirLabelIsStacked(t *testing.T) {
	withScale(t, 1.6)
	r := newScaledRig(t, "ru", 420, 600)
	r.page.SetSection(SectionPerformance)
	stacked := false
	for i := range r.page.items {
		if it := &r.page.items[i]; it.kind == kindSlider && it.stacked {
			stacked = true
			if it.r.Dy() <= dim.rowHeight {
				t.Errorf("%s: a stacked slider needs room for its label", it.id)
			}
			x0, x1, cy := sliderSpan(it)
			r.page.reveal(it.r)
			r.click(x1, cy-r.page.scroll)
			if got := r.page.itemByID(it.id).value; got != it.hi {
				t.Errorf("%s: click at the right end gave %d, want %d", it.id, got, it.hi)
			}
			if x1-x0 < dim.sliderMinLen {
				t.Errorf("%s: track %d..%d is too short", it.id, x0, x1)
			}
		}
	}
	if !stacked {
		t.Skip("no slider needed the stacked layout in this window")
	}
}

func TestFormatNameIsKeptClearOfTheSwitch(t *testing.T) {
	withScale(t, 1.6)
	r := newScaledRig(t, "en", 560, 600)
	r.page.SetSection(SectionFormats)
	it := r.page.itemByID("format:avif")
	nameW := measure(bodyFont, it.name)
	if it.r.Min.X+nameW > it.box.Min.X {
		t.Fatalf("name ends at %d, the switch starts at %d", it.r.Min.X+nameW, it.box.Min.X)
	}
}

func TestSnapshotsAtScale(t *testing.T) {
	out := testutil.OutDir()
	if out == "" {
		t.Skip("PIXGO_SNAPSHOTS is not set")
	}
	for _, k := range []float64{1.2, 1.6} {
		t.Run(fmt.Sprintf("k%.2f", k), func(t *testing.T) {
			withScale(t, k)
			tag := fmt.Sprintf("k%02d", int(math.Round(k*10)))
			for s := Section(0); s < sectionCount; s++ {
				r := newRig(t, mockW, mockH)
				r.page.SetSection(s)
				testutil.SavePNG(filepath.Join(out, "settings_"+tag+"_"+sectionNames[s]+".png"), r.render())
			}
			size := scaledSize(k, 420, 320)
			r := newRig(t, size.X, size.Y)
			testutil.SavePNG(filepath.Join(out, "settings_"+tag+"_min.png"), r.render())
			r.page.SetSection(SectionPerformance)
			testutil.SavePNG(filepath.Join(out, "settings_"+tag+"_min_performance.png"), r.render())
			r = newRig(t, 900, 700)
			r.tab(3)
			testutil.SavePNG(filepath.Join(out, "settings_"+tag+"_wide.png"), r.render())
		})
	}
}
