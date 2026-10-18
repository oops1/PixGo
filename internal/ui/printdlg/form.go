package printdlg

import (
	"image"
	"image/color"
	"strconv"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/print"
)

const ringGap = 2

var errorInk = color.RGBA{232, 90, 90, 255}

var warnInk = color.RGBA{224, 160, 48, 255}

type textField struct {
	caret int
	all   bool
}

type listState struct {
	part  Part
	items []string
	sel   int
	hover int
	top   int
	rows  int
	rect  image.Rectangle
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
	scroll  int

	fc, fw, fh textField

	list *listState

	capMgr    widget.CaptureManager
	autofocus bool

	onPrint  func()
	onCancel func()
	onSystem func()
}

func newForm(lay layout, m *Model) *form {
	f := &form{lay: lay, model: m, stop: PartPaper}
	if !lay.export {
		f.stop = PartPrinter
	}
	f.resetFields()
	return f
}

func (f *form) resetFields() {
	f.fc = textField{caret: len(f.model.CopiesText()), all: true}
	f.fw = textField{caret: len(f.model.WidthText()), all: true}
	f.fh = textField{caret: len(f.model.HeightText()), all: true}
}

func (f *form) origin() image.Point { return f.Bounds().Min }

func (f *form) scrollOrigin() image.Point {
	return f.Bounds().Min.Add(f.lay.view.Min).Sub(image.Pt(0, f.scroll))
}

func (f *form) rect(p Part) image.Rectangle {
	if p <= PartNone || p >= partCount {
		return image.Rectangle{}
	}
	r := f.lay.ctl[p]
	if r.Empty() {
		return r
	}
	if fixedPart(p) {
		return r.Add(f.origin())
	}
	return r.Add(f.scrollOrigin())
}

func (f *form) labelRect(p Part) image.Rectangle {
	r := f.lay.lbl[p]
	if r.Empty() {
		return r
	}
	return r.Add(f.scrollOrigin())
}

func (f *form) viewRect() image.Rectangle { return f.lay.view.Add(f.origin()) }

func (f *form) previewRect() image.Rectangle {
	if f.lay.wide {
		return f.lay.preview.Add(f.origin())
	}
	return f.lay.preview.Add(f.scrollOrigin())
}

func (f *form) infoRect() image.Rectangle {
	if f.lay.wide {
		return f.lay.info.Add(f.origin())
	}
	return f.lay.info.Add(f.scrollOrigin())
}

func (f *form) shown(p Part) bool { return !f.lay.ctl[p].Empty() }

func (f *form) enabled(p Part) bool {
	if !f.shown(p) {
		return false
	}
	pdf := f.model.PDF()
	switch p {
	case PartSystem, PartCopies, PartCopiesDec, PartCopiesInc:
		return !pdf
	case PartWidth, PartHeight, PartCm, PartInch, PartKeep:
		return f.model.CustomActive()
	case PartPrint:
		return f.model.Valid()
	}
	return true
}

var order = []Part{
	PartPrinter, PartSystem, PartCopies, PartPaper, PartOrientation, PartPlacement,
	PartWidth, PartHeight, PartCm, PartInch, PartKeep, PartMargins, PartPrint, PartCancel,
}

func isDropdown(p Part) bool {
	switch p {
	case PartPrinter, PartPaper, PartOrientation, PartPlacement, PartMargins:
		return true
	}
	return false
}

func isField(p Part) bool { return p == PartCopies || p == PartWidth || p == PartHeight }

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
	f.closeList()
	list := f.stops()
	if len(list) == 0 {
		return
	}
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
	case PartCopies:
		f.fc = textField{caret: len(f.model.CopiesText()), all: true}
	case PartWidth:
		f.fw = textField{caret: len(f.model.WidthText()), all: true}
	case PartHeight:
		f.fh = textField{caret: len(f.model.HeightText()), all: true}
	}
	f.reveal(p)
}

func (f *form) reveal(p Part) {
	if fixedPart(p) || f.lay.scrollMax == 0 {
		return
	}
	r := f.lay.ctl[p]
	if r.Empty() {
		return
	}
	pad := f.lay.m.rowGap
	switch {
	case r.Min.Y-pad < f.scroll:
		f.scroll = max(0, r.Min.Y-pad)
	case r.Max.Y+pad > f.scroll+f.lay.view.Dy():
		f.scroll = min(f.lay.scrollMax, r.Max.Y+pad-f.lay.view.Dy())
	}
}

func (f *form) scrollBy(d int) bool {
	if f.lay.scrollMax == 0 || d == 0 {
		return false
	}
	old := f.scroll
	f.scroll = min(max(f.scroll+d, 0), f.lay.scrollMax)
	if f.scroll != old {
		f.Invalidate()
	}
	return true
}

func (f *form) fieldOf(p Part) (*textField, string) {
	switch p {
	case PartWidth:
		return &f.fw, f.model.WidthText()
	case PartHeight:
		return &f.fh, f.model.HeightText()
	}
	return &f.fc, f.model.CopiesText()
}

func (f *form) textOf(p Part) string {
	_, t := f.fieldOf(p)
	return t
}

func (f *form) clampCarets() {
	f.fc.caret = min(f.fc.caret, len(f.model.CopiesText()))
	f.fw.caret = min(f.fw.caret, len(f.model.WidthText()))
	f.fh.caret = min(f.fh.caret, len(f.model.HeightText()))
}

func (f *form) setText(p Part, s string) {
	switch p {
	case PartWidth:
		f.model.SetWidthText(s)
	case PartHeight:
		f.model.SetHeightText(s)
	default:
		f.model.SetCopiesText(s)
	}
	f.clampCarets()
	f.Invalidate()
}

