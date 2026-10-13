package settings

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/ui/gfx"
	"github.com/oops1/PixGo/internal/ui/theme"
)

func TestWrapLinesBreaksOnWordsAndEllipsizesTheTail(t *testing.T) {
	f := bodyFont
	for _, tc := range []struct {
		name  string
		text  string
		width int
		max   int
		lines int
	}{
		{"fits", "short text", 400, 3, 1},
		{"empty", "", 100, 3, 1},
		{"two lines", "Восстанавливать последнее изображение при запуске", 160, 3, 2},
		{"limit", "один два три четыре пять шесть семь восемь девять десять", 60, 2, 2},
		{"long word", "Сверхдлинноеслововбезпробелов", 60, 2, 1},
		{"zero limit", "a b c", 20, 0, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lines := wrapLines(f, tc.text, tc.width, tc.max)
			if len(lines) != tc.lines {
				t.Fatalf("lines = %q, want %d", lines, tc.lines)
			}
			for _, l := range lines {
				if measure(f, l) > tc.width {
					t.Errorf("line %q is %d wide, limit %d", l, measure(f, l), tc.width)
				}
			}
		})
	}
	last := wrapLines(f, "один два три четыре пять шесть семь восемь девять десять", 60, 2)
	if got := last[len(last)-1]; got[len(got)-len("…"):] != "…" {
		t.Fatalf("the cut tail must end with an ellipsis: %q", got)
	}
}

func TestFitTextKeepsShortTextAndCutsLongOnes(t *testing.T) {
	if got := fitText(bodyFont, "abc", 200); got != "abc" {
		t.Fatalf("got %q", got)
	}
	got := fitText(bodyFont, "очень длинная строка настроек", 60)
	if measure(bodyFont, got) > 60 || got[len(got)-len("…"):] != "…" {
		t.Fatalf("got %q (%d px)", got, measure(bodyFont, got))
	}
	if got := fitText(bodyFont, "abc", 0); got != "…" {
		t.Fatalf("nothing fits in zero width: %q", got)
	}
}

func TestDrawTextAddsWeightOnlyToConfiguredFonts(t *testing.T) {
	r := newRig(t, 200, 60)
	img := r.render()
	if img.Bounds().Dx() != 200 {
		t.Fatal("unexpected canvas")
	}
	for _, f := range []gfx.Font{bodyFont, navFont, headerFont, captionFont, smallCapFont, noteFont} {
		if textWeights[f] <= 0 || textWeights[f] >= 1 {
			t.Errorf("font %v has weight %v", f, textWeights[f])
		}
	}
	for _, f := range []gfx.Font{titleFont, headingFont, aboutFont} {
		if _, ok := textWeights[f]; ok {
			t.Errorf("semi-bold font %v must not be thickened", f)
		}
	}
}

func TestPicturesAreCachedAndRoundedAtTheCorners(t *testing.T) {
	r := newRig(t, mockW, mockH)
	th := theme.Dark()
	for _, tc := range []struct {
		pic  picture
		w, h int
	}{
		{picThemeSystem, dim.themePicW, dim.themePicH}, {picThemeLight, dim.themePicW, dim.themePicH}, {picThemeDark, dim.themePicW, dim.themePicH},
		{picBgSystem, dim.bgPicW, dim.bgPicH}, {picBgWhite, dim.bgPicW, dim.bgPicH}, {picBgBlack, dim.bgPicW, dim.bgPicH}, {picBgChecker, dim.bgPicW, dim.bgPicH},
	} {
		img := r.page.picture(tc.pic, th)
		if img.Bounds().Dx() != tc.w || img.Bounds().Dy() != tc.h {
			t.Errorf("picture %d is %v, want %dx%d", tc.pic, img.Bounds(), tc.w, tc.h)
		}
		if img.RGBAAt(0, 0).A > 40 || img.RGBAAt(tc.w-1, tc.h-1).A > 40 {
			t.Errorf("picture %d: the corners must be transparent, got %v and %v", tc.pic, img.RGBAAt(0, 0), img.RGBAAt(tc.w-1, tc.h-1))
		}
		if img.RGBAAt(tc.w/2, tc.h/2).A != 255 {
			t.Errorf("picture %d: the middle must be opaque", tc.pic)
		}
		if r.page.picture(tc.pic, th) != img {
			t.Errorf("picture %d must be cached", tc.pic)
		}
	}
	if r.page.picture(picBgChecker, theme.Light()) == r.page.picture(picBgChecker, th) {
		t.Fatal("the checker follows the theme colours")
	}
}

