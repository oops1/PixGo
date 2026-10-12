//go:build windows

package viewer

import (
	"os"
	"path/filepath"
	"slices"
	"syscall"
	"testing"
	"time"
)

func setAttributes(t *testing.T, path string, attrs uint32) {
	t.Helper()
	ptr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.SetFileAttributes(ptr, attrs); err != nil {
		t.Fatal(err)
	}
}

func TestScanDirSkipsHiddenAndSystemFilesOnWindows(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "visible.jpg", 1, time.Time{})
	hidden := writeFile(t, dir, "hidden.jpg", 1, time.Time{})
	system := writeFile(t, dir, "system.jpg", 1, time.Time{})
	both := writeFile(t, dir, "both.jpg", 1, time.Time{})
	setAttributes(t, hidden, syscall.FILE_ATTRIBUTE_HIDDEN)
	setAttributes(t, system, syscall.FILE_ATTRIBUTE_SYSTEM)
	setAttributes(t, both, syscall.FILE_ATTRIBUTE_HIDDEN|syscall.FILE_ATTRIBUTE_SYSTEM)
	got, err := ScanDir(dir, nil, SortName, false)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"visible.jpg"}; !slices.Equal(names(got), want) {
		t.Fatalf("got %v, want %v", names(got), want)
	}
}

func TestOpenFolderIncludesHiddenFileThatWasOpenedExplicitly(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.jpg", 1, time.Time{})
	hidden := writeFile(t, dir, "b.jpg", 1, time.Time{})
	writeFile(t, dir, "c.jpg", 1, time.Time{})
	setAttributes(t, hidden, syscall.FILE_ATTRIBUTE_HIDDEN)
	f, idx, err := OpenFolder(hidden, nil, SortName, false)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"a.jpg", "b.jpg", "c.jpg"}; !slices.Equal(names(f.Entries()), want) || idx != 1 {
		t.Fatalf("got %v at %d", names(f.Entries()), idx)
	}
}

func TestFolderLookupIsCaseInsensitiveOnWindows(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "Photo.JPG", 1, time.Time{})
	f, idx, err := OpenFolder(filepath.Join(dir, "photo.jpg"), nil, SortName, false)
	if err != nil || idx != 0 {
		t.Fatalf("idx %d, err %v", idx, err)
	}
	if f.At(0).Path != path {
		t.Fatalf("entry keeps on-disk spelling: %q, want %q", f.At(0).Path, path)
	}
	if f.IndexOf(filepath.Join(dir, "PHOTO.jpg")) != 0 {
		t.Fatal("IndexOf must ignore case on Windows")
	}
}

func TestIsHiddenReadsWindowsAttributes(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name  string
		attrs uint32
		want  bool
	}{
		{"visible.jpg", syscall.FILE_ATTRIBUTE_NORMAL, false},
		{"hidden.jpg", syscall.FILE_ATTRIBUTE_HIDDEN, true},
		{"system.jpg", syscall.FILE_ATTRIBUTE_SYSTEM, true},
	}
	for _, tc := range cases {
		path := writeFile(t, dir, tc.name, 1, time.Time{})
		setAttributes(t, path, tc.attrs)
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := IsHidden(tc.name, info); got != tc.want {
			t.Errorf("IsHidden(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}
}
