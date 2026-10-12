package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestDefaultsMatchSpec(t *testing.T) {
	d := Defaults()
	if d.Version != Current || d.Theme != "system" || d.Language != "system" ||
		!d.RememberWindow || !d.RestoreLastImage || !d.ConfirmDelete || !d.AutoHidePanels ||
		d.TransparentBackground != "checker" || !d.SingleWindow {
		t.Fatalf("general defaults: %+v", d)
	}
	if d.InitialZoom != "fit" || d.Wheel != "zoom" || !d.CtrlWheelZoom || d.SmoothZoom || d.Preload != 1 ||
		!d.ApplyExifOrientation || d.KeepZoomOnNavigate || d.LoopFolder || d.Sort != "name" || d.SortDesc ||
		!d.PixelGridAbove400 {
		t.Fatalf("view defaults: %+v", d)
	}
	if d.Slideshow != (Slideshow{Interval: 5, Loop: true, Shuffle: false, Transition: "none"}) {
		t.Fatalf("slideshow defaults: %+v", d.Slideshow)
	}
	if d.MaxPixels != 100 || d.DecodeThreads != 0 || d.ThumbCacheMB != 256 || d.CacheMB < 256 || d.CacheMB > 1024 {
		t.Fatalf("performance defaults: %+v", d)
	}
	if d.AnimationFPS != 10 && d.AnimationFPS != 30 {
		t.Fatalf("animationFPS %d", d.AnimationFPS)
	}
	for _, id := range OptionalFormats {
		if !d.FormatEnabled(id) {
			t.Errorf("format %s disabled by default", id)
		}
	}
	if !d.FormatEnabled("unknown") {
		t.Error("unknown format must default to enabled")
	}
}

func TestDefaultsRoundTrip(t *testing.T) {
	d := Defaults()
	b, err := d.Encode()
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	b2, _ := got.Encode()
	if string(b) != string(b2) {
		t.Fatalf("round trip differs:\n%s\n%s", b, b2)
	}
	for _, key := range []string{`"theme"`, `"rememberWindow"`, `"transparentBackground"`, `"initialZoom"`, `"formats"`,
		`"slideshow"`, `"interval"`, `"cacheMB"`, `"maxPixels"`, `"decodeThreads"`, `"animationFPS"`, `"thumbCacheMB"`} {
		if !strings.Contains(string(b), key) {
			t.Errorf("key %s missing in %s", key, b)
		}
	}
}

func TestUnknownKeysKept(t *testing.T) {
	s, err := Decode([]byte(`{"version":1,"theme":"dark","futureKey":{"a":1}}`))
	if err != nil {
		t.Fatal(err)
	}
	if s.Theme != "dark" {
		t.Fatal(s.Theme)
	}
	b, _ := s.Encode()
	if !strings.Contains(string(b), `"futureKey"`) {
		t.Fatalf("unknown key lost: %s", b)
	}
}

func TestNormalize(t *testing.T) {
	s, _ := Decode([]byte(`{"theme":"purple","language":"Русский","transparentBackground":"x","initialZoom":"zoom",
		"wheel":"spin","preload":9,"sort":"color","cacheMB":3,"maxPixels":0,"decodeThreads":-4,"animationFPS":500,
		"thumbCacheMB":1,"slideshow":{"interval":0,"transition":"wipe"}}`))
	d := Defaults()
	if s.Theme != d.Theme || s.Language != d.Language || s.TransparentBackground != d.TransparentBackground ||
		s.InitialZoom != d.InitialZoom || s.Wheel != d.Wheel || s.Preload != d.Preload || s.Sort != d.Sort ||
		s.CacheMB != d.CacheMB || s.MaxPixels != d.MaxPixels || s.DecodeThreads != d.DecodeThreads ||
		s.AnimationFPS != d.AnimationFPS || s.ThumbCacheMB != d.ThumbCacheMB ||
		s.Slideshow.Interval != d.Slideshow.Interval || s.Slideshow.Transition != d.Slideshow.Transition {
		t.Fatalf("not normalized: %+v", s)
	}
	s, _ = Decode([]byte(`{"language":"de","preload":0,"slideshow":{"interval":60,"loop":false}}`))
	if s.Language != "de" || s.Preload != 0 || s.Slideshow.Interval != 60 || s.Slideshow.Loop {
		t.Fatalf("valid values changed: %+v", s)
	}
}

