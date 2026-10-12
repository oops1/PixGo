package viewer

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSamePathNormalizesBeforeComparing(t *testing.T) {
	dir := t.TempDir()
	sep := string(os.PathSeparator)
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		a, b string
		want bool
	}{
		{"identical", dir, dir, true},
		{"trailing separator", dir + sep, dir, true},
		{"dot dot segment", dir + sep + "x" + sep + ".." + sep + "b", filepath.Join(dir, "b"), true},
		{"different folders", filepath.Join(dir, "a"), filepath.Join(dir, "b"), false},
		{"relative against absolute", ".", wd, true},
		{"empty against folder", "", dir, false},
		{"both empty", "", "", true},
		{"case differs", strings.ToUpper(dir), strings.ToLower(dir), foldPaths},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := SamePath(tc.a, tc.b); got != tc.want {
				t.Fatalf("SamePath(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

type fakeInfo struct {
	name string
	mode fs.FileMode
}

func (f fakeInfo) Name() string       { return f.name }
func (f fakeInfo) Size() int64        { return 0 }
func (f fakeInfo) Mode() fs.FileMode  { return f.mode }
func (f fakeInfo) ModTime() time.Time { return time.Time{} }
func (f fakeInfo) IsDir() bool        { return f.mode.IsDir() }
func (f fakeInfo) Sys() any           { return nil }

func TestIsHiddenTreatsDotNamesAsHidden(t *testing.T) {
	cases := []struct {
		name string
		file string
		info fs.FileInfo
		want bool
	}{
		{"dot file", ".cache.jpg", fakeInfo{name: ".cache.jpg"}, true},
		{"dot folder", ".git", fakeInfo{name: ".git", mode: fs.ModeDir}, true},
		{"empty name", "", fakeInfo{}, true},
		{"plain file", "a.jpg", fakeInfo{name: "a.jpg"}, false},
		{"dot inside the name", "a.b.jpg", fakeInfo{name: "a.b.jpg"}, false},
		{"missing info", "a.jpg", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsHidden(tc.file, tc.info); got != tc.want {
				t.Fatalf("IsHidden(%q) = %v, want %v", tc.file, got, tc.want)
			}
		})
	}
}

func TestIsFileAcceptsOnlyFileModes(t *testing.T) {
	cases := []struct {
		name string
		mode fs.FileMode
		want bool
	}{
		{"regular", 0o644, true},
		{"directory", fs.ModeDir | 0o755, false},
		{"symlink", fs.ModeSymlink, false},
		{"device", fs.ModeDevice, false},
		{"pipe", fs.ModeNamedPipe, false},
		{"socket", fs.ModeSocket, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsFile(tc.mode); got != tc.want {
				t.Fatalf("IsFile(%v) = %v, want %v", tc.mode, got, tc.want)
			}
		})
	}
}

func TestCompareEntriesMatchesScanOrder(t *testing.T) {
	older, newer := time.Unix(1000, 0), time.Unix(2000, 0)
	a := Entry{Name: "img2.jpg", Size: 10, ModTime: newer}
	b := Entry{Name: "img10.jpg", Size: 5, ModTime: older}
	cases := []struct {
		name string
		mode SortMode
		desc bool
		want int
	}{
		{"name ascending is natural", SortName, false, -1},
		{"name descending reverses", SortName, true, 1},
		{"date ascending", SortDate, false, 1},
		{"date descending", SortDate, true, -1},
		{"size ascending", SortSize, false, 1},
		{"size descending", SortSize, true, -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CompareEntries(a, b, tc.mode, tc.desc)
			if (got < 0) != (tc.want < 0) || (got > 0) != (tc.want > 0) {
				t.Fatalf("CompareEntries = %d, want sign %d", got, tc.want)
			}
		})
	}
}
