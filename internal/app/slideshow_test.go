package app

import (
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/oops1/PixGo/internal/settings"
)

type slideRig struct {
	*harness
	paths    []string
	progress int
	states   int
	visible  bool
}

func newSlideRig(t *testing.T, files int, mutate func(*settings.Settings)) *slideRig {
	t.Helper()
	h := newHarness(t, func(s *settings.Settings) {
		s.Slideshow.Interval = 2
		s.Slideshow.Loop = true
		s.Slideshow.Shuffle = false
		if mutate != nil {
			mutate(s)
		}
	})
	_, paths := makeFolder(t, files)
	if err := h.c.Open(paths[0]); err != nil {
		t.Fatal(err)
	}
	h.pump(func() bool { return h.c.Status() == StatusReady })
	r := &slideRig{harness: h, paths: paths}
	s := h.c.Slides()
	s.OnProgress = func() { r.progress++ }
	s.OnState = func() { r.states++ }
	s.Visible = func() bool { return r.visible }
	return r
}

func (r *slideRig) settle() {
	r.t.Helper()
	r.pump(func() bool { return r.c.Status() != StatusLoading })
}

func (r *slideRig) run(total, step time.Duration) {
	r.t.Helper()
	for elapsed := time.Duration(0); elapsed < total; elapsed += step {
		r.advance(step)
		r.settle()
	}
}

func (r *slideRig) index() int { return r.c.Navigator().Index() }

func TestSlideshowAdvancesAfterEachInterval(t *testing.T) {
	cases := []struct {
		name     string
		interval int
		steps    []time.Duration
		want     []int
	}{
		{"two seconds", 2, []time.Duration{time.Second, time.Second, 2 * time.Second, 2 * time.Second}, []int{0, 1, 2, 3}},
		{"one second", 1, []time.Duration{time.Second, time.Second, time.Second}, []int{1, 2, 3}},
		{"thirty seconds", 30, []time.Duration{29 * time.Second, time.Second}, []int{0, 1}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newSlideRig(t, 6, func(s *settings.Settings) { s.Slideshow.Interval = c.interval })
			if !r.c.Slides().Start() || !r.c.Slides().Playing() {
				t.Fatal("slideshow did not start")
			}
			for i, d := range c.steps {
				r.advance(d)
				r.settle()
				if got := r.index(); got != c.want[i] {
					t.Fatalf("after step %d (%v): index %d, want %d", i, d, got, c.want[i])
				}
			}
		})
	}
}

func TestSlideshowDoesNotStartWithoutTwoImages(t *testing.T) {
	for _, files := range []int{1, 2} {
		r := newSlideRig(t, files, nil)
		if got := r.c.Slides().Start(); got != (files >= 2) {
			t.Errorf("%d files: Start() = %v", files, got)
		}
		if got := r.c.Slides().CanStart(); got != (files >= 2) {
			t.Errorf("%d files: CanStart() = %v", files, got)
		}
		if files < 2 && r.pendingTimers() != 0 {
			t.Errorf("a refused start left %d timers", r.pendingTimers())
		}
	}
	empty := newHarness(t, nil)
	if empty.c.Slides().Start() || empty.c.Slides().Active() {
		t.Error("a slideshow cannot start without a folder")
	}
}

func TestSlideshowPauseDoesNotAccumulateTime(t *testing.T) {
	r := newSlideRig(t, 5, nil)
	s := r.c.Slides()
	s.Start()
	r.advance(time.Second)
	s.TogglePause()
	if s.Playing() || !s.Active() || r.pendingTimers() != 0 {
		t.Fatalf("paused: playing %v active %v timers %d", s.Playing(), s.Active(), r.pendingTimers())
	}
	r.advance(10 * time.Minute)
	r.settle()
	if r.index() != 0 {
		t.Fatalf("a paused slideshow moved to %d", r.index())
	}
	if p := s.View().Progress; p < .49 || p > .51 {
		t.Errorf("paused progress %.2f, want 0.5", p)
	}
	s.TogglePause()
	r.advance(900 * time.Millisecond)
	r.settle()
	if r.index() != 0 {
		t.Fatalf("resumed too early: index %d", r.index())
	}
	r.advance(100 * time.Millisecond)
	r.settle()
	if r.index() != 1 {
		t.Fatalf("resumed slideshow did not use up the remaining second: index %d", r.index())
	}
}

