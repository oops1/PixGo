package window

import (
	"testing"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/commands"
)

func (h *fakeHost) Editing() bool { return h.editing }

type editKeyContent struct {
	fakeContent
	keys    []widget.KeyCode
	consume map[widget.KeyCode]bool
}

func (k *editKeyContent) HandleKey(e widget.KeyEvent) bool {
	k.keys = append(k.keys, e.Code)
	return k.consume[e.Code]
}

func newEditingRig(t *testing.T) (*rig, *editKeyContent) {
	t.Helper()
	kc := &editKeyContent{consume: map[widget.KeyCode]bool{widget.KeyTab: true, widget.KeyEscape: true}}
	kc.has = true
	r := newRig(t, func(h *fakeHost) {
		h.contentOverride = kc
		h.editing = true
	})
	return r, kc
}

func TestEditingHidesTheToolbarAndTheArrows(t *testing.T) {
	r, _ := newEditingRig(t)
	l := r.root.Layout()
	for _, z := range l.Zones {
		if z.Kind >= ZonePrev && z.Kind <= ZoneToolFullscreen {
			t.Errorf("zone %d is active while editing", z.Kind)
		}
	}
	if full := r.root.fullOverlayLayout(); !full.Rect(ZoneToolEdit).Empty() || !full.Rect(ZonePrev).Empty() {
		t.Error("the full overlay layout still has tool zones")
	}
	if l.Rect(ZoneBack).Empty() || l.Rect(ZoneClose).Empty() {
		t.Error("the title bar must stay")
	}
	r.host.editing = false
	if l := r.root.Layout(); l.Rect(ZoneToolEdit).Empty() || l.Rect(ZonePrev).Empty() {
		t.Error("the toolbar did not come back")
	}
}

func TestEditingForwardsKeysToTheContentFirst(t *testing.T) {
	r, kc := newEditingRig(t)
	r.root.HandleKey(widget.KeyEvent{Code: widget.KeyTab, Pressed: true})
	if len(kc.keys) != 1 || kc.keys[0] != widget.KeyTab {
		t.Fatalf("keys %v", kc.keys)
	}
	if _, kbd := r.root.KeyboardFocus(); kbd {
		t.Fatal("Tab moved the toolbar focus")
	}
}

func TestEditingFallsBackToCommandsForUnhandledKeys(t *testing.T) {
	r, kc := newEditingRig(t)
	r.host.reg.Add(commands.Command{ID: "edit.start", Keys: []commands.Chord{commands.C(commands.Ctrl, 'E')}, Run: func() { r.host.ran = append(r.host.ran, "edit.start") }})
	if !r.root.HandleKey(widget.KeyEvent{Code: widget.KeyCode('E'), Mod: widget.ModCtrl, Pressed: true}) {
		t.Fatal("the chord was not handled")
	}
	if len(kc.keys) != 1 || len(r.host.ran) != 1 || r.host.ran[0] != "edit.start" {
		t.Fatalf("keys %v ran %v", kc.keys, r.host.ran)
	}
}

func TestEditingSwallowsKeysTheContentHandles(t *testing.T) {
	r, _ := newEditingRig(t)
	r.host.reg.Add(commands.Command{ID: "app.escape", Keys: []commands.Chord{commands.C(0, commands.KeyEscape)}, Run: func() { r.host.ran = append(r.host.ran, "app.escape") }})
	r.root.HandleKey(widget.KeyEvent{Code: widget.KeyEscape, Pressed: true})
	if len(r.host.ran) != 0 {
		t.Fatalf("Escape reached the command: %v", r.host.ran)
	}
}

func TestEditingKeepsTheOverlayAndDoesNotArmTheHideTimer(t *testing.T) {
	r, _ := newEditingRig(t)
	r.host.autoHide = true
	r.root.Wake()
	if r.ts.live() != 0 {
		t.Fatalf("%d timers pending", r.ts.live())
	}
	if !r.root.overlayVisible() {
		t.Fatal("overlay hidden while editing")
	}
}

func TestToolbarEditButtonRunsTheCommandAndIsLabelled(t *testing.T) {
	r := newRig(t, nil)
	z := r.root.Layout().Rect(ZoneToolEdit)
	if z.Empty() {
		t.Fatal("no edit button")
	}
	r.click((z.Min.X+z.Max.X)/2, (z.Min.Y+z.Max.Y)/2)
	if len(r.host.ran) != 1 || r.host.ran[0] != "edit.start" {
		t.Fatalf("ran %v", r.host.ran)
	}
}

func TestToolbarStaysCentredWithTheEditButton(t *testing.T) {
	for _, w := range []int{420, 600, 770, 1100, 1920} {
		l := Compute(w, 600, Options{Toolbar: true, Arrows: true})
		if mid := (l.Toolbar.Min.X + l.Toolbar.Max.X) / 2; mid < w/2-1 || mid > w/2+1 {
			t.Errorf("w=%d toolbar centre %d", w, mid)
		}
	}
}

func TestToolButtonsDoNotOverlapAtTheNarrowestWindow(t *testing.T) {
	l := Compute(420, 400, Options{Toolbar: true, Arrows: true})
	var prev ZoneKind
	var prevRect = l.Rect(ZoneToolGallery)
	for k := ZoneToolDelete; k <= ZoneToolFullscreen; k++ {
		r := l.Rect(k)
		if r.Empty() {
			t.Fatalf("zone %d missing", k)
		}
		if k != ZoneToolZoomLabel && prev != ZoneToolZoomLabel && r.Overlaps(prevRect) {
			t.Errorf("zone %d %v overlaps zone %d %v", k, r, prev, prevRect)
		}
		prev, prevRect = k, r
	}
}
