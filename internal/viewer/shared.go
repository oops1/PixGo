package viewer

import "io/fs"

func SamePath(a, b string) bool {
	return samePath(normalizePath(a), normalizePath(b))
}

func IsHidden(name string, info fs.FileInfo) bool {
	if name == "" || name[0] == '.' {
		return true
	}
	return info != nil && isHidden(info)
}

func IsFile(mode fs.FileMode) bool { return isFileMode(mode) }

func CompareEntries(a, b Entry, mode SortMode, desc bool) int {
	return compareEntries(a, b, mode, desc)
}
