package viewer

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
)

type Folder struct {
	mu       sync.RWMutex
	dir      string
	supports func(string) bool
	mode     SortMode
	desc     bool
	pinned   string
	entries  []Entry
}

func OpenFolder(path string, supports func(string) bool, mode SortMode, desc bool) (*Folder, int, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, 0, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, 0, err
	}
	f := &Folder{supports: supports, mode: mode, desc: desc}
	if info.IsDir() {
		entries, err := ScanDir(abs, supports, mode, desc)
		if err != nil {
			return nil, 0, err
		}
		f.dir = abs
		f.entries = entries
		return f, 0, nil
	}
	f.dir = filepath.Dir(abs)
	f.pinned = abs
	entries, err := scanWithPinned(f.dir, supports, mode, desc, abs)
	if err != nil {
		entries = nil
		if entry, ok := statEntry(abs); ok {
			entries = []Entry{entry}
		}
	}
	f.entries = entries
	return f, indexOfPath(entries, abs), nil
}

func scanWithPinned(dir string, supports func(string) bool, mode SortMode, desc bool, pinned string) ([]Entry, error) {
	entries, err := ScanDir(dir, supports, mode, desc)
	if err != nil {
		return nil, err
	}
	if pinned != "" && indexOfPath(entries, pinned) < 0 {
		if entry, ok := statEntry(pinned); ok {
			entries = append(entries, entry)
			sortEntries(entries, mode, desc)
		}
	}
	return entries, nil
}

func statEntry(path string) (Entry, bool) {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return Entry{}, false
	}
	return Entry{Path: path, Name: filepath.Base(path), Size: info.Size(), ModTime: info.ModTime()}, true
}

func (f *Folder) Dir() string {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.dir
}

func (f *Folder) Sort() (SortMode, bool) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.mode, f.desc
}

func (f *Folder) Entries() []Entry {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.entries
}

func (f *Folder) Len() int {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return len(f.entries)
}

func (f *Folder) At(i int) Entry {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if i < 0 || i >= len(f.entries) {
		return Entry{}
	}
	return f.entries[i]
}

func (f *Folder) IndexOf(path string) int {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return indexOfPath(f.entries, path)
}

func (f *Folder) Scan() ([]Entry, error) {
	f.mu.RLock()
	dir, supports, mode, desc, pinned := f.dir, f.supports, f.mode, f.desc, f.pinned
	f.mu.RUnlock()
	return scanWithPinned(dir, supports, mode, desc, pinned)
}

func (f *Folder) Replace(entries []Entry) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !entriesSorted(entries, f.mode, f.desc) {
		entries = slices.Clone(entries)
		sortEntries(entries, f.mode, f.desc)
	}
	if slices.EqualFunc(f.entries, entries, sameEntry) {
		return false
	}
	f.entries = entries
	return true
}

func (f *Folder) Rescan() (bool, error) {
	entries, err := f.Scan()
	if err != nil {
		return false, err
	}
	return f.Replace(entries), nil
}

func (f *Folder) Remove(path string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.pinned != "" && samePath(normalizePath(path), f.pinned) {
		f.pinned = ""
	}
	i := indexOfPath(f.entries, path)
	if i < 0 {
		return false
	}
	next := make([]Entry, 0, len(f.entries)-1)
	next = append(next, f.entries[:i]...)
	next = append(next, f.entries[i+1:]...)
	f.entries = next
	return true
}

func (f *Folder) SetSort(mode SortMode, desc bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.mode == mode && f.desc == desc {
		return
	}
	f.mode, f.desc = mode, desc
	next := slices.Clone(f.entries)
	sortEntries(next, mode, desc)
	f.entries = next
}

func sameEntry(a, b Entry) bool {
	return a.Path == b.Path && a.Size == b.Size && a.ModTime.Equal(b.ModTime)
}

func indexOfPath(entries []Entry, path string) int {
	want := normalizePath(path)
	for i := range entries {
		if samePath(entries[i].Path, want) {
			return i
		}
	}
	return -1
}

func normalizePath(path string) string {
	if path == "" {
		return ""
	}
	if !filepath.IsAbs(path) {
		if abs, err := filepath.Abs(path); err == nil {
			return abs
		}
	}
	return filepath.Clean(path)
}

func samePath(a, b string) bool {
	if foldPaths {
		return strings.EqualFold(a, b)
	}
	return a == b
}
