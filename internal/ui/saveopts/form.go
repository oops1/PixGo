package saveopts

import (
	"image"
	"image/color"
	"math"
	"strconv"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/ui/gfx"
	"github.com/oops1/PixGo/internal/ui/icons"
)

type Part uint8

const (
	PartNone Part = iota
	PartCheck
	PartSlider
	PartSave
	PartCancel
)

const (
	pageStep = 10
	ringGap  = 2
)

type form struct {
	widget.Base
	kind Kind
	lay  layout

	quality  int
	lossless bool

	stop     Part
	hover    Part
	pressed  Part
	dragging bool
	focused  bool
	keyNav   bool

	capMgr    widget.CaptureManager
	autofocus bool

	onSave   func()
	onCancel func()
}

func newForm(kind Kind, lay layout, v Values) *form {
	f := &form{kind: kind, lay: lay, quality: clampQuality(v.Quality), lossless: kind.lossless() && v.Lossless}
	f.stop = PartSlider
	if lay.hasCheck {
		f.stop = PartCheck
	}
	return f
}

func clampQuality(q int) int { return min(max(q, MinQuality), MaxQuality) }

func (f *form) rect(p Part) image.Rectangle {
	var r image.Rectangle
	switch p {
	case PartCheck:
		r = f.lay.check
	case PartSlider:
		r = f.lay.slider
	case PartSave:
		r = f.lay.save
	case PartCancel:
		r = f.lay.cancel
	}
	if r.Empty() {
		return r
	}
	return r.Add(f.Bounds().Min)
}

func (f *form) enabled(p Part) bool {
	switch p {
	case PartCheck:
		return f.lay.hasCheck
	case PartSlider:
		return !f.lossless
	case PartSave, PartCancel:
		return true
	}
	return false
}

