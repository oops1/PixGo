package window

import (
	"testing"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/commands"
)

func TestChordOfCodes(t *testing.T) {
	cases := []struct {
		e    widget.KeyEvent
		want commands.Chord
	}{
		{widget.KeyEvent{Code: widget.KeyLeft, Pressed: true}, commands.C(0, commands.KeyLeft)},
		{widget.KeyEvent{Code: widget.KeyAdd, Pressed: true}, commands.C(0, commands.KeyPlus)},
		{widget.KeyEvent{Code: widget.KeySubtract, Pressed: true}, commands.C(0, commands.KeyMinus)},
		{widget.KeyEvent{Code: widget.KeyOemPlus, Pressed: true}, commands.C(0, commands.KeyPlus)},
		{widget.KeyEvent{Code: widget.KeyO, Mod: widget.ModCtrl, Pressed: true}, commands.C(commands.Ctrl, 'O')},
		{widget.KeyEvent{Code: widget.KeyS, Mod: widget.ModCtrl | widget.ModShift, Pressed: true}, commands.C(commands.Ctrl|commands.Shift, 'S')},
		{widget.KeyEvent{Code: widget.KeyEnter, Mod: widget.ModAlt, Pressed: true}, commands.C(commands.Alt, commands.KeyEnter)},
		{widget.KeyEvent{Code: widget.KeyOemComma, Mod: widget.ModCtrl, Pressed: true}, commands.C(commands.Ctrl, commands.KeyComma)},
		{widget.KeyEvent{Code: widget.KeyNumpad0, Pressed: true}, commands.C(0, '0')},
		{widget.KeyEvent{Code: widget.KeyNumpad1, Pressed: true}, commands.C(0, '1')},
		{widget.KeyEvent{Code: widget.KeyNumpad2, Pressed: true}, commands.C(0, '2')},
		{widget.KeyEvent{Code: widget.KeyNumpad1, Mod: widget.ModCtrl, Pressed: true}, commands.C(commands.Ctrl, '1')},
		{widget.KeyEvent{Code: widget.KeyOemPlus, Mod: widget.ModShift, Pressed: true}, commands.C(commands.Shift, commands.KeyPlus)},
		{widget.KeyEvent{Code: widget.KeyOemPlus, Mod: widget.ModCtrl | widget.ModShift, Pressed: true}, commands.C(commands.Ctrl|commands.Shift, commands.KeyPlus)},
		{widget.KeyEvent{Code: widget.KeyAdd, Mod: widget.ModCtrl, Pressed: true}, commands.C(commands.Ctrl, commands.KeyPlus)},
		{widget.KeyEvent{Code: widget.KeySubtract, Mod: widget.ModCtrl, Pressed: true}, commands.C(commands.Ctrl, commands.KeyMinus)},
	}
	for _, c := range cases {
		got, ok := ChordOf(c.e)
		if !ok || got != c.want {
			t.Errorf("%+v: got %v ok=%v want %v", c.e, got, ok, c.want)
		}
	}
}

func TestChordOfRuneUnderCyrillicLayout(t *testing.T) {
	cases := []struct {
		r    rune
		mod  widget.KeyMod
		want commands.Chord
	}{
		{'щ', widget.ModCtrl, commands.C(commands.Ctrl, 'O')},
		{'ы', widget.ModCtrl, commands.C(commands.Ctrl, 'S')},
		{'Ы', widget.ModCtrl, commands.C(commands.Ctrl|commands.Shift, 'S')},
		{'к', widget.ModCtrl, commands.C(commands.Ctrl, 'R')},
		{'б', widget.ModCtrl, commands.C(commands.Ctrl, commands.KeyComma)},
	}
	for _, c := range cases {
		got, ok := ChordOf(widget.KeyEvent{Rune: c.r, Mod: c.mod, Pressed: true})
		if !ok || got != c.want {
			t.Errorf("%q: got %v ok=%v want %v", c.r, got, ok, c.want)
		}
	}
}

func TestChordOfIgnoresTextAndAltGr(t *testing.T) {
	for _, e := range []widget.KeyEvent{
		{Rune: 'a', Pressed: true},
		{Rune: 'ф', Pressed: true},
		{Rune: '@', Mod: widget.ModCtrl | widget.ModAlt, Pressed: true},
	} {
		if ch, ok := ChordOf(e); ok {
			t.Errorf("%+v produced chord %v", e, ch)
		}
	}
}

func TestHitAreas(t *testing.T) {
	want := map[ZoneKind]int{ZoneDrag: HitCaption, ZoneMin: HitMinButton, ZoneMax: HitMaxButton, ZoneClose: HitCloseButton, ZoneInfo: HitClient, ZonePrev: HitClient}
	for k, w := range want {
		if got := HitAreaOf(k); got != w {
			t.Errorf("zone %d: got %d want %d", k, got, w)
		}
	}
}
