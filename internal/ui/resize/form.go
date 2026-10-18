package resize

import (
	"image"
	"image/color"
	"strconv"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/ui/gfx"
	"github.com/oops1/PixGo/internal/ui/icons"
)

type Part uint8

const (
	PartNone Part = iota
	PartWidth
	PartHeight
	PartPixels
	PartPercent
	PartKeep
	PartApply
	PartCancel
)

const ringGap = 2

var errorInk = color.RGBA{232, 90, 90, 255}

type field struct {
	caret int
	all   bool
}

type form struct {
	widget.Base
	lay   layout
	model *Model

	stop    Part
	hover   Part
	pressed Part
	focused bool
	keyNav  bool

	fw, fh field

	capMgr    widget.CaptureManager
	autofocus bool

	onApply  func(image.Point)
	onCancel func()
}

func newForm(lay layout, m *Model) *form {
	f := &form{lay: lay, model: m, stop: PartWidth}
	f.fw = field{caret: len(m.WidthText()), all: true}
	f.fh = field{caret: len(m.HeightText()), all: true}
	return f
}

func (f *form) rect(p Part) image.Rectangle {
	var r image.Rectangle
	switch p {
	case PartWidth:
		r = f.lay.width
	case PartHeight:
		r = f.lay.height
	case PartPixels:
		r = f.lay.pixels
	case PartPercent:
		r = f.lay.percent
	case PartKeep:
		r = f.lay.keep
	case PartApply:
		r = f.lay.apply
	case PartCancel:
		r = f.lay.cancel
	}
	if r.Empty() {
		return r
	}
	return r.Add(f.Bounds().Min)
}

func (f *form) valid() bool {
	_, st := f.model.Result()
	return st == StatusOK
}

func (f *form) enabled(p Part) bool {
	if p == PartApply {
		return f.valid()
	}
	return p != PartNone
}

var order = []Part{PartWidth, PartHeight, PartPixels, PartPercent, PartKeep, PartApply, PartCancel}

func (f *form) stops() []Part {
	var out []Part
	for _, p := range order {
		if f.enabled(p) {
			out = append(out, p)
		}
	}
	return out
}

func (f *form) moveStop(dir int) {
	list := f.stops()
	at := -1
	for i, p := range list {
		if p == f.stop {
			at = i
		}
	}
	switch {
	case at < 0 && dir > 0:
		at = 0
	case at < 0:
		at = len(list) - 1
	default:
		at = (at + dir + len(list)) % len(list)
	}
	f.enter(list[at])
	f.Invalidate()
}

func (f *form) enter(p Part) {
	f.stop = p
	switch p {
	case PartWidth:
		f.fw = field{caret: len(f.model.WidthText()), all: true}
	case PartHeight:
		f.fh = field{caret: len(f.model.HeightText()), all: true}
	}
}

func (f *form) fieldOf(p Part) (*field, string) {
	if p == PartHeight {
		return &f.fh, f.model.HeightText()
	}
	return &f.fw, f.model.WidthText()
}

func (f *form) setText(p Part, s string) {
	if p == PartHeight {
		f.model.SetHeight(s)
	} else {
		f.model.SetWidth(s)
	}
	f.clampCarets()
	f.Invalidate()
}

func (f *form) clampCarets() {
	f.fw.caret = min(f.fw.caret, len(f.model.WidthText()))
	f.fh.caret = min(f.fh.caret, len(f.model.HeightText()))
}

func (f *form) typeDigit(p Part, r rune) {
	fl, text := f.fieldOf(p)
	if fl.all {
		text, fl.caret, fl.all = "", 0, false
	}
	if len(text) >= maxDigits {
		return
	}
	next := text[:fl.caret] + string(r) + text[fl.caret:]
	fl.caret++
	f.setText(p, next)
	fl.caret = min(fl.caret, len(f.textOf(p)))
}

func (f *form) textOf(p Part) string {
	_, t := f.fieldOf(p)
	return t
}

func (f *form) backspace(p Part) {
	fl, text := f.fieldOf(p)
	switch {
	case fl.all:
		fl.all, fl.caret = false, 0
		f.setText(p, "")
	case fl.caret > 0:
		fl.caret--
		f.setText(p, text[:fl.caret]+text[fl.caret+1:])
	}
}

func (f *form) deleteForward(p Part) {
	fl, text := f.fieldOf(p)
	switch {
	case fl.all:
		fl.all, fl.caret = false, 0
		f.setText(p, "")
	case fl.caret < len(text):
		f.setText(p, text[:fl.caret]+text[fl.caret+1:])
	}
}

func (f *form) bump(p Part, delta int) {
	_, text := f.fieldOf(p)
	n, _ := strconv.Atoi(text)
	n = max(1, n+delta)
	fl, _ := f.fieldOf(p)
	f.setText(p, strconv.Itoa(n))
	fl.caret, fl.all = len(f.textOf(p)), true
}

