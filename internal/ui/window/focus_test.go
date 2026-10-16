package window

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/commands"
	"github.com/oops1/PixGo/internal/testutil"
	"github.com/oops1/PixGo/internal/ui/theme"
)

func (r *rig) disable(id commands.ID) {
	r.host.reg.Add(commands.Command{ID: id, Title: accessTitles[id], Run: func() {}, Enabled: func() bool { return false }})
}

type fakePanel struct {
	widget.Base
	stops  int
	cur    int
	active bool
	keys   []widget.KeyCode
	closed bool
}

func (p *fakePanel) Draw(c widget.DrawContext) {}

func (p *fakePanel) FocusEnter(dir int) {
	p.active = true
	p.cur = 0
	if dir < 0 {
		p.cur = p.stops - 1
	}
}

func (p *fakePanel) FocusStep(dir int) bool {
	next := p.cur + dir
	if next < 0 || next >= p.stops {
		p.active = false
		return false
	}
	p.cur = next
	return true
}

func (p *fakePanel) FocusLeave()       { p.active = false }
func (p *fakePanel) FocusActive() bool { return p.active }

func (p *fakePanel) HandleKey(e widget.KeyEvent) bool {
	p.keys = append(p.keys, e.Code)
	if e.Code == widget.KeyEscape {
		p.closed = true
		return true
	}
	return false
}

func keyDown(code widget.KeyCode, mod widget.KeyMod) widget.KeyEvent {
	return widget.KeyEvent{Code: code, Mod: mod, Pressed: true}
}

func (r *rig) key(code widget.KeyCode, mod widget.KeyMod) bool {
	return r.root.HandleKey(keyDown(code, mod))
}

func (r *rig) focusTrail(n int, mod widget.KeyMod) []ZoneKind {
	var out []ZoneKind
	for i := 0; i < n; i++ {
		r.key(widget.KeyTab, mod)
		k, _ := r.root.KeyboardFocus()
		out = append(out, k)
	}
	return out
}

func sameZones(a, b []ZoneKind) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

var viewerCycle = []ZoneKind{
	ZoneToolGallery, ZoneToolDelete, ZoneToolRotateLeft, ZoneToolZoomOut, ZoneToolZoomLabel, ZoneToolZoomIn,
	ZoneToolRotateRight, ZoneToolMore, ZoneToolFullscreen, ZonePrev, ZoneNext,
	ZoneBack, ZoneInfo, ZoneFavorite, ZoneMore, ZoneNone,
}

func TestTabWalksEveryViewerControlAndWraps(t *testing.T) {
	r := newAccessRig(t, "en", nil)
	got := r.focusTrail(len(viewerCycle)*2, 0)
	want := append(append([]ZoneKind{}, viewerCycle...), viewerCycle...)
	if !sameZones(got, want) {
		t.Errorf("tab order %v\nwant %v", got, want)
	}
}

func TestShiftTabWalksBackwards(t *testing.T) {
	r := newAccessRig(t, "en", nil)
	got := r.focusTrail(len(viewerCycle), widget.ModShift)
	want := []ZoneKind{ZoneMore, ZoneFavorite, ZoneInfo, ZoneBack, ZoneNext, ZonePrev, ZoneToolFullscreen, ZoneToolMore, ZoneToolRotateRight, ZoneToolZoomIn, ZoneToolZoomLabel, ZoneToolZoomOut, ZoneToolRotateLeft, ZoneToolDelete, ZoneToolGallery, ZoneNone}
	if !sameZones(got, want) {
		t.Errorf("shift-tab order %v\nwant %v", got, want)
	}
}

func TestTabSkipsDisabledControls(t *testing.T) {
	r := newAccessRig(t, "en", nil)
	r.disable("nav.prev")
	for _, k := range r.root.focusStops() {
		if k == ZonePrev {
			t.Error("disabled previous arrow is a tab stop")
		}
	}
}

