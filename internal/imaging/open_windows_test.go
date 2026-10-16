//go:build windows

package imaging

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtendedPathOnlyForLongAbsolutePaths(t *testing.T) {
	short := `C:\photos\a.jpg`
	if got := extendedPath(short); got != short {
		t.Fatalf("short path changed: %s", got)
	}
	long := `C:\` + strings.Repeat(`folder\`, 50) + `a.jpg`
	if got := extendedPath(long); got != `\\?\`+long {
		t.Fatalf("long path: %s", got)
	}
	unc := `\\server\share\` + strings.Repeat(`folder\`, 50) + `a.jpg`
	if got := extendedPath(unc); !strings.HasPrefix(got, `\\?\UNC\server\share\`) {
		t.Fatalf("unc path: %s", got)
	}
	prefixed := `\\?\` + long
	if got := extendedPath(prefixed); got != prefixed {
		t.Fatalf("prefixed path changed: %s", got)
	}
	relative := strings.Repeat(`folder\`, 50) + `a.jpg`
	if got := extendedPath(relative); got != relative {
		t.Fatalf("relative path changed: %s", got)
	}
}

func TestOpenSharedAllowsDeleteAndRename(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.bin")
	if err := os.WriteFile(p, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := openShared(p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := os.Rename(p, filepath.Join(dir, "b.bin")); err != nil {
		t.Fatalf("rename of an open file: %v", err)
	}
	buf := make([]byte, 4)
	if n, err := f.ReadAt(buf, 0); err != nil || n != 4 || string(buf) != "data" {
		t.Fatalf("read after rename: %d %v %q", n, err, buf)
	}
	if err := os.Remove(filepath.Join(dir, "b.bin")); err != nil {
		t.Fatalf("delete of an open file: %v", err)
	}
}

func TestOpenSharedReportsMissingFile(t *testing.T) {
	_, err := openShared(filepath.Join(t.TempDir(), "missing.bin"))
	if !os.IsNotExist(err) {
		t.Fatalf("err %v", err)
	}
}

func TestOpenSharedReadsLongPaths(t *testing.T) {
	dir := t.TempDir()
	for len(dir) < 300 {
		dir = filepath.Join(dir, "a_rather_long_folder_name")
	}
	if err := os.MkdirAll(extendedPath(dir), 0o755); err != nil {
		t.Skipf("cannot create long directories here: %v", err)
	}
	p := filepath.Join(dir, "a.bin")
	if err := os.WriteFile(extendedPath(p), []byte("data"), 0o644); err != nil {
		t.Skipf("cannot create long files here: %v", err)
	}
	f, err := openShared(p)
	if err != nil {
		t.Fatalf("open of a path with %d characters: %v", len(p), err)
	}
	f.Close()
}
