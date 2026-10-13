package window

import (
	"image"
	"image/color"
	"math"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/ui/gfx"
)

const (
	slideTrackLeft  = 97.4
	slideTrackRight = 90.5
	slideTrackMin   = 8
)

type Slide struct {
	Panel       image.Rectangle
	Play        image.Rectangle
	Settings    image.Rectangle
	Exit        image.Rectangle
	CounterX    int
	CounterBase int
	TrackX0     float64
	TrackX1     float64
	TrackY      float64
	Damage      image.Rectangle
}

func (s Slide) HasTrack() bool { return s.TrackX1-s.TrackX0 >= slideTrackMin }

func (s Slide) KnobX(progress float64) float64 {
	return s.TrackX0 + (s.TrackX1-s.TrackX0)*min(max(progress, 0), 1)
}

func slideGeometry(w, h, extra int) Slide {
	extra = max(0, extra)
	pw := min(SlideBarWidth+extra, max(0, w-2*SlideBarMargin))
	left := (w - pw + 1) / 2
	top := h - SlideBarBottom - SlideBarHeight
	panel := image.Rect(left, top, left+pw, top+SlideBarHeight)
	cy := top + SlideBarHeight/2
	button := func(cx int) image.Rectangle {
		return image.Rect(cx-SlideButtonSize/2, cy-SlideButtonSize/2, cx-SlideButtonSize/2+SlideButtonSize, cy-SlideButtonSize/2+SlideButtonSize)
	}
	s := Slide{
		Panel:       panel,
		Play:        button(panel.Min.X + SlidePlayCenter),
		Settings:    button(panel.Max.X - SlideSettingsCenter),
		Exit:        button(panel.Max.X - SlideExitCenter),
		CounterX:    panel.Min.X + SlideCounterX,
		CounterBase: top + SlideCounterBase,
		TrackX0:     float64(panel.Min.X) + slideTrackLeft + float64(extra),
		TrackX1:     float64(panel.Max.X) - slideTrackRight,
		TrackY:      float64(cy),
	}
	r := SlideKnobRadius + 1
	s.Damage = image.Rect(int(math.Floor(s.TrackX0))-r, cy-r, int(math.Ceil(s.TrackX1))+r, cy+r)
	return s
}

var slideCounterFont = gfx.UI(9)

func capsuleDistance(x0, x1, y, radius float64) gfx.Distance {
	a, b := x0+radius, x1-radius
	return func(px, py float64) float64 {
		cx := math.Max(a, math.Min(b, px))
		return math.Hypot(px-cx, py-y) - radius
	}
}

func (r *Root) drawSlide(c widget.DrawContext, l Layout) {
	t := r.Theme
	sl := l.Slide
	if sl.Panel.Empty() || !hasSlidePanel(l) {
		return
	}
	v := r.host.Slideshow()
	gfx.FillRound(c, sl.Panel, SlideBarRadius, t.SlideBar)
	gfx.StrokeRound(c, sl.Panel, SlideBarRadius, 1, t.SlideBarEdge)
	for _, z := range l.Zones {
		if z.Kind >= ZoneSlidePlay && z.Kind <= ZoneSlideExit {
			r.drawSlideButton(c, z, v)
		}
	}
	limit := int(math.Floor(sl.TrackX0)) - slideCounterGap - sl.CounterX
	if limit > 0 {
		slideCounterFont.DrawFit(c, v.CounterText(), sl.CounterX, sl.CounterBase+textLift(slideCounterFont), limit, t.OverlayText)
	}
	r.drawSlideTrack(c, sl, v)
}

func hasSlidePanel(l Layout) bool { return !l.Rect(ZoneSlideBar).Empty() }

func (r *Root) drawSlideButton(c widget.DrawContext, z Zone, v SlideshowView) {
	t := r.Theme
	on := r.enabled(z.Kind) || z.Kind == ZoneSlideSettings
	col := t.OverlayText
	if !on {
		col = t.Disabled
	}
	if on && r.hover == z.Kind {
		hover := t.OverlayHover
		if r.pressing && r.pressed == z.Kind {
			hover = gfx.Alpha(color.RGBA{255, 255, 255, 255}, .14)
		}
		gfx.FillRound(c, z.Rect.Inset(2), 6, hover)
	}
	name := "settings-bar"
	switch z.Kind {
	case ZoneSlidePlay:
		name = "play"
		if v.Playing {
			name = "pause"
		}
	case ZoneSlideExit:
		name = "exit-fullscreen"
	}
	drawIcon(c, name, (z.Rect.Min.X+z.Rect.Max.X)/2, (z.Rect.Min.Y+z.Rect.Max.Y)/2, col)
}

func (r *Root) drawSlideTrack(c widget.DrawContext, sl Slide, v SlideshowView) {
	if !sl.HasTrack() {
		return
	}
	t := r.Theme
	half := float64(SlideTrackThickness) / 2
	gfx.FillDistance(c, sl.Damage, capsuleDistance(sl.TrackX0, sl.TrackX1, sl.TrackY, half), t.SlideTrack)
	if !v.Active {
		return
	}
	kx := sl.KnobX(v.Progress)
	gfx.FillDistance(c, sl.Damage, capsuleDistance(sl.TrackX0, math.Max(kx, sl.TrackX0+2*half), sl.TrackY, half), t.Accent)
	gfx.FillCircle(c, kx, sl.TrackY, SlideKnobRadius, t.Accent)
}