func TestEnterAndSpaceActivateTheFocusedControl(t *testing.T) {
	for _, code := range []widget.KeyCode{widget.KeyEnter, widget.KeySpace} {
		r := newAccessRig(t, "en", nil)
		r.focusTrail(2, 0)
		if k, _ := r.root.KeyboardFocus(); k != ZoneToolDelete {
			t.Fatalf("focus on %d", k)
		}
		if !r.key(code, 0) {
			t.Errorf("key %d not consumed", code)
		}
		if len(r.host.ran) != 1 || r.host.ran[0] != "file.delete" {
			t.Errorf("key %d ran %v", code, r.host.ran)
		}
	}
}

func TestSpaceOnTheImageStillRunsItsCommand(t *testing.T) {
	r := newAccessRig(t, "en", nil)
	r.host.reg.Add(commands.Command{ID: "slideshow.toggle", Title: "cmd.slideshowPause", Keys: []commands.Chord{commands.C(0, commands.KeySpace)}, Run: func() { r.host.ran = append(r.host.ran, "slideshow.toggle") }})
	r.key(widget.KeySpace, 0)
	if len(r.host.ran) != 1 || r.host.ran[0] != "slideshow.toggle" {
		t.Errorf("space ran %v", r.host.ran)
	}
}

func TestEscapeReturnsFocusToTheImage(t *testing.T) {
	r := newAccessRig(t, "en", nil)
	r.focusTrail(3, 0)
	if !r.key(widget.KeyEscape, 0) {
		t.Fatal("escape not consumed")
	}
	if k, _ := r.root.KeyboardFocus(); k != ZoneNone {
		t.Errorf("focus on %d after escape", k)
	}
}

func TestEscapeInFullscreenIsLeftToTheCommand(t *testing.T) {
	r := newAccessRig(t, "en", func(h *fakeHost) { h.fullscreen = true })
	r.focusTrail(2, 0)
	if r.key(widget.KeyEscape, 0) {
		t.Error("escape consumed while the full screen is on")
	}
}

func TestFullscreenCycleCoversTheSlideshowBar(t *testing.T) {
	r := newAccessRig(t, "en", func(h *fakeHost) { h.fullscreen = true })
	got := r.focusTrail(7, 0)
	want := []ZoneKind{ZoneSlidePlay, ZoneSlideSettings, ZoneSlideExit, ZonePrev, ZoneNext, ZoneExit, ZoneNone}
	if !sameZones(got, want) {
		t.Errorf("fullscreen tab order %v\nwant %v", got, want)
	}
}

func TestMouseClickHidesTheFocusRing(t *testing.T) {
	r := newAccessRig(t, "en", nil)
	r.focusTrail(1, 0)
	if _, kbd := r.root.KeyboardFocus(); !kbd {
		t.Fatal("tab did not start keyboard navigation")
	}
	r.click(300, 300)
	if _, kbd := r.root.KeyboardFocus(); kbd {
		t.Error("keyboard focus still shown after a click")
	}
	r.focusTrail(1, 0)
	r.root.PointerPressed()
	if _, kbd := r.root.KeyboardFocus(); kbd {
		t.Error("pointer observer did not clear the ring")
	}
}

func TestTabWakesTheHiddenToolbarAndKeepsItVisible(t *testing.T) {
	r := newAccessRig(t, "en", func(h *fakeHost) { h.autoHide = true })
	r.root.Wake()
	r.ts.fireAll()
	if r.root.overlayVisible() {
		t.Fatal("overlay did not hide")
	}
	r.key(widget.KeyTab, 0)
	if !r.root.overlayVisible() {
		t.Fatal("tab did not bring the toolbar back")
	}
	r.ts.fireAll()
	if !r.root.overlayVisible() {
		t.Error("toolbar hid while a control had keyboard focus")
	}
	r.key(widget.KeyEscape, 0)
	r.ts.fireAll()
	if r.root.overlayVisible() {
		t.Error("toolbar stayed after the focus returned to the image")
	}
}

