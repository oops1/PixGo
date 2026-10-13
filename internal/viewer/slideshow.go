package viewer

import "time"

const DefaultSlideInterval = 5 * time.Second

type Slideshow struct {
	interval time.Duration
	running  bool
	start    time.Time
	elapsed  time.Duration
}

func NewSlideshow(interval time.Duration) *Slideshow {
	return &Slideshow{interval: sanitizeInterval(interval)}
}

func (s *Slideshow) Interval() time.Duration { return s.interval }

func (s *Slideshow) Running() bool { return s.running }

func (s *Slideshow) Start(now time.Time) {
	if s.running {
		return
	}
	s.running = true
	s.start = now.Add(-s.elapsed)
}

func (s *Slideshow) Pause(now time.Time) {
	if !s.running {
		return
	}
	s.elapsed = min(s.spent(now), s.interval)
	s.running = false
}

func (s *Slideshow) Stop() {
	s.running = false
	s.elapsed = 0
}

func (s *Slideshow) Toggle(now time.Time) {
	if s.running {
		s.Pause(now)
		return
	}
	s.Start(now)
}

func (s *Slideshow) SetInterval(d time.Duration, now time.Time) {
	s.interval = sanitizeInterval(d)
	s.restart(now)
}

func (s *Slideshow) Progress(now time.Time) float64 {
	return min(max(float64(s.spent(now))/float64(s.interval), 0), 1)
}

func (s *Slideshow) Due(now time.Time) bool {
	return s.running && s.spent(now) >= s.interval
}

func (s *Slideshow) Advance(now time.Time) { s.restart(now) }

func (s *Slideshow) Remaining(now time.Time) time.Duration {
	return max(s.interval-s.spent(now), 0)
}

func (s *Slideshow) restart(now time.Time) {
	s.start = now
	s.elapsed = 0
}

func (s *Slideshow) spent(now time.Time) time.Duration {
	if !s.running {
		return s.elapsed
	}
	return max(now.Sub(s.start), 0)
}

func sanitizeInterval(d time.Duration) time.Duration {
	if d <= 0 {
		return DefaultSlideInterval
	}
	return d
}
