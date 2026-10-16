package settings

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveOptionsDefaultToTheSpecValues(t *testing.T) {
	d := Defaults()

	if d.SaveJPEGQuality != 90 || d.SaveWebPQuality != 80 || d.SaveWebPLossless {
		t.Fatalf("save defaults: jpeg %d, webp %d, lossless %v", d.SaveJPEGQuality, d.SaveWebPQuality, d.SaveWebPLossless)
	}
}

func TestSaveQualityOutsideTheRangeFallsBackToTheDefault(t *testing.T) {
	tests := []struct {
		name     string
		json     string
		wantJPEG int
		wantWebP int
	}{
		{"both inside", `{"saveJpegQuality":55,"saveWebpQuality":33}`, 55, 33},
		{"lower bound", `{"saveJpegQuality":1,"saveWebpQuality":1}`, 1, 1},
		{"upper bound", `{"saveJpegQuality":100,"saveWebpQuality":100}`, 100, 100},
		{"zero", `{"saveJpegQuality":0,"saveWebpQuality":0}`, 90, 80},
		{"negative", `{"saveJpegQuality":-4,"saveWebpQuality":-1}`, 90, 80},
		{"above the top", `{"saveJpegQuality":101,"saveWebpQuality":1000}`, 90, 80},
		{"only jpeg is bad", `{"saveJpegQuality":500,"saveWebpQuality":42}`, 90, 42},
		{"only webp is bad", `{"saveJpegQuality":42,"saveWebpQuality":0}`, 42, 80},
		{"absent keys", `{"theme":"dark"}`, 90, 80},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Decode([]byte(tc.json))
			if err != nil {
				t.Fatal(err)
			}

			if got.SaveJPEGQuality != tc.wantJPEG || got.SaveWebPQuality != tc.wantWebP {
				t.Fatalf("jpeg %d webp %d, want %d and %d", got.SaveJPEGQuality, got.SaveWebPQuality, tc.wantJPEG, tc.wantWebP)
			}
		})
	}
}

func TestNormalizeFixesSaveQualityWrittenByTheStore(t *testing.T) {
	st := NewMemory(Defaults())

	st.Update(func(s *Settings) { s.SaveJPEGQuality = 250; s.SaveWebPQuality = -3 })

	got := st.Get()
	if got.SaveJPEGQuality != DefaultSaveJPEGQuality || got.SaveWebPQuality != DefaultSaveWebPQuality {
		t.Fatalf("jpeg %d webp %d", got.SaveJPEGQuality, got.SaveWebPQuality)
	}
}

func TestSaveOptionsUseTheDocumentedJSONNames(t *testing.T) {
	s := Defaults()
	s.SaveWebPLossless = true

	b, err := s.Encode()
	if err != nil {
		t.Fatal(err)
	}

	for _, key := range []string{`"saveJpegQuality": 90`, `"saveWebpQuality": 80`, `"saveWebpLossless": true`} {
		if !strings.Contains(string(b), key) {
			t.Errorf("%s is missing in %s", key, b)
		}
	}
}

func TestLastSaveChoiceSurvivesARestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	st, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	st.Update(func(s *Settings) {
		s.SaveJPEGQuality = 71
		s.SaveWebPQuality = 12
		s.SaveWebPLossless = true
	})
	if err := st.Flush(); err != nil {
		t.Fatal(err)
	}

	again, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	got := again.Get()
	if got.SaveJPEGQuality != 71 || got.SaveWebPQuality != 12 || !got.SaveWebPLossless {
		t.Fatalf("after restart: jpeg %d, webp %d, lossless %v", got.SaveJPEGQuality, got.SaveWebPQuality, got.SaveWebPLossless)
	}
}

func TestSaveOptionsDoNotDisturbTheOtherSettings(t *testing.T) {
	st := NewMemory(Defaults())
	before := st.Get()

	st.Update(func(s *Settings) { s.SaveJPEGQuality = 10 })

	after := st.Get()
	if after.Theme != before.Theme || after.SaveWebPQuality != before.SaveWebPQuality || after.SaveWebPLossless != before.SaveWebPLossless ||
		after.GalleryThumbSize != before.GalleryThumbSize || !after.FormatEnabled("avif") {
		t.Fatalf("unrelated settings changed: %+v", after)
	}
}

func TestJPEGLosslessIsOnByDefaultAndWhenTheKeyIsMissing(t *testing.T) {
	if !Defaults().SaveJPEGLossless {
		t.Fatal("the default must be lossless")
	}
	for _, in := range []string{`{}`, `{"version":2}`, `{"version":2,"saveJpegQuality":55}`} {
		got, err := Decode([]byte(in))
		if err != nil {
			t.Fatal(err)
		}
		if !got.SaveJPEGLossless || got.Version != Current {
			t.Errorf("%s: lossless %v version %d", in, got.SaveJPEGLossless, got.Version)
		}
	}
}

func TestJPEGLosslessSwitchedOffStaysOff(t *testing.T) {
	got, err := Decode([]byte(`{"version":2,"saveJpegLossless":false}`))
	if err != nil {
		t.Fatal(err)
	}
	if got.SaveJPEGLossless {
		t.Fatal("an explicit false was turned back on")
	}
	b, err := got.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"saveJpegLossless": false`) || !strings.Contains(string(b), `"version": 2`) {
		t.Fatalf("encoded: %s", b)
	}
}

func TestJPEGLosslessChoiceSurvivesARestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	st, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	st.Update(func(s *Settings) { s.SaveJPEGLossless = false })
	if err := st.Flush(); err != nil {
		t.Fatal(err)
	}

	again, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	if again.Get().SaveJPEGLossless {
		t.Fatal("the switch came back on after a restart")
	}
}

func TestSchemaVersionStaysTwoWithTheNewSaveField(t *testing.T) {
	if Current != 2 {
		t.Fatalf("the schema version is %d", Current)
	}
}
