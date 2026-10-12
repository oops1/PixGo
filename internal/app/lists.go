package app

import (
	"os"
	"path/filepath"

	"github.com/oops1/PixGo/internal/fsutil"
	"github.com/oops1/PixGo/internal/viewer"
)

const recentLimit = 200

func favoritesPath() string { return filepath.Join(fsutil.StateDir(), "favorites.json") }
func recentPath() string    { return filepath.Join(fsutil.StateDir(), "recent.json") }

func writeFile(path string, data []byte) error { return fsutil.WriteAtomic(path, data, 0o644) }

func (a *App) loadLists() {
	fav, _ := viewer.Load(favoritesPath())
	if fav == nil {
		fav = viewer.NewPathList(0)
	}
	rec, _ := viewer.Load(recentPath())
	if rec == nil {
		rec = viewer.NewPathList(recentLimit)
	}
	rec.SetLimit(recentLimit)
	a.Ctl.Favorites, a.Ctl.Recent = fav, rec
}

func (a *App) saveLists() {
	fav, rec := a.Ctl.Favorites, a.Ctl.Recent
	a.Ctl.Async("lists", func(ctx contextT) (any, error) {
		if err := os.MkdirAll(fsutil.StateDir(), 0o755); err != nil {
			return nil, err
		}
		if err := fav.Save(favoritesPath(), writeFile); err != nil {
			return nil, err
		}
		return nil, rec.Save(recentPath(), writeFile)
	}, func(any, error) {})
}
