package gallery

import (
	"image"
	"slices"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"

	model "github.com/oops1/PixGo/internal/gallery"
	"github.com/oops1/PixGo/internal/ui/theme"
)

func (r *rig) tab(mod widget.KeyMod) stop {
	r.t.Helper()
	if !r.key(widget.KeyTab, mod) {
		r.t.Fatal("tab not consumed")
	}
	s, _ := r.page.KeyboardFocus()
	return s
}

func (r *rig) trail(n int, mod widget.KeyMod) []stop {
	r.t.Helper()
	var out []stop
	for i := 0; i < n; i++ {
		out = append(out, r.tab(mod))
	}
	return out
}

func TestTabWalksTheGalleryControlsAndWraps(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)
	want := []stop{stopOpen, stopMore, stopBack, stopUp, stopAddress, stopFilter, stopRefresh, stopViews, stopThumbs, stopNav, stopGrid}
	got := r.trail(len(want)*2, 0)
	if !slices.Equal(got, append(slices.Clone(want), want...)) {
		t.Errorf("tab order %v\nwant   %v", got, want)
	}
}

func TestShiftTabWalksBackwards(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)
	want := []stop{stopNav, stopThumbs, stopViews, stopRefresh, stopFilter, stopAddress, stopUp, stopBack, stopMore, stopOpen, stopGrid}
	if got := r.trail(len(want), widget.ModShift); !slices.Equal(got, want) {
		t.Errorf("shift-tab order %v\nwant %v", got, want)
	}
}

func TestTabSkipsDisabledAndUnavailableStops(t *testing.T) {
	src := mockSource()
	src.canBack, src.canUp, src.selected = false, false, -1
	src.items = nil
	r := newRig(t, src, mockW, mockH)
	stops := r.page.stops()
	for _, s := range []stop{stopBack, stopForward, stopUp, stopOpen, stopMore, stopGrid} {
		if slices.Contains(stops, s) {
			t.Errorf("stop %d should be skipped", s)
		}
	}
	if !slices.Contains(stops, stopAddress) || !slices.Contains(stops, stopNav) {
		t.Errorf("stops %v", stops)
	}
}

func TestTabIntoTheGridSelectsTheFirstItem(t *testing.T) {
	src := mockSource()
	src.selected = -1
	r := newRig(t, src, mockW, mockH)
	r.rec.reset()
	for s, _ := r.page.KeyboardFocus(); s != stopGrid || !r.page.keyNav; s = r.tab(0) {
	}
	if !r.rec.has("select:0") || src.selected != 0 {
		t.Errorf("grid focus did not select the first item: %v", r.rec.calls)
	}
}

func TestEnterAndSpaceActivateTheFocusedButtons(t *testing.T) {
	cases := []struct {
		name string
		stop stop
		want string
	}{
		{"back", stopBack, "back"},
		{"up", stopUp, "up"},
		{"refresh", stopRefresh, "refresh"},
		{"filter", stopFilter, "menu"},
		{"thumbs", stopThumbs, "menu"},
		{"open", stopOpen, "open:0"},
		{"more", stopMore, "menu"},
	}
	for _, c := range cases {
		for _, code := range []widget.KeyCode{widget.KeyEnter, widget.KeySpace} {
			r := newRig(t, mockSource(), mockW, mockH)
			r.page.setFocus(c.stop)
			r.rec.reset()
			if !r.key(code, 0) {
				t.Errorf("%s: key %d not consumed", c.name, code)
			}
			if !r.rec.has(c.want) {
				t.Errorf("%s: key %d produced %v, want %q", c.name, code, r.rec.calls, c.want)
			}
		}
	}
}

func TestEnterOnTheAddressStartsEditing(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)
	r.page.setFocus(stopAddress)
	r.key(widget.KeyEnter, 0)
	if !r.page.Editing() {
		t.Error("enter did not start editing the address")
	}
}

func TestTabWhileEditingTheAddressLeavesTheField(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)
	r.page.FocusAddress()
	if !r.key(widget.KeyTab, 0) {
		t.Fatal("tab not consumed while editing")
	}
	if r.page.Editing() {
		t.Error("still editing after tab")
	}
	if s, _ := r.page.KeyboardFocus(); s != stopFilter {
		t.Errorf("focus on %d, want the filter button", s)
	}
}

func TestViewToggleBehavesLikeARadioGroup(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)
	r.page.setFocus(stopViews)
	r.rec.reset()
	r.key(widget.KeyLeft, 0)
	if !r.rec.has("mode:1") {
		t.Errorf("left produced %v", r.rec.calls)
	}
	r.rec.reset()
	r.key(widget.KeyRight, 0)
	if !r.rec.has("mode:0") {
		t.Errorf("right produced %v", r.rec.calls)
	}
}

func TestPlacesListMovesWithTheArrowsAndNavigatesOnEnter(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)
	r.page.setFocus(stopNav)
	if got := r.page.navCurrent(); got != 11 {
		t.Fatalf("cursor starts on row %d, want the current place (11)", got)
	}
	r.key(widget.KeyUp, 0)
	if got := r.page.navCurrent(); got != 10 {
		t.Errorf("up moved to row %d, want 10", got)
	}
	r.key(widget.KeyUp, 0)
	if got := r.page.navCurrent(); got != 8 {
		t.Errorf("headers are not skipped: row %d, want 8", got)
	}
	r.key(widget.KeyUp, 0)
	r.rec.reset()
	r.key(widget.KeyEnter, 0)
	if !r.rec.has("navigate:" + model.Folder(`C:\Users\u\Documents`).Display()) {
		t.Errorf("enter produced %v", r.rec.calls)
	}
	r.key(widget.KeyHome, 0)
	if got := r.page.navCurrent(); got != 0 {
		t.Errorf("home moved to row %d", got)
	}
	r.key(widget.KeyEnd, 0)
	if got := r.page.navCurrent(); got != 11 {
		t.Errorf("end moved to row %d, want 11 (the disabled network row is skipped)", got)
	}
}

