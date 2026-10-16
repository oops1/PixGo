package saveopts

import (
	"image"
	"strings"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/i18n"
)

func TestInitialValuesAreClampedAndKindSpecific(t *testing.T) {
	tests := []struct {
		name    string
		kind    Kind
		initial Values
		want    Values
	}{
		{"jpeg default", JPEG, Values{Quality: 90}, Values{Quality: 90}},
		{"jpeg below range", JPEG, Values{Quality: 0}, Values{Quality: 1}},
		{"jpeg negative", JPEG, Values{Quality: -50}, Values{Quality: 1}},
		{"jpeg above range", JPEG, Values{Quality: 400}, Values{Quality: 100}},
		{"jpeg has no lossless mode", JPEG, Values{Quality: 70, Lossless: true}, Values{Quality: 70}},
		{"webp lossy", WebP, Values{Quality: 80}, Values{Quality: 80}},
		{"webp lossless is kept", WebP, Values{Quality: 80, Lossless: true}, Values{Quality: 80, Lossless: true}},
		{"jpeg lossless is kept", JPEGLossless, Values{Quality: 90, Lossless: true}, Values{Quality: 90, Lossless: true}},
		{"jpeg lossless can be off", JPEGLossless, Values{Quality: 55}, Values{Quality: 55}},
		{"jpeg lossless clamps the quality", JPEGLossless, Values{Quality: 0, Lossless: true}, Values{Quality: 1, Lossless: true}},
		{"webp lower bound", WebP, Values{Quality: 1}, Values{Quality: 1}},
		{"webp upper bound", WebP, Values{Quality: 100, Lossless: true}, Values{Quality: 100, Lossless: true}},
		{"webp out of range", WebP, Values{Quality: 101}, Values{Quality: 100}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t, tc.kind, tc.initial, 760, 420)

			if got := r.d.Values(); got != tc.want {
				t.Fatalf("values = %+v, want %+v", got, tc.want)
			}
			if r.d.Kind() != tc.kind {
				t.Fatalf("kind = %v", r.d.Kind())
			}
		})
	}
}

func TestDialogOffersExactlyTheControlsOfItsFormat(t *testing.T) {
	tests := []struct {
		kind        Kind
		wantCheck   bool
		wantControl []widget.AccessRole
	}{
		{JPEG, false, []widget.AccessRole{widget.RoleSlider, widget.RoleButton, widget.RoleButton}},
		{WebP, true, []widget.AccessRole{widget.RoleCheckBox, widget.RoleSlider, widget.RoleButton, widget.RoleButton}},
		{JPEGLossless, true, []widget.AccessRole{widget.RoleCheckBox, widget.RoleSlider, widget.RoleButton, widget.RoleButton}},
	}
	for _, tc := range tests {
		t.Run(kindName(tc.kind), func(t *testing.T) {
			r := newRig(t, tc.kind, Values{Quality: 50}, 760, 420)

			if has := !r.d.Rect(PartCheck).Empty(); has != tc.wantCheck {
				t.Errorf("checkbox present = %v, want %v", has, tc.wantCheck)
			}
			var roles []widget.AccessRole
			for _, a := range r.d.form.AccessChildren() {
				roles = append(roles, a.Role)
			}
			if len(roles) != len(tc.wantControl) {
				t.Fatalf("roles = %v, want %v", roles, tc.wantControl)
			}
			for i := range roles {
				if roles[i] != tc.wantControl[i] {
					t.Errorf("role %d = %v, want %v", i, roles[i], tc.wantControl[i])
				}
			}
		})
	}
}

