package gallery

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/oops1/PixGo/internal/i18n"
)

func setLang(t *testing.T, lang string) {
	t.Helper()
	prev := i18n.Lang()
	i18n.SetLang(lang)
	t.Cleanup(func() { i18n.SetLang(prev) })
}

func touch(t *testing.T, path string, size int, mtime time.Time) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}
	if !mtime.IsZero() {
		if err := os.Chtimes(path, mtime, mtime); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func mkdir(t *testing.T, path string) string {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func names(items []Item) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.Name
	}
	return out
}

func relPaths(t *testing.T, root string, items []Item) []string {
	t.Helper()
	out := make([]string, len(items))
	for i, it := range items {
		rel, err := filepath.Rel(root, it.Path)
		if err != nil {
			t.Fatal(err)
		}
		out[i] = filepath.ToSlash(rel)
	}
	return out
}

func byExt(exts ...string) func(string) bool {
	return func(path string) bool { return slices.Contains(exts, strings.ToLower(filepath.Ext(path))) }
}

type fakeEntry struct {
	name    string
	typ     fs.FileMode
	info    fs.FileInfo
	infoErr error
}

func (f fakeEntry) Name() string               { return f.name }
func (f fakeEntry) IsDir() bool                { return f.typ.IsDir() }
func (f fakeEntry) Type() fs.FileMode          { return f.typ }
func (f fakeEntry) Info() (fs.FileInfo, error) { return f.info, f.infoErr }

type fakeInfo struct {
	name string
	mode fs.FileMode
	size int64
}

func (f fakeInfo) Name() string       { return f.name }
func (f fakeInfo) Size() int64        { return f.size }
func (f fakeInfo) Mode() fs.FileMode  { return f.mode }
func (f fakeInfo) ModTime() time.Time { return time.Time{} }
func (f fakeInfo) IsDir() bool        { return f.mode.IsDir() }
func (f fakeInfo) Sys() any           { return nil }
