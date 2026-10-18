package app

import (
	"image"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"

	"github.com/oops1/headless-gui/v3/widget"
	"github.com/oops1/headless-gui/v3/window"

	"github.com/oops1/PixGo/internal/codec"
	model "github.com/oops1/PixGo/internal/gallery"
	"github.com/oops1/PixGo/internal/platform"
	"github.com/oops1/PixGo/internal/settings"
	"github.com/oops1/PixGo/internal/thumbs"
	uigallery "github.com/oops1/PixGo/internal/ui/gallery"
	uiwindow "github.com/oops1/PixGo/internal/ui/window"
	"github.com/oops1/PixGo/internal/viewer"
)

type galleryState struct {
	a       *App
	page    *uigallery.Page
	hist    *model.History
	lister  *model.Lister
	svc     *thumbs.Service
	unsub   func()
	listing model.Listing
	items   []model.Item
	sel     model.Selection
	format  string
	loading bool
	failed  bool
	gen     int
	dims    map[string][2]int
	volumes []platform.Volume
	folders platform.Folders
	redraw  atomic.Bool
}

func (a *App) ensureGallery() *galleryState {
	if a.gal != nil {
		return a.gal
	}
	g := &galleryState{a: a, dims: map[string][2]int{}}
	g.lister = &model.Lister{
		Supports:  a.Ctl.env.Registry.Supports,
		Recent:    a.Ctl.Recent,
		Favorites: a.Ctl.Favorites,
	}
	opts := thumbs.DefaultOptions()
	opts.DiskLimitBytes = int64(a.Settings.Get().ThumbCacheMB) << 20
	opts.Limits = a.Ctl.limits
	opts.Color = a.Ctl.color
	g.svc = thumbs.New(a.Ctl.env.Registry, a.Eng.Post, opts)
	g.unsub = g.svc.Subscribe(func(string) {
		if g.redraw.CompareAndSwap(false, true) {
			a.Eng.Post(func() {
				g.redraw.Store(false)
				a.Eng.Invalidate()
			})
		}
	})
	g.page = uigallery.New(a.Eng, uigallery.Deps{
		Source:  g,
		Actions: g.actions(),
		Theme:   a.CurrentTheme,
		After:   a.Root.After,
		Now:     a.Root.Now,
	})
	g.page.Chrome = a.Root.Owns
	a.gal = g
	return g
}

func (g *galleryState) close() {
	if g.unsub != nil {
		g.unsub()
	}
	if g.svc != nil {
		g.svc.Close()
	}
}

func (a *App) Shutdown() {
	if a.gal != nil {
		a.gal.close()
	}
	a.Ctl.Close()
}

func (a *App) knownFolders() platform.Folders {
	if a.Platform != nil && a.Platform.Folders != nil {
		return a.Platform.Folders.Folders()
	}
	return platform.Folders{}
}

func (a *App) galleryStart() model.Location {
	if p := a.Ctl.Path(); p != "" {
		return model.Folder(filepath.Dir(p))
	}
	if f := a.knownFolders(); f.Pictures != "" {
		return model.Folder(f.Pictures)
	}
	if home, err := os.UserHomeDir(); err == nil {
		return model.Folder(home)
	}
	return model.Folder(string(filepath.Separator))
}

func (a *App) OpenGallery() { a.OpenGalleryAt(a.galleryStart(), a.Ctl.Path()) }

func (a *App) OpenGalleryAt(loc model.Location, selectPath string) {
	g := a.ensureGallery()
	if a.fullscreen {
		a.SetFullscreen(false)
	}
	if a.page == uiwindow.PageGallery {
		return
	}
	g.refreshEnvironment()
	if g.hist == nil {
		g.hist = model.NewHistory(loc)
	} else {
		g.hist.Push(loc)
	}
	a.page = uiwindow.PageGallery
	g.list(g.hist.Current(), selectPath)
	a.Root.Refresh()
	a.Eng.SetFocus(g.page)
}

func (a *App) CloseGallery() {
	if a.page != uiwindow.PageGallery {
		return
	}
	a.page = uiwindow.PageViewer
	if a.gal != nil {
		a.gal.svc.SetVisible(nil)
	}
	a.Root.Refresh()
	a.Root.FocusContent()
}

func (a *App) GalleryOpen() bool { return a.page == uiwindow.PageGallery }

func (a *App) fileRemoved(path string) {
	if a.gal != nil {
		a.gal.removed(path)
	}
}

func (g *galleryState) refreshEnvironment() {
	g.folders = g.a.knownFolders()
	g.lister.Pictures = g.folders.Pictures
	g.lister.Recent, g.lister.Favorites = g.a.Ctl.Recent, g.a.Ctl.Favorites
	if g.a.Platform != nil && g.a.Platform.Volumes != nil {
		g.volumes = g.a.Platform.Volumes.List()
	}
}

