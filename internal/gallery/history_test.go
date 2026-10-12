package gallery

import (
	"path/filepath"
	"strconv"
	"testing"
)

func place(name string) Location {
	return Folder(filepath.Join(string(filepath.Separator), "gallery-test", name))
}

func TestNewHistoryStartsAtGivenLocation(t *testing.T) {
	start := place("start")
	h := NewHistory(start)
	if h.Current() != start || h.CanBack() || h.CanForward() {
		t.Fatalf("current %+v, back %v, forward %v", h.Current(), h.CanBack(), h.CanForward())
	}
}

func TestZeroHistoryIsUsable(t *testing.T) {
	var h History
	if !h.Current().IsZero() || h.CanBack() || h.CanForward() || h.CanUp() {
		t.Fatal("zero history must be empty")
	}
	if _, ok := h.Back(); ok {
		t.Fatal("Back on an empty history")
	}
	if _, ok := h.Forward(); ok {
		t.Fatal("Forward on an empty history")
	}
	if _, ok := h.Up(); ok {
		t.Fatal("Up on an empty history")
	}
	a := place("a")
	if !h.Push(a) || h.Current() != a || h.CanBack() {
		t.Fatalf("first push: current %+v", h.Current())
	}
}

func TestPushIgnoresConsecutiveDuplicate(t *testing.T) {
	a, b := place("a"), place("b")
	h := NewHistory(a)
	cases := []struct {
		name string
		loc  Location
		want bool
	}{
		{"same as start", a, false},
		{"new place", b, true},
		{"same as current", b, false},
		{"back to the first", a, true},
	}
	for _, tc := range cases {
		if got := h.Push(tc.loc); got != tc.want {
			t.Fatalf("%s: Push = %v, want %v", tc.name, got, tc.want)
		}
	}
	steps := 0
	for h.CanBack() {
		h.Back()
		steps++
	}
	if steps != 2 {
		t.Fatalf("history has %d steps back, want 2", steps)
	}
}

func TestPushTreatsVirtualPlacesAsEqualRegardlessOfPath(t *testing.T) {
	h := NewHistory(Location{Kind: KindAll})
	if h.Push(Location{Kind: KindAll, Path: "ignored"}) {
		t.Fatal("same virtual place must not be pushed twice")
	}
}

func TestBackAndForwardWalkThroughHistory(t *testing.T) {
	a, b, c := place("a"), place("b"), place("c")
	h := NewHistory(a)
	h.Push(b)
	h.Push(c)
	steps := []struct {
		name    string
		do      func() (Location, bool)
		want    Location
		wantOK  bool
		back    bool
		forward bool
	}{
		{"back to b", h.Back, b, true, true, true},
		{"back to a", h.Back, a, true, false, true},
		{"back at the start", h.Back, Location{}, false, false, true},
		{"forward to b", h.Forward, b, true, true, true},
		{"forward to c", h.Forward, c, true, true, false},
		{"forward at the end", h.Forward, Location{}, false, true, false},
	}
	for _, s := range steps {
		got, ok := s.do()
		if got != s.want || ok != s.wantOK || h.CanBack() != s.back || h.CanForward() != s.forward {
			t.Fatalf("%s: got %+v, %v; back %v, forward %v", s.name, got, ok, h.CanBack(), h.CanForward())
		}
	}
	if h.Current() != c {
		t.Fatalf("current %+v, want %+v", h.Current(), c)
	}
}

func TestPushAfterBackDropsForwardEntries(t *testing.T) {
	a, b, c, d := place("a"), place("b"), place("c"), place("d")
	h := NewHistory(a)
	h.Push(b)
	h.Push(c)
	h.Back()
	h.Back()
	if !h.Push(d) {
		t.Fatal("push after back must succeed")
	}
	if h.CanForward() {
		t.Fatal("forward entries must be dropped")
	}
	if got, ok := h.Back(); !ok || got != a {
		t.Fatalf("back from d = %+v, %v; want a", got, ok)
	}
	if got, ok := h.Forward(); !ok || got != d {
		t.Fatalf("forward from a = %+v, %v; want d", got, ok)
	}
}

func TestHistoryDropsOldestBeyondLimit(t *testing.T) {
	h := NewHistory(place("0"))
	total := HistoryLimit + 50
	for i := 1; i < total; i++ {
		h.Push(place(strconv.Itoa(i)))
	}
	if want := place(strconv.Itoa(total - 1)); h.Current() != want {
		t.Fatalf("current %+v, want %+v", h.Current(), want)
	}
	steps := 0
	var oldest Location
	for h.CanBack() {
		oldest, _ = h.Back()
		steps++
	}
	if steps != HistoryLimit-1 {
		t.Fatalf("%d steps back, want %d", steps, HistoryLimit-1)
	}
	if want := place(strconv.Itoa(total - HistoryLimit)); oldest != want {
		t.Fatalf("oldest kept %+v, want %+v", oldest, want)
	}
}

func TestUpPushesParentIntoHistory(t *testing.T) {
	dir := t.TempDir()
	child := Folder(filepath.Join(dir, "a", "b"))
	h := NewHistory(child)
	if !h.CanUp() {
		t.Fatal("a nested folder can go up")
	}
	parent, ok := h.Up()
	if !ok || parent != Folder(filepath.Join(dir, "a")) || h.Current() != parent {
		t.Fatalf("Up = %+v, %v; current %+v", parent, ok, h.Current())
	}
	if back, ok := h.Back(); !ok || back != child {
		t.Fatalf("Back after Up = %+v, %v; want the child", back, ok)
	}
}

func TestUpIsUnavailableForRootsAndVirtualPlaces(t *testing.T) {
	root := Folder(filepath.VolumeName(t.TempDir()) + string(filepath.Separator))
	cases := []struct {
		name string
		loc  Location
	}{
		{"filesystem root", root},
		{"all images", Location{Kind: KindAll}},
		{"recent", Location{Kind: KindRecent}},
		{"favorites", Location{Kind: KindFavorites}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := NewHistory(tc.loc)
			if h.CanUp() {
				t.Fatal("CanUp must be false")
			}
			if got, ok := h.Up(); ok || !got.IsZero() || h.CanBack() {
				t.Fatalf("Up = %+v, %v; history must stay untouched", got, ok)
			}
		})
	}
}
