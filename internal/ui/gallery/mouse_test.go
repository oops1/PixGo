package gallery

import (
	"errors"
	"image"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/widget"

	model "github.com/oops1/PixGo/internal/gallery"
)

func TestHeaderButtonsInvokeActionsOnRelease(t *testing.T) {
	cases := []struct {
		name string
		at   image.Point
		want string
	}{
		{"back", image.Pt(25, 22), "back"},
		{"up", image.Pt(77, 22), "up"},
		{"refresh", image.Pt(368, 22), "refresh"},
		{"list view", image.Pt(406, 22), "mode:1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			src := mockSource()
			src.mode = ViewGrid
			r := newRig(t, src, mockW, mockH)

			r.page.OnMouseMove(c.at.X, c.at.Y)
			r.page.OnMouseButton(widget.MouseEvent{X: c.at.X, Y: c.at.Y, Button: widget.MouseLeft, Pressed: true})
			if len(r.rec.calls) != 0 {
				t.Fatalf("action ran on press: %v", r.rec.calls)
			}
			r.page.OnMouseButton(widget.MouseEvent{X: c.at.X, Y: c.at.Y, Button: widget.MouseLeft, Pressed: false})

			if !r.rec.has(c.want) {
				t.Fatalf("calls = %v, want %q", r.rec.calls, c.want)
			}
		})
	}
}

func TestDisabledHeaderButtonsAreIgnored(t *testing.T) {
	src := mockSource()
	src.canBack, src.canForward, src.canUp = false, false, false
	r := newRig(t, src, mockW, mockH)

	r.click(25, 22)
	r.click(51, 22)
	r.click(77, 22)

	if len(r.rec.calls) != 0 {
		t.Fatalf("calls = %v", r.rec.calls)
	}
}

func TestForwardButtonRunsWhenEnabled(t *testing.T) {
	src := mockSource()
	src.canForward = true
	r := newRig(t, src, mockW, mockH)

	r.click(51, 22)

	if !r.rec.has("forward") {
		t.Fatalf("calls = %v", r.rec.calls)
	}
}

func TestReleasingOutsideThePressedButtonDoesNothing(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)

	r.page.OnMouseButton(widget.MouseEvent{X: 25, Y: 22, Button: widget.MouseLeft, Pressed: true})
	r.page.OnMouseButton(widget.MouseEvent{X: 200, Y: 300, Button: widget.MouseLeft, Pressed: false})

	if len(r.rec.calls) != 0 {
		t.Fatalf("calls = %v", r.rec.calls)
	}
}

func TestViewModeToggleSkipsTheActiveMode(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)

	r.click(434, 22)

	if len(r.rec.calls) != 0 {
		t.Fatalf("grid is already active, calls = %v", r.rec.calls)
	}
}

func TestClickingATileSelectsIt(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)
	x, y := r.tileCenter(5)

	r.click(x, y)

	if !r.rec.has("select:5") {
		t.Fatalf("calls = %v", r.rec.calls)
	}
	if r.page.Selected() != 5 {
		t.Fatalf("selected = %d", r.page.Selected())
	}
}

func TestClickingTheSelectedTileDoesNotReselect(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)
	x, y := r.tileCenter(0)

	r.click(x, y)

	if r.rec.count("select:0") != 0 {
		t.Fatalf("calls = %v", r.rec.calls)
	}
}

func TestDoubleClickOpensTheTile(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)
	x, y := r.tileCenter(3)

	r.doubleClick(x, y)

	if !r.rec.has("open:3") {
		t.Fatalf("calls = %v", r.rec.calls)
	}
	if r.rec.count("open:3") != 1 {
		t.Fatalf("opened %d times", r.rec.count("open:3"))
	}
}

func TestDoubleClickIsDetectedWithoutEngineClickCounts(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)
	x, y := r.tileCenter(2)
	press := func() {
		r.page.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft, Pressed: true})
		r.page.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft, Pressed: false})
	}

	press()
	r.now = r.now.Add(200 * time.Millisecond)
	press()

	if !r.rec.has("open:2") {
		t.Fatalf("calls = %v", r.rec.calls)
	}
}

func TestSlowSecondClickIsNotADoubleClick(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)
	x, y := r.tileCenter(2)
	press := func() {
		r.page.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft, Pressed: true})
		r.page.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft, Pressed: false})
	}

	press()
	r.now = r.now.Add(2 * time.Second)
	press()

	if r.rec.count("open:2") != 0 {
		t.Fatalf("calls = %v", r.rec.calls)
	}
}

