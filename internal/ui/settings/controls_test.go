package settings

import (
	"strings"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/i18n"
	data "github.com/oops1/PixGo/internal/settings"
)

func TestThemeCardsChooseTheTheme(t *testing.T) {
	for _, tc := range []struct{ id, want string }{
		{"theme:light", data.ThemeLight},
		{"theme:dark", data.ThemeDark},
		{"theme:system", data.ThemeSystem},
	} {
		t.Run(tc.id, func(t *testing.T) {
			r := newRig(t, mockW, mockH)
			r.src.cfg.Theme = data.ThemeLight
			if tc.want == data.ThemeLight {
				r.src.cfg.Theme = data.ThemeDark
			}
			r.page.Refresh()
			r.clickItem(tc.id)
			if r.src.cfg.Theme != tc.want {
				t.Fatalf("theme = %q, want %q", r.src.cfg.Theme, tc.want)
			}
			if !r.item(tc.id).on {
				t.Fatal("the clicked card must be selected after the refresh")
			}
		})
	}
}

func TestBackgroundCardsChooseTheBackground(t *testing.T) {
	for _, tc := range []struct{ id, want string }{
		{"background:system", data.BackgroundSystem},
		{"background:white", data.BackgroundWhite},
		{"background:black", data.BackgroundBlack},
		{"background:checker", data.BackgroundChecker},
	} {
		t.Run(tc.id, func(t *testing.T) {
			r := newRig(t, mockW, mockH)
			r.src.cfg.TransparentBackground = data.BackgroundWhite
			if tc.want == data.BackgroundWhite {
				r.src.cfg.TransparentBackground = data.BackgroundBlack
			}
			r.page.Refresh()
			r.clickItem(tc.id)
			if r.src.cfg.TransparentBackground != tc.want {
				t.Fatalf("background = %q, want %q", r.src.cfg.TransparentBackground, tc.want)
			}
		})
	}
}

func TestClickingTheSelectedCardChangesNothing(t *testing.T) {
	r := newRig(t, mockW, mockH)
	r.clickItem("theme:system")
	if r.rec.has("update") {
		t.Fatal("selecting the card that is already selected must not write settings")
	}
}

func TestCheckboxesToggleTheirSetting(t *testing.T) {
	type cb struct {
		section Section
		id      string
		get     func(*data.Settings) bool
	}
	cases := []cb{
		{SectionGeneral, "rememberWindow", func(s *data.Settings) bool { return s.RememberWindow }},
		{SectionGeneral, "restoreLastImage", func(s *data.Settings) bool { return s.RestoreLastImage }},
		{SectionGeneral, "confirmDelete", func(s *data.Settings) bool { return s.ConfirmDelete }},
		{SectionGeneral, "autoHidePanels", func(s *data.Settings) bool { return s.AutoHidePanels }},
		{SectionGeneral, "singleWindow", func(s *data.Settings) bool { return s.SingleWindow }},
		{SectionGeneral, "updatesCheck", func(s *data.Settings) bool { return s.Updates.Check }},
		{SectionView, "ctrlWheelZoom", func(s *data.Settings) bool { return s.CtrlWheelZoom }},
		{SectionView, "smoothZoom", func(s *data.Settings) bool { return s.SmoothZoom }},
		{SectionView, "applyExifOrientation", func(s *data.Settings) bool { return s.ApplyExifOrientation }},
		{SectionView, "keepZoomOnNavigate", func(s *data.Settings) bool { return s.KeepZoomOnNavigate }},
		{SectionView, "loopFolder", func(s *data.Settings) bool { return s.LoopFolder }},
		{SectionView, "sortDesc", func(s *data.Settings) bool { return s.SortDesc }},
		{SectionSlideshow, "slideshow.loop", func(s *data.Settings) bool { return s.Slideshow.Loop }},
		{SectionSlideshow, "slideshow.shuffle", func(s *data.Settings) bool { return s.Slideshow.Shuffle }},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			r := newRig(t, mockW, mockH)
			r.page.SetSection(tc.section)
			before := tc.get(&r.src.cfg)
			r.clickItem(tc.id)
			if after := tc.get(&r.src.cfg); after == before {
				t.Fatalf("%s stayed %v after a click", tc.id, before)
			}
			if r.item(tc.id).on == before {
				t.Fatal("the checkbox must show the new state")
			}
			r.clickItem(tc.id)
			if tc.get(&r.src.cfg) != before {
				t.Fatalf("%s must return to %v after the second click", tc.id, before)
			}
		})
	}
}

