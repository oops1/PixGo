package settings

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"

	data "github.com/oops1/PixGo/internal/settings"
)

func press(r *rig, x, y int) {
	r.page.OnMouseMove(x, y)
	r.page.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft, Pressed: true, Clicks: 1})
}

func release(r *rig, x, y int) {
	r.page.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft, Pressed: false, Clicks: 1})
}

func trackOf(r *rig, id string) (x0, x1, cy int) {
	r.page.reveal(r.item(id).r)
	it := r.item(id)
	x0, x1, cy = sliderSpan(it)
	return x0, x1, cy - r.page.scroll
}

func TestSliderJumpsToTheClickedPosition(t *testing.T) {
	r := newRig(t, mockW, mockH)
	r.page.SetSection(SectionSlideshow)
	x0, x1, cy := trackOf(r, "slideshow.interval")
	for _, tc := range []struct {
		x    int
		want int
	}{
		{x0, 1},
		{x1, 60},
		{x0 - 5, 1},
		{x1 + 5, 60},
		{(x0 + x1) / 2, 31},
	} {
		r.click(tc.x, cy)
		if got := r.src.cfg.Slideshow.Interval; got != tc.want {
			t.Errorf("click at %d: interval %d, want %d", tc.x, got, tc.want)
		}
	}
}

func TestSliderFollowsADragAndStopsOnRelease(t *testing.T) {
	r := newRig(t, mockW, mockH)
	r.page.SetSection(SectionSlideshow)
	x0, x1, cy := trackOf(r, "slideshow.interval")
	press(r, x0, cy)
	if r.page.drag == nil {
		t.Fatal("a press on the track starts a drag")
	}
	var seen []int
	for x := x0; x <= x1+30; x += 20 {
		r.page.OnMouseMove(x, cy+40)
		seen = append(seen, r.src.cfg.Slideshow.Interval)
	}
	if seen[len(seen)-1] != 60 {
		t.Fatalf("dragging past the end must clamp to 60, saw %v", seen)
	}
	for i := 1; i < len(seen); i++ {
		if seen[i] < seen[i-1] {
			t.Fatalf("the value must grow with the pointer: %v", seen)
		}
	}
	release(r, x1, cy)
	if r.page.drag != nil {
		t.Fatal("release ends the drag")
	}
	r.rec.reset()
	r.page.OnMouseMove(x0, cy)
	if r.rec.has("update") {
		t.Fatal("moving after the release must not change the value")
	}
}

func TestSliderStepsAreRespected(t *testing.T) {
	r := newRig(t, mockW, mockH)
	r.page.SetSection(SectionPerformance)
	x0, x1, cy := trackOf(r, "maxPixels")
	r.click(x0+(x1-x0)/3, cy)
	got := r.src.cfg.MaxPixels
	if (got-4)%4 != 0 || got <= 4 || got >= 1000 {
		t.Fatalf("maxPixels = %d, must sit on the 4-step grid inside the range", got)
	}
	x0, x1, cy = trackOf(r, "cacheMB")
	r.click(x1, cy)
	if r.src.cfg.CacheMB != 8192 {
		t.Fatalf("cache = %d", r.src.cfg.CacheMB)
	}
	r.click(x0, cy)
	if r.src.cfg.CacheMB != 256 {
		t.Fatalf("cache = %d", r.src.cfg.CacheMB)
	}
}

func TestSliderLabelsFollowTheValue(t *testing.T) {
	r := newRig(t, mockW, mockH)
	r.page.SetSection(SectionPerformance)
	x0, x1, cy := trackOf(r, "decodeThreads")
	if got := r.item("decodeThreads").valueText; got != "Автоматически" {
		t.Fatalf("zero threads = %q", got)
	}
	r.click(x1, cy)
	if got := r.item("decodeThreads").valueText; got != "64" {
		t.Fatalf("64 threads = %q", got)
	}
	r.click(x0, cy)
	if got := r.item("decodeThreads").valueText; got != "Автоматически" {
		t.Fatalf("back to automatic = %q", got)
	}
	_, x1, cy = trackOf(r, "animationFPS")
	r.click(x1, cy)
	if got := r.item("animationFPS").valueText; got != "60 кадр/с" {
		t.Fatalf("fps = %q", got)
	}
}

func TestClickOnTheSliderValueLabelDoesNothing(t *testing.T) {
	r := newRig(t, mockW, mockH)
	r.page.SetSection(SectionSlideshow)
	it := r.item("slideshow.interval")
	r.rec.reset()
	r.click(it.r.Max.X-10, it.r.Min.Y+10)
	if r.rec.has("update") {
		t.Fatal("the value label is not part of the slider")
	}
}

func TestPressAndReleaseOnDifferentControlsDoesNothing(t *testing.T) {
	r := newRig(t, mockW, mockH)
	x, y := r.center("rememberWindow")
	x2, y2 := r.center("confirmDelete")
	press(r, x, y)
	r.page.OnMouseMove(x2, y2)
	release(r, x2, y2)
	if r.rec.has("update") {
		t.Fatal("a press must be released over the same control to count")
	}
}

