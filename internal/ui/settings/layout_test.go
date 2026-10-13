package settings

import (
	"fmt"
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/ui/theme"
)

var layoutSizes = []image.Point{{420, 320}, {457, 430}, {520, 480}, {900, 600}, {1600, 900}}

var layoutThemes = map[string]func() *theme.Theme{
	"dark": theme.Dark, "light": theme.Light, "contrast": theme.HighContrast,
}

func TestItemsStayInsideTheColumnAtEverySize(t *testing.T) {
	for _, size := range layoutSizes {
		for s := Section(0); s < sectionCount; s++ {
			name := fmt.Sprintf("%dx%d/%s", size.X, size.Y, sectionNames[s])
			t.Run(name, func(t *testing.T) {
				r := newRig(t, size.X, size.Y)
				r.page.SetSection(s)
				g := r.page.geo
				prevTop := 0
				for i := range r.page.items {
					it := &r.page.items[i]
					if it.r.Min.X < g.x0 || it.r.Max.X > g.xt {
						t.Errorf("%s %q: rect %v leaves the column %d..%d", it.id, it.text, it.r, g.x0, g.xt)
					}
					if it.r.Min.Y < prevTop {
						t.Errorf("%s: items must run top to bottom, %d after %d", it.id, it.r.Min.Y, prevTop)
					}
					prevTop = it.r.Min.Y
					for j := 0; j < i; j++ {
						if o := &r.page.items[j]; it.r.Overlaps(o.r) && !(it.kind == kindCard && o.kind == kindCard && it.r.Min.X >= o.r.Max.X) {
							t.Errorf("%s overlaps %s: %v %v", it.id, o.id, it.r, o.r)
						}
					}
				}
			})
		}
	}
}

func TestEveryTextFitsItsBox(t *testing.T) {
	for _, size := range layoutSizes {
		for s := Section(0); s < sectionCount; s++ {
			name := fmt.Sprintf("%dx%d/%s", size.X, size.Y, sectionNames[s])
			t.Run(name, func(t *testing.T) {
				r := newRig(t, size.X, size.Y)
				r.page.SetSection(s)
				g := r.page.geo
				for i := range r.page.items {
					it := &r.page.items[i]
					switch it.kind {
					case kindCheck, kindRadio:
						for _, l := range it.lines {
							if w := measure(bodyFont, l); w > g.xt-g.x0-dim.checkLabelX {
								t.Errorf("%s: line %q is %d wide, %d available", it.id, l, w, g.xt-g.x0-dim.checkLabelX)
							}
						}
					case kindNote:
						for _, l := range it.lines {
							if w := measure(noteFont, l); w > g.xt-g.x0-it.indent {
								t.Errorf("note line %q is %d wide", l, w)
							}
						}
					case kindValue:
						for _, l := range it.lines {
							if w := measure(bodyFont, l); w > g.xt-g.x0 {
								t.Errorf("value line %q is %d wide", l, w)
							}
						}
					case kindTitle:
						for _, l := range it.lines {
							if w := measure(titleFont, l); w > g.x1-g.x0 {
								t.Errorf("title line %q is %d wide", l, w)
							}
						}
					case kindHeading:
						for _, l := range it.lines {
							if w := measure(headingFont, l); it.name != "big" && w > g.x1-g.x0 {
								t.Errorf("heading line %q is %d wide", l, w)
							}
						}
					case kindDropdown:
						if w := measure(bodyFont, it.text); w > it.r.Dx()-dim.dropdownTextX-chevronFromR-6 {
							t.Errorf("dropdown text %q is %d wide", it.text, w)
						}
					case kindButton:
						for _, l := range it.lines {
							if w := measure(bodyFont, l); w > it.r.Dx()-2*dim.buttonPadding {
								t.Errorf("button line %q is %d wide in %v", l, w, it.r)
							}
						}
					}
				}
			})
		}
	}
}

