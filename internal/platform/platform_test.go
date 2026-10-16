package platform

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"image/color"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestParseThemeFile(t *testing.T) {
	src := `
# WinLine theme
  Family = Windows11
mode = Dark
DARK=Yes
; other comment
accent = #0078D4
broken line
[section]
`
	tf := parseThemeFile([]byte(src))
	if tf.family != "windows11" || tf.mode != "dark" || !tf.dark || !tf.darkSet {
		t.Fatalf("got %+v", tf)
	}
	if !tf.hasAccent || tf.accent != (color.RGBA{0x00, 0x78, 0xD4, 0xff}) {
		t.Fatalf("accent %+v", tf.accent)
	}

	tf = parseThemeFile([]byte("family = windows11\ndark = false\n"))
	if tf.dark || !tf.darkSet || tf.mode != "" || tf.hasAccent {
		t.Fatalf("got %+v", tf)
	}
	tf = parseThemeFile([]byte("dark = maybe\naccent = #12\nmode = purple\n"))
	if tf.darkSet || tf.hasAccent || tf.mode != "" {
		t.Fatalf("bad values accepted: %+v", tf)
	}
	if c, ok := parseHexColor("#abc"); !ok || c != (color.RGBA{0xaa, 0xbb, 0xcc, 0xff}) {
		t.Fatalf("short hex %v %v", c, ok)
	}
	if _, ok := parseHexColor("#zzzzzz"); ok {
		t.Fatal("non-hex accepted")
	}
}

func TestParseAccessibilityFile(t *testing.T) {
	af := parseAccessibilityFile([]byte("reduce_motion = true\ncontrast = high\n"))
	if !af.reduceMotion || !af.reduceSet || !af.highContrast || !af.hcSet {
		t.Fatalf("%+v", af)
	}
	af = parseAccessibilityFile([]byte("reduce-motion = off\ncontrast = normal\n"))
	if af.reduceMotion || !af.reduceSet || af.highContrast || !af.hcSet {
		t.Fatalf("%+v", af)
	}
	af = parseAccessibilityFile([]byte("contrast = sparkly\nreduce_motion = ?\n"))
	if af.reduceSet || af.hcSet || af.highContrast {
		t.Fatalf("%+v", af)
	}
	if af := parseAccessibilityFile(nil); af != (accessFile{}) {
		t.Fatalf("%+v", af)
	}
}

func TestResolveTheme(t *testing.T) {
	cases := []struct {
		name    string
		file    string
		sysDark bool
		want    bool
		system  bool
	}{
		{"mode dark beats system", "mode = dark\n", false, true, false},
		{"mode light beats system", "mode = light\n", true, false, false},
		{"mode system follows system", "mode = system\n", true, true, true},
		{"legacy dark", "dark = true\n", false, true, false},
		{"legacy light", "dark = false\n", true, false, false},
		{"mode wins over legacy", "mode = light\ndark = true\n", false, false, false},
		{"no file follows system dark", "", true, true, true},
		{"no file follows system light", "", false, false, true},
		{"mode system ignores legacy", "mode = system\ndark = true\n", false, false, true},
	}
	for _, c := range cases {
		tf := parseThemeFile([]byte(c.file))
		if got := resolveTheme(tf, accessFile{}, c.sysDark).dark; got != c.want {
			t.Errorf("%s: dark = %v, want %v", c.name, got, c.want)
		}
		if tf.wantsSystem() != c.system {
			t.Errorf("%s: wantsSystem = %v, want %v", c.name, tf.wantsSystem(), c.system)
		}
	}
	st := resolveTheme(parseThemeFile([]byte("accent = #112233\n")), accessFile{reduceMotion: true, highContrast: true}, false)
	if !st.reduceMotion || !st.highContrast || !st.hasAccent || st.accent.B != 0x33 {
		t.Fatalf("%+v", st)
	}
}

