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

func TestColorManagementDefaultsAndNormalization(t *testing.T) {
	d := Defaults()
	if !d.ColorManagement || d.DisplayProfile != DisplayProfileSystem {
		t.Fatalf("defaults: %v %q", d.ColorManagement, d.DisplayProfile)
	}
	old, err := Decode([]byte(`{"version":2,"theme":"dark"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !old.ColorManagement || old.DisplayProfile != DisplayProfileSystem {
		t.Fatalf("a file written by 1.1 must get the defaults: %v %q", old.ColorManagement, old.DisplayProfile)
	}
	off, err := Decode([]byte(`{"version":2,"colorManagement":false,"displayProfile":"srgb"}`))
	if err != nil || off.ColorManagement || off.DisplayProfile != DisplayProfileSRGB {
		t.Fatalf("explicit choices lost: %v %+v", err, off)
	}
	bad, err := Decode([]byte(`{"version":2,"displayProfile":"adobe"}`))
	if err != nil || bad.DisplayProfile != DisplayProfileSystem {
		t.Fatalf("unknown profile mode must normalise: %v %q", err, bad.DisplayProfile)
	}
	data, err := off.Encode()
	if err != nil {
		t.Fatal(err)
	}
	back, err := Decode(data)
	if err != nil || back.ColorManagement || back.DisplayProfile != DisplayProfileSRGB {
		t.Fatalf("round trip: %v %+v", err, back)
	}
}