func TestTitleAndButtonsAreTranslated(t *testing.T) {
	tests := []struct {
		lang, title, save, cancel string
	}{
		{"ru", "Параметры сохранения", "Сохранить", "Отмена"},
		{"en", "Save options", "Save", "Cancel"},
	}
	for _, tc := range tests {
		t.Run(tc.lang, func(t *testing.T) {
			withScale(t, 1, tc.lang)
			r := newRig(t, JPEG, Values{Quality: 90}, 760, 420)

			var names []string
			for _, a := range r.d.form.AccessChildren() {
				if a.Role == widget.RoleButton {
					names = append(names, a.Name)
				}
			}

			if r.d.Widget().Title != tc.title {
				t.Errorf("title %q, want %q", r.d.Widget().Title, tc.title)
			}
			if len(names) != 2 || names[0] != tc.save || names[1] != tc.cancel {
				t.Errorf("buttons %v, want [%s %s]", names, tc.save, tc.cancel)
			}
		})
	}
}

func TestSliderKeysMoveByTheDocumentedSteps(t *testing.T) {
	tests := []struct {
		name  string
		start int
		keys  []widget.KeyCode
		want  int
	}{
		{"right adds one", 50, []widget.KeyCode{widget.KeyRight}, 51},
		{"up adds one", 50, []widget.KeyCode{widget.KeyUp}, 51},
		{"left removes one", 50, []widget.KeyCode{widget.KeyLeft}, 49},
		{"down removes one", 50, []widget.KeyCode{widget.KeyDown}, 49},
		{"page up adds ten", 50, []widget.KeyCode{widget.KeyPageUp}, 60},
		{"page down removes ten", 50, []widget.KeyCode{widget.KeyPageDown}, 40},
		{"home goes to the minimum", 50, []widget.KeyCode{widget.KeyHome}, 1},
		{"end goes to the maximum", 50, []widget.KeyCode{widget.KeyEnd}, 100},
		{"left stops at the minimum", 1, []widget.KeyCode{widget.KeyLeft, widget.KeyLeft}, 1},
		{"right stops at the maximum", 100, []widget.KeyCode{widget.KeyRight, widget.KeyRight}, 100},
		{"page up is clamped", 95, []widget.KeyCode{widget.KeyPageUp}, 100},
		{"page down is clamped", 5, []widget.KeyCode{widget.KeyPageDown}, 1},
		{"home then end", 50, []widget.KeyCode{widget.KeyHome, widget.KeyEnd}, 100},
		{"end then left twice", 50, []widget.KeyCode{widget.KeyEnd, widget.KeyLeft, widget.KeyLeft}, 98},
		{"unrelated key changes nothing", 50, []widget.KeyCode{widget.KeyA, widget.KeyF1}, 50},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t, JPEG, Values{Quality: tc.start}, 760, 420)

			for _, k := range tc.keys {
				r.press(k)
			}

			if got := r.d.Values().Quality; got != tc.want {
				t.Fatalf("quality = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestEveryQualityFromOneToAHundredIsReachableByKeys(t *testing.T) {
	r := newRig(t, JPEG, Values{Quality: 1}, 760, 420)
	for want := 2; want <= 100; want++ {
		r.press(widget.KeyRight)
		if got := r.d.Values().Quality; got != want {
			t.Fatalf("after %d presses quality = %d", want-1, got)
		}
	}
	for want := 99; want >= 1; want-- {
		r.press(widget.KeyLeft)
		if got := r.d.Values().Quality; got != want {
			t.Fatalf("going down: quality = %d, want %d", got, want)
		}
	}
}

func TestSliderMouseMapsTheTrackOntoTheWholeRange(t *testing.T) {
	tests := []struct {
		name string
		x    func(r *rig) int
		want int
	}{
		{"left end of the track", func(r *rig) int { return r.trackX(1) }, 1},
		{"right end of the track", func(r *rig) int { return r.trackX(100) }, 100},
		{"left of the track is clamped", func(r *rig) int { return r.d.Rect(PartSlider).Min.X }, 1},
		{"right of the track is clamped", func(r *rig) int { return r.d.Rect(PartSlider).Max.X - 1 }, 100},
		{"a quarter of the way", func(r *rig) int { return r.trackX(26) }, 26},
		{"the middle", func(r *rig) int { return r.trackX(51) }, 51},
		{"three quarters", func(r *rig) int { return r.trackX(75) }, 75},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t, JPEG, Values{Quality: 90}, 760, 420)
			_, _, cy := r.d.form.track()

			r.click(tc.x(r), cy)

			if got := r.d.Values().Quality; got != tc.want {
				t.Fatalf("quality = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestDraggingTheThumbFollowsTheMouseAndStaysInRange(t *testing.T) {
	r := newRig(t, JPEG, Values{Quality: 90}, 760, 420)
	_, _, cy := r.d.form.track()
	r.eng.SendMouseMove(r.trackX(90), cy)
	r.eng.SendMouseButton(r.trackX(90), cy, widget.MouseLeft, true)

	r.eng.SendMouseMove(r.trackX(30), cy)
	atThirty := r.d.Values().Quality
	r.eng.SendMouseMove(r.d.Rect(PartSlider).Min.X-300, cy+200)
	farLeft := r.d.Values().Quality
	r.eng.SendMouseMove(r.d.Rect(PartSlider).Max.X+300, cy-200)
	farRight := r.d.Values().Quality
	r.eng.SendMouseButton(r.trackX(60), cy, widget.MouseLeft, false)
	r.eng.SendMouseMove(r.trackX(10), cy)

	if atThirty != 30 || farLeft != 1 || farRight != 100 {
		t.Fatalf("while dragging: %d, %d, %d; want 30, 1, 100", atThirty, farLeft, farRight)
	}
	if got := r.d.Values().Quality; got != 100 {
		t.Fatalf("moving after the release changed the value to %d", got)
	}
}

func TestMouseOnTheSliderMovesTheKeyboardFocusThere(t *testing.T) {
	r := newRig(t, WebP, Values{Quality: 80}, 760, 420)
	if r.d.Focused() != PartCheck {
		t.Fatalf("a WebP dialog starts on %v", r.d.Focused())
	}

	r.clickQuality(40)
	r.press(widget.KeyRight)

	if r.d.Focused() != PartSlider || r.d.Values().Quality != 41 {
		t.Fatalf("focus %v quality %d", r.d.Focused(), r.d.Values().Quality)
	}
}

func TestKeyboardWorksAtOnceWithoutAClick(t *testing.T) {
	tests := []struct {
		kind Kind
		want Part
	}{{JPEG, PartSlider}, {WebP, PartCheck}, {JPEGLossless, PartCheck}}
	for _, tc := range tests {
		t.Run(kindName(tc.kind), func(t *testing.T) {
			r := newRig(t, tc.kind, Values{Quality: 50}, 760, 420)

			if got := r.d.Focused(); got != tc.want {
				t.Fatalf("initial focus %v, want %v", got, tc.want)
			}
		})
	}
	r := newRig(t, JPEG, Values{Quality: 50}, 760, 420)
	r.press(widget.KeyEnd)
	if r.d.Values().Quality != 100 {
		t.Fatal("an arrow key must work before any click")
	}
}

func TestEnterSavesTheCurrentValues(t *testing.T) {
	tests := []struct {
		name string
		kind Kind
		init Values
		keys []widget.KeyCode
		want Values
	}{
		{"jpeg untouched", JPEG, Values{Quality: 90}, nil, Values{Quality: 90}},
		{"jpeg after the arrows", JPEG, Values{Quality: 90}, []widget.KeyCode{widget.KeyLeft, widget.KeyLeft, widget.KeyPageDown}, Values{Quality: 78}},
		{"jpeg home", JPEG, Values{Quality: 90}, []widget.KeyCode{widget.KeyHome}, Values{Quality: 1}},
		{"jpeg end", JPEG, Values{Quality: 10}, []widget.KeyCode{widget.KeyEnd}, Values{Quality: 100}},
		{"webp untouched", WebP, Values{Quality: 80}, nil, Values{Quality: 80}},
		{"webp lossless by space", WebP, Values{Quality: 80}, []widget.KeyCode{widget.KeySpace}, Values{Quality: 80, Lossless: true}},
		{"webp quality after tab", WebP, Values{Quality: 80}, []widget.KeyCode{widget.KeyTab, widget.KeyLeft}, Values{Quality: 79}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t, tc.kind, tc.init, 760, 420)
			for _, k := range tc.keys {
				r.press(k)
			}

			r.press(widget.KeyEnter)

			if len(r.results) != 1 || !r.last().ok || r.last().values != tc.want {
				t.Fatalf("results = %+v, want one save with %+v", r.results, tc.want)
			}
			if !r.d.Done() {
				t.Fatal("the dialog must report itself done")
			}
		})
	}
}

func TestEscapeCancelsWithoutSaving(t *testing.T) {
	r := newRig(t, JPEG, Values{Quality: 90}, 760, 420)
	r.press(widget.KeyHome)

	r.press(widget.KeyEscape)

	if len(r.results) != 1 || r.last().ok {
		t.Fatalf("results = %+v, want a single cancellation", r.results)
	}
}

func TestEscapeClosesTheModalAndEnterNoLongerReachesIt(t *testing.T) {
	r := newRig(t, JPEG, Values{Quality: 90}, 760, 420)
	r.press(widget.KeyEscape)

	r.press(widget.KeyEnter)

	if len(r.results) != 1 || r.last().ok {
		t.Fatalf("results = %+v: nothing may be saved after the dialog went away", r.results)
	}
}

func TestButtonsByMouse(t *testing.T) {
	tests := []struct {
		name   string
		button Part
		want   bool
	}{{"save", PartSave, true}, {"cancel", PartCancel, false}}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t, JPEG, Values{Quality: 90}, 760, 420)
			r.clickQuality(33)

			r.clickPart(tc.button)

			if len(r.results) != 1 || r.last().ok != tc.want {
				t.Fatalf("results = %+v", r.results)
			}
			if tc.want && r.last().values.Quality != 33 {
				t.Fatalf("saved %d, want 33", r.last().values.Quality)
			}
		})
	}
}

