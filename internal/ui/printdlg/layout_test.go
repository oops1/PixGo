package printdlg

import (
	"fmt"
	"image"
	"testing"

	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/print"
)

func inside(inner, outer image.Rectangle) bool { return !inner.Empty() && inner.In(outer) }

var contentParts = []Part{
	PartPrinter, PartSystem, PartCopies, PartCopiesDec, PartCopiesInc, PartPaper, PartOrientation, PartPlacement,
	PartWidth, PartHeight, PartCm, PartInch, PartKeep, PartMargins,
}

func shownParts(l layout) []Part {
	var out []Part
	for _, p := range contentParts {
		if !l.ctl[p].Empty() {
			out = append(out, p)
		}
	}
	return out
}

type size struct{ w, h int }

func canvases() []size {
	return []size{{0, 0}, {1200, 800}, {900, 700}, {760, 600}, {420, 320}, {396, 296}, {300, 200}, {100, 100}}
}

func TestLayoutKeepsEveryControlInsideAtEveryScaleAndCanvas(t *testing.T) {
	eachCase(t, func(t *testing.T, k float64, lang string) {
		for _, cv := range canvases() {
			for _, export := range []bool{false, true} {
				l := compute(cv.w, cv.h, export, !export)
				name := fmt.Sprintf("canvas %dx%d export=%v", cv.w, cv.h, export)
				if cv.w > 0 && cv.w < 396 {
					if l.size.X > max(cv.w, 2*l.m.padX+l.m.s(180)) {
						t.Errorf("%s: dialog %d wide", name, l.size.X)
					}
					continue
				}
				body := image.Rectangle{Max: l.size}
				content := image.Rect(0, 0, l.view.Dx(), l.contentH)
				for _, p := range shownParts(l) {
					if !inside(l.ctl[p], content) {
						t.Errorf("%s: part %d %v leaves the content %v", name, p, l.ctl[p], content)
					}
				}
				for _, p := range []Part{PartPrint, PartCancel} {
					if !inside(l.ctl[p], body) {
						t.Errorf("%s: button %d %v leaves the dialog %v", name, p, l.ctl[p], body)
					}
				}
				if !inside(l.view, body) {
					t.Errorf("%s: viewport %v leaves the dialog %v", name, l.view, body)
				}
				parts := shownParts(l)
				for i := 0; i < len(parts); i++ {
					for j := i + 1; j < len(parts); j++ {
						if l.ctl[parts[i]].Overlaps(l.ctl[parts[j]]) {
							t.Errorf("%s: %d %v overlaps %d %v", name, parts[i], l.ctl[parts[i]], parts[j], l.ctl[parts[j]])
						}
					}
				}
				for p, lr := range l.lbl {
					if lr.Empty() {
						continue
					}
					for _, q := range parts {
						if l.ctl[q].Overlaps(lr) {
							t.Errorf("%s: the label of %d overlaps %d", name, p, q)
						}
					}
				}
				if l.ctl[PartPrint].Overlaps(l.ctl[PartCancel]) || l.ctl[PartPrint].Max.X > l.ctl[PartCancel].Min.X {
					t.Errorf("%s: buttons collide", name)
				}
				if l.ctl[PartPrint].Min.Y != l.ctl[PartCancel].Min.Y {
					t.Errorf("%s: buttons are not aligned", name)
				}
				if l.preview.Empty() || l.info.Empty() {
					t.Errorf("%s: preview %v info %v", name, l.preview, l.info)
				}
				if cv.w > 0 {
					floor := 2*l.m.padX + l.m.s(180)
					if l.size.X > max(cv.w, floor) {
						t.Errorf("%s: dialog %d wide", name, l.size.X)
					}
				}
				if cv.h > 0 {
					floor := l.m.padTop + l.m.s(60) + (l.size.Y - l.view.Max.Y)
					if l.size.Y > max(cv.h, floor) {
						t.Errorf("%s: dialog %d high", name, l.size.Y)
					}
				}
				if cv.w == 0 && cv.h == 0 && l.scrollMax != 0 {
					t.Errorf("%s: an unlimited dialog must not scroll", name)
				}
				if l.scrollMax < 0 || l.view.Dy() < 1 {
					t.Errorf("%s: scrollMax %d view %v", name, l.scrollMax, l.view)
				}
			}
		}
	})
}

