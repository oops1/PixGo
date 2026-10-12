package gallery

import (
	"image"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/widget"
)

func TestArrowKeysMoveTheSelection(t *testing.T) {
	cases := []struct {
		name  string
		start int
		code  widget.KeyCode
		want  int
	}{
		{"right", 0, widget.KeyRight, 1},
		{"down", 0, widget.KeyDown, 4},
		{"left from second", 1, widget.KeyLeft, 0},
		{"up from second row", 5, widget.KeyUp, 1},
		{"end", 0, widget.KeyEnd, 11},
		{"home", 7, widget.KeyHome, 0},
		{"page down", 1, widget.KeyPageDown, 9},
		{"page up", 9, widget.KeyPageUp, 1},
		{"down on last row stays", 9, widget.KeyDown, 9},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			src := mockSource()
			src.selected = c.start
			r := newRig(t, src, mockW, mockH)

			handled := r.key(c.code, 0)

			if !handled {
				t.Fatal("navigation keys are always handled")
			}
			if r.page.Selected() != c.want {
				t.Fatalf("selected = %d, want %d (calls %v)", r.page.Selected(), c.want, r.rec.calls)
			}
		})
	}
}

func TestMovingAtTheEdgeDoesNotReselect(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)

	r.key(widget.KeyUp, 0)
	r.key(widget.KeyLeft, 0)

	if len(r.rec.calls) != 0 {
		t.Fatalf("calls = %v", r.rec.calls)
	}
}

func TestNavigationFromNoSelectionPicksTheFirstOrLast(t *testing.T) {
	src := mockSource()
	src.selected = -1
	r := newRig(t, src, mockW, mockH)
	r.key(widget.KeyRight, 0)
	if r.page.Selected() != 0 {
		t.Fatalf("selected = %d", r.page.Selected())
	}
	src.selected = -1
	r.page.Refresh()
	r.key(widget.KeyEnd, 0)
	if r.page.Selected() != 11 {
		t.Fatalf("selected = %d", r.page.Selected())
	}
}

func TestKeyboardNavigationScrollsTheSelectionIntoView(t *testing.T) {
	src := mockSource()
	src.items = manyItems(60)
	r := newRig(t, src, mockW, mockH)

	r.key(widget.KeyEnd, 0)

	if r.page.scroll == 0 {
		t.Fatal("the view must follow the selection")
	}
	cell := r.page.tileRect(59)
	if !cell.In(r.page.geo.grid) {
		t.Fatalf("selected cell %v is outside the viewport %v", cell, r.page.geo.grid)
	}
	r.key(widget.KeyHome, 0)
	if r.page.scroll != 0 {
		t.Fatalf("scroll = %d after Home", r.page.scroll)
	}
}

func TestCommandKeys(t *testing.T) {
	cases := []struct {
		name string
		code widget.KeyCode
		mod  widget.KeyMod
		want string
	}{
		{"enter opens", widget.KeyEnter, 0, "open:0"},
		{"delete deletes", widget.KeyDelete, 0, "delete:0"},
		{"ctrl+d favourites", widget.KeyD, widget.ModCtrl, "favorite:0"},
		{"alt+left goes back", widget.KeyLeft, widget.ModAlt, "back"},
		{"alt+up goes up", widget.KeyUp, widget.ModAlt, "up"},
		{"backspace goes up", widget.KeyBackspace, 0, "up"},
		{"f5 refreshes", widget.KeyF5, 0, "refresh"},
		{"ctrl+r refreshes", widget.KeyR, widget.ModCtrl, "refresh"},
		{"escape closes", widget.KeyEscape, 0, "close"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newRig(t, mockSource(), mockW, mockH)

			if !r.key(c.code, c.mod) {
				t.Fatal("key not handled")
			}

			if !r.rec.has(c.want) {
				t.Fatalf("calls = %v, want %q", r.rec.calls, c.want)
			}
		})
	}
}

func TestAltRightGoesForward(t *testing.T) {
	src := mockSource()
	src.canForward = true
	r := newRig(t, src, mockW, mockH)

	r.key(widget.KeyRight, widget.ModAlt)

	if !r.rec.has("forward") {
		t.Fatalf("calls = %v", r.rec.calls)
	}
}

func TestHistoryKeysAreSwallowedButInertWhenUnavailable(t *testing.T) {
	src := mockSource()
	src.canBack, src.canForward, src.canUp = false, false, false
	r := newRig(t, src, mockW, mockH)

	for _, c := range []struct {
		code widget.KeyCode
		mod  widget.KeyMod
	}{{widget.KeyLeft, widget.ModAlt}, {widget.KeyRight, widget.ModAlt}, {widget.KeyUp, widget.ModAlt}, {widget.KeyBackspace, 0}} {
		if !r.key(c.code, c.mod) {
			t.Errorf("%v+%v must be consumed", c.mod, c.code)
		}
	}

	if len(r.rec.calls) != 0 {
		t.Fatalf("calls = %v", r.rec.calls)
	}
}

