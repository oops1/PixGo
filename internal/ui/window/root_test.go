package window

import (
	"image"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/commands"
	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/testutil"
)

type fakeContent struct {
	widget.Base
	has bool
}

func (f *fakeContent) HasImage() bool { return f.has }

func (f *fakeContent) Draw(c widget.DrawContext) {}

type fakeHost struct {
	reg        *commands.Registry
	content    *fakeContent
	state      ContentState
	autoHide   bool
	fullscreen bool
	info       bool
	canPrev    bool
	canNext    bool
	ran        []commands.ID
	page       Page
	holes      []image.Rectangle
	slide      SlideshowView
	title      string
	zoom       string
	failure    string
	menu       widget.Widget
	preview    bool
	panel      widget.Widget

	contentOverride widget.Widget
	editing         bool
}

func (h *fakeHost) Page() Page                     { return h.page }
func (h *fakeHost) HeaderHoles() []image.Rectangle { return h.holes }

func newHost() *fakeHost {
	h := &fakeHost{reg: commands.NewRegistry(), content: &fakeContent{has: true}, state: StateReady, canPrev: true, canNext: true}
	for _, id := range []commands.ID{"app.back", "view.info", "file.favorite", "app.more", "nav.prev", "nav.next", "view.gallery", "file.delete",
		"view.rotateLeft", "view.zoomOut", "view.zoomToggle", "view.zoomIn", "view.rotateRight", "edit.start", "view.fullscreen", "slideshow.start", "slideshow.toggle"} {
		id := id
		h.reg.Add(commands.Command{ID: id, Run: func() { h.ran = append(h.ran, id) }})
	}
	h.reg.Add(commands.Command{ID: "app.disabled", Run: func() {}, Enabled: func() bool { return false }})
	return h
}

func (h *fakeHost) Registry() *commands.Registry { return h.reg }
func (h *fakeHost) Title() string {
	if h.title != "" {
		return h.title
	}
	return "mountain.jpg — 6000 × 4000"
}
func (h *fakeHost) ZoomText() string {
	if h.zoom != "" {
		return h.zoom
	}
	return "100%"
}
func (h *fakeHost) State() ContentState { return h.state }
func (h *fakeHost) FailureText() string { return h.failure }
func (h *fakeHost) Favorite() bool      { return false }
func (h *fakeHost) InfoOpen() bool      { return h.info }
func (h *fakeHost) CanPrev() bool       { return h.canPrev }
func (h *fakeHost) CanNext() bool       { return h.canNext }
func (h *fakeHost) Content() widget.Widget {
	if h.contentOverride != nil {
		return h.contentOverride
	}
	return h.content
}
func (h *fakeHost) Panel() widget.Widget     { return h.panel }
func (h *fakeHost) Menu() widget.Widget      { return h.menu }
func (h *fakeHost) AutoHide() bool           { return h.autoHide }
func (h *fakeHost) Fullscreen() bool         { return h.fullscreen }
func (h *fakeHost) Slideshow() SlideshowView { return h.slide }
func (h *fakeHost) Preview() bool            { return h.preview }

type timers struct {
	pending []*timer
}

type timer struct {
	fn      func()
	stopped bool
}

func (ts *timers) after(d time.Duration, fn func()) func() {
	t := &timer{fn: fn}
	ts.pending = append(ts.pending, t)
	return func() { t.stopped = true }
}

func (ts *timers) fireAll() {
	for _, t := range append([]*timer(nil), ts.pending...) {
		if !t.stopped {
			t.stopped = true
			t.fn()
		}
	}
}

func (ts *timers) live() int {
	n := 0
	for _, t := range ts.pending {
		if !t.stopped {
			n++
		}
	}
	return n
}

type recordingEngine struct {
	*engine.Engine
	rects   []image.Rectangle
	full    int
	focused widget.Widget
}

func (r *recordingEngine) SetFocus(w widget.Widget) {
	r.focused = w
	r.Engine.SetFocus(w)
}

func (r *recordingEngine) InvalidateRect(rc image.Rectangle) {
	r.rects = append(r.rects, rc)
	r.Engine.InvalidateRect(rc)
}

func (r *recordingEngine) Invalidate() {
	r.full++
	r.Engine.Invalidate()
}

func (r *recordingEngine) reset() { r.rects, r.full = nil, 0 }

type rig struct {
	t     *testing.T
	eng   *engine.Engine
	rec   *recordingEngine
	root  *Root
	host  *fakeHost
	ts    *timers
	now   time.Time
	ops   []string
	moved []image.Point
}

func newRig(t *testing.T, mutate func(*fakeHost)) *rig {
	t.Helper()
	return newSizedRig(t, 770, 504, mutate)
}

