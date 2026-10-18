package edit

import (
	"image"
	"image/color"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/ui/gfx"
	"github.com/oops1/PixGo/internal/ui/icons"
)

const (
	dimAlpha     = 0.55
	pillPadX     = 14
	pillH        = 26
	pillBottom   = 14
	btnRadius    = 6
	cornerSize   = 10
	edgeLong     = 18
	edgeShort    = 6
	minEdgeShown = 44
)

var white = color.RGBA{255, 255, 255, 255}

func (e *Editor) Draw(c widget.DrawContext) {
	b := e.Bounds()
	if b.Empty() || !e.active {
		return
	}
	t := e.Theme
	c.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), t.Canvas)
	e.layout()
	e.canvas.Draw(c)
	e.drawCrop(c)
	e.drawBar(c)
	e.drawSize(c)
	e.drawTip(c)
}

func (e *Editor) drawCrop(c widget.DrawContext) {
	pic, cr := e.PictureScreen(), e.CropScreen()
	if pic.Empty() || cr.Empty() {
		return
	}
	dim := gfx.Alpha(color.RGBA{0, 0, 0, 255}, dimAlpha)
	fill := func(r image.Rectangle) {
		if !r.Empty() {
			c.FillRectAlpha(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), dim)
		}
	}
	fill(image.Rect(pic.Min.X, pic.Min.Y, pic.Max.X, cr.Min.Y))
	fill(image.Rect(pic.Min.X, cr.Max.Y, pic.Max.X, pic.Max.Y))
	fill(image.Rect(pic.Min.X, cr.Min.Y, cr.Min.X, cr.Max.Y))
	fill(image.Rect(cr.Max.X, cr.Min.Y, pic.Max.X, cr.Max.Y))
	if e.dragging.on {
		e.drawThirds(c, cr)
	}
	shadow := gfx.Alpha(color.RGBA{0, 0, 0, 255}, .45)
	outer := cr.Inset(-1)
	line(c, outer, shadow)
	line(c, cr, white)
	if !e.snap.Empty() {
		e.drawSnap(c)
	}
	e.drawHandles(c, cr)
	if e.kbd && e.focus == PartCrop {
		gfx.RingOf(e.Theme).Rect(c, cr.Inset(-4), 2)
	}
}

func line(c widget.DrawContext, r image.Rectangle, col color.RGBA) {
	if r.Empty() {
		return
	}
	c.FillRectAlpha(r.Min.X, r.Min.Y, r.Dx(), 1, col)
	c.FillRectAlpha(r.Min.X, r.Max.Y-1, r.Dx(), 1, col)
	c.FillRectAlpha(r.Min.X, r.Min.Y+1, 1, r.Dy()-2, col)
	c.FillRectAlpha(r.Max.X-1, r.Min.Y+1, 1, r.Dy()-2, col)
}

func (e *Editor) drawThirds(c widget.DrawContext, cr image.Rectangle) {
	col := gfx.Alpha(white, .5)
	for i := 1; i <= 2; i++ {
		x := cr.Min.X + cr.Dx()*i/3
		y := cr.Min.Y + cr.Dy()*i/3
		c.FillRectAlpha(x, cr.Min.Y, 1, cr.Dy(), col)
		c.FillRectAlpha(cr.Min.X, y, cr.Dx(), 1, col)
	}
}

func (e *Editor) drawSnap(c widget.DrawContext) {
	ox, oy, k, ok := e.mapping()
	if !ok {
		return
	}
	r := toScreen(e.snap, ox, oy, k)
	col := e.Theme.Accent
	const dash = 5
	for x := r.Min.X; x < r.Max.X; x += 2 * dash {
		w := min(dash, r.Max.X-x)
		c.FillRect(x, r.Min.Y, w, 2, col)
		c.FillRect(x, r.Max.Y-2, w, 2, col)
	}
	for y := r.Min.Y; y < r.Max.Y; y += 2 * dash {
		h := min(dash, r.Max.Y-y)
		c.FillRect(r.Min.X, y, 2, h, col)
		c.FillRect(r.Max.X-2, y, 2, h, col)
	}
}

func (e *Editor) drawHandles(c widget.DrawContext, cr image.Rectangle) {
	edge := gfx.Alpha(color.RGBA{0, 0, 0, 255}, .55)
	paint := func(r image.Rectangle) {
		gfx.FillRound(c, r, 2.5, white)
		gfx.StrokeRound(c, r, 2.5, 1, edge)
	}
	for _, h := range handleList {
		x, y := e.HandleCenter(h)
		switch {
		case h.corner():
			paint(image.Rect(x-cornerSize/2, y-cornerSize/2, x-cornerSize/2+cornerSize, y-cornerSize/2+cornerSize))
		case h == HandleN || h == HandleS:
			if cr.Dx() >= minEdgeShown {
				paint(image.Rect(x-edgeLong/2, y-edgeShort/2, x-edgeLong/2+edgeLong, y-edgeShort/2+edgeShort))
			}
		default:
			if cr.Dy() >= minEdgeShown {
				paint(image.Rect(x-edgeShort/2, y-edgeLong/2, x-edgeShort/2+edgeShort, y-edgeLong/2+edgeLong))
			}
		}
	}
}

