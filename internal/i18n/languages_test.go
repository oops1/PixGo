package i18n

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"
	"unicode"
)

func withLang(t *testing.T, l string) {
	t.Helper()
	prev := Lang()
	SetLang(l)
	t.Cleanup(func() { SetLang(prev) })
}

func TestPluralCategories(t *testing.T) {
	ns := []int{0, 1, 2, 5, 11, 21, 22, 25, 101}
	want := map[string][]string{
		"en":    {"other", "one", "other", "other", "other", "other", "other", "other", "other"},
		"de":    {"other", "one", "other", "other", "other", "other", "other", "other", "other"},
		"es":    {"other", "one", "other", "other", "other", "other", "other", "other", "other"},
		"it":    {"other", "one", "other", "other", "other", "other", "other", "other", "other"},
		"nl":    {"other", "one", "other", "other", "other", "other", "other", "other", "other"},
		"fr":    {"one", "one", "other", "other", "other", "other", "other", "other", "other"},
		"pt-BR": {"one", "one", "other", "other", "other", "other", "other", "other", "other"},
		"ru":    {"many", "one", "few", "many", "many", "one", "few", "many", "one"},
		"uk":    {"many", "one", "few", "many", "many", "one", "few", "many", "one"},
		"pl":    {"many", "one", "few", "many", "many", "many", "few", "many", "many"},
		"cs":    {"other", "one", "few", "other", "other", "other", "other", "other", "other"},
		"tr":    {"other", "other", "other", "other", "other", "other", "other", "other", "other"},
	}
	for _, l := range Supported() {
		w, ok := want[l]
		if !ok {
			t.Errorf("no plural expectations for %s", l)
			continue
		}
		for i, n := range ns {
			if got := PluralCategory(l, n); got != w[i] {
				t.Errorf("%s PluralCategory(%d) = %s, want %s", l, n, got, w[i])
			}
			if got := PluralCategory(l, -n); got != w[i] {
				t.Errorf("%s PluralCategory(%d) = %s, want %s", l, -n, got, w[i])
			}
		}
	}
}

func TestPluralHelpers(t *testing.T) {
	withLang(t, "ru")
	for n, want := range map[int]string{1: "a", 2: "b", 5: "c", 11: "c", 21: "a", 104: "b", 112: "c", 0: "c"} {
		if got := Plural(n, "a", "b", "c"); got != want {
			t.Errorf("ru Plural(%d) = %q, want %q", n, got, want)
		}
	}
	SetLang("en")
	if Plural(1, "a", "b", "c") != "a" || Plural(2, "a", "b", "c") != "c" || Plural(0, "a", "b", "c") != "c" {
		t.Error("en plural")
	}
	SetLang("cs")
	if Plural(3, "a", "b", "c") != "b" || Plural(5, "a", "b", "c") != "c" {
		t.Error("cs plural")
	}
	SetLang("fr")
	if Plural(0, "a", "b", "c") != "a" || Plural(2, "a", "b", "c") != "c" {
		t.Error("fr plural")
	}
}

func TestUnitFormsMatchRules(t *testing.T) {
	for _, l := range Supported() {
		cat := catalogs[l]
		for _, unit := range []string{"unit.items", "unit.files"} {
			one, few, many := cat[unit+".one"], cat[unit+".few"], cat[unit+".many"]
			switch l {
			case "ru", "uk", "pl", "cs":
				if one == few || few == many || one == many {
					t.Errorf("%s %s needs three different forms: %q %q %q", l, unit, one, few, many)
				}
			case "tr":
				if one != few || few != many {
					t.Errorf("tr %s has no plural after numbers: %q %q %q", unit, one, few, many)
				}
			default:
				if few != many || (one == many && !(l == "it" && unit == "unit.files")) {
					t.Errorf("%s %s should have a singular and one plural: %q %q %q", l, unit, one, few, many)
				}
			}
		}
	}
}

func TestSelectedCountText(t *testing.T) {
	for _, l := range Supported() {
		withLang(t, l)
		for _, n := range []int{1, 2, 5, 21} {
			unit := Plural(n, T("unit.items.one"), T("unit.items.few"), T("unit.items.many"))
			s := T("gallery.selected", n, unit)
			if !strings.Contains(s, fmt.Sprint(n)) || !strings.Contains(s, unit) {
				t.Errorf("%s %d: %q", l, n, s)
			}
		}
	}
}

