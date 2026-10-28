package app

import (
	"errors"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/colorm"
	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/imaging"
	"github.com/oops1/PixGo/internal/jobs"
	"github.com/oops1/PixGo/internal/par"
	"github.com/oops1/PixGo/internal/settings"
	"github.com/oops1/PixGo/internal/ui/view"
	"github.com/oops1/PixGo/internal/viewer"
)

type Status uint8

const (
	StatusEmpty Status = iota
	StatusLoading
	StatusReady
	StatusFailed
)

type Env struct {
	Registry   *codec.Registry
	Settings   *settings.Store
	Post       func(func())
	After      func(d time.Duration, fn func()) (stop func())
	Invalidate func()
	Now        func() time.Time

	DisplayProfile colorm.DisplayProvider
}

type Controller struct {
	env    Env
	canvas *view.Canvas
	cache  *imaging.Cache
	loader *imaging.Loader
	color  *colorm.Manager
	runner *jobs.Runner

	folder *viewer.Folder
	nav    *viewer.Navigator

	Favorites *viewer.PathList
	Recent    *viewer.PathList

	current string
	shown   *imaging.Decoded
	pinned  bool
	status  Status
	err     error
	info    codec.Info
	exif    codec.Orientation

	anim     animation
	keepView bool

	slides  *Slides
	shuffle bool
	rnd     *rand.Rand

	lastScan time.Time
	scanGen  uint64
	scanning bool

	unsub    func()
	OnChange func()
	OnLists  func()
}

func NewController(env Env, canvas *view.Canvas) *Controller {
	if env.Now == nil {
		env.Now = time.Now
	}
	st := env.Settings.Get()
	par.Set(threadSetting(st.DecodeThreads))
	c := &Controller{
		env:       env,
		canvas:    canvas,
		cache:     imaging.NewCache(int64(st.CacheMB) << 20),
		runner:    jobs.NewRunner(env.Post),
		Favorites: viewer.NewPathList(0),
		Recent:    viewer.NewPathList(200),
		exif:      codec.OrientNormal,
	}
	c.loader = imaging.NewLoader(env.Registry, c.cache, env.Post, 0, c.limits)
	c.color = colorm.NewManager(env.DisplayProfile)
	c.color.Configure(colorConfig(st))
	c.loader.SetColor(c.color)
	c.loader.SetTileBudget(tileBudgetFor(int64(st.CacheMB) << 20))
	c.unsub = c.loader.Subscribe(c.onLoaderEvent)
	c.anim.owner = c
	seed := uint64(env.Now().UnixNano())
	c.rnd = rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
	c.slides = newSlides(c)
	return c
}

func (c *Controller) Slides() *Slides { return c.slides }

func (c *Controller) Close() {
	c.slides.Close()
	c.anim.stop()
	if c.unsub != nil {
		c.unsub()
	}
	c.loader.Close()
	c.runner.Close()
	c.setShown(nil)
	c.cache.Clear()
}

const (
	cacheBytesPerTiledPixel = 16
	tileBudgetDivisor       = 4
	minTileBudget           = 32 << 20
)

func tileBudgetFor(cacheBytes int64) int64 {
	return min(max(cacheBytes/tileBudgetDivisor, minTileBudget), imaging.DefaultTileBudget)
}

func (c *Controller) limits() codec.Limits {
	st := c.env.Settings.Get()
	l := codec.DefaultLimits()
	if st.MaxPixels > 0 {
		l.MaxPixels = int64(st.MaxPixels) * 1_000_000
	}
	l.MaxTiledPixels = max(l.MaxTiledPixels, l.MaxPixels)
	l.TiledAbove = min(l.MaxPixels, (int64(st.CacheMB)<<20)/cacheBytesPerTiledPixel)
	return l
}

func (c *Controller) changed() {
	if c.OnChange != nil {
		c.OnChange()
	}
}

func (c *Controller) Status() Status { return c.status }
func (c *Controller) Err() error     { return c.err }
func (c *Controller) Path() string   { return c.current }
func (c *Controller) Info() codec.Info {
	return c.info
}
func (c *Controller) Decoded() *imaging.Decoded {
	if c.shown.FullResolution() {
		return c.shown
	}
	return nil
}
func (c *Controller) Shown() *imaging.Decoded { return c.shown }
func (c *Controller) Folder() *viewer.Folder  { return c.folder }
func (c *Controller) Navigator() *viewer.Navigator {
	return c.nav
}
func (c *Controller) Canvas() *view.Canvas { return c.canvas }