func TestSlideshowEndsOrWrapsAtTheLastImage(t *testing.T) {
	cases := []struct {
		name       string
		loop       bool
		wantIndex  int
		wantActive bool
		wantTimers int
		manualLoop bool
	}{
		{"loop on wraps to the first", true, 0, true, 1, false},
		{"loop off stops on the last", false, 3, false, 0, false},
		{"loop off ignores the folder loop setting", false, 3, false, 0, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newSlideRig(t, 4, func(s *settings.Settings) {
				s.Slideshow.Loop = c.loop
				s.Slideshow.Interval = 1
				s.LoopFolder = c.manualLoop
			})
			r.c.Go(2)
			r.settle()
			r.c.Slides().Start()
			r.run(time.Second, time.Second)
			if r.index() != 3 {
				t.Fatalf("expected the last image first, got %d", r.index())
			}
			r.run(time.Second, time.Second)
			if r.index() != c.wantIndex || r.c.Slides().Active() != c.wantActive || r.pendingTimers() != c.wantTimers {
				t.Errorf("index %d active %v timers %d, want %d %v %d", r.index(), r.c.Slides().Active(), r.pendingTimers(), c.wantIndex, c.wantActive, c.wantTimers)
			}
		})
	}
}

func TestSlideshowRestartedAtTheEndWithoutLoopBeginsFromTheFirstImage(t *testing.T) {
	r := newSlideRig(t, 4, func(s *settings.Settings) { s.Slideshow.Loop = false })
	r.c.Last()
	r.settle()
	if !r.c.Slides().Start() || r.index() != 0 {
		t.Fatalf("started at index %d, want a restart from the first image", r.index())
	}
	r.run(2*time.Second, 2*time.Second)
	if r.index() != 1 {
		t.Errorf("index %d after one interval", r.index())
	}
}

func TestSlideshowShuffleShowsEveryFileOncePerCycle(t *testing.T) {
	const files = 7
	for seed := 0; seed < 6; seed++ {
		r := newSlideRig(t, files, func(s *settings.Settings) {
			s.Slideshow.Shuffle = true
			s.Slideshow.Interval = 1
		})
		start := r.index()
		r.c.Slides().Start()
		played := []int{start}
		for i := 0; i < 3*files-1; i++ {
			r.run(time.Second, time.Second)
			played = append(played, r.index())
		}
		if played[0] != start {
			t.Fatalf("the current file must stay first, got %v", played)
		}
		for cycle := 0; cycle < 3; cycle++ {
			chunk := slices.Clone(played[cycle*files : (cycle+1)*files])
			slices.Sort(chunk)
			if !slices.Equal(chunk, []int{0, 1, 2, 3, 4, 5, 6}) {
				t.Fatalf("seed %d cycle %d does not show every file exactly once: %v", seed, cycle, played)
			}
		}
		for i := 1; i < len(played); i++ {
			if played[i] == played[i-1] {
				t.Fatalf("file %d repeated back to back: %v", played[i], played)
			}
		}
	}
}

func TestSlideshowShuffleEndsWithTheSlideshow(t *testing.T) {
	r := newSlideRig(t, 5, func(s *settings.Settings) { s.Slideshow.Shuffle = true })
	r.c.Slides().Start()
	if !r.c.Navigator().Shuffled() {
		t.Fatal("the slideshow must shuffle the folder when asked to")
	}
	r.c.Slides().Stop()
	if r.c.Navigator().Shuffled() {
		t.Fatal("manual navigation goes back to the folder order after the slideshow")
	}
	r.c.Step(1)
	if got := r.index(); got != 1 {
		t.Errorf("next after the slideshow = %d, want 1", got)
	}
}

func TestSlideshowManualNavigationFollowsTheShuffledOrderAndRestartsTheCountdown(t *testing.T) {
	r := newSlideRig(t, 8, func(s *settings.Settings) { s.Slideshow.Shuffle = true })
	r.c.Slides().Start()
	order := r.c.Navigator().Order()
	r.advance(1500 * time.Millisecond)
	r.c.Step(1)
	r.settle()
	if r.index() != order[1] {
		t.Fatalf("manual next went to %d, want %d from the shuffled order %v", r.index(), order[1], order)
	}
	r.advance(1900 * time.Millisecond)
	r.settle()
	if r.index() != order[1] {
		t.Fatalf("the countdown was not restarted by the manual step: index %d", r.index())
	}
	r.advance(100 * time.Millisecond)
	r.settle()
	if r.index() != order[2] {
		t.Fatalf("automatic step went to %d, want %d", r.index(), order[2])
	}
	r.c.Step(-1)
	r.settle()
	if r.index() != order[1] {
		t.Errorf("manual previous went to %d, want %d", r.index(), order[1])
	}
}