func TestClickingBlankSpaceClearsTheSelection(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)

	r.click(400, 375)

	if !r.rec.has("select:-1") {
		t.Fatalf("calls = %v", r.rec.calls)
	}
}

func TestClickingBlankSpaceWithoutSelectionDoesNothing(t *testing.T) {
	src := mockSource()
	src.selected = -1
	r := newRig(t, src, mockW, mockH)

	r.click(400, 375)

	if len(r.rec.calls) != 0 {
		t.Fatalf("calls = %v", r.rec.calls)
	}
}

func TestSidebarEntriesNavigate(t *testing.T) {
	cases := []struct {
		name string
		y    int
		want string
	}{
		{"all images", 65, "navigate:" + mockSidebar()[0].Loc.Display()},
		{"downloads", 252, "navigate:C:\\Users\\u\\Downloads"},
		{"system drive", 308, "navigate:C:\\"},
		{"selected drive", 331, "navigate:D:\\"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newRig(t, mockSource(), mockW, mockH)

			r.click(60, c.y)

			if !r.rec.has(c.want) {
				t.Fatalf("calls = %v, want %q", r.rec.calls, c.want)
			}
		})
	}
}

func TestSidebarHeadersAndDisabledEntriesDoNotNavigate(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)

	r.click(30, 141)
	r.click(60, 283)
	r.click(60, 357)

	if len(r.rec.calls) != 0 {
		t.Fatalf("calls = %v", r.rec.calls)
	}
}

func TestSideButtonsGoBackAndForward(t *testing.T) {
	src := mockSource()
	src.canForward = true
	r := newRig(t, src, mockW, mockH)

	handledBack := r.page.OnMouseButton(widget.MouseEvent{X: 300, Y: 200, Button: widget.MouseButtonX1, Pressed: true})
	handledForward := r.page.OnMouseButton(widget.MouseEvent{X: 300, Y: 200, Button: widget.MouseButtonX2, Pressed: true})
	r.page.OnMouseButton(widget.MouseEvent{X: 300, Y: 200, Button: widget.MouseButtonX1, Pressed: false})

	if !handledBack || !handledForward {
		t.Fatalf("handled back=%v forward=%v", handledBack, handledForward)
	}
	if r.rec.count("back") != 1 || r.rec.count("forward") != 1 {
		t.Fatalf("calls = %v", r.rec.calls)
	}
}

func TestSideButtonsRespectHistoryAvailability(t *testing.T) {
	src := mockSource()
	src.canBack, src.canForward = false, false
	r := newRig(t, src, mockW, mockH)

	r.page.OnMouseButton(widget.MouseEvent{X: 300, Y: 200, Button: widget.MouseButtonX1, Pressed: true})
	r.page.OnMouseButton(widget.MouseEvent{X: 300, Y: 200, Button: widget.MouseButtonX2, Pressed: true})

	if len(r.rec.calls) != 0 {
		t.Fatalf("calls = %v", r.rec.calls)
	}
}

func TestOpenButtonOpensTheSelection(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)

	r.click(496, 400)

	if !r.rec.has("open:0") {
		t.Fatalf("calls = %v", r.rec.calls)
	}
}

func TestOpenButtonIsInertWithoutSelection(t *testing.T) {
	src := mockSource()
	src.selected = -1
	r := newRig(t, src, mockW, mockH)

	r.click(496, 400)
	r.click(560, 400)

	if len(r.rec.calls) != 0 {
		t.Fatalf("calls = %v", r.rec.calls)
	}
}

func TestChromeOwnedPointsAreNotHandled(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)
	r.page.Chrome = func(x, y int) (widget.Cursor, bool) { return widget.CursorSizeNS, y < 5 }

	handled := r.page.OnMouseButton(widget.MouseEvent{X: 25, Y: 3, Button: widget.MouseLeft, Pressed: true})
	wants := r.page.WantsCapture(widget.MouseEvent{X: 25, Y: 3, Button: widget.MouseLeft, Pressed: true})

	if handled || wants {
		t.Fatalf("handled=%v wantsCapture=%v for a chrome point", handled, wants)
	}
	if got := r.page.Cursor(25, 3); got != widget.CursorSizeNS {
		t.Fatalf("cursor = %v", got)
	}
	if r.page.OnMouseWheelPixelsMod(25, 3, 0, 40, 0) {
		t.Fatal("wheel over chrome must be ignored")
	}
}