func baseline(r image.Rectangle, f gfx.Font) int {
	return r.Min.Y + (r.Dy()-textHeightOf(f))/2 + f.Ascent()
}

func (e *Editor) drawBar(c widget.DrawContext) {
	t := e.Theme
	l := e.bar
	if l.bar.Empty() {
		return
	}
	gfx.FillRound(c, l.bar, barRadius, t.Overlay)
	gfx.StrokeRound(c, l.bar, barRadius, 1, t.OverlayBorder)
	if !l.sep.Empty() {
		c.FillRect(l.sep.Min.X, l.sep.Min.Y, 1, l.sep.Dy(), t.OverlayBorder)
	}
	for _, p := range barStops {
		r := l.rect(p)
		if r.Empty() {
			continue
		}
		switch p {
		case PartAspect:
			e.drawAspect(c, r)
		case PartResize, PartReset:
			e.drawIconText(c, p, r)
		case PartSaveCopy, PartSaveMore:
		case PartCancel:
			e.drawCancel(c, r)
		default:
			e.drawIconButton(c, p, r)
		}
	}
	e.drawSave(c)
	e.drawBarFocus(c)
}

func (e *Editor) state(p Part) (on, hover, down bool) {
	on = e.Enabled(p)
	hover = on && (e.hover == p || e.menuPart == p)
	down = hover && e.pressed == p
	return
}

func (e *Editor) hoverFill(c widget.DrawContext, p Part, r image.Rectangle) {
	_, hover, down := e.state(p)
	switch {
	case down:
		gfx.FillRound(c, r, btnRadius, gfx.Alpha(white, .14))
	case hover:
		gfx.FillRound(c, r, btnRadius, e.Theme.OverlayHover)
	}
}

func (e *Editor) ink(p Part) color.RGBA {
	if e.Enabled(p) {
		return e.Theme.OverlayText
	}
	return e.Theme.Disabled
}

func iconNameOf(p Part) string {
	switch p {
	case PartSwap:
		return "aspect-swap"
	case PartRotateLeft:
		return "rotate-left"
	case PartRotateRight:
		return "rotate-right"
	case PartFlipH:
		return "flip-h"
	case PartFlipV:
		return "flip-v"
	case PartResize:
		return "resize"
	case PartReset:
		return "reset"
	case PartAspect:
		return "crop"
	}
	return ""
}

