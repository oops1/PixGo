package app

import (
	"image"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/commands"
	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/settings"
	"github.com/oops1/PixGo/internal/ui/window"
)

type appClock struct {
	now    time.Time
	timers []*fakeTimer
}

func (c *appClock) after(d time.Duration, fn func()) func() {
	tm := &fakeTimer{at: c.now.Add(d), fn: fn}
	c.timers = append(c.timers, tm)
	return func() { tm.stopped = true }
}

func (c *appClock) advance(d time.Duration) {
	c.now = c.now.Add(d)
	for _, tm := range append([]*fakeTimer(nil), c.timers...) {
		if !tm.stopped && !tm.at.After(c.now) {
			tm.stopped = true
			tm.fn()
		}
	}
}

func (c *appClock) pending() int {
	n := 0
	for _, tm := range c.timers {
		if !tm.stopped {
			n++
		}
	}
	return n
}

type slideApp struct {
	*App
	tb    *testing.T
	win   *fakeWindow
	clock *appClock
	paths []string
}

func newSlideApp(t *testing.T, files int, mutate func(*settings.Settings)) *slideApp {
	t.Helper()
	a, _ := goldenApp(t, 327, 343, func(s *settings.Settings) {
		s.AutoHidePanels = false
		s.Slideshow.Interval = 2
		s.Slideshow.Loop = true
		if mutate != nil {
			mutate(s)
		}
	})
	w := &fakeWindow{fsAccept: true}
	a.SetOps(w.ops())
	clock := &appClock{now: time.Unix(7000, 0)}
	a.Ctl.env.After = clock.after
	a.Ctl.env.Now = func() time.Time { return clock.now }
	_, paths := makeFolder(t, files)
	if files > 0 {
		if err := a.Ctl.Open(paths[0]); err != nil {
			t.Fatal(err)
		}
		waitReady(t, a)
	}
	return &slideApp{App: a, tb: t, win: w, clock: clock, paths: paths}
}

func (s *slideApp) key(code widget.KeyCode, mod widget.KeyMod) {
	s.Root.HandleKey(widget.KeyEvent{Code: code, Mod: mod, Pressed: true})
}

func (s *slideApp) confirmFullscreen() {
	if n := len(s.win.fsCalls); n > 0 {
		s.OnFullscreenChanged(s.win.fsCalls[n-1])
	}
}

func (s *slideApp) settle() {
	s.Eng.Flush()
	waitReady(s.tb, s.App)
}

func TestF5EntersFullscreenAndStartsTheSlideshow(t *testing.T) {
	s := newSlideApp(t, 5, nil)
	s.key(widget.KeyF5, 0)
	if len(s.win.fsCalls) != 1 || !s.win.fsCalls[0] {
		t.Fatalf("F5 must ask the window for fullscreen: %v", s.win.fsCalls)
	}
	s.confirmFullscreen()
	if !s.Fullscreen() || !s.Ctl.Slides().Playing() {
		t.Fatalf("fullscreen %v playing %v", s.Fullscreen(), s.Ctl.Slides().Playing())
	}
	v := s.Slideshow()
	if !v.Active || !v.Playing || v.Index != 1 || v.Total != 5 || v.Interval != 2*time.Second {
		t.Errorf("host view %+v", v)
	}
	if s.clock.pending() != 1 {
		t.Errorf("%d timers pending, want one", s.clock.pending())
	}
}

func TestF5InFullscreenDoesNotAskForFullscreenAgain(t *testing.T) {
	s := newSlideApp(t, 3, nil)
	s.key(widget.KeyF11, 0)
	s.confirmFullscreen()
	s.win.fsCalls = nil
	s.key(widget.KeyF5, 0)
	if len(s.win.fsCalls) != 0 || !s.Ctl.Slides().Playing() {
		t.Fatalf("fs calls %v playing %v", s.win.fsCalls, s.Ctl.Slides().Playing())
	}
	s.key(widget.KeyF5, 0)
	if s.Ctl.Slides().Active() || !s.Fullscreen() || len(s.win.fsCalls) != 0 {
		t.Fatalf("F5 again must only stop the slideshow: active %v fullscreen %v calls %v", s.Ctl.Slides().Active(), s.Fullscreen(), s.win.fsCalls)
	}
}

