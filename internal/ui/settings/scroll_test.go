package settings

import (
	"image"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/widget"
)

func TestShortSectionHasNothingToScroll(t *testing.T) {
	r := newRig(t, 900, 700)
	r.page.SetSection(SectionSlideshow)
	if r.page.maxScroll() != 0 {
		t.Fatalf("maxScroll = %d", r.page.maxScroll())
	}
	if r.page.OnMouseWheelPixelsMod(400, 300, 0, 400, 0); r.page.scroll != 0 {
		t.Fatalf("scroll = %d", r.page.scroll)
	}
	if _, _, ok := r.page.barRects(); ok {
		t.Fatal("no scrollbar without overflow")
	}
}

func TestWheelNotchScrollsAFixedStep(t *testing.T) {
	r := newRig(t, mockW, mockH)
	handled := r.page.OnMouseWheelPixelsMod(300, 200, 0, wheelNotchPixels, 0)
	if !handled || r.page.scroll != int(wheelStep) {
		t.Fatalf("handled = %v, scroll = %d, want %d", handled, r.page.scroll, int(wheelStep))
	}
	r.page.OnMouseWheelPixelsMod(300, 200, 0, -wheelNotchPixels, 0)
	if r.page.scroll != 0 {
		t.Fatalf("scroll = %d after scrolling back", r.page.scroll)
	}
}

func TestWheelIsLimitedToTheContent(t *testing.T) {
	r := newRig(t, mockW, mockH)
	r.page.OnMouseWheelPixelsMod(300, 200, 0, 1e6, 0)
	if r.page.scroll != r.page.maxScroll() || r.page.scroll == 0 {
		t.Fatalf("scroll = %d, max = %d", r.page.scroll, r.page.maxScroll())
	}
	r.page.OnMouseWheelPixelsMod(300, 200, 0, -1e6, 0)
	if r.page.scroll != 0 {
		t.Fatalf("scroll = %d", r.page.scroll)
	}
}

func TestWheelTicksFromButtonsScrollToo(t *testing.T) {
	r := newRig(t, mockW, mockH)
	r.page.OnMouseButton(widget.MouseEvent{X: 300, Y: 200, Button: widget.MouseWheelDown, Pressed: true})
	r.page.OnMouseButton(widget.MouseEvent{X: 300, Y: 200, Button: widget.MouseWheelDown, Pressed: false})
	if r.page.scroll != int(wheelStep) {
		t.Fatalf("scroll = %d after one tick", r.page.scroll)
	}
	if handled := r.page.OnMouseButton(widget.MouseEvent{X: 300, Y: 200, Button: widget.MouseWheelUp, Pressed: true}); !handled || r.page.scroll != 0 {
		t.Fatalf("handled = %v, scroll = %d", handled, r.page.scroll)
	}
	r.page.OnMouseButton(widget.MouseEvent{X: 50, Y: 200, Button: widget.MouseWheelDown, Pressed: true})
	if r.page.scroll != 0 {
		t.Fatal("ticks over the navigation do not scroll the content")
	}
}

func TestFractionalWheelDeltasAccumulate(t *testing.T) {
	r := newRig(t, mockW, mockH)
	for i := 0; i < 5; i++ {
		r.page.OnMouseWheelPixelsMod(300, 200, 0, 4, 0)
	}
	if r.page.scroll != 24 {
		t.Fatalf("scroll = %d, want 24 (5 x 4.8 px)", r.page.scroll)
	}
}

func TestWheelOverTheHeaderAndNavigationIsNotConsumed(t *testing.T) {
	r := newRig(t, mockW, mockH)
	if r.page.OnMouseWheelPixelsMod(300, 10, 0, 40, 0) {
		t.Fatal("the header does not scroll")
	}
	if r.page.OnMouseWheelPixelsMod(60, 200, 0, 40, 0) {
		t.Fatal("the navigation does not scroll")
	}
	if r.page.OnMouseWheelPixelsMod(300, 200, 0, 0, 0) {
		t.Fatal("a zero delta is nothing")
	}
}