func (f *form) typeRune(p Part, r rune) {
	fl, text := f.fieldOf(p)
	if fl.all {
		text, fl.caret, fl.all = "", 0, false
	}
	old := fl.caret
	f.setText(p, text[:old]+string(r)+text[old:])
	cleaned := f.textOf(p)
	fl.caret = min(max(old+len(cleaned)-len(text), 0), len(cleaned))
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

func (f *form) bump(p Part, dir int, big bool) {
	if p == PartCopies {
		n := f.model.Copies() + dir
		if big {
			n = f.model.Copies() + 10*dir
		}
		f.model.SetCopies(n)
		fl, _ := f.fieldOf(p)
		fl.caret, fl.all = len(f.model.CopiesText()), true
		f.Invalidate()
		return
	}
	text := f.textOf(p)
	mm, ok := ParseSize(text, f.model.Options().Unit)
	if !ok {
		mm = print.MinCustomMM
	}
	unit := f.model.Options().Unit
	step := 0.1
	if big {
		step = 1
	}
	v := mm/unit.MM() + float64(dir)*step
	v = max(v, 0.1)
	s := strconv.FormatFloat(v, 'f', 2, 64)
	for len(s) > 0 && s[len(s)-1] == '0' && containsDot(s) {
		s = s[:len(s)-1]
	}
	if len(s) > 0 && s[len(s)-1] == '.' {
		s = s[:len(s)-1]
	}
	f.setText(p, s)
	fl, _ := f.fieldOf(p)
	fl.caret, fl.all = len(f.textOf(p)), true
}

func containsDot(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == '.' {
			return true
		}
	}
	return false
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
		f.bump(p, 1, e.Mod&widget.ModShift != 0)
	case widget.KeyDown:
		f.bump(p, -1, e.Mod&widget.ModShift != 0)
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

func (f *form) fieldRune(p Part, e widget.KeyEvent) bool {
	if e.Mod&(widget.ModCtrl|widget.ModAlt) != 0 {
		return false
	}
	switch {
	case e.Rune >= '0' && e.Rune <= '9':
	case (e.Rune == '.' || e.Rune == ',') && p != PartCopies:
	default:
		return false
	}
	f.typeRune(p, e.Rune)
	return true
}

func (f *form) setUnit(u print.Unit) {
	f.model.SetUnit(u)
	f.fw = textField{caret: len(f.model.WidthText()), all: f.stop == PartWidth}
	f.fh = textField{caret: len(f.model.HeightText()), all: f.stop == PartHeight}
	f.Invalidate()
}

func (f *form) setKeep(on bool) {
	f.model.SetKeep(on)
	f.clampCarets()
	f.Invalidate()
}

func (f *form) activate(p Part) {
	switch p {
	case PartPrinter, PartPaper, PartOrientation, PartPlacement, PartMargins:
		f.toggleList(p)
	case PartSystem:
		if f.onSystem != nil {
			f.onSystem()
		}
	case PartCopiesDec:
		f.bump(PartCopies, -1, false)
	case PartCopiesInc:
		f.bump(PartCopies, 1, false)
	case PartCm:
		f.setUnit(print.UnitCM)
	case PartInch:
		f.setUnit(print.UnitInch)
	case PartKeep:
		f.setKeep(!f.model.Options().Keep)
	case PartPrint:
		if f.model.Valid() && f.onPrint != nil {
			f.onPrint()
		}
	case PartCancel:
		if f.onCancel != nil {
			f.onCancel()
		}
	}
}

func (f *form) activateDefault() {
	if f.list != nil {
		f.pickList(f.list.sel)
		return
	}
	switch f.stop {
	case PartCancel:
		f.activate(PartCancel)
	case PartSystem:
		f.activate(PartSystem)
	default:
		f.activate(PartPrint)
	}
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

func (f *form) OnKeyEvent(e widget.KeyEvent) {
	if !e.Pressed {
		return
	}
	f.keyNav = true
	if f.list != nil {
		f.listKey(e)
		f.Invalidate()
		return
	}
	if !f.enabled(f.stop) {
		if l := f.stops(); len(l) > 0 && e.Code != widget.KeyTab {
			f.enter(l[0])
		}
	}
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
	switch {
	case isDropdown(f.stop):
		f.dropdownKey(f.stop, e)
	case isField(f.stop):
		f.fieldKey(f.stop, e)
	case f.stop == PartCm || f.stop == PartInch:
		switch e.Code {
		case widget.KeySpace:
			f.activate(f.stop)
		case widget.KeyLeft, widget.KeyUp:
			f.stop = PartCm
			f.activate(PartCm)
		case widget.KeyRight, widget.KeyDown:
			f.stop = PartInch
			f.activate(PartInch)
		}
	default:
		if e.Code == widget.KeySpace {
			f.activate(f.stop)
		}
	}
	f.Invalidate()
}

func (f *form) dropdownKey(p Part, e widget.KeyEvent) {
	items, sel := f.itemsOf(p)
	switch e.Code {
	case widget.KeySpace, widget.KeyF4:
		f.openList(p)
	case widget.KeyDown:
		if e.Mod&widget.ModAlt != 0 {
			f.openList(p)
			return
		}
		f.choose(p, min(sel+1, len(items)-1))
	case widget.KeyUp:
		if e.Mod&widget.ModAlt != 0 {
			f.openList(p)
			return
		}
		f.choose(p, max(sel-1, 0))
	case widget.KeyRight:
		f.choose(p, min(sel+1, len(items)-1))
	case widget.KeyLeft:
		f.choose(p, max(sel-1, 0))
	case widget.KeyHome:
		f.choose(p, 0)
	case widget.KeyEnd:
		f.choose(p, len(items)-1)
	}
}
