package window

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/commands"
	"github.com/oops1/PixGo/internal/ui/theme"
)

func fullscreenRig(t *testing.T, mutate func(*fakeHost)) *rig {
	t.Helper()
	return newSizedRig(t, mockupW, mockupH, func(h *fakeHost) {
		h.fullscreen = true
		h.slide = slideshowMockupView()
		if mutate != nil {
			mutate(h)
		}
	})
}

func middle(r image.Rectangle) (float64, float64) {
	return float64(r.Min.X+r.Max.X) / 2, float64(r.Min.Y+r.Max.Y) / 2
}

func zoneCenterPoint(l Layout, k ZoneKind) image.Point {
	x, y := l.Center(k)
	return image.Pt(x, y)
}

type openMenu struct {
	widget.Base
	open bool
}

func (m *openMenu) IsOpen() bool { return m.open }

func (m *openMenu) Draw(c widget.DrawContext) {}

func TestFullscreenLayoutFollowsTheMockup(t *testing.T) {
	l := mockupLayout()
	cases := []struct {
		name   string
		r      image.Rectangle
		cx, cy float64
	}{
		{"close", l.Rect(ZoneExit), 307, 20},
		{"prev arrow", l.Prev, 27, 165},
		{"next arrow", l.Next, 302, 165},
		{"play", l.Rect(ZoneSlidePlay), 43, 310},
		{"settings", l.Rect(ZoneSlideSettings), 245, 310},
		{"exit fullscreen", l.Rect(ZoneSlideExit), 285, 310},
	}
	for _, c := range cases {
		x, y := middle(c.r)
		near(t, c.name, x, y, c.cx, c.cy, 0.5)
	}
	if l.Slide.Panel != image.Rect(18, 287, 310, 333) {
		t.Errorf("panel = %v, want (18,287)-(310,333)", l.Slide.Panel)
	}
	if l.Prev.Dx() != FullArrowDiameter || l.Next.Dy() != FullArrowDiameter {
		t.Errorf("arrows are %v and %v, want %d px circles", l.Prev, l.Next, FullArrowDiameter)
	}
	if got, want := l.Slide.TrackX0, 115.4; math.Abs(got-want) > .05 {
		t.Errorf("track starts at %.2f, want %.1f", got, want)
	}
	if got, want := l.Slide.TrackX1, 219.5; math.Abs(got-want) > .05 {
		t.Errorf("track ends at %.2f, want %.1f", got, want)
	}
	if l.Slide.CounterX != 71 || l.Slide.CounterBase != 315 {
		t.Errorf("counter at (%d, %d), want x 71 and baseline 315", l.Slide.CounterX, l.Slide.CounterBase)
	}
	if l.Toolbar != l.Slide.Panel {
		t.Errorf("toasts and tips anchor to the toolbar rectangle, which must be the panel: %v", l.Toolbar)
	}
}

func TestFullscreenLayoutAtFullHD(t *testing.T) {
	l := Compute(1920, 1080, Options{Fullscreen: true, Toolbar: true, Arrows: true, CounterDigits: 2})
	if l.Slide.Panel != image.Rect(814, 1024, 1106, 1070) {
		t.Errorf("panel = %v, want a 292 x 46 pill centred 10 px above the bottom", l.Slide.Panel)
	}
	if l.Content != image.Rect(0, 0, 1920, 1080) || !l.Title.Empty() {
		t.Errorf("fullscreen content %v title %v", l.Content, l.Title)
	}
	px, py := middle(l.Prev)
	nx, ny := middle(l.Next)
	cx, cy := middle(l.Rect(ZoneExit))
	near(t, "prev", px, py, 27, 533.5, 1)
	near(t, "next", nx, ny, 1895, 533.5, 1)
	near(t, "close", cx, cy, 1900, 20, .5)
	if py != ny {
		t.Errorf("arrows at different heights: %v and %v", py, ny)
	}
}

