package infopanel

import (
	"strings"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/testutil"
)

type panelCalls struct {
	closed, revealed, focused int
	copied, opened            string
}

func wirePanel(p *Panel) *panelCalls {
	c := &panelCalls{}
	p.OnClose = func() { c.closed++ }
	p.OnReveal = func() { c.revealed++ }
	p.OnCopy = func(s string) { c.copied = s }
	p.OnOpenURL = func(s string) { c.opened = s }
	p.OnFocus = func() { c.focused++ }
	return c
}

func (p *Panel) act(id string, kind widget.AccessActionKind) bool {
	return p.eng.(*engine.Engine).AccessAction(p, id, kind)
}

func interactiveInfos(p *Panel) []widget.AccessInfo {
	var out []widget.AccessInfo
	for _, in := range p.AccessChildren() {
		if testutil.InteractiveRole(in.Role) {
			out = append(out, in)
		}
	}
	return out
}

func TestPanelInteractiveNodesHaveStableUniqueIDs(t *testing.T) {
	p := panelIn(t, "en")
	testutil.RequireActionIDs(t, p.AccessChildren())
	first := interactiveInfos(p)
	if len(first) != 4 {
		t.Fatalf("%d interactive nodes, want close, copy, location, map", len(first))
	}
	p.SetModel(realModel("en"))
	for i, in := range interactiveInfos(p) {
		if in.ID != first[i].ID {
			t.Errorf("id of %q changed from %q to %q", in.Name, first[i].ID, in.ID)
		}
	}
}

func TestPanelInvokeDoesWhatAClickDoes(t *testing.T) {
	probe := panelIn(t, "en")
	hs := probe.hits()
	ids := probe.hitIDs()
	if len(hs) != len(ids) {
		t.Fatalf("%d hits, %d ids", len(hs), len(ids))
	}
	for i, id := range ids {
		clicked := panelIn(t, "en")
		cc := wirePanel(clicked)
		h := hs[i]
		x, y := (h.rect.Min.X+h.rect.Max.X)/2, (h.rect.Min.Y+h.rect.Max.Y)/2
		clicked.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft, Pressed: true})
		clicked.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft, Pressed: false})
		acted := panelIn(t, "en")
		ac := wirePanel(acted)
		if !acted.act(id, widget.AccessInvoke) {
			t.Errorf("invoke refused on %s", id)
			continue
		}
		if *cc != *ac {
			t.Errorf("%s: click %+v, invoke %+v", id, *cc, *ac)
		}
		if *ac == (panelCalls{}) {
			t.Errorf("%s: invoke did nothing", id)
		}
		if acted.FocusActive() {
			t.Errorf("%s: invoke moved the keyboard focus like a key would", id)
		}
	}
}

func TestPanelInvokeReachesEveryCallback(t *testing.T) {
	p := panelIn(t, "en")
	c := wirePanel(p)
	for _, in := range interactiveInfos(p) {
		if !p.act(in.ID, widget.AccessInvoke) {
			t.Fatalf("invoke refused on %q", in.Name)
		}
	}
	if c.closed != 1 || c.copied != `D:\Photos\mountain.jpg` || c.revealed != 1 || !strings.HasPrefix(c.opened, "https://www.openstreetmap.org/") {
		t.Errorf("callbacks %+v", *c)
	}
}

func TestPanelFocusPutsTheRingOnTheNode(t *testing.T) {
	p := panelIn(t, "en")
	c := wirePanel(p)
	for _, in := range interactiveInfos(p) {
		if !p.act(in.ID, widget.AccessFocus) {
			t.Fatalf("focus refused on %q", in.Name)
		}
		if !p.FocusActive() || p.hitIDs()[p.cur] != in.ID {
			t.Errorf("%q: active %v cur %d", in.Name, p.FocusActive(), p.cur)
		}
		var focused []string
		for _, n := range testutil.FlatAccess(widget.BuildAccessTree(p, nil)) {
			if testutil.HasState(n, widget.StateFocused) {
				focused = append(focused, n.ActionID)
			}
		}
		if len(focused) != 1 || focused[0] != in.ID {
			t.Errorf("%q: focused nodes %v", in.Name, focused)
		}
	}
	if c.focused != 4 || c.closed+c.revealed != 0 || c.copied != "" || c.opened != "" {
		t.Errorf("focus ran callbacks: %+v", *c)
	}
}

func TestPanelFocusScrollsAHiddenRowIntoView(t *testing.T) {
	m := realModel("en")
	for i := 0; i < 5; i++ {
		m.Cards = append(m.Cards, Card{Kind: CardGPS, Title: "GPS", Rows: []Row{{Label: "Map", Value: "OSM", Link: true, URL: "https://example.org"}}})
	}
	_, p := renderPanel(t, m)
	ids := p.hitIDs()
	last := ids[len(ids)-1]
	if !p.act(last, widget.AccessFocus) {
		t.Fatalf("focus refused on %s", last)
	}
	h, ok := p.focusedHit()
	if !ok || !h.rect.In(p.Bounds()) {
		t.Errorf("focused stop %v outside the panel %v (scroll %d)", h.rect, p.Bounds(), p.ScrollY())
	}
}

func TestPanelRefusesUnknownAndUnsupportedActions(t *testing.T) {
	p := panelIn(t, "en")
	c := wirePanel(p)
	for _, id := range []string{"", "nope", "copy", "copy.9.9", "link.-1.0", "close.0", "label.0.0"} {
		if p.act(id, widget.AccessInvoke) || p.act(id, widget.AccessFocus) {
			t.Errorf("id %q accepted", id)
		}
	}
	for _, k := range []widget.AccessActionKind{widget.AccessToggle, widget.AccessSelect, widget.AccessExpand, widget.AccessCollapse} {
		if p.act("close", k) {
			t.Errorf("%s accepted on the close button", k)
		}
	}
	if *c != (panelCalls{}) || p.FocusActive() {
		t.Errorf("refused actions had effects: %+v active %v", *c, p.FocusActive())
	}
}