func TestFocusAddressKeys(t *testing.T) {
	for _, c := range []struct {
		name string
		code widget.KeyCode
		mod  widget.KeyMod
	}{{"ctrl+l", widget.KeyL, widget.ModCtrl}, {"f4", widget.KeyF4, 0}} {
		t.Run(c.name, func(t *testing.T) {
			r := newRig(t, mockSource(), mockW, mockH)

			r.key(c.code, c.mod)

			if !r.page.Editing() {
				t.Fatal("the address must be in edit mode")
			}
		})
	}
}

func TestGlobalShortcutsAreLeftToTheWindow(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)

	for _, c := range []struct {
		code widget.KeyCode
		mod  widget.KeyMod
	}{{widget.KeyO, widget.ModCtrl}, {widget.KeyG, widget.ModCtrl}, {widget.KeyS, widget.ModCtrl | widget.ModShift}, {widget.KeyF11, 0}, {widget.KeyEnter, widget.ModAlt}} {
		if r.key(c.code, c.mod) {
			t.Errorf("%v+%v must not be consumed", c.mod, c.code)
		}
	}

	if len(r.rec.calls) != 0 {
		t.Fatalf("calls = %v", r.rec.calls)
	}
}

func TestKeyReleasesAndRepeatsDoNotRepeatCommands(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)

	if r.page.HandleKey(widget.KeyEvent{Code: widget.KeyEnter, Pressed: false}) {
		t.Error("key up must be ignored")
	}
	r.page.HandleKey(widget.KeyEvent{Code: widget.KeyEnter, Pressed: true, Repeat: true})
	r.page.HandleKey(widget.KeyEvent{Code: widget.KeyDelete, Pressed: true, Repeat: true})
	r.page.HandleKey(widget.KeyEvent{Code: widget.KeyD, Mod: widget.ModCtrl, Pressed: true, Repeat: true})
	r.page.HandleKey(widget.KeyEvent{Code: widget.KeyF5, Pressed: true, Repeat: true})
	r.page.HandleKey(widget.KeyEvent{Code: widget.KeyR, Mod: widget.ModCtrl, Pressed: true, Repeat: true})
	r.page.HandleKey(widget.KeyEvent{Code: widget.KeyEscape, Pressed: true, Repeat: true})

	if len(r.rec.calls) != 0 {
		t.Fatalf("calls = %v", r.rec.calls)
	}
}

func TestRepeatedArrowsKeepMoving(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)

	r.page.HandleKey(widget.KeyEvent{Code: widget.KeyRight, Pressed: true, Repeat: true})
	r.page.HandleKey(widget.KeyEvent{Code: widget.KeyRight, Pressed: true, Repeat: true})

	if r.page.Selected() != 2 {
		t.Fatalf("selected = %d", r.page.Selected())
	}
}

func TestEnterAndDeleteNeedASelection(t *testing.T) {
	src := mockSource()
	src.selected = -1
	r := newRig(t, src, mockW, mockH)

	r.key(widget.KeyEnter, 0)
	r.key(widget.KeyDelete, 0)
	r.key(widget.KeyD, widget.ModCtrl)

	if len(r.rec.calls) != 0 {
		t.Fatalf("calls = %v", r.rec.calls)
	}
}

func TestFavouriteShortcutIgnoresFolders(t *testing.T) {
	src := mockSource()
	src.items[0].Dir = true
	r := newRig(t, src, mockW, mockH)

	r.key(widget.KeyD, widget.ModCtrl)

	if len(r.rec.calls) != 0 {
		t.Fatalf("calls = %v", r.rec.calls)
	}
}

func TestMenuKeyOpensTheContextMenuOfTheSelection(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)

	r.key(widget.KeyMenu, 0)

	if !r.rec.has("menu") || len(r.rec.menuItems) != 7 {
		t.Fatalf("calls = %v, menu = %d", r.rec.calls, len(r.rec.menuItems))
	}
	src := mockSource()
	src.selected = -1
	r2 := newRig(t, src, mockW, mockH)
	r2.key(widget.KeyMenu, 0)
	if r2.rec.has("menu") {
		t.Fatal("no menu without a selection")
	}
}

func TestTypingJumpsToTheFirstMatchingName(t *testing.T) {
	cases := []struct {
		name  string
		typed string
		want  int
	}{
		{"single letter", "f", 2},
		{"longer prefix", "fl", 3},
		{"upper case", "SN", 11},
		{"no match keeps selection", "z", 0},
		{"two letters", "ma", 9},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newRig(t, mockSource(), mockW, mockH)

			for _, ch := range c.typed {
				if !r.typeRune(ch) {
					t.Fatalf("%q not consumed", ch)
				}
			}

			if r.page.Selected() != c.want {
				t.Fatalf("selected = %d, want %d", r.page.Selected(), c.want)
			}
		})
	}
}

func TestTypeAheadForgetsThePrefixAfterAPause(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)
	r.typeRune('f')
	if r.page.Selected() != 2 {
		t.Fatalf("selected = %d", r.page.Selected())
	}

	r.tm.advance(typeAheadReset)
	r.typeRune('l')

	if r.page.Selected() != 1 {
		t.Fatalf("the second letter must start a new prefix (lake), got %d", r.page.Selected())
	}
}

