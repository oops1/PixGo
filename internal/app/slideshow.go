package app

import (
	"time"

	"github.com/oops1/PixGo/internal/settings"
	"github.com/oops1/PixGo/internal/viewer"
)

const progressStep = 250 * time.Millisecond

var SlideIntervals = []int{1, 3, 5, 10, 30}

type SlideState struct {
	Active   bool
	Playing  bool
	Index    int
	Total    int
	Progress float64
	Interval time.Duration
}

type Slides struct {
	ctl       *Controller
	model     *viewer.Slideshow
	active    bool
	advancing bool
	closed    bool
	cancel    func()
	gen       int
	unsub     func()

	Visible    func() bool
	OnState    func()
	OnProgress func()
}

func newSlides(c *Controller) *Slides {
	s := &Slides{ctl: c}
	s.model = viewer.NewSlideshow(s.configured().interval())
	if c.env.Settings != nil {
		s.unsub = c.env.Settings.Subscribe(func(old, cur settings.Settings) {
			if c.env.Post != nil {
				c.env.Post(s.syncSettings)
			}
		})
	}
	return s
}

type slideSettings settings.Slideshow

func (s slideSettings) interval() time.Duration { return time.Duration(s.Interval) * time.Second }

func (s *Slides) configured() slideSettings {
	return slideSettings(s.ctl.env.Settings.Get().Slideshow)
}

func (s *Slides) now() time.Time { return s.ctl.env.Now() }

func (s *Slides) Active() bool { return s.active }

func (s *Slides) Playing() bool { return s.active && s.model.Running() }

func (s *Slides) CanStart() bool {
	n := s.ctl.Navigator()
	return !s.closed && n != nil && n.Count() >= 2
}

func (s *Slides) Start() bool {
	if !s.CanStart() {
		return false
	}
	if s.active {
		if !s.model.Running() {
			s.model.Start(s.now())
			s.plan()
			s.stateChanged()
		}
		return true
	}
	cfg := s.configured()
	s.model = viewer.NewSlideshow(cfg.interval())
	s.active = true
	s.ctl.SetShuffle(cfg.Shuffle)
	if !cfg.Loop && s.ctl.Navigator().AtEnd() {
		s.advancing = true
		s.ctl.First()
		s.advancing = false
	}
	s.model.Start(s.now())
	s.plan()
	s.stateChanged()
	return true
}

func (s *Slides) Stop() {
	if !s.active {
		return
	}
	s.active = false
	s.model.Stop()
	s.cancelTimer()
	s.ctl.SetShuffle(false)
	s.stateChanged()
}

func (s *Slides) TogglePause() {
	if !s.active {
		s.Start()
		return
	}
	s.model.Toggle(s.now())
	s.plan()
	s.stateChanged()
}

func (s *Slides) SetInterval(seconds int) {
	if s.ctl.env.Settings != nil {
		s.ctl.env.Settings.Update(func(st *settings.Settings) { st.Slideshow.Interval = seconds })
	}
	s.syncSettings()
}

func (s *Slides) SetLoop(on bool) {
	if s.ctl.env.Settings != nil {
		s.ctl.env.Settings.Update(func(st *settings.Settings) { st.Slideshow.Loop = on })
	}
}

func (s *Slides) SetShuffle(on bool) {
	if s.ctl.env.Settings != nil {
		s.ctl.env.Settings.Update(func(st *settings.Settings) { st.Slideshow.Shuffle = on })
	}
	s.syncSettings()
}

func (s *Slides) View() SlideState {
	idx, total := s.ctl.Position()
	if n := s.ctl.Navigator(); n != nil && n.Shuffled() {
		idx = n.Position() + 1
	}
	v := SlideState{Active: s.active, Playing: s.Playing(), Index: idx, Total: total, Interval: s.configured().interval()}
	if s.active {
		v.Interval = s.model.Interval()
		v.Progress = s.model.Progress(s.now())
	}
	return v
}

func (s *Slides) Replan() {
	if s.active {
		s.plan()
	}
}

func (s *Slides) Close() {
	if s.closed {
		return
	}
	s.closed = true
	s.active = false
	s.cancelTimer()
	if s.unsub != nil {
		s.unsub()
		s.unsub = nil
	}
}

func (s *Slides) fileChanged() {
	if !s.active || s.advancing || s.closed {
		return
	}
	s.model.Advance(s.now())
	s.plan()
	s.progressChanged()
}

func (s *Slides) syncSettings() {
	if !s.active || s.closed {
		return
	}
	cfg := s.configured()
	changed := false
	if cfg.interval() != s.model.Interval() {
		s.model.SetInterval(cfg.interval(), s.now())
		changed = true
	}
	if cfg.Shuffle != s.ctl.shuffle {
		s.ctl.SetShuffle(cfg.Shuffle)
		changed = true
	}
	if changed {
		s.plan()
		s.stateChanged()
	}
}

func (s *Slides) plan() {
	s.cancelTimer()
	if !s.active || s.closed || !s.model.Running() || s.ctl.env.After == nil {
		return
	}
	wait := s.model.Remaining(s.now())
	if s.Visible != nil && s.Visible() {
		wait = min(wait, progressStep)
	}
	gen := s.gen
	s.cancel = s.ctl.env.After(wait, func() {
		if gen != s.gen {
			return
		}
		s.cancel = nil
		s.tick()
	})
}

func (s *Slides) cancelTimer() {
	s.gen++
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
}

func (s *Slides) tick() {
	if !s.active || s.closed {
		return
	}
	now := s.now()
	if s.model.Due(now) {
		s.advance(now)
		return
	}
	s.progressChanged()
	s.plan()
}

func (s *Slides) advance(now time.Time) {
	s.advancing = true
	moved := s.ctl.Advance(s.configured().Loop)
	s.advancing = false
	if !moved {
		s.Stop()
		return
	}
	s.model.Advance(now)
	s.plan()
	s.progressChanged()
}

func (s *Slides) stateChanged() {
	if s.OnState != nil {
		s.OnState()
	}
}

func (s *Slides) progressChanged() {
	if s.OnProgress != nil {
		s.OnProgress()
	}
}
