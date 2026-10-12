package fsutil

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestWriteAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "a.json")
	if err := WriteAtomic(path, []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteAtomic(path, []byte("two"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "two" {
		t.Fatalf("got %q %v", got, err)
	}
	ents, _ := os.ReadDir(filepath.Dir(path))
	if len(ents) != 1 {
		t.Fatalf("temp files left: %v", ents)
	}
}

func TestEnvOverrides(t *testing.T) {
	d := t.TempDir()
	t.Setenv("PIXGO_CONFIG_DIR", filepath.Join(d, "c"))
	t.Setenv("PIXGO_STATE_DIR", filepath.Join(d, "s"))
	t.Setenv("PIXGO_CACHE_DIR", filepath.Join(d, "k"))
	if ConfigDir() != filepath.Join(d, "c") || StateDir() != filepath.Join(d, "s") || CacheDir() != filepath.Join(d, "k") {
		t.Fatal(ConfigDir(), StateDir(), CacheDir())
	}
}

func TestDefaultLayout(t *testing.T) {
	d := t.TempDir()
	t.Setenv("PIXGO_CONFIG_DIR", "")
	t.Setenv("PIXGO_STATE_DIR", "")
	t.Setenv("PIXGO_CACHE_DIR", "")
	if runtime.GOOS == "windows" {
		t.Setenv("APPDATA", filepath.Join(d, "roaming"))
		t.Setenv("LOCALAPPDATA", filepath.Join(d, "local"))
		if got, want := ConfigDir(), filepath.Join(d, "roaming", "PixGo"); got != want {
			t.Errorf("config %q, want %q", got, want)
		}
		if got, want := StateDir(), filepath.Join(d, "local", "PixGo"); got != want {
			t.Errorf("state %q, want %q", got, want)
		}
		if got, want := CacheDir(), filepath.Join(d, "local", "PixGo", "cache"); got != want {
			t.Errorf("cache %q, want %q", got, want)
		}
		return
	}
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(d, "cfg"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(d, "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(d, "cache"))
	if got, want := ConfigDir(), filepath.Join(d, "cfg", "pixgo"); got != want {
		t.Errorf("config %q, want %q", got, want)
	}
	if got, want := StateDir(), filepath.Join(d, "state", "pixgo"); got != want {
		t.Errorf("state %q, want %q", got, want)
	}
	if got, want := CacheDir(), filepath.Join(d, "cache", "pixgo"); got != want {
		t.Errorf("cache %q, want %q", got, want)
	}
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("XDG_CACHE_HOME", "relative/path")
	t.Setenv("HOME", d)
	if got, want := StateDir(), filepath.Join(d, ".local", "state", "pixgo"); got != want {
		t.Errorf("state fallback %q, want %q", got, want)
	}
	if got := CacheDir(); !strings.HasSuffix(got, filepath.Join(".cache", "pixgo")) {
		t.Errorf("cache fallback %q", got)
	}
}

func TestCreationTimeIsRecentForNewFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "f.txt")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	ct, ok := CreationTime(p)
	if !ok {
		t.Skip("creation time is not available on this filesystem")
	}
	if d := time.Since(ct); d < -time.Minute || d > time.Minute {
		t.Errorf("creation time %v is %v away from now", ct, d)
	}
	if _, ok := CreationTime(filepath.Join(t.TempDir(), "missing")); ok {
		t.Error("creation time reported for a missing file")
	}
}
