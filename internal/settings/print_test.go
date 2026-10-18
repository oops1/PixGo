package settings

import (
	"strings"
	"testing"
)

func TestPrintDefaults(t *testing.T) {
	d := Defaults()
	if d.PrintPaper != "a4" || d.PrintOrientation != "auto" || d.PrintPlacement != "fit" || d.PrintMargins != "narrow" || d.PrintUnit != "cm" || d.PrintDestination != "" {
		t.Fatalf("%+v", d)
	}
}

func TestPrintFieldsMissingInOldFileKeepDefaults(t *testing.T) {
	s, err := Decode([]byte(`{"version":2,"theme":"dark"}`))
	if err != nil {
		t.Fatal(err)
	}
	if s.PrintPaper != "a4" || s.PrintPlacement != "fit" || s.PrintMargins != "narrow" {
		t.Fatalf("%+v", s)
	}
}

func TestPrintFieldsRoundTrip(t *testing.T) {
	s := Defaults()
	s.PrintDestination = "Office Laser"
	s.PrintPaper = "10x15"
	s.PrintOrientation = "landscape"
	s.PrintPlacement = "custom"
	s.PrintMargins = "normal"
	s.PrintUnit = "in"
	data, err := s.Encode()
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"printDestination": "Office Laser"`, `"printPaper": "10x15"`, `"printOrientation": "landscape"`, `"printPlacement": "custom"`, `"printMargins": "normal"`, `"printUnit": "in"`} {
		if !strings.Contains(string(data), key) {
			t.Fatalf("%s missing in\n%s", key, data)
		}
	}
	back, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	if back.PrintDestination != s.PrintDestination || back.PrintPaper != s.PrintPaper || back.PrintOrientation != s.PrintOrientation ||
		back.PrintPlacement != s.PrintPlacement || back.PrintMargins != s.PrintMargins || back.PrintUnit != s.PrintUnit {
		t.Fatalf("%+v", back)
	}
}

func TestPrintFieldsNormalise(t *testing.T) {
	s, err := Decode([]byte(`{"version":2,"printPaper":"a0","printOrientation":"sideways","printPlacement":"stretch","printMargins":"huge","printUnit":"yd","printDestination":"line\nbreak"}`))
	if err != nil {
		t.Fatal(err)
	}
	d := Defaults()
	if s.PrintPaper != d.PrintPaper || s.PrintOrientation != d.PrintOrientation || s.PrintPlacement != d.PrintPlacement ||
		s.PrintMargins != d.PrintMargins || s.PrintUnit != d.PrintUnit || s.PrintDestination != "" {
		t.Fatalf("%+v", s)
	}
	long := Defaults()
	long.PrintDestination = strings.Repeat("x", 600)
	long.Normalize()
	if long.PrintDestination != "" {
		t.Fatal("an absurdly long printer name must be dropped")
	}
}
