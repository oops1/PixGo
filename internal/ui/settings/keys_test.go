package settings

import (
	"slices"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"

	data "github.com/oops1/PixGo/internal/settings"
)

func TestTabVisitsEveryControlOfTheSectionInOrder(t *testing.T) {
	r := newRig(t, mockW, mockH)
	want := []string{
		focusBack, focusNav, "theme:system", "language", "rememberWindow", "restoreLastImage",
		"confirmDelete", "autoHidePanels", "background:checker", "singleWindow", "updatesCheck",
	}
	var got []string
	for range want {
		if !r.key(widget.KeyTab, 0) {
			t.Fatal("Tab must be consumed")
		}
		got = append(got, r.page.focus)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("Tab order = %v\nwant %v", got, want)
	}
	r.key(widget.KeyTab, 0)
	if r.page.focus != focusBack {
		t.Fatalf("Tab after the last control must wrap to the back button, focus = %q", r.page.focus)
	}
}

func TestShiftTabWalksBackwardsAndStartsFromTheEnd(t *testing.T) {
	r := newRig(t, mockW, mockH)
	r.key(widget.KeyTab, widget.ModShift)
	if r.page.focus != "updatesCheck" {
		t.Fatalf("Shift+Tab from nothing = %q, want the last control", r.page.focus)
	}
	r.key(widget.KeyTab, widget.ModShift)
	if r.page.focus != "singleWindow" {
		t.Fatalf("focus = %q", r.page.focus)
	}
	r.focusOn(focusBack)
	r.key(widget.KeyTab, widget.ModShift)
	if r.page.focus != "updatesCheck" {
		t.Fatalf("Shift+Tab from the first stop must wrap, focus = %q", r.page.focus)
	}
}

func TestTabInAGroupVisitsOnlyTheSelectedMember(t *testing.T) {
	r := newRig(t, mockW, mockH)
	r.src.cfg.Theme = data.ThemeDark
	r.page.Refresh()
	r.focusOn(focusNav)
	r.key(widget.KeyTab, 0)
	if r.page.focus != "theme:dark" {
		t.Fatalf("focus = %q, the selected card is the group's tab stop", r.page.focus)
	}
}

func TestSpaceAndEnterToggleTheFocusedCheckbox(t *testing.T) {
	for _, code := range []widget.KeyCode{widget.KeySpace, widget.KeyEnter} {
		r := newRig(t, mockW, mockH)
		r.focusOn("rememberWindow")
		if !r.key(code, 0) || r.src.cfg.RememberWindow {
			t.Fatalf("key %d must toggle the checkbox", code)
		}
		r.page.HandleKey(widget.KeyEvent{Code: code, Pressed: true, Repeat: true})
		if r.src.cfg.RememberWindow {
			t.Fatal("auto-repeat must not toggle again")
		}
	}
}

func TestArrowsMoveWithinAGroupAndSelect(t *testing.T) {
	r := newRig(t, mockW, mockH)
	r.focusOn("theme:system")
	steps := []struct {
		code  widget.KeyCode
		focus string
		theme string
	}{
		{widget.KeyRight, "theme:light", data.ThemeLight},
		{widget.KeyDown, "theme:dark", data.ThemeDark},
		{widget.KeyRight, "theme:system", data.ThemeSystem},
		{widget.KeyLeft, "theme:dark", data.ThemeDark},
		{widget.KeyUp, "theme:light", data.ThemeLight},
	}
	for _, s := range steps {
		if !r.key(s.code, 0) {
			t.Fatalf("arrow %d must be consumed", s.code)
		}
		if r.page.focus != s.focus || r.src.cfg.Theme != s.theme {
			t.Fatalf("after %d: focus %q theme %q, want %q %q", s.code, r.page.focus, r.src.cfg.Theme, s.focus, s.theme)
		}
	}
}

func TestSpaceSelectsAFocusedRadio(t *testing.T) {
	r := newRig(t, mockW, mockH)
	r.page.SetSection(SectionView)
	r.focusOn("wheel:zoom")
	r.key(widget.KeySpace, 0)
	if r.src.cfg.Wheel != data.WheelZoom {
		t.Fatalf("wheel = %q", r.src.cfg.Wheel)
	}
}

