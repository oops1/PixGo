package window

import (
	"image"
	"image/color"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/commands"
	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/ui/gfx"
)

const (
	titleBaseline  = 30
	zoomBaseline   = 5
	titleGap       = 14
	arrowRadius    = 20.5
	toolbarRadius  = ToolbarRadius
	hoverRadius    = 4
	stateTitleGap  = 10
	stateLineSpace = 8
)

var (
	titleFont      = gfx.UI(10)
	zoomFont       = gfx.UI(10)
	stateTitleFont = gfx.UISemi(14)
	stateTextFont  = gfx.UI(10)
)

func DefaultActions() map[ZoneKind]commands.ID {
	return map[ZoneKind]commands.ID{
		ZoneBack:            "app.back",
		ZoneInfo:            "view.info",
		ZoneFavorite:        "file.favorite",
		ZoneMore:            "app.more",
		ZonePrev:            "nav.prev",
		ZoneNext:            "nav.next",
		ZoneToolGallery:     "view.gallery",
		ZoneToolDelete:      "file.delete",
		ZoneToolRotateLeft:  "view.rotateLeft",
		ZoneToolZoomOut:     "view.zoomOut",
		ZoneToolZoomLabel:   "view.zoomToggle",
		ZoneToolZoomIn:      "view.zoomIn",
		ZoneToolRotateRight: "view.rotateRight",
		ZoneToolMore:        "app.more",
		ZoneToolFullscreen:  "view.fullscreen",
		ZoneExit:            "view.fullscreen",
	}
}

func (r *Root) command(k ZoneKind) *commands.Command {
	id, ok := r.Actions[k]
	if !ok || r.host.Registry() == nil {
		return nil
	}
	return r.host.Registry().Get(id)
}

func (r *Root) enabled(k ZoneKind) bool {
	if r.ForceEnabled {
		return true
	}
	cmd := r.command(k)
	return cmd != nil && cmd.IsEnabled()
}

func (r *Root) drawBackdrop(c widget.DrawContext, l Layout) {
	t := r.Theme
	b := r.Bounds()
	c.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), t.Canvas)
}

func (r *Root) drawTitle(c widget.DrawContext, l Layout) {
	t := r.Theme
	fr := l.Opt
	_ = fr
	c.FillRect(0, 0, l.W, TitleHeight, t.TitleBar)
	c.DrawHLine(FrameWidth, SeparatorY, l.W-2*FrameWidth, t.Separator)

	for _, z := range l.Zones {
		switch z.Kind {
		case ZoneBack, ZoneInfo, ZoneFavorite, ZoneMore:
			r.drawTitleButton(c, z)
		case ZoneMin, ZoneMax, ZoneClose:
			r.drawCaption(c, l, z)
		}
	}

	if mn := l.Rect(ZoneMin); !mn.Empty() {
		c.DrawVLine(mn.Min.X-4, 8, 32, t.Divider)
	}
	limit := l.Rect(ZoneInfo).Min.X - titleGap
	if limit <= TitleTextX {
		limit = l.Rect(ZoneDrag).Max.X - titleGap
	}
	titleFont.DrawFit(c, r.host.Title(), TitleTextX-1, titleBaseline, limit-TitleTextX, t.TitleText)
}

func (r *Root) hoverFill(c widget.DrawContext, z Zone, fill color.RGBA) {
	gfx.FillRound(c, z.Rect, hoverRadius, fill)
}

func (r *Root) drawTitleButton(c widget.DrawContext, z Zone) {
	t := r.Theme
	cx, cy := (z.Rect.Min.X+z.Rect.Max.X)/2, (z.Rect.Min.Y+z.Rect.Max.Y)/2
	switch {
	case z.Kind == ZoneInfo && r.host.InfoOpen():
		r.hoverFill(c, z, t.TitleActive)
	case r.pressing && r.pressed == z.Kind:
		r.hoverFill(c, z, t.CaptionPressed)
	case r.hover == z.Kind:
		r.hoverFill(c, z, t.CaptionHover)
	}
	col := t.TitleIcon
	if !r.enabled(z.Kind) {
		col = t.Disabled
	}
	name := map[ZoneKind]string{ZoneBack: "back", ZoneInfo: "info", ZoneFavorite: "heart", ZoneMore: "more-title"}[z.Kind]
	if z.Kind == ZoneFavorite && r.host.Favorite() {
		name = "heart-filled"
		col = t.Accent
	}
	drawIcon(c, name, cx, cy, col)
}

func (r *Root) drawCaption(c widget.DrawContext, l Layout, z Zone) {
	t := r.Theme
	cx, cy := (z.Rect.Min.X+z.Rect.Max.X)/2, (z.Rect.Min.Y+z.Rect.Max.Y)/2
	col := t.TitleIcon
	switch {
	case r.pressing && r.pressed == z.Kind:
		fill := t.CaptionPressed
		if z.Kind == ZoneClose {
			fill = gfxBlend(t.CloseHover, t.TitleBar, .8)
			col = t.CloseText
		}
		c.FillRect(z.Rect.Min.X, z.Rect.Min.Y, z.Rect.Dx(), z.Rect.Dy(), fill)
	case r.hover == z.Kind:
		fill := t.CaptionHover
		if z.Kind == ZoneClose {
			fill = t.CloseHover
			col = t.CloseText
		}
		c.FillRect(z.Rect.Min.X, z.Rect.Min.Y, z.Rect.Dx(), z.Rect.Dy(), fill)
	}
	name := map[ZoneKind]string{ZoneMin: "caption-min", ZoneMax: "caption-max", ZoneClose: "caption-close"}[z.Kind]
	if z.Kind == ZoneMax && l.Opt.Maximized {
		name = "caption-restore"
	}
	drawIcon(c, name, cx, cy, col)
}

