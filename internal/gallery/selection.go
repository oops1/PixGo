package gallery

import (
	"slices"

	"github.com/oops1/PixGo/internal/viewer"
)

type Selection struct {
	index int
	path  string
	set   bool
}

func (s *Selection) Set(i int, items []Item) {
	if i < 0 || i >= len(items) {
		s.Clear()
		return
	}
	s.index, s.path, s.set = i, items[i].Path, true
}

func (s *Selection) Clear() { *s = Selection{} }

func (s *Selection) Index() int {
	if !s.set {
		return -1
	}
	return s.index
}

func (s *Selection) Path() string { return s.path }

func (s *Selection) Item(items []Item) (Item, bool) {
	if !s.set || s.index >= len(items) {
		return Item{}, false
	}
	return items[s.index], true
}

func (s *Selection) Restore(items []Item) int {
	if !s.set {
		return -1
	}
	if len(items) == 0 {
		s.Clear()
		return -1
	}
	i := slices.IndexFunc(items, func(it Item) bool { return it.Path == s.path })
	if i < 0 {
		i = slices.IndexFunc(items, func(it Item) bool { return viewer.SamePath(it.Path, s.path) })
	}
	if i < 0 {
		i = min(s.index, len(items)-1)
	}
	s.index, s.path = i, items[i].Path
	return i
}
