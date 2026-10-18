//go:build !windows

package i18n

import "testing"

func TestUnixLanguagesFollowTheDocumentedOrder(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PIXGO_LANG", "")
	for _, c := range []struct {
		env  map[string]string
		want string
	}{
		{map[string]string{"LANG": "de_DE.UTF-8"}, "de"},
		{map[string]string{"LANG": "de_DE.UTF-8", "LC_MESSAGES": "fr_FR.UTF-8"}, "fr"},
		{map[string]string{"LANG": "de_DE.UTF-8", "LC_MESSAGES": "fr_FR.UTF-8", "LC_ALL": "pt_PT.UTF-8"}, "pt-BR"},
		{map[string]string{"LANG": "de_DE.UTF-8", "WINLINE_LANG_DEFAULT": "tr_TR"}, "tr"},
		{map[string]string{"LANG": "de_DE.UTF-8", "WINLINE_LANG_DEFAULT": "tr_TR", "WINLINE_LANG": "uk_UA.UTF-8"}, "uk"},
		{map[string]string{"LANG": "zh_CN.UTF-8"}, "en"},
		{map[string]string{"LANG": "C"}, "en"},
	} {
		for _, k := range []string{"LANG", "LC_ALL", "LC_MESSAGES", "WINLINE_LANG", "WINLINE_LANG_DEFAULT"} {
			t.Setenv(k, c.env[k])
		}
		if got := Detect(); got != c.want {
			t.Errorf("%v: %q, want %q", c.env, got, c.want)
		}
	}
}
