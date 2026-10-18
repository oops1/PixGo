package window

import (
	"fmt"
	"image"
	"reflect"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/commands"
	"github.com/oops1/PixGo/internal/testutil"
)

type zoneEffects struct {
	ran   []string
	ops   []string
	focus ZoneKind
}

func (r *rig) effects() zoneEffects {
	e := zoneEffects{ops: append([]string(nil), r.ops...)}
	for _, id := range r.host.ran {
		e.ran = append(e.ran, string(id))
	}
	e.focus, _ = r.root.KeyboardFocus()
	return e
}

func wireAccessRig(r *rig) {
	r.root.OnMore = func(a image.Rectangle) { r.ops = append(r.ops, fmt.Sprintf("more@%v", a)) }
	r.root.OnSlideSettings = func(a image.Rectangle) { r.ops = append(r.ops, fmt.Sprintf("slideSettings@%v", a)) }
}

func newActionRig(t *testing.T, mutate func(*fakeHost)) *rig {
	r := newAccessRig(t, "en", mutate)
	wireAccessRig(r)
	return r
}

func actionNodes(r *rig) []*widget.AccessNode {
	var out []*widget.AccessNode
	for _, n := range accessTree(r).Children {
		if testutil.InteractiveRole(n.Role) {
			out = append(out, n)
		}
	}
	return out
}

func viewerActionCases() map[string]func(*fakeHost) {
	return map[string]func(*fakeHost){
		"viewer": nil,
		"info":   func(h *fakeHost) { h.info = true },
		"full": func(h *fakeHost) {
			h.fullscreen = true
			h.slide = SlideshowView{Active: true, Playing: true, Index: 3, Total: 24, Progress: 0.4}
		},
	}
}

func TestEveryInteractiveNodeOfTheChromeHasAStableID(t *testing.T) {
	for name, mutate := range viewerActionCases() {
		r := newActionRig(t, mutate)
		testutil.RequireActionIDs(t, r.root.AccessChildren())
		first := r.root.AccessChildren()
		r.root.Refresh()
		again := r.root.AccessChildren()
		for i := range first {
			if first[i].ID != again[i].ID {
				t.Errorf("%s: id of %q changed from %q to %q", name, first[i].Name, first[i].ID, again[i].ID)
			}
		}
	}
	for _, p := range []Page{PageGallery, PageSettings} {
		r := newActionRig(t, func(h *fakeHost) { h.page = p })
		testutil.RequireActionIDs(t, r.root.AccessChildren())
	}
}

func TestInvokeDoesWhatAClickDoes(t *testing.T) {
	for name, mutate := range viewerActionCases() {
		probe := newActionRig(t, mutate)
		nodes := actionNodes(probe)
		if len(nodes) < 6 {
			t.Fatalf("%s: only %d interactive nodes", name, len(nodes))
		}
		for _, n := range nodes {
			clicked := newActionRig(t, mutate)
			clicked.click(n.Bounds.Min.X+n.Bounds.Dx()/2, n.Bounds.Min.Y+n.Bounds.Dy()/2)
			acted := newActionRig(t, mutate)
			node := nodeByActionID(accessTree(acted), n.ActionID)
			if node == nil {
				t.Fatalf("%s: node %q missing in the second rig", name, n.Name)
			}
			ok := testutil.AccessDo(acted.eng, node, widget.AccessInvoke)
			if !ok {
				t.Errorf("%s: invoke on %q refused", name, n.Name)
				continue
			}
			if want, got := clicked.effects(), acted.effects(); !reflect.DeepEqual(want.ran, got.ran) || !reflect.DeepEqual(want.ops, got.ops) {
				t.Errorf("%s: %q: click %+v, invoke %+v", name, n.Name, want, got)
			}
			if len(acted.host.ran)+len(acted.ops) == 0 {
				t.Errorf("%s: invoke on %q did nothing", name, n.Name)
			}
		}
	}
}

func TestToggleActsOnTheCheckBoxesOnly(t *testing.T) {
	r := newActionRig(t, nil)
	tree := accessTree(r)
	info := testutil.FindAccess(tree, widget.RoleCheckBox, "Info")
	fav := testutil.FindAccess(tree, widget.RoleCheckBox, "Favorite")
	if !testutil.AccessDo(r.eng, info, widget.AccessToggle) || !testutil.AccessDo(r.eng, fav, widget.AccessToggle) {
		t.Fatal("toggle refused on a check box")
	}
	if got := fmt.Sprint(r.host.ran); got != "[view.info file.favorite]" {
		t.Errorf("ran %s", got)
	}
	if testutil.AccessDo(r.eng, testutil.FindAccess(tree, widget.RoleButton, "Zoom in"), widget.AccessToggle) {
		t.Error("toggle accepted on a button")
	}
	if len(r.host.ran) != 2 {
		t.Errorf("refused toggle ran a command: %v", r.host.ran)
	}
}