func TestNavigationClickSwitchesSectionAndResetsScroll(t *testing.T) {
	for s := Section(0); s < sectionCount; s++ {
		t.Run(sectionNames[s], func(t *testing.T) {
			r := newRig(t, mockW, mockH)
			base := SectionGeneral
			if s == base {
				base = SectionPerformance
			}
			r.page.SetSection(base)
			if r.page.setScroll(10); r.page.scroll == 0 {
				t.Fatal("the starting section must be scrollable")
			}
			row := navRowRect(s)
			r.click((row.Min.X+row.Max.X)/2, (row.Min.Y+row.Max.Y)/2)
			if r.page.Section() != s {
				t.Fatalf("section = %d, want %d", r.page.Section(), s)
			}
			if r.page.scroll != 0 {
				t.Fatalf("scroll = %d after switching, want 0", r.page.scroll)
			}
		})
	}
}

func TestBackButtonClickLeavesThePage(t *testing.T) {
	r := newRig(t, mockW, mockH)
	g := r.page.geo.back
	r.click((g.Min.X+g.Max.X)/2, (g.Min.Y+g.Max.Y)/2)
	if !r.rec.has("back") {
		t.Fatal("the arrow must leave the page")
	}
}

func TestHeaderHolesOnlyCoverTheBackButton(t *testing.T) {
	r := newRig(t, mockW, mockH)
	holes := r.page.HeaderHoles()
	if len(holes) != 1 || holes[0] != r.page.geo.back {
		t.Fatalf("holes = %v, want just the back button %v", holes, r.page.geo.back)
	}
	if holes[0].Max.X > mockW-110 || holes[0].Max.Y > headerHeight {
		t.Fatalf("the hole %v must stay left of the window buttons and inside the header", holes[0])
	}
	empty := New(nil, Deps{})
	if len(empty.HeaderHoles()) != 1 {
		t.Fatal("holes are available before the first layout too")
	}
}

func TestChromeOwnsItsPointsAndThePageStaysOutOfThem(t *testing.T) {
	r := newRig(t, mockW, mockH)
	owned := image.Rect(mockW-105, 0, mockW, headerHeight)
	r.page.Chrome = func(x, y int) (widget.Cursor, bool) {
		if in(owned, x, y) {
			return widget.CursorSizeWE, true
		}
		return widget.CursorArrow, false
	}
	x, y := mockW-50, 10
	if r.page.WantsCapture(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft, Pressed: true}) {
		t.Fatal("the page must not capture presses on the window buttons")
	}
	if r.page.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft, Pressed: true}) {
		t.Fatal("presses on the window buttons are not consumed")
	}
	if r.page.OnMouseWheelPixelsMod(x, y, 0, 40, 0) {
		t.Fatal("the wheel over chrome is not consumed")
	}
	if got := r.page.Cursor(x, y); got != widget.CursorSizeWE {
		t.Fatalf("cursor over chrome = %v", got)
	}
	r.page.OnMouseMove(x, y)
	if r.page.hover != noHit {
		t.Fatal("nothing is hovered under chrome")
	}
	if !r.page.WantsCapture(widget.MouseEvent{X: 200, Y: 200, Button: widget.MouseLeft, Pressed: true}) {
		t.Fatal("the page captures presses elsewhere")
	}
	if r.page.WantsCapture(widget.MouseEvent{X: 200, Y: 200, Button: widget.MouseRight, Pressed: true}) ||
		r.page.WantsCapture(widget.MouseEvent{X: 200, Y: 200, Button: widget.MouseLeft, Pressed: false}) ||
		r.page.WantsCapture(widget.MouseEvent{X: -5, Y: 200, Button: widget.MouseLeft, Pressed: true}) {
		t.Fatal("only left presses inside the page are captured")
	}
}

func TestCursorIsAHandOverCardsAndLinksOnly(t *testing.T) {
	r := newRig(t, mockW, mockH)
	x, y := r.center("theme:light")
	if got := r.page.Cursor(x, y); got != widget.CursorHand {
		t.Fatalf("cursor over a card = %v", got)
	}
	x, y = r.center("rememberWindow")
	if got := r.page.Cursor(x, y); got != widget.CursorArrow {
		t.Fatalf("cursor over a checkbox = %v", got)
	}
	if got := r.page.Cursor(200, 36); got != widget.CursorArrow {
		t.Fatalf("cursor over blank space = %v", got)
	}
	r.page.SetSection(SectionAbout)
	x, y = r.center("repository")
	if got := r.page.Cursor(x, y); got != widget.CursorHand {
		t.Fatalf("cursor over a link = %v", got)
	}
}