func TestFullscreenPanelFitsNarrowWindows(t *testing.T) {
	for _, w := range []int{240, 280, 300, 327, 360, 420, 640} {
		l := Compute(w, 300, Options{Fullscreen: true, Toolbar: true, Arrows: true, CounterDigits: 2})
		panel := l.Slide.Panel
		if panel.Min.X < SlideBarMargin || panel.Max.X > w-SlideBarMargin {
			t.Errorf("w=%d: panel %v leaves less than %d px at the sides", w, panel, SlideBarMargin)
		}
		if panel.Dx() > SlideBarWidth {
			t.Errorf("w=%d: panel %v is wider than the mockup", w, panel)
		}
		for _, z := range l.Zones {
			if z.Kind >= ZoneSlidePlay && z.Kind <= ZoneSlideExit && !z.Rect.In(panel) {
				t.Errorf("w=%d: zone %d %v sticks out of the panel %v", w, z.Kind, z.Rect, panel)
			}
		}
		if l.Rect(ZoneSlidePlay).Overlaps(l.Rect(ZoneSlideSettings)) || l.Rect(ZoneSlideSettings).Overlaps(l.Rect(ZoneSlideExit)) {
			t.Errorf("w=%d: buttons overlap", w)
		}
		if w >= 300 && l.Slide.TrackX0-float64(l.Slide.CounterX) < 43 {
			t.Errorf("w=%d: track starts at %.1f, too close to the counter that begins at %d", w, l.Slide.TrackX0, l.Slide.CounterX)
		}
		if w >= 300 && !l.Slide.HasTrack() {
			t.Errorf("w=%d: no room for the progress bar", w)
		}
		if l.Slide.HasTrack() && l.Slide.TrackX1 > float64(l.Rect(ZoneSlideSettings).Min.X) {
			t.Errorf("w=%d: track ends at %.1f, under the settings button %v", w, l.Slide.TrackX1, l.Rect(ZoneSlideSettings))
		}
	}
}

func TestFullscreenCounterReservesRoomForBigFolders(t *testing.T) {
	cases := []struct {
		digits int
		extra  int
	}{{1, 0}, {2, 0}, {3, SlideDigitWidth}, {4, 2 * SlideDigitWidth}, {5, 3 * SlideDigitWidth}}
	base := Compute(1920, 1080, Options{Fullscreen: true, Toolbar: true, CounterDigits: 2}).Slide
	for _, c := range cases {
		s := Compute(1920, 1080, Options{Fullscreen: true, Toolbar: true, CounterDigits: c.digits}).Slide
		if s.Panel.Dx() != SlideBarWidth+c.extra {
			t.Errorf("%d digits: panel width %d, want %d", c.digits, s.Panel.Dx(), SlideBarWidth+c.extra)
		}
		if got, want := s.TrackX1-s.TrackX0, base.TrackX1-base.TrackX0; math.Abs(got-want) > .01 {
			t.Errorf("%d digits: track length %.1f, want it unchanged at %.1f", c.digits, got, want)
		}
		if s.Settings.Min.X-s.Panel.Min.X != base.Settings.Min.X-base.Panel.Min.X+c.extra {
			t.Errorf("%d digits: the right-hand buttons must keep their distance from the right edge", c.digits)
		}
	}
}

func TestFullscreenHitTesting(t *testing.T) {
	l := mockupLayout()
	cases := []struct {
		name string
		p    image.Point
		want ZoneKind
	}{
		{"close", image.Pt(307, 20), ZoneExit},
		{"close corner", image.Pt(326, 0), ZoneExit},
		{"prev arrow", image.Pt(27, 165), ZonePrev},
		{"next arrow", image.Pt(302, 165), ZoneNext},
		{"play", image.Pt(43, 310), ZoneSlidePlay},
		{"settings", image.Pt(245, 310), ZoneSlideSettings},
		{"exit", image.Pt(285, 310), ZoneSlideExit},
		{"panel between buttons", image.Pt(150, 298), ZoneSlideBar},
		{"panel counter", image.Pt(85, 310), ZoneSlideBar},
		{"photo", image.Pt(160, 150), ZoneNone},
		{"just above the panel", image.Pt(160, 286), ZoneNone},
		{"just below the panel", image.Pt(160, 333), ZoneNone},
		{"left of the panel", image.Pt(17, 310), ZoneNone},
		{"window corner", image.Pt(0, 342), ZoneNone},
	}
	for _, c := range cases {
		if got := l.ZoneAt(c.p).Kind; got != c.want {
			t.Errorf("%s at %v: zone %d, want %d", c.name, c.p, got, c.want)
		}
	}
}

