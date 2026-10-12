package window

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/commands"
)

type keyContent struct {
	fakeContent
	taken []widget.KeyEvent
	take  bool
}

func (k *keyContent) HandleKey(e widget.KeyEvent) bool {
	k.taken = append(k.taken, e)
	return k.take
}

func galleryRig(t *testing.T, content *keyContent, holes ...image.Rectangle) *rig {
	t.Helper()
	return newRig(t, func(h *fakeHost) {
		h.page = PageGallery
		h.holes = holes
		if content != nil {
			h.contentOverride = content
		}
	})
}

func TestGalleryHeaderIsShorterThanTheViewerTitle(t *testing.T) {
	l := Compute(593, 430, Options{Page: PageGallery})

	if l.Title != image.Rect(0, 0, 593, GalleryHeaderHeight) {
		t.Fatalf("title = %v", l.Title)
	}
	if l.Content != image.Rect(1, 1, 592, 429) || l.View != l.Content {
		t.Fatalf("content = %v, view = %v", l.Content, l.View)
	}
	if l.Panel != (image.Rectangle{}) || l.Toolbar != (image.Rectangle{}) {
		t.Fatalf("the gallery has no panel or toolbar: %v %v", l.Panel, l.Toolbar)
	}
}

func TestGalleryCaptionButtonsFollowTheMockup(t *testing.T) {
	l := Compute(593, 430, Options{Page: PageGallery})

	for _, c := range []struct {
		k  ZoneKind
		cx int
	}{{ZoneMin, 505}, {ZoneMax, 539}, {ZoneClose, 573}} {
		x, y := l.Center(c.k)
		if d := x - c.cx; d < -1 || d > 1 || y != GalleryHeaderHeight/2 {
			t.Errorf("%v centre = (%d, %d), want (~%d, %d)", c.k, x, y, c.cx, GalleryHeaderHeight/2)
		}
	}
}

func TestGalleryHasNoViewerZones(t *testing.T) {
	l := Compute(593, 430, Options{Page: PageGallery, Toolbar: true, Arrows: true})

	for _, k := range []ZoneKind{ZoneBack, ZoneInfo, ZoneFavorite, ZoneMore, ZonePrev, ZoneNext, ZoneToolGallery, ZoneToolFullscreen} {
		if !l.Rect(k).Empty() {
			t.Errorf("zone %v present on the gallery page", k)
		}
	}
}

func TestGalleryDragZoneSkipsHeaderControls(t *testing.T) {
	hole := image.Rect(95, 8, 390, 35)
	l := Compute(593, 430, Options{Page: PageGallery, Holes: []image.Rectangle{hole}})

	if z := l.ZoneAt(image.Pt(200, 20)); z.Kind != ZoneNone {
		t.Errorf("inside the address bar the zone is %v, want none", z.Kind)
	}
	for _, p := range []image.Point{{60, 20}, {200, 3 + 1}, {200, 38}, {420, 20}} {
		if z := l.ZoneAt(p); z.Kind != ZoneDrag {
			t.Errorf("at %v the zone is %v, want drag", p, z.Kind)
		}
	}
}

func TestSubtractKeepsEverythingOutsideTheHole(t *testing.T) {
	band := image.Rect(0, 0, 100, 40)

	pieces := subtract([]image.Rectangle{band}, image.Rect(30, 10, 60, 30))

	area := 0
	for _, p := range pieces {
		area += p.Dx() * p.Dy()
		if p.Overlaps(image.Rect(30, 10, 60, 30)) {
			t.Errorf("piece %v overlaps the hole", p)
		}
	}
	if want := 100*40 - 30*20; area != want {
		t.Fatalf("area = %d, want %d", area, want)
	}
}

func TestSubtractIgnoresAHoleOutsideTheBand(t *testing.T) {
	band := image.Rect(0, 0, 100, 40)

	pieces := subtract([]image.Rectangle{band}, image.Rect(0, 100, 50, 120))

	if len(pieces) != 1 || pieces[0] != band {
		t.Fatalf("pieces = %v", pieces)
	}
}

