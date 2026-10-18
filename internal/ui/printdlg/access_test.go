package printdlg

import (
	"testing"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/print"
	"github.com/oops1/PixGo/internal/testutil"
)

func accessNode(r *rig, role widget.AccessRole, name string) *widget.AccessNode {
	return testutil.FindAccess(widget.BuildAccessTree(r.d.form, nil), role, name)
}

func TestAccessChildrenHaveUniqueActionIDs(t *testing.T) {
	for _, lang := range langs {
		withScale(t, 1, lang)
		r := newRig(t, testEnv(), testValues(), 1000, 700)
		testutil.RequireActionIDs(t, r.d.form.AccessChildren())
		r.focus(PartPaper)
		r.press(widget.KeySpace)
		testutil.RequireActionIDs(t, r.d.form.AccessChildren())
	}
}

func TestAccessTreeIsNamedInEveryLanguage(t *testing.T) {
	for _, lang := range langs {
		withScale(t, 1, lang)
		r := newRig(t, testEnv(), testValues(), 1000, 700)
		root := widget.BuildAccessTree(r.d.form, nil)
		testutil.RequireNamed(t, root)
		if root.Role != widget.RoleDialog || root.Name != i18n.T("print.title") {
			t.Errorf("%s: root %s %q", lang, root.Role, root.Name)
		}
	}
}

func TestAccessExposesEveryControl(t *testing.T) {
	withScale(t, 1, "en")
	r := newRig(t, testEnv(), testValues(), 1000, 700)
	want := []struct {
		role widget.AccessRole
		name string
	}{
		{widget.RoleComboBox, "Printer"}, {widget.RoleButton, "System dialog…"}, {widget.RoleSpinner, "Copies"},
		{widget.RoleButton, "Fewer copies"}, {widget.RoleButton, "More copies"},
		{widget.RoleComboBox, "Paper"}, {widget.RoleComboBox, "Orientation"}, {widget.RoleComboBox, "Placement"},
		{widget.RoleTextInput, "Width"}, {widget.RoleTextInput, "Height"},
		{widget.RoleRadioButton, "cm"}, {widget.RoleRadioButton, "in"}, {widget.RoleCheckBox, "Keep proportions"},
		{widget.RoleComboBox, "Margins"}, {widget.RoleButton, "Print"}, {widget.RoleButton, "Cancel"},
		{widget.RoleImage, "Page preview"},
	}
	root := widget.BuildAccessTree(r.d.form, nil)
	for _, w := range want {
		if testutil.FindAccess(root, w.role, w.name) == nil {
			t.Errorf("no %s %q in\n%s", w.role, w.name, testutil.DescribeAccess(root))
		}
	}
}

func TestAccessValuesFollowTheDialog(t *testing.T) {
	withScale(t, 1, "en")
	r := newRig(t, testEnv(), testValues(), 1000, 700)
	if n := accessNode(r, widget.RoleComboBox, "Printer"); n == nil || n.Value != "Photo Inkjet" {
		t.Fatalf("%+v", n)
	}
	if n := accessNode(r, widget.RoleComboBox, "Paper"); n.Value != "A4" {
		t.Fatalf("%q", n.Value)
	}
	if n := accessNode(r, widget.RoleSpinner, "Copies"); n.Value != "1" {
		t.Fatalf("%q", n.Value)
	}
	r.choose(PartPaper, "Letter")
	if n := accessNode(r, widget.RoleComboBox, "Paper"); n.Value != "Letter" {
		t.Fatalf("%q", n.Value)
	}
	if n := accessNode(r, widget.RoleImage, "Page preview"); n.Value == "" {
		t.Fatal("the preview must describe itself")
	}
}

func TestAccessStates(t *testing.T) {
	withScale(t, 1, "en")
	r := newRig(t, testEnv(), testValues(), 1000, 700)
	printer := accessNode(r, widget.RoleComboBox, "Printer")
	if !testutil.HasState(printer, widget.StateFocused) || !testutil.HasState(printer, widget.StateHasPopup) || testutil.HasState(printer, widget.StateExpanded) {
		t.Fatalf("%v", printer.States)
	}
	r.press(widget.KeySpace)
	if !testutil.HasState(accessNode(r, widget.RoleComboBox, "Printer"), widget.StateExpanded) {
		t.Fatal("an open list must be expanded")
	}
	r.press(widget.KeyEscape)
	if testutil.HasState(accessNode(r, widget.RoleSpinner, "Copies"), widget.StateDisabled) {
		t.Fatal("copies are enabled for a printer")
	}
	r.choose(PartPrinter, i18n.T("print.dest.pdf"))
	if !testutil.HasState(accessNode(r, widget.RoleSpinner, "Copies"), widget.StateDisabled) {
		t.Fatal("copies must be disabled for PDF")
	}
	if !testutil.HasState(accessNode(r, widget.RoleTextInput, "Width"), widget.StateDisabled) {
		t.Fatal("the size fields are disabled without a custom size")
	}
	if !testutil.HasState(accessNode(r, widget.RoleRadioButton, "cm"), widget.StateChecked) {
		t.Fatal("cm is the current unit")
	}
	if !testutil.HasState(accessNode(r, widget.RoleCheckBox, "Keep proportions"), widget.StateChecked) {
		t.Fatal("keep is checked by default")
	}
}

