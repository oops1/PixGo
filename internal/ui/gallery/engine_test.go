package gallery

import (
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

func TestEngineDeliversClicksToTheCapturedPage(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)
	x, y := r.tileCenter(5)

	r.eng.SendMouseMove(x, y)
	r.eng.SendMouseButton(x, y, widget.MouseLeft, true)
	r.eng.SendMouseButton(x, y, widget.MouseLeft, false)

	if !r.rec.has("select:5") {
		t.Fatalf("calls = %v", r.rec.calls)
	}
}

func TestEngineCountsDoubleClicks(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)
	x, y := r.tileCenter(6)

	for i := 0; i < 2; i++ {
		r.eng.SendMouseButton(x, y, widget.MouseLeft, true)
		r.eng.SendMouseButton(x, y, widget.MouseLeft, false)
	}

	if !r.rec.has("open:6") {
		t.Fatalf("calls = %v", r.rec.calls)
	}
}

func TestEngineWheelScrollsTheGrid(t *testing.T) {
	r := longRig(t, 60)

	r.eng.SendMouseWheelPixels(300, 200, 0, 3*wheelNotchPixels)

	if r.page.scroll != 102 {
		t.Fatalf("scroll = %d", r.page.scroll)
	}
}

func TestEngineRoutesTypedTextAndEnterToTheAddressField(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)
	r.eng.SendMouseButton(200, 22, widget.MouseLeft, true)
	r.eng.SendMouseButton(200, 22, widget.MouseLeft, false)
	if !r.page.Editing() {
		t.Fatal("clicking the address must start editing")
	}

	for _, ch := range `D:\Other` {
		r.eng.SendKeyEvent(widget.KeyEvent{Code: widget.KeyUnknown, Rune: ch, Pressed: true})
	}
	r.eng.SendKeyEvent(widget.KeyEvent{Code: widget.KeyEnter, Pressed: true})

	if r.rec.addrText != `D:\Other` {
		t.Fatalf("address = %q (selected text must be replaced by typing)", r.rec.addrText)
	}
}

func TestEngineClickOutsideTheAddressEndsTheEdit(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)
	r.eng.SendMouseButton(200, 22, widget.MouseLeft, true)
	r.eng.SendMouseButton(200, 22, widget.MouseLeft, false)
	x, y := r.tileCenter(2)

	r.eng.SendMouseButton(x, y, widget.MouseLeft, true)
	r.eng.SendMouseButton(x, y, widget.MouseLeft, false)

	if r.page.Editing() || !r.rec.has("select:2") {
		t.Fatalf("editing = %v, calls = %v", r.page.Editing(), r.rec.calls)
	}
}

func TestEngineClickInsideTheEditorKeepsTheCaretThere(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)
	r.eng.SendMouseButton(200, 22, widget.MouseLeft, true)
	r.eng.SendMouseButton(200, 22, widget.MouseLeft, false)

	r.eng.SendMouseButton(150, 22, widget.MouseLeft, true)
	r.eng.SendMouseButton(150, 22, widget.MouseLeft, false)

	if !r.page.Editing() {
		t.Fatal("a click inside the field must not cancel the edit")
	}
}

func TestEngineCursorOverTheEditorIsATextCursor(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)
	r.page.FocusAddress()

	if got := r.page.input.Cursor(150, 22); got != widget.CursorIBeam {
		t.Fatalf("cursor = %v", got)
	}
}
