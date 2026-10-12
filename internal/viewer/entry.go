package viewer

import (
	"cmp"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type Entry struct {
	Path    string
	Name    string
	Size    int64
	ModTime time.Time
}

type SortMode uint8

const (
	SortName SortMode = iota
	SortDate
	SortSize
)

func ScanDir(dir string, supports func(path string) bool, mode SortMode, desc bool) ([]Entry, error) {
	items, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, 0, len(items))
	for _, item := range items {
		name := item.Name()
		if name == "" || name[0] == '.' || item.IsDir() {
			continue
		}
		path := filepath.Join(dir, name)
		if supports != nil && !supports(path) {
			continue
		}
		info, err := item.Info()
		if err != nil || isHidden(info) {
			continue
		}
		if !item.Type().IsRegular() {
			target, err := os.Stat(path)
			if err != nil || !isFileMode(target.Mode()) {
				continue
			}
			info = target
		}
		entries = append(entries, Entry{Path: path, Name: name, Size: info.Size(), ModTime: info.ModTime()})
	}
	sortEntries(entries, mode, desc)
	return entries, nil
}

func sortEntries(entries []Entry, mode SortMode, desc bool) {
	slices.SortStableFunc(entries, func(a, b Entry) int { return compareEntries(a, b, mode, desc) })
}

func entriesSorted(entries []Entry, mode SortMode, desc bool) bool {
	return slices.IsSortedFunc(entries, func(a, b Entry) int { return compareEntries(a, b, mode, desc) })
}

func compareEntries(a, b Entry, mode SortMode, desc bool) int {
	var primary int
	switch mode {
	case SortDate:
		primary = a.ModTime.Compare(b.ModTime)
	case SortSize:
		primary = cmp.Compare(a.Size, b.Size)
	default:
		primary = CompareNatural(a.Name, b.Name)
		if desc {
			return -primary
		}
		return primary
	}
	if primary != 0 {
		if desc {
			return -primary
		}
		return primary
	}
	return CompareNatural(a.Name, b.Name)
}

func CompareNatural(a, b string) int {
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		if isDigit(a[i]) && isDigit(b[j]) {
			ie, je := i, j
			for ie < len(a) && isDigit(a[ie]) {
				ie++
			}
			for je < len(b) && isDigit(b[je]) {
				je++
			}
			if c := compareDigitRuns(a[i:ie], b[j:je]); c != 0 {
				return c
			}
			i, j = ie, je
			continue
		}
		ra, wa := decodeRune(a[i:])
		rb, wb := decodeRune(b[j:])
		if ra != rb {
			fa, fb := foldRune(ra), foldRune(rb)
			if fa != fb {
				if c := cmp.Compare(runeClass(fa), runeClass(fb)); c != 0 {
					return c
				}
				return cmp.Compare(collationRank(fa), collationRank(fb))
			}
		}
		i += wa
		j += wb
	}
	switch {
	case i < len(a):
		return 1
	case j < len(b):
		return -1
	}
	return strings.Compare(a, b)
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func compareDigitRuns(a, b string) int {
	a = strings.TrimLeft(a, "0")
	b = strings.TrimLeft(b, "0")
	if len(a) != len(b) {
		return cmp.Compare(len(a), len(b))
	}
	return strings.Compare(a, b)
}

func decodeRune(s string) (rune, int) {
	if s[0] < utf8.RuneSelf {
		return rune(s[0]), 1
	}
	return utf8.DecodeRuneInString(s)
}

func foldRune(r rune) rune {
	if r < utf8.RuneSelf {
		if 'A' <= r && r <= 'Z' {
			return r + 'a' - 'A'
		}
		return r
	}
	return unicode.ToLower(unicode.ToUpper(r))
}

func collationRank(r rune) int {
	if r == 'ё' {
		return int('е')*2 + 1
	}
	return int(r) * 2
}

func runeClass(r rune) int {
	switch {
	case '0' <= r && r <= '9':
		return 1
	case unicode.IsLetter(r):
		return 2
	}
	return 0
}