func (g *galleryState) order() model.Order {
	st := g.a.Settings.Get()
	return model.Order{Mode: g.a.Ctl.sortMode(), Desc: st.SortDesc}
}

func (g *galleryState) list(loc model.Location, selectPath string) {
	g.gen++
	gen := g.gen
	g.loading, g.failed = true, false
	order := g.order()
	lister := g.lister
	g.a.Ctl.Async("gallery-list", func(ctx contextT) (any, error) {
		return lister.List(ctx, loc, order)
	}, func(v any, err error) {
		if gen != g.gen {
			return
		}
		g.loading = false
		if err != nil {
			g.failed = true
			g.listing = model.Listing{Loc: loc}
			g.items = nil
			g.sel.Clear()
			g.refresh()
			return
		}
		g.listing, _ = v.(model.Listing)
		if g.format != "" && !g.listing.HasFormat(g.format) {
			g.format = ""
		}
		g.items = model.FilterFormat(g.listing.Items, g.format)
		g.selectInitial(selectPath)
		g.refresh()
	})
	g.refresh()
}

func (g *galleryState) selectInitial(path string) {
	if path != "" {
		for i, it := range g.items {
			if viewer.SamePath(it.Path, path) {
				g.sel.Set(i, g.items)
				g.page.ScrollTo(i)
				g.probeSelected()
				return
			}
		}
	}
	g.sel.Clear()
}

func (g *galleryState) refresh() {
	if g.page != nil {
		g.page.Refresh()
	}
}

func (g *galleryState) reload() {
	path := ""
	if it, ok := g.sel.Item(g.items); ok {
		path = it.Path
	}
	g.lister.InvalidateAll()
	g.list(g.hist.Current(), path)
}

func (g *galleryState) removed(path string) {
	if g.hist == nil || g.a.page != uiwindow.PageGallery {
		return
	}
	for i, it := range g.items {
		if viewer.SamePath(it.Path, path) {
			g.listing.Items = removeItem(g.listing.Items, path)
			g.items = removeItem(g.items, path)
			g.sel.Set(min(i, len(g.items)-1), g.items)
			if len(g.items) == 0 {
				g.sel.Clear()
			}
			g.refresh()
			return
		}
	}
}

func removeItem(items []model.Item, path string) []model.Item {
	out := make([]model.Item, 0, len(items))
	for _, it := range items {
		if !viewer.SamePath(it.Path, path) {
			out = append(out, it)
		}
	}
	return out
}

func (g *galleryState) navigate(loc model.Location, selectPath string) {
	g.hist.Push(loc)
	g.list(loc, selectPath)
}

func (g *galleryState) probeSelected() {
	it, ok := g.sel.Item(g.items)
	if !ok || it.Dir {
		return
	}
	if _, known := g.dims[it.Path]; known {
		return
	}
	path := it.Path
	reg := g.a.Ctl.env.Registry
	lim := g.a.Ctl.limits()
	g.a.Ctl.Async("gallery-probe", func(ctx contextT) (any, error) {
		src, err := codec.OpenFile(path)
		if err != nil {
			return [2]int{}, err
		}
		defer src.Close()
		_, info, err := reg.Probe(ctx, src, lim)
		if err != nil {
			return [2]int{}, err
		}
		w, h := info.Orientation.Size(info.Width, info.Height)
		return [2]int{w, h}, nil
	}, func(v any, err error) {
		d, _ := v.([2]int)
		g.dims[path] = d
		g.refresh()
	})
}

