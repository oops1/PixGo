package settings

import (
	"reflect"
	"strings"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"

	data "github.com/oops1/PixGo/internal/settings"
	"github.com/oops1/PixGo/internal/testutil"
)

type settingsState struct {
	Calls []string
	Cfg   data.Settings
	Sec   Section
	Menu  int
	Focus string
	Keys  bool
}

func (r *rig) state() settingsState {
	return settingsState{Calls: append([]string(nil), r.rec.calls...), Cfg: r.src.cfg, Sec: r.page.sec, Menu: len(r.rec.menuItems), Focus: r.page.focus, Keys: r.page.keyNav}
}

func (r *rig) act(id string, kind widget.AccessActionKind) bool {
	return r.eng.AccessAction(r.page, id, kind)
}

func sectionRig(t *testing.T, s Section) *rig {
	t.Helper()
	r := newRig(t, 560, 560)
	setLang("en")
	r.page.SetSection(s)
	r.page.Refresh()
	r.eng.Flush()
	return r
}

func hasState(in widget.AccessInfo, s string) bool {
	for _, st := range in.States {
		if st == s {
			return true
		}
	}
	return false
}

func TestEveryInteractiveSettingsNodeHasAStableUniqueID(t *testing.T) {
	for s := Section(0); s < sectionCount; s++ {
		r := sectionRig(t, s)
		first := r.page.AccessChildren()
		testutil.RequireActionIDs(t, first)
		r.page.Refresh()
		again := r.page.AccessChildren()
		if len(first) != len(again) {
			t.Fatalf("section %d: %d nodes, then %d", s, len(first), len(again))
		}
		for i := range first {
			if first[i].ID != again[i].ID {
				t.Errorf("section %d: id of %q changed: %q, %q", s, first[i].Name, first[i].ID, again[i].ID)
			}
		}
	}
}

func actionFor(in widget.AccessInfo, kinds map[widget.AccessRole]widget.AccessActionKind) widget.AccessActionKind {
	return kinds[in.Role]
}

func TestActionsOnSettingsControlsDoWhatAClickDoes(t *testing.T) {
	kinds := map[widget.AccessRole]widget.AccessActionKind{
		widget.RoleButton:      widget.AccessInvoke,
		widget.RoleCheckBox:    widget.AccessToggle,
		widget.RoleRadioButton: widget.AccessSelect,
		widget.RoleComboBox:    widget.AccessInvoke,
	}
	compared := map[widget.AccessRole]int{}
	for s := Section(0); s < sectionCount; s++ {
		probe := sectionRig(t, s)
		for _, in := range probe.page.AccessChildren() {
			kind := actionFor(in, kinds)
			if kind == 0 || !strings.HasPrefix(in.ID, idItemPref) {
				continue
			}
			clicked := sectionRig(t, s)
			clicked.clickItem(strings.TrimPrefix(in.ID, idItemPref))
			acted := sectionRig(t, s)
			ok := acted.act(in.ID, kind)
			if hasState(in, widget.StateDisabled) {
				if ok || len(acted.rec.calls) != 0 {
					t.Errorf("section %d: %q is disabled but %s gave %v, calls %v", s, in.Name, kind, ok, acted.rec.calls)
				}
				continue
			}
			if !ok {
				t.Errorf("section %d: %s refused on %q (%s)", s, kind, in.Name, in.ID)
				continue
			}
			if want, got := clicked.state(), acted.state(); !reflect.DeepEqual(want, got) {
				t.Errorf("section %d: %q: click %+v, %s %+v", s, in.Name, want, kind, got)
			}
			compared[in.Role]++
		}
	}
	for role, n := range compared {
		if n < 3 {
			t.Errorf("only %d %s nodes compared", n, role)
		}
	}
	for role := range kinds {
		if compared[role] == 0 {
			t.Errorf("no %s node was compared", role)
		}
	}
}

func TestInvokeOnTheSectionListAndBack(t *testing.T) {
	for s := Section(0); s < sectionCount; s++ {
		clicked := sectionRig(t, SectionGeneral)
		clicked.click(navRowRect(s).Min.X+10, navRowRect(s).Min.Y+10)
		for _, kind := range []widget.AccessActionKind{widget.AccessInvoke, widget.AccessSelect} {
			acted := sectionRig(t, SectionGeneral)
			if !acted.act(navID(s), kind) {
				t.Fatalf("%s refused on section %d", kind, s)
			}
			if !reflect.DeepEqual(clicked.state(), acted.state()) {
				t.Errorf("section %d: click %+v, %s %+v", s, clicked.state(), kind, acted.state())
			}
		}
	}
	r := sectionRig(t, SectionGeneral)
	if !r.act(idBack, widget.AccessInvoke) || r.rec.count("back") != 1 {
		t.Errorf("back: calls %v", r.rec.calls)
	}
}

func TestSliderAcceptsFocusOnly(t *testing.T) {
	r := sectionRig(t, SectionPerformance)
	var slider widget.AccessInfo
	for _, in := range r.page.AccessChildren() {
		if in.Role == widget.RoleSlider {
			slider = in
			break
		}
	}
	if slider.ID == "" {
		t.Fatal("no slider")
	}
	for _, k := range []widget.AccessActionKind{widget.AccessInvoke, widget.AccessToggle, widget.AccessSelect, widget.AccessExpand} {
		if r.act(slider.ID, k) {
			t.Errorf("%s accepted on a slider", k)
		}
	}
	if len(r.rec.calls) != 0 {
		t.Errorf("slider actions ran %v", r.rec.calls)
	}
	if !r.act(slider.ID, widget.AccessFocus) || !r.page.keyNav || "item."+r.page.focus != slider.ID {
		t.Errorf("focus %q keyNav %v", r.page.focus, r.page.keyNav)
	}
	for _, n := range testutil.FlatAccess(widget.BuildAccessTree(r.page, nil)) {
		if n.ActionID == slider.ID && !testutil.HasState(n, widget.StateFocused) {
			t.Error("slider node is not marked focused")
		}
	}
}

