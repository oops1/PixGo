package confirm

import (
	"image"
	"image/color"
	"math"
	"strings"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/fonts"
	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/ui/gfx"
	"github.com/oops1/PixGo/internal/ui/icons"
)

type Part uint8

const (
	PartNone Part = iota
	PartCheck
	PartYes
	PartCancel
)

const (
	screenMargin = 12
	ringGap      = 2
	maxLines     = 6
)

var bodyFont = gfx.UI(7.5)

type Config struct {
	Title   string
	Message string
	Yes     string
	Check   string
}

type Result struct {
	Yes     bool
	Checked bool
}

type layout struct {
	size    image.Point
	message image.Rectangle
	check   image.Rectangle
	yes     image.Rectangle
	cancel  image.Rectangle
	lines   []string
	m       metrics
}

type metrics struct {
	k, ks                            float64
	padX, padTop, padBottom          int
	boxSize, boxGap, btnGap, btnMinW int
	btnMinH, btnPad, lineGap         int
}

func newMetrics(k float64) metrics {
	ks := 1 + (k-1)/2
	s := func(v float64) int { return int(math.Round(v * k)) }
	return metrics{
		k: k, ks: ks,
		padX: s(20), padTop: s(18), padBottom: s(14),
		boxSize: int(math.Round(16 * ks)), boxGap: s(8), btnGap: s(8), btnMinW: s(88), btnMinH: s(30),
		btnPad: s(18), lineGap: s(3),
	}
}

func textHeight(f gfx.Font) int {
	m := fonts.MetricsOf(f.Name, f.Size(), 96)
	return m.Ascent + m.Descent
}

func wrap(f gfx.Font, text string, width int) []string {
	var lines []string
	for _, para := range strings.Split(text, "\n") {
		cur := ""
		for _, word := range i18n.Words(para) {
			switch {
			case cur == "":
				cur = word
			case f.Measure(nil, cur+" "+word) <= width:
				cur += " " + word
			default:
				lines = append(lines, cur)
				cur = word
			}
		}
		lines = append(lines, cur)
	}
	if len(lines) > maxLines {
		lines = append(lines[:maxLines-1], f.Fit(nil, strings.Join(lines[maxLines-1:], " "), width))
	}
	return lines
}

func build(cfg Config, maxW int) layout {
	m := newMetrics(fonts.TextScale())
	lineH := textHeight(bodyFont)
	yesText, cancelText := cfg.Yes, i18n.T("dialog.cancel")
	yesW := max(m.btnMinW, bodyFont.Measure(nil, yesText)+2*m.btnPad)
	cancelW := max(m.btnMinW, bodyFont.Measure(nil, cancelText)+2*m.btnPad)
	cw := max(int(math.Round(340*m.k)), yesW+m.btnGap+cancelW)
	w := cw + 2*m.padX
	if maxW > 0 && w > maxW {
		w = max(maxW, 2*m.padX+int(math.Round(200*m.k)))
		cw = w - 2*m.padX
	}
	l := layout{m: m}
	l.lines = wrap(bodyFont, cfg.Message, cw)
	y := m.padTop
	h := len(l.lines) * (lineH + m.lineGap)
	l.message = image.Rect(m.padX, y, m.padX+cw, y+h)
	y += h + int(math.Round(14*m.k))
	if cfg.Check != "" {
		rowH := max(m.boxSize, lineH) + int(math.Round(8*m.k))
		l.check = image.Rect(m.padX, y, m.padX+min(cw, m.boxSize+m.boxGap+bodyFont.Measure(nil, cfg.Check)+int(math.Round(8*m.k))), y+rowH)
		y += rowH + int(math.Round(14*m.k))
	}
	btnH := max(m.btnMinH, lineH+int(math.Round(14*m.k)))
	right := m.padX + cw
	if yesW+m.btnGap+cancelW > cw {
		each := (cw - m.btnGap) / 2
		yesW, cancelW = each, each
	}
	l.cancel = image.Rect(right-cancelW, y, right, y+btnH)
	l.yes = image.Rect(l.cancel.Min.X-m.btnGap-yesW, y, l.cancel.Min.X-m.btnGap, y+btnH)
	y += btnH + m.padBottom
	l.size = image.Pt(w, y)
	return l
}