func TestF5WhileRunningStopsAndGivesTheWindowBackWhenItEnteredFullscreen(t *testing.T) {
	s := newSlideApp(t, 3, nil)
	s.key(widget.KeyF5, 0)
	s.confirmFullscreen()
	s.key(widget.KeyF5, 0)
	if s.Ctl.Slides().Active() {
		t.Fatal("the second F5 must stop the slideshow")
	}
	if n := len(s.win.fsCalls); n != 2 || s.win.fsCalls[1] {
		t.Fatalf("the window must leave fullscreen again: %v", s.win.fsCalls)
	}
}

func TestSpaceStartsPausesAndResumes(t *testing.T) {
	s := newSlideApp(t, 4, nil)
	s.key(widget.KeySpace, 0)
	s.confirmFullscreen()
	if !s.Ctl.Slides().Playing() {
		t.Fatal("Space must start a slideshow that is not running")
	}
	s.key(widget.KeySpace, 0)
	if v := s.Slideshow(); !v.Active || v.Playing {
		t.Fatalf("paused view %+v", v)
	}
	if s.clock.pending() != 0 {
		t.Errorf("%d timers while paused", s.clock.pending())
	}
	s.key(widget.KeySpace, 0)
	if !s.Slideshow().Playing || s.clock.pending() != 1 {
		t.Errorf("resume: view %+v timers %d", s.Slideshow(), s.clock.pending())
	}
}

func TestEscapeStopsTheSlideshowAndLeavesFullscreen(t *testing.T) {
	s := newSlideApp(t, 4, nil)
	s.key(widget.KeyF5, 0)
	s.confirmFullscreen()
	s.key(widget.KeyEscape, 0)
	if s.Ctl.Slides().Active() || s.clock.pending() != 0 {
		t.Fatalf("Esc left the slideshow active=%v timers=%d", s.Ctl.Slides().Active(), s.clock.pending())
	}
	if n := len(s.win.fsCalls); n != 2 || s.win.fsCalls[1] {
		t.Fatalf("Esc must leave fullscreen: %v", s.win.fsCalls)
	}
	s.confirmFullscreen()
	if s.Fullscreen() {
		t.Error("still fullscreen")
	}
}

func TestLeavingFullscreenByAnyRouteStopsTheSlideshow(t *testing.T) {
	routes := []struct {
		name string
		do   func(s *slideApp)
	}{
		{"F11", func(s *slideApp) { s.key(widget.KeyF11, 0); s.confirmFullscreen() }},
		{"Alt+Enter", func(s *slideApp) { s.key(widget.KeyEnter, widget.ModAlt); s.confirmFullscreen() }},
		{"window reports it", func(s *slideApp) { s.OnFullscreenChanged(false) }},
		{"exit button", func(s *slideApp) {
			l := s.Root.Layout()
			x, y := l.Center(window.ZoneSlideExit)
			s.click(x, y)
			s.confirmFullscreen()
		}},
		{"close button", func(s *slideApp) {
			l := s.Root.Layout()
			x, y := l.Center(window.ZoneExit)
			s.click(x, y)
			s.confirmFullscreen()
		}},
	}
	for _, r := range routes {
		t.Run(r.name, func(t *testing.T) {
			s := newSlideApp(t, 4, nil)
			s.key(widget.KeyF5, 0)
			s.confirmFullscreen()
			r.do(s)
			if s.Fullscreen() || s.Ctl.Slides().Active() || s.clock.pending() != 0 {
				t.Errorf("fullscreen %v active %v timers %d", s.Fullscreen(), s.Ctl.Slides().Active(), s.clock.pending())
			}
		})
	}
}

func (s *slideApp) click(x, y int) {
	e := widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft, Clicks: 1, Pressed: true}
	s.Root.OnMouseButton(e)
	e.Pressed = false
	s.Root.OnMouseButton(e)
}