func TestWideLayoutPutsThePreviewBesideTheControls(t *testing.T) {
	withScale(t, 1, "en")
	l := compute(0, 0, false, true)
	if !l.wide {
		t.Fatal("an unlimited canvas must give the wide layout")
	}
	if l.preview.Min.X < l.view.Max.X || l.info.Min.X < l.view.Max.X {
		t.Errorf("preview %v info %v overlap the controls %v", l.preview, l.info, l.view)
	}
	if l.preview.Dx() < 240 || l.preview.Dy() < 200 {
		t.Errorf("the preview is only %v", l.preview)
	}
	if l.info.Min.Y < l.preview.Max.Y {
		t.Errorf("info %v overlaps the preview %v", l.info, l.preview)
	}
	narrow := compute(396, 296, false, true)
	if narrow.wide {
		t.Error("a 396 px canvas cannot hold two columns")
	}
	if narrow.scrollMax == 0 {
		t.Error("a 296 px tall canvas must scroll")
	}
}

func TestLayoutTextFitsItsBox(t *testing.T) {
	eachCase(t, func(t *testing.T, k float64, lang string) {
		for _, cv := range []size{{0, 0}, {900, 700}, {396, 296}} {
			if cv.w == 396 && k > 1.25 {
				continue
			}
			l := compute(cv.w, cv.h, false, true)
			name := fmt.Sprintf("canvas %dx%d", cv.w, cv.h)
			for p, key := range labelKeys {
				lr := l.lbl[p]
				if lr.Empty() {
					t.Errorf("%s: no label rectangle for %s", name, key)
					continue
				}
				if w := bodyFont.Measure(nil, i18n.T(key)); w > lr.Dx() {
					t.Errorf("%s: label %q is %d wide in %d", name, i18n.T(key), w, lr.Dx())
				}
			}
			for _, b := range []struct {
				part Part
				text string
			}{
				{PartPrint, i18n.T("print.print")}, {PartPrint, i18n.T("print.savePdf")},
				{PartCancel, i18n.T("dialog.cancel")},
			} {
				if w := bodyFont.Measure(nil, b.text); w+2*l.m.btnPad > l.ctl[b.part].Dx() {
					t.Errorf("%s: %q needs %d, the button has %d", name, b.text, w+2*l.m.btnPad, l.ctl[b.part].Dx())
				}
			}
			if cv.w == 0 || cv.w >= 396 {
				if w := bodyFont.Measure(nil, i18n.T("print.system")); w+2*l.m.btnPad > l.ctl[PartSystem].Dx() {
					t.Errorf("%s: the system button needs %d, has %d", name, w+2*l.m.btnPad, l.ctl[PartSystem].Dx())
				}
			}
			keep := l.m.boxSize + l.m.boxGap + bodyFont.Measure(nil, i18n.T("print.keep"))
			if keep > l.ctl[PartKeep].Dx() {
				t.Errorf("%s: the checkbox row needs %d, has %d", name, keep, l.ctl[PartKeep].Dx())
			}
			for _, u := range []struct {
				part Part
				key  string
			}{{PartCm, "print.unit.cm"}, {PartInch, "print.unit.in"}} {
				if w := bodyFont.Measure(nil, i18n.T(u.key)); w+l.m.s(8) > l.ctl[u.part].Dx() {
					t.Errorf("%s: unit %q needs %d, has %d", name, u.key, w, l.ctl[u.part].Dx())
				}
			}
		}
	})
}

func fixedItems() map[Part][]string {
	out := map[Part][]string{}
	for _, p := range print.Papers {
		out[PartPaper] = append(out[PartPaper], PaperLabel(p))
	}
	for _, o := range print.Orientations {
		out[PartOrientation] = append(out[PartOrientation], OrientationLabel(o))
	}
	for _, v := range print.Placements {
		out[PartPlacement] = append(out[PartPlacement], PlacementLabel(v))
	}
	for _, g := range print.Margins {
		out[PartMargins] = append(out[PartMargins], MarginLabel(g))
	}
	out[PartPrinter] = []string{i18n.T("print.dest.pdf")}
	return out
}

