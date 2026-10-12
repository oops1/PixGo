package gallery

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

func longRig(t *testing.T, n int) *rig {
	t.Helper()
	src := mockSource()
	src.items = manyItems(n)
	return newRig(t, src, mockW, mockH)
}

func TestShortFolderHasNothingToScroll(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)

	r.page.OnMouseWheelPixelsMod(300, 200, 0, 400, 0)

	if r.page.scroll != 0 {
		t.Fatalf("scroll = %d", r.page.scroll)
	}
	if _, _, ok := r.page.gridBarRects(); ok {
		t.Fatal("no scrollbar without overflow")
	}
}

func TestWheelNotchScrollsAThirdOfARow(t *testing.T) {
	r := longRig(t, 60)

	handled := r.page.OnMouseWheelPixelsMod(300, 200, 0, wheelNotchPixels, 0)

	if !handled || r.page.scroll != 34 {
		t.Fatalf("handled = %v, scroll = %d, want 34", handled, r.page.scroll)
	}
}

func TestWheelIsLimitedToTheContent(t *testing.T) {
	r := longRig(t, 60)

	r.page.OnMouseWheelPixelsMod(300, 200, 0, 1e6, 0)
	if r.page.scroll != r.page.grid.MaxScroll() {
		t.Fatalf("scroll = %d, max = %d", r.page.scroll, r.page.grid.MaxScroll())
	}
	r.page.OnMouseWheelPixelsMod(300, 200, 0, -1e6, 0)
	if r.page.scroll != 0 {
		t.Fatalf("scroll = %d", r.page.scroll)
	}
}

func TestWheelTicksFromButtonsScrollToo(t *testing.T) {
	r := longRig(t, 60)

	r.page.OnMouseButton(widget.MouseEvent{X: 300, Y: 200, Button: widget.MouseWheelDown, Pressed: true})
	r.page.OnMouseButton(widget.MouseEvent{X: 300, Y: 200, Button: widget.MouseWheelDown, Pressed: false})
	if r.page.scroll != 34 {
		t.Fatalf("scroll = %d after one tick", r.page.scroll)
	}
	handled := r.page.OnMouseButton(widget.MouseEvent{X: 300, Y: 200, Button: widget.MouseWheelUp, Pressed: true})
	if !handled || r.page.scroll != 0 {
		t.Fatalf("handled = %v, scroll = %d", handled, r.page.scroll)
	}
}

func TestFractionalWheelDeltasAccumulate(t *testing.T) {
	r := longRig(t, 60)

	for i := 0; i < 4; i++ {
		r.page.OnMouseWheelPixelsMod(300, 200, 0, 4, 0)
	}

	if r.page.scroll != 13 {
		t.Fatalf("scroll = %d, want 13 (4 x 3.4 px)", r.page.scroll)
	}
}

func TestWheelOverTheHeaderIsNotConsumed(t *testing.T) {
	r := longRig(t, 60)

	if r.page.OnMouseWheelPixelsMod(200, 20, 0, 40, 0) {
		t.Fatal("the header does not scroll")
	}
	if r.page.OnMouseWheelPixelsMod(200, 20, 0, 0, 0) {
		t.Fatal("zero delta")
	}
}

func TestCtrlWheelChangesTheThumbnailSize(t *testing.T) {
	src := mockSource()
	src.side = 128
	r := newRig(t, src, mockW, mockH)

	r.page.OnMouseWheelPixelsMod(300, 200, 0, -wheelNotchPixels, widget.ModCtrl)
	if !r.rec.has("size:160") {
		t.Fatalf("calls = %v", r.rec.calls)
	}
	r.page.OnMouseButton(widget.MouseEvent{X: 300, Y: 200, Button: widget.MouseWheelDown, Pressed: true, Mod: widget.ModCtrl})
	if r.rec.count("size:128") != 1 {
		t.Fatalf("calls = %v", r.rec.calls)
	}
}

