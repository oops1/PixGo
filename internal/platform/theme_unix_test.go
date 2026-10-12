//go:build !windows

package platform

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func newTestTheme(t *testing.T, engine func() (bool, bool)) (*fileTheme, string, string) {
	t.Helper()
	dir := t.TempDir()
	th := &fileTheme{
		themePath:  filepath.Join(dir, "theme"),
		accessPath: filepath.Join(dir, "accessibility"),
		detect:     engine,
		poll:       10 * time.Millisecond,
	}
	return th, th.themePath, th.accessPath
}

func TestFileThemeNoFilesUsesEngine(t *testing.T) {
	var calls atomic.Int32
	th, _, _ := newTestTheme(t, func() (bool, bool) { calls.Add(1); return true, true })
	th.refresh(false)
	if !th.Dark() || calls.Load() != 1 {
		t.Fatalf("dark=%v calls=%d", th.Dark(), calls.Load())
	}
	if th.HighContrast() || th.ReduceMotion() {
		t.Fatal("accessibility flags set without a file")
	}
	if _, ok := th.Accent(); ok {
		t.Fatal("accent without a file")
	}
	th.refresh(false)
	if calls.Load() != 1 {
		t.Fatalf("engine asked again without a reason: %d", calls.Load())
	}
	th.refresh(true)
	if calls.Load() != 2 {
		t.Fatalf("engine not re-asked on request: %d", calls.Load())
	}
}

func TestFileThemeUnknownEngineIsLight(t *testing.T) {
	th, _, _ := newTestTheme(t, func() (bool, bool) { return true, false })
	th.refresh(false)
	if th.Dark() {
		t.Fatal("unknown system theme must not become dark")
	}
}

func TestFileThemeModes(t *testing.T) {
	var calls atomic.Int32
	th, themePath, _ := newTestTheme(t, func() (bool, bool) { calls.Add(1); return true, true })
	write := func(s string) {
		t.Helper()
		if err := os.WriteFile(themePath, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
		future := time.Now().Add(time.Duration(len(s)+int(calls.Load())) * time.Second)
		os.Chtimes(themePath, future, future)
		th.refresh(false)
	}
	write("mode = light\n")
	if th.Dark() || calls.Load() != 0 {
		t.Fatalf("light: dark=%v engine calls=%d", th.Dark(), calls.Load())
	}
	write("mode = dark\naccent = #112233\n")
	if !th.Dark() || calls.Load() != 0 {
		t.Fatalf("dark: dark=%v engine calls=%d", th.Dark(), calls.Load())
	}
	if c, ok := th.Accent(); !ok || c.R != 0x11 || c.G != 0x22 || c.B != 0x33 {
		t.Fatalf("accent %v %v", c, ok)
	}
	write("dark = false\n")
	if th.Dark() {
		t.Fatalf("legacy dark = false ignored")
	}
	write("mode = system\n")
	if !th.Dark() || calls.Load() != 1 {
		t.Fatalf("system: dark=%v engine calls=%d", th.Dark(), calls.Load())
	}
}

func TestFileThemeAccessibility(t *testing.T) {
	th, _, accessPath := newTestTheme(t, func() (bool, bool) { return false, true })
	th.refresh(false)
	if th.ReduceMotion() || th.HighContrast() {
		t.Fatal("flags set without a file")
	}
	os.WriteFile(accessPath, []byte("reduce_motion = true\ncontrast = high\n"), 0o644)
	th.refresh(false)
	if !th.ReduceMotion() || !th.HighContrast() {
		t.Fatalf("reduce=%v hc=%v", th.ReduceMotion(), th.HighContrast())
	}
}

func TestFileThemeWatch(t *testing.T) {
	th, themePath, accessPath := newTestTheme(t, func() (bool, bool) { return false, true })
	th.refresh(false)
	if th.Dark() {
		t.Fatal("dark without file")
	}
	changed := make(chan struct{}, 8)
	stop := th.Watch(func() { changed <- struct{}{} })
	defer stop()
	wait := func(what string) {
		t.Helper()
		select {
		case <-changed:
		case <-time.After(5 * time.Second):
			t.Fatalf("%s: no change reported", what)
		}
	}

	if err := os.WriteFile(themePath, []byte("family = windows11\nmode = dark\naccent = #112233\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	wait("theme file")
	if !th.Dark() {
		t.Fatal("not dark after change")
	}
	if c, ok := th.Accent(); !ok || c.R != 0x11 || c.B != 0x33 {
		t.Fatalf("accent %v %v", c, ok)
	}
	os.WriteFile(accessPath, []byte("reduce_motion = yes\n"), 0o644)
	wait("accessibility file")
	if !th.ReduceMotion() {
		t.Fatal("reduce motion not picked up")
	}
}

func TestFileThemeWatchStopTwice(t *testing.T) {
	th, _, _ := newTestTheme(t, nil)
	stop := th.Watch(func() {})
	stop()
	stop()
}

func TestNewSystemThemeSmoke(t *testing.T) {
	t.Setenv("GTK_THEME", "Adwaita:dark")
	t.Setenv("HOME", t.TempDir())
	th := NewSystemTheme()
	if !th.Dark() {
		t.Fatal("GTK_THEME dark ignored by the engine detection")
	}
	if th.HighContrast() || th.ReduceMotion() {
		t.Fatal("unexpected flags")
	}
}
