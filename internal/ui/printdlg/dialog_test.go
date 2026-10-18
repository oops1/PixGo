package printdlg

import (
	"image"
	"strings"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/print"
)

func newEN(t testing.TB, mutate func(*Env, *Values)) *rig {
	t.Helper()
	withScale(t, 1, "en")
	env, v := testEnv(), testValues()
	if mutate != nil {
		mutate(&env, &v)
	}
	return newRig(t, env, v, 1000, 700)
}

func walk(r *rig, key widget.KeyCode, mod widget.KeyMod, n int) []Part {
	var out []Part
	for i := 0; i < n; i++ {
		r.key(key, mod)
		out = append(out, r.d.Focused())
	}
	return out
}

func TestOpensOnThePrinterWithTheGivenValues(t *testing.T) {
	r := newEN(t, nil)
	if r.d.Focused() != PartPrinter {
		t.Fatalf("focus on %d", r.d.Focused())
	}
	v := r.d.Values()
	if v.PDF || v.Printer != "Photo Inkjet" || v.Copies != 1 || v.Options != print.DefaultOptions() {
		t.Fatalf("%+v", v)
	}
	if r.d.Widget().Title != "Print" {
		t.Errorf("title %q", r.d.Widget().Title)
	}
	if r.d.PrintLabel() != "Print" {
		t.Errorf("button %q", r.d.PrintLabel())
	}
}

func TestEnterPrintsAndReportsTheValues(t *testing.T) {
	r := newEN(t, nil)
	r.press(widget.KeyEnter)
	got := r.last()
	if !got.ok || got.values.Printer != "Photo Inkjet" || got.values.PDF {
		t.Fatalf("%+v", got)
	}
	if !r.d.Done() {
		t.Fatal("the dialog must be done")
	}
	r.press(widget.KeyEnter)
	if len(r.results) != 1 {
		t.Fatalf("%d results", len(r.results))
	}
}

func TestEscapeCancels(t *testing.T) {
	r := newEN(t, nil)
	r.press(widget.KeyEscape)
	if got := r.last(); got.ok {
		t.Fatalf("%+v", got)
	}
}

func TestEnterOnCancelCancels(t *testing.T) {
	r := newEN(t, nil)
	r.focus(PartCancel)
	r.press(widget.KeyEnter)
	if r.last().ok {
		t.Fatal("Enter on Cancel must cancel")
	}
}

func TestTabWalksTheEnabledStopsAndWraps(t *testing.T) {
	r := newEN(t, nil)
	want := []Part{PartSystem, PartCopies, PartPaper, PartOrientation, PartPlacement, PartMargins, PartPrint, PartCancel, PartPrinter, PartSystem}
	got := walk(r, widget.KeyTab, 0, len(want))
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("step %d: %d, want %d (all %v)", i, got[i], want[i], got)
		}
	}
	back := walk(r, widget.KeyTab, widget.ModShift, 3)
	if back[0] != PartPrinter || back[1] != PartCancel || back[2] != PartPrint {
		t.Fatalf("shift+tab: %v", back)
	}
}

func TestPDFSkipsCopiesAndTheSystemDialog(t *testing.T) {
	r := newEN(t, nil)
	r.choose(PartPrinter, i18n.T("print.dest.pdf"))
	if r.d.Enabled(PartCopies) || r.d.Enabled(PartSystem) || r.d.Enabled(PartCopiesInc) {
		t.Fatal("PDF has no copies and no system dialog")
	}
	got := walk(r, widget.KeyTab, 0, 6)
	for _, p := range got {
		if p == PartCopies || p == PartSystem {
			t.Fatalf("tab stopped on a disabled part: %v", got)
		}
	}
	if r.d.PrintLabel() != "Save as PDF…" {
		t.Errorf("button %q", r.d.PrintLabel())
	}
	r.focus(PartPrint)
	r.press(widget.KeyEnter)
	if v := r.last().values; !r.last().ok || !v.PDF || v.Printer != "" || v.Copies != 1 {
		t.Fatalf("%+v", v)
	}
}

