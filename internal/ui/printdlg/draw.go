package printdlg

import (
	"errors"
	"image"
	"image/color"
	"math"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/print"
	"github.com/oops1/PixGo/internal/ui/gfx"
	"github.com/oops1/PixGo/internal/ui/icons"
)

func inkOn(fill color.RGBA) color.RGBA {
	luma := (0.299*float64(fill.R) + 0.587*float64(fill.G) + 0.114*float64(fill.B)) / 255
	if luma > 0.6 {
		return color.RGBA{0, 0, 0, 255}
	}
	return color.RGBA{255, 255, 255, 255}
}

func baseline(r image.Rectangle, f gfx.Font) int {
	return r.Min.Y + (r.Dy()-textHeight(f))/2 + f.Ascent()
}

func (f *form) showRing(p Part) bool { return f.focused && f.keyNav && f.stop == p && f.list == nil }

func (f *form) ring(c widget.DrawContext, t *widget.Theme, r image.Rectangle, radius float64) {
	gfx.StrokeRound(c, r.Inset(-ringGap), radius+ringGap, 2, t.LabelText)
}

type infoLine struct {
	text string
	ink  color.RGBA
}

func (f *form) InfoLines() []string {
	var out []string
	for _, l := range f.infoLines(nil) {
		out = append(out, l.text)
	}
	return out
}

func sizeText(mm float64, u print.Unit) string {
	prec := 1
	if u == print.UnitInch {
		prec = 2
	}
	return i18n.FormatFloat(mm/u.MM(), prec)
}

func (f *form) infoLines(t *widget.Theme) []infoLine {
	secondary, warn, bad := color.RGBA{}, warnInk, errorInk
	if t != nil {
		secondary = t.SecondaryText
	}
	info := f.model.Info()
	if info.Err != nil {
		key := "print.info.noRoom"
		if errors.Is(info.Err, print.ErrBadSize) {
			key = "print.info.badSize"
		}
		return []infoLine{{i18n.T(key), bad}}
	}
	o := f.model.Options()
	unitKey := "print.info.sizeCm"
	if o.Unit == print.UnitInch {
		unitKey = "print.info.sizeIn"
	}
	out := []infoLine{{i18n.T(unitKey, sizeText(info.WidthMM, o.Unit), sizeText(info.HeightMM, o.Unit)), secondary}}
	dpi := int(info.DPI + .5)
	if info.Low {
		out = append(out, infoLine{i18n.T("print.info.dpiLow", dpi), warn})
	} else {
		out = append(out, infoLine{i18n.T("print.info.dpi", dpi), secondary})
	}
	switch {
	case info.Cut:
		out = append(out, infoLine{i18n.T("print.info.cut"), warn})
	case o.Placement == print.PlaceActual && !info.Known:
		out = append(out, infoLine{i18n.T("print.info.unknownDpi", print.UnknownDPI), secondary})
	}
	return out
}

func (f *form) Draw(c widget.DrawContext) {
	b := f.Bounds()
	if b.Empty() {
		return
	}
	restore := gfx.Clip(c, b)
	defer restore()
	t := widget.CurrentTheme()
	view := gfx.Clip(c, f.viewRect())
	f.drawLabels(c, t)
	for _, p := range []Part{PartPrinter, PartPaper, PartOrientation, PartPlacement, PartMargins} {
		if f.shown(p) {
			f.drawDropdown(c, t, p)
		}
	}
	if f.shown(PartSystem) {
		f.drawButton(c, t, PartSystem, i18n.T("print.system"), false)
	}
	if f.shown(PartCopies) {
		f.drawField(c, t, PartCopies)
		f.drawStep(c, t, PartCopiesDec, "zoom-out")
		f.drawStep(c, t, PartCopiesInc, "zoom-in")
	}
	f.drawSize(c, t)
	f.drawUnits(c, t)
	f.drawKeep(c, t)
	if !f.lay.wide {
		f.drawPreview(c, t)
		f.drawInfo(c, t)
	}
	view()
	if f.lay.wide {
		f.drawPreview(c, t)
		f.drawInfo(c, t)
	}
	f.drawScrollbar(c, t)
	f.drawBar(c, t)
	f.drawList(c, t)
}