func TestCursorIsAnArrowOverTilesAndIBeamOverTheAddress(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)
	x, y := r.tileCenter(1)

	if got := r.page.Cursor(x, y); got != widget.CursorArrow {
		t.Errorf("tiles: %v", got)
	}
	if got := r.page.Cursor(200, 22); got != widget.CursorIBeam {
		t.Errorf("address: %v", got)
	}
}

func TestPageCapturesPressesInsideItself(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)

	if !r.page.WantsCapture(widget.MouseEvent{X: 300, Y: 200, Button: widget.MouseLeft, Pressed: true}) {
		t.Error("left press inside must be captured")
	}
	if r.page.WantsCapture(widget.MouseEvent{X: 300, Y: 200, Button: widget.MouseRight, Pressed: true}) {
		t.Error("right press must not be captured")
	}
	if r.page.WantsCapture(widget.MouseEvent{X: 900, Y: 900, Button: widget.MouseLeft, Pressed: true}) {
		t.Error("press outside must not be captured")
	}
}

func TestHoverLeavingClearsTheHighlight(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)
	x, y := r.tileCenter(4)
	r.page.OnMouseMove(x, y)
	if r.page.hover.kind != hitTile || r.page.hover.index != 4 {
		t.Fatalf("hover = %+v", r.page.hover)
	}

	r.page.OnMouseMove(widget.CursorNowhere, widget.CursorNowhere)

	if r.page.hover != noHit {
		t.Fatalf("hover = %+v", r.page.hover)
	}
}

func TestRightClickSelectsAndShowsTheItemMenu(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)
	x, y := r.tileCenter(6)

	r.rightClick(x, y)

	if !r.rec.has("select:6") || !r.rec.has("menu") {
		t.Fatalf("calls = %v", r.rec.calls)
	}
	got := menuTexts(r.rec.menuItems)
	want := []string{"Открыть", "Открыть в новом окне", "Показать в папке", "", "Добавить в избранное", "", "Удалить"}
	if !equalStrings(got, want) {
		t.Fatalf("menu = %q, want %q", got, want)
	}
	if r.rec.menuAt != [2]int{x, y} {
		t.Fatalf("menu at %v, want %v", r.rec.menuAt, [2]int{x, y})
	}
}

func TestItemMenuOffersRemovingAFavourite(t *testing.T) {
	src := mockSource()
	src.favorites[mockPath("lake.jpg")] = true
	r := newRig(t, src, mockW, mockH)
	x, y := r.tileCenter(1)

	r.rightClick(x, y)

	if got := menuTexts(r.rec.menuItems)[4]; got != "Удалить из избранного" {
		t.Fatalf("favourite entry = %q", got)
	}
}

func TestItemMenuEntriesRunTheirActions(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)
	x, y := r.tileCenter(2)
	r.rightClick(x, y)
	items := r.rec.menuItems

	for _, i := range []int{0, 1, 2, 4, 6} {
		items[i].OnClick()
	}

	for _, want := range []string{"open:2", "openNew:2", "reveal:2", "favorite:2", "delete:2"} {
		if !r.rec.has(want) {
			t.Errorf("missing %q in %v", want, r.rec.calls)
		}
	}
}

func TestFolderMenuOnlyOffersOpen(t *testing.T) {
	src := mockSource()
	src.items = append([]model.Item{{Name: "Trips", Path: `D:\Photos\Trips`, Dir: true}}, src.items...)
	r := newRig(t, src, mockW, mockH)
	x, y := r.tileCenter(0)

	r.rightClick(x, y)

	if got := menuTexts(r.rec.menuItems); !equalStrings(got, []string{"Открыть"}) {
		t.Fatalf("menu = %q", got)
	}
}

func TestRightClickOnBlankSpaceShowsTheRefreshAndSettingsMenu(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)

	r.rightClick(400, 375)

	if !r.rec.has("menu") {
		t.Fatalf("calls = %v", r.rec.calls)
	}
	items := r.rec.menuItems
	if len(items) != 3 || items[0].Text != "Обновить" || !items[1].Separator || items[2].Text != "Настройки" {
		t.Fatalf("menu = %q", menuTexts(items))
	}
	if r.rec.menuAt != [2]int{400, 375} {
		t.Fatalf("menu at %v", r.rec.menuAt)
	}
	items[2].OnClick()
	items[0].OnClick()
	if !r.rec.has("settings") || !r.rec.has("refresh") {
		t.Fatalf("calls = %v", r.rec.calls)
	}
}

