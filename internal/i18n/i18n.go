package i18n

import (
	"fmt"
	"os"
	"strings"
	"sync/atomic"
)

const fallback = "en"

var catalogs = map[string]map[string]string{"ru": ru, "en": en}

var supported = []string{"ru", "en"}

var lang atomic.Value

func init() { lang.Store(Detect()) }

func Supported() []string { return append([]string(nil), supported...) }

func Detect() string { return detectFrom(os.Getenv, systemLanguage) }

func detectFrom(getenv func(string) string, native func() string) string {
	if v := strings.TrimSpace(getenv("PIXGO_LANG")); v != "" {
		return Match(v)
	}
	if tag := native(); tag != "" {
		return Match(tag)
	}
	return fallback
}

func Match(tag string) string {
	if l, ok := lookup(normalizeTag(tag)); ok {
		return l
	}
	return fallback
}

func lookup(tag string) (string, bool) {
	if tag == "" {
		return "", false
	}
	if _, ok := catalogs[tag]; ok {
		return tag, true
	}
	if i := strings.IndexByte(tag, '-'); i > 0 {
		if _, ok := catalogs[tag[:i]]; ok {
			return tag[:i], true
		}
	}
	return "", false
}

func normalizeTag(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if i := strings.IndexAny(s, ".@"); i >= 0 {
		s = s[:i]
	}
	s = strings.ReplaceAll(s, "_", "-")
	if s == "c" || s == "posix" {
		return ""
	}
	return s
}

func envLanguage(getenv func(string) string) string {
	for _, name := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if tag := normalizeTag(getenv(name)); tag != "" {
			return tag
		}
	}
	return ""
}

func regionOf(tag string) string {
	parts := strings.Split(normalizeTag(tag), "-")
	for _, p := range parts[1:] {
		if len(p) == 2 {
			return strings.ToUpper(p)
		}
	}
	return ""
}

func SystemLanguage() string { return systemLanguage() }

func SystemRegion() string { return regionOf(systemLanguage()) }

func SetLang(l string) {
	l = strings.TrimSpace(l)
	if l == "" || strings.EqualFold(l, "system") {
		lang.Store(Detect())
		return
	}
	if m, ok := lookup(normalizeTag(l)); ok {
		lang.Store(m)
	}
}

func Lang() string { return lang.Load().(string) }

func T(key string, args ...any) string {
	s, ok := catalogs[Lang()][key]
	if !ok {
		if s, ok = en[key]; !ok {
			s = key
		}
	}
	if len(args) > 0 {
		return fmt.Sprintf(s, args...)
	}
	return s
}

func Plural(n int, one, few, many string) string {
	if Lang() == "en" {
		if n == 1 {
			return one
		}
		return many
	}
	if n < 0 {
		n = -n
	}
	n %= 100
	if n >= 11 && n <= 14 {
		return many
	}
	switch n % 10 {
	case 1:
		return one
	case 2, 3, 4:
		return few
	}
	return many
}

func Keys(l string) []string {
	var out []string
	for k := range catalogs[l] {
		out = append(out, k)
	}
	return out
}