func (f *form) drawLabels(c widget.DrawContext, t *widget.Theme) {
	for p, key := range labelKeys {
		if !f.shown(p) {
			continue
		}
		r := f.labelRect(p)
		ink := t.LabelText
		if !f.enabled(p) && p == PartCopies || p == PartWidth && !f.model.CustomActive() {
			ink = t.Disabled
		}
		bodyFont.DrawFit(c, i18n.T(key), r.Min.X, baseline(r, bodyFont), r.Dx(), ink)
	}
}

func (f *form) fieldBox(c widget.DrawContext, t *widget.Theme, p Part, r image.Rectangle) {
	radius := 4 * f.lay.m.ks
	on := f.enabled(p)
	hover := on && f.hover == p
	fill := t.InputBG
	if hover {
		fill = gfx.Blend(t.InputBG, t.LabelText, .05)
	}
	if !on {
		fill = gfx.Blend(t.DialogBG, t.InputBG, .5)
	}
	gfx.FillRound(c, r, radius, fill)
	gfx.StrokeRound(c, r, radius, 1, t.InputBorder)
	if on && f.focused && f.stop == p {
		c.FillRect(r.Min.X+3, r.Max.Y-2, r.Dx()-6, 2, t.InputFocus)
	}
	if f.showRing(p) {
		f.ring(c, t, r, radius)
	}
}

func (f *form) drawDropdown(c widget.DrawContext, t *widget.Theme, p Part) {
	r := f.rect(p)
	if !gfx.Visible(c, r) {
		return
	}
	f.fieldBox(c, t, p, r)
	on := f.enabled(p)
	ink := t.InputText
	if !on {
		ink = t.Disabled
	}
	pad := f.fieldPad()
	chev := f.lay.m.s(26)
	bodyFont.DrawFit(c, f.valueText(p), r.Min.X+pad, baseline(r, bodyFont), r.Dx()-pad-chev, ink)
	icons.DrawCentered(c, "chevron-down", r.Max.X-chev/2-f.lay.m.s(2), r.Min.Y+r.Dy()/2, gfx.Alpha(ink, .8))
}

func (f *form) drawField(c widget.DrawContext, t *widget.Theme, p Part) {
	r := f.rect(p)
	if !gfx.Visible(c, r) {
		return
	}
	f.fieldBox(c, t, p, r)
	on := f.enabled(p)
	fl, text := f.fieldOf(p)
	if !on && p != PartCopies {
		text = f.restingSize(p)
	}
	pad := f.fieldPad()
	x := r.Min.X + pad
	base := baseline(r, bodyFont)
	focus := on && f.focused && f.stop == p
	invalid := false
	switch p {
	case PartCopies:
		invalid = on && !f.model.CopiesValid()
	case PartWidth:
		invalid = on && !f.model.SizeValid(true)
	case PartHeight:
		invalid = on && !f.model.SizeValid(false)
	}
	if focus && fl.all && text != "" {
		w := bodyFont.Measure(c, text)
		hh := textHeight(bodyFont)
		c.FillRectAlpha(x, r.Min.Y+(r.Dy()-hh)/2, w, hh, t.TextSelectionBG)
	}
	ink := t.InputText
	switch {
	case !on:
		ink = t.Disabled
	case invalid:
		ink = errorInk
	}
	bodyFont.DrawFit(c, text, x, base, r.Dx()-2*pad, ink)
	if focus && !fl.all {
		cx := x + bodyFont.Measure(c, text[:min(fl.caret, len(text))])
		hh := textHeight(bodyFont)
		c.FillRect(cx, r.Min.Y+(r.Dy()-hh)/2, 1, hh, t.InputCaret)
	}
}

func (f *form) drawSize(c widget.DrawContext, t *widget.Theme) {
	if !f.shown(PartWidth) {
		return
	}
	f.drawField(c, t, PartWidth)
	f.drawField(c, t, PartHeight)
	tr := f.lay.times.Add(f.scrollOrigin())
	ink := t.SecondaryText
	if !f.model.CustomActive() {
		ink = t.Disabled
	}
	w := bodyFont.Measure(c, "×")
	bodyFont.Draw(c, "×", tr.Min.X+(tr.Dx()-w)/2, baseline(tr, bodyFont), ink)
}

