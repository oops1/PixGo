package gallery

import (
	"reflect"
	"strings"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/testutil"
)

type galleryState struct {
	Calls    []string
	Selected int
	Editing  bool
	Menu     int
	Mode     ViewMode
}

func (r *rig) state() galleryState {
	return galleryState{Calls: append([]string(nil), r.rec.calls...), Selected: r.src.selected, Editing: r.page.Editing(), Menu: len(r.rec.menuItems), Mode: r.src.mode}
}

func (r *rig) act(id string, kind widget.AccessActionKind) bool {
	return r.eng.AccessAction(r.page, id, kind)
}

func disabledNode(in widget.AccessInfo) bool {
	for _, s := range in.States {
		if s == widget.StateDisabled {
			return true
		}
	}
	return false
}

func center(in widget.AccessInfo) (int, int) {
	return (in.Bounds.Min.X + in.Bounds.Max.X) / 2, (in.Bounds.Min.Y + in.Bounds.Max.Y) / 2
}

func TestEveryInteractiveGalleryNodeHasAStableID(t *testing.T) {
	cases := map[string]func(*fakeSource){
		"grid":    nil,
		"list":    func(s *fakeSource) { s.mode = ViewList },
		"many":    func(s *fakeSource) { s.items = manyItems(500) },
		"nothing": func(s *fakeSource) { s.items = nil; s.selected = -1 },
	}
	for name, mutate := range cases {
		r := accessRig(t, "en", mutate)
		first := r.page.AccessChildren()
		testutil.RequireActionIDs(t, first)
		r.page.Refresh()
		again := r.page.AccessChildren()
		if len(first) != len(again) {
			t.Fatalf("%s: %d nodes, then %d", name, len(first), len(again))
		}
		for i := range first {
			if first[i].ID != again[i].ID {
				t.Errorf("%s: id of %q changed: %q, %q", name, first[i].Name, first[i].ID, again[i].ID)
			}
		}
	}
	r := accessRig(t, "en", nil)
	r.page.FocusAddress()
	testutil.RequireActionIDs(t, r.page.AccessChildren())
}

func TestInvokeOnAGalleryControlDoesWhatAClickDoes(t *testing.T) {
	probe := accessRig(t, "en", nil)
	checked := 0
	for _, in := range probe.page.AccessChildren() {
		if !testutil.InteractiveRole(in.Role) || strings.HasPrefix(in.ID, idItemPref) || strings.HasPrefix(in.ID, idPlacePref) {
			continue
		}
		clicked := accessRig(t, "en", nil)
		clicked.click(center(in))
		acted := accessRig(t, "en", nil)
		ok := acted.act(in.ID, widget.AccessInvoke)
		if disabledNode(in) {
			if ok {
				t.Errorf("invoke accepted on the disabled %q", in.Name)
			}
			if got := acted.state(); got.Menu != 0 || len(got.Calls) != 0 {
				t.Errorf("disabled %q acted: %+v", in.Name, got)
			}
			continue
		}
		if !ok {
			t.Errorf("invoke refused on %q (%s)", in.Name, in.ID)
			continue
		}
		if want, got := clicked.state(), acted.state(); !reflect.DeepEqual(want, got) {
			t.Errorf("%q: click %+v, invoke %+v", in.Name, want, got)
		}
		checked++
	}
	if checked < 8 {
		t.Errorf("only %d controls were compared", checked)
	}
}

func TestInvokeOnPlacesNavigatesLikeAClick(t *testing.T) {
	probe := accessRig(t, "en", nil)
	rows := 0
	for _, in := range probe.page.AccessChildren() {
		if !strings.HasPrefix(in.ID, idPlacePref) {
			continue
		}
		clicked := accessRig(t, "en", nil)
		clicked.click(center(in))
		acted := accessRig(t, "en", nil)
		ok := acted.act(in.ID, widget.AccessInvoke)
		if ok == disabledNode(in) {
			t.Errorf("place %q: invoke result %v, disabled %v", in.Name, ok, disabledNode(in))
		}
		if !reflect.DeepEqual(clicked.state(), acted.state()) {
			t.Errorf("place %q: click %+v, invoke %+v", in.Name, clicked.state(), acted.state())
		}
		if ok {
			rows++
			if len(acted.rec.calls) != 1 {
				t.Errorf("place %q: calls %v", in.Name, acted.rec.calls)
			}
		}
		sel := accessRig(t, "en", nil)
		if sel.act(in.ID, widget.AccessSelect) != ok {
			t.Errorf("place %q: select and invoke disagree", in.Name)
		}
	}
	if rows < 7 {
		t.Errorf("%d places navigated", rows)
	}
}