func TestSizeFieldsJoinTheWalkOnlyForACustomSize(t *testing.T) {
	r := newEN(t, nil)
	for _, p := range []Part{PartWidth, PartHeight, PartCm, PartInch, PartKeep} {
		if r.d.Enabled(p) {
			t.Fatalf("part %d is enabled without a custom size", p)
		}
	}
	r.choose(PartPlacement, i18n.T("print.place.custom"))
	r.focus(PartPlacement)
	got := walk(r, widget.KeyTab, 0, 6)
	want := []Part{PartWidth, PartHeight, PartCm, PartInch, PartKeep, PartMargins}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("step %d: %d, want %d (%v)", i, got[i], want[i], got)
		}
	}
}

func TestArrowsChangeTheValueOfAClosedDropdown(t *testing.T) {
	r := newEN(t, nil)
	r.focus(PartPaper)
	r.press(widget.KeyDown)
	if r.d.Values().Options.Paper != "a5" {
		t.Fatalf("down: %q", r.d.Values().Options.Paper)
	}
	r.press(widget.KeyRight)
	if r.d.Values().Options.Paper != "a3" {
		t.Fatalf("right: %q", r.d.Values().Options.Paper)
	}
	r.press(widget.KeyUp)
	r.press(widget.KeyLeft)
	if r.d.Values().Options.Paper != "a4" {
		t.Fatalf("up/left: %q", r.d.Values().Options.Paper)
	}
	r.press(widget.KeyLeft)
	if r.d.Values().Options.Paper != "a4" {
		t.Fatal("the first item must stop the arrows")
	}
	r.press(widget.KeyEnd)
	if r.d.Values().Options.Paper != "13x18" {
		t.Fatalf("end: %q", r.d.Values().Options.Paper)
	}
	r.press(widget.KeyRight)
	if r.d.Values().Options.Paper != "13x18" {
		t.Fatal("the last item must stop the arrows")
	}
	r.press(widget.KeyHome)
	if r.d.Values().Options.Paper != "a4" {
		t.Fatalf("home: %q", r.d.Values().Options.Paper)
	}
	r.focus(PartOrientation)
	r.press(widget.KeyRight)
	r.focus(PartPlacement)
	r.press(widget.KeyRight)
	r.focus(PartMargins)
	r.press(widget.KeyRight)
	o := r.d.Values().Options
	if o.Orientation != print.OrientPortrait || o.Placement != print.PlaceFill || o.Margin != print.MarginNormal {
		t.Fatalf("%+v", o)
	}
}

func TestSpaceOpensTheListAndEnterPicksWithoutPrinting(t *testing.T) {
	r := newEN(t, nil)
	r.focus(PartPaper)
	r.press(widget.KeySpace)
	if !r.d.ListOpen() {
		t.Fatal("the list did not open")
	}
	r.press(widget.KeyDown)
	r.press(widget.KeyDown)
	if r.d.Values().Options.Paper != "a4" {
		t.Fatal("moving through the list must not change the value yet")
	}
	r.press(widget.KeyEnter)
	if r.d.ListOpen() || r.d.Values().Options.Paper != "a3" {
		t.Fatalf("open=%v paper=%q", r.d.ListOpen(), r.d.Values().Options.Paper)
	}
	if len(r.results) != 0 {
		t.Fatal("Enter inside the list must not print")
	}
}

func TestListKeysCoverHomeEndAndPages(t *testing.T) {
	r := newEN(t, nil)
	r.focus(PartPaper)
	r.key(widget.KeyDown, widget.ModAlt)
	if !r.d.ListOpen() {
		t.Fatal("Alt+Down must open the list")
	}
	r.press(widget.KeyEnd)
	r.press(widget.KeySpace)
	if r.d.Values().Options.Paper != "13x18" {
		t.Fatalf("%q", r.d.Values().Options.Paper)
	}
	r.press(widget.KeyF4)
	r.press(widget.KeyHome)
	r.press(widget.KeyPageDown)
	r.press(widget.KeyPageUp)
	r.press(widget.KeyEnter)
	if r.d.Values().Options.Paper != "a4" {
		t.Fatalf("%q", r.d.Values().Options.Paper)
	}
	r.press(widget.KeyF4)
	r.press(widget.KeyF4)
	if r.d.ListOpen() {
		t.Fatal("F4 must close the list")
	}
}

func TestEscapeClosesTheListBeforeTheDialog(t *testing.T) {
	r := newEN(t, nil)
	r.focus(PartPaper)
	r.press(widget.KeySpace)
	r.press(widget.KeyEscape)
	if r.d.ListOpen() {
		t.Fatal("the list is still open")
	}
	if len(r.results) != 0 || r.d.Done() {
		t.Fatal("the first Escape must only close the list")
	}
	r.press(widget.KeyEscape)
	if r.last().ok {
		t.Fatal("the second Escape cancels")
	}
}

