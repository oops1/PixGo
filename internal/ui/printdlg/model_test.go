package printdlg

import (
	"image"
	"math"
	"testing"

	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/print"
)

func near(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

func newModel(t *testing.T, mutate func(*Env, *Values)) *Model {
	t.Helper()
	i18n.SetLang("en")
	env, v := testEnv(), testValues()
	if mutate != nil {
		mutate(&env, &v)
	}
	return NewModel(env, v)
}

func TestDefaultPrinterIsPreselected(t *testing.T) {
	m := newModel(t, func(e *Env, v *Values) { v.Printer = "" })
	if m.PDF() || m.Printer() != "Photo Inkjet" {
		t.Fatalf("pdf=%v printer=%q", m.PDF(), m.Printer())
	}
	m = newModel(t, func(e *Env, v *Values) { v.Printer = "Gone" })
	if m.Printer() != "Photo Inkjet" {
		t.Fatalf("an unknown remembered printer must fall back to the default, got %q", m.Printer())
	}
	m = newModel(t, func(e *Env, v *Values) { v.Printer = "Office Laser" })
	if m.Printer() != "Office Laser" {
		t.Fatalf("the remembered printer is lost: %q", m.Printer())
	}
}

func TestWithoutADefaultTheFirstPrinterIsUsed(t *testing.T) {
	m := newModel(t, func(e *Env, v *Values) {
		e.Printers = []Printer{{Name: "A"}, {Name: "B"}}
		v.Printer = ""
	})
	if m.Printer() != "A" {
		t.Fatalf("%q", m.Printer())
	}
}

func TestNoPrintersMeansPDF(t *testing.T) {
	m := newModel(t, func(e *Env, v *Values) { e.Printers = nil })
	if !m.PDF() || m.Values().Printer != "" {
		t.Fatalf("%+v", m.Values())
	}
	if d := m.Destinations(); len(d) != 1 || d[0] != i18n.T("print.dest.pdf") {
		t.Fatalf("%v", d)
	}
}

func TestExportModeIsAlwaysPDF(t *testing.T) {
	m := newModel(t, func(e *Env, v *Values) { e.Export = true })
	if !m.PDF() || m.Values().Copies != 1 {
		t.Fatalf("%+v", m.Values())
	}
}

func TestDestinationsListPrintersThenPDF(t *testing.T) {
	m := newModel(t, nil)
	d := m.Destinations()
	if len(d) != 4 || d[0] != "Office Laser" || d[3] != i18n.T("print.dest.pdf") {
		t.Fatalf("%v", d)
	}
	if m.DestIndex() != 1 {
		t.Fatalf("index %d", m.DestIndex())
	}
	m.SetDestIndex(3)
	if !m.PDF() || m.DestIndex() != 3 {
		t.Fatal("PDF not selected")
	}
	m.SetDestIndex(0)
	if m.PDF() || m.Printer() != "Office Laser" {
		t.Fatal("printer not selected")
	}
	m.SetDestIndex(9)
	m.SetDestIndex(-1)
	if m.Printer() != "Office Laser" {
		t.Fatal("out of range choice changed the destination")
	}
	m.SetPrinter("Nope")
	if m.Printer() != "Office Laser" {
		t.Fatal("an unknown printer was accepted")
	}
}

func TestPDFValuesDropPrinterAndCopies(t *testing.T) {
	m := newModel(t, nil)
	m.SetCopies(5)
	m.SetDestIndex(3)
	v := m.Values()
	if !v.PDF || v.Printer != "" || v.Copies != 1 {
		t.Fatalf("%+v", v)
	}
}

func TestCopiesAreClampedAndParsed(t *testing.T) {
	m := newModel(t, nil)
	for _, c := range []struct {
		in     string
		text   string
		copies int
		valid  bool
	}{
		{"3", "3", 3, true},
		{"12", "12", 12, true},
		{"123", "12", 12, true},
		{"", "", 12, false},
		{"0", "0", 12, false},
		{"a7b", "7", 7, true},
	} {
		m.SetCopiesText(c.in)
		if m.CopiesText() != c.text || m.Copies() != c.copies || m.CopiesValid() != c.valid {
			t.Errorf("%q: text %q copies %d valid %v", c.in, m.CopiesText(), m.Copies(), m.CopiesValid())
		}
	}
	m.SetCopies(500)
	if m.Copies() != MaxCopies || m.CopiesText() != "99" {
		t.Errorf("max: %d %q", m.Copies(), m.CopiesText())
	}
	m.SetCopies(-4)
	if m.Copies() != MinCopies {
		t.Errorf("min: %d", m.Copies())
	}
}

func TestInvalidCopiesBlockPrintingToAPrinterButNotPDF(t *testing.T) {
	m := newModel(t, nil)
	m.SetCopiesText("")
	if m.Valid() {
		t.Fatal("empty copies accepted")
	}
	m.SetDestIndex(3)
	if !m.Valid() {
		t.Fatal("PDF does not need copies")
	}
}

func TestPlacementCustomStartsFromTheFitSize(t *testing.T) {
	m := newModel(t, nil)
	m.SetPlacement(print.PlaceCustom)
	fw, fh := print.DefaultOptions().FitSizeMM(m.Picture())
	o := m.Options()
	if !near(o.CustomW, fw, 0.01) || !near(o.CustomH, fh, 0.01) {
		t.Fatalf("custom %.2f x %.2f, fit %.2f x %.2f", o.CustomW, o.CustomH, fw, fh)
	}
	if m.WidthText() != "26.4" || m.HeightText() != "19.8" {
		t.Fatalf("texts %q %q", m.WidthText(), m.HeightText())
	}
	if !m.Valid() {
		t.Fatal("the initial custom size must be printable")
	}
}

func TestKeepProportionsTiesHeightToWidth(t *testing.T) {
	m := newModel(t, nil)
	m.SetPlacement(print.PlaceCustom)
	m.SetWidthText("20")
	if m.HeightText() != "15" || !near(m.Options().CustomH, 150, 1e-6) {
		t.Fatalf("height %q %.3f", m.HeightText(), m.Options().CustomH)
	}
	m.SetHeightText("9")
	if m.WidthText() != "12" || !near(m.Options().CustomW, 120, 1e-6) {
		t.Fatalf("width %q %.3f", m.WidthText(), m.Options().CustomW)
	}
	m.SetKeep(false)
	m.SetWidthText("30")
	if m.HeightText() != "9" {
		t.Fatalf("without keep the height must stay: %q", m.HeightText())
	}
	m.SetKeep(true)
	if m.HeightText() != "22.5" {
		t.Fatalf("keep must re-derive the height from the width: %q", m.HeightText())
	}
}

func TestUnitSwitchConvertsTheTexts(t *testing.T) {
	m := newModel(t, nil)
	m.SetPlacement(print.PlaceCustom)
	m.SetWidthText("25.4")
	m.SetUnit(print.UnitInch)
	if m.WidthText() != "10" {
		t.Fatalf("25.4 cm is 10 in, got %q", m.WidthText())
	}
	if !near(m.Options().CustomW, 254, 1e-6) {
		t.Fatalf("mm %v", m.Options().CustomW)
	}
	m.SetWidthText("2")
	if !near(m.Options().CustomW, 50.8, 1e-6) {
		t.Fatalf("2 in is 50.8 mm, got %v", m.Options().CustomW)
	}
	m.SetUnit(print.UnitCM)
	if m.WidthText() != "5.08" {
		t.Fatalf("%q", m.WidthText())
	}
}

func TestCustomSizeValidation(t *testing.T) {
	m := newModel(t, nil)
	m.SetPlacement(print.PlaceCustom)
	m.SetKeep(false)
	m.SetWidthText("")
	if m.Valid() || m.SizeValid(true) {
		t.Fatal("an empty width is not valid")
	}
	m.SetWidthText("0")
	if m.SizeValid(true) {
		t.Fatal("zero width")
	}
	m.SetWidthText("0.05")
	if m.SizeValid(true) {
		t.Fatal("0.5 mm is below the minimum")
	}
	m.SetWidthText("201")
	if m.SizeValid(true) {
		t.Fatal("2.01 m is above the maximum")
	}
	m.SetWidthText("10")
	if !m.SizeValid(true) || !m.Valid() {
		t.Fatal("10 cm must be fine")
	}
	m.SetPlacement(print.PlaceFit)
	m.SetWidthText("")
	if !m.Valid() {
		t.Fatal("the custom fields do not matter in another placement")
	}
}

func TestCleanDecimal(t *testing.T) {
	for in, want := range map[string]string{
		"12":         "12",
		"12.5":       "12.5",
		"12,5":       "12,5",
		"1.2.3":      "1.23",
		"abc":        "",
		"12.345":     "12.34",
		".5":         ".5",
		"1234567890": "1234567",
		"-3":         "3",
		"4 5":        "45",
	} {
		if got := CleanDecimal(in); got != want {
			t.Errorf("%q -> %q, want %q", in, got, want)
		}
	}
}

func TestParseSize(t *testing.T) {
	for _, c := range []struct {
		in   string
		unit print.Unit
		mm   float64
		ok   bool
	}{
		{"10", print.UnitCM, 100, true},
		{"10,5", print.UnitCM, 105, true},
		{"1", print.UnitInch, 25.4, true},
		{"", print.UnitCM, 0, false},
		{".", print.UnitCM, 0, false},
		{"0", print.UnitCM, 0, false},
		{"1e3", print.UnitCM, 0, false},
	} {
		got, ok := ParseSize(c.in, c.unit)
		if ok != c.ok || (ok && !near(got, c.mm, 1e-9)) {
			t.Errorf("%q: %v %v, want %v %v", c.in, got, ok, c.mm, c.ok)
		}
	}
}

func TestFormatSizeUsesTheLocaleDecimal(t *testing.T) {
	i18n.SetLang("en")
	if got := formatSize(264, print.UnitCM); got != "26.4" {
		t.Errorf("en: %q", got)
	}
	i18n.SetLang("ru")
	defer i18n.SetLang("en")
	if got := formatSize(264, print.UnitCM); got != "26,4" {
		t.Errorf("ru: %q", got)
	}
	if got := formatSize(210, print.UnitCM); got != "21" {
		t.Errorf("trailing zeros must go: %q", got)
	}
	if got := formatSize(254, print.UnitInch); got != "10" {
		t.Errorf("%q", got)
	}
}

func TestInfoReportsTheSizeOnPaper(t *testing.T) {
	m := newModel(t, func(e *Env, v *Values) { v.Options.Margin = print.MarginNone })
	info := m.Info()
	if info.Err != nil || !near(info.WidthMM, 280, 0.1) || !near(info.HeightMM, 210, 0.1) {
		t.Fatalf("%+v", info)
	}
	if !near(info.DPI, 4000/(280/25.4), 1) || info.Low || info.Cut || !info.Known {
		t.Fatalf("%+v", info)
	}
	m.SetPlacement(print.PlaceFill)
	if info := m.Info(); !info.Cut {
		t.Fatalf("a 4:3 picture filling an A4 sheet is cropped: %+v", info)
	}
}

func TestInfoFlagsLowResolutionAndUnknownDensity(t *testing.T) {
	m := newModel(t, func(e *Env, v *Values) {
		e.Pic = print.Picture{W: 800, H: 600}
		v.Options.Margin = print.MarginNone
	})
	info := m.Info()
	if !info.Low || info.Known {
		t.Fatalf("%+v", info)
	}
}

func TestInfoErrorsOnAnInvalidCustomSize(t *testing.T) {
	m := newModel(t, nil)
	m.SetPlacement(print.PlaceCustom)
	m.SetKeep(false)
	m.SetWidthText("")
	if m.Info().Err == nil {
		t.Fatal("expected an error")
	}
}

func TestSheetPreviewFollowsTheOptions(t *testing.T) {
	m := newModel(t, nil)
	pv := m.Sheet(300, 400)
	if pv.Err != nil || pv.Sheet == nil {
		t.Fatalf("%+v", pv.Err)
	}
	if pv.Page.Sheet.X > 300 || pv.Page.Sheet.Y > 400 || (pv.Page.Sheet.X < 290 && pv.Page.Sheet.Y < 390) {
		t.Fatalf("the sheet %v does not fill the 300x400 box", pv.Page.Sheet)
	}
	if !pv.Page.Landscape {
		t.Fatal("a 4:3 picture on an automatic sheet is landscape")
	}
	if pv.Sheet.Bounds().Size() != pv.Page.Sheet {
		t.Fatalf("image %v page %v", pv.Sheet.Bounds().Size(), pv.Page.Sheet)
	}
	same := m.Sheet(300, 400)
	if same.Sheet != pv.Sheet {
		t.Fatal("an unchanged request must come from the cache")
	}
	m.SetOrientation(print.OrientPortrait)
	port := m.Sheet(300, 400)
	if port.Page.Landscape || port.Page.Sheet.Y <= port.Page.Sheet.X {
		t.Fatalf("portrait sheet %v", port.Page.Sheet)
	}
	m.SetPaper(print.A3.ID)
	a3 := m.Sheet(300, 400)
	if a3.Page.Sheet.X > 300 || a3.Page.Sheet.Y > 400 {
		t.Fatalf("%v", a3.Page.Sheet)
	}
	if got := m.Sheet(0, 10); got.Err == nil {
		t.Fatal("an empty box must be refused")
	}
}

func TestSheetPreviewShowsThePictureOnPaper(t *testing.T) {
	m := newModel(t, func(e *Env, v *Values) { v.Options.Margin = print.MarginNone })
	pv := m.Sheet(400, 300)
	c := pv.Page.Dest
	mid := image.Pt((c.Min.X+c.Max.X)/2, (c.Min.Y+c.Max.Y)/2)
	px := pv.Sheet.RGBAAt(mid.X, mid.Y)
	if px.R < 90 || px.R > 165 || px.B != 150 {
		t.Errorf("the centre of the gradient picture is %v", px)
	}
	if corner := pv.Sheet.RGBAAt(0, 0); corner.R != 255 && !pv.Page.Dest.In(image.Rectangle{Max: pv.Page.Sheet}) {
		t.Errorf("corner %v", corner)
	}
}

func TestSheetPreviewKeepsTheFrameWhenTheCustomSizeIsInvalid(t *testing.T) {
	m := newModel(t, nil)
	m.SetPlacement(print.PlaceCustom)
	m.SetKeep(false)
	m.SetWidthText("")
	pv := m.Sheet(300, 400)
	if pv.Err == nil || pv.Page.Sheet.X < 1 || !pv.Page.Dest.Empty() || pv.Sheet != nil {
		t.Fatalf("%+v", pv.Page)
	}
}

func TestLabels(t *testing.T) {
	i18n.SetLang("en")
	if got := PaperLabel(print.A4); got != "A4" {
		t.Error(got)
	}
	if got := PaperLabel(print.Photo1); got != "10 × 15 cm" {
		t.Error(got)
	}
	if got := PaperLabel(print.Photo2); got != "13 × 18 cm" {
		t.Error(got)
	}
	if OrientationLabel(print.OrientAuto) != "Automatic" || PlacementLabel(print.PlaceFill) != "Fill the page" || MarginLabel(print.MarginNarrow) != "Narrow" {
		t.Error("labels")
	}
}

func FuzzCleanDecimal(f *testing.F) {
	f.Add("12.5")
	f.Add("١٢")
	f.Add("1e9,,..")
	f.Fuzz(func(t *testing.T, s string) {
		c := CleanDecimal(s)
		if len(c) > maxSizeChars+maxDecimals {
			t.Fatalf("%q -> %q is too long", s, c)
		}
		if CleanDecimal(c) != c {
			t.Fatalf("%q is not stable: %q", c, CleanDecimal(c))
		}
		if v, ok := ParseSize(c, print.UnitCM); ok && (v <= 0 || math.IsInf(v, 0) || math.IsNaN(v)) {
			t.Fatalf("%q parsed to %v", c, v)
		}
	})
}
