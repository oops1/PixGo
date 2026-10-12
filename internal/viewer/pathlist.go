package viewer

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"slices"
	"sync"
)

var ErrNoWriter = errors.New("viewer: no write function")

type PathList struct {
	mu    sync.RWMutex
	limit int
	items []string
}

type pathListJSON struct {
	Limit int      `json:"limit,omitempty"`
	Items []string `json:"items"`
}

func NewPathList(limit int) *PathList {
	return &PathList{limit: max(limit, 0)}
}

func Load(path string) (*PathList, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return NewPathList(0), nil
	}
	if err != nil {
		return NewPathList(0), err
	}
	list := NewPathList(0)
	if len(bytes.TrimSpace(data)) == 0 {
		return list, nil
	}
	if err := json.Unmarshal(data, list); err != nil {
		return NewPathList(0), err
	}
	return list, nil
}

func (l *PathList) Save(path string, write func(path string, data []byte) error) error {
	if write == nil {
		return ErrNoWriter
	}
	data, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}
	return write(path, append(data, '\n'))
}

func (l *PathList) Limit() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.limit
}

func (l *PathList) SetLimit(limit int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.limit = max(limit, 0)
	l.trim()
}

func (l *PathList) Len() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.items)
}

func (l *PathList) Items() []string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return slices.Clone(l.items)
}

func (l *PathList) Contains(path string) bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.indexOf(normalizePath(path)) >= 0
}

func (l *PathList) Add(path string) {
	path = normalizePath(path)
	if path == "" {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.remove(path)
	l.pushFront(path)
}

func (l *PathList) Remove(path string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.remove(normalizePath(path))
}

func (l *PathList) Toggle(path string) bool {
	path = normalizePath(path)
	if path == "" {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.remove(path) {
		return false
	}
	l.pushFront(path)
	return true
}

func (l *PathList) Clear() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.items = nil
}

func (l *PathList) MarshalJSON() ([]byte, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	items := l.items
	if items == nil {
		items = []string{}
	}
	return json.Marshal(pathListJSON{Limit: l.limit, Items: items})
}

func (l *PathList) UnmarshalJSON(data []byte) error {
	var raw pathListJSON
	if trimmed := bytes.TrimSpace(data); len(trimmed) > 0 && trimmed[0] == '[' {
		if err := json.Unmarshal(trimmed, &raw.Items); err != nil {
			return err
		}
	} else if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if raw.Limit > 0 {
		l.limit = raw.Limit
	}
	l.items = l.items[:0]
	for _, item := range raw.Items {
		item = normalizePath(item)
		if item == "" || l.indexOf(item) >= 0 {
			continue
		}
		l.items = append(l.items, item)
	}
	l.trim()
	return nil
}

func (l *PathList) indexOf(path string) int {
	for i, item := range l.items {
		if samePath(item, path) {
			return i
		}
	}
	return -1
}

func (l *PathList) remove(path string) bool {
	i := l.indexOf(path)
	if i < 0 {
		return false
	}
	l.items = slices.Delete(l.items, i, i+1)
	return true
}

func (l *PathList) pushFront(path string) {
	l.items = append(l.items, "")
	copy(l.items[1:], l.items)
	l.items[0] = path
	l.trim()
}

func (l *PathList) trim() {
	if l.limit > 0 && len(l.items) > l.limit {
		l.items = l.items[:l.limit]
	}
}