func TestTabInsideTheListClosesItAndMoves(t *testing.T) {
	r := newEN(t, nil)
	r.focus(PartPaper)
	r.press(widget.KeySpace)
	r.press(widget.KeyDown)
	r.press(widget.KeyTab)
	if r.d.ListOpen() || r.d.Focused() != PartOrientation {
		t.Fatalf("open=%v focus=%d", r.d.ListOpen(), r.d.Focused())
	}
	if r.d.Values().Options.Paper != "a4" {
		t.Fatal("Tab must not pick")
	}
}

func TestClickOpensTheListAndAnItemPicksIt(t *testing.T) {
	r := newEN(t, nil)
	r.render()
	r.clickPart(PartPaper)
	if !r.d.ListOpen() {
		t.Fatal("the list did not open")
	}
	x, y := centre(r.d.form.itemRect(4))
	r.click(x, y)
	if r.d.ListOpen() || r.d.Values().Options.Paper != "legal" {
		t.Fatalf("open=%v paper=%q", r.d.ListOpen(), r.d.Values().Options.Paper)
	}
	if len(r.results) != 0 {
		t.Fatal("picking must not print")
	}
}

func TestClickingAFieldTwiceClosesTheList(t *testing.T) {
	r := newEN(t, nil)
	r.clickPart(PartMargins)
	if !r.d.ListOpen() {
		t.Fatal("not open")
	}
	x, y := centre(r.d.Rect(PartMargins))
	if r.d.form.listIndexAt(x, y) >= 0 {
		t.Skip("the list covers its own field")
	}
	r.click(x, y)
	if r.d.ListOpen() {
		t.Fatal("a click outside the list must close it")
	}
}

func TestClickOutsideTheListClosesItWithoutActivatingWhatIsUnder(t *testing.T) {
	r := newEN(t, nil)
	r.clickPart(PartPaper)
	if !r.d.ListOpen() {
		t.Fatal("not open")
	}
	r.clickPart(PartCancel)
	if r.d.ListOpen() {
		t.Fatal("the list is still open")
	}
	if len(r.results) != 0 {
		t.Fatal("the click that closes the list must not press Cancel")
	}
}

func TestMouseOverTheListMovesTheHighlight(t *testing.T) {
	r := newEN(t, nil)
	r.clickPart(PartPaper)
	x, y := centre(r.d.form.itemRect(3))
	r.eng.SendMouseMove(x, y)
	if r.d.form.list.sel != 3 {
		t.Fatalf("highlight on %d", r.d.form.list.sel)
	}
	if r.d.Values().Options.Paper != "a4" {
		t.Fatal("hovering must not change the value")
	}
}

func TestWheelScrollsALongList(t *testing.T) {
	printers := make([]Printer, 14)
	for i := range printers {
		printers[i] = Printer{Name: "Printer " + string(rune('A'+i))}
	}
	r := newEN(t, func(e *Env, v *Values) { e.Printers = printers; v.Printer = "Printer A" })
	r.focus(PartPrinter)
	r.press(widget.KeySpace)
	ls := r.d.form.list
	if ls.rows >= len(ls.items) {
		t.Fatalf("a 15 item list must scroll, rows %d", ls.rows)
	}
	x, y := centre(ls.rect)
	r.eng.SendMouseWheelPixels(x, y, 0, 120)
	if r.d.form.list.top == 0 {
		t.Fatal("the wheel did not scroll the list")
	}
	r.press(widget.KeyEnd)
	if ls.top != len(ls.items)-ls.rows {
		t.Fatalf("end must show the tail, top=%d rows=%d", ls.top, ls.rows)
	}
	r.press(widget.KeyEnter)
	if r.d.Values().Printer != "" || !r.d.Values().PDF {
		t.Fatalf("the last item is the PDF destination: %+v", r.d.Values())
	}
}

