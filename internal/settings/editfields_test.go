package settings

import (
	"strings"
	"testing"
)

func TestEditFieldDefaults(t *testing.T) {
	d := Defaults()
	if !d.ConfirmOverwrite {
		t.Fatal("overwriting the original must ask by default")
	}
	if d.SaveStripGPS {
		t.Fatal("location data must be kept by default")
	}
}

func TestEditFieldsMissingInOldFileKeepDefaults(t *testing.T) {
	s, err := Decode([]byte(`{"version":2,"theme":"dark"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !s.ConfirmOverwrite || s.SaveStripGPS {
		t.Fatalf("got confirm=%v strip=%v", s.ConfirmOverwrite, s.SaveStripGPS)
	}
}

func TestEditFieldsRoundTrip(t *testing.T) {
	s := Defaults()
	s.ConfirmOverwrite = false
	s.SaveStripGPS = true
	data, err := s.Encode()
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"confirmOverwrite": false`, `"saveStripGPS": true`} {
		if !strings.Contains(string(data), key) {
			t.Fatalf("%s missing in\n%s", key, data)
		}
	}
	back, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	if back.ConfirmOverwrite || !back.SaveStripGPS {
		t.Fatalf("got confirm=%v strip=%v", back.ConfirmOverwrite, back.SaveStripGPS)
	}
}