func TestCtrlWheelAtTheLimitsDoesNothing(t *testing.T) {
	src := mockSource()
	src.side = 96
	r := newRig(t, src, mockW, mockH)

	r.page.OnMouseWheelPixelsMod(300, 200, 0, wheelNotchPixels, widget.ModCtrl)

	if len(r.rec.calls) != 0 {
		t.Fatalf("calls = %v", r.rec.calls)
	}
}

func TestScrollbarAppearsOnScrollAndHidesAfterADelay(t *testing.T) {
	r := longRig(t, 60)
	if r.page.gridBar.shown {
		t.Fatal("the bar starts hidden")
	}

	r.page.OnMouseWheelPixelsMod(300, 200, 0, 40, 0)
	if !r.page.gridBar.shown {
		t.Fatal("scrolling must show the bar")
	}
	r.render()
	r.tm.advance(barHideDelay)

	if r.page.gridBar.shown {
		t.Fatal("the bar must hide again")
	}
}

func TestHoveringTheRightEdgeShowsTheBar(t *testing.T) {
	r := longRig(t, 60)

	r.page.OnMouseMove(588, 200)

	if !r.page.gridBar.shown {
		t.Fatal("the bar must appear under the pointer")
	}
}

func TestScrollbarThumbTracksTheScrollPosition(t *testing.T) {
	r := longRig(t, 60)
	track, top, ok := r.page.gridBarRects()
	if !ok || top.Min.Y != track.Min.Y {
		t.Fatalf("thumb %v in track %v", top, track)
	}

	r.page.setScroll(r.page.grid.MaxScroll())
	_, bottom, _ := r.page.gridBarRects()

	if bottom.Max.Y != track.Max.Y {
		t.Fatalf("thumb %v must end at the track end %v", bottom, track)
	}
	if bottom.Dy() < 24 {
		t.Fatalf("thumb height %d", bottom.Dy())
	}
}

func TestDraggingTheThumbScrollsTheContent(t *testing.T) {
	r := longRig(t, 60)
	r.page.OnMouseMove(588, 200)
	track, thumb, _ := r.page.gridBarRects()
	x, y := thumb.Min.X+1, thumb.Min.Y+4
	r.page.OnMouseMove(x, y)

	r.page.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft, Pressed: true})
	if !r.page.WantsCapture(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft, Pressed: true}) {
		t.Fatal("the bar press must be captured")
	}
	if r.page.gridBar.drag == nil {
		t.Fatal("drag must start on the thumb")
	}
	r.page.OnMouseMove(x, track.Min.Y+(track.Dy()-thumb.Dy())/2+4)
	half := r.page.scroll
	r.page.OnMouseMove(x, track.Max.Y+200)
	end := r.page.scroll
	r.page.OnMouseButton(widget.MouseEvent{X: x, Y: track.Max.Y, Button: widget.MouseLeft, Pressed: false})

	maxScroll := r.page.grid.MaxScroll()
	if half < maxScroll/2-10 || half > maxScroll/2+10 {
		t.Fatalf("half-way drag scrolled to %d of %d", half, maxScroll)
	}
	if end != maxScroll {
		t.Fatalf("dragging past the end scrolled to %d of %d", end, maxScroll)
	}
	if r.page.gridBar.drag != nil {
		t.Fatal("the drag must end on release")
	}
	r.page.OnMouseMove(x, track.Min.Y-300)
	if r.page.scroll != maxScroll {
		t.Fatal("moves after the release must not scroll")
	}
}

func TestScrollToRevealsAnItem(t *testing.T) {
	r := longRig(t, 60)

	r.page.ScrollTo(40)

	if !r.page.tileRect(40).In(r.page.geo.grid) {
		t.Fatalf("tile 40 at %v, viewport %v", r.page.tileRect(40), r.page.geo.grid)
	}
	r.page.ScrollTo(-1)
	r.page.ScrollTo(1000)
}