func TestCopiesFieldTypingAndArrows(t *testing.T) {
	r := newEN(t, nil)
	r.focus(PartCopies)
	r.typeText("3")
	if r.d.Values().Copies != 3 {
		t.Fatalf("%d", r.d.Values().Copies)
	}
	r.press(widget.KeyUp)
	if r.d.Values().Copies != 4 {
		t.Fatalf("up: %d", r.d.Values().Copies)
	}
	r.key(widget.KeyUp, widget.ModShift)
	if r.d.Values().Copies != 14 {
		t.Fatalf("shift+up: %d", r.d.Values().Copies)
	}
	r.press(widget.KeyDown)
	if r.d.Values().Copies != 13 {
		t.Fatalf("down: %d", r.d.Values().Copies)
	}
	r.typeText("55")
	if r.d.Values().Copies != 55 {
		t.Fatalf("typed 55: %d", r.d.Values().Copies)
	}
	r.typeText("7")
	if r.d.Model().CopiesText() != "557" && r.d.Model().CopiesText() != "55" {
		t.Fatalf("text %q", r.d.Model().CopiesText())
	}
	r.key(widget.KeyUp, widget.ModShift)
	r.key(widget.KeyUp, widget.ModShift)
	r.key(widget.KeyUp, widget.ModShift)
	r.key(widget.KeyUp, widget.ModShift)
	r.key(widget.KeyUp, widget.ModShift)
	r.key(widget.KeyUp, widget.ModShift)
	if r.d.Values().Copies != MaxCopies {
		t.Fatalf("the maximum is %d, got %d", MaxCopies, r.d.Values().Copies)
	}
	r.focus(PartCopies)
	r.key(widget.KeyDown, widget.ModShift)
	for i := 0; i < 12; i++ {
		r.key(widget.KeyDown, widget.ModShift)
	}
	if r.d.Values().Copies != MinCopies {
		t.Fatalf("the minimum is %d, got %d", MinCopies, r.d.Values().Copies)
	}
}

func TestCopiesFieldEditing(t *testing.T) {
	r := newEN(t, nil)
	r.focus(PartCopies)
	r.typeText("12")
	r.press(widget.KeyLeft)
	r.press(widget.KeyBackspace)
	if r.d.Model().CopiesText() != "2" {
		t.Fatalf("backspace: %q", r.d.Model().CopiesText())
	}
	r.press(widget.KeyHome)
	r.press(widget.KeyDelete)
	if r.d.Model().CopiesText() != "" || r.d.Enabled(PartPrint) {
		t.Fatalf("empty copies must disable Print: %q", r.d.Model().CopiesText())
	}
	r.press(widget.KeyEnter)
	if len(r.results) != 0 {
		t.Fatal("Enter with invalid copies must not print")
	}
	r.typeText("x")
	r.typeText("2")
	r.press(widget.KeyEnd)
	r.press(widget.KeyRight)
	r.typeText("5")
	if r.d.Model().CopiesText() != "25" {
		t.Fatalf("%q", r.d.Model().CopiesText())
	}
	r.key(widget.KeyA, widget.ModCtrl)
	r.typeText("9")
	if r.d.Model().CopiesText() != "9" {
		t.Fatalf("select all then type: %q", r.d.Model().CopiesText())
	}
}

func TestCopiesButtonsStepTheCount(t *testing.T) {
	r := newEN(t, nil)
	r.render()
	r.clickPart(PartCopiesInc)
	r.clickPart(PartCopiesInc)
	if r.d.Values().Copies != 3 {
		t.Fatalf("%d", r.d.Values().Copies)
	}
	r.clickPart(PartCopiesDec)
	r.clickPart(PartCopiesDec)
	r.clickPart(PartCopiesDec)
	if r.d.Values().Copies != 1 {
		t.Fatalf("%d", r.d.Values().Copies)
	}
}

func customRig(t testing.TB) *rig {
	r := newEN(t, nil)
	r.focus(PartPlacement)
	r.press(widget.KeyEnd)
	if r.d.Values().Options.Placement != print.PlaceCustom {
		t.Fatalf("placement %v", r.d.Values().Options.Placement)
	}
	return r
}