func TestClickingTheLabelTogglesTheCheckbox(t *testing.T) {
	r := newRig(t, mockW, mockH)
	it := r.item("confirmDelete")
	r.click(it.r.Min.X+checkLabelX+20, it.r.Min.Y+10)
	if r.src.cfg.ConfirmDelete {
		t.Fatal("a click on the label must toggle the checkbox")
	}
}

func TestClickRightOfTheLabelDoesNothing(t *testing.T) {
	r := newRig(t, mockW, mockH)
	it := r.item("confirmDelete")
	r.click(it.r.Max.X-2, it.r.Min.Y+10)
	if r.rec.has("update") {
		t.Fatal("empty space to the right of a label is not part of the checkbox")
	}
}

func TestRadioGroupsPickOneValue(t *testing.T) {
	type rd struct {
		section Section
		id      string
		check   func(*data.Settings) bool
		prepare func(*data.Settings)
	}
	cases := []rd{
		{SectionView, "wheel:zoom", func(s *data.Settings) bool { return s.Wheel == data.WheelZoom }, nil},
		{SectionView, "wheel:navigate", func(s *data.Settings) bool { return s.Wheel == data.WheelNavigate }, func(s *data.Settings) { s.Wheel = data.WheelZoom }},
		{SectionView, "preload:0", func(s *data.Settings) bool { return s.Preload == 0 }, nil},
		{SectionView, "preload:1", func(s *data.Settings) bool { return s.Preload == 1 }, func(s *data.Settings) { s.Preload = 2 }},
		{SectionView, "preload:2", func(s *data.Settings) bool { return s.Preload == 2 }, nil},
		{SectionLanguage, "language:ru", func(s *data.Settings) bool { return s.Language == "ru" }, nil},
		{SectionLanguage, "language:en", func(s *data.Settings) bool { return s.Language == "en" }, nil},
		{SectionLanguage, "language:system", func(s *data.Settings) bool { return s.Language == data.LanguageSystem }, func(s *data.Settings) { s.Language = "en" }},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			r := newRig(t, mockW, mockH)
			if tc.prepare != nil {
				tc.prepare(&r.src.cfg)
			}
			r.page.SetSection(tc.section)
			r.page.Refresh()
			r.clickItem(tc.id)
			if !tc.check(&r.src.cfg) {
				t.Fatalf("%s did not apply: %+v", tc.id, r.src.cfg)
			}
			chosen := r.item(tc.id)
			if !chosen.on {
				t.Fatal("the chosen radio must be on after the refresh")
			}
			on := 0
			for i := range r.page.items {
				if it := &r.page.items[i]; it.kind == kindRadio && it.group == chosen.group && it.on {
					on++
				}
			}
			if on != 1 {
				t.Fatalf("%d radios are on in the group, want exactly one", on)
			}
		})
	}
}

func TestLanguageDropdownOffersSystemAndEveryLanguage(t *testing.T) {
	r := newRig(t, mockW, mockH)
	r.clickItem("language")
	if !r.rec.has("menu") {
		t.Fatal("the dropdown must show a menu")
	}
	var texts []string
	var checked []bool
	for _, m := range r.rec.menuItems {
		texts = append(texts, m.Text)
		checked = append(checked, m.Checked)
		if !m.Checkable {
			t.Errorf("%q must be checkable", m.Text)
		}
	}
	want := []string{"Системный (Русский)", "Русский", "English"}
	if strings.Join(texts, "|") != strings.Join(want, "|") {
		t.Fatalf("menu = %v, want %v", texts, want)
	}
	if !checked[0] || checked[1] || checked[2] {
		t.Fatalf("only the current value is marked: %v", checked)
	}
	it := r.item("language")
	if r.rec.menuAt != [2]int{it.r.Min.X, it.r.Max.Y + 2} {
		t.Fatalf("menu opens at %v, want under the field %v", r.rec.menuAt, [2]int{it.r.Min.X, it.r.Max.Y + 2})
	}
	r.rec.menuItems[2].OnClick()
	if r.src.cfg.Language != "en" {
		t.Fatalf("language = %q after choosing English", r.src.cfg.Language)
	}
	if r.item("language").text != "English" {
		t.Fatalf("the field shows %q", r.item("language").text)
	}
}

func TestLanguageDropdownKeepsAnAppSuppliedSystemEntry(t *testing.T) {
	src := newSource()
	src.langs = []Language{{ID: data.LanguageSystem, Name: "Авто"}, {ID: "ru", Name: "Русский"}}
	r := newRigWith(t, src, mockW, mockH)
	r.clickItem("language")
	if len(r.rec.menuItems) != 2 || r.rec.menuItems[0].Text != "Авто" {
		t.Fatalf("menu = %+v", r.rec.menuItems)
	}
}