func (e *Editor) drawIconButton(c widget.DrawContext, p Part, r image.Rectangle) {
	e.hoverFill(c, p, r.Inset(2))
	icons.DrawCentered(c, iconNameOf(p), (r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2, e.ink(p))
}

func (e *Editor) drawIconText(c widget.DrawContext, p Part, r image.Rectangle) {
	e.hoverFill(c, p, r.Inset(2))
	col := e.ink(p)
	if r.Dx() <= barBtn {
		icons.DrawCentered(c, iconNameOf(p), (r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2, col)
		return
	}
	icons.DrawCentered(c, iconNameOf(p), r.Min.X+textPadX+10, (r.Min.Y+r.Max.Y)/2, col)
	label := i18n.T("edit.resize")
	if p == PartReset {
		label = i18n.T("edit.reset")
	}
	labelFont.Draw(c, label, r.Min.X+textPadX+20+barIconText, baseline(r, labelFont), col)
}

func (e *Editor) drawAspect(c widget.DrawContext, r image.Rectangle) {
	e.hoverFill(c, PartAspect, r.Inset(2))
	col := e.ink(PartAspect)
	icons.DrawCentered(c, "crop", r.Min.X+textPadX-2+10, (r.Min.Y+r.Max.Y)/2, col)
	if e.bar.level != levelTight {
		labelFont.Draw(c, e.aspectLabel(), r.Min.X+textPadX-2+20+barIconText, baseline(r, labelFont), col)
	}
	icons.DrawCentered(c, "chevron-down", r.Max.X-textPadX+2-6, (r.Min.Y+r.Max.Y)/2, col)
}

func (e *Editor) drawCancel(c widget.DrawContext, r image.Rectangle) {
	t := e.Theme
	box := r.Inset(2)
	e.hoverFill(c, PartCancel, box)
	gfx.StrokeRound(c, box, btnRadius, 1, t.OverlayBorder)
	label := i18n.T("edit.cancel")
	w := labelFont.Measure(c, label)
	labelFont.Draw(c, label, r.Min.X+(r.Dx()-w)/2, baseline(r, labelFont), e.ink(PartCancel))
}

func (e *Editor) drawSave(c widget.DrawContext) {
	t := e.Theme
	main, more := e.bar.rect(PartSaveCopy), e.bar.rect(PartSaveMore)
	if main.Empty() || more.Empty() {
		return
	}
	union := image.Rect(main.Min.X, main.Min.Y+2, more.Max.X, main.Max.Y-2)
	fill := t.Accent
	ink := t.AccentText
	if !e.Enabled(PartSaveCopy) {
		fill = gfx.Blend(t.Overlay, t.Disabled, .35)
		ink = t.Disabled
	}
	gfx.FillRound(c, union, btnRadius, fill)
	for _, p := range []Part{PartSaveCopy, PartSaveMore} {
		_, hover, down := e.state(p)
		if !hover {
			continue
		}
		over := gfx.Alpha(white, .14)
		if down {
			over = gfx.Alpha(color.RGBA{0, 0, 0, 255}, .18)
		}
		half := e.bar.rect(p)
		restore := gfx.Clip(c, half)
		gfx.FillRound(c, union, btnRadius, over)
		restore()
	}
	c.FillRect(more.Min.X-1, union.Min.Y+6, 1, union.Dy()-12, gfx.Alpha(ink, .5))
	label := i18n.T("edit.saveCopy")
	w := labelFont.Measure(c, label)
	labelFont.Draw(c, label, main.Min.X+(main.Dx()-w)/2, baseline(main, labelFont), ink)
	icons.DrawCentered(c, "chevron-down", (more.Min.X+more.Max.X)/2, (more.Min.Y+more.Max.Y)/2, ink)
}

func (e *Editor) drawBarFocus(c widget.DrawContext) {
	if !e.kbd || e.focus == PartCrop || e.focus == PartNone {
		return
	}
	r := e.bar.rect(e.focus)
	if r.Empty() {
		return
	}
	ring := gfx.RingOf(e.Theme)
	switch e.focus {
	case PartSaveCopy, PartSaveMore:
		main, more := e.bar.rect(PartSaveCopy), e.bar.rect(PartSaveMore)
		ring.Rect(c, image.Rect(main.Min.X, main.Min.Y+2, more.Max.X, main.Max.Y-2), btnRadius)
	default:
		ring.Rect(c, r.Inset(2), btnRadius)
	}
}

func (e *Editor) sizeText() string {
	out := e.OutputSize()
	if e.HasResize() {
		return i18n.T("edit.sizeResized", out.X, out.Y)
	}
	return i18n.T("edit.size", out.X, out.Y)
}

func (e *Editor) pillRect(text string) image.Rectangle {
	b := e.Bounds()
	w := labelFont.Measure(nil, text) + 2*pillPadX
	w = min(w, max(80, b.Dx()-32))
	h := pillH + textHeightOf(labelFont) - 14
	cx := (b.Min.X + b.Max.X) / 2
	return image.Rect(cx-w/2, b.Max.Y-pillBottom-h, cx-w/2+w, b.Max.Y-pillBottom)
}

func (e *Editor) SizePill() image.Rectangle { return e.pillRect(e.sizeText()) }

func (e *Editor) drawSize(c widget.DrawContext) {
	t := e.Theme
	text := e.sizeText()
	r := e.pillRect(text)
	gfx.FillRound(c, r, 8, t.Menu)
	gfx.StrokeRound(c, r, 8, 1, t.MenuBorder)
	labelFont.DrawFit(c, text, r.Min.X+pillPadX, baseline(r, labelFont), r.Dx()-2*pillPadX, t.Text)
}

func (e *Editor) tipRect(c widget.DrawContext) image.Rectangle {
	if e.tipPart == PartNone || e.tipText == "" && c == nil {
		return image.Rectangle{}
	}
	anchor := e.bar.rect(e.tipPart)
	if anchor.Empty() {
		return image.Rectangle{}
	}
	text := e.tipText
	w := min(labelFont.Measure(nil, text)+20, max(0, e.Bounds().Dx()-8))
	h := 14 + textHeightOf(labelFont)
	cx := (anchor.Min.X + anchor.Max.X) / 2
	b := e.Bounds()
	x0 := max(b.Min.X+4, min(cx-w/2, b.Max.X-w-4))
	y0 := e.bar.bar.Max.Y + 6
	return image.Rect(x0, y0, x0+w, y0+h)
}

func (e *Editor) drawTip(c widget.DrawContext) {
	if e.tipText == "" {
		return
	}
	t := e.Theme
	r := e.tipRect(c)
	if r.Empty() {
		return
	}
	gfx.FillRound(c, r, 6, t.Menu)
	gfx.StrokeRound(c, r, 6, 1, t.MenuBorder)
	labelFont.DrawFit(c, e.tipText, r.Min.X+10, baseline(r, labelFont), r.Dx()-20, t.Text)
}