func TestCustomSizeFollowsTheKeyboard(t *testing.T) {
	r := customRig(t)
	r.press(widget.KeyTab)
	if r.d.Focused() != PartWidth {
		t.Fatalf("focus %d", r.d.Focused())
	}
	r.typeText("20")
	m := r.d.Model()
	if m.WidthText() != "20" || m.HeightText() != "15" {
		t.Fatalf("%q x %q", m.WidthText(), m.HeightText())
	}
	r.press(widget.KeyTab)
	r.typeText("9")
	if m.HeightText() != "9" || m.WidthText() != "12" {
		t.Fatalf("%q x %q", m.WidthText(), m.HeightText())
	}
	r.press(widget.KeyUp)
	if m.HeightText() != "9.1" {
		t.Fatalf("up: %q", m.HeightText())
	}
	r.key(widget.KeyDown, widget.ModShift)
	if m.HeightText() != "8.1" {
		t.Fatalf("shift+down: %q", m.HeightText())
	}
	r.press(widget.KeyTab)
	r.press(widget.KeyRight)
	if r.d.Values().Options.Unit != print.UnitInch || r.d.Focused() != PartInch {
		t.Fatalf("unit %v focus %d", r.d.Values().Options.Unit, r.d.Focused())
	}
	if m.HeightText() != "3.19" {
		t.Fatalf("8.1 cm in inches: %q", m.HeightText())
	}
	r.press(widget.KeyLeft)
	if r.d.Values().Options.Unit != print.UnitCM {
		t.Fatal("left must go back to centimetres")
	}
	r.press(widget.KeyTab)
	r.press(widget.KeyTab)
	if r.d.Focused() != PartKeep {
		t.Fatalf("focus %d", r.d.Focused())
	}
	r.press(widget.KeySpace)
	if r.d.Values().Options.Keep {
		t.Fatal("space must clear the checkbox")
	}
}

func TestCustomSizeWithAPointOrACommaAndNoise(t *testing.T) {
	r := customRig(t)
	r.focus(PartWidth)
	r.typeText("12,5")
	if r.d.Model().WidthText() != "12,5" || !near(r.d.Values().Options.CustomW, 125, 1e-6) {
		t.Fatalf("%q %v", r.d.Model().WidthText(), r.d.Values().Options.CustomW)
	}
	r.typeText("x")
	r.typeText(".")
	r.typeText("7")
	r.typeText("7")
	r.typeText("7")
	if got := r.d.Model().WidthText(); got != "12,57" {
		t.Fatalf("only the first separator and two decimals are kept: %q", got)
	}
}

func TestEmptyCustomSizeBlocksPrinting(t *testing.T) {
	r := customRig(t)
	r.focus(PartWidth)
	r.press(widget.KeyBackspace)
	if r.d.Enabled(PartPrint) {
		t.Fatal("Print must be disabled")
	}
	r.press(widget.KeyEnter)
	if len(r.results) != 0 {
		t.Fatal("Enter must not print an invalid custom size")
	}
	lines := r.d.InfoLines()
	if len(lines) != 1 || lines[0] != i18n.T("print.info.badSize") {
		t.Fatalf("%v", lines)
	}
	r.typeText("5")
	if !r.d.Enabled(PartPrint) {
		t.Fatal("a valid width re-enables Print")
	}
	r.press(widget.KeyEnter)
	if got := r.last(); !got.ok || got.values.Options.Placement != print.PlaceCustom {
		t.Fatalf("%+v", got)
	}
}

func TestCustomSizeMouse(t *testing.T) {
	r := customRig(t)
	r.render()
	r.clickPart(PartWidth)
	if r.d.Focused() != PartWidth {
		t.Fatalf("focus %d", r.d.Focused())
	}
	r.key(widget.KeyA, widget.ModCtrl)
	r.typeText("30")
	if r.d.Model().WidthText() != "30" {
		t.Fatalf("%q", r.d.Model().WidthText())
	}
	r.clickPart(PartInch)
	if r.d.Values().Options.Unit != print.UnitInch {
		t.Fatal("the inch segment did not switch the unit")
	}
	r.clickPart(PartCm)
	if r.d.Values().Options.Unit != print.UnitCM {
		t.Fatal("the cm segment did not switch the unit")
	}
	r.clickPart(PartKeep)
	if r.d.Values().Options.Keep {
		t.Fatal("the checkbox did not toggle")
	}
}

func TestClickOnADisabledSizeFieldDoesNothing(t *testing.T) {
	r := newEN(t, nil)
	r.render()
	before := r.d.Focused()
	r.clickPart(PartWidth)
	if r.d.Focused() != before {
		t.Fatalf("focus moved to %d", r.d.Focused())
	}
}

func TestPrintButtonClickReportsTheValues(t *testing.T) {
	r := newEN(t, nil)
	r.render()
	r.choose(PartOrientation, i18n.T("print.orient.landscape"))
	r.clickPart(PartPrint)
	got := r.last()
	if !got.ok || got.values.Options.Orientation != print.OrientLandscape {
		t.Fatalf("%+v", got)
	}
}

