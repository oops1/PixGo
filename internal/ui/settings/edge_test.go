package settings

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/ui/theme"
)

func TestScrollbarThumbIsPaintedWhileShownAndGoneAfterwards(t *testing.T) {
	r := newRig(t, mockW, mockH)
	r.page.OnMouseWheelPixelsMod(300, 200, 0, 40, 0)
	_, thumb, ok := r.page.barRects()
	if !ok {
		t.Fatal("the section overflows")
	}
	mid := image.Pt((thumb.Min.X+thumb.Max.X)/2, (thumb.Min.Y+thumb.Max.Y)/2)
	bg := r.th.Page
	if r.render().RGBAAt(mid.X, mid.Y) == bg {
		t.Fatal("the shown thumb must be painted")
	}
	r.tm.advance(barHideDelay)
	if got := r.render().RGBAAt(mid.X, mid.Y); got != bg {
		t.Fatalf("the hidden bar leaves the page colour, got %v", got)
	}
	r.page.drag = &dragState{bar: true}
	r.page.bar.shown = true
	_, wide, _ := r.page.barRects()
	if wide.Dx() != dim.barWide {
		t.Fatalf("a held thumb is wide: %v", wide)
	}
	r.page.drag = nil
	r.page.hover = hit{kind: hitBar}
	r.render()
	r.page.hover = noHit
}

func TestTabFromAnUnselectedGroupMemberContinuesAfterTheGroup(t *testing.T) {
	r := newRig(t, mockW, mockH)
	r.focusOn("theme:dark")
	r.key(widget.KeyTab, 0)
	if r.page.focus != "language" {
		t.Fatalf("Tab from a group member = %q, want the control after the group", r.page.focus)
	}
	r.focusOn("theme:dark")
	r.key(widget.KeyTab, widget.ModShift)
	if r.page.focus != focusNav {
		t.Fatalf("Shift+Tab from a group member = %q, want the navigation", r.page.focus)
	}
}

func TestPageWorksWithoutThemeAndTimerHooks(t *testing.T) {
	r := newRig(t, mockW, mockH)
	r.page.deps.Theme = nil
	r.page.deps.After = nil
	r.page.OnMouseWheelPixelsMod(300, 200, 0, 40, 0)
	r.render()
	r.page.deps.Theme = func() *theme.Theme { return nil }
	r.render()
	r.page.SetSection(SectionPerformance)
	r.clickItem("clearThumbCache")
	if !r.page.noted {
		t.Fatal("the note still appears without a timer service")
	}
}

func TestUnknownKeysOnControlsFallThroughToScrolling(t *testing.T) {
	r := newRig(t, mockW, mockH)
	r.page.SetSection(SectionSlideshow)
	r.focusOn("slideshow.interval")
	if r.key(widget.KeyDelete, 0) {
		t.Fatal("Delete means nothing to a slider")
	}
	r.focusOn("slideshow.loop")
	if r.key(widget.KeyDelete, 0) {
		t.Fatal("Delete means nothing to a checkbox")
	}
	r.page.SetSection(SectionAbout)
	r.focusOn("checkUpdates")
	if r.key(widget.KeyDelete, 0) {
		t.Fatal("Delete means nothing to a button")
	}
	r.focusOn(focusBack)
	if r.key(widget.KeyLeft, 0) {
		t.Fatal("arrows mean nothing to the back button")
	}
	r.focusOn(focusNav)
	if r.key(widget.KeyDelete, 0) {
		t.Fatal("Delete means nothing to the navigation")
	}
	r.page.SetSection(SectionGeneral)
	r.focusOn("language")
	if r.key(widget.KeyDelete, 0) {
		t.Fatal("Delete means nothing to a dropdown")
	}
	r.focusOn("theme:system")
	if r.key(widget.KeyDelete, 0) {
		t.Fatal("Delete means nothing to a card group")
	}
}