func TestAccessInvokeOnPrintAndCancelActsLikeAClick(t *testing.T) {
	withScale(t, 1, "en")
	r := newRig(t, testEnv(), testValues(), 1000, 700)
	if !testutil.AccessDo(r.eng, accessNode(r, widget.RoleButton, "Print"), widget.AccessInvoke) {
		t.Fatal("invoke on Print refused")
	}
	if got := r.last(); !got.ok || got.values.Printer != "Photo Inkjet" {
		t.Fatalf("%+v", got)
	}
	r = newRig(t, testEnv(), testValues(), 1000, 700)
	if !testutil.AccessDo(r.eng, accessNode(r, widget.RoleButton, "Cancel"), widget.AccessInvoke) {
		t.Fatal("invoke on Cancel refused")
	}
	if r.last().ok {
		t.Fatal("Cancel reported ok")
	}
}

func TestAccessSelectAndExpandAndItems(t *testing.T) {
	withScale(t, 1, "en")
	r := newRig(t, testEnv(), testValues(), 1000, 700)
	paper := accessNode(r, widget.RoleComboBox, "Paper")
	if !testutil.AccessDo(r.eng, paper, widget.AccessExpand) || !r.d.ListOpen() {
		t.Fatal("expand must open the list")
	}
	item := accessNode(r, widget.RoleMenuItem, "Legal")
	if item == nil || !testutil.HasState(item, widget.StatePopup) {
		t.Fatalf("no list item in the tree:\n%s", testutil.DescribeAccess(widget.BuildAccessTree(r.d.form, nil)))
	}
	if !testutil.AccessDo(r.eng, item, widget.AccessInvoke) {
		t.Fatal("invoke on the item refused")
	}
	if r.d.ListOpen() || r.d.Values().Options.Paper != "legal" {
		t.Fatalf("open=%v paper=%q", r.d.ListOpen(), r.d.Values().Options.Paper)
	}
	paper = accessNode(r, widget.RoleComboBox, "Paper")
	testutil.AccessDo(r.eng, paper, widget.AccessExpand)
	if !testutil.AccessDo(r.eng, accessNode(r, widget.RoleComboBox, "Paper"), widget.AccessCollapse) || r.d.ListOpen() {
		t.Fatal("collapse must close the list")
	}
	if r.eng.AccessAction(r.d.form, itemIDPrefix+"1", widget.AccessInvoke) {
		t.Error("an item of a closed list accepted an action")
	}
	if r.eng.AccessAction(r.d.form, itemIDPrefix+"99", widget.AccessInvoke) || r.eng.AccessAction(r.d.form, itemIDPrefix+"x", widget.AccessInvoke) {
		t.Error("a bad item id was accepted")
	}
}

func TestAccessToggleSelectOnTheCustomControls(t *testing.T) {
	withScale(t, 1, "en")
	r := newRig(t, testEnv(), testValues(), 1000, 700)
	r.choose(PartPlacement, i18n.T("print.place.custom"))
	keep := accessNode(r, widget.RoleCheckBox, "Keep proportions")
	if !testutil.AccessDo(r.eng, keep, widget.AccessToggle) || r.d.Values().Options.Keep {
		t.Fatal("toggle did not clear keep")
	}
	if !testutil.AccessDo(r.eng, accessNode(r, widget.RoleCheckBox, "Keep proportions"), widget.AccessInvoke) || !r.d.Values().Options.Keep {
		t.Fatal("invoke did not set keep")
	}
	if !testutil.AccessDo(r.eng, accessNode(r, widget.RoleRadioButton, "in"), widget.AccessSelect) || r.d.Values().Options.Unit != print.UnitInch {
		t.Fatal("select did not switch to inches")
	}
	if !testutil.AccessDo(r.eng, accessNode(r, widget.RoleRadioButton, "cm"), widget.AccessInvoke) || r.d.Values().Options.Unit != print.UnitCM {
		t.Fatal("invoke did not switch to centimetres")
	}
	if !testutil.AccessDo(r.eng, accessNode(r, widget.RoleButton, "More copies"), widget.AccessInvoke) || r.d.Values().Copies != 2 {
		t.Fatalf("copies %d", r.d.Values().Copies)
	}
	if !testutil.AccessDo(r.eng, accessNode(r, widget.RoleButton, "Fewer copies"), widget.AccessInvoke) || r.d.Values().Copies != 1 {
		t.Fatalf("copies %d", r.d.Values().Copies)
	}
}