func TestDropdownKeys(t *testing.T) {
	r := newRig(t, mockW, mockH)
	r.page.SetSection(SectionView)
	r.focusOn("sort")
	r.key(widget.KeyDown, 0)
	if r.src.cfg.Sort != data.SortDate {
		t.Fatalf("Down must pick the next choice, sort = %q", r.src.cfg.Sort)
	}
	r.key(widget.KeyDown, 0)
	r.key(widget.KeyDown, 0)
	if r.src.cfg.Sort != data.SortSize {
		t.Fatalf("Down at the end keeps the last choice, sort = %q", r.src.cfg.Sort)
	}
	r.key(widget.KeyUp, 0)
	if r.src.cfg.Sort != data.SortDate {
		t.Fatalf("Up must pick the previous choice, sort = %q", r.src.cfg.Sort)
	}
	r.rec.reset()
	r.key(widget.KeyEnter, 0)
	if !r.rec.has("menu") {
		t.Fatal("Enter must open the menu")
	}
	r.rec.reset()
	r.key(widget.KeyDown, widget.ModAlt)
	if !r.rec.has("menu") {
		t.Fatal("Alt+Down must open the menu")
	}
	r.rec.reset()
	r.key(widget.KeySpace, 0)
	if !r.rec.has("menu") {
		t.Fatal("Space must open the menu")
	}
}

func TestSliderKeys(t *testing.T) {
	r := newRig(t, mockW, mockH)
	r.page.SetSection(SectionSlideshow)
	r.focusOn("slideshow.interval")
	cases := []struct {
		code widget.KeyCode
		want int
	}{
		{widget.KeyRight, 6},
		{widget.KeyUp, 7},
		{widget.KeyLeft, 6},
		{widget.KeyDown, 5},
		{widget.KeyPageUp, 15},
		{widget.KeyPageDown, 5},
		{widget.KeyEnd, 60},
		{widget.KeyRight, 60},
		{widget.KeyPageUp, 60},
		{widget.KeyHome, 1},
		{widget.KeyLeft, 1},
		{widget.KeyPageDown, 1},
	}
	for i, c := range cases {
		if !r.key(c.code, 0) {
			t.Fatalf("step %d: key %d must be consumed", i, c.code)
		}
		if got := r.src.cfg.Slideshow.Interval; got != c.want {
			t.Fatalf("step %d: key %d gave %d, want %d", i, c.code, got, c.want)
		}
	}
}

func TestSliderKeysDoNotWriteWhenNothingChanges(t *testing.T) {
	r := newRig(t, mockW, mockH)
	r.page.SetSection(SectionSlideshow)
	r.src.cfg.Slideshow.Interval = 1
	r.page.Refresh()
	r.focusOn("slideshow.interval")
	r.rec.reset()
	r.key(widget.KeyLeft, 0)
	r.key(widget.KeyHome, 0)
	if r.rec.has("update") {
		t.Fatal("a slider already at its minimum must not write settings")
	}
}

func TestEscapeBackspaceAndAltLeftLeaveThePage(t *testing.T) {
	for name, e := range map[string]widget.KeyEvent{
		"Escape":    {Code: widget.KeyEscape},
		"Backspace": {Code: widget.KeyBackspace},
		"Alt+Left":  {Code: widget.KeyLeft, Mod: widget.ModAlt},
	} {
		r := newRig(t, mockW, mockH)
		e.Pressed = true
		if !r.page.HandleKey(e) || !r.rec.has("back") {
			t.Errorf("%s must leave the page, calls = %v", name, r.rec.calls)
		}
		r.rec.reset()
		e.Repeat = true
		r.page.HandleKey(e)
		if r.rec.has("back") {
			t.Errorf("%s auto-repeat must not leave twice", name)
		}
	}
}

func TestEnterOnTheBackButtonLeavesThePage(t *testing.T) {
	r := newRig(t, mockW, mockH)
	r.focusOn(focusBack)
	r.key(widget.KeyEnter, 0)
	if !r.rec.has("back") {
		t.Fatal("Enter on the focused back button must leave")
	}
}

