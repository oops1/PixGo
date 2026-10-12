package gallery

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func pics(paths ...string) []Item {
	out := make([]Item, len(paths))
	for i, p := range paths {
		out[i] = Item{Name: p, Path: p}
	}
	return out
}

func TestZeroSelectionIsEmpty(t *testing.T) {
	var s Selection
	if s.Index() != -1 || s.Path() != "" {
		t.Fatalf("index %d, path %q", s.Index(), s.Path())
	}
	if _, ok := s.Item(pics("a")); ok {
		t.Fatal("empty selection has no item")
	}
}

func TestSelectionSetRemembersIndexAndPath(t *testing.T) {
	items := pics("/x/a.jpg", "/x/b.jpg", "/x/c.jpg")
	var s Selection
	s.Set(1, items)
	if s.Index() != 1 || s.Path() != "/x/b.jpg" {
		t.Fatalf("index %d, path %q", s.Index(), s.Path())
	}
	if it, ok := s.Item(items); !ok || it.Name != "/x/b.jpg" {
		t.Fatalf("item %+v, %v", it, ok)
	}
	s.Set(0, items)
	if s.Index() != 0 || s.Path() != "/x/a.jpg" {
		t.Fatalf("index %d, path %q after moving", s.Index(), s.Path())
	}
}

func TestSelectionSetOutOfRangeClears(t *testing.T) {
	items := pics("a", "b")
	for _, i := range []int{-1, 2, 99} {
		var s Selection
		s.Set(0, items)
		s.Set(i, items)
		if s.Index() != -1 || s.Path() != "" {
			t.Fatalf("Set(%d) left index %d, path %q", i, s.Index(), s.Path())
		}
	}
	var s Selection
	s.Set(0, nil)
	if s.Index() != -1 {
		t.Fatal("selecting in an empty list must leave nothing selected")
	}
}

func TestSelectionClear(t *testing.T) {
	var s Selection
	s.Set(0, pics("a"))
	s.Clear()
	if s.Index() != -1 || s.Path() != "" {
		t.Fatalf("index %d, path %q", s.Index(), s.Path())
	}
	s.Clear()
	if s.Index() != -1 {
		t.Fatal("clearing twice must be harmless")
	}
}

func TestSelectionItemIgnoresStaleIndex(t *testing.T) {
	var s Selection
	s.Set(2, pics("a", "b", "c"))
	if _, ok := s.Item(pics("a")); ok {
		t.Fatal("index past the new list must not resolve")
	}
}

func TestSelectionRestoreFindsEntryAfterRescan(t *testing.T) {
	before := pics("/x/a.jpg", "/x/b.jpg", "/x/c.jpg", "/x/d.jpg")
	cases := []struct {
		name     string
		selected int
		after    []Item
		want     int
		wantPath string
	}{
		{"same position", 2, pics("/x/a.jpg", "/x/b.jpg", "/x/c.jpg", "/x/d.jpg"), 2, "/x/c.jpg"},
		{"moved down by an insertion", 2, pics("/x/0.jpg", "/x/a.jpg", "/x/b.jpg", "/x/c.jpg", "/x/d.jpg"), 3, "/x/c.jpg"},
		{"moved up by a removal", 2, pics("/x/b.jpg", "/x/c.jpg", "/x/d.jpg"), 1, "/x/c.jpg"},
		{"removed entry falls to the item that took its place", 1, pics("/x/a.jpg", "/x/c.jpg", "/x/d.jpg"), 1, "/x/c.jpg"},
		{"removed last entry falls to the new last", 3, pics("/x/a.jpg", "/x/b.jpg", "/x/c.jpg"), 2, "/x/c.jpg"},
		{"list shrank a lot", 3, pics("/x/a.jpg"), 0, "/x/a.jpg"},
		{"list became empty", 1, nil, -1, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var s Selection
			s.Set(tc.selected, before)
			if got := s.Restore(tc.after); got != tc.want {
				t.Fatalf("Restore = %d, want %d", got, tc.want)
			}
			if s.Index() != tc.want || s.Path() != tc.wantPath {
				t.Fatalf("selection now %d %q, want %d %q", s.Index(), s.Path(), tc.want, tc.wantPath)
			}
		})
	}
}

func TestSelectionRestoreWithoutSelectionStaysEmpty(t *testing.T) {
	var s Selection
	if got := s.Restore(pics("a", "b")); got != -1 || s.Index() != -1 {
		t.Fatalf("Restore = %d, index %d", got, s.Index())
	}
}

func TestSelectionRestoreMatchesPathsAccordingToPlatformCase(t *testing.T) {
	dir := t.TempDir()
	original := filepath.Join(dir, "Photo.JPG")
	var s Selection
	s.Set(0, pics(original))
	renamed := strings.ToLower(original)
	want := 0
	if runtime.GOOS == "windows" {
		want = 1
	}
	if got := s.Restore(pics(filepath.Join(dir, "other.jpg"), renamed)); got != want {
		t.Fatalf("Restore = %d, want %d", got, want)
	}
}