func TestReleasingOutsideTheButtonDoesNothing(t *testing.T) {
	r := newRig(t, JPEG, Values{Quality: 90}, 760, 420)
	x, y := centre(r.d.Rect(PartSave))
	r.eng.SendMouseMove(x, y)
	r.eng.SendMouseButton(x, y, widget.MouseLeft, true)

	ox, oy := centre(r.d.Rect(PartSlider))
	r.eng.SendMouseMove(ox, oy)
	r.eng.SendMouseButton(ox, oy, widget.MouseLeft, false)

	if len(r.results) != 0 {
		t.Fatalf("a press dragged away from the button must not click it: %+v", r.results)
	}
}

func TestSpaceOnAFocusedButtonPressesIt(t *testing.T) {
	r := newRig(t, JPEG, Values{Quality: 90}, 760, 420)
	r.press(widget.KeyTab)
	r.press(widget.KeyTab)

	r.press(widget.KeySpace)

	if len(r.results) != 1 || r.last().ok {
		t.Fatalf("Space on Cancel gave %+v", r.results)
	}
}

func TestEnterOnTheFocusedCancelButtonCancels(t *testing.T) {
	r := newRig(t, JPEG, Values{Quality: 90}, 760, 420)
	r.press(widget.KeyTab)
	r.press(widget.KeyTab)
	if r.d.Focused() != PartCancel {
		t.Fatalf("focus is on %v", r.d.Focused())
	}

	r.press(widget.KeyEnter)

	if len(r.results) != 1 || r.last().ok {
		t.Fatalf("Enter on Cancel gave %+v", r.results)
	}
}