func TestCancelButtonClickReportsCancel(t *testing.T) {
	r := newEN(t, nil)
	r.render()
	r.clickPart(PartCancel)
	if r.last().ok {
		t.Fatal("cancelled with ok")
	}
}

func TestSystemDialogHookAndApply(t *testing.T) {
	r := newEN(t, nil)
	r.render()
	r.clickPart(PartSystem)
	if len(r.systems) != 1 || r.systems[0].Printer != "Photo Inkjet" {
		t.Fatalf("hook calls %+v", r.systems)
	}
	r.applyFn("Office Laser", 4)
	v := r.d.Values()
	if v.Printer != "Office Laser" || v.Copies != 4 {
		t.Fatalf("%+v", v)
	}
	r.applyFn("", 0)
	if r.d.Values().Printer != "Office Laser" || r.d.Values().Copies != 4 {
		t.Fatal("an empty answer must keep the choice")
	}
	r.applyFn("Unknown", 2)
	if r.d.Values().Printer != "Office Laser" || r.d.Values().Copies != 2 {
		t.Fatalf("%+v", r.d.Values())
	}
	r.focus(PartPrint)
	r.press(widget.KeyEnter)
	r.applyFn("Photo Inkjet", 9)
	if r.last().values.Printer != "Office Laser" {
		t.Fatal("a late answer must not change a finished dialog")
	}
}

func TestSystemDialogHiddenWithoutAHookOrOnRequest(t *testing.T) {
	withScale(t, 1, "en")
	eng := newRig(t, testEnv(), testValues(), 1000, 700)
	if eng.d.Rect(PartSystem).Empty() {
		t.Fatal("the button must exist with a hook")
	}
	env := testEnv()
	env.System = false
	r := newRig(t, env, testValues(), 1000, 700)
	if !r.d.Rect(PartSystem).Empty() {
		t.Fatal("the button must be hidden when the system dialog is unavailable")
	}
}

func TestExportDialog(t *testing.T) {
	r := newEN(t, func(e *Env, v *Values) { e.Export = true; e.System = false })
	if r.d.Widget().Title != "Export to PDF" {
		t.Errorf("title %q", r.d.Widget().Title)
	}
	if r.d.Focused() != PartPaper {
		t.Errorf("focus %d", r.d.Focused())
	}
	if !r.d.Rect(PartPrinter).Empty() || !r.d.Rect(PartCopies).Empty() {
		t.Error("printer and copies must be hidden")
	}
	got := walk(r, widget.KeyTab, 0, 5)
	for _, p := range got {
		if p == PartPrinter || p == PartCopies {
			t.Fatalf("tab reached a hidden part: %v", got)
		}
	}
	r.focus(PartPrint)
	r.press(widget.KeyEnter)
	if v := r.last().values; !r.last().ok || !v.PDF || v.Printer != "" {
		t.Fatalf("%+v", v)
	}
}

func TestInfoLinesDescribeThePrint(t *testing.T) {
	r := newEN(t, nil)
	lines := r.d.InfoLines()
	if len(lines) != 2 || !strings.HasPrefix(lines[0], "On paper: 26.4 × 19.8 cm") || !strings.HasPrefix(lines[1], "Resolution: 385 dpi") {
		t.Fatalf("%v", lines)
	}
	r.choose(PartPlacement, i18n.T("print.place.fill"))
	lines = r.d.InfoLines()
	if len(lines) != 3 || lines[2] != i18n.T("print.info.cut") {
		t.Fatalf("%v", lines)
	}
	r.choose(PartPlacement, i18n.T("print.place.actual"))
	lines = r.d.InfoLines()
	if len(lines) != 3 || !strings.Contains(lines[0], "28.5 × 19.8 cm") || lines[1] != "Resolution: 300 dpi" || lines[2] != i18n.T("print.info.cut") {
		t.Fatalf("actual size of 4000 px at 300 dpi is 33.9 cm wide and is cut to the page: %v", lines)
	}
}

