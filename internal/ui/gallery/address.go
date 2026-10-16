package gallery

import (
	"image"

	"github.com/oops1/headless-gui/v3/widget"

	model "github.com/oops1/PixGo/internal/gallery"
	"github.com/oops1/PixGo/internal/ui/gfx"
)

type addressField struct {
	*widget.TextInput
	onBlur func()
}

func (a *addressField) SetFocused(focused bool) {
	a.TextInput.SetFocused(focused)
	if !focused && a.onBlur != nil {
		a.onBlur()
	}
}

func newAddressField(p *Page) *addressField {
	in := widget.NewTextInput("")
	f := &addressField{TextInput: in}
	f.onBlur = func() {
		if p.editing {
			p.finishEdit(false)
		}
	}
	in.OnEnter = p.commitEdit
	in.OnChange = func(string) {
		if p.bubble != "" {
			p.clearBubble()
			in.SetValidationError("")
		}
	}
	return f
}

func (p *Page) FocusAddress() {
	p.startEdit()
}

func (p *Page) startEdit() {
	if p.editing || p.src() == nil {
		return
	}
	p.ensureLayout()
	p.focus = stopAddress
	t := p.theme()
	in := p.input
	in.Background, in.BorderColor, in.FocusBorder = t.GalleryField, t.GalleryField, t.Accent
	in.TextColor, in.CaretColor, in.PlaceColor = t.Text, t.Text, t.Disabled
	in.SelColor = gfx.Alpha(t.Accent, 110.0/255)
	in.PaddingX = addressTextX - 2
	p.editing = true
	p.placeInput()
	in.SetText(p.snap.loc.Display())
	in.SetValidationError("")
	if p.eng != nil {
		in.SetCaptureManager(p.eng)
		p.eng.SetFocus(in)
	}
	in.OnKeyEvent(widget.KeyEvent{Code: widget.KeyA, Mod: widget.ModCtrl, Pressed: true})
	p.invalidateRect(p.geo.address)
}

func (p *Page) placeInput() {
	if !p.editing {
		return
	}
	a := p.geo.address
	p.input.SetBounds(image.Rect(a.Min.X+2, a.Min.Y+2, p.geo.filter.Min.X, a.Max.Y-2))
}

func (p *Page) endEdit() { p.finishEdit(true) }

func (p *Page) finishEdit(refocus bool) {
	if !p.editing {
		return
	}
	p.editing = false
	p.clearBubble()
	p.input.SetValidationError("")
	if refocus && p.eng != nil {
		p.eng.SetFocus(p)
	}
	p.invalidateRect(p.geo.address)
}

func (p *Page) commitEdit() {
	if !p.editing {
		return
	}
	fn := p.deps.Actions.Address
	if fn == nil {
		p.endEdit()
		return
	}
	err := fn(p.input.GetText())
	if err == nil {
		p.endEdit()
		p.resync()
		p.invalidate()
		return
	}
	msg := model.AddressErrorText(err)
	if msg == "" {
		p.endEdit()
		return
	}
	p.input.SetValidationError(msg)
	p.showBubble(msg)
}

func (p *Page) Editing() bool { return p.editing }

func (p *Page) showBubble(msg string) {
	p.clearBubble()
	p.bubble = msg
	p.invalidateRect(p.bubbleArea())
	p.stopBub = p.after(bubbleDelay, func() {
		p.stopBub = nil
		p.clearBubble()
	})
}

func (p *Page) clearBubble() {
	if p.stopBub != nil {
		p.stopBub()
		p.stopBub = nil
	}
	if p.bubble == "" {
		return
	}
	p.bubble = ""
	p.invalidateRect(p.bubbleArea())
}

func (p *Page) bubbleArea() image.Rectangle {
	a := p.geo.address
	return image.Rect(a.Min.X-2, a.Max.Y, a.Max.X+2, a.Max.Y+14+p.m.bubbleH)
}