func TestTabWalksTheControlsInOrderAndWraps(t *testing.T) {
	tests := []struct {
		name  string
		kind  Kind
		init  Values
		shift bool
		want  []Part
	}{
		{"jpeg forward", JPEG, Values{Quality: 50}, false, []Part{PartSave, PartCancel, PartSlider, PartSave}},
		{"jpeg backward", JPEG, Values{Quality: 50}, true, []Part{PartCancel, PartSave, PartSlider, PartCancel}},
		{"webp forward", WebP, Values{Quality: 50}, false, []Part{PartSlider, PartSave, PartCancel, PartCheck, PartSlider}},
		{"webp backward", WebP, Values{Quality: 50}, true, []Part{PartCancel, PartSave, PartSlider, PartCheck, PartCancel}},
		{"jpeg lossless forward", JPEGLossless, Values{Quality: 50}, false, []Part{PartSlider, PartSave, PartCancel, PartCheck, PartSlider}},
		{"jpeg lossless on skips the slider", JPEGLossless, Values{Quality: 50, Lossless: true}, false, []Part{PartSave, PartCancel, PartCheck, PartSave}},
		{"lossless skips the slider forward", WebP, Values{Quality: 50, Lossless: true}, false, []Part{PartSave, PartCancel, PartCheck, PartSave}},
		{"lossless skips the slider backward", WebP, Values{Quality: 50, Lossless: true}, true, []Part{PartCancel, PartSave, PartCheck, PartCancel}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t, tc.kind, tc.init, 760, 420)
			var mod widget.KeyMod
			if tc.shift {
				mod = widget.ModShift
			}

			var got []Part
			for range tc.want {
				r.key(widget.KeyTab, mod)
				got = append(got, r.d.Focused())
			}

			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Fatalf("focus order %v, want %v", got, tc.want)
				}
			}
		})
	}
}