func TestFullscreenHasNoWindowedZones(t *testing.T) {
	l := mockupLayout()
	for _, z := range l.Zones {
		switch {
		case z.Kind >= ZoneToolGallery && z.Kind <= ZoneToolFullscreen, z.Kind >= ZoneResizeTop,
			z.Kind == ZoneBack, z.Kind == ZoneInfo, z.Kind == ZoneFavorite, z.Kind == ZoneMore,
			z.Kind == ZoneMin, z.Kind == ZoneMax, z.Kind == ZoneClose, z.Kind == ZoneDrag:
			t.Errorf("zone %d present in fullscreen", z.Kind)
		}
	}
}

func TestFullscreenHiddenOverlayHasNoZones(t *testing.T) {
	l := Compute(mockupW, mockupH, Options{Fullscreen: true})
	if len(l.Zones) != 0 {
		t.Errorf("zones with the overlay hidden: %v", l.Zones)
	}
	if l.Slide.Panel.Empty() {
		t.Error("the panel rectangle is still needed to repaint the area that was hidden")
	}
}

func TestSlideButtonsRunCommandsOpenTheMenuAndLeaveFullscreen(t *testing.T) {
	r := fullscreenRig(t, nil)
	var anchor image.Rectangle
	r.root.OnSlideSettings = func(a image.Rectangle) { anchor = a }
	l := r.root.Layout()
	r.click(zoneCenterPoint(l, ZoneSlidePlay).X, zoneCenterPoint(l, ZoneSlidePlay).Y)
	r.click(zoneCenterPoint(l, ZoneSlideSettings).X, zoneCenterPoint(l, ZoneSlideSettings).Y)
	r.click(zoneCenterPoint(l, ZoneSlideExit).X, zoneCenterPoint(l, ZoneSlideExit).Y)
	r.click(zoneCenterPoint(l, ZoneExit).X, zoneCenterPoint(l, ZoneExit).Y)
	r.click(150, 298)
	want := []commands.ID{"slideshow.toggle", "view.fullscreen", "view.fullscreen"}
	if len(r.host.ran) != len(want) {
		t.Fatalf("ran %v, want %v", r.host.ran, want)
	}
	for i := range want {
		if r.host.ran[i] != want[i] {
			t.Fatalf("ran %v, want %v", r.host.ran, want)
		}
	}
	if anchor != l.Rect(ZoneSlideSettings) {
		t.Errorf("the settings menu is anchored to %v, want the gear %v", anchor, l.Rect(ZoneSlideSettings))
	}
}

func TestSlidePanelSwallowsClicksAndDoesNotPassThemToThePhoto(t *testing.T) {
	r := fullscreenRig(t, nil)
	press := widget.MouseEvent{X: 150, Y: 298, Button: widget.MouseLeft, Pressed: true, Clicks: 1}
	if !r.root.WantsCapture(press) {
		t.Error("a press on the panel background must be captured by the chrome")
	}
	press.Y = 150
	if r.root.WantsCapture(press) {
		t.Error("a press on the photo must reach the canvas")
	}
}

func TestSlideButtonReleasedElsewhereDoesNothing(t *testing.T) {
	r := fullscreenRig(t, nil)
	l := r.root.Layout()
	p := zoneCenterPoint(l, ZoneSlidePlay)
	r.root.OnMouseButton(widget.MouseEvent{X: p.X, Y: p.Y, Button: widget.MouseLeft, Pressed: true, Clicks: 1})
	r.root.OnMouseButton(widget.MouseEvent{X: 150, Y: 150, Button: widget.MouseLeft, Clicks: 1})
	if len(r.host.ran) != 0 {
		t.Errorf("ran %v after releasing outside the button", r.host.ran)
	}
}

func TestSlideHoverRepaintsOnlyTheHoveredButton(t *testing.T) {
	r := fullscreenRig(t, nil)
	r.rec.reset()
	p := zoneCenterPoint(r.root.Layout(), ZoneSlidePlay)
	r.root.OnMouseMove(p.X, p.Y)
	if r.rec.full != 0 {
		t.Errorf("hover caused %d full repaints", r.rec.full)
	}
	play := r.root.Layout().Rect(ZoneSlidePlay)
	found := false
	for _, rc := range r.rec.rects {
		if !rc.In(play.Inset(-3)) {
			t.Errorf("hover invalidated %v, outside the button %v", rc, play)
		}
		found = found || !rc.Empty()
	}
	if !found {
		t.Error("hover did not repaint the button")
	}
}