func TestPopupMenuOwnsTheKeysWhileOpen(t *testing.T) {
	menu := &openMenu{open: true}
	r := newAccessRig(t, "en", func(h *fakeHost) { h.menu = menu })
	if !r.key(widget.KeyTab, 0) {
		t.Error("tab must be swallowed so it cannot move the engine focus away from the menu")
	}
	for _, code := range []widget.KeyCode{widget.KeyEnter, widget.KeyEscape, widget.KeyDown, widget.KeyLeft} {
		if r.key(code, 0) {
			t.Errorf("key %d consumed while a menu is open", code)
		}
	}
	if len(r.host.ran) != 0 {
		t.Errorf("commands ran behind the menu: %v", r.host.ran)
	}
}

func TestEscapeReachesAnOpenMenuOnTheGalleryPageToo(t *testing.T) {
	r := newAccessRig(t, "en", func(h *fakeHost) { h.page = PageGallery; h.menu = &openMenu{open: true} })
	h := &keyRecorder{}
	r.host.contentOverride = h
	if r.key(widget.KeyEscape, 0) || len(h.keys) != 0 {
		t.Errorf("escape was taken by the page behind the menu: %v", h.keys)
	}
}

func TestFocusReturnsToTheContentAfterTheMenuCloses(t *testing.T) {
	menu := &openMenu{open: true}
	r := newAccessRig(t, "en", func(h *fakeHost) { h.menu = menu })
	r.key(widget.KeyDown, 0)
	menu.open = false
	r.key(widget.KeyDown, 0)
	if r.rec.focused != widget.Widget(r.host.content) {
		t.Errorf("focus went to %T", r.rec.focused)
	}
}

func TestPanelJoinsTheCycleAndKeepsItsOwnStops(t *testing.T) {
	panel := &fakePanel{stops: 3}
	r := newAccessRig(t, "en", func(h *fakeHost) { h.info = true; h.panel = panel })
	want := []ZoneKind{
		ZoneToolGallery, ZoneToolDelete, ZoneToolRotateLeft, ZoneToolZoomOut, ZoneToolZoomLabel, ZoneToolZoomIn,
		ZoneToolRotateRight, ZoneToolMore, ZoneToolFullscreen, ZonePrev, ZoneNext, ZonePanel,
	}
	got := r.focusTrail(len(want), 0)
	if !sameZones(got, want) {
		t.Fatalf("tab order %v\nwant %v", got, want)
	}
	if !panel.active || panel.cur != 0 {
		t.Fatalf("panel not entered: active=%v cur=%d", panel.active, panel.cur)
	}
	r.key(widget.KeyTab, 0)
	r.key(widget.KeyTab, 0)
	if panel.cur != 2 {
		t.Errorf("panel stop %d, want 2", panel.cur)
	}
	r.key(widget.KeyTab, 0)
	if panel.active {
		t.Error("panel kept focus after its last stop")
	}
	if k, _ := r.root.KeyboardFocus(); k != ZoneBack {
		t.Errorf("focus %d after the panel, want back", k)
	}
}

func TestShiftTabEntersThePanelFromTheEnd(t *testing.T) {
	panel := &fakePanel{stops: 3}
	r := newAccessRig(t, "en", func(h *fakeHost) { h.info = true; h.panel = panel })
	r.focusTrail(5, widget.ModShift)
	if k, _ := r.root.KeyboardFocus(); k != ZonePanel || panel.cur != 2 {
		t.Errorf("focus %d, panel stop %d", k, panel.cur)
	}
}

func TestPanelReceivesItsKeysAndClosingReturnsToTheInfoButton(t *testing.T) {
	panel := &fakePanel{stops: 2}
	r := newAccessRig(t, "en", func(h *fakeHost) { h.info = true; h.panel = panel })
	r.root.setFocus(ZonePanel, 1)
	r.key(widget.KeyEnter, 0)
	if len(panel.keys) != 1 || panel.keys[0] != widget.KeyEnter {
		t.Errorf("panel keys %v", panel.keys)
	}
	r.key(widget.KeyEscape, 0)
	if !panel.closed {
		t.Fatal("escape did not reach the panel")
	}
	r.host.panel = nil
	r.host.info = false
	r.root.Refresh()
	if k, kbd := r.root.KeyboardFocus(); k != ZoneInfo || !kbd {
		t.Errorf("focus %d kbd=%v after the panel closed, want the info button", k, kbd)
	}
}