func TestVisibleRangeIsReportedOnChange(t *testing.T) {
	r := longRig(t, 60)
	if len(r.rec.visible) == 0 || r.rec.visible[len(r.rec.visible)-1] != [2]int{0, 16} {
		t.Fatalf("initial range = %v", r.rec.visible)
	}
	before := len(r.rec.visible)

	r.page.setScroll(r.page.scroll)
	if len(r.rec.visible) != before {
		t.Fatal("an unchanged range must not be reported again")
	}
	r.page.OnMouseWheelPixelsMod(300, 200, 0, wheelNotchPixels*4, 0)

	got := r.rec.visible[len(r.rec.visible)-1]
	first, last := r.page.grid.Visible(r.page.scroll)
	if got != [2]int{first, last} || first == 0 {
		t.Fatalf("reported %v, grid says %d..%d", got, first, last)
	}
}

func TestVisibleRangeFollowsResizing(t *testing.T) {
	r := longRig(t, 60)
	before := len(r.rec.visible)

	r.page.SetBounds(image.Rect(0, 0, 593, 700))

	if len(r.rec.visible) == before {
		t.Fatalf("a taller window shows more rows: %v", r.rec.visible)
	}
}

func shortRig(t *testing.T, n int) *rig {
	t.Helper()
	src := mockSource()
	if n != len(src.items) {
		src.items = manyItems(n)
	}
	return newRig(t, src, mockW, 300)
}

func selectedNavRow(r *rig) navRow {
	for _, row := range r.page.rows {
		if row.entry.Selected {
			return row
		}
	}
	return navRow{}
}

func TestSidebarOpensScrolledToTheSelectedEntry(t *testing.T) {
	r := shortRig(t, 12)

	row := selectedNavRow(r)
	pill := row.rect(r.page.geo.sidebar.Min.Y-r.page.sideY, sidebarWidth)

	if r.page.sideY == 0 || !pill.In(r.page.geo.sidebar) {
		t.Fatalf("sideY = %d, selected pill %v, panel %v", r.page.sideY, pill, r.page.geo.sidebar)
	}
}

func TestSidebarWheelStepMatchesTheGridStep(t *testing.T) {
	r := shortRig(t, 12)
	r.page.sideY = 0

	r.page.OnMouseWheelPixelsMod(60, 150, 0, wheelNotchPixels, 0)

	if r.page.sideY != 34 {
		t.Fatalf("sideY = %d, want 34", r.page.sideY)
	}
	r.page.OnMouseButton(widget.MouseEvent{X: 60, Y: 150, Button: widget.MouseWheelUp, Pressed: true})
	if r.page.sideY != 0 {
		t.Fatalf("sideY = %d after one tick up", r.page.sideY)
	}
}

func TestSidebarScrollIsLimitedToItsContent(t *testing.T) {
	r := shortRig(t, 12)

	r.page.OnMouseWheelPixelsMod(60, 150, 0, 1e6, 0)
	max := navContentHeight(r.page.rows) - r.page.geo.sidebar.Dy()
	if r.page.sideY != max || max <= 0 {
		t.Fatalf("sideY = %d, want %d", r.page.sideY, max)
	}
	r.page.OnMouseWheelPixelsMod(60, 150, 0, -1e6, 0)
	if r.page.sideY != 0 {
		t.Fatalf("sideY = %d", r.page.sideY)
	}
	last := r.page.rows[len(r.page.rows)-1]
	r.page.OnMouseWheelPixelsMod(60, 150, 0, 1e6, 0)
	bottom := r.page.geo.sidebar.Min.Y - r.page.sideY + int(last.center)
	if bottom > r.page.geo.sidebar.Max.Y {
		t.Fatalf("the last entry at y=%d is still below the panel %v", bottom, r.page.geo.sidebar)
	}
}

func TestWheelOverTheSidebarDoesNotMoveTheGridAndViceVersa(t *testing.T) {
	r := shortRig(t, 80)
	r.page.sideY = 0

	r.page.OnMouseWheelPixelsMod(60, 150, 0, 2*wheelNotchPixels, 0)
	if r.page.sideY == 0 || r.page.scroll != 0 {
		t.Fatalf("over the sidebar: sideY = %d, scroll = %d", r.page.sideY, r.page.scroll)
	}
	side := r.page.sideY
	r.page.OnMouseWheelPixelsMod(400, 150, 0, 2*wheelNotchPixels, 0)
	if r.page.scroll == 0 || r.page.sideY != side {
		t.Fatalf("over the grid: sideY = %d (was %d), scroll = %d", r.page.sideY, side, r.page.scroll)
	}
}

