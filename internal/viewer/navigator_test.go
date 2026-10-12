package viewer

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func folderOf(count int) *Folder {
	root := filepath.Join(os.TempDir(), "pixgo-fake-folder")
	f := &Folder{dir: root}
	for i := 0; i < count; i++ {
		name := fmt.Sprintf("p%02d.jpg", i)
		f.entries = append(f.entries, Entry{Path: filepath.Join(root, name), Name: name})
	}
	return f
}

func TestNavigatorBasicMovement(t *testing.T) {
	n := NewNavigator(folderOf(4), 1)
	if n.Index() != 1 || n.Count() != 4 {
		t.Fatalf("index %d count %d", n.Index(), n.Count())
	}
	if cur, ok := n.Current(); !ok || cur.Name != "p01.jpg" {
		t.Fatalf("current %+v %v", cur, ok)
	}
	steps := []struct {
		name  string
		do    func() bool
		moved bool
		index int
	}{
		{"next", n.Next, true, 2},
		{"next", n.Next, true, 3},
		{"next at end", n.Next, false, 3},
		{"prev", n.Prev, true, 2},
		{"first", n.First, true, 0},
		{"first again", n.First, false, 0},
		{"prev at start", n.Prev, false, 0},
		{"last", n.Last, true, 3},
		{"last again", n.Last, false, 3},
		{"go 1", func() bool { return n.Go(1) }, true, 1},
		{"go same", func() bool { return n.Go(1) }, false, 1},
		{"go -1", func() bool { return n.Go(-1) }, false, 1},
		{"go past end", func() bool { return n.Go(4) }, false, 1},
	}
	for _, s := range steps {
		if moved := s.do(); moved != s.moved || n.Index() != s.index {
			t.Errorf("%s: moved %v index %d, want %v %d", s.name, moved, n.Index(), s.moved, s.index)
		}
	}
}

func TestNavigatorLoop(t *testing.T) {
	n := NewNavigator(folderOf(3), 2)
	n.SetLoop(true)
	if !n.Loop() || !n.Next() || n.Index() != 0 {
		t.Fatalf("next must wrap, index %d", n.Index())
	}
	if !n.Prev() || n.Index() != 2 {
		t.Fatalf("prev must wrap, index %d", n.Index())
	}
	single := NewNavigator(folderOf(1), 0)
	single.SetLoop(true)
	if single.Next() || single.Prev() || single.Index() != 0 {
		t.Fatal("a single file cannot move")
	}
}

func TestNavigatorEmptyAndNil(t *testing.T) {
	for _, n := range []*Navigator{NewNavigator(folderOf(0), 0), NewNavigator(nil, 3)} {
		n.SetLoop(true)
		if _, ok := n.Current(); ok || n.Count() != 0 || n.Index() != 0 {
			t.Fatal("empty navigator must have nothing current")
		}
		if n.Next() || n.Prev() || n.First() || n.Last() || n.Go(0) || len(n.Neighbors(3)) != 0 {
			t.Fatal("empty navigator must not move")
		}
		if n.Follow(filepath.Join(os.TempDir(), "x")) {
			t.Fatal("Follow in an empty folder cannot succeed")
		}
	}
}

func TestNavigatorClampsInitialIndex(t *testing.T) {
	if got := NewNavigator(folderOf(3), 99).Index(); got != 2 {
		t.Errorf("index %d, want 2", got)
	}
	if got := NewNavigator(folderOf(3), -5).Index(); got != 0 {
		t.Errorf("index %d, want 0", got)
	}
}