func TestHoverTracksThePointerAndClearsWhenItLeaves(t *testing.T) {
	r := newRig(t, mockW, mockH)
	x, y := r.center("theme:dark")
	r.page.OnMouseMove(x, y)
	if h := r.page.hover; h.kind != hitItem || r.page.items[h.index].id != "theme:dark" {
		t.Fatalf("hover = %+v", h)
	}
	r.page.OnMouseMove(widget.CursorNowhere, widget.CursorNowhere)
	if r.page.hover != noHit {
		t.Fatal("the pointer leaving the page clears the hover")
	}
	row := navRowRect(SectionView)
	r.page.OnMouseMove(row.Min.X+20, (row.Min.Y+row.Max.Y)/2)
	if r.page.hover != (hit{kind: hitNav, index: int(SectionView)}) {
		t.Fatalf("hover = %+v", r.page.hover)
	}
	g := r.page.geo.back
	r.page.OnMouseMove(g.Min.X+5, g.Min.Y+5)
	if r.page.hover.kind != hitBack {
		t.Fatalf("hover = %+v", r.page.hover)
	}
}

func TestHoverAndPressedStatesChangeThePixels(t *testing.T) {
	cases := []struct {
		section Section
		id      string
	}{
		{SectionGeneral, "theme:light"},
		{SectionGeneral, "language"},
		{SectionGeneral, "rememberWindow"},
		{SectionGeneral, "background:white"},
		{SectionView, "wheel:zoom"},
		{SectionView, "wheel:navigate"},
		{SectionFormats, "defaultApps"},
		{SectionFormats, "format:avif"},
		{SectionSlideshow, "slideshow.interval"},
		{SectionAbout, "repository"},
	}
	for _, c := range cases {
		t.Run(c.id, func(t *testing.T) {
			r := newRig(t, mockW, 600)
			r.page.SetSection(c.section)
			r.page.OnMouseMove(widget.CursorNowhere, widget.CursorNowhere)
			plain := r.render()
			x, y := r.center(c.id)
			r.page.OnMouseMove(x, y)
			hovered := r.render()
			region := r.item(c.id).r.Inset(-6)
			if sameImage(plain, hovered, region) {
				t.Fatal("hover must be visible")
			}
			press(r, x, y)
			pressed := r.render()
			if sameImage(hovered, pressed, region) && c.section != SectionAbout {
				t.Fatal("pressed must differ from hover")
			}
			release(r, x, y)
		})
	}
}

func TestNavigationAndBackHoverArePainted(t *testing.T) {
	r := newRig(t, mockW, mockH)
	plain := r.render()
	row := navRowRect(SectionFormats)
	r.page.OnMouseMove(row.Min.X+20, (row.Min.Y+row.Max.Y)/2)
	if sameImage(plain, r.render(), row) {
		t.Fatal("a hovered navigation row must be highlighted")
	}
	r.page.OnMouseMove(widget.CursorNowhere, widget.CursorNowhere)
	g := r.page.geo.back
	r.page.OnMouseMove((g.Min.X+g.Max.X)/2, (g.Min.Y+g.Max.Y)/2)
	hover := r.render()
	if sameImage(plain, hover, g) {
		t.Fatal("the hovered back button must be highlighted")
	}
	press(r, (g.Min.X+g.Max.X)/2, (g.Min.Y+g.Max.Y)/2)
	if sameImage(hover, r.render(), g) {
		t.Fatal("the pressed back button must look pressed")
	}
	release(r, (g.Min.X+g.Max.X)/2, (g.Min.Y+g.Max.Y)/2)
}

func TestLanguageChangeRelabelsTheWholePage(t *testing.T) {
	r := newRig(t, mockW, mockH)
	if r.item("rememberWindow").text != "Запоминать размер и положение окна" {
		t.Fatalf("text = %q", r.item("rememberWindow").text)
	}
	setLang("en")
	defer setLang("ru")
	r.page.ensureLayout()
	if got := r.item("rememberWindow").text; got != "Remember window size and position" {
		t.Fatalf("text = %q after switching to English", got)
	}
	if r.page.items[0].text != "General" {
		t.Fatalf("title = %q", r.page.items[0].text)
	}
}

func TestSettingsStoreShapeIsUsedDirectly(t *testing.T) {
	st := data.NewMemory(data.Defaults())
	src := newSource()
	r := newRigWith(t, src, mockW, mockH)
	r.page.deps.Actions.Update = func(change func(*data.Settings)) {
		st.Update(change)
		src.cfg = st.Get()
	}
	r.clickItem("theme:dark")
	if st.Get().Theme != data.ThemeDark || src.cfg.Theme != data.ThemeDark {
		t.Fatal("the change closure must work against the real settings store")
	}
	r.page.SetSection(SectionFormats)
	r.clickItem("format:avif")
	if st.Get().Formats["avif"].Enabled {
		t.Fatal("format switch must reach the store")
	}
	if !st.Get().Formats["jxl"].Enabled {
		t.Fatal("other formats stay untouched")
	}
}
