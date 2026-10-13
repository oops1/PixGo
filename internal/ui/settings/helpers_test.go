package settings

import (
	"image"
	"maps"
	"sync"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/i18n"
	data "github.com/oops1/PixGo/internal/settings"
	"github.com/oops1/PixGo/internal/testutil"
	"github.com/oops1/PixGo/internal/ui/theme"
)

const (
	mockW = 457
	mockH = 430
)

type fakeSource struct {
	mu      sync.Mutex
	cfg     data.Settings
	langs   []Language
	formats []Format
	about   About
	disk    int64
	last    time.Time
}

func (f *fakeSource) Values() data.Settings      { return f.cfg }
func (f *fakeSource) Languages() []Language      { return f.langs }
func (f *fakeSource) Formats() []Format          { return f.formats }
func (f *fakeSource) About() About               { return f.about }
func (f *fakeSource) ThumbCacheBytes() int64     { return f.disk }
func (f *fakeSource) LastUpdateCheck() time.Time { return f.last }

func (f *fakeSource) apply(change func(*data.Settings)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s := f.cfg
	s.Formats = maps.Clone(f.cfg.Formats)
	change(&s)
	s.Normalize()
	f.cfg = s
}

func mockConfig() data.Settings {
	s := data.Defaults()
	s.AutoHidePanels = false
	s.Theme = data.ThemeSystem
	s.Language = data.LanguageSystem
	s.TransparentBackground = data.BackgroundChecker
	return s
}

func mockFormats() []Format {
	return []Format{
		{ID: "jpeg", Name: "JPEG", Extensions: []string{".jpg", ".jpeg", ".jpe"}, Available: true, Enabled: true},
		{ID: "png", Name: "PNG", Extensions: []string{".png"}, Available: true, Enabled: true},
		{ID: "gif", Name: "GIF", Extensions: []string{".gif"}, Available: true, Enabled: true},
		{ID: "avif", Name: "AVIF", Extensions: []string{".avif"}, Optional: true, Available: true, Enabled: true},
		{ID: "jxl", Name: "JPEG XL", Extensions: []string{".jxl"}, Optional: true, Available: true, Enabled: false},
		{ID: "heic", Name: "HEIC / HEIF", Extensions: []string{".heic", ".heif"}, Optional: true},
	}
}

func mockAbout() About {
	return About{
		Version: "0.4.0", Revision: "a1b2c3d", Engine: "headless-gui v3.34.0", Go: "go1.27.0",
		Platform: "windows/amd64", Repository: "https://github.com/oops1/PixGo", License: "Apache-2.0",
		Components: []Component{
			{Name: "golang.org/x/image", License: "BSD-3-Clause", URL: "https://pkg.go.dev/golang.org/x/image"},
			{Name: "Open Sans", License: "OFL-1.1"},
		},
	}
}

func newSource() *fakeSource {
	return &fakeSource{
		cfg:     mockConfig(),
		langs:   []Language{{ID: "ru", Name: "Русский"}, {ID: "en", Name: "English"}},
		formats: mockFormats(),
		about:   mockAbout(),
		disk:    12_900_000,
	}
}

type recorder struct {
	calls     []string
	menuItems []widget.MenuItem
	menuAt    [2]int
	urls      []string
}

func (r *recorder) logf(s string) { r.calls = append(r.calls, s) }

func (r *recorder) has(s string) bool {
	for _, c := range r.calls {
		if c == s {
			return true
		}
	}
	return false
}

func (r *recorder) count(s string) int {
	n := 0
	for _, c := range r.calls {
		if c == s {
			n++
		}
	}
	return n
}

func (r *recorder) reset() { r.calls = nil }

type timer struct {
	at   time.Duration
	fn   func()
	dead bool
}

type timers struct{ pending []*timer }

func (t *timers) after(d time.Duration, fn func()) func() {
	tm := &timer{at: d, fn: fn}
	t.pending = append(t.pending, tm)
	return func() { tm.dead = true }
}

