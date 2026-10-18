package i18n

import "testing"

func TestSessionLanguageOrder(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	choice := func(s string) func() string { return func() string { return s } }
	cases := []struct {
		name   string
		env    map[string]string
		choice string
		want   string
	}{
		{"variable wins", map[string]string{"WINLINE_LANG": "ru_RU.UTF-8", "WINLINE_LANG_DEFAULT": "en-US"}, "language = en-US", "ru-ru"},
		{"person's choice next", map[string]string{"WINLINE_LANG_DEFAULT": "en-US"}, "# chosen in Settings\nlanguage = ru-RU\n", "ru-ru"},
		{"quoted choice", nil, `language = "ru-RU"`, "ru-ru"},
		{"deployment default last", map[string]string{"WINLINE_LANG_DEFAULT": "ru-RU"}, "", "ru-ru"},
		{"nothing from the session", map[string]string{"LANG": "en_US.UTF-8"}, "theme = dark", ""},
	}
	for _, c := range cases {
		if got := sessionLanguage(env(c.env), choice(c.choice)); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
	if Match(sessionLanguage(env(map[string]string{"WINLINE_LANG": "ru-RU"}), choice(""))) != "ru" {
		t.Fatal("ru-RU must select the Russian catalogue")
	}
}