func TestLanguageThatIsNotInTheListIsShownByItsCode(t *testing.T) {
	r := newRig(t, mockW, mockH)
	r.src.cfg.Language = "de"
	r.page.Refresh()
	if got := r.item("language").text; got != "de" {
		t.Fatalf("the field shows %q", got)
	}
}

func TestDropdownWithoutAnActiveChoiceShowsTheFirstOne(t *testing.T) {
	r := newRig(t, mockW, mockH)
	r.src.cfg.InitialZoom = "bogus"
	r.page.SetSection(SectionView)
	r.page.Refresh()
	if got := r.item("initialZoom").text; got != "По размеру окна" {
		t.Fatalf("the field shows %q", got)
	}
	if got := selectedText(nil); got != "" {
		t.Fatalf("no choices, no text: %q", got)
	}
}

func TestSliderOutsideItsRangeIsClamped(t *testing.T) {
	r := newRig(t, mockW, mockH)
	r.page.SetSection(SectionSlideshow)
	it := r.item("slideshow.interval")
	it.value = 500
	x0, x1, _ := sliderSpan(it)
	if got := sliderThumbX(it, x0, x1); got != float64(x1) {
		t.Fatalf("thumb at %v, want the right end %d", got, x1)
	}
	flat := &item{value: 3, lo: 3, hi: 3}
	if sliderThumbX(flat, x0, x1) != float64(x0) {
		t.Fatal("a flat slider keeps its thumb at the start")
	}
	if sliderValueAt(flat, x1) != 3 {
		t.Fatal("a flat slider has one value")
	}
	r.page.dragSlider(x0)
	r.page.drag = &dragState{id: "missing"}
	r.page.dragSlider(x0)
	r.page.drag = nil
	r.page.stepSlider(&item{}, 1)
	r.page.setSliderValue(&item{}, 3)
}

func TestRevealKeepsVisibleItemsInPlace(t *testing.T) {
	r := newRig(t, mockW, mockH)
	r.page.reveal(r.item("rememberWindow").r)
	if r.page.scroll != 0 {
		t.Fatalf("scroll = %d for a visible item", r.page.scroll)
	}
	r.page.reveal(r.item("updatesCheck").r)
	down := r.page.scroll
	if down == 0 {
		t.Fatal("an item below the fold scrolls into view")
	}
	r.page.reveal(r.item("theme:system").r)
	if r.page.scroll >= down {
		t.Fatal("an item above scrolls back")
	}
}

func TestFocusSurvivesARefreshAndDropsWhenTheItemIsGone(t *testing.T) {
	r := newRig(t, mockW, mockH)
	r.focusOn("confirmDelete")
	r.page.Refresh()
	if r.page.focus != "confirmDelete" {
		t.Fatalf("focus = %q after a refresh", r.page.focus)
	}
	r.page.SetSection(SectionAbout)
	if r.page.focus == "confirmDelete" {
		t.Fatal("a control of another section cannot keep the focus")
	}
	r.page.focus = "ghost"
	r.page.Refresh()
	if r.page.focus != "" {
		t.Fatalf("focus = %q for a vanished control", r.page.focus)
	}
}

func TestFormatsWithoutAnyRowsStillRender(t *testing.T) {
	src := newSource()
	src.formats = nil
	r := newRigWith(t, src, mockW, mockH)
	r.page.SetSection(SectionFormats)
	r.render()
	r = newRigWith(t, func() *fakeSource {
		s := newSource()
		s.about = About{}
		s.langs = nil
		return s
	}(), mockW, mockH)
	r.page.SetSection(SectionAbout)
	r.render()
	r.page.SetSection(SectionLanguage)
	r.render()
}

func TestSettingsClonePerChangeDoesNotLeakIntoTheSnapshot(t *testing.T) {
	r := newRig(t, mockW, mockH)
	before := r.page.cfg
	r.page.SetSection(SectionFormats)
	r.clickItem("format:avif")
	if !before.Formats["avif"].Enabled {
		t.Fatal("the page must not mutate the snapshot it was given")
	}
}