func TestSlidePanelBackgroundDoesNotRepaintOnHover(t *testing.T) {
	r := fullscreenRig(t, nil)
	r.root.OnMouseMove(150, 298)
	r.rec.reset()
	r.root.OnMouseMove(160, 300)
	r.root.OnMouseMove(150, 150)
	if len(r.rec.rects) != 0 || r.rec.full != 0 {
		t.Errorf("moving over the bare panel repainted %v (full %d)", r.rec.rects, r.rec.full)
	}
}

func TestSlideTooltips(t *testing.T) {
	cases := []struct {
		name  string
		slide SlideshowView
		zone  ZoneKind
		want  string
	}{
		{"idle play", SlideshowView{Index: 5, Total: 24}, ZoneSlidePlay, "Слайд-шоу (F5)"},
		{"running", SlideshowView{Active: true, Playing: true, Index: 5, Total: 24}, ZoneSlidePlay, "Пауза (Пробел)"},
		{"paused", SlideshowView{Active: true, Index: 5, Total: 24}, ZoneSlidePlay, "Продолжить (Пробел)"},
		{"settings", slideshowMockupView(), ZoneSlideSettings, "Настройки слайд-шоу"},
		{"exit", slideshowMockupView(), ZoneSlideExit, "Выйти из полноэкранного режима (F11)"},
		{"close", slideshowMockupView(), ZoneExit, "Выйти из полноэкранного режима (F11)"},
		{"panel", slideshowMockupView(), ZoneSlideBar, ""},
	}
	for _, c := range cases {
		r := fullscreenRig(t, func(h *fakeHost) {
			h.slide = c.slide
			h.reg.Add(commands.Command{ID: "slideshow.start", Title: "cmd.slideshow", Keys: []commands.Chord{commands.C(0, commands.KeyF5)}, Run: func() {}})
			h.reg.Add(commands.Command{ID: "slideshow.toggle", Title: "cmd.slideshowPause", Keys: []commands.Chord{commands.C(0, commands.KeySpace)}, Run: func() {}})
			h.reg.Add(commands.Command{ID: "view.fullscreen", Title: "cmd.fullscreen", Keys: []commands.Chord{commands.C(0, commands.KeyF11)}, Run: func() {}})
		})
		p := zoneCenterPoint(r.root.Layout(), c.zone)
		r.root.OnMouseMove(p.X, p.Y)
		r.ts.fireAll()
		if got := r.root.tip.text; got != c.want {
			t.Errorf("%s: tooltip %q, want %q", c.name, got, c.want)
		}
		if c.want != "" {
			rect := r.root.tipRect(nil)
			if rect.Min.X < 0 || rect.Max.X > mockupW {
				t.Errorf("%s: tooltip %v leaves the window", c.name, rect)
			}
			if c.zone >= ZoneSlidePlay && c.zone <= ZoneSlideExit && rect.Max.Y > r.root.Layout().Slide.Panel.Min.Y {
				t.Errorf("%s: tooltip %v covers the panel", c.name, rect)
			}
		}
	}
}

func TestFullscreenOverlayHidesAndReturnsWithTheMouse(t *testing.T) {
	r := fullscreenRig(t, nil)
	var seen []bool
	r.root.OnOverlayChange = func(v bool) { seen = append(seen, v) }
	r.root.Wake()
	if r.ts.live() != 1 || !r.root.OverlayVisible() {
		t.Fatalf("armed timers %d, visible %v", r.ts.live(), r.root.OverlayVisible())
	}
	r.ts.fireAll()
	if r.root.OverlayVisible() || len(r.root.Layout().Zones) != 0 {
		t.Fatal("the idle timer must hide the panel, the arrows and the close button")
	}
	if got := r.eng.CursorAt(100, 100); got != widget.CursorNone {
		t.Errorf("cursor after idle = %v, want none", got)
	}
	r.root.OnMouseMove(160, 150)
	if !r.root.OverlayVisible() || len(r.root.Layout().Zones) == 0 {
		t.Fatal("moving the mouse must bring the overlay back")
	}
	if got := r.eng.CursorAt(100, 100); got == widget.CursorNone {
		t.Error("the cursor is still hidden after the mouse moved")
	}
	if len(seen) != 2 || seen[0] != false || seen[1] != true {
		t.Errorf("overlay notifications = %v, want [false true]", seen)
	}
}

