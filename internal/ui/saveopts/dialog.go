package saveopts

import (
	"image"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/i18n"
)

const screenMargin = 12

type Values struct {
	Quality  int
	Lossless bool
	StripGPS bool
}

type Option func(*Dialog)

func WithLosslessWatch(fn func(on bool)) Option {
	return func(d *Dialog) { d.watch = fn }
}

func (v Values) normalized(kind Kind) Values {
	v.Quality = clampQuality(v.Quality)
	v.Lossless = v.Lossless && kind.lossless()
	v.StripGPS = v.StripGPS && kind.hasGPS()
	return v
}

type Dialog struct {
	host   widget.ModalShower
	dlg    *widget.Dialog
	form   *form
	kind   Kind
	result func(Values, bool)
	done   bool
	watch  func(on bool)
}

func Show(host widget.ModalShower, kind Kind, initial Values, result func(Values, bool), opts ...Option) *Dialog {
	initial = initial.normalized(kind)
	d := &Dialog{host: host, kind: kind, result: result}
	for _, o := range opts {
		o(d)
	}
	d.dlg = widget.NewDialog(i18n.T("save.options.title"), 0, 0)
	titleH := d.dlg.TitleHeight
	maxW, maxH := 0, 0
	if cs, ok := host.(interface{ CanvasSize() (int, int) }); ok {
		if w, h := cs.CanvasSize(); w > 0 && h > 0 {
			maxW, maxH = w-2*screenMargin, h-2*screenMargin-titleH
		}
	}
	lay := compute(kind, maxW, maxH)
	d.dlg.SetBounds(image.Rect(0, 0, lay.size.X, titleH+lay.size.Y))
	d.form = newForm(kind, lay, initial)
	d.form.SetBounds(image.Rect(0, titleH, lay.size.X, titleH+lay.size.Y))
	d.form.onLossless = d.watch
	d.form.onSave = d.save
	d.form.onCancel = d.cancel
	d.dlg.AddChild(d.form)
	d.dlg.DefaultAction = d.form.activateDefault
	d.dlg.CancelAction = d.dismissed
	d.dlg.OnLanguageChange(func() { d.dlg.SetTitle(i18n.T("save.options.title")) })
	host.ShowModal(d.dlg)
	if d.watch != nil {
		d.watch(d.form.lossless)
	}
	return d
}

func (d *Dialog) finish(ok bool, closeModal bool) {
	if d.done {
		return
	}
	d.done = true
	if closeModal {
		d.host.CloseModal(d.dlg)
	}
	if d.result != nil {
		d.result(d.form.values(), ok)
	}
}

func (d *Dialog) save() { d.finish(true, true) }

func (d *Dialog) cancel() { d.finish(false, true) }

func (d *Dialog) dismissed() { d.finish(false, false) }

func (d *Dialog) Done() bool { return d.done }

func (d *Dialog) Kind() Kind { return d.kind.base() }

func (d *Dialog) Values() Values { return d.form.values() }

func (d *Dialog) Widget() *widget.Dialog { return d.dlg }

func (d *Dialog) Size() image.Point { return d.dlg.Bounds().Size() }

func (d *Dialog) Rect(p Part) image.Rectangle { return d.form.rect(p) }

func (d *Dialog) Focused() Part {
	if !d.form.IsFocused() {
		return PartNone
	}
	return d.form.stop
}

func (d *Dialog) Enabled(p Part) bool { return d.form.enabled(p) }

func (d *Dialog) HintLines() []string { return append([]string(nil), d.form.lay.hintText...) }
