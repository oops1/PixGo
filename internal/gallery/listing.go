package gallery

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/oops1/PixGo/internal/viewer"
)

const (
	DefaultMaxDepth = 3
	DefaultMaxFiles = 5000

	cancelCheckEvery = 64
)

var (
	ErrNoPictures      = errors.New("gallery: pictures folder is not set")
	ErrUnknownLocation = errors.New("gallery: unknown location")
)

type Item struct {
	Name    string
	Path    string
	Dir     bool
	Size    int64
	ModTime time.Time
}

func (i Item) Entry() viewer.Entry {
	return viewer.Entry{Path: i.Path, Name: i.Name, Size: i.Size, ModTime: i.ModTime}
}

type Order struct {
	Mode viewer.SortMode
	Desc bool
}

type Listing struct {
	Loc       Location
	Items     []Item
	Truncated bool
	Limit     int
}

type Lister struct {
	Supports  func(path string) bool
	Pictures  string
	MaxDepth  int
	MaxFiles  int
	Recent    *viewer.PathList
	Favorites *viewer.PathList

	mu    sync.Mutex
	cache map[walkKey]walkResult
}

type walkKey struct {
	root     string
	maxDepth int
	maxFiles int
}

type walkResult struct {
	items     []Item
	truncated bool
}

type entryKind uint8

const (
	kindSkip entryKind = iota
	kindFile
	kindDir
)

func (l *Lister) List(ctx context.Context, loc Location, order Order) (Listing, error) {
	if err := ctx.Err(); err != nil {
		return Listing{}, err
	}
	switch loc.Kind {
	case KindFolder:
		return l.listFolder(ctx, loc, order)
	case KindAll:
		return l.listAll(ctx, loc, order)
	case KindRecent:
		return l.listPaths(ctx, loc, l.Recent)
	case KindFavorites:
		return l.listPaths(ctx, loc, l.Favorites)
	}
	return Listing{}, ErrUnknownLocation
}

func (l *Lister) InvalidateAll() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.cache = nil
}

func (l *Lister) supports(path string) bool {
	return l.Supports == nil || l.Supports(path)
}

func (l *Lister) limits() (depth, files int) {
	depth, files = l.MaxDepth, l.MaxFiles
	if depth <= 0 {
		depth = DefaultMaxDepth
	}
	if files <= 0 {
		files = DefaultMaxFiles
	}
	return depth, files
}

func (l *Lister) listFolder(ctx context.Context, loc Location, order Order) (Listing, error) {
	entries, err := os.ReadDir(loc.Path)
	if err != nil {
		return Listing{}, err
	}
	var dirs, files []Item
	for i, e := range entries {
		if i%cancelCheckEvery == cancelCheckEvery-1 {
			if err := ctx.Err(); err != nil {
				return Listing{}, err
			}
		}
		item, kind, _ := l.classify(loc.Path, e)
		switch kind {
		case kindDir:
			dirs = append(dirs, item)
		case kindFile:
			files = append(files, item)
		}
	}
	sortItems(dirs, Order{Mode: viewer.SortName, Desc: order.Desc && order.Mode == viewer.SortName})
	sortItems(files, order)
	return Listing{Loc: loc, Items: append(dirs, files...)}, nil
}

func (l *Lister) listAll(ctx context.Context, loc Location, order Order) (Listing, error) {
	if l.Pictures == "" {
		return Listing{}, ErrNoPictures
	}
	depth, limit := l.limits()
	key := walkKey{root: l.Pictures, maxDepth: depth, maxFiles: limit}
	res, ok := l.cached(key)
	if !ok {
		items, truncated, err := l.walk(ctx, l.Pictures, depth, limit)
		if err != nil {
			return Listing{}, err
		}
		res = walkResult{items: items, truncated: truncated}
		l.store(key, res)
	}
	items := slices.Clone(res.items)
	sortItems(items, order)
	return Listing{Loc: loc, Items: items, Truncated: res.truncated, Limit: limit}, nil
}

func (l *Lister) cached(key walkKey) (walkResult, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	res, ok := l.cache[key]
	return res, ok
}

func (l *Lister) store(key walkKey, res walkResult) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.cache == nil {
		l.cache = map[walkKey]walkResult{}
	}
	l.cache[key] = res
}

type dirJob struct {
	path  string
	depth int
}

func (l *Lister) walk(ctx context.Context, root string, maxDepth, maxFiles int) ([]Item, bool, error) {
	queue := []dirJob{{path: root}}
	var items []Item
	for head := 0; head < len(queue); head++ {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		job := queue[head]
		entries, err := os.ReadDir(job.path)
		if err != nil {
			if head == 0 {
				return nil, false, err
			}
			continue
		}
		for i, e := range entries {
			if i%cancelCheckEvery == cancelCheckEvery-1 {
				if err := ctx.Err(); err != nil {
					return nil, false, err
				}
			}
			item, kind, linked := l.classify(job.path, e)
			switch kind {
			case kindDir:
				if !linked && job.depth < maxDepth {
					queue = append(queue, dirJob{path: item.Path, depth: job.depth + 1})
				}
			case kindFile:
				if len(items) >= maxFiles {
					return items, true, nil
				}
				items = append(items, item)
			}
		}
	}
	return items, false, nil
}

func (l *Lister) listPaths(ctx context.Context, loc Location, list *viewer.PathList) (Listing, error) {
	if list == nil {
		return Listing{Loc: loc}, nil
	}
	var items []Item
	for i, path := range list.Items() {
		if i%cancelCheckEvery == 0 {
			if err := ctx.Err(); err != nil {
				return Listing{}, err
			}
		}
		if !l.supports(path) {
			continue
		}
		info, err := os.Stat(path)
		if err != nil || !viewer.IsFile(info.Mode()) {
			continue
		}
		items = append(items, Item{Name: filepath.Base(path), Path: path, Size: info.Size(), ModTime: info.ModTime()})
	}
	return Listing{Loc: loc, Items: items}, nil
}

func (l *Lister) classify(dir string, e fs.DirEntry) (Item, entryKind, bool) {
	name := e.Name()
	if viewer.IsHidden(name, nil) {
		return Item{}, kindSkip, false
	}
	path := filepath.Join(dir, name)
	typ := e.Type()
	linked := typ&(fs.ModeSymlink|fs.ModeIrregular) != 0
	switch {
	case typ.IsDir():
	case typ.IsRegular():
		if !l.supports(path) {
			return Item{}, kindSkip, false
		}
	case linked:
	default:
		return Item{}, kindSkip, false
	}
	info, err := e.Info()
	if err != nil || viewer.IsHidden(name, info) {
		return Item{}, kindSkip, false
	}
	if linked {
		if info, err = os.Stat(path); err != nil {
			return Item{}, kindSkip, false
		}
	}
	mode := info.Mode()
	switch {
	case mode.IsDir():
		return Item{Name: name, Path: path, Dir: true, ModTime: info.ModTime()}, kindDir, linked
	case viewer.IsFile(mode) && (!linked || l.supports(path)):
		return Item{Name: name, Path: path, Size: info.Size(), ModTime: info.ModTime()}, kindFile, linked
	}
	return Item{}, kindSkip, false
}

func sortItems(items []Item, order Order) {
	slices.SortStableFunc(items, func(a, b Item) int {
		return viewer.CompareEntries(a.Entry(), b.Entry(), order.Mode, order.Desc)
	})
}