func TestSelectAndInvokeOnItemsMatchClickAndDoubleClick(t *testing.T) {
	probe := accessRig(t, "en", nil)
	n := 0
	for _, in := range probe.page.AccessChildren() {
		if !strings.HasPrefix(in.ID, idItemPref) {
			continue
		}
		x, _ := center(in)
		y := in.Bounds.Min.Y + 20
		clicked := accessRig(t, "en", nil)
		clicked.click(x, y)
		selected := accessRig(t, "en", nil)
		if !selected.act(in.ID, widget.AccessSelect) {
			t.Fatalf("select refused on %q", in.Name)
		}
		if !reflect.DeepEqual(clicked.state(), selected.state()) {
			t.Errorf("%q: click %+v, select %+v", in.Name, clicked.state(), selected.state())
		}
		double := accessRig(t, "en", nil)
		double.doubleClick(x, y)
		invoked := accessRig(t, "en", nil)
		if !invoked.act(in.ID, widget.AccessInvoke) {
			t.Fatalf("invoke refused on %q", in.Name)
		}
		if !reflect.DeepEqual(double.state(), invoked.state()) {
			t.Errorf("%q: double click %+v, invoke %+v", in.Name, double.state(), invoked.state())
		}
		n++
	}
	if n != len(mockNames) {
		t.Errorf("%d item nodes", n)
	}
}

func TestInvokeOnTheAddressStartsEditingAndFocusDoesToo(t *testing.T) {
	for _, kind := range []widget.AccessActionKind{widget.AccessInvoke, widget.AccessFocus} {
		r := accessRig(t, "en", nil)
		if !r.act(idAddress, kind) || !r.page.Editing() {
			t.Fatalf("%s did not start editing the address", kind)
		}
		if r.act(idAddress, kind) {
			t.Errorf("%s accepted while the real input is on screen", kind)
		}
	}
}

func TestFocusMovesTheGalleryFocusRing(t *testing.T) {
	want := map[string]stop{
		idBack: stopBack, idUp: stopUp, idFilter: stopFilter, idRefresh: stopRefresh,
		idViewList: stopViews, idViewGrid: stopViews, idThumbs: stopThumbs, idOpen: stopOpen, idMore: stopMore,
		itemID(3): stopGrid, placeID(1): stopNav,
	}
	for id, s := range want {
		r := accessRig(t, "en", nil)
		if !r.act(id, widget.AccessFocus) {
			t.Errorf("focus refused on %s", id)
			continue
		}
		got, kbd := r.page.KeyboardFocus()
		if got != s || !kbd {
			t.Errorf("focus on %s: stop %d keyNav %v, want %d", id, got, kbd, s)
		}
		var focused []string
		for _, n := range testutil.FlatAccess(widget.BuildAccessTree(r.page, nil)) {
			if testutil.HasState(n, widget.StateFocused) {
				focused = append(focused, n.ActionID)
			}
		}
		if len(focused) != 1 {
			t.Errorf("focus on %s marks %v", id, focused)
		}
		if len(r.rec.calls) != 0 && s != stopGrid {
			t.Errorf("focus on %s ran %v", id, r.rec.calls)
		}
	}
	r := accessRig(t, "en", nil)
	r.act(itemID(3), widget.AccessFocus)
	if r.src.selected != 3 {
		t.Errorf("focus on an item left %d selected", r.src.selected)
	}
	r.act(placeID(1), widget.AccessFocus)
	if r.page.navCursor != 1 {
		t.Errorf("place cursor %d", r.page.navCursor)
	}
}

func TestRefusedGalleryActions(t *testing.T) {
	r := accessRig(t, "en", func(s *fakeSource) { s.items = manyItems(500); s.selected = -1 })
	for _, c := range []struct {
		id   string
		kind widget.AccessActionKind
	}{
		{"", widget.AccessInvoke}, {"nope", widget.AccessInvoke}, {"item.", widget.AccessInvoke}, {"item.-1", widget.AccessInvoke},
		{"item.01", widget.AccessInvoke}, {"item.499", widget.AccessInvoke}, {"item.500", widget.AccessInvoke},
		{"place.", widget.AccessInvoke}, {"place.3", widget.AccessInvoke}, {"place.12", widget.AccessInvoke}, {"place.99", widget.AccessInvoke},
		{idForward, widget.AccessInvoke}, {idOpen, widget.AccessInvoke}, {idMore, widget.AccessFocus},
		{idBack, widget.AccessToggle}, {idBack, widget.AccessSelect}, {idBack, widget.AccessExpand}, {idThumbs, widget.AccessCollapse},
		{"item.0", widget.AccessToggle}, {"item.0", widget.AccessExpand}, {"place.1", widget.AccessToggle},
	} {
		if r.act(c.id, c.kind) {
			t.Errorf("%s on %q accepted", c.kind, c.id)
		}
	}
	if len(r.rec.calls) != 0 || r.src.selected != -1 {
		t.Errorf("refused actions changed the state: %v selected %d", r.rec.calls, r.src.selected)
	}
	if !r.act(itemID(0), widget.AccessSelect) || r.src.selected != 0 {
		t.Error("the first visible item cannot be selected")
	}
}

func TestSelectOnTheChosenViewIsAcceptedAndOnTheOtherSwitches(t *testing.T) {
	r := accessRig(t, "en", nil)
	if !r.act(idViewGrid, widget.AccessSelect) || len(r.rec.calls) != 0 {
		t.Errorf("select on the current view: calls %v", r.rec.calls)
	}
	if !r.act(idViewList, widget.AccessSelect) || r.src.mode != ViewList {
		t.Errorf("select on the list view: mode %d", r.src.mode)
	}
}

func TestActionsAreRefusedWithoutASource(t *testing.T) {
	r := accessRig(t, "en", nil)
	r.page.deps.Source = nil
	if r.act(idBack, widget.AccessInvoke) {
		t.Error("action accepted without a source")
	}
}