func TestControlTabAndPageKeysSwitchSections(t *testing.T) {
	r := newRig(t, mockW, mockH)
	steps := []struct {
		code widget.KeyCode
		mod  widget.KeyMod
		want Section
	}{
		{widget.KeyTab, widget.ModCtrl, SectionView},
		{widget.KeyPageDown, widget.ModCtrl, SectionFormats},
		{widget.KeyPageUp, widget.ModCtrl, SectionView},
		{widget.KeyTab, widget.ModCtrl | widget.ModShift, SectionGeneral},
		{widget.KeyPageUp, widget.ModCtrl, SectionAbout},
		{widget.KeyTab, widget.ModCtrl, SectionGeneral},
	}
	for i, s := range steps {
		if !r.key(s.code, s.mod) {
			t.Fatalf("step %d must be consumed", i)
		}
		if r.page.Section() != s.want {
			t.Fatalf("step %d: section %d, want %d", i, r.page.Section(), s.want)
		}
	}
}

func TestNavigationFocusMovesBetweenSections(t *testing.T) {
	r := newRig(t, mockW, mockH)
	r.focusOn(focusNav)
	steps := []struct {
		code widget.KeyCode
		want Section
	}{
		{widget.KeyDown, SectionView},
		{widget.KeyRight, SectionFormats},
		{widget.KeyUp, SectionView},
		{widget.KeyLeft, SectionGeneral},
		{widget.KeyUp, SectionAbout},
		{widget.KeyDown, SectionGeneral},
		{widget.KeyEnd, SectionAbout},
		{widget.KeyHome, SectionGeneral},
	}
	for i, s := range steps {
		r.key(s.code, 0)
		if r.page.Section() != s.want || r.page.focus != focusNav {
			t.Fatalf("step %d: section %d focus %q, want section %d on the navigation", i, r.page.Section(), r.page.focus, s.want)
		}
	}
}

func TestFocusIsHiddenUntilTheKeyboardIsUsed(t *testing.T) {
	r := newRig(t, mockW, mockH)
	r.focusOn("rememberWindow")
	r.page.keyNav = false
	before := r.render()
	r.page.keyNav = true
	r.page.invalidate()
	after := r.render()
	it := r.item("rememberWindow")
	region := it.r.Inset(-6)
	if sameImage(before, after, region) {
		t.Fatal("keyboard focus must be drawn as a visible frame")
	}
	r.page.keyNav = false
	if !sameImage(before, r.render(), region) {
		t.Fatal("the frame must disappear when the mouse takes over")
	}
}

func TestMousePressHidesTheKeyboardFrameAndMovesFocus(t *testing.T) {
	r := newRig(t, mockW, mockH)
	r.key(widget.KeyTab, 0)
	if !r.page.keyNav {
		t.Fatal("keyboard navigation must show the frame")
	}
	r.clickItem("confirmDelete")
	if r.page.keyNav || r.page.focus != "confirmDelete" {
		t.Fatalf("keyNav %v focus %q after a click", r.page.keyNav, r.page.focus)
	}
}

func TestFocusRingIsDrawnForEveryKindOfControl(t *testing.T) {
	cases := []struct {
		section Section
		id      string
	}{
		{SectionGeneral, "theme:system"},
		{SectionGeneral, "language"},
		{SectionGeneral, "rememberWindow"},
		{SectionGeneral, "background:checker"},
		{SectionView, "wheel:navigate"},
		{SectionFormats, "format:avif"},
		{SectionFormats, "defaultApps"},
		{SectionSlideshow, "slideshow.interval"},
		{SectionAbout, "repository"},
		{SectionAbout, "checkUpdates"},
	}
	for _, c := range cases {
		t.Run(c.id, func(t *testing.T) {
			r := newRig(t, mockW, 700)
			r.page.SetSection(c.section)
			r.page.focus, r.page.keyNav = "", false
			plain := r.render()
			r.focusOn(c.id)
			r.page.invalidate()
			focused := r.render()
			it := r.item(c.id)
			if sameImage(plain, focused, it.r.Inset(-6)) {
				t.Fatalf("%s: no focus frame is drawn", c.id)
			}
		})
	}
}