func newSizedRig(t *testing.T, w, hgt int, mutate func(*fakeHost)) *rig {
	t.Helper()
	i18n.SetLang("ru")
	h := newHost()
	if mutate != nil {
		mutate(h)
	}
	eng := testutil.NewEngine(w, hgt)
	t.Cleanup(eng.Stop)
	rec := &recordingEngine{Engine: eng}
	r := &rig{t: t, eng: eng, rec: rec, host: h, ts: &timers{}, now: time.Unix(100, 0)}
	r.root = New(h, rec, WindowOps{
		Minimize:       func() { r.ops = append(r.ops, "min") },
		ToggleMaximize: func() { r.ops = append(r.ops, "max") },
		Close:          func() { r.ops = append(r.ops, "close") },
		MoveBy:         func(dx, dy int) { r.moved = append(r.moved, image.Pt(dx, dy)) },
		BeginResize:    func(e int) bool { r.ops = append(r.ops, "resize"); return true },
	})
	r.root.After = r.ts.after
	r.root.Now = func() time.Time { return r.now }
	eng.SetRootFullCanvas(r.root)
	eng.Flush()
	eng.RenderOnce()
	return r
}

func (r *rig) click(x, y int) {
	e := widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft, Clicks: 1}
	e.Pressed = true
	r.root.OnMouseButton(e)
	e.Pressed = false
	r.root.OnMouseButton(e)
}

func TestCaptionButtonsInvokeOps(t *testing.T) {
	r := newRig(t, nil)
	r.click(741, 24)
	r.click(693, 24)
	r.click(645, 24)
	want := []string{"close", "max", "min"}
	if len(r.ops) != 3 {
		t.Fatalf("ops = %v, want %v", r.ops, want)
	}
	for i := range want {
		if r.ops[i] != want[i] {
			t.Fatalf("ops = %v, want %v", r.ops, want)
		}
	}
}

func TestToolbarButtonsRunCommands(t *testing.T) {
	r := newRig(t, nil)
	r.click(178, 466)
	r.click(301, 466)
	r.click(410, 466)
	r.click(355, 466)
	r.click(646, 466)
	want := []commands.ID{"file.delete", "view.zoomOut", "view.zoomIn", "view.zoomToggle", "view.fullscreen"}
	if len(r.host.ran) != len(want) {
		t.Fatalf("ran %v, want %v", r.host.ran, want)
	}
	for i := range want {
		if r.host.ran[i] != want[i] {
			t.Fatalf("ran %v, want %v", r.host.ran, want)
		}
	}
}

func TestArrowsNavigate(t *testing.T) {
	r := newRig(t, nil)
	r.click(37, 258)
	r.click(731, 258)
	if len(r.host.ran) != 2 || r.host.ran[0] != "nav.prev" || r.host.ran[1] != "nav.next" {
		t.Errorf("ran %v", r.host.ran)
	}
}

func TestReleaseOutsideDoesNotClick(t *testing.T) {
	r := newRig(t, nil)
	e := widget.MouseEvent{X: 206, Y: 466, Button: widget.MouseLeft, Clicks: 1, Pressed: true}
	r.root.OnMouseButton(e)
	e.X, e.Y, e.Pressed = 300, 300, false
	r.root.OnMouseButton(e)
	if len(r.host.ran) != 0 {
		t.Errorf("command ran after release outside: %v", r.host.ran)
	}
}

func TestDisabledCommandDoesNothing(t *testing.T) {
	r := newRig(t, nil)
	r.root.Actions[ZoneToolDelete] = "app.disabled"
	r.click(178, 466)
	if len(r.host.ran) != 0 {
		t.Errorf("disabled command ran: %v", r.host.ran)
	}
}

func TestTitleDragMovesAndDoubleClickMaximises(t *testing.T) {
	r := newRig(t, nil)
	e := widget.MouseEvent{X: 300, Y: 24, Button: widget.MouseLeft, Clicks: 1, Pressed: true}
	r.root.OnMouseButton(e)
	r.root.OnMouseMove(320, 30)
	r.root.OnMouseMove(340, 40)
	e.Pressed = false
	r.root.OnMouseButton(e)
	if len(r.moved) != 2 || r.moved[1] != image.Pt(40, 16) {
		t.Errorf("moves = %v", r.moved)
	}
	r.now = r.now.Add(2 * time.Second)
	e.Pressed = true
	r.root.OnMouseButton(e)
	e.Pressed = false
	r.root.OnMouseButton(e)
	r.now = r.now.Add(100 * time.Millisecond)
	e.Pressed = true
	r.root.OnMouseButton(e)
	if len(r.ops) != 1 || r.ops[0] != "max" {
		t.Errorf("ops = %v, want [max]", r.ops)
	}
}

