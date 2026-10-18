//go:build windows

package i18n

import (
	"slices"
	"testing"
)

func TestWindowsSystemLanguage(t *testing.T) {
	tag := SystemLanguage()
	if tag == "" {
		t.Fatal("GetUserDefaultUILanguage returned nothing")
	}
	t.Logf("UI language %q region %q matched %q", tag, SystemRegion(), Match(tag))
	if !slices.Contains(Supported(), Match(tag)) {
		t.Fatalf("matched %q", Match(tag))
	}
}

func TestWindowsPreferredLanguages(t *testing.T) {
	tags := systemLanguages()
	if len(tags) == 0 || tags[0] != SystemLanguage() {
		t.Fatalf("languages %v", tags)
	}
	seen := map[string]bool{}
	for _, tag := range tags {
		if tag == "" || seen[tag] {
			t.Fatalf("empty or repeated tag in %v", tags)
		}
		seen[tag] = true
	}
	t.Logf("preferred UI languages: %v", tags)
}
