package printdlg

import (
	"image"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/i18n"
)

const screenMargin = 12

type SystemHook func(v Values, apply func(printer string, copies int))

type Option func(*Dialog)

func WithSystemDialog(fn SystemHook) Option {
	return func(d *Dialog) { d.system = fn }
}

type Dialog struct {
	host   widget.ModalShower
	dlg    *widget.Dialog
	form   *form
	result func(Values, bool)
	done   bool
	system SystemHook
}

func titleKey(export bool) string {
	if export {
		return "print.titleExport"
	}
	return "print.title"
}

func Show(host widget.ModalShower, env Env, initial Values, result func(Values, bool), opts ...Option) *Dialog {
	d := &Dialog{host: host, result: result}
	for _, o := range opts {
		o(d)
	}
	if d.system == nil {
		env.System = false
	}
	m := NewModel(env, initial)
	d.dlg = widget.NewDialog(i18n.T(titleKey(env.Export)), 0, 0)
	titleH := d.dlg.TitleHeight
	maxW, maxH := 0, 0
	if cs, ok := host.(interface{ CanvasSize() (int, int) }); ok {
		if w, h := cs.CanvasSize(); w > 0 && h > 0 {
			maxW, maxH = w-2*screenMargin, h-2*screenMargin-titleH
		}
	}
	lay := compute(maxW, maxH, env.Export, env.System)
	d.dlg.SetBounds(image.Rect(0, 0, lay.size.X, titleH+lay.size.Y))
	d.form = newForm(lay, m)
	d.form.SetBounds(image.Rect(0, titleH, lay.size.X, titleH+lay.size.Y))
	d.form.onPrint = d.accept
	d.form.onCancel = d.cancel
	d.form.onSystem = d.openSystem
	d.dlg.AddChild(d.form)
	d.dlg.DefaultAction = d.form.activateDefault
	d.dlg.CancelAction = d.dismissed
	d.dlg.OnClosing = func() bool { return !d.form.closeList() }
	d.dlg.OnLanguageChange(func() { d.dlg.SetTitle(i18n.T(titleKey(env.Export))) })
	host.ShowModal(d.dlg)
	return d
}

func (d *Dialog) finish(ok, closeModal bool) {
	if d.done {
		return
	}
	d.done = true
	if closeModal {
		d.host.CloseModal(d.dlg)
	}
	if d.result != nil {
		d.result(d.form.model.Values(), ok)
	}
}

func (d *Dialog) accept() { d.finish(true, true) }

func (d *Dialog) cancel() { d.finish(false, true) }

func (d *Dialog) dismissed() { d.finish(false, false) }

func (d *Dialog) openSystem() {
	if d.system == nil || d.done {
		return
	}
	d.system(d.form.model.Values(), func(printer string, copies int) {
		if d.done {
			return
		}
		if printer != "" {
			d.form.model.SetPrinter(printer)
		}
		if copies > 0 {
			d.form.model.SetCopies(copies)
		}
		d.form.resetFields()
		d.form.Invalidate()
	})
}

func (d *Dialog) Done() bool { return d.done }

func (d *Dialog) Model() *Model { return d.form.model }

func (d *Dialog) Values() Values { return d.form.model.Values() }

func (d *Dialog) Widget() *widget.Dialog { return d.dlg }

func (d *Dialog) Size() image.Point { return d.dlg.Bounds().Size() }

func (d *Dialog) Rect(p Part) image.Rectangle { return d.form.rect(p) }

func (d *Dialog) Preview() image.Rectangle { return d.form.previewRect() }

func (d *Dialog) Focused() Part {
	if !d.form.IsFocused() {
		return PartNone
	}
	return d.form.stop
}

func (d *Dialog) Enabled(p Part) bool { return d.form.enabled(p) }

func (d *Dialog) ListOpen() bool { return d.form.list != nil }

func (d *Dialog) InfoLines() []string { return d.form.InfoLines() }

func (d *Dialog) PrintLabel() string { return d.form.PrintLabel() }