func TestResizeBandCallsBeginResize(t *testing.T) {
	r := newRig(t, nil)
	r.root.OnMouseButton(widget.MouseEvent{X: 769, Y: 250, Button: widget.MouseLeft, Pressed: true, Clicks: 1})
	if len(r.ops) != 1 || r.ops[0] != "resize" {
		t.Errorf("ops = %v", r.ops)
	}
}

func TestOverlayAutoHidesAndWakes(t *testing.T) {
	r := newRig(t, func(h *fakeHost) { h.autoHide = true })
	r.root.Wake()
	if !r.root.overlayVisible() || r.ts.live() != 1 {
		t.Fatalf("visible=%v timers=%d", r.root.overlayVisible(), r.ts.live())
	}
	r.ts.fireAll()
	if r.root.overlayVisible() {
		t.Fatal("overlay still visible after the idle timer")
	}
	if got := r.root.Layout().ZoneAt(image.Pt(178, 466)).Kind; got != ZoneNone {
		t.Errorf("hidden toolbar still hit-testable: zone %d", got)
	}
	r.root.OnMouseMove(300, 300)
	if !r.root.overlayVisible() {
		t.Fatal("mouse move did not bring the overlay back")
	}
	if r.ts.live() != 1 {
		t.Errorf("hide timer not re-armed: %d", r.ts.live())
	}
}

func TestOverlayStaysWhileHovered(t *testing.T) {
	r := newRig(t, func(h *fakeHost) { h.autoHide = true })
	r.root.OnMouseMove(178, 466)
	r.ts.fireAll()
	if !r.root.overlayVisible() {
		t.Fatal("overlay hid while the pointer was over the toolbar")
	}
	r.root.OnMouseMove(300, 300)
	r.ts.fireAll()
	if r.root.overlayVisible() {
		t.Fatal("overlay did not hide after the pointer left")
	}
}

func TestNoAutoHideKeepsOverlay(t *testing.T) {
	r := newRig(t, nil)
	r.root.Wake()
	if r.ts.live() != 0 {
		t.Errorf("timer armed although auto-hide is off")
	}
	if !r.root.overlayVisible() {
		t.Error("overlay hidden with auto-hide off")
	}
}

func TestFullscreenAlwaysAutoHides(t *testing.T) {
	r := newRig(t, func(h *fakeHost) { h.fullscreen = true })
	r.root.Wake()
	if r.ts.live() != 1 {
		t.Errorf("fullscreen did not arm the hide timer")
	}
}

func TestArrowsAbsentForSingleImage(t *testing.T) {
	r := newRig(t, func(h *fakeHost) { h.canPrev, h.canNext = false, false })
	l := r.root.Layout()
	if l.Rect(ZonePrev) != (image.Rectangle{}) || l.Rect(ZoneNext) != (image.Rectangle{}) {
		t.Error("arrow zones present with a single image")
	}
}

func TestKeyRunsCommandOncePerKeyAndRune(t *testing.T) {
	r := newRig(t, nil)
	r.host.reg.Add(commands.Command{ID: "t.open", Keys: []commands.Chord{commands.C(commands.Ctrl, 'O')}, Run: func() { r.host.ran = append(r.host.ran, "t.open") }})
	r.root.HandleKey(widget.KeyEvent{Code: widget.KeyO, Mod: widget.ModCtrl, Pressed: true})
	r.root.HandleKey(widget.KeyEvent{Rune: 'o', Mod: widget.ModCtrl, Pressed: true})
	if len(r.host.ran) != 1 {
		t.Errorf("ran %v, want exactly one", r.host.ran)
	}
	r.root.HandleKey(widget.KeyEvent{Rune: 'щ', Mod: widget.ModCtrl, Pressed: true})
	if len(r.host.ran) != 2 {
		t.Errorf("ran %v, want a second run for the Cyrillic layout", r.host.ran)
	}
}

func TestDefaultActionsPointToRegisteredZones(t *testing.T) {
	for k, id := range DefaultActions() {
		if k == ZoneNone || id == "" {
			t.Errorf("bad action entry %d -> %q", k, id)
		}
	}
}

func TestHitTestUsesLastDrawnZones(t *testing.T) {
	r := newRig(t, nil)
	r.eng.Flush()
	r.eng.RenderOnce()
	cases := map[[2]int]int{{741, 24}: HitCloseButton, {693, 24}: HitMaxButton, {645, 24}: HitMinButton, {300, 24}: HitCaption, {300, 300}: HitClient}
	for p, want := range cases {
		if got := r.root.HitTest(p[0], p[1]); got != want {
			t.Errorf("HitTest%v = %d, want %d", p, got, want)
		}
	}
}