type form struct {
	widget.Base
	cfg     Config
	lay     layout
	checked bool
	stop    Part
	hover   Part
	pressed Part
	focused bool
	keyNav  bool

	capMgr    widget.CaptureManager
	autofocus bool

	onYes    func()
	onCancel func()
}

func (f *form) rect(p Part) image.Rectangle {
	var r image.Rectangle
	switch p {
	case PartCheck:
		r = f.lay.check
	case PartYes:
		r = f.lay.yes
	case PartCancel:
		r = f.lay.cancel
	}
	if r.Empty() {
		return r
	}
	return r.Add(f.Bounds().Min)
}

func (f *form) enabled(p Part) bool {
	if p == PartCheck {
		return f.cfg.Check != ""
	}
	return p != PartNone
}

func (f *form) stops() []Part {
	var out []Part
	for _, p := range []Part{PartCheck, PartYes, PartCancel} {
		if f.enabled(p) {
			out = append(out, p)
		}
	}
	return out
}

func (f *form) moveStop(dir int) {
	list := f.stops()
	at := 0
	for i, p := range list {
		if p == f.stop {
			at = i
		}
	}
	f.stop = list[(at+dir+len(list))%len(list)]
	f.Invalidate()
}

func (f *form) setChecked(on bool) {
	if f.cfg.Check == "" || on == f.checked {
		return
	}
	f.checked = on
	f.Invalidate()
}