func TestRightClickOnTheHeaderShowsNoPageMenu(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)

	r.rightClick(60, 20)

	if r.rec.has("menu") {
		t.Fatalf("calls = %v", r.rec.calls)
	}
}

func TestPageMenuOmitsEntriesWithoutActions(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)
	r.page.deps.Actions.Settings = nil

	r.rightClick(400, 375)

	if got := menuTexts(r.rec.menuItems); !equalStrings(got, []string{"Обновить"}) {
		t.Fatalf("menu = %q", got)
	}
}

func TestFilterMenuListsFormatsAndMarksTheActiveOne(t *testing.T) {
	src := mockSource()
	src.format = "PNG"
	r := newRig(t, src, mockW, mockH)

	r.click(345, 22)

	got := menuTexts(r.rec.menuItems)
	if !equalStrings(got, []string{"Все форматы", "JPEG", "PNG"}) {
		t.Fatalf("menu = %q", got)
	}
	var checked []bool
	for _, it := range r.rec.menuItems {
		checked = append(checked, it.Checked)
	}
	if checked[0] || checked[1] || !checked[2] {
		t.Fatalf("checked = %v", checked)
	}
	if r.rec.menuAt != [2]int{r.page.geo.filter.Min.X, r.page.geo.address.Max.Y} {
		t.Fatalf("menu anchored at %v", r.rec.menuAt)
	}
}

func TestFilterMenuEntriesSetTheFormat(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)
	r.click(345, 22)

	r.rec.menuItems[1].OnClick()
	r.rec.menuItems[0].OnClick()

	if !r.rec.has("format:JPEG") || !r.rec.has("format:") {
		t.Fatalf("calls = %v", r.rec.calls)
	}
}

func TestActiveFilterIsMarkedOnTheFunnel(t *testing.T) {
	plain := newRig(t, mockSource(), mockW, mockH)
	src := mockSource()
	src.format = "JPEG"
	active := newRig(t, src, mockW, mockH)

	a, b := plain.render(), active.render()

	if string(a.Pix) == string(b.Pix) {
		t.Fatal("an active filter must change the funnel colour")
	}
}

func TestThumbnailSizeMenuMarksTheCurrentSize(t *testing.T) {
	src := mockSource()
	src.side = 128
	r := newRig(t, src, mockW, mockH)

	r.click(465, 22)

	got := menuTexts(r.rec.menuItems)
	want := []string{"96 px", "128 px", "160 px", "192 px", "256 px", "384 px"}
	if !equalStrings(got, want) {
		t.Fatalf("menu = %q", got)
	}
	for i, it := range r.rec.menuItems {
		if it.Checked != (i == 1) {
			t.Errorf("entry %d checked = %v", i, it.Checked)
		}
	}
	r.rec.menuItems[4].OnClick()
	if !r.rec.has("size:256") {
		t.Fatalf("calls = %v", r.rec.calls)
	}
}

func TestMoreButtonOpensTheOpenMenu(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)

	r.click(560, 400)

	got := menuTexts(r.rec.menuItems)
	if !equalStrings(got, []string{"Открыть", "Открыть в новом окне", "Показать в папке"}) {
		t.Fatalf("menu = %q", got)
	}
	r.rec.menuItems[1].OnClick()
	r.rec.menuItems[2].OnClick()
	if !r.rec.has("openNew:0") || !r.rec.has("reveal:0") {
		t.Fatalf("calls = %v", r.rec.calls)
	}
}

func TestNoMenuWithoutAShowMenuAction(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)
	r.page.deps.Actions.ShowMenu = nil

	r.click(345, 22)
	r.click(560, 400)
	r.rightClick(r.tileCenter(1))
}

