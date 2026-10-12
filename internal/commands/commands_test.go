package commands

import (
	"testing"

	"github.com/oops1/PixGo/internal/i18n"
)

func TestChordString(t *testing.T) {
	i18n.SetLang("en")
	defer i18n.SetLang("ru")
	for _, c := range []struct {
		ch   Chord
		want string
	}{
		{C(Ctrl|Shift, 'S'), "Ctrl+Shift+S"},
		{C(Alt, KeyLeft), "Alt+Left"},
		{C(0, KeyPageDown), "PgDn"},
		{C(0, KeyF11), "F11"},
		{C(0, KeyF5), "F5"},
		{C(Ctrl, KeyComma), "Ctrl+,"},
		{C(0, KeyPlus), "Plus"},
		{C(0, KeyNumSub), "Minus"},
		{C(0, KeyDelete), "Del"},
		{C(Alt, KeyEnter), "Alt+Enter"},
		{C(0, '0'), "0"},
	} {
		if got := c.ch.String(); got != c.want {
			t.Errorf("%+v: got %q, want %q", c.ch, got, c.want)
		}
	}
}

func TestRegistry(t *testing.T) {
	r := NewRegistry()
	ran, enabled := 0, true
	r.Add(Command{
		ID: "view.zoomIn", Title: "cmd.zoomIn", Keys: []Chord{C(0, KeyPlus), C(0, KeyNumAdd)},
		Enabled: func() bool { return enabled }, Run: func() { ran++ },
	})
	r.Add(Command{ID: "view.noop", Title: "cmd.more"})
	if cmd := r.ForChord(C(0, KeyNumAdd)); cmd == nil || cmd.ID != "view.zoomIn" {
		t.Fatal("chord lookup")
	}
	if r.ForChord(C(Ctrl, KeyPlus)) != nil {
		t.Fatal("unbound chord found")
	}
	if !r.Run("view.zoomIn") || ran != 1 {
		t.Fatal("run")
	}
	enabled = false
	if r.Run("view.zoomIn") || ran != 1 {
		t.Fatal("disabled command ran")
	}
	if r.Run("view.noop") || r.Run("missing") {
		t.Fatal("command without Run or missing ran")
	}
	if ids := r.IDs(); len(ids) != 2 || ids[0] != "view.noop" {
		t.Fatalf("ids %v", ids)
	}
	i18n.SetLang("ru")
	if got := r.Get("view.zoomIn").Label(); got != "Увеличить" {
		t.Fatalf("label %q", got)
	}
	if got := r.Get("view.zoomIn").Shortcut(); got != "Плюс" {
		t.Fatalf("shortcut %q", got)
	}
}

func TestItems(t *testing.T) {
	if !Sep.Separator || Cmd("x").Cmd != "x" {
		t.Fatal("items")
	}
}