func TestSlideshowAnyNavigationRestartsTheCountdown(t *testing.T) {
	actions := []struct {
		name string
		do   func(r *slideRig)
		want int
	}{
		{"next", func(r *slideRig) { r.c.Step(1) }, 1},
		{"previous", func(r *slideRig) { r.c.Go(2); r.c.Step(-1) }, 1},
		{"first", func(r *slideRig) { r.c.Go(2); r.c.First() }, 0},
		{"last", func(r *slideRig) { r.c.Last() }, 3},
		{"go", func(r *slideRig) { r.c.Go(2) }, 2},
	}
	for _, a := range actions {
		t.Run(a.name, func(t *testing.T) {
			r := newSlideRig(t, 4, nil)
			r.c.Slides().Start()
			r.advance(1900 * time.Millisecond)
			a.do(r)
			r.settle()
			if r.index() != a.want {
				t.Fatalf("action left index %d, want %d", r.index(), a.want)
			}
			r.advance(1900 * time.Millisecond)
			r.settle()
			if r.index() != a.want {
				t.Fatalf("the slideshow moved on after only 1.9 s: index %d", r.index())
			}
			if p := r.c.Slides().View().Progress; p < .9 || p > 1 {
				t.Errorf("progress %.2f, want about 0.95", p)
			}
			r.advance(100 * time.Millisecond)
			r.settle()
			if r.index() == a.want {
				t.Errorf("the slideshow did not advance when the new interval ran out")
			}
		})
	}
}

func TestSlideshowIntervalChangeRestartsTheCountdown(t *testing.T) {
	r := newSlideRig(t, 5, nil)
	s := r.c.Slides()
	s.Start()
	r.advance(1500 * time.Millisecond)
	s.SetInterval(10)
	if got := s.View().Interval; got != 10*time.Second {
		t.Fatalf("interval %v", got)
	}
	if got := r.c.env.Settings.Get().Slideshow.Interval; got != 10 {
		t.Fatalf("the choice must be stored in the settings, got %d", got)
	}
	r.advance(9 * time.Second)
	r.settle()
	if r.index() != 0 {
		t.Fatalf("moved after 9 s of a 10 s interval: %d", r.index())
	}
	r.advance(time.Second)
	r.settle()
	if r.index() != 1 {
		t.Fatalf("did not move after 10 s: %d", r.index())
	}
	if r.pendingTimers() != 1 {
		t.Errorf("%d timers pending, want exactly one", r.pendingTimers())
	}
}

func TestSlideshowFollowsSettingsChangedElsewhere(t *testing.T) {
	r := newSlideRig(t, 5, nil)
	s := r.c.Slides()
	s.Start()
	r.c.env.Settings.Update(func(st *settings.Settings) { st.Slideshow.Interval = 4; st.Slideshow.Shuffle = true })
	r.pump(func() bool { return s.View().Interval == 4*time.Second })
	if !r.c.Navigator().Shuffled() {
		t.Error("turning shuffle on while the slideshow runs must shuffle at once")
	}
	r.c.env.Settings.Update(func(st *settings.Settings) { st.Slideshow.Shuffle = false })
	r.pump(func() bool { return !r.c.Navigator().Shuffled() })
}

func TestSlideshowProgressTicksFourTimesASecondOnlyWhileThePanelIsVisible(t *testing.T) {
	cases := []struct {
		name    string
		visible bool
		min     int
		max     int
	}{
		{"panel visible", true, 6, 8},
		{"panel hidden", false, 0, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newSlideRig(t, 5, func(s *settings.Settings) { s.Slideshow.Interval = 10 })
			r.visible = c.visible
			r.c.Slides().Start()
			r.progress = 0
			r.run(2*time.Second, 10*time.Millisecond)
			if r.progress < c.min || r.progress > c.max {
				t.Errorf("%d progress repaints in two seconds, want %d..%d", r.progress, c.min, c.max)
			}
			if r.pendingTimers() > 1 {
				t.Errorf("%d timers pending", r.pendingTimers())
			}
		})
	}
}