func (c *Controller) Position() (index, count int) {
	if c.nav == nil {
		return 0, 0
	}
	return c.nav.Index() + 1, c.nav.Count()
}

func (c *Controller) Name() string {
	if c.current == "" {
		return ""
	}
	return filepath.Base(c.current)
}

func (c *Controller) Title() string {
	if c.current == "" {
		return i18n.T("app.name")
	}
	w, h := c.info.Width, c.info.Height
	if w <= 0 || h <= 0 || c.info.Model {
		return c.Name()
	}
	if c.exif.Valid() {
		w, h = c.exif.Size(w, h)
	}
	return i18n.T("title.dimensions", c.Name(), w, h)
}

func (c *Controller) IsFavorite() bool { return c.current != "" && c.Favorites.Contains(c.current) }

func (c *Controller) initialMode() viewer.ZoomMode {
	switch c.env.Settings.Get().InitialZoom {
	case settings.ZoomActual:
		return viewer.ZoomActual
	case settings.ZoomWidth:
		return viewer.ZoomFitWidth
	}
	return viewer.ZoomFit
}

func (c *Controller) sortMode() viewer.SortMode {
	switch c.env.Settings.Get().Sort {
	case settings.SortDate:
		return viewer.SortDate
	case settings.SortSize:
		return viewer.SortSize
	}
	return viewer.SortName
}

var ErrNothingToShow = errors.New("app: no supported images")

func (c *Controller) Open(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	c.cancelScan()
	info, err := os.Stat(abs)
	if err != nil {
		c.fail(abs, err)
		return err
	}
	if info.IsDir() {
		return c.openDir(abs)
	}
	return c.openFile(abs)
}

func (c *Controller) openFile(abs string) error {
	st := c.env.Settings.Get()
	folder, ok := viewer.SingleFolder(abs, c.env.Registry.Supports, c.sortMode(), st.SortDesc)
	if !ok {
		c.fail(abs, os.ErrNotExist)
		return os.ErrNotExist
	}
	c.folder = folder
	c.nav = viewer.NewNavigator(folder, 0)
	c.nav.SetLoop(st.LoopFolder)
	c.applyOrder()
	c.Recent.Add(abs)
	c.listsChanged()
	c.show(false)
	c.RescanAsync()
	return nil
}

func (c *Controller) openDir(abs string) error {
	st := c.env.Settings.Get()
	folder, idx, err := viewer.OpenFolder(abs, c.env.Registry.Supports, c.sortMode(), st.SortDesc)
	if err != nil {
		return err
	}
	if folder.Len() == 0 {
		c.folder, c.nav = folder, viewer.NewNavigator(folder, 0)
		c.clear()
		return ErrNothingToShow
	}
	c.folder = folder
	c.nav = viewer.NewNavigator(folder, idx)
	c.nav.SetLoop(st.LoopFolder)
	c.applyOrder()
	if e, ok := c.nav.Current(); ok {
		c.Recent.Add(e.Path)
		c.listsChanged()
	}
	c.show(false)
	return nil
}

func (c *Controller) fail(path string, err error) {
	c.anim.stop()
	c.loader.Want(nil)
	c.current = path
	c.setShown(nil)
	c.status = StatusFailed
	c.err = err
	c.info = codec.Info{}
	c.canvas.Clear()
	c.changed()
}

func (c *Controller) clear() {
	c.anim.stop()
	c.current = ""
	c.setShown(nil)
	c.status = StatusEmpty
	c.err = nil
	c.info = codec.Info{}
	c.loader.Want(nil)
	c.canvas.Clear()
	c.changed()
}

func (c *Controller) Step(delta int) bool {
	if c.nav == nil {
		return false
	}
	var moved bool
	switch {
	case delta > 0:
		moved = c.nav.Next()
	case delta < 0:
		moved = c.nav.Prev()
	}
	if moved {
		c.show(c.env.Settings.Get().KeepZoomOnNavigate)
	}
	return moved
}

func (c *Controller) Advance(wrap bool) bool {
	if c.nav == nil || !c.nav.Forward(wrap) {
		return false
	}
	c.show(false)
	return true
}