func TestFormatsNested(t *testing.T) {
	s, err := Decode([]byte(`{"formats":{"avif":{"enabled":false},"jxl":{"enabled":true}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if s.FormatEnabled("avif") || !s.FormatEnabled("jxl") || !s.FormatEnabled("heic") || !s.FormatEnabled("raw") {
		t.Fatalf("formats: %+v", s.Formats)
	}
	b, _ := s.Encode()
	if !strings.Contains(string(b), `"heic"`) {
		t.Fatalf("missing optional format in output: %s", b)
	}
}

func TestCacheMBFor(t *testing.T) {
	const gb = uint64(1) << 30
	for ram, want := range map[uint64]int{
		0:       512,
		gb / 2:  256,
		2 * gb:  512,
		4 * gb:  1024,
		16 * gb: 1024,
		64 * gb: 1024,
	} {
		if got := CacheMBFor(ram); got != want {
			t.Errorf("CacheMBFor(%d) = %d, want %d", ram, got, want)
		}
	}
}

func TestParseMemTotal(t *testing.T) {
	got := parseMemTotal(strings.NewReader("MemFree:  1 kB\nMemTotal:       16318336 kB\nBuffers: 5 kB\n"))
	if got != 16318336<<10 {
		t.Fatalf("got %d", got)
	}
	if parseMemTotal(strings.NewReader("MemTotal: x kB\n")) != 0 || parseMemTotal(strings.NewReader("")) != 0 {
		t.Fatal("bad input accepted")
	}
	if ram := totalRAM(); ram == 0 {
		t.Log("total RAM unknown on this machine")
	} else if ram < 64<<20 {
		t.Fatalf("implausible total RAM %d", ram)
	}
}

func TestInWinLine(t *testing.T) {
	home := t.TempDir()
	h := func() (string, error) { return home, nil }
	if inWinLine([]string{"PATH=/bin"}, h) {
		t.Fatal("plain environment detected as WinLine")
	}
	if !inWinLine([]string{"PATH=/bin", "WINLINE_SESSION=1"}, h) {
		t.Fatal("WINLINE_ variable ignored")
	}
	os.MkdirAll(filepath.Join(home, ".winline"), 0o755)
	if inWinLine(nil, h) {
		t.Fatal("empty directory detected as WinLine")
	}
	os.WriteFile(filepath.Join(home, ".winline", "theme"), []byte("dark = true\n"), 0o644)
	if !inWinLine(nil, h) {
		t.Fatal("theme file ignored")
	}
}

func TestStoreSaveAndNotify(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	st, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	var seen []bool
	st.Subscribe(func(old, new Settings) { seen = append(seen, new.AutoHidePanels) })
	st.Update(func(s *Settings) { s.AutoHidePanels = false })
	if len(seen) != 1 || seen[0] {
		t.Fatalf("subscriber got %v", seen)
	}
	if err := st.Flush(); err != nil {
		t.Fatal(err)
	}
	st2, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if st2.Get().AutoHidePanels {
		t.Fatal("setting not persisted")
	}
}

func TestStoreUpdateIsolatesCopies(t *testing.T) {
	st := NewMemory(Defaults())
	before := st.Get()
	st.Update(func(s *Settings) { s.Formats["avif"] = Format{Enabled: false} })
	if !before.FormatEnabled("avif") {
		t.Fatal("old snapshot was modified through the shared map")
	}
	if st.Get().FormatEnabled("avif") {
		t.Fatal("update lost")
	}
}

func TestStoreUnsubscribeAndExtra(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	os.WriteFile(path, []byte(`{"version":1,"keep":[1,2]}`), 0o600)
	st, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	var n atomic.Int32
	unsub := st.Subscribe(func(old, new Settings) { n.Add(1) })
	st.Update(func(s *Settings) { s.SortDesc = true })
	unsub()
	st.Update(func(s *Settings) { s.SortDesc = false })
	if n.Load() != 1 {
		t.Fatalf("calls %d", n.Load())
	}
	if err := st.Flush(); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), `"keep"`) {
		t.Fatalf("extra key lost after update: %s", b)
	}
}

func TestStoreDebounce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	st, _ := Load(path)
	st.delay = 20 * time.Millisecond
	st.Update(func(s *Settings) { s.Theme = "dark" })
	if _, err := os.Stat(path); err == nil {
		t.Fatal("written before the delay")
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("debounced save never happened")
}

func TestBrokenFileSetAside(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	os.WriteFile(path, []byte("{not json"), 0o600)
	st, err := Load(path)
	if err == nil {
		t.Fatal("expected an error")
	}
	if st.Get().Theme != Defaults().Theme {
		t.Fatal("defaults expected")
	}
	if _, err := os.Stat(path + ".broken"); err != nil {
		t.Fatal("broken file not kept")
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("broken file left in place")
	}
}

func TestMigrateHook(t *testing.T) {
	old := migrations
	defer func() { migrations = old }()
	calls := 0
	migrations = map[int]func(*Settings, map[string]json.RawMessage){
		0: func(s *Settings, raw map[string]json.RawMessage) {
			calls++
			if _, ok := raw["legacyTheme"]; ok {
				s.Theme = ThemeDark
			}
		},
	}
	s, err := Decode([]byte(`{"version":0,"legacyTheme":"dark"}`))
	if err != nil || calls != 1 || s.Theme != ThemeDark || s.Version != Current {
		t.Fatalf("%+v %v calls=%d", s, err, calls)
	}
	if _, err := Decode([]byte(`{"version":1}`)); err != nil || calls != 1 {
		t.Fatalf("current version migrated: calls=%d", calls)
	}
}

func TestNewerVersionRead(t *testing.T) {
	s, err := Decode([]byte(`{"version":99,"theme":"light"}`))
	if err != nil || s.Theme != "light" {
		t.Fatalf("%+v %v", s, err)
	}
}

func TestWindowStateRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "window.json")
	w, err := LoadWindowStateFrom(path)
	if err != nil || w != DefaultWindowState() || w.HasBounds() {
		t.Fatalf("missing file: %+v %v", w, err)
	}
	in := WindowState{X: -1920, Y: 40, W: 1100, H: 700, Maximized: true, LastFile: `D:\Фото\a b.jpg`, InfoPanelWidth: 360}
	if err := in.SaveTo(path); err != nil {
		t.Fatal(err)
	}
	got, err := LoadWindowStateFrom(path)
	if err != nil || got != in || !got.HasBounds() {
		t.Fatalf("got %+v %v", got, err)
	}
}

func TestWindowStateNormalizeAndBroken(t *testing.T) {
	w, err := DecodeWindowState([]byte(`{"x":5,"y":6,"w":-3,"h":400,"infoPanelWidth":5000}`))
	if err != nil || w.HasBounds() || w.X != 0 || w.Y != 0 || w.InfoPanelWidth != DefaultInfoPanelWidth {
		t.Fatalf("%+v %v", w, err)
	}
	path := filepath.Join(t.TempDir(), "window.json")
	os.WriteFile(path, []byte("garbage"), 0o600)
	w, err = LoadWindowStateFrom(path)
	if err == nil || w != DefaultWindowState() {
		t.Fatalf("%+v %v", w, err)
	}
	if _, err := os.Stat(path + ".broken"); err != nil {
		t.Fatal("broken window state not kept")
	}
}

func TestWindowStatePathUsesStateDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PIXGO_STATE_DIR", dir)
	if WindowStatePath() != filepath.Join(dir, "window.json") {
		t.Fatal(WindowStatePath())
	}
	in := WindowState{W: 800, H: 600, InfoPanelWidth: 300}
	if err := in.Save(); err != nil {
		t.Fatal(err)
	}
	got, err := LoadWindowState()
	if err != nil || got != in {
		t.Fatalf("%+v %v", got, err)
	}
}
