package app

import (
	"slices"
	"testing"

	"github.com/oops1/PixGo/internal/commands"
	"github.com/oops1/PixGo/internal/i18n"
)

func TestSpecsTitlesTranslated(t *testing.T) {
	for _, l := range []string{"ru", "en"} {
		keys := i18n.Keys(l)
		for _, s := range Specs {
			if !slices.Contains(keys, s.Title) {
				t.Errorf("%s: no %q for command %s", l, s.Title, s.ID)
			}
		}
	}
}

func TestSpecsChordsUnique(t *testing.T) {
	seen := map[commands.Chord]commands.ID{}
	ids := map[commands.ID]bool{}
	for _, s := range Specs {
		if ids[s.ID] {
			t.Errorf("duplicate command id %s", s.ID)
		}
		ids[s.ID] = true
		for _, k := range s.Keys {
			if other, dup := seen[k]; dup {
				t.Errorf("chord %s bound to both %s and %s", k, other, s.ID)
			}
			seen[k] = s.ID
		}
	}
}

func TestSpecsCoverArchitectureShortcuts(t *testing.T) {
	want := []struct {
		chord commands.Chord
		id    commands.ID
	}{
		{commands.C(0, commands.KeyLeft), CmdPrev},
		{commands.C(0, commands.KeyRight), CmdNext},
		{commands.C(0, commands.KeyPageUp), CmdPrev},
		{commands.C(0, commands.KeyPageDown), CmdNext},
		{commands.C(0, commands.KeyPlus), CmdZoomIn},
		{commands.C(0, commands.KeyMinus), CmdZoomOut},
		{commands.C(0, '0'), CmdZoomActual},
		{commands.C(0, '1'), CmdZoomFit},
		{commands.C(commands.Ctrl, 'O'), CmdOpen},
		{commands.C(commands.Ctrl|commands.Shift, 'S'), CmdSaveAs},
		{commands.C(commands.Ctrl, 'C'), CmdCopy},
		{commands.C(0, commands.KeyDelete), CmdDelete},
		{commands.C(0, 'R'), CmdRotateRight},
		{commands.C(commands.Shift, 'R'), CmdRotateLeft},
		{commands.C(0, commands.KeyF11), CmdFullscreen},
		{commands.C(commands.Alt, commands.KeyEnter), CmdFullscreen},
		{commands.C(0, commands.KeyF5), CmdSlideshow},
		{commands.C(0, commands.KeySpace), CmdSlideshowToggle},
		{commands.C(0, commands.KeyEscape), CmdEscape},
		{commands.C(commands.Ctrl, commands.KeyComma), CmdSettings},
	}
	got := map[commands.Chord]commands.ID{}
	for _, s := range Specs {
		for _, k := range s.Keys {
			got[k] = s.ID
		}
	}
	for _, w := range want {
		if got[w.chord] != w.id {
			t.Errorf("chord %s: got %q want %q", w.chord, got[w.chord], w.id)
		}
	}
}

func TestZoomShortcutsCoverTypedPlusAndTheNumpad(t *testing.T) {
	reg := commands.NewRegistry()
	for _, s := range Specs {
		reg.Add(commands.Command{ID: s.ID, Keys: s.Keys, Run: func() {}})
	}
	for _, c := range []struct {
		chord commands.Chord
		id    commands.ID
	}{
		{commands.C(commands.Shift, commands.KeyPlus), CmdZoomIn},
		{commands.C(commands.Ctrl|commands.Shift, commands.KeyPlus), CmdZoomIn},
		{commands.C(commands.Ctrl, commands.KeyPlus), CmdZoomIn},
		{commands.C(0, commands.KeyPlus), CmdZoomIn},
		{commands.C(0, commands.KeyMinus), CmdZoomOut},
		{commands.C(commands.Ctrl, commands.KeyMinus), CmdZoomOut},
		{commands.C(0, '0'), CmdZoomActual},
		{commands.C(0, '1'), CmdZoomFit},
		{commands.C(0, '2'), CmdZoomWidth},
	} {
		cmd := reg.ForChord(c.chord)
		if cmd == nil || cmd.ID != c.id {
			t.Errorf("chord %s: got %v want %s", c.chord, cmd, c.id)
		}
	}
}