func TestFullscreenOverlayStaysWhileTheMouseIsOverThePanelOrAMenuIsOpen(t *testing.T) {
	r := fullscreenRig(t, nil)
	r.root.OnMouseMove(150, 298)
	r.ts.fireAll()
	if !r.root.OverlayVisible() {
		t.Fatal("the overlay hid under the pointer")
	}
	menu := &openMenu{open: true}
	r.host.menu = menu
	r.root.OnMouseMove(160, 150)
	r.ts.fireAll()
	if !r.root.OverlayVisible() {
		t.Fatal("the overlay hid while the settings menu was open")
	}
	menu.open = false
	r.ts.fireAll()
	if r.root.OverlayVisible() {
		t.Fatal("the overlay must hide once the menu is closed")
	}
}

func TestRefreshSlideProgressRepaintsOnlyTheTrack(t *testing.T) {
	r := fullscreenRig(t, nil)
	r.rec.reset()
	r.root.RefreshSlideProgress()
	want := r.root.Layout().Slide.Damage
	if r.rec.full != 0 || len(r.rec.rects) != 1 || r.rec.rects[0] != want {
		t.Fatalf("progress repaint = %v (full %d), want only %v", r.rec.rects, r.rec.full, want)
	}
	if want.Dy() > 14 || want.Dx() < 100 {
		t.Errorf("the progress repaint rectangle %v must be a thin strip", want)
	}
	if !want.In(r.root.Layout().Slide.Panel) {
		t.Errorf("the strip %v sticks out of the panel", want)
	}

	r.root.Wake()
	r.ts.fireAll()
	r.rec.reset()
	r.root.RefreshSlideProgress()
	if len(r.rec.rects) != 0 || r.rec.full != 0 {
		t.Errorf("a hidden panel was repainted: %v", r.rec.rects)
	}

	windowed := newRig(t, nil)
	windowed.rec.reset()
	windowed.root.RefreshSlideProgress()
	windowed.root.RefreshSlide()
	if len(windowed.rec.rects) != 0 || windowed.rec.full != 0 {
		t.Errorf("outside fullscreen nothing may repaint: %v", windowed.rec.rects)
	}
}

func progressPhoto(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, color.RGBA{uint8(40 + x*180/w), uint8(60 + y*150/h), uint8(180 - x*90/w), 255})
		}
	}
	return img
}

func TestProgressOnlyRepaintEqualsAFullRepaint(t *testing.T) {
	photo := progressPhoto(mockupW, mockupH)
	mutate := func(p float64) func(*fakeHost) {
		return func(h *fakeHost) {
			h.slide = slideshowMockupView()
			h.slide.Progress = p
		}
	}
	partial := newGoldenRig(t, mockupW, mockupH, photo, mutate(0.8))
	partial.render()
	partial.host.slide.Progress = 0.1
	partial.root.RefreshSlideProgress()
	partial.eng.Flush()
	got := partial.eng.RenderOnce()

	want := newGoldenRig(t, mockupW, mockupH, photo, mutate(0.1)).render()
	for y := 0; y < mockupH; y++ {
		for x := 0; x < mockupW; x++ {
			if got.RGBAAt(x, y) != want.RGBAAt(x, y) {
				t.Fatalf("pixel (%d,%d) differs after a progress-only repaint: %v, want %v", x, y, got.RGBAAt(x, y), want.RGBAAt(x, y))
			}
		}
	}
}

func BenchmarkSlideProgressRepaint(b *testing.B) {
	photo := progressPhoto(1920, 1080)
	run := func(b *testing.B, repaint func(g *goldenRig)) {
		g := newGoldenRig(b, 1920, 1080, photo, func(h *fakeHost) { h.slide = slideshowMockupView() })
		g.render()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			g.host.slide.Progress = float64(i%100) / 100
			repaint(g)
			g.eng.Flush()
			g.eng.RenderOnce()
		}
	}
	b.Run("track only", func(b *testing.B) { run(b, func(g *goldenRig) { g.root.RefreshSlideProgress() }) })
	b.Run("whole window", func(b *testing.B) { run(b, func(g *goldenRig) { g.eng.Invalidate() }) })
}