func (f *form) stops() []Part {
	var out []Part
	for _, p := range []Part{PartCheck, PartSlider, PartSave, PartCancel} {
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
	f.stop = list[at]
	f.Invalidate()
}

func (f *form) setQuality(v int) {
	v = clampQuality(v)
	if v == f.quality {
		return
	}
	f.quality = v
	f.Invalidate()
}

func (f *form) setLossless(on bool) {
	if !f.lay.hasCheck || on == f.lossless {
		return
	}
	f.lossless = on
	if on && f.stop == PartSlider {
		f.stop = PartCheck
	}
	f.dragging = false
	f.Invalidate()
}

func (f *form) values() Values { return Values{Quality: f.quality, Lossless: f.lossless} }

func (f *form) track() (x0, x1, cy int) {
	r := f.rect(PartSlider)
	return r.Min.X + f.lay.m.thumbR, r.Max.X - f.lay.m.thumbR, r.Min.Y + r.Dy()/2
}

func (f *form) qualityAt(x int) int {
	x0, x1, _ := f.track()
	if x1 <= x0 {
		return MinQuality
	}
	ratio := min(max(float64(x-x0)/float64(x1-x0), 0), 1)
	return clampQuality(MinQuality + int(math.Round(ratio*float64(MaxQuality-MinQuality))))
}

func (f *form) thumbX() float64 {
	x0, x1, _ := f.track()
	ratio := float64(f.quality-MinQuality) / float64(MaxQuality-MinQuality)
	return float64(x0) + ratio*float64(x1-x0)
}

func (f *form) hitAt(x, y int) Part {
	pt := image.Pt(x, y)
	for _, p := range []Part{PartSave, PartCancel, PartSlider, PartCheck} {
		if f.enabled(p) && pt.In(f.rect(p)) {
			return p
		}
	}
	return PartNone
}

func (f *form) activate(p Part) {
	switch p {
	case PartCheck:
		f.setLossless(!f.lossless)
	case PartSave:
		if f.onSave != nil {
			f.onSave()
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
	f.activate(PartSave)
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

func (f *form) OnMouseButton(e widget.MouseEvent) bool {
	if e.Button != widget.MouseLeft {
		return false
	}
	if e.Pressed {
		f.keyNav = false
		h := f.hitAt(e.X, e.Y)
		f.pressed = h
		if h != PartNone {
			f.stop = h
		}
		if h == PartSlider {
			f.dragging = true
			f.setQuality(f.qualityAt(e.X))
		}
		f.Invalidate()
		return true
	}
	over := f.hitAt(e.X, e.Y)
	pressed := f.pressed
	f.pressed, f.dragging = PartNone, false
	f.release()
	f.Invalidate()
	if pressed != PartNone && pressed == over && pressed != PartSlider {
		f.activate(pressed)
	}
	return true
}

func (f *form) OnMouseMove(x, y int) {
	if f.dragging {
		f.setQuality(f.qualityAt(x))
		return
	}
	if h := f.hitAt(x, y); h != f.hover {
		f.hover = h
		f.Invalidate()
	}
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
	case widget.KeySpace:
		if f.stop != PartSlider {
			f.activate(f.stop)
		}
	case widget.KeyEnter:
		f.activateDefault()
	default:
		if f.stop == PartSlider {
			f.sliderKey(e.Code)
		}
	}
	f.Invalidate()
}

func (f *form) sliderKey(code widget.KeyCode) {
	switch code {
	case widget.KeyLeft, widget.KeyDown:
		f.setQuality(f.quality - 1)
	case widget.KeyRight, widget.KeyUp:
		f.setQuality(f.quality + 1)
	case widget.KeyPageDown:
		f.setQuality(f.quality - pageStep)
	case widget.KeyPageUp:
		f.setQuality(f.quality + pageStep)
	case widget.KeyHome:
		f.setQuality(MinQuality)
	case widget.KeyEnd:
		f.setQuality(MaxQuality)
	}
}

func (f *form) Draw(c widget.DrawContext) {
	b := f.Bounds()
	if b.Empty() {
		return
	}
	restore := gfx.Clip(c, b)
	defer restore()
	t := widget.CurrentTheme()
	if f.lay.hasCheck {
		f.drawCheck(c, t)
	}
	f.drawCaption(c, t)
	f.drawSlider(c, t)
	f.drawHint(c, t)
	f.drawButton(c, t, PartSave, i18n.T("save.options.save"), true)
	f.drawButton(c, t, PartCancel, i18n.T("dialog.cancel"), false)
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

func (f *form) ring(c widget.DrawContext, t *widget.Theme, r image.Rectangle, radius float64) {
	gfx.StrokeRound(c, r.Inset(-ringGap), radius+ringGap, 2, t.LabelText)
}

func (f *form) showRing(p Part) bool { return f.focused && f.keyNav && f.stop == p }

func (f *form) drawCheck(c widget.DrawContext, t *widget.Theme) {
	m := f.lay.m
	row := f.rect(PartCheck)
	top := row.Min.Y + (row.Dy()-m.boxSize)/2
	box := image.Rect(row.Min.X, top, row.Min.X+m.boxSize, top+m.boxSize)
	radius := 3 * m.ks
	hover := f.hover == PartCheck || f.pressed == PartCheck
	if f.lossless {
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
	label := i18n.T("save.lossless")
	bodyFont.Draw(c, label, box.Max.X+m.boxGap, baseline(row, bodyFont), t.LabelText)
	if f.showRing(PartCheck) {
		f.ring(c, t, box, radius)
	}
}

func (f *form) valueText() string { return strconv.Itoa(f.quality) }

func (f *form) drawCaption(c widget.DrawContext, t *widget.Theme) {
	label, value := t.LabelText, t.SecondaryText
	if f.lossless {
		label, value = t.Disabled, t.Disabled
	}
	cap := f.lay.caption.Add(f.Bounds().Min)
	bodyFont.Draw(c, i18n.T("save.quality"), cap.Min.X, baseline(cap, bodyFont), label)
	text := f.valueText()
	right := f.lay.value.Add(f.Bounds().Min).Max.X
	bodyFont.Draw(c, text, right-bodyFont.Measure(c, text), baseline(cap, bodyFont), value)
}

func (f *form) drawSlider(c widget.DrawContext, t *widget.Theme) {
	m := f.lay.m
	x0, x1, cy := f.track()
	tx := f.thumbX()
	off := f.lossless
	trackCol := gfx.Blend(t.DialogBG, t.LabelText, .3)
	fillCol := t.Accent
	if off {
		trackCol = gfx.Blend(t.DialogBG, t.LabelText, .15)
		fillCol = gfx.Blend(t.DialogBG, t.LabelText, .3)
	}
	top := cy - m.trackH/2
	half := float64(m.trackH) / 2
	gfx.FillRound(c, image.Rect(x0, top, x1, top+m.trackH), half, trackCol)
	if int(tx)-x0 >= m.trackH {
		gfx.FillRound(c, image.Rect(x0, top, int(math.Round(tx)), top+m.trackH), half, fillCol)
	}
	outer := gfx.Blend(t.DialogBG, t.LabelText, .22)
	core := float64(m.thumbR) * .55
	switch {
	case off:
		outer = gfx.Blend(t.DialogBG, t.LabelText, .12)
		core = float64(m.thumbR) * .45
	case f.dragging:
		core = float64(m.thumbR) * .45
	case f.hover == PartSlider:
		core = float64(m.thumbR) * .7
	}
	fy := float64(cy)
	gfx.StrokeCircle(c, tx, fy, float64(m.thumbR)+1, 1, gfx.Alpha(color.RGBA{0, 0, 0, 255}, .25))
	gfx.FillCircle(c, tx, fy, float64(m.thumbR), outer)
	coreCol := t.Accent
	if off {
		coreCol = gfx.Blend(t.DialogBG, t.LabelText, .3)
	}
	gfx.FillCircle(c, tx, fy, core, coreCol)
	if f.showRing(PartSlider) {
		gfx.StrokeCircle(c, tx, fy, float64(m.thumbR)+3.5, 2, t.LabelText)
	}
}

func (f *form) drawHint(c widget.DrawContext, t *widget.Theme) {
	r := f.lay.hint.Add(f.Bounds().Min)
	step := textHeight(hintFont) + f.lay.m.lineGap
	for i, line := range f.lay.hintText {
		row := image.Rect(r.Min.X, r.Min.Y+i*step, r.Max.X, r.Min.Y+(i+1)*step)
		hintFont.Draw(c, line, row.Min.X, baseline(row, hintFont), t.SecondaryText)
	}
}

func (f *form) drawButton(c widget.DrawContext, t *widget.Theme, p Part, text string, primary bool) {
	r := f.rect(p)
	hover := f.hover == p && (f.pressed == PartNone || f.pressed == p)
	down := f.pressed == p && f.hover == p
	radius := 4 * f.lay.m.ks
	white := color.RGBA{255, 255, 255, 255}
	fill, ink := t.BtnBG, t.BtnText
	switch {
	case primary:
		fill, ink = t.Accent, inkOn(t.Accent)
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

func (f *form) AccessInfo() widget.AccessInfo {
	return widget.AccessInfo{Role: widget.RoleGroup, Name: i18n.T("save.options.title"), Bounds: f.Bounds()}
}

func (f *form) states(p Part, checked bool) []string {
	var out []string
	if checked {
		out = append(out, widget.StateChecked)
	}
	if !f.enabled(p) {
		out = append(out, widget.StateDisabled)
	}
	if f.focused && f.stop == p {
		out = append(out, widget.StateFocused)
	}
	return out
}

func (f *form) AccessChildren() []widget.AccessInfo {
	var out []widget.AccessInfo
	if f.lay.hasCheck {
		out = append(out, widget.AccessInfo{Role: widget.RoleCheckBox, Name: i18n.T("save.lossless"), States: f.states(PartCheck, f.lossless), Bounds: f.rect(PartCheck)})
	}
	out = append(out,
		widget.AccessInfo{Role: widget.RoleSlider, Name: i18n.T("save.quality"), Value: f.valueText(), States: f.states(PartSlider, false), Bounds: f.rect(PartSlider)},
		widget.AccessInfo{Role: widget.RoleButton, Name: i18n.T("save.options.save"), States: f.states(PartSave, false), Bounds: f.rect(PartSave)},
		widget.AccessInfo{Role: widget.RoleButton, Name: i18n.T("dialog.cancel"), States: f.states(PartCancel, false), Bounds: f.rect(PartCancel)})
	return out
}
