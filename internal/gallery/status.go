package gallery

import (
	"strings"

	"github.com/oops1/PixGo/internal/i18n"
)

const statusSeparator = " • "

func StatusParts(count int, sel *Item, w, h int) []string {
	if sel == nil {
		return []string{i18n.T("gallery.items", i18n.FormatInt(int64(count)))}
	}
	head := i18n.T("gallery.selected", 1, i18n.Plural(1, i18n.T("unit.items.one"), i18n.T("unit.items.few"), i18n.T("unit.items.many")))
	if sel.Dir {
		return []string{head, i18n.T("gallery.folder")}
	}
	parts := []string{head, i18n.FormatBytes(sel.Size)}
	if w > 0 && h > 0 {
		parts = append(parts, i18n.T("format.dimensions", w, h))
	}
	return parts
}

func StatusText(count int, sel *Item, w, h int) string {
	return strings.Join(StatusParts(count, sel, w, h), statusSeparator)
}

func EmptyText(loc Location, unavailable bool) string {
	if unavailable {
		return i18n.T("gallery.unavailable")
	}
	switch loc.Kind {
	case KindAll:
		return i18n.T("gallery.emptyAll")
	case KindRecent:
		return i18n.T("gallery.emptyRecent")
	case KindFavorites:
		return i18n.T("gallery.emptyFavorites")
	}
	return i18n.T("gallery.empty")
}

func truncatedText(limit int) string {
	return i18n.T("gallery.truncated", i18n.FormatInt(int64(limit)))
}