func TestFocusMovesTheRingToEveryFocusableNode(t *testing.T) {
	for s := Section(0); s < sectionCount; s++ {
		probe := sectionRig(t, s)
		for _, in := range probe.page.AccessChildren() {
			if !testutil.InteractiveRole(in.Role) || hasState(in, widget.StateDisabled) {
				continue
			}
			r := sectionRig(t, s)
			if !r.act(in.ID, widget.AccessFocus) {
				t.Errorf("section %d: focus refused on %q (%s)", s, in.Name, in.ID)
				continue
			}
			if !r.page.keyNav {
				t.Errorf("section %d: focus on %q left the ring off", s, in.Name)
			}
			var focused []string
			for _, n := range testutil.FlatAccess(widget.BuildAccessTree(r.page, nil)) {
				if testutil.HasState(n, widget.StateFocused) {
					focused = append(focused, n.ActionID)
				}
			}
			if len(focused) != 1 || focused[0] != in.ID {
				t.Errorf("section %d: focus on %q marks %v", s, in.ID, focused)
			}
			if len(r.rec.calls) != 0 {
				t.Errorf("section %d: focus on %q ran %v", s, in.ID, r.rec.calls)
			}
		}
	}
}

func TestFocusScrollsAHiddenControlIntoView(t *testing.T) {
	r := newRig(t, 560, 300)
	setLang("en")
	r.page.SetSection(SectionPerformance)
	r.page.Refresh()
	r.eng.Flush()
	var hidden widget.AccessInfo
	for _, in := range r.page.AccessChildren() {
		if testutil.InteractiveRole(in.Role) && in.Bounds.Empty() && strings.HasPrefix(in.ID, idItemPref) {
			hidden = in
		}
	}
	if hidden.ID == "" {
		t.Skip("everything fits")
	}
	if !r.act(hidden.ID, widget.AccessFocus) || r.page.scroll == 0 {
		t.Errorf("focus on the hidden %q: scroll %d", hidden.ID, r.page.scroll)
	}
}

func TestSettingsRefuseUnknownDisabledAndUnsupportedActions(t *testing.T) {
	r := sectionRig(t, SectionFormats)
	disabled := ""
	for _, in := range r.page.AccessChildren() {
		if strings.HasPrefix(in.ID, idItemPref) && hasState(in, widget.StateDisabled) {
			disabled = in.ID
		}
	}
	if disabled == "" {
		t.Fatal("the formats section has no disabled format")
	}
	for _, id := range []string{"", "nope", "nav.", "nav.-1", "nav.7", "nav.01", "nav.x", "item.", "item.nope", "back.0", disabled} {
		for _, k := range []widget.AccessActionKind{widget.AccessInvoke, widget.AccessFocus, widget.AccessToggle, widget.AccessSelect} {
			if r.act(id, k) {
				t.Errorf("%s accepted on %q", k, id)
			}
		}
	}
	for _, k := range []widget.AccessActionKind{widget.AccessToggle, widget.AccessSelect, widget.AccessExpand, widget.AccessCollapse} {
		if r.act(idBack, k) {
			t.Errorf("%s accepted on back", k)
		}
	}
	for _, k := range []widget.AccessActionKind{widget.AccessToggle, widget.AccessInvoke, widget.AccessExpand, widget.AccessCollapse} {
		if r.act(navID(SectionAbout), k) && k != widget.AccessInvoke {
			t.Errorf("%s accepted on a section", k)
		}
	}
	if len(r.rec.calls) != 0 {
		t.Errorf("refused actions ran %v", r.rec.calls)
	}
}

func TestCheckBoxesRefuseSelectAndRadiosRefuseToggle(t *testing.T) {
	r := sectionRig(t, SectionGeneral)
	var check, radio string
	for _, in := range r.page.AccessChildren() {
		switch in.Role {
		case widget.RoleCheckBox:
			check = in.ID
		case widget.RoleRadioButton:
			if strings.HasPrefix(in.ID, idItemPref) {
				radio = in.ID
			}
		}
	}
	if check == "" || radio == "" {
		t.Fatalf("check %q radio %q", check, radio)
	}
	if r.act(check, widget.AccessSelect) || r.act(radio, widget.AccessToggle) || len(r.rec.calls) != 0 {
		t.Errorf("wrong action kind accepted: calls %v", r.rec.calls)
	}
}

func TestExpandOpensADropDown(t *testing.T) {
	r := sectionRig(t, SectionGeneral)
	for _, in := range r.page.AccessChildren() {
		if in.Role != widget.RoleComboBox {
			continue
		}
		if !r.act(in.ID, widget.AccessExpand) || len(r.rec.menuItems) == 0 {
			t.Fatalf("expand on %q: menu %d", in.Name, len(r.rec.menuItems))
		}
		return
	}
	t.Fatal("no drop-down in the general section")
}

func TestActionsAreRefusedWithoutASource(t *testing.T) {
	r := sectionRig(t, SectionGeneral)
	r.page.deps.Source = nil
	if r.act(idBack, widget.AccessInvoke) {
		t.Error("action accepted without a source")
	}
}