func TestLosslessSwitchesTheQualitySliderOff(t *testing.T) {
	r := newRig(t, WebP, Values{Quality: 70}, 760, 420)
	if !r.d.Enabled(PartSlider) {
		t.Fatal("the slider starts enabled")
	}

	r.press(widget.KeySpace)
	r.press(widget.KeyEnd)
	r.clickQuality(10)

	if r.d.Enabled(PartSlider) {
		t.Fatal("the slider must be off in lossless mode")
	}
	if got := r.d.Values(); got != (Values{Quality: 70, Lossless: true}) {
		t.Fatalf("values = %+v: quality must be left alone while lossless is on", got)
	}
}

func TestSwitchingLosslessBackReturnsTheQualityControl(t *testing.T) {
	r := newRig(t, WebP, Values{Quality: 70, Lossless: true}, 760, 420)
	if r.d.Enabled(PartSlider) {
		t.Fatal("a lossless dialog starts with the slider off")
	}

	r.clickPart(PartCheck)
	r.clickQuality(20)

	if !r.d.Enabled(PartSlider) || r.d.Values() != (Values{Quality: 20}) {
		t.Fatalf("values = %+v, enabled %v", r.d.Values(), r.d.Enabled(PartSlider))
	}
}

func TestClickingTheLosslessLabelTogglesIt(t *testing.T) {
	r := newRig(t, WebP, Values{Quality: 70}, 760, 420)
	row := r.d.Rect(PartCheck)

	r.click(row.Max.X-3, (row.Min.Y+row.Max.Y)/2)

	if !r.d.Values().Lossless {
		t.Fatal("the label is part of the control")
	}
}

func TestTogglingLosslessWithTheSliderFocusedMovesFocusToTheCheckbox(t *testing.T) {
	r := newRig(t, WebP, Values{Quality: 70}, 760, 420)
	r.press(widget.KeyTab)
	if r.d.Focused() != PartSlider {
		t.Fatalf("focus %v", r.d.Focused())
	}

	r.clickPart(PartCheck)
	r.press(widget.KeyRight)

	if r.d.Focused() != PartCheck || r.d.Values().Quality != 70 {
		t.Fatalf("focus %v quality %d", r.d.Focused(), r.d.Values().Quality)
	}
}