func TestDropdownValuesFitTheirFields(t *testing.T) {
	eachCase(t, func(t *testing.T, k float64, lang string) {
		for _, cv := range []size{{0, 0}, {900, 700}, {396, 296}} {
			if cv.w == 396 && k > 1.25 {
				continue
			}
			l := compute(cv.w, cv.h, false, true)
			for p, items := range fixedItems() {
				room := l.ctl[p].Dx() - l.m.s(10) - l.m.s(26)
				for _, it := range items {
					if w := bodyFont.Measure(nil, it); w > room {
						t.Errorf("canvas %dx%d: %q needs %d, the field offers %d", cv.w, cv.h, it, w, room)
					}
				}
			}
		}
	})
}

func TestInfoLinesFitTheirColumn(t *testing.T) {
	eachCase(t, func(t *testing.T, k float64, lang string) {
		for _, cv := range []size{{0, 0}, {396, 296}} {
			l := compute(cv.w, cv.h, false, true)
			room := l.info.Dx()
			texts := []string{
				i18n.T("print.info.sizeCm", "297,5", "210,5"),
				i18n.T("print.info.sizeIn", "11,69", "8,27"),
				i18n.T("print.info.dpi", 1234),
				i18n.T("print.info.dpiLow", 1234),
				i18n.T("print.info.cut"),
				i18n.T("print.info.unknownDpi", 300),
				i18n.T("print.info.badSize"),
				i18n.T("print.info.noRoom"),
			}
			for _, s := range texts {
				if w := hintFont.Measure(nil, s); w > room {
					t.Errorf("canvas %dx%d: %q is %d wide in %d", cv.w, cv.h, s, w, room)
				}
			}
		}
	})
}

func TestInfoRowsHaveRoomForThreeLines(t *testing.T) {
	withScale(t, 1.6, "ru")
	l := compute(0, 0, false, true)
	need := infoLines * (textHeight(hintFont) + l.m.lineGap)
	if l.info.Dy() < need {
		t.Fatalf("the info block is %d high, three lines need %d", l.info.Dy(), need)
	}
}

func TestDialogGrowsWithTheTextScale(t *testing.T) {
	var prev image.Point
	for _, k := range scales {
		withScale(t, k, "ru")
		got := compute(0, 0, false, true).size
		if got.X <= prev.X || got.Y <= prev.Y {
			t.Errorf("k=%.2f gives %v, not larger than %v", k, got, prev)
		}
		prev = got
	}
}

func TestScaleOneUsesTheBaseMetrics(t *testing.T) {
	withScale(t, 1, "en")
	l := compute(0, 0, false, true)
	if l.m.padX != 18 || l.m.fieldH != 32 || l.m.btnMinW != 88 || l.m.btnMinH != 30 || l.m.boxSize != 16 {
		t.Fatalf("base metrics changed: %+v", l.m)
	}
	if l.size.X < 700 || l.size.Y < 420 || l.size.Y > 620 {
		t.Fatalf("the dialog is %v", l.size)
	}
}

func TestExportLayoutHasNoPrinterOrCopiesRows(t *testing.T) {
	withScale(t, 1, "en")
	full := compute(0, 0, false, true)
	export := compute(0, 0, true, false)
	for _, p := range []Part{PartPrinter, PartSystem, PartCopies, PartCopiesDec, PartCopiesInc} {
		if !export.ctl[p].Empty() {
			t.Errorf("export shows part %d", p)
		}
		if full.ctl[p].Empty() {
			t.Errorf("the print dialog lacks part %d", p)
		}
	}
	if export.size.Y >= full.size.Y {
		t.Errorf("export %v is not shorter than print %v", export.size, full.size)
	}
	nosys := compute(0, 0, false, false)
	if !nosys.ctl[PartSystem].Empty() || nosys.ctl[PartPrinter].Empty() {
		t.Error("without the system dialog only that button disappears")
	}
}

func TestGapSqueezeNeverShrinksControls(t *testing.T) {
	withScale(t, 1, "en")
	roomy := compute(0, 0, false, true)
	tight := compute(0, 560, false, true)
	if tight.ctl[PartPaper].Dy() != roomy.ctl[PartPaper].Dy() {
		t.Errorf("a field was squeezed from %d to %d", roomy.ctl[PartPaper].Dy(), tight.ctl[PartPaper].Dy())
	}
	if tight.size.Y > 560 {
		t.Errorf("%v", tight.size)
	}
}