func TestFallbackFullscreenEscapeAlsoStopsTheSlideshow(t *testing.T) {
	s := newSlideApp(t, 3, nil)
	s.win.fsAccept = false
	s.key(widget.KeyF5, 0)
	if !s.Fullscreen() || !s.win.maximized || !s.Ctl.Slides().Playing() {
		t.Fatalf("fallback fullscreen %v maximized %v playing %v", s.Fullscreen(), s.win.maximized, s.Ctl.Slides().Playing())
	}
	s.key(widget.KeyEscape, 0)
	if s.Fullscreen() || s.win.maximized || s.Ctl.Slides().Active() {
		t.Errorf("fullscreen %v maximized %v active %v", s.Fullscreen(), s.win.maximized, s.Ctl.Slides().Active())
	}
}

func TestSlideshowCommandsAreEnabledOnlyForTwoOrMoreImages(t *testing.T) {
	cases := []struct {
		files int
		want  bool
	}{{0, false}, {1, false}, {2, true}, {24, true}}
	for _, c := range cases {
		s := newSlideApp(t, c.files, nil)
		for _, id := range []commands.ID{CmdSlideshow, CmdSlideshowToggle} {
			if got := s.Commands.Get(id).IsEnabled(); got != c.want {
				t.Errorf("%d files: %s enabled = %v, want %v", c.files, id, got, c.want)
			}
		}
		s.key(widget.KeyF5, 0)
		if started := s.Ctl.Slides().Active(); started != c.want {
			t.Errorf("%d files: F5 started = %v, want %v", c.files, started, c.want)
		}
		if !c.want && len(s.win.fsCalls) != 0 {
			t.Errorf("%d files: F5 asked for fullscreen anyway: %v", c.files, s.win.fsCalls)
		}
	}
}

func TestSlideshowIsUnavailableOnTheGalleryPage(t *testing.T) {
	s := newSlideApp(t, 4, nil)
	s.page = window.PageGallery
	if s.Commands.Get(CmdSlideshow).IsEnabled() || s.Commands.Get(CmdSlideshowToggle).IsEnabled() {
		t.Error("the slideshow must not start from the gallery")
	}
}

func TestEscapeIsEnabledForAnActiveSlideshowOrFullscreen(t *testing.T) {
	s := newSlideApp(t, 3, nil)
	esc := s.Commands.Get(CmdEscape)
	if esc.IsEnabled() {
		t.Fatal("Esc has nothing to do in a plain window")
	}
	s.key(widget.KeyF5, 0)
	if !esc.IsEnabled() {
		t.Error("Esc must be enabled while the slideshow starts")
	}
}

func TestSlideshowMenuListsIntervalsLoopAndShuffleWithTheCurrentChoiceChecked(t *testing.T) {
	cases := []struct {
		name     string
		mutate   func(*settings.Settings)
		interval int
		loop     bool
		shuffle  bool
	}{
		{"defaults of the test", nil, 2, true, false},
		{"five seconds shuffled", func(s *settings.Settings) { s.Slideshow.Interval = 5; s.Slideshow.Shuffle = true }, 5, true, true},
		{"thirty seconds without loop", func(s *settings.Settings) { s.Slideshow.Interval = 30; s.Slideshow.Loop = false }, 30, false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := newSlideApp(t, 4, c.mutate)
			s.showSlideshowMenu(image.Rect(225, 290, 265, 330))
			if !s.menu.IsOpen() {
				t.Fatal("the menu is not open")
			}
			items := s.menu.Items()
			if len(items) != len(SlideIntervals)+3 {
				t.Fatalf("%d menu items, want %d", len(items), len(SlideIntervals)+3)
			}
			for i, sec := range SlideIntervals {
				want := itoaSeconds(sec)
				if items[i].Text != want || !items[i].Checkable || items[i].Checked != (sec == c.interval) {
					t.Errorf("item %d = %+v, want %q checked=%v", i, items[i], want, sec == c.interval)
				}
			}
			n := len(SlideIntervals)
			if !items[n].Separator {
				t.Error("a separator must split the intervals from the switches")
			}
			if items[n+1].Text != "Повторять по кругу" || items[n+1].Checked != c.loop {
				t.Errorf("repeat item %+v", items[n+1])
			}
			if items[n+2].Text != "Перемешать" || items[n+2].Checked != c.shuffle {
				t.Errorf("shuffle item %+v", items[n+2])
			}
		})
	}
}