func TestInfoLinesFlagLowResolutionAndUnknownDensity(t *testing.T) {
	r := newEN(t, func(e *Env, v *Values) {
		e.Pic = print.Picture{W: 800, H: 600}
		v.Options.Placement = print.PlaceActual
	})
	lines := r.d.InfoLines()
	if len(lines) != 3 || !strings.Contains(lines[1], "(low)") && !strings.Contains(lines[1], "dpi") {
		t.Fatalf("%v", lines)
	}
	if lines[2] != i18n.T("print.info.unknownDpi", 300) {
		t.Fatalf("%v", lines)
	}
	r.choose(PartPlacement, i18n.T("print.place.fit"))
	if got := r.d.InfoLines(); !strings.Contains(got[1], "(low)") {
		t.Fatalf("800 px over 28 cm is a low resolution: %v", got)
	}
}

func TestInchesShowInTheInfoLine(t *testing.T) {
	r := customRig(t)
	r.focus(PartInch)
	r.press(widget.KeySpace)
	if got := r.d.InfoLines()[0]; !strings.HasSuffix(got, " in") || !strings.Contains(got, "10.39 × 7.79") {
		t.Fatalf("%q", got)
	}
}

func TestSmallCanvasScrollsWithTheWheelAndTheKeyboard(t *testing.T) {
	withScale(t, 1, "en")
	r := newRig(t, testEnv(), testValues(), 420, 320)
	f := r.d.form
	if f.lay.scrollMax == 0 {
		t.Fatal("a 420x320 canvas must scroll")
	}
	if f.scroll != 0 {
		t.Fatal("starts at the top")
	}
	x, y := centre(f.viewRect())
	r.eng.SendMouseWheelPixels(x, y, 0, 60)
	if f.scroll == 0 || f.scroll > f.lay.scrollMax {
		t.Fatalf("scroll %d of %d", f.scroll, f.lay.scrollMax)
	}
	r.eng.SendMouseWheelPixels(x, y, 0, 100000)
	if f.scroll != f.lay.scrollMax {
		t.Fatalf("scroll %d, max %d", f.scroll, f.lay.scrollMax)
	}
	r.eng.SendMouseWheelPixels(x, y, 0, -100000)
	if f.scroll != 0 {
		t.Fatalf("scroll %d", f.scroll)
	}
	r.focus(PartMargins)
	if f.scroll == 0 {
		t.Fatal("focusing the last control must scroll it into view")
	}
	if !f.rect(PartMargins).In(f.viewRect()) {
		t.Fatalf("Margins %v is outside the view %v", f.rect(PartMargins), f.viewRect())
	}
	r.focus(PartPrinter)
	if f.scroll != 0 {
		t.Fatalf("focusing the first control must scroll back: %d", f.scroll)
	}
}

func TestScrolledControlsAreStillClickable(t *testing.T) {
	withScale(t, 1, "en")
	r := newRig(t, testEnv(), testValues(), 420, 320)
	f := r.d.form
	r.focus(PartMargins)
	r.render()
	r.clickPart(PartMargins)
	if !r.d.ListOpen() {
		t.Fatal("a scrolled dropdown must open on click")
	}
	r.press(widget.KeyEscape)
	r.focus(PartPrinter)
	x, y := centre(r.d.Rect(PartPrint))
	if !r.d.Rect(PartPrint).In(f.Bounds()) {
		t.Fatal("the buttons must stay inside the dialog")
	}
	r.click(x, y)
	if !r.last().ok {
		t.Fatal("Print did not work on a scrolled dialog")
	}
}

func TestControlsOutsideTheViewportAreNotHit(t *testing.T) {
	withScale(t, 1, "en")
	r := newRig(t, testEnv(), testValues(), 420, 320)
	f := r.d.form
	hidden := f.rect(PartMargins)
	if hidden.In(f.viewRect()) {
		t.Skip("the control is visible")
	}
	if got := f.hitAt(hidden.Min.X+2, hidden.Min.Y+2); got != PartNone {
		t.Fatalf("hit %d outside the viewport", got)
	}
}

func TestCursors(t *testing.T) {
	r := newEN(t, nil)
	r.render()
	x, y := centre(r.d.Rect(PartCopies))
	if got := r.d.form.Cursor(x, y); got != widget.CursorIBeam {
		t.Errorf("field: %v", got)
	}
	x, y = centre(r.d.Rect(PartPrint))
	if got := r.d.form.Cursor(x, y); got != widget.CursorHand {
		t.Errorf("button: %v", got)
	}
	b := r.d.form.Bounds()
	if got := r.d.form.Cursor(b.Min.X+1, b.Min.Y+1); got != widget.CursorArrow {
		t.Errorf("background: %v", got)
	}
}

