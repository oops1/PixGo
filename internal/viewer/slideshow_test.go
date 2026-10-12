package viewer

import (
	"testing"
	"time"
)

var t0 = time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)

func near(a, b, tol float64) bool {
	d := a - b
	return d <= tol && d >= -tol
}

func TestSlideshowIdleByDefault(t *testing.T) {
	s := NewSlideshow(4 * time.Second)
	if s.Running() || s.Due(t0.Add(time.Hour)) || s.Progress(t0.Add(time.Hour)) != 0 {
		t.Fatal("a new slideshow must not run")
	}
	if s.Remaining(t0) != 4*time.Second || s.Interval() != 4*time.Second {
		t.Fatalf("remaining %v interval %v", s.Remaining(t0), s.Interval())
	}
}

func TestSlideshowProgressAndDue(t *testing.T) {
	s := NewSlideshow(4 * time.Second)
	s.Start(t0)
	if !s.Running() {
		t.Fatal("Start")
	}
	cases := []struct {
		at        time.Duration
		progress  float64
		due       bool
		remaining time.Duration
	}{
		{0, 0, false, 4 * time.Second},
		{time.Second, 0.25, false, 3 * time.Second},
		{2 * time.Second, 0.5, false, 2 * time.Second},
		{4*time.Second - time.Nanosecond, 1, false, time.Nanosecond},
		{4 * time.Second, 1, true, 0},
		{9 * time.Second, 1, true, 0},
	}
	for _, c := range cases {
		now := t0.Add(c.at)
		if got := s.Progress(now); !near(got, c.progress, 1e-6) {
			t.Errorf("at %v: progress %v, want %v", c.at, got, c.progress)
		}
		if got := s.Due(now); got != c.due {
			t.Errorf("at %v: due %v", c.at, got)
		}
		if got := s.Remaining(now); got != c.remaining {
			t.Errorf("at %v: remaining %v, want %v", c.at, got, c.remaining)
		}
	}
}

func TestSlideshowAdvanceRestartsTimer(t *testing.T) {
	s := NewSlideshow(3 * time.Second)
	s.Start(t0)
	now := t0.Add(3 * time.Second)
	if !s.Due(now) {
		t.Fatal("slide is due")
	}
	s.Advance(now)
	if s.Due(now) || s.Progress(now) != 0 || !s.Running() {
		t.Fatal("Advance must restart the slide timer and keep running")
	}
	if s.Due(now.Add(2*time.Second)) || !s.Due(now.Add(3*time.Second)) {
		t.Fatal("next slide timing")
	}
}

func TestSlideshowPauseResumeKeepsElapsed(t *testing.T) {
	s := NewSlideshow(10 * time.Second)
	s.Start(t0)
	s.Pause(t0.Add(4 * time.Second))
	if s.Running() {
		t.Fatal("Pause")
	}
	later := t0.Add(time.Minute)
	if s.Due(later) || !near(s.Progress(later), 0.4, 1e-9) || s.Remaining(later) != 6*time.Second {
		t.Fatalf("paused state: progress %v remaining %v", s.Progress(later), s.Remaining(later))
	}
	s.Start(later)
	if !near(s.Progress(later), 0.4, 1e-9) {
		t.Fatalf("resume progress %v", s.Progress(later))
	}
	if s.Due(later.Add(5*time.Second)) || !s.Due(later.Add(6*time.Second)) {
		t.Fatal("resumed timing")
	}
}

func TestSlideshowToggle(t *testing.T) {
	s := NewSlideshow(time.Second)
	s.Toggle(t0)
	if !s.Running() {
		t.Fatal("first toggle starts")
	}
	s.Toggle(t0.Add(500 * time.Millisecond))
	if s.Running() || !near(s.Progress(t0.Add(time.Hour)), 0.5, 1e-9) {
		t.Fatal("second toggle pauses")
	}
	s.Toggle(t0.Add(time.Hour))
	if !s.Running() {
		t.Fatal("third toggle resumes")
	}
}

func TestSlideshowStartPauseAreIdempotent(t *testing.T) {
	s := NewSlideshow(10 * time.Second)
	s.Pause(t0)
	s.Start(t0)
	s.Start(t0.Add(5 * time.Second))
	if !near(s.Progress(t0.Add(5*time.Second)), 0.5, 1e-9) {
		t.Fatal("a second Start must not restart the timer")
	}
	s.Pause(t0.Add(5 * time.Second))
	s.Pause(t0.Add(9 * time.Second))
	if !near(s.Progress(t0.Add(time.Hour)), 0.5, 1e-9) {
		t.Fatal("a second Pause must not move the pause point")
	}
}

func TestSlideshowSetIntervalRestartsSlide(t *testing.T) {
	s := NewSlideshow(5 * time.Second)
	s.Start(t0)
	now := t0.Add(4 * time.Second)
	s.SetInterval(2*time.Second, now)
	if s.Interval() != 2*time.Second || s.Due(now) || s.Progress(now) != 0 {
		t.Fatal("SetInterval must give the current slide a fresh interval")
	}
	if !s.Due(now.Add(2 * time.Second)) {
		t.Fatal("new interval not honoured")
	}
	s.Pause(now.Add(time.Second))
	s.SetInterval(8*time.Second, now.Add(time.Second))
	if s.Progress(now.Add(time.Hour)) != 0 || s.Running() {
		t.Fatal("SetInterval while paused keeps it paused at zero")
	}
}

func TestSlideshowClampsAndDefaults(t *testing.T) {
	s := NewSlideshow(0)
	if s.Interval() != DefaultSlideInterval {
		t.Fatalf("interval %v", s.Interval())
	}
	s.SetInterval(-time.Second, t0)
	if s.Interval() != DefaultSlideInterval {
		t.Fatalf("interval %v", s.Interval())
	}
	s.Start(t0)
	if got := s.Progress(t0.Add(-time.Hour)); got != 0 {
		t.Fatalf("clock going backwards must clamp progress, got %v", got)
	}
	if s.Due(t0.Add(-time.Hour)) {
		t.Fatal("clock going backwards")
	}
	if s.Remaining(t0.Add(-time.Hour)) != DefaultSlideInterval {
		t.Fatal("remaining under backwards clock")
	}
}

func TestSlideshowPauseNeverExceedsInterval(t *testing.T) {
	s := NewSlideshow(2 * time.Second)
	s.Start(t0)
	s.Pause(t0.Add(time.Hour))
	if s.Progress(t0) != 1 || s.Remaining(t0) != 0 {
		t.Fatalf("progress %v remaining %v", s.Progress(t0), s.Remaining(t0))
	}
}
