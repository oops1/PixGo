//go:build windows

package i18n

import "testing"

func TestWindowsSystemLanguage(t *testing.T) {
	tag := SystemLanguage()
	if tag == "" {
		t.Fatal("GetUserDefaultUILanguage returned nothing")
	}
	t.Logf("UI language %q region %q matched %q", tag, SystemRegion(), Match(tag))
	if m := Match(tag); m != "ru" && m != "en" {
		t.Fatalf("matched %q", m)
	}
}
