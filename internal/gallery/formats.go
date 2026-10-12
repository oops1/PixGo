package gallery

import (
	"path/filepath"
	"slices"
	"strings"
)

var formatAliases = map[string]string{
	"JPG":  "JPEG",
	"JPE":  "JPEG",
	"JFIF": "JPEG",
	"TIF":  "TIFF",
	"SVGZ": "SVG",
	"DIB":  "BMP",
	"HEIF": "HEIC",
}

func canonicalFormat(ext string) string {
	ext = strings.ToUpper(strings.TrimPrefix(strings.TrimSpace(ext), "."))
	if alias, ok := formatAliases[ext]; ok {
		return alias
	}
	return ext
}

func FormatOf(name string) string {
	return canonicalFormat(filepath.Ext(name))
}

func (l Listing) Formats() []string {
	seen := map[string]bool{}
	var out []string
	for _, item := range l.Items {
		if item.Dir {
			continue
		}
		format := FormatOf(item.Name)
		if format == "" || seen[format] {
			continue
		}
		seen[format] = true
		out = append(out, format)
	}
	slices.Sort(out)
	return out
}

func (l Listing) HasFormat(format string) bool {
	return slices.Contains(l.Formats(), canonicalFormat(format))
}

func FilterFormat(items []Item, format string) []Item {
	format = canonicalFormat(format)
	if format == "" {
		return items
	}
	var out []Item
	for _, item := range items {
		if item.Dir || FormatOf(item.Name) == format {
			out = append(out, item)
		}
	}
	return out
}

func (l Listing) TruncatedText() string {
	if !l.Truncated {
		return ""
	}
	return truncatedText(l.Limit)
}
