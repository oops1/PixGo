package platform

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func profileBytes(n int) []byte {
	b := make([]byte, n)
	copy(b[36:], "acsp")
	return b
}

func TestReadProfileFile(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "My Monitor.icm")
	if err := os.WriteFile(good, profileBytes(400), 0o600); err != nil {
		t.Fatal(err)
	}
	data, name := readProfileFile(good)
	if len(data) != 400 || name != "My Monitor" {
		t.Fatalf("got %d bytes %q", len(data), name)
	}
	for label, content := range map[string][]byte{
		"empty":    nil,
		"short":    profileBytes(100)[:100],
		"no acsp":  bytes.Repeat([]byte{1}, 300),
		"too long": append(profileBytes(300), make([]byte, maxDisplayProfileBytes)...),
	} {
		p := filepath.Join(dir, "bad.icc")
		os.WriteFile(p, content, 0o600)
		if d, n := readProfileFile(p); d != nil || n != "" {
			t.Errorf("%s accepted", label)
		}
	}
	if d, _ := readProfileFile(filepath.Join(dir, "missing.icc")); d != nil {
		t.Fatal("missing file")
	}
	if d, _ := readProfileFile(""); d != nil {
		t.Fatal("empty path")
	}
}

func TestDisplayProfileNeverPanics(t *testing.T) {
	icc, name := NewDisplayProfile().Profile()
	if runtime.GOOS != "windows" {
		if icc != nil || name != "" {
			t.Fatalf("only Windows has a monitor profile, got %d bytes %q", len(icc), name)
		}
		return
	}
	if icc != nil && (len(icc) < 128 || string(icc[36:40]) != "acsp") {
		t.Fatalf("not an ICC profile: %d bytes", len(icc))
	}
	if New().Display == nil {
		t.Fatal("platform must carry the display profile reader")
	}
}