func TestThemePicturesAreHalfLightHalfDarkForTheSystemChoice(t *testing.T) {
	r := newRig(t, mockW, mockH)
	th := theme.Dark()
	lum := func(c color.RGBA) int { return int(c.R) + int(c.G) + int(c.B) }
	sys := r.page.picture(picThemeSystem, th)
	light := r.page.picture(picThemeLight, th)
	dark := r.page.picture(picThemeDark, th)
	at := func(img *image.RGBA, x int) int { return lum(img.RGBAAt(x, 8)) }
	if at(sys, 10) <= at(sys, 60)*2 {
		t.Fatalf("the system card is light on the left (%d) and dark on the right (%d)", at(sys, 10), at(sys, 60))
	}
	if at(light, 10) < 400 || at(light, 60) < 400 {
		t.Fatal("the light card is light everywhere")
	}
	if at(dark, 10) > 150 || at(dark, 60) > 150 {
		t.Fatal("the dark card is dark everywhere")
	}
}

func TestShapesDistanceAndFill(t *testing.T) {
	f := frect{10, 10, 30, 20}
	d := f.distance(3)
	if d(20, 15) >= 0 {
		t.Fatal("the centre is inside")
	}
	if d(5, 15) <= 0 || d(20, 40) <= 0 {
		t.Fatal("outside points have a positive distance")
	}
	if d(10.2, 10.2) <= 0 {
		t.Fatal("the rounded corner is cut away")
	}
	if g := f.grow(2); g != (frect{8, 8, 32, 22}) {
		t.Fatalf("grow = %v", g)
	}
	b := f.bounds()
	if b != image.Rect(9, 9, 31, 21) {
		t.Fatalf("bounds = %v", b)
	}
	if fr(image.Rect(1, 2, 3, 4)) != (frect{1, 2, 3, 4}) {
		t.Fatal("fr")
	}
	if got := (frect{0, 0, 4, 4}).distance(100)(2, 2); got >= 0 {
		t.Fatalf("an oversized radius is clamped, centre distance = %v", got)
	}
}

func TestAccessibilityTreeDescribesEveryControl(t *testing.T) {
	r := newRig(t, mockW, mockH)
	if info := r.page.AccessInfo(); info.Role != widget.RoleGroup || info.Name != i18n.T("settings.title") {
		t.Fatalf("page info = %+v", info)
	}
	kids := r.page.AccessChildren()
	byName := map[string]widget.AccessInfo{}
	for _, k := range kids {
		byName[k.Name] = k
		if k.Name == "" {
			t.Errorf("a nameless node %+v", k)
		}
	}
	if byName[i18n.T("cmd.back")].Role != widget.RoleButton {
		t.Error("the back arrow is a button")
	}
	selected := 0
	for _, k := range kids {
		if k.Role == widget.RoleRadioButton && len(k.States) == 1 && k.States[0] == widget.StateSelected && k.Name == i18n.T(sectionKeys[SectionGeneral]) {
			selected++
		}
	}
	if selected != 1 {
		t.Errorf("the current section must be exposed as the selected navigation entry, found %d", selected)
	}
	if n := byName[i18n.T("settings.rememberWindow")]; n.Role != widget.RoleCheckBox || len(n.States) == 0 || n.States[0] != widget.StateChecked {
		t.Errorf("checked box = %+v", n)
	}
	if n := byName[i18n.T("settings.autoHidePanels")]; n.Role != widget.RoleCheckBox || len(n.States) != 0 {
		t.Errorf("unchecked box = %+v", n)
	}
	if n := byName[i18n.T("settings.theme.system")]; n.Role != widget.RoleRadioButton {
		t.Errorf("theme card = %+v", n)
	}
	if n := byName[i18n.T("settings.language")]; n.Role != widget.RoleLabel && n.Role != widget.RoleComboBox {
		t.Errorf("language = %+v", n)
	}
	var combo widget.AccessInfo
	for _, k := range kids {
		if k.Role == widget.RoleComboBox {
			combo = k
		}
	}
	if combo.Value != "Системный (Русский)" {
		t.Errorf("combo = %+v", combo)
	}
}