func TestSlideshowPanelAppearingStartsTheProgressTicks(t *testing.T) {
	r := newSlideRig(t, 5, func(s *settings.Settings) { s.Slideshow.Interval = 10 })
	r.c.Slides().Start()
	r.run(time.Second, 100*time.Millisecond)
	if r.progress != 0 {
		t.Fatalf("progress repainted %d times while the panel was hidden", r.progress)
	}
	r.visible = true
	r.c.Slides().Replan()
	r.run(time.Second, 50*time.Millisecond)
	if r.progress < 3 {
		t.Errorf("only %d progress repaints after the panel appeared", r.progress)
	}
}

func TestSlideshowSwitchesExactlyOnTimeEvenWhenTicksAreFrequent(t *testing.T) {
	r := newSlideRig(t, 5, func(s *settings.Settings) { s.Slideshow.Interval = 1 })
	r.visible = true
	r.c.Slides().Start()
	for i := 1; i <= 12; i++ {
		r.run(250*time.Millisecond, 250*time.Millisecond)
		if want := i / 4; r.index() != want {
			t.Fatalf("at %v: index %d, want %d", time.Duration(i)*250*time.Millisecond, r.index(), want)
		}
	}
}

func TestSlideshowStopCancelsTheTimerAndKeepsTheImage(t *testing.T) {
	r := newSlideRig(t, 5, nil)
	s := r.c.Slides()
	s.Start()
	r.advance(time.Second)
	s.Stop()
	if s.Active() || s.Playing() || r.pendingTimers() != 0 {
		t.Fatalf("stopped: active %v playing %v timers %d", s.Active(), s.Playing(), r.pendingTimers())
	}
	r.advance(time.Hour)
	r.settle()
	if r.index() != 0 {
		t.Fatalf("a stopped slideshow moved to %d", r.index())
	}
	if v := s.View(); v.Progress != 0 || v.Active {
		t.Errorf("view after stop: %+v", v)
	}
	s.Stop()
	if !s.Start() || !s.Playing() {
		t.Error("a stopped slideshow must start again")
	}
}

func TestClosingTheControllerStopsTheSlideshowTimers(t *testing.T) {
	r := newSlideRig(t, 5, nil)
	r.visible = true
	s := r.c.Slides()
	s.Start()
	if r.pendingTimers() != 1 {
		t.Fatalf("a running slideshow keeps %d timers, want one", r.pendingTimers())
	}
	r.c.Close()
	if r.pendingTimers() != 0 {
		t.Fatalf("%d timers survived Close", r.pendingTimers())
	}
	if s.Start() || s.Active() {
		t.Error("a closed slideshow must not start")
	}
	r.advance(time.Hour)
	if r.index() != 0 {
		t.Error("a closed slideshow moved")
	}
	r.c.env.Settings.Update(func(st *settings.Settings) { st.Slideshow.Interval = 7 })
	r.pump(func() bool { return true })
}

func TestSlideshowOpeningAnotherFolderRestartsAndReshuffles(t *testing.T) {
	r := newSlideRig(t, 6, func(s *settings.Settings) { s.Slideshow.Shuffle = true })
	r.c.Slides().Start()
	r.advance(1500 * time.Millisecond)
	_, other := makeFolder(t, 9)
	if err := r.c.Open(other[3]); err != nil {
		t.Fatal(err)
	}
	r.settle()
	if !r.c.Navigator().Shuffled() || len(r.c.Navigator().Order()) != 9 || r.c.Navigator().Order()[0] != 3 {
		t.Fatalf("the new folder must be shuffled with the opened file first: %v", r.c.Navigator().Order())
	}
	r.advance(1900 * time.Millisecond)
	r.settle()
	if r.c.Name() != filepath.Base(other[3]) {
		t.Fatalf("the countdown must restart for a newly opened file, now showing %s", r.c.Name())
	}
}

func TestSlideshowViewReportsPositionInPlayOrderWhenShuffled(t *testing.T) {
	r := newSlideRig(t, 8, func(s *settings.Settings) { s.Slideshow.Shuffle = true })
	s := r.c.Slides()
	if v := s.View(); v.Index != 1 || v.Total != 8 || v.Active {
		t.Fatalf("idle view %+v", v)
	}
	s.Start()
	r.run(2*time.Second, 2*time.Second)
	if v := s.View(); v.Index != 2 || v.Total != 8 || !v.Active || !v.Playing {
		t.Errorf("view after one step %+v, want position 2 of 8", v)
	}
}
