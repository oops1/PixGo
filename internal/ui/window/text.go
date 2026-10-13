package window

import (
	"math"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/fonts"
	"github.com/oops1/PixGo/internal/ui/gfx"
)

const (
	capCenterRatio  = 0.334
	zoomLabelPadX   = 4
	zoomLabelMinW   = ToolZoomLabelW
	toastHeightBase = 40
	tipHeightBase   = 28
)

func textWidth(f gfx.Font, s string) int { return widget.MeasureUITextFont(s, f.Size(), f.Name) }

func referenceTextWidth(f gfx.Font, s string) int {
	return widget.MeasureUITextFont(s, f.Pt, f.Name)
}

func textLift(f gfx.Font) int {
	ref := fonts.MetricsOf(f.Name, f.Pt, 96).Ascent
	return int(math.Round(float64(f.Ascent()-ref) * capCenterRatio))
}

func textGrowth(f gfx.Font) int {
	now := fonts.MetricsOf(f.Name, f.Size(), 96)
	ref := fonts.MetricsOf(f.Name, f.Pt, 96)
	return now.Ascent + now.Descent - ref.Ascent - ref.Descent
}

func toastH() int { return toastHeightBase + textGrowth(toastFont) }

func tipH() int { return tipHeightBase + textGrowth(tipFont) }

func textOpticalRise(f gfx.Font) int {
	ref := fonts.MetricsOf(f.Name, f.Pt, 96).Ascent
	return int(math.Round(float64(f.Ascent()-ref) * (0.5 - capCenterRatio)))
}
