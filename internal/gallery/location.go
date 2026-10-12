package gallery

import (
	"path/filepath"

	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/viewer"
)

type Kind uint8

const (
	KindFolder Kind = iota
	KindAll
	KindRecent
	KindFavorites
)

type Location struct {
	Kind Kind
	Path string
}

func Folder(path string) Location { return Location{Kind: KindFolder, Path: path} }

func (l Location) IsZero() bool { return l == Location{} }

func (l Location) Equal(o Location) bool {
	if l.Kind != o.Kind {
		return false
	}
	if l.Kind != KindFolder {
		return true
	}
	return viewer.SamePath(l.Path, o.Path)
}

func (l Location) Display() string {
	switch l.Kind {
	case KindAll:
		return i18n.T("gallery.all")
	case KindRecent:
		return i18n.T("gallery.recent")
	case KindFavorites:
		return i18n.T("gallery.favorites")
	}
	return l.Path
}

func (l Location) Parent() (Location, bool) {
	if l.Kind != KindFolder || l.Path == "" {
		return Location{}, false
	}
	clean := filepath.Clean(l.Path)
	if vol := filepath.VolumeName(clean); vol != "" && clean == vol {
		return Location{}, false
	}
	parent := filepath.Dir(clean)
	if parent == clean || parent == "." {
		return Location{}, false
	}
	return Folder(parent), true
}

func within(path, root string) bool {
	if path == "" || root == "" {
		return false
	}
	for current := filepath.Clean(path); ; {
		if viewer.SamePath(current, root) {
			return true
		}
		parent := filepath.Dir(current)
		if parent == current {
			return false
		}
		current = parent
	}
}