func TestSidebarHitTestFollowsTheScroll(t *testing.T) {
	r := shortRig(t, 12)
	r.page.sideY = 0
	var target navRow
	for _, row := range r.page.rows {
		if row.entry.Label == "Изображения" {
			target = row
		}
	}
	y := r.page.geo.sidebar.Min.Y + int(target.center)
	if got := r.page.hitAt(60, y); got.kind != hitNav || got.index != target.index {
		t.Fatalf("unscrolled hit at %d = %+v", y, got)
	}

	r.page.scrollSide(60)

	if got := r.page.hitAt(60, y); got.kind == hitNav && got.index == target.index {
		t.Fatalf("after scrolling the old position must hit another row: %+v", got)
	}
	r.click(60, y-60)
	if !r.rec.has(`navigate:C:\Users\u\Pictures`) {
		t.Fatalf("calls = %v", r.rec.calls)
	}
	r.page.OnMouseMove(60, y-60)
	if r.page.hover.kind != hitNav || r.page.hover.index != target.index {
		t.Fatalf("hover = %+v", r.page.hover)
	}
}

func TestSidebarScrollbarAppearsHidesAndDrags(t *testing.T) {
	r := shortRig(t, 12)
	r.page.sideY = 0
	if r.page.navBar.shown {
		t.Fatal("hidden before any interaction")
	}

	r.page.OnMouseWheelPixelsMod(60, 150, 0, wheelNotchPixels, 0)
	if !r.page.navBar.shown {
		t.Fatal("scrolling shows the bar")
	}
	r.render()
	r.tm.advance(barHideDelay)
	if r.page.navBar.shown {
		t.Fatal("the bar hides again")
	}

	_, thin, ok := r.page.navBarRects()
	if !ok || thin.Dx() != barThin || thin.Max.X > sidebarWidth {
		t.Fatalf("resting thumb %v", thin)
	}
	r.page.OnMouseMove(147, 150)
	if !r.page.navBar.shown {
		t.Fatal("hovering the right edge shows the bar")
	}
	track, thumb, _ := r.page.navBarRects()
	x, y := thumb.Min.X+1, thumb.Min.Y+2
	r.page.OnMouseMove(x, y)
	if _, wide, _ := r.page.navBarRects(); wide.Dx() != barWide {
		t.Fatalf("hovered thumb %v must widen", wide)
	}
	r.page.OnMouseMove(x, y)
	r.page.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft, Pressed: true})
	if r.page.navBar.drag == nil {
		t.Fatal("pressing the thumb starts a drag")
	}
	r.page.OnMouseMove(x, track.Max.Y+100)
	if r.page.sideY != r.page.navMax() {
		t.Fatalf("dragging to the end: sideY = %d, max %d", r.page.sideY, r.page.navMax())
	}
	r.page.OnMouseButton(widget.MouseEvent{X: x, Y: track.Max.Y, Button: widget.MouseLeft, Pressed: false})
	if r.page.navBar.drag != nil {
		t.Fatal("release ends the drag")
	}
	r.page.OnMouseMove(x, track.Min.Y-100)
	if r.page.sideY != r.page.navMax() {
		t.Fatal("moves after the release must not scroll")
	}
	r.render()
}

func TestSidebarShiftSurvivesRefreshAndIsClampedToTheNewHeight(t *testing.T) {
	r := shortRig(t, 12)
	r.page.OnMouseWheelPixelsMod(60, 150, 0, 1e6, 0)
	max := r.page.sideY

	r.page.Refresh()
	if r.page.sideY != max {
		t.Fatalf("refresh changed the shift: %d -> %d", max, r.page.sideY)
	}
	r.page.SetBounds(image.Rect(0, 0, mockW, 340))
	if r.page.sideY != r.page.navMax() || r.page.sideY >= max {
		t.Fatalf("a taller window must clamp the shift: %d (max %d, was %d)", r.page.sideY, r.page.navMax(), max)
	}
	r.page.SetBounds(image.Rect(0, 0, mockW, 430))
	if r.page.sideY != 0 {
		t.Fatalf("sideY = %d in a window that fits everything", r.page.sideY)
	}
	r.page.SetBounds(image.Rect(0, 0, mockW, 300))
	r.src.sidebar = r.src.sidebar[:5]
	r.page.sideY = 500
	r.page.Refresh()
	if r.page.sideY != r.page.navMax() {
		t.Fatalf("fewer entries must clamp the shift: %d vs %d", r.page.sideY, r.page.navMax())
	}
}