func TestOtherDropdownsApplyTheirChoice(t *testing.T) {
	r := newRig(t, mockW, mockH)
	r.page.SetSection(SectionView)
	r.clickItem("initialZoom")
	if len(r.rec.menuItems) != 3 || !r.rec.menuItems[0].Checked {
		t.Fatalf("zoom menu = %+v", r.rec.menuItems)
	}
	r.rec.menuItems[2].OnClick()
	if r.src.cfg.InitialZoom != data.ZoomWidth {
		t.Fatalf("zoom = %q", r.src.cfg.InitialZoom)
	}
	r.clickItem("sort")
	r.rec.menuItems[1].OnClick()
	if r.src.cfg.Sort != data.SortDate {
		t.Fatalf("sort = %q", r.src.cfg.Sort)
	}
	if got := r.item("sort").text; got != i18n.T("sort.date") {
		t.Fatalf("sort field shows %q", got)
	}
}

func TestDropdownWithoutAMenuHandlerIsHarmless(t *testing.T) {
	r := newRig(t, mockW, mockH)
	r.page.deps.Actions.ShowMenu = nil
	r.clickItem("language")
	if r.rec.has("menu") {
		t.Fatal("no menu can be shown without a handler")
	}
}

func TestButtonsRunTheirActions(t *testing.T) {
	for _, tc := range []struct {
		section Section
		id      string
		call    string
	}{
		{SectionFormats, "defaultApps", "defaultApps"},
		{SectionPerformance, "clearThumbCache", "clear"},
		{SectionAbout, "checkUpdates", "checkUpdates"},
	} {
		t.Run(tc.id, func(t *testing.T) {
			r := newRig(t, mockW, mockH)
			r.page.SetSection(tc.section)
			r.clickItem(tc.id)
			if r.rec.count(tc.call) != 1 {
				t.Fatalf("calls = %v, want one %q", r.rec.calls, tc.call)
			}
		})
	}
}

func TestButtonsWithoutActionsAreNotShown(t *testing.T) {
	r := newRig(t, mockW, mockH)
	r.page.deps.Actions.OpenDefaultApps = nil
	r.page.deps.Actions.CheckUpdates = nil
	r.page.SetSection(SectionFormats)
	if r.page.itemByID("defaultApps") != nil {
		t.Fatal("a button without an action must not be built")
	}
	r.page.SetSection(SectionAbout)
	if r.page.itemByID("checkUpdates") != nil {
		t.Fatal("a button without an action must not be built")
	}
}

func hasNote(p *Page, part string) bool {
	for i := range p.items {
		if p.items[i].kind == kindNote && strings.Contains(joinLines(p.items[i].lines), part) {
			return true
		}
	}
	return false
}

func lastNote(p *Page) string {
	for i := len(p.items) - 1; i >= 0; i-- {
		if p.items[i].kind == kindNote {
			return joinLines(p.items[i].lines)
		}
	}
	return ""
}

func TestClearingTheCacheShowsAndHidesAConfirmation(t *testing.T) {
	r := newRig(t, mockW, mockH)
	r.page.SetSection(SectionPerformance)
	r.clickItem("clearThumbCache")
	if !r.page.noted {
		t.Fatal("the confirmation must appear after clearing")
	}
	if got := lastNote(r.page); got != i18n.T("settings.clearThumbCache.done") {
		t.Fatalf("note = %q", got)
	}
	r.tm.advance(noteDelay)
	if r.page.noted || lastNote(r.page) == i18n.T("settings.clearThumbCache.done") {
		t.Fatal("the confirmation must disappear after a few seconds")
	}
}

func TestDiskUsageIsShownThroughTheLocaleFormatter(t *testing.T) {
	r := newRig(t, mockW, mockH)
	r.page.SetSection(SectionPerformance)
	want := i18n.T("settings.thumbCache.used", i18n.FormatBytes(12_900_000))
	for i := range r.page.items {
		if it := &r.page.items[i]; it.kind == kindValue && joinLines(it.lines) == want {
			return
		}
	}
	t.Fatalf("the disk usage line %q is missing", want)
}

func TestRepositoryAndComponentLinksOpenTheirURLs(t *testing.T) {
	r := newRig(t, mockW, mockH)
	r.page.SetSection(SectionAbout)
	r.clickItem("repository")
	r.clickItem("component:0")
	want := []string{"https://github.com/oops1/PixGo", "https://pkg.go.dev/golang.org/x/image"}
	if strings.Join(r.rec.urls, "|") != strings.Join(want, "|") {
		t.Fatalf("urls = %v, want %v", r.rec.urls, want)
	}
	if r.page.itemByID("component:1") != nil {
		t.Fatal("a component without a URL is plain text, not a link")
	}
}