func (f *form) drawStep(c widget.DrawContext, t *widget.Theme, p Part, icon string) {
	r := f.rect(p)
	if r.Empty() || !gfx.Visible(c, r) {
		return
	}
	on := f.enabled(p)
	hover := on && f.hover == p && (f.pressed == PartNone || f.pressed == p)
	down := on && f.pressed == p && f.hover == p
	radius := 4 * f.lay.m.ks
	fill := t.BtnBG
	switch {
	case !on:
		fill = gfx.Blend(t.DialogBG, t.BtnBG, .5)
	case down:
		fill = gfx.Blend(t.BtnBG, t.DialogBG, .35)
	case hover:
		fill = t.BtnHoverBG
	}
	gfx.FillRound(c, r, radius, fill)
	gfx.StrokeRound(c, r, radius, 1, t.BtnBorder)
	ink := t.BtnText
	if !on {
		ink = t.Disabled
	}
	icons.DrawCentered(c, icon, r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2, ink)
}

func (f *form) drawUnits(c widget.DrawContext, t *widget.Theme) {
	cm, in := f.rect(PartCm), f.rect(PartInch)
	if cm.Empty() || !gfx.Visible(c, cm.Union(in)) {
		return
	}
	on := f.model.CustomActive()
	whole := cm.Union(in)
	radius := 4 * f.lay.m.ks
	base := t.BtnBG
	if !on {
		base = gfx.Blend(t.DialogBG, t.BtnBG, .5)
	}
	gfx.FillRound(c, whole, radius, base)
	sel, other := cm, in
	if f.model.Options().Unit == print.UnitInch {
		sel, other = in, cm
	}
	accent := t.Accent
	if !on {
		accent = gfx.Blend(t.DialogBG, t.LabelText, .25)
	}
	restore := gfx.Clip(c, sel)
	gfx.FillRound(c, whole, radius, accent)
	restore()
	if on && (f.hover == PartCm && sel != cm || f.hover == PartInch && sel != in) {
		restore := gfx.Clip(c, other)
		gfx.FillRound(c, whole, radius, gfx.Alpha(t.LabelText, .08))
		restore()
	}
	gfx.StrokeRound(c, whole, radius, 1, t.BtnBorder)
	mid := (cm.Max.X + in.Min.X) / 2
	c.FillRect(mid, whole.Min.Y+4, 1, whole.Dy()-8, t.BtnBorder)
	for _, p := range []struct {
		r    image.Rectangle
		text string
		on   bool
	}{{cm, i18n.T("print.unit.cm"), sel == cm}, {in, i18n.T("print.unit.in"), sel == in}} {
		ink := t.BtnText
		switch {
		case !on:
			ink = t.Disabled
		case p.on:
			ink = inkOn(t.Accent)
		}
		w := bodyFont.Measure(c, p.text)
		bodyFont.Draw(c, p.text, p.r.Min.X+(p.r.Dx()-w)/2, baseline(p.r, bodyFont), ink)
	}
	if f.showRing(PartCm) {
		f.ring(c, t, cm, radius)
	}
	if f.showRing(PartInch) {
		f.ring(c, t, in, radius)
	}
}

func (f *form) drawKeep(c widget.DrawContext, t *widget.Theme) {
	row := f.rect(PartKeep)
	if row.Empty() || !gfx.Visible(c, row) {
		return
	}
	m := f.lay.m
	on := f.enabled(PartKeep)
	top := row.Min.Y + (row.Dy()-m.boxSize)/2
	box := image.Rect(row.Min.X, top, row.Min.X+m.boxSize, top+m.boxSize)
	radius := 3 * m.ks
	hover := on && (f.hover == PartKeep || f.pressed == PartKeep)
	if f.model.Options().Keep {
		fill := t.Accent
		if !on {
			fill = gfx.Blend(t.DialogBG, t.LabelText, .25)
		} else if hover {
			fill = gfx.Blend(t.Accent, color.RGBA{255, 255, 255, 255}, .12)
		}
		gfx.FillRound(c, box, radius, fill)
		icons.Draw(c, "checkmark", box.Min.X+(box.Dx()-12)/2, box.Min.Y+(box.Dy()-12)/2, inkOn(fill))
	} else {
		fill := t.CheckBG
		if hover {
			fill = t.CheckHoverBG
		}
		gfx.FillRound(c, box, radius, fill)
		border := t.CheckBorder
		if !on {
			border = t.Disabled
		}
		gfx.StrokeRound(c, box, radius, 1.3, border)
	}
	ink := t.LabelText
	if !on {
		ink = t.Disabled
	}
	bodyFont.DrawFit(c, i18n.T("print.keep"), box.Max.X+m.boxGap, baseline(row, bodyFont), row.Max.X-box.Max.X-m.boxGap, ink)
	if f.showRing(PartKeep) {
		f.ring(c, t, box, radius)
	}
}

