package window

import (
	"image"
	"math"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/fonts"
	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/ui/gfx"
)

const (
	previewBadgeInset  = 12
	previewBadgeHeight = 24
	previewBadgePadX   = 10
	previewBadgeRadius = 6
)

var previewBadgeFont = gfx.UI(7.5)

type previewAware interface{ Preview() bool }

func (r *Root) previewShown() bool {
	p, ok := r.host.(previewAware)
	return ok && p.Preview() && r.host.Page() == PageViewer
}

func previewBadgeRect(view image.Rectangle, text string) image.Rectangle {
	k := fonts.TextScale()
	h := int(math.Round(previewBadgeHeight * k))
	w := textWidth(previewBadgeFont, text) + 2*int(math.Round(previewBadgePadX*k))
	w = max(min(w, view.Dx()-2*previewBadgeInset), 0)
	x, y := view.Min.X+previewBadgeInset, view.Min.Y+previewBadgeInset
	return image.Rect(x, y, x+w, y+min(h, max(view.Dy()-2*previewBadgeInset, 0)))
}

func (r *Root) PreviewBadge() image.Rectangle {
	if !r.previewShown() {
		return image.Rectangle{}
	}
	return previewBadgeRect(r.Layout().View, i18n.T("viewer.preview"))
}

func (r *Root) drawPreviewBadge(c widget.DrawContext, l Layout) {
	if !r.previewShown() {
		return
	}
	text := i18n.T("viewer.preview")
	rect := previewBadgeRect(l.View, text)
	if rect.Empty() {
		return
	}
	t := r.Theme
	gfx.FillRound(c, rect, previewBadgeRadius, t.Overlay)
	gfx.StrokeRound(c, rect, previewBadgeRadius, 1, t.OverlayBorder)
	pad := int(math.Round(previewBadgePadX * fonts.TextScale()))
	baseline := rect.Min.Y + (rect.Dy()+previewBadgeFont.Ascent()-4)/2 - textOpticalRise(previewBadgeFont)
	previewBadgeFont.DrawFit(c, text, rect.Min.X+pad, baseline, rect.Dx()-2*pad, t.OverlayText)
}
