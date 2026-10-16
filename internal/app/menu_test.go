package app

import (
	"testing"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/commands"
	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/viewer"
)

func menuIDs(a *App) []commands.ID {
	var out []commands.ID
	for _, e := range a.menuLayout() {
		if !e.sep {
			out = append(out, e.id)
		}
	}
	return out
}

func TestMenuLayoutPlacesTheZoomGroupBeforeTheRotateGroup(t *testing.T) {
	a := viewerWithImage(t)
	ids := menuIDs(a)
	index := func(id commands.ID) int {
		for i, v := range ids {
			if v == id {
				return i
			}
		}
		return -1
	}

	fit, width, actual, rotate := index(CmdZoomFit), index(CmdZoomWidth), index(CmdZoomActual), index(CmdRotateRight)

	if fit < 0 || width != fit+1 || actual != width+1 || rotate != actual+1 {
		t.Fatalf("order = %v", ids)
	}
	layout := a.menuLayout()
	sepBefore, sepAfter := false, false
	for i, e := range layout {
		if e.id == CmdZoomFit {
			sepBefore = i > 0 && layout[i-1].sep
		}
		if e.id == CmdZoomActual {
			sepAfter = i+1 < len(layout) && layout[i+1].sep
		}
	}
	if !sepBefore || !sepAfter {
		t.Fatalf("the zoom group must be set off by separators: before %v after %v", sepBefore, sepAfter)
	}
}

func TestMenuZoomEntriesAreEnabledAndChangeTheMode(t *testing.T) {
	a := viewerWithImage(t)
	for _, lang := range []string{"ru", "en"} {
		i18n.SetLang(lang)
		for _, c := range []struct {
			id   commands.ID
			mode viewer.ZoomMode
		}{
			{CmdZoomWidth, viewer.ZoomFitWidth},
			{CmdZoomActual, viewer.ZoomActual},
			{CmdZoomFit, viewer.ZoomFit},
		} {
			cmd := a.Commands.Get(c.id)
			var item widget.MenuItem
			found := false
			for _, it := range a.menuItems() {
				if it.Text == cmd.Label() && it.Shortcut == cmd.Shortcut() {
					item, found = it, true
				}
			}
			if !found {
				t.Fatalf("%s: no menu entry for %s", lang, c.id)
			}
			if item.Disabled {
				t.Fatalf("%s: %s is disabled with an image shown", lang, c.id)
			}
			item.OnClick()
			if a.Canvas.Mode() != c.mode {
				t.Fatalf("%s: %s left the mode at %v", lang, c.id, a.Canvas.Mode())
			}
		}
	}
	i18n.SetLang("ru")
}

func TestMenuZoomEntriesAreDisabledWithoutAnImage(t *testing.T) {
	a, _ := goldenApp(t, 770, 504, nil)
	a.Root.ForceEnabled = false
	t.Cleanup(a.Shutdown)
	disabled := map[string]bool{}
	for _, id := range []commands.ID{CmdZoomFit, CmdZoomWidth, CmdZoomActual} {
		disabled[a.Commands.Get(id).Label()] = true
	}

	seen := 0
	for _, it := range a.menuItems() {
		if disabled[it.Text] {
			seen++
			if !it.Disabled {
				t.Errorf("%q must be disabled without an image", it.Text)
			}
		}
	}
	if seen != 3 {
		t.Fatalf("zoom entries seen = %d", seen)
	}
}