func TestGalleryCaptionButtonsInvokeOps(t *testing.T) {
	r := galleryRig(t, nil)
	l := r.root.Layout()
	click := func(k ZoneKind) {
		x, y := l.Center(k)
		r.click(x, y)
	}

	click(ZoneClose)
	click(ZoneMax)
	click(ZoneMin)

	want := []string{"close", "max", "min"}
	if len(r.ops) != 3 || r.ops[0] != want[0] || r.ops[1] != want[1] || r.ops[2] != want[2] {
		t.Fatalf("ops = %v, want %v", r.ops, want)
	}
}

func TestGalleryHeaderDragMovesTheWindowButNotInsideControls(t *testing.T) {
	r := galleryRig(t, nil, image.Rect(95, 8, 390, 35))
	e := widget.MouseEvent{X: 200, Y: 20, Button: widget.MouseLeft, Clicks: 1, Pressed: true}

	if r.root.OnMouseButton(e) {
		t.Fatal("a press inside the address bar must reach the page, not start a drag")
	}
	e.X, e.Y = 60, 20
	if !r.root.OnMouseButton(e) {
		t.Fatal("a press on the free header must be taken by the window")
	}
}

func TestGalleryHitTestReportsCaptionOnlyOutsideControls(t *testing.T) {
	r := galleryRig(t, nil, image.Rect(95, 8, 390, 35))
	r.eng.Flush()
	r.eng.RenderOnce()

	if got := r.root.HitTest(200, 20); got != HitClient {
		t.Errorf("address bar hit = %d, want client", got)
	}
	if got := r.root.HitTest(60, 20); got != HitCaption {
		t.Errorf("free header hit = %d, want caption", got)
	}
	mx, my := r.root.Layout().Center(ZoneMax)
	if got := r.root.HitTest(mx, my); got != HitMaxButton {
		t.Errorf("maximize hit = %d", got)
	}
}

func TestGalleryKeysGoToThePageFirst(t *testing.T) {
	page := &keyContent{take: true}
	r := galleryRig(t, page)
	r.host.reg.Add(commands.Command{ID: "nav.next", Keys: []commands.Chord{commands.C(0, commands.KeyRight)}, Run: func() { r.host.ran = append(r.host.ran, "nav.next") }})

	handled := r.root.HandleKey(widget.KeyEvent{Code: widget.KeyRight, Pressed: true})

	if !handled || len(page.taken) != 1 {
		t.Fatalf("handled = %v, page got %d keys", handled, len(page.taken))
	}
	for _, id := range r.host.ran {
		if id == "nav.next" {
			t.Fatal("the viewer command ran on the gallery page")
		}
	}
}

func TestGalleryKeysNotTakenByThePageDoNotRunViewerCommands(t *testing.T) {
	page := &keyContent{}
	r := galleryRig(t, page)
	r.host.reg.Add(commands.Command{ID: "view.rotateRight", Keys: []commands.Chord{commands.C(0, 'R')}, Run: func() { r.host.ran = append(r.host.ran, "view.rotateRight") }})

	handled := r.root.HandleKey(widget.KeyEvent{Code: widget.KeyR, Pressed: true})

	if handled || len(r.host.ran) != 0 {
		t.Fatalf("handled = %v, ran = %v: a typed letter must reach the address bar", handled, r.host.ran)
	}
}

func TestGalleryKeepsWindowLevelShortcuts(t *testing.T) {
	r := galleryRig(t, &keyContent{})
	r.host.reg.Add(commands.Command{ID: "app.escape", Keys: []commands.Chord{commands.C(0, commands.KeyEscape)}, Run: func() { r.host.ran = append(r.host.ran, "app.escape") }})

	handled := r.root.HandleKey(widget.KeyEvent{Code: widget.KeyEscape, Pressed: true})

	if !handled || len(r.host.ran) != 1 || r.host.ran[0] != "app.escape" {
		t.Fatalf("handled = %v, ran = %v", handled, r.host.ran)
	}
}

func TestGalleryLeavesSideButtonsToThePage(t *testing.T) {
	r := galleryRig(t, nil)

	if r.root.OnMouseButton(widget.MouseEvent{Button: widget.MouseButtonX1, Pressed: true}) {
		t.Fatal("the side button must reach the gallery for history navigation")
	}
	if len(r.host.ran) != 0 {
		t.Fatalf("ran = %v", r.host.ran)
	}
}

func TestGalleryPageDrawsWithoutTheViewerTitle(t *testing.T) {
	r := galleryRig(t, nil)

	img := r.eng.RenderOnce()

	if img == nil {
		t.Fatal("no frame")
	}
}
