package resize

import (
	"image"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/i18n"
)

const screenMargin = 12

type Dialog struct {
	host   widget.ModalShower
	dlg    *widget.Dialog
	form   *form
	result func(image.Point, bool)
	done   bool
	size   image.Point
}

func Show(host widget.ModalShower, orig, initial image.Point, lim Limits, result func(image.Point, bool)) *Dialog {
	d := &Dialog{host: host, result: result}
	m := NewModel(orig, initial, lim)
	d.dlg = widget.NewDialog(i18n.T("resize.title"), 0, 0)
	titleH := d.dlg.TitleHeight
	maxW, maxH := 0, 0
	if cs, ok := host.(interface{ CanvasSize() (int, int) }); ok {
		if w, h := cs.CanvasSize(); w > 0 && h > 0 {
			maxW, maxH = w-2*screenMargin, h-2*screenMargin-titleH
		}
	}
	lay := compute(maxW, maxH, lim)
	d.dlg.SetBounds(image.Rect(0, 0, lay.size.X, titleH+lay.size.Y))
	d.form = newForm(lay, m)
	d.form.SetBounds(image.Rect(0, titleH, lay.size.X, titleH+lay.size.Y))
	d.form.onApply = d.apply
	d.form.onCancel = d.cancel
	d.dlg.AddChild(d.form)
	d.dlg.DefaultAction = d.form.activateDefault
	d.dlg.CancelAction = d.dismissed
	d.dlg.OnLanguageChange(func() { d.dlg.SetTitle(i18n.T("resize.title")) })
	host.ShowModal(d.dlg)
	return d
}

func (d *Dialog) finish(size image.Point, ok, closeModal bool) {
	if d.done {
		return
	}
	d.done = true
	if closeModal {
		d.host.CloseModal(d.dlg)
	}
	if d.result != nil {
		d.result(size, ok)
	}
}

func (d *Dialog) apply(size image.Point) { d.finish(size, true, true) }

func (d *Dialog) cancel() { d.finish(image.Point{}, false, true) }

func (d *Dialog) dismissed() { d.finish(image.Point{}, false, false) }

func (d *Dialog) Done() bool { return d.done }

func (d *Dialog) Model() *Model { return d.form.model }

func (d *Dialog) Result() (image.Point, Status) { return d.form.model.Result() }

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

func (d *Dialog) ResultText() string { return d.form.ResultText() }

func (d *Dialog) HintLines() []string { return append([]string(nil), d.form.lay.hintText...) }