func (f *form) fieldKey(p Part, e widget.KeyEvent) bool {
	fl, text := f.fieldOf(p)
	ctrl := e.Mod&widget.ModCtrl != 0
	switch e.Code {
	case widget.KeyBackspace:
		f.backspace(p)
	case widget.KeyDelete:
		f.deleteForward(p)
	case widget.KeyLeft:
		if fl.all {
			fl.caret, fl.all = 0, false
		} else if fl.caret > 0 {
			fl.caret--
		}
	case widget.KeyRight:
		if fl.all {
			fl.caret, fl.all = len(text), false
		} else if fl.caret < len(text) {
			fl.caret++
		}
	case widget.KeyHome:
		fl.caret, fl.all = 0, false
	case widget.KeyEnd:
		fl.caret, fl.all = len(text), false
	case widget.KeyUp:
		f.bump(p, step(e))
	case widget.KeyDown:
		f.bump(p, -step(e))
	case widget.KeyA:
		if !ctrl {
			return f.fieldRune(p, e)
		}
		fl.all = true
	default:
		return f.fieldRune(p, e)
	}
	f.Invalidate()
	return true
}

func step(e widget.KeyEvent) int {
	if e.Mod&widget.ModShift != 0 {
		return 10
	}
	return 1
}

func (f *form) fieldRune(p Part, e widget.KeyEvent) bool {
	if e.Mod&(widget.ModCtrl|widget.ModAlt) == 0 && e.Rune >= '0' && e.Rune <= '9' {
		f.typeDigit(p, e.Rune)
		return true
	}
	return false
}

func (f *form) setUnit(u Unit) {
	f.model.SetUnit(u)
	f.fw = field{caret: len(f.model.WidthText()), all: f.stop == PartWidth}
	f.fh = field{caret: len(f.model.HeightText()), all: f.stop == PartHeight}
	f.Invalidate()
}

func (f *form) setKeep(on bool) {
	f.model.SetKeep(on)
	f.clampCarets()
	f.Invalidate()
}

func (f *form) activate(p Part) {
	switch p {
	case PartPixels:
		f.setUnit(UnitPixels)
	case PartPercent:
		f.setUnit(UnitPercent)
	case PartKeep:
		f.setKeep(!f.model.Keep())
	case PartApply:
		if size, st := f.model.Result(); st == StatusOK && f.onApply != nil {
			f.onApply(size)
		}
	case PartCancel:
		if f.onCancel != nil {
			f.onCancel()
		}
	}
}

func (f *form) activateDefault() {
	if f.stop == PartCancel {
		f.activate(PartCancel)
		return
	}
	f.activate(PartApply)
}

func (f *form) SetFocused(v bool) {
	if f.focused == v {
		return
	}
	f.focused = v
	f.Invalidate()
}

func (f *form) IsFocused() bool { return f.focused }

func (f *form) AcceptsTab() bool { return true }

func (f *form) SetCaptureManager(cm widget.CaptureManager) {
	f.capMgr = cm
	if f.autofocus {
		return
	}
	f.autofocus = true
	if s, ok := cm.(interface{ SetFocus(widget.Widget) }); ok {
		s.SetFocus(f)
	}
}

func (f *form) WantsCapture(e widget.MouseEvent) bool {
	return e.Button == widget.MouseLeft && e.Pressed && image.Pt(e.X, e.Y).In(f.Bounds())
}

func (f *form) release() {
	if f.capMgr != nil {
		f.capMgr.ReleaseCapture()
	}
}

func (f *form) hitAt(x, y int) Part {
	pt := image.Pt(x, y)
	for _, p := range order {
		if f.enabled(p) && pt.In(f.rect(p)) {
			return p
		}
	}
	return PartNone
}