func TestAccessibilityForOtherControlKinds(t *testing.T) {
	r := newRig(t, mockW, mockH)
	roles := func(s Section) map[widget.AccessRole]int {
		r.page.SetSection(s)
		out := map[widget.AccessRole]int{}
		for _, k := range r.page.AccessChildren() {
			out[k.Role]++
		}
		return out
	}
	if got := roles(SectionSlideshow); got[widget.RoleSlider] != 1 {
		t.Errorf("slideshow roles = %v", got)
	}
	r.page.focus = "slideshow.interval"
	for _, k := range r.page.AccessChildren() {
		if k.Role == widget.RoleSlider {
			if k.Value != "5 с" || len(k.States) != 1 || k.States[0] != widget.StateFocused {
				t.Errorf("slider = %+v", k)
			}
		}
	}
	if got := roles(SectionFormats); got[widget.RoleCheckBox] != 2 || got[widget.RoleButton] < 2 {
		t.Errorf("formats roles = %v", got)
	}
	var disabled int
	for _, k := range r.page.AccessChildren() {
		for _, s := range k.States {
			if s == widget.StateDisabled {
				disabled++
			}
		}
	}
	if disabled != 1 {
		t.Errorf("%d disabled nodes, want the unavailable codec only", disabled)
	}
	if got := roles(SectionAbout); got[widget.RoleButton] < 4 {
		t.Errorf("about roles = %v", got)
	}
}

func TestThemesRenderEverySectionWithoutTheFocusRingLeaking(t *testing.T) {
	for name, mk := range layoutThemes {
		r := newRig(t, mockW, mockH)
		r.th = mk()
		for s := Section(0); s < sectionCount; s++ {
			r.page.SetSection(s)
			r.tab(3)
			img := r.render()
			if img.Bounds().Dx() != mockW {
				t.Fatalf("%s/%d: bad canvas", name, s)
			}
		}
	}
}

func TestAccentColourFollowsTheTheme(t *testing.T) {
	r := newRig(t, mockW, mockH)
	r.th = theme.Dark().WithAccent(color.RGBA{200, 30, 120, 255})
	img := r.render()
	found := false
	for y := 236; y < 248 && !found; y++ {
		for x := 169; x < 182; x++ {
			if c := img.RGBAAt(x, y); c.R > 150 && c.B > 90 && c.G < 80 {
				found = true
				break
			}
		}
	}
	if !found {
		t.Fatal("a checked box must use the accent colour")
	}
	bar := img.RGBAAt(8, 56)
	if bar.R < 150 {
		t.Fatalf("the navigation indicator must follow the accent, got %v", bar)
	}
}

func TestSectionAccessors(t *testing.T) {
	r := newRig(t, mockW, mockH)
	r.page.SetSection(SectionAbout)
	if r.page.Section() != SectionAbout {
		t.Fatal("Section")
	}
	r.page.SetSection(sectionCount)
	if r.page.Section() != SectionAbout {
		t.Fatal("an unknown section is ignored")
	}
	if len(r.page.Children()) != 0 {
		t.Fatal("the page has no child widgets")
	}
}