func TestMissingActionsNeverPanic(t *testing.T) {
	src := mockSource()
	src.canForward = true
	r := newRig(t, src, mockW, mockH)
	r.page.deps.Actions = Actions{}
	x, y := r.tileCenter(1)

	r.click(25, 22)
	r.click(51, 22)
	r.click(77, 22)
	r.click(368, 22)
	r.click(406, 22)
	r.click(345, 22)
	r.click(465, 22)
	r.click(60, 65)
	r.click(x, y)
	r.doubleClick(x, y)
	r.rightClick(x, y)
	r.click(496, 400)
	r.click(560, 400)
	r.key(widget.KeyDelete, 0)
	r.key(widget.KeyD, widget.ModCtrl)
	r.key(widget.KeyF5, 0)
	r.key(widget.KeyEscape, 0)
	r.page.FocusAddress()
	r.key(widget.KeyEnter, 0)
	r.render()
}

func TestAddressClickStartsEditingWithTheTextSelected(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)

	r.click(200, 22)

	if !r.page.Editing() {
		t.Fatal("click on the address must start editing")
	}
	if got := r.page.input.GetText(); got != `D:\Photos\Nature` {
		t.Fatalf("text = %q", got)
	}
	if len(r.page.Children()) != 1 {
		t.Fatal("the input must be a child while editing")
	}
	r.render()
}

func TestRefreshButtonCommitsWhileEditing(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)
	r.page.FocusAddress()

	r.click(368, 22)

	if !r.rec.has("address") || r.rec.has("refresh") {
		t.Fatalf("calls = %v", r.rec.calls)
	}
	if r.page.Editing() {
		t.Fatal("a successful commit leaves the edit mode")
	}
}

func TestClickingElsewhereCancelsTheEditAndStillActs(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)
	r.page.FocusAddress()
	x, y := r.tileCenter(4)

	r.click(x, y)

	if r.page.Editing() {
		t.Fatal("edit must end")
	}
	if r.rec.has("address") || !r.rec.has("select:4") {
		t.Fatalf("calls = %v", r.rec.calls)
	}
}

func TestClickingInsideTheEditedBarKeepsEditing(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)
	r.page.FocusAddress()

	r.page.OnMouseButton(widget.MouseEvent{X: 96, Y: 9, Button: widget.MouseLeft, Pressed: true})
	r.page.OnMouseButton(widget.MouseEvent{X: 96, Y: 9, Button: widget.MouseLeft, Pressed: false})

	if !r.page.Editing() {
		t.Fatal("edit must continue")
	}
}

func TestWrongAddressKeepsEditingAndShowsTheError(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)
	r.rec.addrErr = &model.AddressError{Path: `D:\Nope`, Err: model.ErrNotFound}
	r.page.FocusAddress()

	r.key(widget.KeyEnter, 0)

	if !r.page.Editing() {
		t.Fatal("a failed address must keep the edit open")
	}
	want := `Не удаётся найти «D:\Nope»`
	if r.page.bubble != want || r.page.input.ValidationError() != want {
		t.Fatalf("bubble = %q, validation = %q", r.page.bubble, r.page.input.ValidationError())
	}
	r.render()
	if r.tm.live() == 0 {
		t.Fatal("the bubble must expire on a timer")
	}
	r.tm.advance(bubbleDelay)
	if r.page.bubble != "" {
		t.Fatal("the bubble must disappear")
	}
}

func TestTypingClearsTheAddressError(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)
	r.rec.addrErr = &model.AddressError{Path: "x", Err: model.ErrNotFound}
	r.page.FocusAddress()
	r.key(widget.KeyEnter, 0)

	r.page.input.OnKeyEvent(widget.KeyEvent{Code: widget.KeyUnknown, Rune: 'a', Pressed: true})

	if r.page.bubble != "" || r.page.input.ValidationError() != "" {
		t.Fatalf("bubble = %q", r.page.bubble)
	}
}

func TestEmptyAddressJustLeavesTheEditor(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)
	r.rec.addrErr = model.ErrEmpty
	r.page.FocusAddress()

	r.key(widget.KeyEnter, 0)

	if r.page.Editing() {
		t.Fatal("an empty address cancels the edit")
	}
}

func TestPlainErrorsUseTheGenericMessage(t *testing.T) {
	r := newRig(t, mockSource(), mockW, mockH)
	r.rec.addrErr = errors.New("boom")
	r.page.FocusAddress()

	r.key(widget.KeyEnter, 0)

	if r.page.bubble != "Не удалось выполнить операцию" {
		t.Fatalf("bubble = %q", r.page.bubble)
	}
}

func menuTexts(items []widget.MenuItem) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.Text
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