func TestFocusMovesTheKeyboardFocusToTheControl(t *testing.T) {
	for name, mutate := range viewerActionCases() {
		probe := newActionRig(t, mutate)
		stops := probe.root.focusStops()
		if len(stops) < 4 {
			t.Fatalf("%s: stops %v", name, stops)
		}
		for _, k := range stops {
			if k == ZoneNone || k == ZonePanel {
				continue
			}
			r := newActionRig(t, mutate)
			ok := r.eng.AccessAction(r.root, zoneID(k), widget.AccessFocus)
			got, kbd := r.root.KeyboardFocus()
			if !ok || !kbd || got != k {
				t.Errorf("%s: focus on %s: ok %v, focus %v kbd %v", name, zoneID(k), ok, got, kbd)
			}
			node := nodeByActionID(accessTree(r), zoneID(k))
			if node == nil || !testutil.HasState(node, widget.StateFocused) {
				t.Errorf("%s: node of %s is not marked focused: %+v", name, zoneID(k), node)
			}
			if len(r.host.ran)+len(r.ops) != 0 {
				t.Errorf("%s: focus on %s ran something: %v %v", name, zoneID(k), r.host.ran, r.ops)
			}
		}
	}
}

func TestFocusIsRefusedOnWindowButtons(t *testing.T) {
	r := newActionRig(t, nil)
	for _, id := range []string{"minimize", "maximize", "close"} {
		if r.eng.AccessAction(r.root, id, widget.AccessFocus) {
			t.Errorf("focus accepted on %s", id)
		}
	}
}

func TestWindowButtonsInvokeTheWindowOps(t *testing.T) {
	r := newActionRig(t, nil)
	for _, id := range []string{"close", "maximize", "minimize"} {
		if !r.eng.AccessAction(r.root, id, widget.AccessInvoke) {
			t.Errorf("invoke refused on %s", id)
		}
	}
	if got := fmt.Sprint(r.ops); got != "[close max min]" {
		t.Errorf("ops %s", got)
	}
}

func TestUnknownDisabledAndForeignZonesAreRefused(t *testing.T) {
	r := newActionRig(t, func(h *fakeHost) { h.canPrev = false })
	r.root.Actions[ZonePrev] = "app.disabled"
	r.host.reg.Add(commands.Command{ID: "app.disabled", Title: "cmd.prev", Run: func() {}, Enabled: func() bool { return false }})
	for _, id := range []string{"", "nope", "zone.0", "zone.255", "zone.x", "zone.2", "prev", "slide.play", "exit"} {
		if r.eng.AccessAction(r.root, id, widget.AccessInvoke) {
			t.Errorf("id %q accepted", id)
		}
	}
	for _, k := range []widget.AccessActionKind{widget.AccessSelect, widget.AccessExpand, widget.AccessCollapse} {
		if r.eng.AccessAction(r.root, "tool.zoomIn", k) {
			t.Errorf("%s accepted on a button", k)
		}
	}
	if len(r.host.ran)+len(r.ops) != 0 {
		t.Errorf("refused actions ran: %v %v", r.host.ran, r.ops)
	}
}

func TestPagesOtherThanTheViewerRefuseViewerZones(t *testing.T) {
	r := newActionRig(t, func(h *fakeHost) { h.page = PageGallery })
	if r.eng.AccessAction(r.root, "tool.zoomIn", widget.AccessInvoke) || r.eng.AccessAction(r.root, "back", widget.AccessInvoke) {
		t.Error("viewer zone accepted on the gallery page")
	}
	if !r.eng.AccessAction(r.root, "close", widget.AccessInvoke) || len(r.ops) != 1 {
		t.Errorf("close refused on the gallery page: %v", r.ops)
	}
}

func TestSlideshowZonesAreOnlyActiveInFullscreen(t *testing.T) {
	r := newActionRig(t, nil)
	if r.eng.AccessAction(r.root, "slide.play", widget.AccessInvoke) {
		t.Error("slideshow button accepted outside full screen")
	}
	r = newActionRig(t, viewerActionCases()["full"])
	if r.eng.AccessAction(r.root, "tool.zoomIn", widget.AccessInvoke) {
		t.Error("toolbar button accepted in full screen")
	}
	if !r.eng.AccessAction(r.root, "slide.play", widget.AccessInvoke) || len(r.host.ran) != 1 {
		t.Errorf("slide.play: ran %v", r.host.ran)
	}
}

func nodeByActionID(root *widget.AccessNode, id string) *widget.AccessNode {
	var found *widget.AccessNode
	testutil.WalkAccess(root, func(n *widget.AccessNode) {
		if found == nil && n.ActionID == id {
			found = n
		}
	})
	return found
}

func TestFocusPanelEntersTheInfoPanelWithTheKeyboardFocus(t *testing.T) {
	panel := &fakePanel{stops: 3}
	r := newActionRig(t, func(h *fakeHost) { h.info = true; h.panel = panel })
	r.root.FocusPanel()
	if k, kbd := r.root.KeyboardFocus(); k != ZonePanel || !kbd || !panel.active {
		t.Errorf("focus %d kbd %v active %v", k, kbd, panel.active)
	}
	r = newActionRig(t, func(h *fakeHost) { h.page = PageGallery; h.panel = panel })
	r.root.FocusPanel()
	if k, _ := r.root.KeyboardFocus(); k == ZonePanel {
		t.Error("the panel took the focus on another page")
	}
	r = newActionRig(t, nil)
	r.root.FocusPanel()
	if k, kbd := r.root.KeyboardFocus(); kbd || k == ZonePanel {
		t.Error("focus moved without a panel")
	}
}
