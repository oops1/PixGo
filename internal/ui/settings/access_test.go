package settings

import (
	"strings"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/testutil"
)

func sectionTree(t *testing.T, lang string, s Section) (*rig, *widget.AccessNode) {
	t.Helper()
	r := newRig(t, 560, 560)
	setLang(lang)
	r.page.SetSection(s)
	r.page.Refresh()
	return r, widget.BuildAccessTree(r.page, nil)
}

func TestEverySectionHasNamedControlsInBothLanguages(t *testing.T) {
	for _, lang := range []string{"ru", "en"} {
		for s := Section(0); s < sectionCount; s++ {
			_, tr := sectionTree(t, lang, s)
			testutil.RequireNamed(t, tr)
			if tr.Role != widget.RoleGroup || tr.Name != i18n.T("settings.title") {
				t.Errorf("%s section %d: page node %s %q", lang, s, tr.Role, tr.Name)
			}
			nav := testutil.FindAccess(tr, widget.RoleRadioButton, i18n.T(sectionKeys[s]))
			if nav == nil || !testutil.HasState(nav, widget.StateSelected) {
				t.Errorf("%s section %d: navigation entry %+v", lang, s, nav)
			}
			if testutil.FindAccess(tr, widget.RoleButton, i18n.T("cmd.back")) == nil {
				t.Errorf("%s section %d: back button missing", lang, s)
			}
			interactive := 0
			for _, n := range testutil.FlatAccess(tr) {
				if testutil.InteractiveRole(n.Role) {
					interactive++
				}
			}
			if interactive < int(sectionCount)+1 {
				t.Errorf("%s section %d: only %d interactive nodes", lang, s, interactive)
			}
		}
	}
}

func TestSettingsNamesFollowTheLanguage(t *testing.T) {
	_, ru := sectionTree(t, "ru", SectionGeneral)
	_, en := sectionTree(t, "en", SectionGeneral)
	rn, en2 := strings.Join(testutil.AccessNames(ru), "|"), strings.Join(testutil.AccessNames(en), "|")
	if rn == en2 || !strings.Contains(rn, "Настройки") || !strings.Contains(en2, "Settings") {
		t.Errorf("names do not follow the language\nru: %s\nen: %s", rn, en2)
	}
}

func TestGeneralSectionExposesChecksRadiosAndDropdowns(t *testing.T) {
	_, tr := sectionTree(t, "en", SectionGeneral)
	if n := testutil.FindAccess(tr, widget.RoleCheckBox, i18n.T("settings.rememberWindow")); n == nil {
		t.Errorf("remember-window checkbox missing\n%s", testutil.DescribeAccess(tr))
	}
	if n := testutil.FindAccess(tr, widget.RoleComboBox, i18n.T("settings.language")); n == nil || n.Value == "" {
		t.Errorf("language dropdown %+v", n)
	}
	var selected int
	for _, n := range testutil.FlatAccess(tr) {
		if n.Role == widget.RoleRadioButton && testutil.HasState(n, widget.StateSelected) {
			selected++
		}
	}
	if selected < 3 {
		t.Errorf("%d selected radio buttons, want the section, the theme and the background", selected)
	}
}

func TestPerformanceSectionExposesSliders(t *testing.T) {
	_, tr := sectionTree(t, "en", SectionPerformance)
	var sliders []*widget.AccessNode
	for _, n := range testutil.FlatAccess(tr) {
		if n.Role == widget.RoleSlider {
			sliders = append(sliders, n)
		}
	}
	if len(sliders) == 0 {
		t.Fatal("no sliders")
	}
	for _, s := range sliders {
		if s.Value == "" {
			t.Errorf("slider %q has no value", s.Name)
		}
	}
}

func TestFormatsSectionToggles(t *testing.T) {
	_, tr := sectionTree(t, "en", SectionFormats)
	jpeg := testutil.FindAccess(tr, widget.RoleLabel, "JPEG")
	if jpeg == nil || jpeg.Value == "" {
		t.Errorf("JPEG row %+v", jpeg)
	}
	avif := testutil.FindAccess(tr, widget.RoleCheckBox, "AVIF")
	if avif == nil || !testutil.HasState(avif, widget.StateChecked) {
		t.Errorf("AVIF switch %+v", avif)
	}
	if n := testutil.FindAccess(tr, widget.RoleCheckBox, "JPEG XL"); n == nil || testutil.HasState(n, widget.StateChecked) {
		t.Errorf("JPEG XL switch %+v", n)
	}
}

func TestFocusedControlIsMarkedInTheTree(t *testing.T) {
	r := newRig(t, 560, 560)
	setLang("en")
	r.tab(1)
	var focused []string
	for _, n := range testutil.FlatAccess(widget.BuildAccessTree(r.page, nil)) {
		if testutil.HasState(n, widget.StateFocused) {
			focused = append(focused, n.Name)
		}
	}
	if len(focused) != 1 {
		t.Errorf("focused nodes %v", focused)
	}
}