func (f *form) drawButton(c widget.DrawContext, t *widget.Theme, p Part, text string, primary bool) {
	r := f.rect(p)
	if r.Empty() || !gfx.Visible(c, r) {
		return
	}
	on := f.enabled(p)
	hover := on && f.hover == p && (f.pressed == PartNone || f.pressed == p)
	down := on && f.pressed == p && f.hover == p
	radius := 4 * f.lay.m.ks
	white := color.RGBA{255, 255, 255, 255}
	fill, ink := t.BtnBG, t.BtnText
	switch {
	case primary:
		fill, ink = t.Accent, inkOn(t.Accent)
		if !on {
			fill, ink = gfx.Blend(t.DialogBG, t.LabelText, .15), t.Disabled
		}
		switch {
		case down:
			fill = gfx.Blend(t.Accent, t.DialogBG, .3)
		case hover:
			fill = gfx.Blend(t.Accent, white, .12)
		}
	case !on:
		fill, ink = gfx.Blend(t.DialogBG, t.BtnBG, .5), t.Disabled
	case down:
		fill = gfx.Blend(t.BtnBG, t.DialogBG, .35)
	case hover:
		fill = t.BtnHoverBG
	}
	gfx.FillRound(c, r, radius, fill)
	if !primary {
		gfx.StrokeRound(c, r, radius, 1, t.BtnBorder)
	}
	w := bodyFont.Measure(c, text)
	if w > r.Dx()-2 {
		text = bodyFont.Fit(c, text, r.Dx()-2)
		w = bodyFont.Measure(c, text)
	}
	bodyFont.Draw(c, text, r.Min.X+(r.Dx()-w)/2, baseline(r, bodyFont), ink)
	if f.showRing(p) {
		f.ring(c, t, r, radius)
	}
}

func (f *form) PrintLabel() string {
	if f.model.PDF() {
		return i18n.T("print.savePdf")
	}
	return i18n.T("print.print")
}

func (f *form) drawBar(c widget.DrawContext, t *widget.Theme) {
	rule := f.lay.rule.Add(f.origin())
	c.FillRectAlpha(rule.Min.X, rule.Min.Y, rule.Dx(), 1, gfx.Alpha(t.LabelText, .14))
	f.drawButton(c, t, PartPrint, f.PrintLabel(), true)
	f.drawButton(c, t, PartCancel, i18n.T("dialog.cancel"), false)
}

func (f *form) drawInfo(c widget.DrawContext, t *widget.Theme) {
	r := f.infoRect()
	if r.Empty() || !gfx.Visible(c, r) {
		return
	}
	step := textHeight(hintFont) + f.lay.m.lineGap
	for i, l := range f.infoLines(t) {
		row := image.Rect(r.Min.X, r.Min.Y+i*step, r.Max.X, r.Min.Y+(i+1)*step)
		hintFont.DrawFit(c, l.text, row.Min.X, baseline(row, hintFont), row.Dx(), l.ink)
	}
}

func (f *form) drawPreview(c widget.DrawContext, t *widget.Theme) {
	area := f.previewRect()
	if area.Empty() || !gfx.Visible(c, area) {
		return
	}
	radius := 6 * f.lay.m.ks
	gfx.FillRound(c, area, radius, gfx.Blend(t.DialogBG, t.LabelText, .06))
	inner := area.Inset(f.lay.m.s(10))
	if inner.Empty() {
		return
	}
	pv := f.model.Sheet(inner.Dx(), inner.Dy())
	sh := pv.Page.Sheet
	if sh.X < 1 || sh.Y < 1 {
		return
	}
	at := image.Pt(inner.Min.X+(inner.Dx()-sh.X)/2, inner.Min.Y+(inner.Dy()-sh.Y)/2)
	sheet := image.Rectangle{Min: at, Max: at.Add(sh)}
	c.FillRectAlpha(sheet.Min.X+2, sheet.Min.Y+3, sheet.Dx(), sheet.Dy(), color.RGBA{0, 0, 0, 56})
	if pv.Sheet != nil {
		c.DrawImage(pv.Sheet, sheet.Min.X, sheet.Min.Y)
	} else {
		c.FillRect(sheet.Min.X, sheet.Min.Y, sheet.Dx(), sheet.Dy(), color.RGBA{255, 255, 255, 255})
		if !pv.Page.Dest.Empty() {
			d := pv.Page.Dest.Add(at)
			c.FillRectAlpha(d.Min.X, d.Min.Y, d.Dx(), d.Dy(), gfx.Alpha(t.Accent, .35))
			c.DrawBorder(d.Min.X, d.Min.Y, d.Dx(), d.Dy(), t.Accent)
		}
	}
	if pv.Page.Content != (image.Rectangle{}) && f.model.Options().Margin != print.MarginNone {
		dashedRect(c, pv.Page.Content.Add(at), color.RGBA{120, 120, 120, 160})
	}
	c.DrawBorder(sheet.Min.X, sheet.Min.Y, sheet.Dx(), sheet.Dy(), t.InputBorder)
}