func TestFocusingAControlBelowTheFoldScrollsToIt(t *testing.T) {
	r := newRig(t, mockW, mockH)
	r.focusOn("autoHidePanels")
	for i := 0; i < 3; i++ {
		r.key(widget.KeyTab, 0)
	}
	if r.page.focus != "updatesCheck" {
		t.Fatalf("focus = %q", r.page.focus)
	}
	it := r.item("updatesCheck")
	bottom := it.r.Max.Y - r.page.scroll
	if r.page.scroll == 0 || bottom > r.page.geo.view.Max.Y {
		t.Fatalf("scroll = %d, control bottom at %d, view bottom %d", r.page.scroll, bottom, r.page.geo.view.Max.Y)
	}
}

func TestButtonsAndLinksActivateWithTheKeyboard(t *testing.T) {
	r := newRig(t, mockW, mockH)
	r.page.SetSection(SectionAbout)
	r.focusOn("checkUpdates")
	r.key(widget.KeyEnter, 0)
	r.key(widget.KeySpace, 0)
	r.page.HandleKey(widget.KeyEvent{Code: widget.KeySpace, Pressed: true, Repeat: true})
	if r.rec.count("checkUpdates") != 2 {
		t.Fatalf("calls = %v, want the action twice (repeats are ignored)", r.rec.calls)
	}
	r.focusOn("repository")
	r.key(widget.KeyEnter, 0)
	if len(r.rec.urls) != 1 {
		t.Fatalf("urls = %v", r.rec.urls)
	}
}

func TestVerticalArrowsMoveBetweenPlainControls(t *testing.T) {
	r := newRig(t, mockW, mockH)
	r.focusOn("rememberWindow")
	r.key(widget.KeyDown, 0)
	if r.page.focus != "restoreLastImage" {
		t.Fatalf("Down: focus = %q", r.page.focus)
	}
	r.key(widget.KeyUp, 0)
	r.key(widget.KeyUp, 0)
	if r.page.focus != "language" {
		t.Fatalf("Up: focus = %q", r.page.focus)
	}
}

func TestKeysThePageDoesNotUseAreNotConsumed(t *testing.T) {
	r := newRig(t, mockW, mockH)
	for name, e := range map[string]widget.KeyEvent{
		"rune":      {Code: widget.KeyUnknown, Rune: 'a', Pressed: true},
		"F5":        {Code: widget.KeyF5, Pressed: true},
		"Ctrl+C":    {Code: widget.KeyC, Mod: widget.ModCtrl, Pressed: true},
		"Alt+Right": {Code: widget.KeyRight, Mod: widget.ModAlt, Pressed: true},
		"release":   {Code: widget.KeyEscape, Pressed: false},
		"Ctrl+Alt":  {Code: widget.KeyTab, Mod: widget.ModCtrl | widget.ModAlt, Pressed: true},
	} {
		if r.page.HandleKey(e) {
			t.Errorf("%s must be left to the root", name)
		}
	}
	if r.rec.has("back") {
		t.Fatal("none of these keys may leave the page")
	}
}

func TestScrollKeysMoveTheContent(t *testing.T) {
	r := newRig(t, mockW, mockH)
	r.focusOn(focusNav)
	r.key(widget.KeyPageDown, 0)
	if r.page.scroll == 0 {
		t.Fatal("PageDown must scroll")
	}
	r.key(widget.KeyEnd, 0)
	if r.page.scroll != r.page.maxScroll() {
		t.Fatalf("End: scroll = %d, max = %d", r.page.scroll, r.page.maxScroll())
	}
	r.key(widget.KeyPageUp, 0)
	r.key(widget.KeyHome, 0)
	if r.page.scroll != 0 {
		t.Fatalf("Home: scroll = %d", r.page.scroll)
	}
	r.page.focus = ""
	r.key(widget.KeyDown, 0)
	if r.page.scroll != scrollKeyStep {
		t.Fatalf("Down: scroll = %d", r.page.scroll)
	}
	r.key(widget.KeyUp, 0)
	if r.page.scroll != 0 {
		t.Fatalf("Up: scroll = %d", r.page.scroll)
	}
}