func TestSlidePanelIsReadableInEveryTheme(t *testing.T) {
	for _, th := range []*theme.Theme{theme.Dark(), theme.Light(), theme.HighContrast()} {
		for _, photo := range []color.RGBA{{0, 0, 0, 255}, {255, 255, 255, 255}, {128, 128, 128, 255}} {
			img := image.NewRGBA(image.Rect(0, 0, mockupW, mockupH))
			for i := 0; i < len(img.Pix); i += 4 {
				img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = photo.R, photo.G, photo.B, 255
			}
			g := newGoldenRig(t, mockupW, mockupH, img, func(h *fakeHost) { h.slide = slideshowMockupView() })
			g.root.Theme = th
			got := g.render()
			l := g.root.Layout()
			inside := got.RGBAAt(l.Slide.Panel.Min.X+12, l.Slide.Panel.Min.Y+8)
			label := fmt.Sprintf("%s on %v", th.Name, photo)
			if c := theme.Contrast(th.OverlayText, inside); c < 4.5 {
				t.Errorf("%s: icons and text on the panel have contrast %.2f", label, c)
			}
			trackAt := got.RGBAAt(int(l.Slide.TrackX1)-6, int(l.Slide.TrackY))
			if c := theme.Contrast(trackAt, inside); c < 1.35 {
				t.Errorf("%s: the track is almost invisible, contrast %.2f", label, c)
			}
			fillAt := got.RGBAAt(int(l.Slide.TrackX0)+8, int(l.Slide.TrackY))
			if d := channelGap(fillAt, trackAt); d < 60 {
				t.Errorf("%s: the progress fill %v does not stand out from the track %v", label, fillAt, trackAt)
			}
			arrowAt := got.RGBAAt(l.Prev.Min.X+9, (l.Prev.Min.Y+l.Prev.Max.Y)/2)
			if c := theme.Contrast(th.ArrowGlyph, arrowAt); c < 4.5 {
				t.Errorf("%s: the arrow glyph has contrast %.2f", label, c)
			}
		}
	}
}

func channelGap(a, b color.RGBA) int {
	d := func(x, y uint8) int {
		if x > y {
			return int(x - y)
		}
		return int(y - x)
	}
	return max(d(a.R, b.R), d(a.G, b.G), d(a.B, b.B))
}

func TestFullscreenKeyboardReachesTheSlideshowCommands(t *testing.T) {
	r := fullscreenRig(t, nil)
	r.host.reg.Add(commands.Command{ID: "slideshow.start", Keys: []commands.Chord{commands.C(0, commands.KeyF5)}, Run: func() { r.host.ran = append(r.host.ran, "slideshow.start") }})
	r.host.reg.Add(commands.Command{ID: "slideshow.toggle", Keys: []commands.Chord{commands.C(0, commands.KeySpace)}, Run: func() { r.host.ran = append(r.host.ran, "slideshow.toggle") }})
	r.root.HandleKey(widget.KeyEvent{Code: widget.KeyF5, Pressed: true})
	r.root.HandleKey(widget.KeyEvent{Code: widget.KeySpace, Pressed: true})
	if len(r.host.ran) != 2 || r.host.ran[0] != "slideshow.start" || r.host.ran[1] != "slideshow.toggle" {
		t.Errorf("ran %v", r.host.ran)
	}
}

func TestSlideshowViewCounterAndDigits(t *testing.T) {
	cases := []struct {
		v      SlideshowView
		text   string
		digits int
	}{
		{SlideshowView{Index: 5, Total: 24}, "5 / 24", 2},
		{SlideshowView{Index: 1, Total: 1}, "1 / 1", 1},
		{SlideshowView{Index: 120, Total: 1200}, "120 / 1200", 4},
		{SlideshowView{}, "0 / 0", 1},
		{SlideshowView{Index: 12345, Total: 9}, "12345 / 9", 5},
	}
	for _, c := range cases {
		if got := c.v.CounterText(); got != c.text {
			t.Errorf("%+v: counter %q, want %q", c.v, got, c.text)
		}
		if got := c.v.digits(); got != c.digits {
			t.Errorf("%+v: digits %d, want %d", c.v, got, c.digits)
		}
	}
}