func TestSelectedEntryIsRevealedWhenTheLocationChanges(t *testing.T) {
	r := shortRig(t, 12)
	if r.page.sideY == 0 {
		t.Fatal("the initial selection is near the bottom")
	}

	for i := range r.src.sidebar {
		r.src.sidebar[i].Selected, r.src.sidebar[i].Expander = false, false
	}
	r.src.sidebar[0].Selected = true
	r.src.loc.Kind = 1
	r.page.Refresh()
	if r.page.sideY != 0 {
		t.Fatalf("selecting the first entry must scroll back to the top, sideY = %d", r.page.sideY)
	}

	r.src.sidebar[0].Selected = false
	r.src.sidebar[11].Selected = true
	r.src.loc = r.src.sidebar[11].Loc
	r.page.Refresh()
	row := selectedNavRow(r)
	pill := row.rect(r.page.geo.sidebar.Min.Y-r.page.sideY, sidebarWidth)
	if !pill.In(r.page.geo.sidebar) {
		t.Fatalf("selected pill %v outside the panel %v (sideY %d)", pill, r.page.geo.sidebar, r.page.sideY)
	}
}

func TestSelectionRevealWaitsForTheFirstLayout(t *testing.T) {
	eng := newRig(t, mockSource(), mockW, mockH).eng
	page := New(eng, Deps{Source: mockSource()})

	page.Refresh()
	if page.sideY != 0 || !page.navSeek {
		t.Fatalf("sideY = %d, pending = %v before the layout", page.sideY, page.navSeek)
	}
	page.SetBounds(image.Rect(1, 1, 592, 300))

	if page.sideY == 0 || page.navSeek {
		t.Fatalf("sideY = %d, pending = %v after the layout", page.sideY, page.navSeek)
	}
}

func TestHistoryActionsResetTheGridScrollBeforeTheAppRefreshes(t *testing.T) {
	r := longRig(t, 60)
	r.src.canBack = true
	r.page.deps.Actions.Back = func() { r.src.loc.Path = `D:\Elsewhere` }
	r.page.setScroll(500)

	r.click(25, 22)

	if r.page.scroll != 0 {
		t.Fatalf("scroll = %d after navigating back", r.page.scroll)
	}
}

func TestTallWindowKeepsTheSidebarStill(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)

	r.page.OnMouseWheelPixelsMod(60, 150, 0, 400, 0)

	if r.page.sideY != 0 {
		t.Fatalf("sideY = %d", r.page.sideY)
	}
}

func TestScrollToSeesItemsAddedSinceTheLastRefresh(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)
	r.src.items = manyItems(60)

	r.page.ScrollTo(50)

	if !r.page.tileRect(50).In(r.page.geo.grid) {
		t.Fatalf("tile 50 at %v, viewport %v", r.page.tileRect(50), r.page.geo.grid)
	}
}

func TestScrollToBeforeTheFirstLayoutWaitsForTheBounds(t *testing.T) {
	eng := newRig(t, mockSource(), mockW, mockH).eng
	src := mockSource()
	src.items = manyItems(60)
	page := New(eng, Deps{Source: src})

	page.ScrollTo(50)
	if page.scroll != 0 {
		t.Fatalf("scroll = %d before any layout", page.scroll)
	}
	page.SetBounds(image.Rect(1, 1, 592, 429))

	if !page.tileRect(50).In(page.geo.grid) {
		t.Fatalf("tile 50 at %v, viewport %v", page.tileRect(50), page.geo.grid)
	}
}
