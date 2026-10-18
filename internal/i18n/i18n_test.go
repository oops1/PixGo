package i18n

import (
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestCataloguesHaveSameKeys(t *testing.T) {
	if len(Supported()) != 12 {
		t.Fatalf("supported: %v", Supported())
	}
	for _, l := range Supported() {
		cat, ok := catalogs[l]
		if !ok {
			t.Fatalf("no catalogue for %q", l)
		}
		if len(cat) != len(en) || len(cat) == 0 {
			t.Errorf("%s has %d keys, en has %d", l, len(cat), len(en))
		}
		for k := range en {
			if _, ok := cat[k]; !ok {
				t.Errorf("%s lacks %q", l, k)
			}
		}
		for k := range cat {
			if _, ok := en[k]; !ok {
				t.Errorf("%s has %q that en lacks", l, k)
			}
		}
	}
}

var verb = regexp.MustCompile(`%[-+# 0]*[0-9]*(?:\.[0-9]+)?[a-zA-Z%]`)

func TestPlaceholdersMatch(t *testing.T) {
	for _, l := range Supported() {
		for k, v := range catalogs[l] {
			e, ok := en[k]
			if !ok {
				continue
			}
			lv, ev := verb.FindAllString(v, -1), verb.FindAllString(e, -1)
			if strings.Join(lv, " ") != strings.Join(ev, " ") {
				t.Errorf("%s %s: verbs %v, en verbs %v", l, k, lv, ev)
			}
		}
	}
}

func TestNoEmptyValues(t *testing.T) {
	for name, cat := range catalogs {
		for k, v := range cat {
			if strings.TrimSpace(v) == "" {
				t.Errorf("%s: %q is empty", name, k)
			}
		}
	}
}

func TestRequiredKeys(t *testing.T) {
	for _, k := range []string{
		"window.minimize", "window.maximize", "window.restore", "window.close",
		"cmd.back", "cmd.info", "cmd.favorite.add", "cmd.favorite.remove", "cmd.more", "cmd.gallery", "cmd.open",
		"cmd.delete", "cmd.rotateLeft", "cmd.rotateRight", "cmd.zoomIn", "cmd.zoomOut", "cmd.zoomFit",
		"cmd.zoomActual", "cmd.zoomWidth", "cmd.fullscreen", "cmd.fullscreenExit", "cmd.prev", "cmd.next",
		"cmd.flipH", "cmd.flipV", "cmd.copy", "cmd.copyPath", "cmd.saveAs", "cmd.reveal", "cmd.openWith",
		"cmd.settings", "cmd.slideshow", "cmd.slideshowPause", "zoom.percent",
		"error.open", "error.unsupported", "error.tooLarge", "error.corrupt", "error.notFound",
		"viewer.loading", "empty.title", "empty.hint",
		"settings.section.general", "settings.section.view", "settings.section.formats",
		"settings.section.slideshow", "settings.section.performance", "settings.section.language",
		"settings.section.about", "key.plus", "key.minus", "key.space",
	} {
		if _, ok := ru[k]; !ok {
			t.Errorf("ru lacks required %q", k)
		}
	}
}

func TestFormat(t *testing.T) {
	SetLang("ru")
	if got := T("title.dimensions", "mountain.jpg", 6000, 4000); got != "mountain.jpg — 6000 × 4000" {
		t.Fatal(got)
	}
	if got := T("zoom.percent", 100); got != "100%" {
		t.Fatal(got)
	}
	if got := T("no.such.key"); got != "no.such.key" {
		t.Fatal(got)
	}
	SetLang("en")
	if got := T("window.close"); got != "Close" {
		t.Fatal(got)
	}
	SetLang("ru")
	if got := T("window.close"); got != "Закрыть" {
		t.Fatal(got)
	}
}

func TestSetLang(t *testing.T) {
	defer SetLang("ru")
	SetLang("en")
	SetLang("zh")
	if Lang() != "en" {
		t.Fatalf("unknown language changed to %q", Lang())
	}
	SetLang("ru_RU.UTF-8")
	if Lang() != "ru" {
		t.Fatalf("got %q", Lang())
	}
	t.Setenv("PIXGO_LANG", "en")
	SetLang("system")
	if Lang() != "en" {
		t.Fatalf("system: %q", Lang())
	}
}

func TestEnvLanguage(t *testing.T) {
	for _, c := range []struct {
		env  map[string]string
		want string
	}{
		{map[string]string{"LC_ALL": "ru_RU.UTF-8", "LANG": "en_US.UTF-8"}, "ru-ru"},
		{map[string]string{"LC_MESSAGES": "en_GB.UTF-8", "LANG": "ru_RU.UTF-8"}, "en-gb"},
		{map[string]string{"LANG": "ru_RU.UTF-8"}, "ru-ru"},
		{map[string]string{"LC_ALL": "C", "LANG": "ru_RU.UTF-8"}, "ru-ru"},
		{map[string]string{"LC_ALL": "C", "LANG": "C.UTF-8"}, "c-utf-8"},
		{map[string]string{"LANG": "POSIX"}, ""},
		{nil, ""},
	} {
		got := envLanguage(func(k string) string { return c.env[k] })
		if c.want == "c-utf-8" {
			c.want = "c"
		}
		if got != c.want && !(c.want == "c" && got == "") {
			t.Errorf("%v: got %q, want %q", c.env, got, c.want)
		}
	}
}

func TestRegionOf(t *testing.T) {
	for in, want := range map[string]string{"ru-RU": "RU", "en_GB.UTF-8": "GB", "ru": "", "": "", "zh-Hans-CN": "CN"} {
		if got := regionOf(in); got != want {
			t.Errorf("regionOf(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLangFromLangID(t *testing.T) {
	for id, want := range map[uint16]string{0x0419: "ru", 0x0409: "en", 0x0809: "en", 0x0422: "uk", 0x0407: "de", 0x0000: ""} {
		if got := langFromLangID(id); got != want {
			t.Errorf("langFromLangID(%#x) = %q, want %q", id, got, want)
		}
	}
}

const nb = "\xc2\xa0"

func TestRussianNumber(t *testing.T) {
	for in, want := range map[string]string{
		"0":             "0",
		"123":           "123",
		"1234":          "1" + nb + "234",
		"-1234567.891":  "-1" + nb + "234" + nb + "567,891",
		"0.":            "0,",
		"12.50":         "12,50",
		"1000000":       "1" + nb + "000" + nb + "000",
		"1.5e20":        "1,5e+20",
		"1.5e-07":       "1,5e-7",
		"2e+3":          "2e+3",
		"123456.5e+100": "123" + nb + "456,5e+100",
		"Ошибка":        "Ошибка",
		"":              "",
		"-":             "-",
		"1.2.3":         "1.2.3",
	} {
		if got := Russian.Number(in); got != want {
			t.Errorf("Russian.Number(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEnglishNumber(t *testing.T) {
	if got := English.Number("-1234567.5"); got != "-1,234,567.5" {
		t.Errorf("English.Number = %q", got)
	}
}

func TestRussianText(t *testing.T) {
	for in, want := range map[string]string{
		"1234 * 5.5 =":     "1" + nb + "234 * 5,5 =",
		"12e+":             "12e+",
		".5":               ",5",
		"1234567 × 89 =":   "1" + nb + "234" + nb + "567 × 89 =",
		"sqrt(10000)+0.25": "sqrt(10" + nb + "000)+0,25",
	} {
		if got := Russian.Text(in); got != want {
			t.Errorf("Russian.Text(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestGroup(t *testing.T) {
	for _, c := range []struct {
		in   string
		n    int
		want string
	}{
		{"FFFFFFFF", 4, "FFFF FFFF"},
		{"1234567", 3, "1 234 567"},
		{"777", 3, "777"},
		{"", 3, ""},
	} {
		if got := Group(c.in, c.n, " "); got != c.want {
			t.Errorf("Group(%q, %d) = %q, want %q", c.in, c.n, got, c.want)
		}
	}
}

func TestFormatBytes(t *testing.T) {
	const mb = 1 << 20
	for _, c := range []struct {
		n      int64
		ru, en string
	}{
		{-5, "0 Б", "0 B"},
		{0, "0 Б", "0 B"},
		{1023, "1023 Б", "1023 B"},
		{1024, "1,00 КБ", "1.00 KB"},
		{1536, "1,50 КБ", "1.50 KB"},
		{mb * 8731 / 1000, "8,73 МБ", "8.73 MB"},
		{mb*8731/1000 - 1, "8,73 МБ", "8.73 MB"},
		{mb * 8731 / 100, "87,3 МБ", "87.3 MB"},
		{mb * 873, "873 МБ", "873 MB"},
		{1023 * 1024, "1023 КБ", "1023 KB"},
		{1<<20 - 1, "1023 КБ", "1023 KB"},
		{1 << 30, "1,00 ГБ", "1.00 GB"},
		{5 << 40, "5,00 ТБ", "5.00 TB"},
		{5000 << 40, "5000 ТБ", "5000 TB"},
		{1<<63 - 1, "8388607 ТБ", "8388607 TB"},
	} {
		if got := Russian.Bytes(c.n); got != c.ru {
			t.Errorf("Russian.Bytes(%d) = %q, want %q", c.n, got, c.ru)
		}
		if got := English.Bytes(c.n); got != c.en {
			t.Errorf("English.Bytes(%d) = %q, want %q", c.n, got, c.en)
		}
	}
	SetLang("ru")
	if got := FormatBytes(mb * 8731 / 1000); got != "8,73 МБ" {
		t.Errorf("FormatBytes = %q", got)
	}
}

func TestFormatDateTime(t *testing.T) {
	d := time.Date(2024, 9, 14, 18, 27, 5, 0, time.UTC)
	if got := Russian.DateTime(d); got != "14.09.2024 18:27" {
		t.Errorf("ru: %q", got)
	}
	if got := English.DateTime(d); got != "9/14/2024 6:27 PM" {
		t.Errorf("en: %q", got)
	}
	if got := English.Date(d); got != "9/14/2024" {
		t.Errorf("en date: %q", got)
	}
	SetLang("en")
	if got := FormatDateTime(d); got != "9/14/2024 6:27 PM" {
		t.Errorf("FormatDateTime en: %q", got)
	}
	SetLang("ru")
	if got := FormatDateTime(d); got != "14.09.2024 18:27" {
		t.Errorf("FormatDateTime ru: %q", got)
	}
	if got := FormatDate(d); got != "14.09.2024" {
		t.Errorf("FormatDate ru: %q", got)
	}
}

func TestFormatIntFloat(t *testing.T) {
	SetLang("ru")
	if got := FormatInt(1234567); got != "1"+nb+"234"+nb+"567" {
		t.Errorf("ru int: %q", got)
	}
	if got := FormatFloat(1234.5, 2); got != "1"+nb+"234,50" {
		t.Errorf("ru float: %q", got)
	}
	if got := English.Float(-0.25, 2); got != "-0.25" {
		t.Errorf("en float: %q", got)
	}
}