func TestTranslateLinkAppearsWithARepository(t *testing.T) {
	r := newRig(t, mockW, mockH)
	r.page.SetSection(SectionLanguage)
	r.clickItem("translate")
	if len(r.rec.urls) != 1 || r.rec.urls[0] != "https://github.com/oops1/PixGo" {
		t.Fatalf("urls = %v", r.rec.urls)
	}
	src := newSource()
	src.about.Repository = ""
	r = newRigWith(t, src, mockW, mockH)
	r.page.SetSection(SectionLanguage)
	if r.page.itemByID("translate") != nil {
		t.Fatal("without a repository there is nothing to link to")
	}
}

func TestLastUpdateCheckIsFormattedOrReportedAsNever(t *testing.T) {
	r := newRig(t, mockW, mockH)
	r.page.SetSection(SectionAbout)
	if !hasNote(r.page, i18n.T("settings.about.neverChecked")) {
		t.Fatal("a never-checked page must say so")
	}
	r.src.last = timeAt(2026, 10, 12, 18, 30)
	r.page.Refresh()
	want := i18n.T("settings.about.lastCheck", i18n.FormatDateTime(r.src.last.Local()))
	for i := range r.page.items {
		if r.page.items[i].kind == kindNote && joinLines(r.page.items[i].lines) == want {
			return
		}
	}
	t.Fatalf("no note %q among the items", want)
}

func TestFormatRowsToggleOnlyAvailableOptionalCodecs(t *testing.T) {
	r := newRig(t, mockW, mockH)
	r.page.SetSection(SectionFormats)
	if it := r.page.itemByID("format:jpeg"); it == nil || it.interactive() {
		t.Fatal("a built-in format has no switch")
	}
	if it := r.page.itemByID("format:heic"); it == nil || it.interactive() || !it.disabled {
		t.Fatalf("a codec missing from the build is greyed out and inert: %+v", it)
	}
	r.clickItem("format:avif")
	if r.src.cfg.Formats["avif"].Enabled {
		t.Fatal("AVIF must be switched off")
	}
	r.src.formats[3].Enabled = false
	r.page.Refresh()
	r.clickItem("format:avif")
	if !r.src.cfg.Formats["avif"].Enabled {
		t.Fatal("AVIF must be switched on again")
	}
	r.clickItem("format:jxl")
	if !r.src.cfg.Formats["jxl"].Enabled {
		t.Fatal("JPEG XL must be switched on")
	}
}

func TestUnavailableFormatSaysItIsNotInTheBuild(t *testing.T) {
	r := newRig(t, mockW, mockH)
	r.page.SetSection(SectionFormats)
	got := r.page.itemByID("format:heic").detail
	if want := i18n.T("settings.format.unavailable"); !strings.HasSuffix(got, want) {
		t.Fatalf("detail = %q, must end with %q", got, want)
	}
}

func TestDefaultAppsHintDependsOnThePlatform(t *testing.T) {
	old := goos
	defer func() { goos = old }()
	for name, key := range map[string]string{"windows": "settings.defaultApps.hint", "linux": "settings.defaultApps.linux"} {
		goos = name
		r := newRig(t, mockW, mockH)
		r.page.SetSection(SectionFormats)
		want := joinLines(wrapLines(noteFont, i18n.T(key), r.page.geo.xt-r.page.geo.x0, 5))
		found := false
		for i := range r.page.items {
			if it := &r.page.items[i]; it.kind == kindNote && joinLines(it.lines) == want {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: hint %q is missing", name, key)
		}
	}
}

func TestExternalRefreshUpdatesTheControls(t *testing.T) {
	r := newRig(t, mockW, mockH)
	r.src.cfg.Theme = data.ThemeDark
	r.src.cfg.AutoHidePanels = true
	r.page.Refresh()
	if !r.item("theme:dark").on || r.item("theme:system").on || !r.item("autoHidePanels").on {
		t.Fatal("Refresh must re-read the settings snapshot")
	}
}

func TestMouseBackButtonLeavesThePage(t *testing.T) {
	r := newRig(t, mockW, mockH)
	r.page.OnMouseButton(widget.MouseEvent{X: 200, Y: 200, Button: widget.MouseButtonX1, Pressed: true})
	if !r.rec.has("back") {
		t.Fatal("the mouse back button must leave the page")
	}
}