func gfxBlend(a, b color.RGBA, f float64) color.RGBA { return gfx.Blend(a, b, f) }

func (r *Root) drawState(c widget.DrawContext, l Layout) {
	t := r.Theme
	st := r.host.State()
	if st == StateReady {
		return
	}
	if st == StateLoading {
		if cv := r.host.Content(); cv != nil {
			if h, ok := cv.(interface{ HasImage() bool }); ok && h.HasImage() {
				return
			}
		}
	}
	var head, body string
	switch st {
	case StateEmpty:
		head, body = i18n.T("empty.title"), i18n.T("empty.hint")
	case StateLoading:
		head = i18n.T("viewer.loading")
	case StateFailed:
		head, body = i18n.T("error.open"), r.host.FailureText()
	}
	cx := (l.View.Min.X + l.View.Max.X) / 2
	cy := (l.View.Min.Y + l.View.Max.Y) / 2
	lines := 1
	if body != "" {
		lines = 2
	}
	total := stateTitleFont.Ascent()
	if lines == 2 {
		total += stateLineSpace + stateTextFont.Ascent() + 6
	}
	y := cy - total/2 + stateTitleFont.Ascent()
	w := l.View.Dx() - 48
	tw := stateTitleFont.Measure(c, head)
	stateTitleFont.DrawFit(c, head, cx-min(tw, w)/2, y, w, t.Text)
	if body != "" {
		y += stateLineSpace + stateTextFont.Ascent() + 6
		bw := stateTextFont.Measure(c, body)
		stateTextFont.DrawFit(c, body, cx-min(bw, w)/2, y, w, t.SecondaryText)
	}
}

func (r *Root) drawOverlay(c widget.DrawContext, l Layout) {
	if r.host.State() == StateEmpty && !r.overlayVisible() {
		return
	}
	for _, z := range l.Zones {
		if z.Kind == ZonePrev || z.Kind == ZoneNext {
			r.drawArrow(c, z)
		}
	}
	for _, z := range l.Zones {
		if z.Kind == ZoneExit {
			r.drawExit(c, z)
		}
	}
	if !hasToolbar(l) {
		return
	}
	t := r.Theme
	gfx.FillRound(c, l.Toolbar, toolbarRadius, t.Overlay)
	gfx.StrokeRound(c, l.Toolbar, toolbarRadius, 1, t.OverlayBorder)
	for _, z := range l.Zones {
		if z.Kind >= ZoneToolGallery && z.Kind <= ZoneToolFullscreen {
			r.drawToolButton(c, l, z)
		}
	}
}

func hasToolbar(l Layout) bool {
	for _, z := range l.Zones {
		if z.Kind == ZoneToolGallery {
			return true
		}
	}
	return false
}

func (r *Root) drawArrow(c widget.DrawContext, z Zone) {
	t := r.Theme
	prev := z.Kind == ZonePrev
	if prev && !r.host.CanPrev() || !prev && !r.host.CanNext() {
		return
	}
	cx := float64(z.Rect.Min.X) + float64(z.Rect.Dx())/2
	cy := float64(z.Rect.Min.Y) + float64(z.Rect.Dy())/2
	gfx.FillCircle(c, cx, cy, arrowRadius, t.ArrowFill)
	if r.hover == z.Kind || r.pressing && r.pressed == z.Kind {
		gfx.FillCircle(c, cx, cy, arrowRadius, t.OverlayHover)
	}
	gfx.StrokeCircle(c, cx, cy, arrowRadius, 1, t.ArrowBorder)
	name := "chevron-right"
	if prev {
		name = "chevron-left"
	}
	drawIcon(c, name, z.Rect.Min.X+z.Rect.Dx()/2, z.Rect.Min.Y+z.Rect.Dy()/2, t.ArrowGlyph)
}

func (r *Root) drawToolButton(c widget.DrawContext, l Layout, z Zone) {
	t := r.Theme
	on := r.enabled(z.Kind)
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
	cx, cy := (z.Rect.Min.X+z.Rect.Max.X)/2, (z.Rect.Min.Y+z.Rect.Max.Y)/2
	if z.Kind == ZoneToolZoomLabel {
		text := r.host.ZoomText()
		w := zoomFont.Measure(c, text)
		zoomFont.Draw(c, text, cx-w/2, cy+zoomBaseline, col)
		return
	}
	name := map[ZoneKind]string{
		ZoneToolGallery: "gallery", ZoneToolDelete: "delete", ZoneToolRotateLeft: "rotate-left",
		ZoneToolZoomOut: "zoom-out", ZoneToolZoomIn: "zoom-in", ZoneToolRotateRight: "rotate-right",
		ZoneToolMore: "more", ZoneToolFullscreen: "fullscreen",
	}[z.Kind]
	if z.Kind == ZoneToolFullscreen && r.host.Fullscreen() {
		name = "fullscreen-exit"
	}
	drawIcon(c, name, cx, cy, col)
}

func (r *Root) drawFrame(c widget.DrawContext, l Layout) {
	if l.Opt.Maximized || l.Opt.Fullscreen {
		return
	}
	c.DrawRoundBorder(0, 0, l.W, l.H, CornerRadius, r.Theme.WindowBorder)
}

var _ = image.Pt

func (r *Root) drawExit(c widget.DrawContext, z Zone) {
	t := r.Theme
	if r.hover == z.Kind || r.pressing && r.pressed == z.Kind {
		gfx.FillRound(c, z.Rect, float64(z.Rect.Dx())/2, t.ArrowFill)
	}
	drawIcon(c, "close", (z.Rect.Min.X+z.Rect.Max.X)/2, (z.Rect.Min.Y+z.Rect.Max.Y)/2, t.OverlayText)
}
