package settings

import (
	"testing"

	"github.com/oops1/PixGo/internal/i18n"
)

func TestRawDevelopLineIsShownOnlyWhenRawIsAvailable(t *testing.T) {
	r := newRig(t, mockW, mockH)
	r.page.SetSection(SectionFormats)
	if r.page.itemByID("rawDevelop") != nil {
		t.Fatal("the line is shown in a build without RAW")
	}
	noted := newNotedRig(t, "en", 700, 600)
	if noted.page.itemByID("rawDevelop") == nil {
		t.Fatal("the line is missing although RAW is available")
	}
	for i := range noted.src.formats {
		if noted.src.formats[i].ID == "raw" {
			noted.src.formats[i].Available = false
		}
	}
	noted.page.Refresh()
	if noted.page.itemByID("rawDevelop") != nil {
		t.Fatal("the line stays when RAW is not available")
	}
}

func TestRawDevelopCheckboxTogglesTheSetting(t *testing.T) {
	r := newNotedRig(t, "ru", 700, 600)
	if !r.src.cfg.RawDevelop || !r.item("rawDevelop").on {
		t.Fatal("development must be on by default")
	}
	r.clickItem("rawDevelop")
	if r.src.cfg.RawDevelop || r.item("rawDevelop").on {
		t.Fatal("a click must switch it off")
	}
	r.clickItem("rawDevelop")
	if !r.src.cfg.RawDevelop {
		t.Fatal("a second click must switch it on")
	}
	if got := r.item("rawDevelop").text; got != "Проявлять DNG (медленнее, точнее)" {
		t.Fatalf("label %q", got)
	}
}

func TestRawDevelopTextsExistInEveryLanguage(t *testing.T) {
	for _, lang := range i18n.Supported() {
		i18n.SetLang(lang)
		for _, key := range []string{"settings.rawDevelop", "settings.rawDevelop.desc"} {
			if i18n.T(key) == key || i18n.T(key) == "" {
				t.Errorf("%s lacks %s", lang, key)
			}
		}
	}
	i18n.SetLang("ru")
}

func TestRawDevelopIsAFocusStopAndNotedBelowItsLine(t *testing.T) {
	r := newNotedRig(t, "en", 700, 600)
	r.focusOn("rawDevelop")
	it := r.item("rawDevelop")
	if !it.focusable() {
		t.Fatal("the checkbox must be focusable")
	}
	found := false
	for i := range r.page.items {
		if r.page.items[i].id == "rawDevelop" && i+1 < len(r.page.items) && r.page.items[i+1].kind == kindNote {
			found = true
		}
	}
	if !found {
		t.Fatal("the description must follow the checkbox")
	}
}
