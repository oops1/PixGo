package window

import (
	"image"
	"math"
	"time"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/ui/gfx"
)

const (
	loadingSize   = 28
	loadingInset  = 12
	loadingDots   = 8
	loadingOrbit  = 8.0
	loadingDot    = 1.7
	loadingTick   = 80 * time.Millisecond
	loadingTrail  = 0.82
	loadingBorder = 1
)

type reducedAware interface{ Reduced() bool }

func (r *Root) contentHasImage() bool {
	cv := r.host.Content()
	if cv == nil {
		return false
	}
	h, ok := cv.(interface{ HasImage() bool })
	return ok && h.HasImage()
}

func (r *Root) loadingShown() bool {
	return r.host.Page() == PageViewer && r.host.State() == StateLoading && r.contentHasImage()
}

func (r *Root) loadingName() string {
	if p, ok := r.host.(reducedAware); ok && p.Reduced() {
		return i18n.T("access.loadingFull")
	}
	return i18n.T("viewer.loading")
}

func loadingRect(view image.Rectangle) image.Rectangle {
	x := view.Max.X - loadingInset - loadingSize
	y := view.Min.Y + loadingInset
	return image.Rect(x, y, x+loadingSize, y+loadingSize).Intersect(view)
}

func (r *Root) loadingPhase() int {
	return int(r.Now().UnixMilli()/loadingTick.Milliseconds()) % loadingDots
}

func (r *Root) drawLoading(c widget.DrawContext, l Layout) {
	if !r.loadingShown() {
		return
	}
	rect := loadingRect(l.View)
	if rect.Dx() < loadingSize || rect.Dy() < loadingSize {
		return
	}
	t := r.Theme
	cx := float64(rect.Min.X) + float64(rect.Dx())/2
	cy := float64(rect.Min.Y) + float64(rect.Dy())/2
	radius := float64(loadingSize) / 2
	gfx.FillCircle(c, cx, cy, radius, t.ArrowFill)
	gfx.StrokeCircle(c, cx, cy, radius, loadingBorder, t.ArrowBorder)
	phase := r.loadingPhase()
	for i := 0; i < loadingDots; i++ {
		age := (phase - i + loadingDots) % loadingDots
		weight := math.Pow(loadingTrail, float64(age))
		angle := float64(i)*2*math.Pi/loadingDots - math.Pi/2
		gfx.FillCircle(c, cx+loadingOrbit*math.Cos(angle), cy+loadingOrbit*math.Sin(angle), loadingDot, gfx.Alpha(t.ArrowGlyph, 0.18+0.82*weight))
	}
}

func (r *Root) syncLoading() {
	if !r.loadingShown() || r.stopLoading != nil || r.After == nil {
		return
	}
	r.stopLoading = r.After(loadingTick, r.loadingTick)
}

func (r *Root) loadingTick() {
	r.stopLoading = nil
	r.invalidateRect(loadingRect(r.Layout().View))
	r.syncLoading()
}
