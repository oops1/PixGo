package platform

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnvironmentOverridesTheBodySize(t *testing.T) {
	cases := map[string]struct {
		pt float64
		ok bool
	}{
		"10.5":  {10.5, true},
		" 12 ":  {12, true},
		"2":     {minBodyPt, true},
		"90":    {maxBodyPt, true},
		"":      {0, false},
		"big":   {0, false},
		"-3":    {0, false},
		"0":     {0, false},
		"11,5":  {0, false},
		"NaN":   {0, false},
		"1e400": {0, false},
	}
	for in, want := range cases {
		pt, ok := envFontPt(func(string) string { return in })
		if ok != want.ok || (ok && pt != want.pt) {
			t.Errorf("%q -> %v %v, want %v %v", in, pt, ok, want.pt, want.ok)
		}
	}
}

func TestGTKFontNameGivesTheLastNumber(t *testing.T) {
	cases := map[string]struct {
		pt float64
		ok bool
	}{
		"[Settings]\ngtk-font-name=Noto Sans 10\n":           {10, true},
		"[Settings]\ngtk-font-name = \"Cantarell 11\"\n":     {11, true},
		"gtk-font-name=Ubuntu Medium 10.5":                   {10.5, true},
		"gtk-theme-name=Adwaita\n":                           {0, false},
		"gtk-font-name=Sans\n":                               {0, false},
		"gtk-font-name=\n":                                   {0, false},
		"gtk-font-name=Sans 0\n":                             {0, false},
		"gtk-font-name=Sans 200\n":                           {maxBodyPt, true},
		"":                                                   {0, false},
		"[Settings]\nother=1\ngtk-font-name=DejaVu Sans 9\n": {9, true},
		"# gtk-font-name=Sans 14\n[Settings]\ngtk-font-name=Sans 9\n": {9, true},
	}
	for in, want := range cases {
		pt, ok := parseGTKFontPt(in)
		if ok != want.ok || (ok && pt != want.pt) {
			t.Errorf("%q -> %v %v, want %v %v", in, pt, ok, want.pt, want.ok)
		}
	}
}

func TestGTKSettingsAreReadFromTheFirstFileThatHasAFont(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "gtk-4.0.ini")
	second := filepath.Join(dir, "gtk-3.0.ini")
	if err := os.WriteFile(first, []byte("[Settings]\ngtk-theme-name=x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("[Settings]\ngtk-font-name=Sans 12\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	pt, ok := gtkFontPt([]string{filepath.Join(dir, "missing.ini"), first, second})

	if !ok || pt != 12 {
		t.Fatalf("pt = %v, ok = %v", pt, ok)
	}
	if _, ok := gtkFontPt([]string{filepath.Join(dir, "missing.ini")}); ok {
		t.Fatal("a missing file has no font")
	}
	if _, ok := gtkFontPt(nil); ok {
		t.Fatal("no files, no font")
	}
}

func TestGTKSettingsFilesFollowTheConfigDirectory(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/cfg")

	files := gtkSettingsFiles()

	if len(files) != 2 || filepath.ToSlash(files[0]) != "/cfg/gtk-4.0/settings.ini" || filepath.ToSlash(files[1]) != "/cfg/gtk-3.0/settings.ini" {
		t.Fatalf("files = %v", files)
	}
}

func TestSystemFontsAlwaysGiveAUsableSize(t *testing.T) {
	t.Setenv(EnvFontPt, "")

	pt := NewSystemFonts().BodyPt()

	if pt < minBodyPt || pt > maxBodyPt {
		t.Fatalf("pt = %v", pt)
	}
}

func TestSystemFontsHonourTheEnvironmentOverride(t *testing.T) {
	t.Setenv(EnvFontPt, "11")

	if pt := NewSystemFonts().BodyPt(); pt != 11 {
		t.Fatalf("pt = %v", pt)
	}
}

func TestPlatformCarriesTheFontsProvider(t *testing.T) {
	if New().Fonts == nil {
		t.Fatal("no fonts provider")
	}
}
