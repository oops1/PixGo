package app

import (
	"strings"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/i18n"
)

func TestEngineStringsCoverEveryLanguage(t *testing.T) {
	for _, l := range i18n.Supported() {
		if l == "en" || l == "ru" {
			continue
		}
		values, ok := engineStrings[strings.ToUpper(l)]
		if !ok {
			t.Errorf("no engine strings for %s", l)
			continue
		}
		if len(values) != len(engineStringKeys) {
			t.Errorf("%s: %d values for %d keys", l, len(values), len(engineStringKeys))
		}
		for i, key := range engineStringKeys {
			got := widget.TrIn(strings.ToUpper(l), key)
			if got != values[i] || strings.TrimSpace(got) == "" || got == key {
				t.Errorf("%s %s = %q", l, key, got)
			}
			if en := widget.TrIn("EN", key); en == got && key != "dlg.ok" && !strings.HasPrefix(key, "dlg.place.") && key != "dlg.title.info" {
				t.Logf("%s %s is the same as English: %q", l, key, got)
			}
		}
	}
}