func TestNothingIsDrawnOutsideTheColumn(t *testing.T) {
	for name, mk := range layoutThemes {
		for _, size := range []image.Point{{420, 320}, {457, 430}, {900, 600}} {
			for s := Section(0); s < sectionCount; s++ {
				t.Run(fmt.Sprintf("%s/%dx%d/%s", name, size.X, size.Y, sectionNames[s]), func(t *testing.T) {
					r := newRig(t, size.X, size.Y)
					r.th = mk()
					r.page.SetSection(s)
					img := r.render()
					g := r.page.geo
					bg := r.th.Page
					drawn := 0
					for y := g.view.Min.Y + 1; y < g.view.Max.Y; y++ {
						for x := g.x0; x < g.x1; x++ {
							if img.RGBAAt(x, y) != bg {
								drawn++
							}
						}
					}
					if drawn < 200 {
						t.Fatalf("only %d pixels drawn in the column, the check would be vacuous", drawn)
					}
					for y := g.view.Min.Y + 1; y < g.view.Max.Y; y++ {
						for x := g.xt + 3; x < g.view.Max.X-dim.barWide-barRightInset-2; x++ {
							if c := img.RGBAAt(x, y); c != bg {
								t.Fatalf("pixel (%d,%d) = %v right of the column, want the page colour %v", x, y, c, bg)
							}
						}
						for x := g.view.Min.X + 1; x < g.x0-6; x++ {
							if c := img.RGBAAt(x, y); c != bg {
								t.Fatalf("pixel (%d,%d) = %v left of the column, want the page colour %v", x, y, c, bg)
							}
						}
					}
				})
			}
		}
	}
}

func TestNarrowWindowWrapsCardsAndLabels(t *testing.T) {
	r := newRig(t, 420, 320)
	light, dark := r.item("theme:light"), r.item("theme:dark")
	if dark.r.Min.Y <= light.r.Min.Y {
		t.Fatalf("the third theme card must wrap to a new row: light %v dark %v", light.r, dark.r)
	}
	if it := r.item("restoreLastImage"); len(it.lines) < 2 {
		t.Fatalf("the long label must wrap in a narrow window, got %v", it.lines)
	}
	wide := newRig(t, 900, 600)
	if wide.item("theme:dark").r.Min.Y != wide.item("theme:light").r.Min.Y {
		t.Fatal("cards share a row in a wide window")
	}
	if len(wide.item("restoreLastImage").lines) != 1 {
		t.Fatal("no wrapping in a wide window")
	}
}

func TestNavigationKeepsItsPlaceInTheMinimumWindow(t *testing.T) {
	r := newRig(t, 420, 320)
	for s := Section(0); s < sectionCount; s++ {
		row := navRowRect(s)
		if row.Max.Y > 320 || row.Max.X > r.page.geo.nav.Max.X {
			t.Errorf("row %d at %v does not fit", s, row)
		}
	}
	if r.page.geo.nav.Max.X != dim.navW {
		t.Fatalf("the navigation is %d wide, want %d", r.page.geo.nav.Max.X, dim.navW)
	}
}

func TestAnchorsFollowTheWindowOriginInsideTheRoot(t *testing.T) {
	r := newRig(t, mockW, mockH)
	r.page.SetBounds(image.Rect(1, 1, mockW-1, mockH-1))
	g := r.page.geo
	if g.win != image.Rect(0, 0, mockW, mockH) {
		t.Fatalf("window = %v", g.win)
	}
	if g.x1 != mockW-columnGap {
		t.Fatalf("the column ends at %d, want %d", g.x1, mockW-columnGap)
	}
	if g.view.Max.X != mockW-1 || g.view.Max.Y != mockH-1 {
		t.Fatalf("view = %v", g.view)
	}
}

func TestColumnWidthIsCappedInVeryWideWindows(t *testing.T) {
	r := newRig(t, 1600, 900)
	if w := r.page.geo.x1 - r.page.geo.x0; w != dim.columnMax {
		t.Fatalf("column width %d, want %d", w, dim.columnMax)
	}
}

func TestTinyWindowsDoNotPanic(t *testing.T) {
	for _, size := range []image.Point{{10, 10}, {160, 40}, {300, 100}} {
		for s := Section(0); s < sectionCount; s++ {
			r := newRig(t, size.X, size.Y)
			r.page.SetSection(s)
			r.render()
			r.key(widget.KeyTab, 0)
			r.click(size.X/2, size.Y/2)
		}
	}
}

func TestRenderingWithoutASourceDrawsNothing(t *testing.T) {
	r := newRig(t, mockW, mockH)
	r.page.deps.Source = nil
	r.render()
	if r.page.HandleKey(widget.KeyEvent{Code: widget.KeyEscape, Pressed: true}) {
		t.Fatal("a page without a source consumes no keys")
	}
	if len(r.page.AccessChildren()) != 0 {
		t.Fatal("no accessibility children without a source")
	}
}