func TestHoverTracksTheMouse(t *testing.T) {
	r := newEN(t, nil)
	r.render()
	x, y := centre(r.d.Rect(PartPaper))
	r.eng.SendMouseMove(x, y)
	if r.d.form.hover != PartPaper {
		t.Fatalf("hover %d", r.d.form.hover)
	}
	r.eng.SendMouseMove(0, 0)
	if r.d.form.hover != PartNone {
		t.Fatalf("hover %d", r.d.form.hover)
	}
}

func TestFocusIndicatorAppearsOnlyAfterTheKeyboardIsUsed(t *testing.T) {
	r := newEN(t, nil)
	before := r.render()
	r.press(widget.KeyTab)
	after := r.render()
	field := r.d.Rect(PartSystem).Inset(-4)
	if sameImage(before, after, field) {
		t.Fatal("Tab must show a focus ring on the next control")
	}
	r.clickPart(PartMargins)
	r.press(widget.KeyEscape)
	if r.d.form.keyNav {
		t.Skip("keyboard state is sticky")
	}
}

func sameImage(a, b *image.RGBA, r image.Rectangle) bool {
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if a.RGBAAt(x, y) != b.RGBAAt(x, y) {
				return false
			}
		}
	}
	return true
}

func TestPreviewDrawsThePictureOnTheSheet(t *testing.T) {
	r := newEN(t, func(e *Env, v *Values) { v.Options.Margin = print.MarginNone })
	img := r.render()
	pv := r.d.Preview()
	if pv.Empty() {
		t.Fatal("no preview rectangle")
	}
	mid := image.Pt((pv.Min.X+pv.Max.X)/2, (pv.Min.Y+pv.Max.Y)/2)
	c := img.RGBAAt(mid.X, mid.Y)
	if c.B != 150 {
		t.Fatalf("the middle of the preview is %v, expected the gradient picture", c)
	}
	r.choose(PartOrientation, i18n.T("print.orient.portrait"))
	img2 := r.render()
	if sameImage(img, img2, pv) {
		t.Fatal("changing the orientation must change the preview")
	}
}

func TestPreviewWithoutAThumbnailStillDrawsASheet(t *testing.T) {
	r := newEN(t, func(e *Env, v *Values) { e.Preview = nil })
	img := r.render()
	pv := r.d.Preview()
	mid := image.Pt((pv.Min.X+pv.Max.X)/2, (pv.Min.Y+pv.Max.Y)/2)
	c := img.RGBAAt(mid.X, mid.Y)
	if c.R == 0 && c.G == 0 && c.B == 0 {
		t.Fatalf("empty preview %v", c)
	}
}

func TestEveryLanguageAndThemeRenders(t *testing.T) {
	for _, lang := range langs {
		withScale(t, 1, lang)
		for _, dark := range []bool{true, false} {
			r := newRigThemed(t, testEnv(), testValues(), 1000, 700, dark)
			img := r.render()
			if img.Bounds().Empty() {
				t.Fatal("empty frame")
			}
			if r.d.Widget().Title != i18n.T("print.title") {
				t.Errorf("%s: title %q", lang, r.d.Widget().Title)
			}
		}
	}
}

func TestOptionsRoundTripThroughTheDialog(t *testing.T) {
	r := newEN(t, func(e *Env, v *Values) {
		v.Options = print.Options{Paper: "legal", Orientation: print.OrientLandscape, Placement: print.PlaceActual, Margin: print.MarginNormal, Unit: print.UnitInch, Keep: false}
		v.Copies = 7
	})
	got := r.d.Values()
	if got.Options.Paper != "legal" || got.Options.Orientation != print.OrientLandscape || got.Options.Placement != print.PlaceActual ||
		got.Options.Margin != print.MarginNormal || got.Options.Unit != print.UnitInch || got.Options.Keep || got.Copies != 7 {
		t.Fatalf("%+v", got)
	}
}

func TestKeysNeverEditADisabledControl(t *testing.T) {
	r := newEN(t, nil)
	r.focus(PartCopies)
	r.d.Model().SetDestIndex(3)
	r.typeText("7")
	if r.d.Model().CopiesText() != "1" || r.d.Values().Copies != 1 {
		t.Fatalf("a PDF has no copies, got %q", r.d.Model().CopiesText())
	}
	if !r.d.Enabled(r.d.Focused()) && r.d.Focused() != PartNone {
		t.Fatalf("the focus stays on the disabled part %d", r.d.Focused())
	}
}