func (g *galleryState) actions() uigallery.Actions {
	a := g.a
	item := func(i int) (model.Item, bool) {
		if i < 0 || i >= len(g.items) {
			return model.Item{}, false
		}
		return g.items[i], true
	}
	return uigallery.Actions{
		Navigate: func(loc model.Location) { g.navigate(loc, "") },
		Back: func() {
			from := g.hist.Current()
			if loc, ok := g.hist.Back(); ok {
				g.list(loc, selectionAfterLeaving(from))
			}
		},
		Forward: func() {
			if loc, ok := g.hist.Forward(); ok {
				g.list(loc, "")
			}
		},
		Up: func() {
			from := g.hist.Current()
			if loc, ok := g.hist.Up(); ok {
				g.list(loc, selectionAfterLeaving(from))
			}
		},
		Select: func(i int) {
			if i < 0 {
				g.sel.Clear()
			} else {
				g.sel.Set(i, g.items)
				g.probeSelected()
			}
			g.refresh()
		},
		Open: func(i int) {
			it, ok := item(i)
			if !ok {
				return
			}
			if it.Dir {
				g.navigate(model.Folder(it.Path), "")
				return
			}
			g.openFile(it.Path)
		},
		OpenNewWindow: func(i int) {
			if it, ok := item(i); ok && !it.Dir {
				openInNewWindow(it.Path)
			}
		},
		Reveal: func(i int) {
			if it, ok := item(i); ok {
				go window.RevealFile(it.Path)
			}
		},
		ToggleFavorite: func(i int) {
			it, ok := item(i)
			if !ok || it.Dir {
				return
			}
			a.Ctl.Favorites.Toggle(it.Path)
			a.Ctl.listsChanged()
			if g.hist.Current().Kind == model.KindFavorites {
				g.reload()
				return
			}
			g.refresh()
		},
		Delete: func(i int) {
			if it, ok := item(i); ok && !it.Dir {
				a.deletePath(it.Path, it.Name, false)
			}
		},
		SetFormat: func(format string) {
			g.format = format
			path := ""
			if it, ok := g.sel.Item(g.items); ok {
				path = it.Path
			}
			g.items = model.FilterFormat(g.listing.Items, g.format)
			g.selectInitial(path)
			g.refresh()
		},
		SetViewMode: func(mode uigallery.ViewMode) {
			view := settings.GalleryGrid
			if mode == uigallery.ViewList {
				view = settings.GalleryList
			}
			a.Settings.Update(func(s *settings.Settings) { s.GalleryView = view })
			g.refresh()
		},
		SetThumbSize: func(px int) {
			size := model.NormalizeThumbSize(px)
			a.Settings.Update(func(s *settings.Settings) { s.GalleryThumbSize = size })
			g.refresh()
		},
		Address: func(text string) error {
			home, _ := os.UserHomeDir()
			addr, err := model.ParseAddress(text, home)
			if err != nil {
				return err
			}
			if addr.File != "" {
				g.openFile(addr.File)
				return nil
			}
			g.navigate(addr.Loc, "")
			return nil
		},
		Refresh:  g.reload,
		Settings: a.OpenSettings,
		Close:    a.CloseGallery,
		Visible: func(first, last int) {
			last = min(last, len(g.items))
			if first < 0 || first >= last {
				g.svc.SetVisible(nil)
				return
			}
			paths := make([]string, 0, last-first)
			for _, it := range g.items[first:last] {
				if !it.Dir {
					paths = append(paths, it.Path)
				}
			}
			g.svc.SetVisible(paths)
		},
		ShowMenu: func(items []widget.MenuItem, x, y int) {
			if a.menu == nil || len(items) == 0 {
				return
			}
			a.menu.SetItems(items)
			a.menu.Show(x, y)
		},
	}
}

func selectionAfterLeaving(from model.Location) string {
	if from.Kind != model.KindFolder {
		return ""
	}
	return from.Path
}

func (g *galleryState) openFile(path string) {
	a := g.a
	a.CloseGallery()
	a.OpenFiles([]string{path})
}

func openInNewWindow(path string) {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	_ = exec.Command(exe, "-new-window", path).Start()
}

func (g *galleryState) Location() model.Location { return g.hist.Current() }

func (g *galleryState) Items() []model.Item { return g.items }

func (g *galleryState) Loading() bool { return g.loading }

func (g *galleryState) Failed() bool { return g.failed }

func (g *galleryState) TruncatedText() string { return g.listing.TruncatedText() }

func (g *galleryState) Sidebar() []model.Entry {
	return model.Sidebar(g.folders, g.volumes, g.hist.Current())
}

func (g *galleryState) Formats() []string { return g.listing.Formats() }

func (g *galleryState) Format() string { return g.format }

func (g *galleryState) ViewMode() uigallery.ViewMode {
	if g.a.Settings.Get().GalleryView == settings.GalleryList {
		return uigallery.ViewList
	}
	return uigallery.ViewGrid
}

func (g *galleryState) ThumbSize() int {
	return model.NormalizeThumbSize(g.a.Settings.Get().GalleryThumbSize)
}

func (g *galleryState) Selected() int { return g.sel.Index() }

func (g *galleryState) Status() []string {
	it, ok := g.sel.Item(g.items)
	if !ok {
		return model.StatusParts(len(g.items), nil, 0, 0)
	}
	d := g.dims[it.Path]
	return model.StatusParts(len(g.items), &it, d[0], d[1])
}

func (g *galleryState) CanBack() bool { return g.hist.CanBack() }

func (g *galleryState) CanForward() bool { return g.hist.CanForward() }

func (g *galleryState) CanUp() bool { return g.hist.CanUp() }

func (g *galleryState) Favorite(path string) bool { return g.a.Ctl.Favorites.Contains(path) }

func (g *galleryState) Thumb(path string, side int, visible bool) (*image.RGBA, bool) {
	t, ok := g.svc.Request(path, side, visible)
	if !ok || t.Err != nil || t.Img == nil {
		return nil, false
	}
	return t.Img, true
}

func (a *App) GalleryBusy() bool {
	return a.gal != nil && a.page == uiwindow.PageGallery && (a.gal.loading || a.gal.svc.Pending() > 0)
}