func (c *Controller) SetShuffle(on bool) {
	c.shuffle = on
	c.applyOrder()
	if c.nav != nil && c.current != "" {
		c.want()
	}
}

func (c *Controller) applyOrder() {
	switch {
	case c.nav == nil:
	case c.shuffle && !c.nav.Shuffled():
		c.nav.Shuffle(c.rnd)
	case !c.shuffle:
		c.nav.Unshuffle()
	}
}

func (c *Controller) First() bool { return c.jump(func() bool { return c.nav.First() }) }
func (c *Controller) Last() bool  { return c.jump(func() bool { return c.nav.Last() }) }

func (c *Controller) Go(i int) bool { return c.jump(func() bool { return c.nav.Go(i) }) }

func (c *Controller) jump(f func() bool) bool {
	if c.nav == nil || !f() {
		return false
	}
	c.show(c.env.Settings.Get().KeepZoomOnNavigate)
	return true
}

func (c *Controller) show(keepView bool) {
	e, ok := c.nav.Current()
	if !ok {
		c.clear()
		return
	}
	changed := e.Path != c.current
	if changed {
		c.current = e.Path
		c.anim.stop()
		c.status = StatusLoading
		c.err = nil
		c.info = codec.Info{}
		if c.shown != nil {
			c.keepView = keepView
		}
	}
	c.want()
	c.sync()
	if changed {
		c.slides.fileChanged()
	}
}

func (c *Controller) want() {
	st := c.env.Settings.Get()
	f := c.canvas.Frame()
	c.loader.SetViewportHint(f.AreaW, f.AreaH)
	reqs := []imaging.Request{{Path: c.current, Prio: imaging.PrioCurrent}}
	for n, i := range c.nav.Neighbors(st.Preload) {
		prio := imaging.PrioNeighbor
		if n >= 2 {
			prio = imaging.PrioPrefetch
		}
		reqs = append(reqs, imaging.Request{Path: c.folder.At(i).Path, Prio: prio})
	}
	c.loader.Want(reqs)
}

func (c *Controller) onLoaderEvent(path string) {
	if path == c.current {
		c.sync()
	}
}

func (c *Controller) sync() {
	if c.current == "" {
		return
	}
	en, ok := c.loader.Entry(c.current)
	if !ok {
		return
	}
	switch en.State {
	case imaging.StateQueued:
		c.status = StatusLoading
	case imaging.StateProbed:
		c.status = StatusLoading
		c.info = en.Info
	case imaging.StatePreview:
		c.status = StatusLoading
		c.info = en.Info
		c.apply(en.Img)
	case imaging.StateFailed:
		c.status = StatusFailed
		c.err = en.Err
		c.info = en.Info
		c.showFailure()
	case imaging.StateReady:
		c.status = StatusReady
		c.err = nil
		c.apply(en.Img)
	}
	c.changed()
}

func (c *Controller) showFailure() {
	c.anim.stop()
	c.setShown(nil)
	c.canvas.Clear()
}

func (c *Controller) setShown(d *imaging.Decoded) {
	prev, wasPinned := c.shown, c.pinned
	c.shown = d
	c.pinned = d != nil && !d.Reduced
	if c.pinned {
		c.cache.Pin(d.Key)
	}
	if prev != nil && wasPinned {
		c.cache.Unpin(prev.Key)
	}
}

func replacesPreview(prev, next *imaging.Decoded) bool {
	if prev == nil || !prev.Reduced || next.Reduced || prev.Key.Path != next.Key.Path {
		return false
	}
	a, b := prev.Source(), next.Source()
	return a.Width == b.Width && a.Height == b.Height
}

func (c *Controller) apply(d *imaging.Decoded) {
	if d == nil || d == c.shown || d.Released() {
		return
	}
	prev := c.shown
	c.info = d.Info
	exif := codec.OrientNormal
	if c.env.Settings.Get().ApplyExifOrientation && d.Info.Orientation.Valid() {
		exif = d.Info.Orientation
	}
	c.exif = exif
	c.canvas.SetPreview(d.Info.Preview)
	switch {
	case replacesPreview(prev, d):
		c.canvas.ReplaceFrame(d.Source())
	case c.keepView:
		c.canvas.SetImageKeepView(d.Source(), exif)
	default:
		c.canvas.SetImage(d.Source(), exif, c.initialMode())
	}
	c.setShown(d)
	c.keepView = false
	c.anim.start(d)
}