func itoaSeconds(n int) string {
	return map[int]string{1: "1 с", 3: "3 с", 5: "5 с", 10: "10 с", 30: "30 с"}[n]
}

func TestSlideshowMenuChoicesAreStoredAndApplied(t *testing.T) {
	s := newSlideApp(t, 5, nil)
	s.key(widget.KeyF5, 0)
	s.confirmFullscreen()
	s.clock.advance(1500 * time.Millisecond)

	s.showSlideshowMenu(image.Rect(225, 290, 265, 330))
	items := s.menu.Items()
	items[3].OnClick()
	if got := s.Settings.Get().Slideshow.Interval; got != 10 {
		t.Fatalf("stored interval %d, want 10", got)
	}
	if got := s.Slideshow().Interval; got != 10*time.Second {
		t.Errorf("running slideshow interval %v, want 10 s", got)
	}

	n := len(SlideIntervals)
	items = s.menu.Items()
	items[n+1].OnClick()
	if s.Settings.Get().Slideshow.Loop {
		t.Error("repeat must be switched off by the click")
	}
	items[n+2].OnClick()
	if !s.Settings.Get().Slideshow.Shuffle || !s.Ctl.Navigator().Shuffled() {
		t.Errorf("shuffle stored %v applied %v", s.Settings.Get().Slideshow.Shuffle, s.Ctl.Navigator().Shuffled())
	}
}

func TestGearOnThePanelOpensTheMenuAboveIt(t *testing.T) {
	s := newSlideApp(t, 4, nil)
	s.key(widget.KeyF5, 0)
	s.confirmFullscreen()
	l := s.Root.Layout()
	x, y := l.Center(window.ZoneSlideSettings)
	s.click(x, y)
	if !s.menu.IsOpen() {
		t.Fatal("the gear did not open the menu")
	}
	s.menu.Close()
}

func TestSlideshowMenuEntryChangesLabelWithTheState(t *testing.T) {
	s := newSlideApp(t, 4, nil)
	find := func() string {
		for _, it := range s.menuItems() {
			if it.Text == "Слайд-шоу" || it.Text == "Остановить слайд-шоу" {
				return it.Text
			}
		}
		return ""
	}
	if got := find(); got != "Слайд-шоу" {
		t.Fatalf("idle entry %q", got)
	}
	s.key(widget.KeyF5, 0)
	if got := find(); got != "Остановить слайд-шоу" {
		t.Fatalf("running entry %q", got)
	}
}

func TestPanelButtonsDriveTheSlideshow(t *testing.T) {
	s := newSlideApp(t, 4, nil)
	s.key(widget.KeyF11, 0)
	s.confirmFullscreen()
	l := s.Root.Layout()
	x, y := l.Center(window.ZoneSlidePlay)
	s.click(x, y)
	if !s.Ctl.Slides().Playing() {
		t.Fatal("the play button must start the slideshow")
	}
	s.click(x, y)
	if v := s.Slideshow(); !v.Active || v.Playing {
		t.Fatalf("the pause button: %+v", v)
	}
	s.click(x, y)
	if !s.Slideshow().Playing {
		t.Fatal("the play button must resume")
	}
}

func TestArrowsOfTheFullscreenOverlayNavigateAndRestartTheCountdown(t *testing.T) {
	s := newSlideApp(t, 5, nil)
	s.key(widget.KeyF5, 0)
	s.confirmFullscreen()
	s.clock.advance(1900 * time.Millisecond)
	l := s.Root.Layout()
	x, y := l.Center(window.ZoneNext)
	s.click(x, y)
	s.settle()
	if got := s.Ctl.Navigator().Index(); got != 1 {
		t.Fatalf("the next arrow went to %d", got)
	}
	s.clock.advance(1900 * time.Millisecond)
	s.settle()
	if got := s.Ctl.Navigator().Index(); got != 1 {
		t.Fatalf("the countdown did not restart, index %d", got)
	}
	s.clock.advance(100 * time.Millisecond)
	s.settle()
	if got := s.Ctl.Navigator().Index(); got != 2 {
		t.Fatalf("the slideshow did not continue, index %d", got)
	}
}