func TestScrollbarAppearsOnScrollAndHidesAfterAMoment(t *testing.T) {
	r := newRig(t, mockW, mockH)
	if r.page.bar.shown {
		t.Fatal("the bar is hidden initially")
	}
	r.page.OnMouseWheelPixelsMod(300, 200, 0, 40, 0)
	if !r.page.bar.shown {
		t.Fatal("scrolling shows the bar")
	}
	r.tm.advance(barHideDelay / 2)
	if !r.page.bar.shown {
		t.Fatal("the bar stays for a while")
	}
	r.page.OnMouseWheelPixelsMod(300, 200, 0, 40, 0)
	r.tm.advance(barHideDelay - time.Millisecond)
	if !r.page.bar.shown {
		t.Fatal("scrolling again restarts the timer")
	}
	r.tm.advance(time.Millisecond)
	if r.page.bar.shown {
		t.Fatal("the bar hides after the delay")
	}
	if n := r.tm.live(); n != 0 {
		t.Fatalf("%d timers are still pending", n)
	}
}

func TestScrollbarGeometryFollowsTheScrollPosition(t *testing.T) {
	r := newRig(t, mockW, mockH)
	track, thumb, ok := r.page.barRects()
	if !ok || thumb.Min.Y != track.Min.Y {
		t.Fatalf("thumb %v must start at the top of the track %v", thumb, track)
	}
	r.page.setScroll(r.page.maxScroll())
	_, thumb, _ = r.page.barRects()
	if thumb.Max.Y != track.Max.Y {
		t.Fatalf("thumb %v must end at the bottom of the track %v", thumb, track)
	}
	if track.Max.X != r.page.geo.view.Max.X-barRightInset || thumb.Dx() != barThin {
		t.Fatalf("track %v thumb %v", track, thumb)
	}
	if h := thumb.Dy(); h < 24 || h >= track.Dy() {
		t.Fatalf("thumb height %d", h)
	}
}

func TestDraggingTheScrollbarThumbScrolls(t *testing.T) {
	r := newRig(t, mockW, mockH)
	r.page.setScroll(1)
	_, thumb, _ := r.page.barRects()
	x, y := thumb.Min.X+1, (thumb.Min.Y+thumb.Max.Y)/2
	r.page.OnMouseMove(x, y)
	press(r, x, y)
	if r.page.drag == nil || !r.page.drag.bar {
		t.Fatal("pressing the visible thumb starts a drag")
	}
	r.page.OnMouseMove(x, y+400)
	if r.page.scroll != r.page.maxScroll() {
		t.Fatalf("dragging down: scroll = %d, max = %d", r.page.scroll, r.page.maxScroll())
	}
	r.page.OnMouseMove(x, y-400)
	if r.page.scroll != 0 {
		t.Fatalf("dragging up: scroll = %d", r.page.scroll)
	}
	release(r, x, y)
	if r.page.drag != nil {
		t.Fatal("release ends the drag")
	}
	r.tm.advance(barHideDelay)
	if r.page.bar.shown {
		t.Fatal("the bar hides after the drag")
	}
}

func TestScrollbarDoesNotHideWhileDragging(t *testing.T) {
	r := newRig(t, mockW, mockH)
	r.page.setScroll(1)
	_, thumb, _ := r.page.barRects()
	x, y := thumb.Min.X+1, (thumb.Min.Y+thumb.Max.Y)/2
	r.page.OnMouseMove(x, y)
	press(r, x, y)
	r.tm.advance(10 * barHideDelay)
	if !r.page.bar.shown {
		t.Fatal("the bar stays while it is held")
	}
	release(r, x, y)
}

func TestHoveringTheScrollbarAreaRevealsIt(t *testing.T) {
	r := newRig(t, mockW, mockH)
	area := r.page.barArea()
	r.page.OnMouseMove(area.Min.X+2, area.Min.Y+80)
	if !r.page.bar.shown {
		t.Fatal("moving over the right edge shows the bar")
	}
	if got := r.page.hitAt(area.Max.X-4, 100); got.kind != hitBar && got.kind != hitNone {
		t.Fatalf("hit = %+v", got)
	}
}

func TestResizingKeepsTheScrollInRange(t *testing.T) {
	r := newRig(t, mockW, mockH)
	r.page.setScroll(r.page.maxScroll())
	r.page.SetBounds(image.Rect(0, 0, mockW, 900))
	if r.page.scroll > r.page.maxScroll() {
		t.Fatalf("scroll %d beyond max %d after growing the window", r.page.scroll, r.page.maxScroll())
	}
	if r.page.maxScroll() != 0 {
		t.Fatalf("maxScroll = %d in a tall window", r.page.maxScroll())
	}
}