func TestLosslessIsNeverReportedForJPEG(t *testing.T) {
	r := newRig(t, JPEG, Values{Quality: 70}, 760, 420)
	r.press(widget.KeyTab)
	r.press(widget.KeyTab)
	r.press(widget.KeyTab)
	r.press(widget.KeySpace)
	r.press(widget.KeyEnter)

	if r.last().values.Lossless {
		t.Fatal("JPEG has no lossless mode")
	}
}

func TestResultIsDeliveredOnlyOnce(t *testing.T) {
	r := newRig(t, JPEG, Values{Quality: 70}, 760, 420)

	r.press(widget.KeyEnter)
	r.press(widget.KeyEnter)
	r.clickPart(PartSave)
	r.press(widget.KeyEscape)
	r.d.cancel()
	r.d.save()

	if len(r.results) != 1 {
		t.Fatalf("%d results: %+v", len(r.results), r.results)
	}
}

func TestADialogWithoutACallbackStillCloses(t *testing.T) {
	r := newRig(t, JPEG, Values{Quality: 70}, 760, 420)
	r.d.result = nil

	r.press(widget.KeyEnter)

	if !r.d.Done() {
		t.Fatal("the dialog must close without a result handler")
	}
}

func TestAccessibilityReportsValuesAndStates(t *testing.T) {
	r := newRig(t, WebP, Values{Quality: 64, Lossless: true}, 760, 420)

	var check, slider widget.AccessInfo
	for _, a := range r.d.form.AccessChildren() {
		switch a.Role {
		case widget.RoleCheckBox:
			check = a
		case widget.RoleSlider:
			slider = a
		}
	}

	if !has(check.States, widget.StateChecked) || !has(check.States, widget.StateFocused) {
		t.Errorf("check states %v", check.States)
	}
	if slider.Value != "64" || !has(slider.States, widget.StateDisabled) || slider.Name != i18n.T("save.quality") {
		t.Errorf("slider %+v", slider)
	}
	if info := r.d.form.AccessInfo(); info.Role != widget.RoleGroup || info.Name != i18n.T("save.options.title") {
		t.Errorf("group %+v", info)
	}
}

