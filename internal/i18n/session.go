package i18n

import (
	"bufio"
	"strings"
)

const (
	envSessionLang        = "WINLINE_LANG"
	envSessionLangDefault = "WINLINE_LANG_DEFAULT"
)

func sessionLanguage(getenv func(string) string, readChoice func() string) string {
	if tag := normalizeTag(getenv(envSessionLang)); tag != "" {
		return tag
	}
	if tag := normalizeTag(choiceFrom(readChoice())); tag != "" {
		return tag
	}
	return normalizeTag(getenv(envSessionLangDefault))
}

func choiceFrom(data string) string {
	sc := bufio.NewScanner(strings.NewReader(data))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || line[0] == '#' || line[0] == ';' || line[0] == '[' {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || !strings.EqualFold(strings.TrimSpace(key), "language") {
			continue
		}
		return strings.Trim(strings.TrimSpace(value), `"'`)
	}
	return ""
}