func TestSlideshowAtTheLastImageWithoutLoopShowsAStoppedPanel(t *testing.T) {
	s := newSlideApp(t, 3, func(st *settings.Settings) { st.Slideshow.Loop = false; st.Slideshow.Interval = 1 })
	s.key(widget.KeyF5, 0)
	s.confirmFullscreen()
	for i := 0; i < 2; i++ {
		s.clock.advance(time.Second)
		s.settle()
	}
	s.clock.advance(time.Second)
	if v := s.Slideshow(); v.Active || v.Playing || v.Index != 3 || v.Total != 3 {
		t.Errorf("view at the end: %+v", v)
	}
	if !s.Fullscreen() {
		t.Error("finishing the slideshow must stay in fullscreen")
	}
}

func TestShutdownStopsTheSlideshow(t *testing.T) {
	s := newSlideApp(t, 4, nil)
	s.key(widget.KeyF5, 0)
	s.confirmFullscreen()
	if s.clock.pending() != 1 {
		t.Fatalf("%d timers", s.clock.pending())
	}
	s.Shutdown()
	if s.clock.pending() != 0 || s.Ctl.Slides().Active() {
		t.Errorf("after Shutdown: timers %d active %v", s.clock.pending(), s.Ctl.Slides().Active())
	}
	s.clock.advance(time.Hour)
	if got := filepath.Base(s.Ctl.Path()); got != "img01.png" {
		t.Errorf("a closed app moved to %s", got)
	}
}

func TestFullscreenCanPrevCanNextFollowTheShuffledOrder(t *testing.T) {
	s := newSlideApp(t, 6, func(st *settings.Settings) { st.Slideshow.Shuffle = true })
	s.Ctl.Go(3)
	s.settle()
	s.key(widget.KeyF5, 0)
	if s.CanPrev() {
		t.Error("the shuffled slideshow starts at the first position: nothing before it")
	}
	if !s.CanNext() {
		t.Error("the shuffled slideshow has more images ahead")
	}
	s.clock.advance(2 * time.Second)
	s.settle()
	if !s.CanPrev() {
		t.Error("after one step there is an image behind")
	}
}

func TestFullscreenPanelIsDrawnOverThePhotoThroughTheWholeStack(t *testing.T) {
	s := newSlideApp(t, 5, nil)
	s.key(widget.KeyF5, 0)
	s.confirmFullscreen()
	s.Root.Refresh()
	s.Eng.Flush()
	got := s.Eng.RenderOnce()
	l := s.Root.Layout()
	inside := got.RGBAAt(l.Slide.Panel.Min.X+10, l.Slide.Panel.Min.Y+6)
	above := got.RGBAAt(l.Slide.Panel.Min.X+10, l.Slide.Panel.Min.Y-6)
	if inside == above {
		t.Errorf("the panel is not drawn: %v inside, %v above", inside, above)
	}
	x, y := l.Center(window.ZoneSlidePlay)
	glyph := got.RGBAAt(x-4, y)
	if glyph.R < 200 || glyph.G < 200 || glyph.B < 200 {
		t.Errorf("the pause glyph is not drawn at the play button: %v", glyph)
	}
	ex, ey := l.Center(window.ZoneExit)
	if c := got.RGBAAt(ex, ey); c == got.RGBAAt(ex-12, ey-12) {
		t.Errorf("the close cross is not drawn: %v", c)
	}
}

func TestSlideshowTextsAreTranslated(t *testing.T) {
	keys := []string{
		"cmd.slideshow", "cmd.slideshowPause", "cmd.slideshowResume", "cmd.slideshowStop",
		"slideshow.settings", "slideshow.seconds", "slideshow.counter", "slideshow.repeat", "slideshow.mix",
		"cmd.fullscreenExit",
	}
	for _, lang := range []string{"ru", "en"} {
		have := i18n.Keys(lang)
		for _, k := range keys {
			if !slices.Contains(have, k) {
				t.Errorf("%s: missing %q", lang, k)
			}
		}
	}
}