func (c *Controller) ToggleFavorite() bool {
	if c.current == "" {
		return false
	}
	on := c.Favorites.Toggle(c.current)
	c.listsChanged()
	c.changed()
	return on
}

func (c *Controller) listsChanged() {
	if c.OnLists != nil {
		c.OnLists()
	}
}

const (
	scanKey        = "scan"
	rescanInterval = 2 * time.Second
)

func (c *Controller) SetActive(active bool) {
	c.anim.setActive(active)
	if active && c.env.Now().Sub(c.lastScan) >= rescanInterval {
		c.RescanAsync()
	}
}

func (c *Controller) Scanning() bool { return c.scanning }

func (c *Controller) cancelScan() {
	c.runner.Cancel(scanKey)
	c.scanning = false
}

func (c *Controller) RescanAsync() {
	if c.folder == nil || c.nav == nil {
		return
	}
	c.lastScan = c.env.Now()
	c.scanGen++
	gen := c.scanGen
	c.scanning = true
	folder := c.folder
	c.runner.Go(scanKey, 0, func(ctx contextT) (any, error) {
		return folder.Scan()
	}, func(v any, err error) {
		if gen != c.scanGen {
			return
		}
		c.scanning = false
		if err != nil || folder != c.folder {
			return
		}
		entries, _ := v.([]viewer.Entry)
		if !folder.Replace(entries) {
			return
		}
		cur := c.current
		if !c.nav.Follow(cur) {
			c.show(false)
			return
		}
		if folder.Len() == 0 {
			c.clear()
			return
		}
		c.want()
		c.changed()
	})
}

func (c *Controller) Async(key string, work func(ctx contextT) (any, error), done func(any, error)) {
	c.runner.Go(key, 0, work, done)
}

func (c *Controller) Removed(path string) {
	c.cache.Remove(path)
	c.Favorites.Remove(path)
	c.Recent.Remove(path)
	if c.folder == nil || c.nav == nil {
		return
	}
	wasCurrent := samePathFold(path, c.current)
	idx := c.nav.Index()
	c.folder.Remove(path)
	if c.folder.Len() == 0 {
		c.clear()
		return
	}
	if !wasCurrent {
		c.nav.Follow(c.current)
		c.want()
		c.changed()
		return
	}
	c.nav.Go(min(idx, c.folder.Len()-1))
	c.current = ""
	c.show(false)
}

func samePathFold(a, b string) bool { return strings.EqualFold(filepath.Clean(a), filepath.Clean(b)) }

func (c *Controller) ExifOrientation() codec.Orientation { return c.exif }

func (c *Controller) ApplySort() {
	if c.folder == nil || c.nav == nil {
		return
	}
	st := c.env.Settings.Get()
	c.folder.SetSort(c.sortMode(), st.SortDesc)
	c.nav.Follow(c.current)
	c.changed()
}

func (c *Controller) LoaderWorkers() int { return c.loader.Workers() }

func (c *Controller) SetViewportHint(w, h int) { c.loader.SetViewportHint(w, h) }

func (c *Controller) LoaderPending() int { return c.loader.Pending() }

func (c *Controller) Limits() codec.Limits { return c.limits() }

func (c *Controller) Registry() *codec.Registry { return c.env.Registry }

func (c *Controller) Supports(path string) bool { return c.env.Registry.Supports(path) }

func (c *Controller) ApplyThreads() {
	par.Set(threadSetting(c.env.Settings.Get().DecodeThreads))
	c.loader.SetWorkers(0)
}

func colorConfig(st settings.Settings) colorm.Config {
	return colorm.Config{Enabled: st.ColorManagement, SystemProfile: st.DisplayProfile == settings.DisplayProfileSystem}
}

func (c *Controller) Color() *colorm.Manager { return c.color }

func (c *Controller) ApplyColor() {
	before := c.color.Stamp()
	c.color.Configure(colorConfig(c.env.Settings.Get()))
	if c.color.Stamp() != before {
		c.loader.ColorChanged()
	}
}

func (c *Controller) ApplyCache() {
	bytes := int64(c.env.Settings.Get().CacheMB) << 20
	c.cache.SetBudget(bytes)
	c.loader.SetTileBudget(tileBudgetFor(bytes))
}

func threadSetting(configured int) int {
	if configured > 0 {
		return configured
	}
	return settings.ThreadsFromEnv()
}