func TestAccessFocusMovesTheKeyboardStop(t *testing.T) {
	withScale(t, 1, "en")
	r := newRig(t, testEnv(), testValues(), 1000, 700)
	for _, c := range []struct {
		role widget.AccessRole
		name string
		want Part
	}{
		{widget.RoleComboBox, "Margins", PartMargins},
		{widget.RoleButton, "Cancel", PartCancel},
		{widget.RoleSpinner, "Copies", PartCopies},
		{widget.RoleComboBox, "Paper", PartPaper},
	} {
		if !testutil.AccessDo(r.eng, accessNode(r, c.role, c.name), widget.AccessFocus) {
			t.Fatalf("focus on %q refused", c.name)
		}
		if r.d.Focused() != c.want || !r.d.form.keyNav {
			t.Errorf("focus on %q: stop %v keyNav %v", c.name, r.d.Focused(), r.d.form.keyNav)
		}
	}
}

func TestAccessRefusesDisabledUnknownAndUnsupportedActions(t *testing.T) {
	withScale(t, 1, "en")
	r := newRig(t, testEnv(), testValues(), 1000, 700)
	width := accessNode(r, widget.RoleTextInput, "Width")
	if testutil.AccessDo(r.eng, width, widget.AccessFocus) {
		t.Error("focus accepted on a disabled field")
	}
	if r.eng.AccessAction(r.d.form, "nope", widget.AccessInvoke) || r.eng.AccessAction(r.d.form, "", widget.AccessFocus) {
		t.Error("unknown id accepted")
	}
	printBtn := accessNode(r, widget.RoleButton, "Print")
	for _, k := range []widget.AccessActionKind{widget.AccessToggle, widget.AccessSelect, widget.AccessExpand, widget.AccessCollapse} {
		if testutil.AccessDo(r.eng, printBtn, k) {
			t.Errorf("%s accepted on a button", k)
		}
	}
	paper := accessNode(r, widget.RoleComboBox, "Paper")
	for _, k := range []widget.AccessActionKind{widget.AccessToggle, widget.AccessSelect} {
		if testutil.AccessDo(r.eng, paper, k) {
			t.Errorf("%s accepted on a dropdown", k)
		}
	}
	copies := accessNode(r, widget.RoleSpinner, "Copies")
	if testutil.AccessDo(r.eng, copies, widget.AccessInvoke) {
		t.Error("invoke accepted on a text field")
	}
	r.choose(PartPrinter, i18n.T("print.dest.pdf"))
	if testutil.AccessDo(r.eng, accessNode(r, widget.RoleButton, "System dialog…"), widget.AccessInvoke) {
		t.Error("the system dialog must be refused for PDF")
	}
	if len(r.results) != 0 || len(r.systems) != 0 {
		t.Errorf("refused actions had effects: %+v %+v", r.results, r.systems)
	}
}

func TestAccessInvokeOnAPrinterDropdownOpensTheList(t *testing.T) {
	withScale(t, 1, "en")
	r := newRig(t, testEnv(), testValues(), 1000, 700)
	if !testutil.AccessDo(r.eng, accessNode(r, widget.RoleComboBox, "Printer"), widget.AccessInvoke) || !r.d.ListOpen() {
		t.Fatal("invoke must open the list")
	}
	if !testutil.AccessDo(r.eng, accessNode(r, widget.RoleMenuItem, "Office Laser"), widget.AccessSelect) {
		t.Fatal("select on an item refused")
	}
	if r.d.Values().Printer != "Office Laser" {
		t.Fatalf("%+v", r.d.Values())
	}
}

func TestAccessExportHasNoPrinterNodes(t *testing.T) {
	withScale(t, 1, "en")
	env := testEnv()
	env.Export = true
	r := newRig(t, env, testValues(), 1000, 700)
	root := widget.BuildAccessTree(r.d.form, nil)
	for _, name := range []string{"Printer", "Copies", "System dialog…"} {
		for _, role := range []widget.AccessRole{widget.RoleComboBox, widget.RoleSpinner, widget.RoleButton} {
			if testutil.FindAccess(root, role, name) != nil {
				t.Errorf("export shows %s %q", role, name)
			}
		}
	}
	if root.Name != "Export to PDF" {
		t.Errorf("root %q", root.Name)
	}
	if n := testutil.FindAccess(root, widget.RoleButton, "Save as PDF…"); n == nil {
		t.Error("no save button")
	}
}