func TestFilterSpecs(t *testing.T) {
	fs := []FileFilter{
		{Name: "JPEG (*.jpg)", Patterns: []string{"*.jpg", "*.jpeg"}},
		{Name: "PNG", Patterns: []string{" *.png "}},
		{Name: "Все файлы", Patterns: nil},
	}
	got := buildFilterSpecs(fs)
	want := []filterSpec{
		{"JPEG (*.jpg)", "*.jpg;*.jpeg"},
		{"PNG", "*.png"},
		{"Все файлы", "*.*"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
	for i, ext := range []string{"jpg", "png", ""} {
		if g := defaultExt(fs, i); g != ext {
			t.Errorf("defaultExt(%d) = %q", i, g)
		}
	}
	if defaultExt(fs, 7) != "" || defaultExt([]FileFilter{{Patterns: []string{"*.*"}}}, 0) != "" {
		t.Error("defaultExt out of range or wildcard")
	}
	if clampIndex(5, 3) != 0 || clampIndex(2, 3) != 2 || clampIndex(-1, 3) != 0 {
		t.Error("clampIndex")
	}
}

type hwndNode struct{}

func (hwndNode) HWND() uintptr { return 42 }

type handleNode struct{}

func (handleNode) Handle() uintptr { return 7 }

func TestWindowsHandle(t *testing.T) {
	if WindowsHandle(hwndNode{}) != 42 || WindowsHandle(handleNode{}) != 7 ||
		WindowsHandle(3) != 0 || WindowsHandle(nil) != 0 {
		t.Fatal("WindowsHandle")
	}
}

func TestFrameRoundTrip(t *testing.T) {
	var b bytes.Buffer
	in := []string{`C:\Фото\a b.jpg`, "--new-window", ""}
	if err := writeFrame(&b, in); err != nil {
		t.Fatal(err)
	}
	out, err := readFrame(&b)
	if err != nil || !reflect.DeepEqual(in, out) {
		t.Fatalf("got %v %v", out, err)
	}
	b.Reset()
	b.Write([]byte{0xff, 0xff, 0xff, 0x7f})
	if _, err := readFrame(&b); err == nil {
		t.Fatal("oversized frame accepted")
	}
}

func TestSanitizeName(t *testing.T) {
	if g := sanitizeName("../win line/pixgo"); g != "_win_line_pixgo" {
		t.Fatalf("got %q", g)
	}
	if sanitizeName("") != "pixgo" {
		t.Fatal("empty name")
	}
	if sanitizeName("PixGo") != "PixGo" {
		t.Fatal("plain name changed")
	}
}

func randomName(t *testing.T) string {
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatal(err)
	}
	return "pgtest-" + hex.EncodeToString(b[:])
}

func roundTrip(t *testing.T, name string) {
	t.Helper()
	a := NewInstance(name)
	p, err := a.Acquire()
	if err != nil || !p {
		t.Fatalf("first Acquire: %v %v", p, err)
	}
	defer a.Close()
	got := make(chan []string, 4)
	if err := a.Listen(func(args []string) { got <- args }); err != nil {
		t.Fatal(err)
	}

	b := NewInstance(name)
	p, err = b.Acquire()
	if err != nil || p {
		t.Fatalf("second Acquire: %v %v", p, err)
	}
	if err := b.Listen(func([]string) {}); err == nil {
		t.Fatal("secondary Listen succeeded")
	}
	for k, want := range [][]string{
		{"/tmp/один.jpg", "--new-window"},
		{},
	} {
		if err := b.Send(want); err != nil {
			t.Fatalf("Send %d: %v", k, err)
		}
		select {
		case args := <-got:
			if len(want) == 0 && len(args) == 0 {
				continue
			}
			if !reflect.DeepEqual(args, want) {
				t.Fatalf("handler got %v, want %v", args, want)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("handler not called for %d", k)
		}
	}
	b.Close()

	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	c := NewInstance(name)
	p, err = c.Acquire()
	if err != nil || !p {
		t.Fatalf("Acquire after Close: %v %v", p, err)
	}
	c.Close()
	if err := NewInstance(name).Send([]string{"x"}); err == nil {
		t.Fatal("Send with no primary succeeded")
	}
}

func TestInstanceRoundTrip(t *testing.T) {
	isolateInstance(t)
	roundTrip(t, randomName(t))
}

func TestUnsupportedImplementations(t *testing.T) {
	if _, ok, err := (unsupportedSaveDialog{}).Save(SaveOptions{}); ok || err != ErrUnsupported {
		t.Fatal(ok, err)
	}
	if err := (unsupportedAssociations{}).OpenDefaultsSettings(); err != ErrUnsupported {
		t.Fatal(err)
	}
}

func TestPathsWrapFsutil(t *testing.T) {
	d := t.TempDir()
	t.Setenv("PIXGO_CONFIG_DIR", filepath.Join(d, "c"))
	t.Setenv("PIXGO_STATE_DIR", filepath.Join(d, "s"))
	t.Setenv("PIXGO_CACHE_DIR", filepath.Join(d, "k"))
	p := NewPaths()
	if p.ConfigDir() != filepath.Join(d, "c") || p.StateDir() != filepath.Join(d, "s") || p.CacheDir() != filepath.Join(d, "k") {
		t.Fatal(p.ConfigDir(), p.StateDir(), p.CacheDir())
	}
}

func TestLocaleWraps(t *testing.T) {
	l := NewLocale()
	if lang := l.UILanguage(); strings.ContainsAny(lang, "_.") {
		t.Fatalf("language %q is not normalised", lang)
	}
	if r := l.Region(); r != "" && len(r) != 2 {
		t.Fatalf("region %q", r)
	}
}

func TestNewBundlesEverything(t *testing.T) {
	t.Setenv("GTK_THEME", "Adwaita")
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	p := New()
	if p.Trash == nil || p.Theme == nil || p.Locale == nil || p.Folders == nil ||
		p.Volumes == nil || p.SaveDialog == nil || p.Instance == nil || p.Associations == nil || p.Paths == nil {
		t.Fatalf("%+v", p)
	}
	if f := p.Folders.Folders(); f.Home == "" || f.Pictures == "" || f.Desktop == "" || f.Downloads == "" ||
		f.Documents == "" || f.Videos == "" {
		t.Fatalf("%+v", f)
	}
	if vs := p.Volumes.List(); len(vs) == 0 {
		t.Fatal("no volumes")
	}
}

func TestDesktopIDFollowsTheAppIDAndTheEnvironmentWins(t *testing.T) {
	t.Setenv("PIXGO_DESKTOP_ID", "")
	SetDesktopID("")
	if got := DesktopID(); got != DefaultDesktopID {
		t.Fatalf("default: %q", got)
	}
	SetDesktopID("other-name.desktop")
	t.Cleanup(func() { SetDesktopID("") })
	if got := DesktopID(); got != "other-name.desktop" {
		t.Fatalf("set: %q", got)
	}
	t.Setenv("PIXGO_DESKTOP_ID", "env.desktop")
	if got := DesktopID(); got != "env.desktop" {
		t.Fatalf("environment: %q", got)
	}
}