func (f *form) caretAt(p Part, x int) int {
	text := f.textOf(p)
	r := f.rect(p)
	left := r.Min.X + f.fieldPad()
	best, bestD := 0, 1<<30
	for i := 0; i <= len(text); i++ {
		cx := left + bodyFont.Measure(nil, text[:i])
		if d := abs(cx - x); d < bestD {
			best, bestD = i, d
		}
	}
	return best
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func (f *form) fieldPad() int { return f.lay.m.s(10) }

func (f *form) OnMouseButton(e widget.MouseEvent) bool {
	if e.Button != widget.MouseLeft {
		return false
	}
	if e.Pressed {
		f.keyNav = false
		h := f.hitAt(e.X, e.Y)
		f.pressed = h
		if h == PartWidth || h == PartHeight {
			fl, _ := f.fieldOf(h)
			f.stop = h
			fl.caret, fl.all = f.caretAt(h, e.X), false
		} else if h != PartNone {
			f.stop = h
		}
		f.Invalidate()
		return true
	}
	over := f.hitAt(e.X, e.Y)
	pressed := f.pressed
	f.pressed = PartNone
	f.release()
	f.Invalidate()
	if pressed != PartNone && pressed == over && pressed != PartWidth && pressed != PartHeight {
		f.activate(pressed)
	}
	return true
}

func (f *form) OnMouseMove(x, y int) {
	if h := f.hitAt(x, y); h != f.hover {
		f.hover = h
		f.Invalidate()
	}
}

func (f *form) Cursor(x, y int) widget.Cursor {
	switch f.hitAt(x, y) {
	case PartWidth, PartHeight:
		return widget.CursorIBeam
	case PartNone:
		return widget.CursorArrow
	}
	return widget.CursorHand
}

func (f *form) OnKeyEvent(e widget.KeyEvent) {
	if !e.Pressed {
		return
	}
	f.keyNav = true
	switch e.Code {
	case widget.KeyTab:
		if e.Mod&widget.ModShift != 0 {
			f.moveStop(-1)
		} else {
			f.moveStop(1)
		}
		return
	case widget.KeyEnter:
		f.activateDefault()
		return
	}
	switch f.stop {
	case PartWidth, PartHeight:
		f.fieldKey(f.stop, e)
	case PartPixels, PartPercent:
		switch e.Code {
		case widget.KeySpace:
			f.activate(f.stop)
		case widget.KeyLeft, widget.KeyUp:
			f.stop = PartPixels
			f.activate(PartPixels)
		case widget.KeyRight, widget.KeyDown:
			f.stop = PartPercent
			f.activate(PartPercent)
		}
	default:
		if e.Code == widget.KeySpace {
			f.activate(f.stop)
		}
	}
	f.Invalidate()
}

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

func (f *form) showRing(p Part) bool { return f.focused && f.keyNav && f.stop == p }

func (f *form) ring(c widget.DrawContext, t *widget.Theme, r image.Rectangle, radius float64) {
	gfx.StrokeRound(c, r.Inset(-ringGap), radius+ringGap, 2, t.LabelText)
}

func (f *form) Draw(c widget.DrawContext) {
	b := f.Bounds()
	if b.Empty() {
		return
	}
	restore := gfx.Clip(c, b)
	defer restore()
	t := widget.CurrentTheme()
	f.drawUnits(c, t)
	f.drawLabels(c, t)
	f.drawField(c, t, PartWidth)
	f.drawField(c, t, PartHeight)
	f.drawKeep(c, t)
	f.drawInfo(c, t)
	f.drawButton(c, t, PartApply, i18n.T("resize.apply"), true)
	f.drawButton(c, t, PartCancel, i18n.T("dialog.cancel"), false)
}

func (f *form) drawUnits(c widget.DrawContext, t *widget.Theme) {
	px, pc := f.rect(PartPixels), f.rect(PartPercent)
	whole := px.Union(pc)
	radius := 4 * f.lay.m.ks
	gfx.FillRound(c, whole, radius, t.BtnBG)
	sel, other := px, pc
	if f.model.Unit() == UnitPercent {
		sel, other = pc, px
	}
	restore := gfx.Clip(c, sel)
	gfx.FillRound(c, whole, radius, t.Accent)
	restore()
	if f.hover == PartPixels && sel != px || f.hover == PartPercent && sel != pc {
		restore := gfx.Clip(c, other)
		gfx.FillRound(c, whole, radius, gfx.Alpha(t.LabelText, .08))
		restore()
	}
	gfx.StrokeRound(c, whole, radius, 1, t.BtnBorder)
	mid := (px.Max.X + pc.Min.X) / 2
	c.FillRect(mid, whole.Min.Y+4, 1, whole.Dy()-8, t.BtnBorder)
	for _, p := range []struct {
		r    image.Rectangle
		text string
		on   bool
	}{{px, i18n.T("resize.unit.px"), sel == px}, {pc, i18n.T("resize.unit.pct"), sel == pc}} {
		ink := t.BtnText
		if p.on {
			ink = inkOn(t.Accent)
		}
		w := bodyFont.Measure(c, p.text)
		bodyFont.Draw(c, p.text, p.r.Min.X+(p.r.Dx()-w)/2, baseline(p.r, bodyFont), ink)
	}
	if f.showRing(PartPixels) {
		f.ring(c, t, px, radius)
	}
	if f.showRing(PartPercent) {
		f.ring(c, t, pc, radius)
	}
}

func (f *form) drawLabels(c widget.DrawContext, t *widget.Theme) {
	for _, p := range []struct {
		r    image.Rectangle
		text string
	}{{f.lay.wLabel, i18n.T("resize.width")}, {f.lay.hLabel, i18n.T("resize.height")}} {
		r := p.r.Add(f.Bounds().Min)
		bodyFont.Draw(c, p.text, r.Min.X, baseline(r, bodyFont), t.LabelText)
	}
}

func (f *form) drawField(c widget.DrawContext, t *widget.Theme, p Part) {
	r := f.rect(p)
	radius := 4 * f.lay.m.ks
	focus := f.focused && f.stop == p
	gfx.FillRound(c, r, radius, t.InputBG)
	gfx.StrokeRound(c, r, radius, 1, t.InputBorder)
	if focus {
		c.FillRect(r.Min.X+3, r.Max.Y-2, r.Dx()-6, 2, t.InputFocus)
	}
	fl, text := f.fieldOf(p)
	pad := f.fieldPad()
	x := r.Min.X + pad
	base := baseline(r, bodyFont)
	invalid := text == ""
	if focus && fl.all && text != "" {
		w := bodyFont.Measure(c, text)
		hh := textHeight(bodyFont)
		c.FillRectAlpha(x, r.Min.Y+(r.Dy()-hh)/2, w, hh, t.TextSelectionBG)
	}
	ink := t.InputText
	if invalid {
		ink = errorInk
	}
	bodyFont.Draw(c, text, x, base, ink)
	if f.model.Unit() == UnitPercent {
		suffix := "%"
		sw := bodyFont.Measure(c, suffix)
		bodyFont.Draw(c, suffix, r.Max.X-pad-sw, base, t.SecondaryText)
	}
	if focus && !fl.all {
		cx := x + bodyFont.Measure(c, text[:min(fl.caret, len(text))])
		hh := textHeight(bodyFont)
		c.FillRect(cx, r.Min.Y+(r.Dy()-hh)/2, 1, hh, t.InputCaret)
	}
	if f.showRing(p) {
		f.ring(c, t, r, radius)
	}
}

func (f *form) drawKeep(c widget.DrawContext, t *widget.Theme) {
	m := f.lay.m
	row := f.rect(PartKeep)
	top := row.Min.Y + (row.Dy()-m.boxSize)/2
	box := image.Rect(row.Min.X, top, row.Min.X+m.boxSize, top+m.boxSize)
	radius := 3 * m.ks
	hover := f.hover == PartKeep || f.pressed == PartKeep
	if f.model.Keep() {
		fill := t.Accent
		if hover {
			fill = gfx.Blend(t.Accent, color.RGBA{255, 255, 255, 255}, .12)
		}
		gfx.FillRound(c, box, radius, fill)
		icons.Draw(c, "checkmark", box.Min.X+(box.Dx()-12)/2, box.Min.Y+(box.Dy()-12)/2, inkOn(t.Accent))
	} else {
		fill := t.CheckBG
		if hover {
			fill = t.CheckHoverBG
		}
		gfx.FillRound(c, box, radius, fill)
		gfx.StrokeRound(c, box, radius, 1.3, t.CheckBorder)
	}
	bodyFont.Draw(c, i18n.T("resize.keep"), box.Max.X+m.boxGap, baseline(row, bodyFont), t.LabelText)
	if f.showRing(PartKeep) {
		f.ring(c, t, box, radius)
	}
}

func (f *form) ResultText() string {
	size, st := f.model.Result()
	switch st {
	case StatusEmpty:
		return i18n.T("resize.invalid")
	case StatusTooBig:
		return i18n.T("resize.tooBig") + " (" + i18n.T("format.dimensions", size.X, size.Y) + ")"
	}
	return i18n.T("resize.result", size.X, size.Y)
}

func (f *form) OrigText() string {
	o := f.model.Orig()
	return i18n.T("resize.original", o.X, o.Y)
}

func (f *form) drawInfo(c widget.DrawContext, t *widget.Theme) {
	off := f.Bounds().Min
	if !f.lay.orig.Empty() {
		o := f.lay.orig.Add(off)
		bodyFont.Draw(c, f.OrigText(), o.Min.X, baseline(o, bodyFont), t.SecondaryText)
	}
	r := f.lay.result.Add(off)
	ink := t.LabelText
	if _, st := f.model.Result(); st != StatusOK {
		ink = errorInk
	}
	bodyFont.DrawFit(c, f.ResultText(), r.Min.X, baseline(r, bodyFont), r.Dx(), ink)
	h := f.lay.hint.Add(off)
	hs := textHeight(hintFont) + f.lay.m.lineGap
	for i, line := range f.lay.hintText {
		row := image.Rect(h.Min.X, h.Min.Y+i*hs, h.Max.X, h.Min.Y+(i+1)*hs)
		hintFont.Draw(c, line, row.Min.X, baseline(row, hintFont), t.SecondaryText)
	}
}

func (f *form) drawButton(c widget.DrawContext, t *widget.Theme, p Part, text string, primary bool) {
	r := f.rect(p)
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