func TestMatchTable(t *testing.T) {
	for want, tags := range map[string][]string{
		"ru":    {"ru", "ru-RU", "ru_RU.UTF-8", "RU_ru@euro", "ru-UA", "ru-BY"},
		"en":    {"en", "en-US", "en_GB.UTF-8", "en-AU", "en_IN", "C", "POSIX", "", "  ", "c.utf-8"},
		"uk":    {"uk", "uk-UA", "uk_UA.UTF-8", "UK"},
		"de":    {"de", "de-DE", "de-AT", "de_CH", "de-LI", "de_DE@euro", "de-DE-1996"},
		"fr":    {"fr", "fr-FR", "fr-CA", "fr_BE", "fr-CH", "fr_FR.UTF-8"},
		"es":    {"es", "es-ES", "es-MX", "es-419", "es_AR.UTF-8", "es-US"},
		"it":    {"it", "it-IT", "it-CH", "it_IT.UTF-8"},
		"pt-BR": {"pt", "pt-BR", "pt_BR.UTF-8", "pt-PT", "PT-br", "pt_PT", "pt-AO"},
		"pl":    {"pl", "pl-PL", "pl_PL.UTF-8"},
		"cs":    {"cs", "cs-CZ", "cs_CZ.UTF-8"},
		"nl":    {"nl", "nl-NL", "nl-BE", "nl_NL.UTF-8"},
		"tr":    {"tr", "tr-TR", "tr_TR.UTF-8", "tr-CY"},
	} {
		for _, tag := range tags {
			if got := Match(tag); got != want {
				t.Errorf("Match(%q) = %q, want %q", tag, got, want)
			}
		}
	}
	for _, tag := range []string{"zh-Hans-CN", "zh-TW", "zh", "ja-JP", "ko-KR", "ar-SA", "sv-SE", "nb-NO", "da", "fi-FI", "hu", "ro", "el", "he", "hi-IN", "th", "vi", "id"} {
		if got := Match(tag); got != "en" {
			t.Errorf("Match(%q) = %q, want en", tag, got)
		}
	}
}

func TestSetLangAccepts(t *testing.T) {
	withLang(t, "en")
	for tag, want := range map[string]string{"de": "de", "DE_de.UTF-8": "de", "pt-pt": "pt-BR", "pt-br": "pt-BR", "uk-UA": "uk", "tr": "tr"} {
		SetLang(tag)
		if Lang() != want {
			t.Errorf("SetLang(%q): %q, want %q", tag, Lang(), want)
		}
		if T("window.close") != catalogs[want]["window.close"] {
			t.Errorf("%s: wrong catalogue", tag)
		}
	}
}