func TestTypeAheadRestartsItsTimerOnEveryLetter(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)
	r.typeRune('f')
	r.tm.advance(typeAheadReset - time.Millisecond)
	r.typeRune('l')

	r.tm.advance(typeAheadReset - time.Millisecond)

	if string(r.page.typed) != "fl" {
		t.Fatalf("typed = %q", string(r.page.typed))
	}
}

func TestTypeAheadHandlesCyrillic(t *testing.T) {
	src := mockSource()
	src.items[4].Name = "Город.jpg"
	src.items[4].Path = mockPath("Город.jpg")
	r := newRig(t, src, mockW, mockH)

	r.typeRune('г')

	if r.page.Selected() != 4 {
		t.Fatalf("selected = %d", r.page.Selected())
	}
}

func TestTypeAheadScrollsTheMatchIntoView(t *testing.T) {
	src := mockSource()
	src.items = manyItems(60)
	r := newRig(t, src, mockW, mockH)

	for _, ch := range "img59" {
		r.typeRune(ch)
	}

	if r.page.Selected() != 59 || r.page.scroll == 0 {
		t.Fatalf("selected = %d, scroll = %d", r.page.Selected(), r.page.scroll)
	}
}

func TestCtrlLettersAreNotTyped(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)

	handled := r.page.HandleKey(widget.KeyEvent{Code: widget.KeyUnknown, Rune: 'f', Mod: widget.ModCtrl, Pressed: true})
	control := r.typeRune(3)

	if handled || control || r.page.Selected() != 0 {
		t.Fatalf("handled=%v control=%v selected=%d", handled, control, r.page.Selected())
	}
}

func TestTypingInAnEmptyFolderIsIgnored(t *testing.T) {
	src := mockSource()
	src.items = nil
	r := newRig(t, src, mockW, mockH)

	if r.typeRune('a') {
		t.Fatal("nothing to match")
	}
}

func TestEditingOnlyHandlesEscapeAndEnter(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)
	r.page.FocusAddress()

	for _, c := range []widget.KeyCode{widget.KeyRight, widget.KeyDelete, widget.KeyBackspace, widget.KeyF5, widget.KeyA} {
		if r.key(c, 0) {
			t.Errorf("%v must go to the text field", c)
		}
	}
	if r.typeRune('x') {
		t.Error("characters must go to the text field")
	}
	if len(r.rec.calls) != 0 {
		t.Fatalf("calls = %v", r.rec.calls)
	}
}

func TestEscapeCancelsTheAddressEdit(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)
	r.page.FocusAddress()

	if !r.key(widget.KeyEscape, 0) {
		t.Fatal("escape must be consumed")
	}

	if r.page.Editing() || r.rec.has("close") || r.rec.has("address") {
		t.Fatalf("editing = %v, calls = %v", r.page.Editing(), r.rec.calls)
	}
	if len(r.page.Children()) != 0 {
		t.Fatal("the input must leave the tree")
	}
}

func TestEnterCommitsTheAddressText(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)
	r.page.FocusAddress()
	r.page.input.SetText(`D:\Photos\Other`)

	if !r.key(widget.KeyEnter, 0) {
		t.Fatal("enter must be consumed")
	}

	if r.rec.addrText != `D:\Photos\Other` || r.page.Editing() {
		t.Fatalf("text = %q, editing = %v", r.rec.addrText, r.page.Editing())
	}
}

func TestEnterFromTheTextFieldItselfAlsoCommits(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)
	r.page.FocusAddress()

	r.page.input.OnKeyEvent(widget.KeyEvent{Code: widget.KeyEnter, Pressed: true})

	if !r.rec.has("address") {
		t.Fatalf("calls = %v", r.rec.calls)
	}
}

func TestLosingFocusCancelsTheEdit(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)
	r.page.FocusAddress()

	r.page.input.SetFocused(false)

	if r.page.Editing() {
		t.Fatal("blur must cancel")
	}
	if r.rec.has("address") {
		t.Fatalf("calls = %v", r.rec.calls)
	}
}

func TestNavigationRefreshEndsTheEditWhenTheFolderChanges(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)
	r.page.FocusAddress()
	r.src.loc.Path = `D:\Photos\Other`

	r.page.Refresh()

	if r.page.Editing() {
		t.Fatal("a new location replaces the edited address")
	}
}

func TestEditingSurvivesAnUnrelatedRefresh(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)
	r.page.FocusAddress()

	r.page.Refresh()

	if !r.page.Editing() {
		t.Fatal("refresh of the same folder must not drop the editor")
	}
}

func TestResizingMovesTheEditor(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)
	r.page.FocusAddress()
	before := r.page.input.Bounds()

	r.page.SetBounds(image.Rect(0, 0, 700, 430))

	if after := r.page.input.Bounds(); after.Max.X <= before.Max.X {
		t.Fatalf("editor bounds %v -> %v", before, after)
	}
}
