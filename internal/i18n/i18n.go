package i18n

import (
	"fmt"
	"os"
	"strings"
	"sync/atomic"
)

const fallback = "en"

var catalogs = map[string]map[string]string{
	"en": en, "ru": ru, "uk": uk, "de": de, "fr": fr, "es": es,
	"it": it, "pt-BR": ptBR, "pl": pl, "cs": cs, "nl": nl, "tr": tr,
}

var supported = []string{"en", "ru", "uk", "de", "fr", "es", "it", "pt-BR", "pl", "cs", "nl", "tr"}

var canonical = func() map[string]string {
	m := make(map[string]string, len(supported))
	for _, id := range supported {
		m[strings.ToLower(id)] = id
	}
	return m
}()

var regionalDefault = map[string]string{"pt": "pt-BR"}

var lang atomic.Value

func init() { lang.Store(Detect()) }

func Supported() []string { return append([]string(nil), supported...) }

func Detect() string { return detectFrom(os.Getenv, systemLanguages) }

func detectFrom(getenv func(string) string, native func() []string) string {
	if v := strings.TrimSpace(getenv("PIXGO_LANG")); v != "" {
		return Match(v)
	}
	for _, tag := range native() {
		if l, ok := lookup(normalizeTag(tag)); ok {
			return l
		}
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
	if id, ok := canonical[tag]; ok {
		return id, true
	}
	base := tag
	if i := strings.IndexByte(tag, '-'); i > 0 {
		base = tag[:i]
	}
	if id, ok := regionalDefault[base]; ok {
		return id, true
	}
	if id, ok := canonical[base]; ok {
		return id, true
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

func SystemLanguage() string {
	if tags := systemLanguages(); len(tags) > 0 {
		return tags[0]
	}
	return ""
}

func SystemRegion() string { return regionOf(SystemLanguage()) }

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

func Keys(l string) []string {
	var out []string
	for k := range catalogs[l] {
		out = append(out, k)
	}
	return out
}