func TestActivatingInfoFromTheKeyboardMovesFocusIntoThePanel(t *testing.T) {
	panel := &fakePanel{stops: 2}
	r := newAccessRig(t, "en", nil)
	r.host.reg.Add(commands.Command{ID: "view.info", Title: "cmd.info", Run: func() { r.host.info, r.host.panel = true, panel }})
	r.root.setFocus(ZoneInfo, 1)
	r.key(widget.KeyEnter, 0)
	if k, _ := r.root.KeyboardFocus(); k != ZonePanel || !panel.active {
		t.Errorf("focus %d, panel active %v", k, panel.active)
	}
}

func TestFocusOnAControlThatDisappearsFallsBackToTheImage(t *testing.T) {
	r := newAccessRig(t, "en", nil)
	r.root.setFocus(ZoneNext, 1)
	r.disable("nav.next")
	r.root.Refresh()
	if k, _ := r.root.KeyboardFocus(); k != ZoneNone {
		t.Errorf("focus %d, want the image", k)
	}
}

func ringPixel(r *rig, p image.Point) color.RGBA {
	r.root.Refresh()
	r.eng.Flush()
	return r.eng.RenderOnce().RGBAAt(p.X, p.Y)
}

func TestFocusRingIsDrawnOnlyForKeyboardFocus(t *testing.T) {
	r := newAccessRig(t, "en", nil)
	r.root.Theme = theme.Dark()
	z := r.root.Layout().Rect(ZoneToolGallery)
	probe := image.Pt(z.Min.X+1, z.Min.Y+z.Dy()/2)
	before := ringPixel(r, probe)
	r.key(widget.KeyTab, 0)
	if k, _ := r.root.KeyboardFocus(); k != ZoneToolGallery {
		t.Fatalf("focus %d", k)
	}
	after := ringPixel(r, probe)
	if want := theme.Dark().Focus; after != want || before == want {
		t.Errorf("ring pixel %v (was %v), want %v", after, before, want)
	}
	r.root.PointerPressed()
	if got := ringPixel(r, probe); got != before {
		t.Errorf("ring left behind after a click: %v", got)
	}
}

func TestFocusRingHasAnInnerLineInHighContrast(t *testing.T) {
	r := newAccessRig(t, "en", nil)
	r.root.Theme = theme.HighContrast()
	z := r.root.Layout().Rect(ZoneToolGallery)
	y := z.Min.Y + z.Dy()/2
	r.key(widget.KeyTab, 0)
	hc := theme.HighContrast()
	for _, x := range []int{z.Min.X - 1, z.Min.X} {
		if got := ringPixel(r, image.Pt(x, y)); got != hc.Focus {
			t.Errorf("outer ring at x=%d is %v, want %v", x, got, hc.Focus)
		}
	}
	if got := ringPixel(r, image.Pt(z.Min.X+1, y)); got != hc.Canvas {
		t.Errorf("inner line %v, want %v", got, hc.Canvas)
	}
}

func TestRingFollowsTheFocusedControlInTheTree(t *testing.T) {
	r := newAccessRig(t, "en", nil)
	r.focusTrail(2, 0)
	var focused []string
	for _, n := range testutil.FlatAccess(accessTree(r)) {
		if testutil.HasState(n, widget.StateFocused) {
			focused = append(focused, n.Name)
		}
	}
	if len(focused) != 1 || focused[0] != "Delete" {
		t.Errorf("focused nodes %v", focused)
	}
}

type keyRecorder struct {
	widget.Base
	keys []widget.KeyCode
}

func (k *keyRecorder) Draw(c widget.DrawContext) {}

func (k *keyRecorder) HandleKey(e widget.KeyEvent) bool {
	k.keys = append(k.keys, e.Code)
	return true
}