func has(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

type noFocusManager struct{ released int }

func (n *noFocusManager) SetCapture(widget.Widget) {}
func (n *noFocusManager) ReleaseCapture()          { n.released++ }

func TestAnEngineWithoutFocusSupportIsTolerated(t *testing.T) {
	r := newRig(t, JPEG, Values{Quality: 70}, 760, 420)
	f := newForm(JPEG, compute(JPEG, 0, 0), Values{Quality: 70})
	cm := &noFocusManager{}

	f.SetCaptureManager(cm)
	f.SetCaptureManager(cm)
	f.SetBounds(image.Rect(0, 0, 400, 300))
	f.OnMouseButton(widget.MouseEvent{Button: widget.MouseLeft, Pressed: false})

	if cm.released != 1 {
		t.Fatalf("capture released %d times", cm.released)
	}
	_ = r
}

func TestSecondaryMouseButtonsAreIgnored(t *testing.T) {
	r := newRig(t, JPEG, Values{Quality: 70}, 760, 420)
	x, y := centre(r.d.Rect(PartSave))

	r.eng.SendMouseButton(x, y, widget.MouseRight, true)
	r.eng.SendMouseButton(x, y, widget.MouseRight, false)

	if len(r.results) != 0 || r.d.Values().Quality != 70 {
		t.Fatalf("results %+v quality %d", r.results, r.d.Values().Quality)
	}
}

func TestHoverTracksTheControlUnderTheCursor(t *testing.T) {
	r := newRig(t, WebP, Values{Quality: 70}, 760, 420)
	for _, p := range []Part{PartCheck, PartSlider, PartSave, PartCancel} {
		x, y := centre(r.d.Rect(p))
		r.eng.SendMouseMove(x, y)
		if r.d.form.hover != p {
			t.Errorf("cursor over %v: hover is %v", p, r.d.form.hover)
		}
	}
	r.eng.SendMouseMove(2, 2)
	if r.d.form.hover != PartNone {
		t.Errorf("hover after leaving: %v", r.d.form.hover)
	}
	r.d.form.OnMouseMove(widget.CursorNowhere, widget.CursorNowhere)
	if r.d.form.hover != PartNone {
		t.Errorf("hover after the cursor left the window: %v", r.d.form.hover)
	}
}

func TestPressAndReleaseRedrawTheDialog(t *testing.T) {
	r := newRig(t, JPEG, Values{Quality: 70}, 760, 420)
	before := r.render()
	x, y := centre(r.d.Rect(PartSave))
	r.eng.SendMouseMove(x, y)
	hover := r.render()
	r.eng.SendMouseButton(x, y, widget.MouseLeft, true)
	pressed := r.render()

	area := r.d.Rect(PartSave)
	if sameImage(before, hover, area) || sameImage(hover, pressed, area) {
		t.Fatal("hover and press must change how the Save button looks")
	}
}

func TestChangingTheValueRedrawsTheSliderAndTheNumber(t *testing.T) {
	r := newRig(t, JPEG, Values{Quality: 70}, 760, 420)
	before := r.render()

	r.press(widget.KeyHome)
	after := r.render()

	if sameImage(before, after, r.d.Rect(PartSlider)) {
		t.Error("the slider did not move")
	}
	number := r.d.form.lay.value.Add(r.d.form.Bounds().Min)
	if sameImage(before, after, number) {
		t.Error("the number did not change")
	}
}

func TestLosslessRedrawsTheCheckboxAndDimsTheSlider(t *testing.T) {
	r := newRig(t, WebP, Values{Quality: 70}, 760, 420)
	before := r.render()

	r.press(widget.KeySpace)
	after := r.render()

	if sameImage(before, after, r.d.Rect(PartCheck)) || sameImage(before, after, r.d.Rect(PartSlider)) {
		t.Fatal("the checkbox must look checked and the slider must look off")
	}
}

func TestFocusRingAppearsOnlyAfterTheKeyboardWasUsed(t *testing.T) {
	r := newRig(t, JPEG, Values{Quality: 70}, 760, 420)
	slider := r.d.Rect(PartSlider)
	area := slider.Inset(-6)
	fresh := r.render()

	r.press(widget.KeyF1)
	ringed := r.render()
	r.click(r.trackX(70), slider.Min.Y+slider.Dy()/2)
	r.eng.SendMouseMove(2, 2)
	clicked := r.render()

	if sameImage(fresh, ringed, area) {
		t.Fatal("no ring after a key press")
	}
	if !sameImage(fresh, clicked, area) {
		t.Fatal("clicking must hide the keyboard focus ring again")
	}
}

func TestOnlyTheDialogAreaChangesBetweenFrames(t *testing.T) {
	r := newRig(t, JPEG, Values{Quality: 70}, 760, 420)
	before := r.render()

	r.press(widget.KeyEnd)
	after := r.render()

	dlg := r.d.Widget().Bounds().Inset(-8)
	for y := 0; y < before.Bounds().Dy(); y++ {
		for x := 0; x < before.Bounds().Dx(); x++ {
			if image.Pt(x, y).In(dlg) {
				continue
			}
			if before.RGBAAt(x, y) != after.RGBAAt(x, y) {
				t.Fatalf("pixel (%d,%d) outside the dialog changed", x, y)
			}
		}
	}
}

func TestHintExplainsTheModeOfTheFormat(t *testing.T) {
	tests := []struct {
		kind Kind
		key  string
	}{{JPEG, "save.options.jpegHint"}, {WebP, "save.options.webpHint"}, {JPEGLossless, "save.options.jpegLosslessHint"}}
	for _, lang := range langs {
		for _, tc := range tests {
			t.Run(lang+"/"+kindName(tc.kind), func(t *testing.T) {
				withScale(t, 1.2, lang)
				r := newRig(t, tc.kind, Values{Quality: 70}, 760, 420)

				got := strings.Join(r.d.HintLines(), " ")

				if got != i18n.T(tc.key) {
					t.Fatalf("hint %q, want %q", got, i18n.T(tc.key))
				}
			})
		}
	}
}

func TestDialogIsCentredOnTheCanvas(t *testing.T) {
	r := newRig(t, JPEG, Values{Quality: 70}, 760, 420)

	b := r.d.Widget().Bounds()

	if dx := (760 - b.Dx()) / 2; abs(b.Min.X-dx) > 1 {
		t.Errorf("x = %d, want about %d", b.Min.X, dx)
	}
	if dy := (420 - b.Dy()) / 2; abs(b.Min.Y-dy) > 1 {
		t.Errorf("y = %d, want about %d", b.Min.Y, dy)
	}
	if got := r.d.Size(); got != b.Size() {
		t.Errorf("Size() = %v, want %v", got, b.Size())
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func TestMovingTheDialogKeepsTheControlsWorking(t *testing.T) {
	r := newRig(t, JPEG, Values{Quality: 70}, 760, 420)
	b := r.d.Widget().Bounds()
	widget.ShiftWidget(r.d.Widget(), 30, -20)
	r.eng.Flush()

	if got := r.d.Widget().Bounds(); got.Min.X != b.Min.X+30 || got.Min.Y != b.Min.Y-20 {
		t.Fatalf("bounds %v after the move from %v", got, b)
	}
	r.clickQuality(15)
	r.clickPart(PartSave)

	if len(r.results) != 1 || r.last().values.Quality != 15 {
		t.Fatalf("results %+v", r.results)
	}
}

func TestJPEGLosslessDialogReportsTheSwitchAndKeepsTheQuality(t *testing.T) {
	r := newRig(t, JPEGLossless, Values{Quality: 64, Lossless: true}, 760, 420)
	if r.d.Kind() != JPEGLossless || r.d.Enabled(PartSlider) || !r.d.Enabled(PartCheck) {
		t.Fatalf("kind %v, slider enabled %v, check enabled %v", r.d.Kind(), r.d.Enabled(PartSlider), r.d.Enabled(PartCheck))
	}

	r.press(widget.KeyEnter)

	if got := r.last(); !got.ok || got.values != (Values{Quality: 64, Lossless: true}) {
		t.Fatalf("result %+v", got)
	}
}

func TestJPEGLosslessSwitchedOffGivesBackTheSliderAndTheChosenQuality(t *testing.T) {
	r := newRig(t, JPEGLossless, Values{Quality: 64, Lossless: true}, 760, 420)

	r.press(widget.KeySpace)
	r.press(widget.KeyTab)
	r.press(widget.KeyHome)
	r.press(widget.KeyRight)
	r.press(widget.KeyEnter)

	if got := r.last(); !got.ok || got.values != (Values{Quality: 2}) {
		t.Fatalf("result %+v", got)
	}
}

func TestJPEGLosslessHintMentionsTheEdgeInBothLanguages(t *testing.T) {
	for _, lang := range langs {
		withScale(t, 1, lang)
		r := newRig(t, JPEGLossless, Values{Quality: 70, Lossless: true}, 760, 420)

		hint := strings.Join(r.d.HintLines(), " ")

		if !strings.Contains(hint, "15") {
			t.Errorf("%s hint %q does not name the 15 px limit", lang, hint)
		}
	}
}