func TestNavigatorNeighbors(t *testing.T) {
	cases := []struct {
		name  string
		count int
		index int
		loop  bool
		k     int
		want  []int
	}{
		{"middle k=2", 10, 5, false, 2, []int{6, 4, 7, 3}},
		{"middle k=1", 10, 5, false, 1, []int{6, 4}},
		{"start no loop", 10, 0, false, 2, []int{1, 2}},
		{"end no loop", 10, 9, false, 2, []int{8, 7}},
		{"start loop", 10, 0, true, 2, []int{1, 9, 2, 8}},
		{"end loop", 10, 9, true, 2, []int{0, 8, 1, 7}},
		{"three files loop", 3, 0, true, 2, []int{1, 2}},
		{"three files no loop", 3, 1, false, 5, []int{2, 0}},
		{"two files loop", 2, 0, true, 3, []int{1}},
		{"even ring meets", 4, 0, true, 3, []int{1, 3, 2}},
		{"huge k no loop", 6, 2, false, 1000, []int{3, 1, 4, 0, 5}},
		{"huge k loop", 6, 2, true, 1000, []int{3, 1, 4, 0, 5}},
		{"single", 1, 0, true, 3, nil},
		{"k zero", 5, 2, false, 0, nil},
		{"k negative", 5, 2, false, -1, nil},
	}
	for _, c := range cases {
		n := NewNavigator(folderOf(c.count), c.index)
		n.SetLoop(c.loop)
		got := n.Neighbors(c.k)
		if !slices.Equal(got, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
		seen := map[int]bool{n.Index(): true}
		for _, j := range got {
			if seen[j] || j < 0 || j >= c.count {
				t.Errorf("%s: bad or duplicate index %d in %v", c.name, j, got)
			}
			seen[j] = true
		}
	}
}

func TestNavigatorFollowAfterRescan(t *testing.T) {
	dir := t.TempDir()
	var paths []string
	for _, name := range []string{"a.jpg", "b.jpg", "c.jpg", "d.jpg"} {
		paths = append(paths, writeFile(t, dir, name, 1, time.Time{}))
	}
	f, idx, _ := OpenFolder(paths[2], nil, SortName, false)
	n := NewNavigator(f, idx)
	if n.Index() != 2 {
		t.Fatalf("index %d", n.Index())
	}
	writeFile(t, dir, "0.jpg", 1, time.Time{})
	if _, err := f.Rescan(); err != nil {
		t.Fatal(err)
	}
	if !n.Follow(paths[2]) || n.Index() != 3 {
		t.Fatalf("follow after insertion: index %d", n.Index())
	}
	if err := os.Remove(paths[2]); err != nil {
		t.Fatal(err)
	}
	f.Rescan()
	if n.Follow(paths[2]) {
		t.Fatal("Follow must report a vanished file")
	}
	if cur, _ := n.Current(); n.Index() != 3 || cur.Name != "d.jpg" {
		t.Fatalf("vanished file: index %d current %+v", n.Index(), cur)
	}
	if err := os.Remove(paths[3]); err != nil {
		t.Fatal(err)
	}
	f.Rescan()
	n.Follow(paths[3])
	if cur, ok := n.Current(); !ok || n.Index() != 2 || cur.Name != "b.jpg" {
		t.Fatalf("last file vanished: index %d current %+v", n.Index(), cur)
	}
}

func TestNavigatorSurvivesFolderShrinking(t *testing.T) {
	f := folderOf(5)
	n := NewNavigator(f, 4)
	f.entries = f.entries[:2]
	if _, ok := n.Current(); ok {
		t.Fatal("index past the end has no current entry")
	}
	shrunk := NewNavigator(folderOf(5), 4)
	shrunk.folder.entries = shrunk.folder.entries[:2]
	if !shrunk.Prev() || shrunk.Index() != 0 {
		t.Fatalf("Prev from a stale index must start at the last valid one: %d", shrunk.Index())
	}
	n.Follow(filepath.Join(os.TempDir(), "pixgo-fake-folder", "missing.jpg"))
	if cur, ok := n.Current(); !ok || cur.Name != "p01.jpg" {
		t.Fatalf("Follow must clamp: %+v %v", cur, ok)
	}
}

func TestNavigatorRemoveCurrent(t *testing.T) {
	f := folderOf(3)
	n := NewNavigator(f, 1)
	cur, _ := n.Current()
	f.Remove(cur.Path)
	n.Follow(cur.Path)
	if next, ok := n.Current(); !ok || next.Name != "p02.jpg" || n.Index() != 1 {
		t.Fatalf("after removing the middle file: %+v index %d", next, n.Index())
	}
	cur, _ = n.Current()
	f.Remove(cur.Path)
	n.Follow(cur.Path)
	if last, ok := n.Current(); !ok || last.Name != "p00.jpg" || n.Index() != 0 {
		t.Fatalf("after removing the last file: %+v index %d", last, n.Index())
	}
}