func dashedRect(c widget.DrawContext, r image.Rectangle, col color.RGBA) {
	const dash = 3
	for x := r.Min.X; x < r.Max.X; x += 2 * dash {
		w := min(dash, r.Max.X-x)
		c.FillRectAlpha(x, r.Min.Y, w, 1, col)
		c.FillRectAlpha(x, r.Max.Y-1, w, 1, col)
	}
	for y := r.Min.Y; y < r.Max.Y; y += 2 * dash {
		h := min(dash, r.Max.Y-y)
		c.FillRectAlpha(r.Min.X, y, 1, h, col)
		c.FillRectAlpha(r.Max.X-1, y, 1, h, col)
	}
}

func (f *form) drawList(c widget.DrawContext, t *widget.Theme) {
	ls := f.list
	if ls == nil {
		return
	}
	m := f.lay.m
	r := ls.rect
	radius := 6 * m.ks
	c.FillRectAlpha(r.Min.X+1, r.Min.Y+3, r.Dx(), r.Dy(), color.RGBA{0, 0, 0, 60})
	gfx.FillRound(c, r, radius, t.DropBG)
	gfx.StrokeRound(c, r, radius, 1, t.DropBorder)
	restore := gfx.Clip(c, r.Inset(1))
	defer restore()
	for row := 0; row < ls.rows; row++ {
		i := ls.top + row
		if i >= len(ls.items) {
			break
		}
		ir := image.Rect(r.Min.X+listPad, r.Min.Y+listPad+row*m.itemH, r.Max.X-listPad, r.Min.Y+listPad+(row+1)*m.itemH)
		ink := t.DropText
		if i == ls.sel {
			gfx.FillRound(c, ir, 4*m.ks, gfx.Alpha(t.LabelText, .1))
		}
		if i == f.currentIndex(ls.part) {
			c.FillRect(ir.Min.X+1, ir.Min.Y+m.s(7), 3, ir.Dy()-2*m.s(7), t.Accent)
		}
		bodyFont.DrawFit(c, ls.items[i], ir.Min.X+m.s(12), baseline(ir, bodyFont), ir.Dx()-m.s(16), ink)
	}
}

func (f *form) currentIndex(p Part) int {
	_, sel := f.itemsOf(p)
	return sel
}

func (f *form) restingSize(p Part) string {
	info := f.model.Info()
	if info.Err != nil {
		return ""
	}
	mm := info.WidthMM
	if p == PartHeight {
		mm = info.HeightMM
	}
	unit := f.model.Options().Unit
	if unit == print.UnitCM {
		mm = math.Round(mm)
	}
	return formatSize(mm, unit)
}

func (f *form) drawScrollbar(c widget.DrawContext, t *widget.Theme) {
	if f.lay.scrollMax <= 0 {
		return
	}
	v := f.viewRect()
	w := max(3, f.lay.m.s(4))
	track := v.Dy()
	thumb := max(f.lay.m.s(24), track*track/(track+f.lay.scrollMax))
	y := v.Min.Y + (track-thumb)*f.scroll/f.lay.scrollMax
	r := image.Rect(v.Max.X+f.lay.m.s(6), y, v.Max.X+f.lay.m.s(6)+w, y+thumb)
	gfx.FillRound(c, r, float64(w)/2, gfx.Alpha(t.LabelText, .35))
}