func (f *form) activate(p Part) {
	switch p {
	case PartCheck:
		f.setChecked(!f.checked)
	case PartYes:
		if f.onYes != nil {
			f.onYes()
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
	f.activate(PartYes)
}

func (f *form) SetFocused(v bool) {
	if f.focused != v {
		f.focused = v
		f.Invalidate()
	}
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

func (f *form) hitAt(x, y int) Part {
	pt := image.Pt(x, y)
	for _, p := range []Part{PartYes, PartCancel, PartCheck} {
		if f.enabled(p) && pt.In(f.rect(p)) {
			return p
		}
	}
	return PartNone
}

func (f *form) OnMouseButton(e widget.MouseEvent) bool {
	if e.Button != widget.MouseLeft {
		return false
	}
	if e.Pressed {
		f.keyNav = false
		f.pressed = f.hitAt(e.X, e.Y)
		if f.pressed != PartNone {
			f.stop = f.pressed
		}
		f.Invalidate()
		return true
	}
	over, pressed := f.hitAt(e.X, e.Y), f.pressed
	f.pressed = PartNone
	if f.capMgr != nil {
		f.capMgr.ReleaseCapture()
	}
	f.Invalidate()
	if pressed != PartNone && pressed == over {
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
		f.activate(f.stop)
	case widget.KeyEnter:
		f.activateDefault()
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

func baseline(r image.Rectangle) int {
	return r.Min.Y + (r.Dy()-textHeight(bodyFont))/2 + bodyFont.Ascent()
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
	step := textHeight(bodyFont) + f.lay.m.lineGap
	msg := f.lay.message.Add(b.Min)
	for i, line := range f.lay.lines {
		row := image.Rect(msg.Min.X, msg.Min.Y+i*step, msg.Max.X, msg.Min.Y+(i+1)*step)
		bodyFont.Draw(c, line, row.Min.X, baseline(row), t.LabelText)
	}
	if f.cfg.Check != "" {
		f.drawCheck(c, t)
	}
	f.drawButton(c, t, PartYes, f.cfg.Yes, true)
	f.drawButton(c, t, PartCancel, i18n.T("dialog.cancel"), false)
}

func (f *form) drawCheck(c widget.DrawContext, t *widget.Theme) {
	m := f.lay.m
	row := f.rect(PartCheck)
	top := row.Min.Y + (row.Dy()-m.boxSize)/2
	box := image.Rect(row.Min.X, top, row.Min.X+m.boxSize, top+m.boxSize)
	radius := 3 * m.ks
	hover := f.hover == PartCheck || f.pressed == PartCheck
	if f.checked {
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
	bodyFont.Draw(c, f.cfg.Check, box.Max.X+m.boxGap, baseline(row), t.LabelText)
	if f.showRing(PartCheck) {
		f.ring(c, t, box, radius)
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
	if bodyFont.Measure(c, text) > r.Dx()-2 {
		text = bodyFont.Fit(c, text, r.Dx()-2)
	}
	w := bodyFont.Measure(c, text)
	bodyFont.Draw(c, text, r.Min.X+(r.Dx()-w)/2, baseline(r), ink)
	if f.showRing(p) {
		f.ring(c, t, r, radius)
	}
}

var partIDs = map[Part]string{PartCheck: "check", PartYes: "yes", PartCancel: "cancel"}

func partOfID(id string) Part {
	for p, s := range partIDs {
		if s == id {
			return p
		}
	}
	return PartNone
}

func (f *form) AccessInfo() widget.AccessInfo {
	return widget.AccessInfo{Role: widget.RoleGroup, Name: f.cfg.Title, Description: f.cfg.Message, Bounds: f.Bounds()}
}

func (f *form) states(p Part, checked bool) []string {
	var out []string
	if checked {
		out = append(out, widget.StateChecked)
	}
	if f.focused && f.stop == p {
		out = append(out, widget.StateFocused)
	}
	return out
}

func (f *form) AccessChildren() []widget.AccessInfo {
	var out []widget.AccessInfo
	if f.cfg.Check != "" {
		out = append(out, widget.AccessInfo{ID: partIDs[PartCheck], Role: widget.RoleCheckBox, Name: f.cfg.Check, States: f.states(PartCheck, f.checked), Bounds: f.rect(PartCheck)})
	}
	return append(out,
		widget.AccessInfo{ID: partIDs[PartYes], Role: widget.RoleButton, Name: f.cfg.Yes, States: f.states(PartYes, false), Bounds: f.rect(PartYes)},
		widget.AccessInfo{ID: partIDs[PartCancel], Role: widget.RoleButton, Name: i18n.T("dialog.cancel"), States: f.states(PartCancel, false), Bounds: f.rect(PartCancel)})
}

func (f *form) AccessAction(id string, action widget.AccessActionKind) bool {
	p := partOfID(id)
	if p == PartNone || !f.enabled(p) {
		return false
	}
	switch action {
	case widget.AccessFocus:
		f.stop, f.keyNav = p, true
		f.Invalidate()
		return true
	case widget.AccessToggle:
		if p != PartCheck {
			return false
		}
	case widget.AccessInvoke:
	default:
		return false
	}
	f.stop = p
	f.activate(p)
	return true
}

type Dialog struct {
	host   widget.ModalShower
	dlg    *widget.Dialog
	form   *form
	result func(Result)
	done   bool
}

func Show(host widget.ModalShower, cfg Config, result func(Result)) *Dialog {
	d := &Dialog{host: host, result: result}
	d.dlg = widget.NewDialog(cfg.Title, 0, 0)
	titleH := d.dlg.TitleHeight
	maxW := 0
	if cs, ok := host.(interface{ CanvasSize() (int, int) }); ok {
		if w, h := cs.CanvasSize(); w > 0 && h > 0 {
			maxW = w - 2*screenMargin
		}
	}
	lay := build(cfg, maxW)
	d.dlg.SetBounds(image.Rect(0, 0, lay.size.X, titleH+lay.size.Y))
	d.form = &form{cfg: cfg, lay: lay, stop: PartYes}
	d.form.SetBounds(image.Rect(0, titleH, lay.size.X, titleH+lay.size.Y))
	d.form.onYes = func() { d.finish(true, true) }
	d.form.onCancel = func() { d.finish(false, true) }
	d.dlg.AddChild(d.form)
	d.dlg.DefaultAction = d.form.activateDefault
	d.dlg.CancelAction = func() { d.finish(false, false) }
	host.ShowModal(d.dlg)
	return d
}

func (d *Dialog) finish(yes, closeModal bool) {
	if d.done {
		return
	}
	d.done = true
	if closeModal {
		d.host.CloseModal(d.dlg)
	}
	if d.result != nil {
		d.result(Result{Yes: yes, Checked: d.form.checked})
	}
}

func (d *Dialog) Done() bool { return d.done }

func (d *Dialog) Widget() *widget.Dialog { return d.dlg }

func (d *Dialog) Rect(p Part) image.Rectangle { return d.form.rect(p) }

func (d *Dialog) Checked() bool { return d.form.checked }

func (d *Dialog) Focused() Part {
	if !d.form.focused {
		return PartNone
	}
	return d.form.stop
}