func TestGridKeysStillWorkWhenTheGridHasFocus(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)
	r.rec.reset()
	r.key(widget.KeyRight, 0)
	r.key(widget.KeyEnter, 0)
	if !r.rec.has("select:1") || !r.rec.has("open:1") {
		t.Errorf("grid keys produced %v", r.rec.calls)
	}
}

func TestDeleteAndTypeAheadOnlyActInTheGrid(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)
	r.page.setFocus(stopThumbs)
	r.rec.reset()
	if r.key(widget.KeyDelete, 0) || r.typeRune('l') {
		t.Error("grid-only keys consumed while a button has focus")
	}
	if len(r.rec.calls) != 0 {
		t.Errorf("calls %v", r.rec.calls)
	}
}

func TestGlobalKeysWorkFromAnyStop(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)
	r.page.setFocus(stopThumbs)
	r.rec.reset()
	r.key(widget.KeyEscape, 0)
	r.key(widget.KeyF5, 0)
	r.key(widget.KeyBackspace, 0)
	if !r.rec.has("close") || !r.rec.has("refresh") || !r.rec.has("up") {
		t.Errorf("global keys produced %v", r.rec.calls)
	}
}

func TestMouseClickMovesFocusAndHidesTheRing(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)
	r.trail(3, 0)
	if _, nav := r.page.KeyboardFocus(); !nav {
		t.Fatal("tab did not start keyboard navigation")
	}
	g := r.page.geo
	x, y := (g.thumbs.Min.X+g.thumbs.Max.X)/2, (g.thumbs.Min.Y+g.thumbs.Max.Y)/2
	r.click(x, y)
	s, nav := r.page.KeyboardFocus()
	if nav || s != stopThumbs {
		t.Errorf("after the click: stop %d keyNav %v", s, nav)
	}
	if got := r.tab(0); got != stopNav {
		t.Errorf("tab after clicking the thumbnail button goes to %d, want the places list", got)
	}
}

func TestClickingAPlaceMakesItTheKeyboardCursor(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)
	r.page.ensureLayout()
	var row navRow
	for _, nr := range r.page.rows {
		if nr.index == 5 {
			row = nr
		}
	}
	rect := row.rect(r.page.geo.sidebar.Min.Y-r.page.sideY, r.page.geo.sidebar.Max.X)
	r.click((rect.Min.X+rect.Max.X)/2, (rect.Min.Y+rect.Max.Y)/2)
	if s, _ := r.page.KeyboardFocus(); s != stopNav || r.page.navCurrent() != 5 {
		t.Errorf("stop %d cursor %d", s, r.page.navCurrent())
	}
}

func ringAt(r *rig, p image.Point) bool {
	img := r.render()
	return img.RGBAAt(p.X, p.Y) == r.th.Focus
}

func TestRingIsDrawnAroundTheFocusedControlOnlyForKeyboardFocus(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)
	r.th = theme.Dark()
	g := r.page.geo
	probe := image.Pt(g.back.Min.X-1, (g.back.Min.Y+g.back.Max.Y)/2)
	if ringAt(r, probe) {
		t.Fatal("ring visible before any key")
	}
	r.page.setFocus(stopBack)
	if !ringAt(r, probe) {
		t.Errorf("no ring at %v after tabbing to the back button", probe)
	}
	r.page.clearKeyNav()
	if ringAt(r, probe) {
		t.Error("ring stayed after the keyboard focus was cleared")
	}
}

func TestRingFollowsTheSelectedTileInTheGrid(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)
	r.page.setFocus(stopGrid)
	cell := r.page.tileRect(r.page.Selected())
	probe := image.Pt(cell.Min.X-1, (cell.Min.Y+cell.Max.Y)/2)
	if !ringAt(r, probe) {
		t.Errorf("no ring beside the selected tile at %v", probe)
	}
}

func TestHighContrastRingHasAnInnerLine(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)
	r.th = theme.HighContrast()
	r.page.setFocus(stopBack)
	g := r.page.geo
	y := (g.back.Min.Y + g.back.Max.Y) / 2
	img := r.render()
	for _, x := range []int{g.back.Min.X - 2, g.back.Min.X - 3} {
		if got := img.RGBAAt(x, y); got != r.th.Focus {
			t.Errorf("outer ring at x=%d is %v, want %v", x, got, r.th.Focus)
		}
	}
	if got := img.RGBAAt(g.back.Min.X-1, y); got != r.th.Canvas {
		t.Errorf("inner line %v, want %v", got, r.th.Canvas)
	}
}

func TestAddressEditingHidesTheRing(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)
	r.page.setFocus(stopAddress)
	r.key(widget.KeyEnter, 0)
	g := r.page.geo
	probe := image.Pt(g.address.Min.X-1, (g.address.Min.Y+g.address.Max.Y)/2)
	if ringAt(r, probe) {
		t.Error("ring drawn while the field is being edited")
	}
}