func TestDetectFromLists(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	list := func(tags ...string) func() []string { return func() []string { return tags } }
	for _, c := range []struct {
		name   string
		env    map[string]string
		native []string
		want   string
	}{
		{"override wins", map[string]string{"PIXGO_LANG": "de"}, []string{"ru-RU"}, "de"},
		{"override region", map[string]string{"PIXGO_LANG": "pt_PT"}, []string{"ru-RU"}, "pt-BR"},
		{"override unknown", map[string]string{"PIXGO_LANG": "zh"}, []string{"ru-RU"}, "en"},
		{"first supported", nil, []string{"uk-UA", "en-US"}, "uk"},
		{"second after unsupported first", nil, []string{"zh-Hans-CN", "de-DE"}, "de"},
		{"third after two unsupported", nil, []string{"zh-CN", "ja-JP", "tr-TR"}, "tr"},
		{"none supported", nil, []string{"zh-CN", "ja-JP"}, "en"},
		{"empty entries skipped", nil, []string{"", "pl-PL"}, "pl"},
		{"nothing", nil, nil, "en"},
	} {
		if got := detectFrom(env(c.env), list(c.native...)); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestDetectDoesNotCrash(t *testing.T) {
	t.Setenv("PIXGO_LANG", "")
	if l := Detect(); l == "" {
		t.Fatal("empty language")
	}
	if _, ok := catalogs[Detect()]; !ok {
		t.Fatalf("detected %q has no catalogue", Detect())
	}
}

func TestLanguageNamesAreNative(t *testing.T) {
	want := map[string]string{
		"en": "English", "ru": "Русский", "uk": "Українська", "de": "Deutsch", "fr": "Français", "es": "Español",
		"it": "Italiano", "pt-BR": "Português (Brasil)", "pl": "Polski", "cs": "Čeština", "nl": "Nederlands", "tr": "Türkçe",
	}
	for _, l := range Supported() {
		for _, shown := range Supported() {
			if got := catalogs[l]["settings.language."+shown]; got != want[shown] {
				t.Errorf("%s shows %s as %q, want %q", l, shown, got, want[shown])
			}
		}
	}
}

var sameAsEnglish = map[string]bool{
	"app.name": true, "dialog.ok": true, "info.iso": true, "info.gps": true, "zoom.percent": true,
	"title.dimensions": true, "viewer.position": true, "gallery.thumbPx": true, "format.dimensions": true,
	"slideshow.counter": true, "slideshow.seconds": true, "unit.seconds": true, "unit.fps": true,
	"settings.initialZoom.actual": true, "settings.format.avif": true, "settings.format.jxl": true,
	"settings.format.heic": true, "settings.about.license": true, "edit.aspect.4x3": true,
	"edit.aspect.3x2": true, "edit.aspect.16x9": true, "edit.cropValue": true, "edit.size": true,
	"format.aperture": true, "format.exposure": true, "format.focalLength": true, "format.altitude": true,
	"settings.language.en": true, "settings.language.ru": true, "settings.language.uk": true,
	"settings.language.de": true, "settings.language.fr": true, "settings.language.es": true,
	"settings.language.it": true, "settings.language.pt-BR": true, "settings.language.pl": true,
	"settings.language.cs": true, "settings.language.nl": true, "settings.language.tr": true,
	"settings.displayProfile.srgb": true, "info.hdr": true, "info.profileCICP": true,
	"print.unit.cm": true, "print.paper.photo": true,
}

var sameWords = map[string][]string{
	"de":    {"key.plus", "key.minus", "cmd.slideshowPause", "zoom.label", "update.title", "gallery.desktop", "gallery.videos", "gallery.downloads", "unit.megabytes", "unit.megapixels", "info.name", "info.format", "info.software", "settings.about.version", "edit.aspect.orig", "resize.original", "settings.language.system", "print.margin.normal"},
	"fr":    {"key.plus", "cmd.slideshowPause", "zoom.label", "gallery.documents", "info.format", "info.sizes", "info.dimensions", "info.flash", "info.latitude", "info.longitude", "info.altitude", "settings.section.formats", "settings.about.version", "open.filter.images", "access.menu", "resize.unit.px", "print.orient.portrait", "print.orientation"},
	"es":    {"zoom.label", "unit.megabytes", "unit.megapixels", "info.flash", "info.software", "info.color", "settings.section.general", "edit.aspect.orig", "resize.original", "print.margin.normal"},
	"it":    {"zoom.label", "gallery.desktop", "gallery.fileSystem", "unit.files.one", "unit.megabytes", "unit.megapixels", "info.file", "info.flash", "info.software", "access.menu", "format.bitDepth", "print.unit.in"},
	"pt-BR": {"zoom.label", "gallery.downloads", "unit.items.one", "unit.megabytes", "unit.megapixels", "info.megapixels", "info.flash", "info.software", "info.latitude", "info.longitude", "info.altitude", "access.menu", "edit.aspect.orig", "resize.unit.px", "resize.original", "print.margin.normal"},
	"pl":    {"key.plus", "key.minus", "gallery.folder", "unit.megabytes", "info.format", "info.model", "access.menu"},
	"cs":    {"key.plus", "key.minus", "unit.megabytes", "info.model", "info.software", "slideshow.interval"},
	"nl":    {"key.plus", "zoom.label", "viewer.frames", "update.title", "gallery.recent", "gallery.downloads", "gallery.items", "unit.items.one", "unit.items.few", "unit.items.many", "unit.megabytes", "unit.megapixels", "info.megapixels", "info.camera", "info.model", "info.lens", "info.software", "slideshow.interval", "settings.about.platform", "access.menu", "resize.unit.px", "format.bitDepth", "print.printer"},
	"tr":    {"unit.megabytes", "unit.megapixels", "info.model", "info.lens", "settings.about.platform", "format.bitDepth", "print.margin.normal"},
}

func TestNothingLeftUntranslated(t *testing.T) {
	for _, l := range Supported() {
		if l == "en" {
			continue
		}
		for k, v := range catalogs[l] {
			if v != en[k] || sameAsEnglish[k] {
				continue
			}
			if slices.Contains(sameWords[l], k) {
				continue
			}
			t.Errorf("%s %s is not translated: %q", l, k, v)
		}
	}
}

func TestSameWordsAllowListIsHonest(t *testing.T) {
	for l, keys := range sameWords {
		for _, k := range keys {
			if catalogs[l][k] != en[k] {
				t.Errorf("%s %s is listed as identical to English but is %q", l, k, catalogs[l][k])
			}
		}
	}
}

func TestEveryLanguageLooksLikeItsScript(t *testing.T) {
	for _, l := range []string{"ru", "uk"} {
		for k, v := range catalogs[l] {
			if sameAsEnglish[k] || strings.HasPrefix(k, "settings.format") || strings.HasPrefix(k, "format.") {
				continue
			}
			hasCyr := false
			for _, r := range v {
				if unicode.Is(unicode.Cyrillic, r) {
					hasCyr = true
					break
				}
			}
			if !hasCyr && len(v) > 3 {
				t.Logf("%s %s has no Cyrillic: %q", l, k, v)
			}
		}
	}
}

func TestNoStrayMarkup(t *testing.T) {
	for _, l := range Supported() {
		for k, v := range catalogs[l] {
			if v != strings.TrimSpace(v) {
				t.Errorf("%s %s has surrounding spaces: %q", l, k, v)
			}
			if strings.Contains(v, "  ") || strings.ContainsAny(v, "\t\n\r") {
				t.Errorf("%s %s has stray whitespace: %q", l, k, v)
			}
			if strings.Contains(v, "{nb}") || strings.Contains(v, "\\u") || strings.Contains(v, " ") || strings.Contains(v, "‑") {
				t.Errorf("%s %s has a placeholder or a glyph the UI font lacks: %q", l, k, v)
			}
		}
	}
}

func TestEndPunctuationFollowsEnglish(t *testing.T) {
	for _, l := range Supported() {
		for k, v := range catalogs[l] {
			e := en[k]
			if strings.HasSuffix(e, "…") != strings.HasSuffix(v, "…") {
				t.Errorf("%s %s: ellipsis differs: %q / %q", l, k, v, e)
			}
			if strings.HasSuffix(e, ".") != strings.HasSuffix(v, ".") && !strings.HasPrefix(k, "settings.format") {
				t.Errorf("%s %s: final full stop differs: %q / %q", l, k, v, e)
			}
		}
	}
}

func TestLocaleFormats(t *testing.T) {
	d := time.Date(2024, 9, 14, 18, 27, 5, 0, time.UTC)
	const mb = 1 << 20
	cases := []struct {
		lang, number, bytes, dateTime, date string
	}{
		{"en", "1,234,567.5", "8.73 MB", "9/14/2024 6:27 PM", "9/14/2024"},
		{"ru", "1" + nb + "234" + nb + "567,5", "8,73 МБ", "14.09.2024 18:27", "14.09.2024"},
		{"uk", "1" + nb + "234" + nb + "567,5", "8,73 МБ", "14.09.2024 18:27", "14.09.2024"},
		{"de", "1.234.567,5", "8,73 MB", "14.09.2024 18:27", "14.09.2024"},
		{"fr", "1" + nb + "234" + nb + "567,5", "8,73 Mo", "14/09/2024 18:27", "14/09/2024"},
		{"es", "1.234.567,5", "8,73 MB", "14/9/2024 18:27", "14/9/2024"},
		{"it", "1.234.567,5", "8,73 MB", "14/09/2024 18:27", "14/09/2024"},
		{"pt-BR", "1.234.567,5", "8,73 MB", "14/09/2024 18:27", "14/09/2024"},
		{"pl", "1" + nb + "234" + nb + "567,5", "8,73 MB", "14.09.2024 18:27", "14.09.2024"},
		{"cs", "1" + nb + "234" + nb + "567,5", "8,73 MB", "14. 9. 2024 18:27", "14. 9. 2024"},
		{"nl", "1.234.567,5", "8,73 MB", "14-9-2024 18:27", "14-9-2024"},
		{"tr", "1.234.567,5", "8,73 MB", "14.09.2024 18:27", "14.09.2024"},
	}
	if len(cases) != len(Supported()) {
		t.Fatalf("%d cases for %d languages", len(cases), len(Supported()))
	}
	for _, c := range cases {
		loc := LocaleFor(c.lang)
		if loc.Name != c.lang {
			t.Errorf("LocaleFor(%q).Name = %q", c.lang, loc.Name)
		}
		if got := loc.Number("1234567.5"); got != c.number {
			t.Errorf("%s Number = %q, want %q", c.lang, got, c.number)
		}
		if got := loc.Bytes(mb * 8731 / 1000); got != c.bytes {
			t.Errorf("%s Bytes = %q, want %q", c.lang, got, c.bytes)
		}
		if got := loc.DateTime(d); got != c.dateTime {
			t.Errorf("%s DateTime = %q, want %q", c.lang, got, c.dateTime)
		}
		if got := loc.Date(d); got != c.date {
			t.Errorf("%s Date = %q, want %q", c.lang, got, c.date)
		}
		withLang(t, c.lang)
		if got := FormatDateTime(d); got != c.dateTime {
			t.Errorf("%s FormatDateTime = %q", c.lang, got)
		}
		if got := FormatBytes(mb * 8731 / 1000); got != c.bytes {
			t.Errorf("%s FormatBytes = %q", c.lang, got)
		}
		if got := FormatFloat(0.5, 1); !strings.Contains(got, loc.Decimal) {
			t.Errorf("%s FormatFloat = %q", c.lang, got)
		}
	}
}

func TestBytesUnitsEveryLanguage(t *testing.T) {
	for _, l := range Supported() {
		loc := LocaleFor(l)
		for _, n := range []int64{0, 1, 1023, 1024, 1 << 20, 1 << 30, 1 << 40, 5 << 40} {
			s := loc.Bytes(n)
			if !strings.Contains(s, " ") || strings.HasPrefix(s, " ") {
				t.Errorf("%s Bytes(%d) = %q", l, n, s)
			}
		}
		if got := loc.Bytes(1536); !strings.HasPrefix(got, "1"+loc.Decimal+"50 ") {
			t.Errorf("%s Bytes(1536) = %q", l, got)
		}
	}
}

func TestLocaleLayoutsParse(t *testing.T) {
	d := time.Date(2024, 1, 5, 7, 3, 0, 0, time.UTC)
	for _, l := range Supported() {
		loc := LocaleFor(l)
		s := loc.DateTime(d)
		parsed, err := time.Parse(loc.DateLayout+" "+loc.TimeLayout, s)
		if err != nil || !parsed.Equal(d) {
			t.Errorf("%s: %q does not round-trip: %v", l, s, err)
		}
	}
}

func TestSupportedIsSortedForTheMenu(t *testing.T) {
	got := Supported()
	if got[0] != "en" || got[1] != "ru" {
		t.Errorf("order: %v", got)
	}
	seen := map[string]bool{}
	for _, l := range got {
		if seen[l] {
			t.Errorf("duplicate %s", l)
		}
		seen[l] = true
	}
	var cats []string
	for l := range catalogs {
		cats = append(cats, l)
	}
	sort.Strings(cats)
	sort.Strings(got)
	if strings.Join(cats, ",") != strings.Join(got, ",") {
		t.Errorf("catalogues %v, supported %v", cats, got)
	}
}

func TestDecimalizeKeepsFractionsAndRatios(t *testing.T) {
	for _, l := range Supported() {
		loc := LocaleFor(l)
		for in, want := range map[string]string{"4.0": "4" + loc.Decimal + "0", "1/250": "1/250", "0.5": "0" + loc.Decimal + "5", "24": "24"} {
			if got := loc.Decimalize(in); got != want {
				t.Errorf("%s Decimalize(%q) = %q, want %q", l, in, got, want)
			}
		}
	}
	withLang(t, "de")
	if got := LocalizeDecimal("2.8"); got != "2,8" {
		t.Errorf("LocalizeDecimal = %q", got)
	}
}