func TestFullscreenHidesCursorWhenIdleAndRestoresOnMove(t *testing.T) {
	r := newRig(t, func(h *fakeHost) { h.fullscreen = true })
	r.root.Wake()
	r.ts.fireAll()
	if got := r.eng.CursorAt(300, 300); got != widget.CursorNone {
		t.Fatalf("cursor after idle = %v, want none", got)
	}
	r.root.OnMouseMove(310, 300)
	if got := r.eng.CursorAt(300, 300); got == widget.CursorNone {
		t.Fatalf("cursor still hidden after the mouse moved")
	}
}

func TestWindowedModeNeverHidesCursor(t *testing.T) {
	r := newRig(t, func(h *fakeHost) { h.autoHide = true })
	r.root.Wake()
	r.ts.fireAll()
	if got := r.eng.CursorAt(300, 300); got == widget.CursorNone {
		t.Fatal("cursor hidden outside fullscreen")
	}
}

func TestSideButtonsNavigate(t *testing.T) {
	r := newRig(t, nil)
	r.root.OnMouseButton(widget.MouseEvent{X: 300, Y: 300, Button: widget.MouseButtonX1, Pressed: true})
	r.root.OnMouseButton(widget.MouseEvent{X: 300, Y: 300, Button: widget.MouseButtonX2, Pressed: true})
	r.root.OnMouseButton(widget.MouseEvent{X: 300, Y: 300, Button: widget.MouseButtonX2, Pressed: false})
	if len(r.host.ran) != 2 || r.host.ran[0] != "nav.prev" || r.host.ran[1] != "nav.next" {
		t.Errorf("ran %v, want [nav.prev nav.next]", r.host.ran)
	}
}

func TestExitButtonOnlyInFullscreen(t *testing.T) {
	r := newRig(t, func(h *fakeHost) { h.fullscreen = true })
	l := r.root.Layout()
	if l.Rect(ZoneExit).Empty() {
		t.Fatal("no exit button in fullscreen")
	}
	c := image.Pt((l.Rect(ZoneExit).Min.X+l.Rect(ZoneExit).Max.X)/2, (l.Rect(ZoneExit).Min.Y+l.Rect(ZoneExit).Max.Y)/2)
	r.click(c.X, c.Y)
	if len(r.host.ran) != 1 || r.host.ran[0] != "view.fullscreen" {
		t.Errorf("ran %v", r.host.ran)
	}
	r2 := newRig(t, nil)
	if !r2.root.Layout().Rect(ZoneExit).Empty() {
		t.Error("exit button in windowed mode")
	}
}

func TestTooltipAppearsAfterDelayWithShortcutAndHidesOnLeave(t *testing.T) {
	r := newRig(t, nil)
	r.host.reg.Add(commands.Command{ID: "view.rotateLeft", Title: "cmd.rotateLeft", Keys: []commands.Chord{commands.C(commands.Shift, 'R')}, Run: func() {}})
	r.root.OnMouseMove(245, 467)
	if r.root.tip.text != "" {
		t.Fatalf("tooltip shown before the delay: %q", r.root.tip.text)
	}
	r.ts.fireAll()
	want := "Повернуть влево (Shift+R)"
	if r.root.tip.text != want {
		t.Fatalf("tooltip = %q, want %q", r.root.tip.text, want)
	}
	rect := r.root.tipRect(nil)
	if rect.Empty() || rect.Max.Y >= r.root.fullOverlayLayout().Toolbar.Min.Y {
		t.Errorf("tooltip rect %v overlaps the toolbar", rect)
	}
	r.root.OnMouseMove(300, 300)
	if r.root.tip.text != "" {
		t.Errorf("tooltip still shown after leaving: %q", r.root.tip.text)
	}
}

func TestTooltipForCaptionButtonAndHiddenOnPress(t *testing.T) {
	r := newRig(t, nil)
	r.root.OnMouseMove(741, 24)
	r.ts.fireAll()
	if r.root.tip.text != "Закрыть" {
		t.Fatalf("caption tooltip = %q", r.root.tip.text)
	}
	if rect := r.root.tipRect(nil); rect.Min.Y <= TitleHeight {
		t.Errorf("title tooltip %v not below the title bar", rect)
	}
	r.root.OnMouseButton(widget.MouseEvent{X: 741, Y: 24, Button: widget.MouseLeft, Pressed: true, Clicks: 1})
	if r.root.tip.text != "" {
		t.Errorf("tooltip survived a press")
	}
}
