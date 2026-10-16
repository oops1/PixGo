package window

import (
	"unicode"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/commands"
)

type KeyHandler interface {
	HandleKey(e widget.KeyEvent) bool
}

type KeyRouter struct {
	*engine.Engine
	Target KeyHandler
}

type PointerObserver interface {
	PointerPressed()
}

func (k *KeyRouter) SendKeyEvent(e widget.KeyEvent) {
	if k.Target != nil && k.Target.HandleKey(e) {
		return
	}
	k.Engine.SendKeyEvent(e)
}

func (k *KeyRouter) SendMouseButton(x, y int, btn widget.MouseButton, pressed bool) {
	if o, ok := k.Target.(PointerObserver); ok && pressed {
		o.PointerPressed()
	}
	k.Engine.SendMouseButton(x, y, btn, pressed)
}

func ChordOf(e widget.KeyEvent) (commands.Chord, bool) {
	var mod commands.Mod
	if e.Mod&widget.ModCtrl != 0 {
		mod |= commands.Ctrl
	}
	if e.Mod&widget.ModShift != 0 {
		mod |= commands.Shift
	}
	if e.Mod&widget.ModAlt != 0 {
		mod |= commands.Alt
	}
	if e.Code != widget.KeyUnknown {
		k := commands.Key(e.Code)
		switch k {
		case commands.KeyNumAdd:
			k = commands.KeyPlus
		case commands.KeyNumSub:
			k = commands.KeyMinus
		case commands.Key(widget.KeyNumpad0):
			k = '0'
		}
		return commands.Chord{Key: k, Mod: mod}, true
	}
	if mod&(commands.Ctrl|commands.Alt) == 0 || mod&commands.Ctrl != 0 && mod&commands.Alt != 0 {
		return commands.Chord{}, false
	}
	k, ok := keyOfRune(e.Rune)
	if !ok {
		return commands.Chord{}, false
	}
	if unicode.IsUpper(e.Rune) {
		mod |= commands.Shift
	}
	return commands.Chord{Key: k, Mod: mod}, true
}

var cyrillicKeys = map[rune]rune{
	'й': 'Q', 'ц': 'W', 'у': 'E', 'к': 'R', 'е': 'T', 'н': 'Y', 'г': 'U', 'ш': 'I', 'щ': 'O', 'з': 'P',
	'ф': 'A', 'ы': 'S', 'в': 'D', 'а': 'F', 'п': 'G', 'р': 'H', 'о': 'J', 'л': 'K', 'д': 'L',
	'я': 'Z', 'ч': 'X', 'с': 'C', 'м': 'V', 'и': 'B', 'т': 'N', 'ь': 'M',
	'б': ',', 'ю': '.',
}

func keyOfRune(r rune) (commands.Key, bool) {
	lr := unicode.ToLower(r)
	switch {
	case lr >= 'a' && lr <= 'z':
		return commands.Key(unicode.ToUpper(lr)), true
	case r >= '0' && r <= '9':
		return commands.Key(r), true
	case r == '+' || r == '=':
		return commands.KeyPlus, true
	case r == '-' || r == '_':
		return commands.KeyMinus, true
	case r == ',' || r == '<':
		return commands.KeyComma, true
	}
	if k, ok := cyrillicKeys[lr]; ok {
		if k == ',' {
			return commands.KeyComma, true
		}
		if k == '.' {
			return 0, false
		}
		return commands.Key(k), true
	}
	return 0, false
}

const (
	HitClient = iota
	HitCaption
	HitMinButton
	HitMaxButton
	HitCloseButton
)

func HitAreaOf(k ZoneKind) int {
	switch k {
	case ZoneDrag:
		return HitCaption
	case ZoneMin:
		return HitMinButton
	case ZoneMax:
		return HitMaxButton
	case ZoneClose:
		return HitCloseButton
	}
	return HitClient
}