func (t *timers) advance(d time.Duration) {
	var keep, fire []*timer
	for _, tm := range t.pending {
		if tm.dead {
			continue
		}
		tm.at -= d
		if tm.at <= 0 {
			fire = append(fire, tm)
		} else {
			keep = append(keep, tm)
		}
	}
	t.pending = keep
	for _, tm := range fire {
		tm.fn()
	}
}

func (t *timers) live() int {
	n := 0
	for _, tm := range t.pending {
		if !tm.dead {
			n++
		}
	}
	return n
}

type rig struct {
	t    testing.TB
	eng  *engine.Engine
	page *Page
	src  *fakeSource
	rec  *recorder
	th   *theme.Theme
	tm   *timers
}

func newRig(t testing.TB, w, h int) *rig {
	t.Helper()
	return newRigWith(t, newSource(), w, h)
}

func newRigWith(t testing.TB, src *fakeSource, w, h int) *rig {
	t.Helper()
	i18n.SetLang("ru")
	eng := testutil.NewEngine(w, h)
	r := &rig{t: t, eng: eng, src: src, rec: &recorder{}, th: theme.Dark(), tm: &timers{}}
	rec := r.rec
	a := Actions{
		Update: func(change func(*data.Settings)) {
			rec.logf("update")
			src.apply(change)
		},
		Back:            func() { rec.logf("back") },
		ClearThumbCache: func() { rec.logf("clear"); src.disk = 0 },
		OpenDefaultApps: func() { rec.logf("defaultApps") },
		CheckUpdates:    func() { rec.logf("checkUpdates") },
		OpenURL:         func(u string) { rec.logf("url:" + u); rec.urls = append(rec.urls, u) },
		ShowMenu: func(items []widget.MenuItem, x, y int) {
			rec.menuItems = items
			rec.menuAt = [2]int{x, y}
			rec.logf("menu")
		},
	}
	r.page = New(eng, Deps{
		Source:  src,
		Actions: a,
		Theme:   func() *theme.Theme { return r.th },
		After:   r.tm.after,
		Now:     func() time.Time { return time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC) },
	})
	eng.SetRootFullCanvas(r.page)
	r.page.Refresh()
	eng.Flush()
	return r
}

func (r *rig) render() *image.RGBA {
	r.eng.Invalidate()
	r.eng.Flush()
	src := r.eng.RenderOnce()
	out := image.NewRGBA(src.Bounds())
	copy(out.Pix, src.Pix)
	return out
}

func sameImage(a, b *image.RGBA, r image.Rectangle) bool {
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if a.RGBAAt(x, y) != b.RGBAAt(x, y) {
				return false
			}
		}
	}
	return true
}

func (r *rig) item(id string) *item {
	r.t.Helper()
	r.page.ensureLayout()
	it := r.page.itemByID(id)
	if it == nil {
		r.t.Fatalf("no item %q", id)
	}
	return it
}

func (r *rig) center(id string) (int, int) {
	r.t.Helper()
	r.page.reveal(r.item(id).r)
	it := r.item(id)
	b := it.hitRect().Add(image.Pt(0, -r.page.scroll))
	return (b.Min.X + b.Max.X) / 2, (b.Min.Y + b.Max.Y) / 2
}

func (r *rig) click(x, y int) {
	r.page.OnMouseMove(x, y)
	r.page.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft, Pressed: true, Clicks: 1})
	r.page.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft, Pressed: false, Clicks: 1})
}

func (r *rig) clickItem(id string) {
	r.t.Helper()
	x, y := r.center(id)
	r.click(x, y)
}

func (r *rig) key(code widget.KeyCode, mod widget.KeyMod) bool {
	return r.page.HandleKey(widget.KeyEvent{Code: code, Mod: mod, Pressed: true})
}

func (r *rig) tab(n int) {
	for i := 0; i < n; i++ {
		r.key(widget.KeyTab, 0)
	}
}

func (r *rig) focusOn(id string) {
	r.t.Helper()
	r.page.ensureLayout()
	r.page.focus = id
	r.page.keyNav = true
}

func timeAt(y, mo, d, h, mi int) time.Time {
	return time.Date(y, time.Month(mo), d, h, mi, 0, 0, time.UTC)
}

func setLang(l string) { i18n.SetLang(l) }
