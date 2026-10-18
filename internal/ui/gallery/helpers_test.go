package gallery

import (
	"image"
	"image/color"
	"sync"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"

	model "github.com/oops1/PixGo/internal/gallery"
	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/testutil"
	"github.com/oops1/PixGo/internal/ui/theme"
)

type thumbCall struct {
	path    string
	side    int
	visible bool
}

type fakeSource struct {
	mu         sync.Mutex
	loc        model.Location
	items      []model.Item
	loading    bool
	failed     bool
	truncated  string
	sidebar    []model.Entry
	formats    []string
	format     string
	mode       ViewMode
	side       int
	selected   int
	status     []string
	canBack    bool
	canForward bool
	canUp      bool
	favorites  map[string]bool
	images     map[string]*image.RGBA
	calls      []thumbCall
}

func (f *fakeSource) Location() model.Location { return f.loc }
func (f *fakeSource) Items() []model.Item      { return f.items }
func (f *fakeSource) Loading() bool            { return f.loading }
func (f *fakeSource) Failed() bool             { return f.failed }
func (f *fakeSource) TruncatedText() string    { return f.truncated }
func (f *fakeSource) Sidebar() []model.Entry   { return f.sidebar }
func (f *fakeSource) Formats() []string        { return f.formats }
func (f *fakeSource) Format() string           { return f.format }
func (f *fakeSource) ViewMode() ViewMode       { return f.mode }
func (f *fakeSource) ThumbSize() int           { return f.side }
func (f *fakeSource) Selected() int            { return f.selected }
func (f *fakeSource) Status() []string         { return f.status }
func (f *fakeSource) CanBack() bool            { return f.canBack }
func (f *fakeSource) CanForward() bool         { return f.canForward }
func (f *fakeSource) CanUp() bool              { return f.canUp }
func (f *fakeSource) Favorite(p string) bool   { return f.favorites[p] }

func (f *fakeSource) Thumb(path string, side int, visible bool) (*image.RGBA, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, thumbCall{path, side, visible})
	if img, ok := f.images[path]; ok {
		return img, true
	}
	return nil, false
}

func (f *fakeSource) thumbCalls() []thumbCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]thumbCall(nil), f.calls...)
}

type recorder struct {
	calls     []string
	menuItems []widget.MenuItem
	menuAt    [2]int
	visible   [][2]int
	addrErr   error
	addrText  string
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

func indexed(r *recorder, name string) func(int) {
	return func(i int) { r.logf(name + ":" + itoa(i)) }
}

func itoa(i int) string {
	if i < 0 {
		return "-" + itoa(-i)
	}
	if i < 10 {
		return string(rune('0' + i))
	}
	return itoa(i/10) + string(rune('0'+i%10))
}

type rig struct {
	t    testing.TB
	eng  *engine.Engine
	page *Page
	src  *fakeSource
	rec  *recorder
	th   *theme.Theme
	now  time.Time
	tm   *timers
}

type timers struct {
	pending []*timer
}

type timer struct {
	at   time.Duration
	fn   func()
	dead bool
}

func (t *timers) after(d time.Duration, fn func()) func() {
	tm := &timer{at: d, fn: fn}
	t.pending = append(t.pending, tm)
	return func() { tm.dead = true }
}

func (t *timers) advance(d time.Duration) {
	var keep []*timer
	var fire []*timer
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

const (
	mockW = 593
	mockH = 430
)

var mockNames = []string{
	"mountain.jpg", "lake.jpg", "forest.jpg", "flower.jpg",
	"city.jpg", "autumm.jpg", "beach.jpg", "night.jpg",
	"animal.jpg", "macro.jpg", "desert.jpg", "snow.jpg",
}

func mockPath(name string) string { return `D:\Photos\Nature\` + name }

func mockItems() []model.Item {
	items := make([]model.Item, len(mockNames))
	for i, n := range mockNames {
		items[i] = model.Item{Name: n, Path: mockPath(n), Size: 8_730_000, ModTime: time.Date(2024, 9, 14, 18, 27, 0, 0, time.UTC)}
	}
	return items
}

func mockSidebar() []model.Entry {
	return []model.Entry{
		{Label: i18n.T("gallery.all"), Icon: "images", Loc: model.Location{Kind: model.KindAll}},
		{Label: i18n.T("gallery.recent"), Icon: "recent", Loc: model.Location{Kind: model.KindRecent}},
		{Label: i18n.T("gallery.favorites"), Icon: "heart-small", Loc: model.Location{Kind: model.KindFavorites}},
		{Label: i18n.T("gallery.folders"), Header: true},
		{Label: i18n.T("gallery.desktop"), Icon: "desktop", Loc: model.Folder(`C:\Users\u\Desktop`)},
		{Label: i18n.T("gallery.pictures"), Icon: "pictures", Loc: model.Folder(`C:\Users\u\Pictures`)},
		{Label: i18n.T("gallery.videos"), Icon: "video", Loc: model.Folder(`C:\Users\u\Videos`)},
		{Label: i18n.T("gallery.documents"), Icon: "documents", Loc: model.Folder(`C:\Users\u\Documents`)},
		{Label: i18n.T("gallery.downloads"), Icon: "download", Loc: model.Folder(`C:\Users\u\Downloads`)},
		{Label: i18n.T("gallery.computer"), Header: true},
		{Label: "Windows (C:)", Icon: "drive-system", Loc: model.Folder(`C:\`)},
		{Label: "Data (D:)", Icon: "drive", Loc: model.Folder(`D:\`), Selected: true, Expander: true},
		{Label: i18n.T("gallery.network"), Icon: "network", Disabled: true},
	}
}

func mockSource() *fakeSource {
	return &fakeSource{
		loc:       model.Folder(`D:\Photos\Nature`),
		items:     mockItems(),
		sidebar:   mockSidebar(),
		formats:   []string{"JPEG", "PNG"},
		side:      96,
		selected:  0,
		status:    []string{"Выбрано: 1 элемент", "8,73 МБ", "6000 × 4000"},
		canBack:   true,
		canUp:     true,
		favorites: map[string]bool{},
		images:    map[string]*image.RGBA{},
	}
}

var testLang = "ru"

func newRig(t testing.TB, src *fakeSource, w, h int) *rig {
	t.Helper()
	i18n.SetLang(testLang)
	eng := testutil.NewEngine(w, h)
	t.Cleanup(eng.Stop)
	th := theme.Dark()
	r := &rig{t: t, eng: eng, src: src, rec: &recorder{}, th: th, now: time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC), tm: &timers{}}
	rec := r.rec
	a := Actions{
		Navigate:       func(l model.Location) { rec.logf("navigate:" + l.Display()) },
		Back:           func() { rec.logf("back") },
		Forward:        func() { rec.logf("forward") },
		Up:             func() { rec.logf("up") },
		Select:         func(i int) { rec.logf("select:" + itoa(i)); src.selected = i },
		Open:           indexed(rec, "open"),
		OpenNewWindow:  indexed(rec, "openNew"),
		Reveal:         indexed(rec, "reveal"),
		ToggleFavorite: indexed(rec, "favorite"),
		Delete:         indexed(rec, "delete"),
		SetFormat:      func(f string) { rec.logf("format:" + f); src.format = f },
		SetViewMode:    func(m ViewMode) { rec.logf("mode:" + itoa(int(m))); src.mode = m },
		SetThumbSize:   func(px int) { rec.logf("size:" + itoa(px)); src.side = px },
		Address: func(text string) error {
			rec.addrText = text
			rec.logf("address")
			return rec.addrErr
		},
		Refresh:  func() { rec.logf("refresh") },
		Settings: func() { rec.logf("settings") },
		Close:    func() { rec.logf("close") },
		Visible:  func(first, last int) { rec.visible = append(rec.visible, [2]int{first, last}) },
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
		Now:     func() time.Time { return r.now },
	})
	eng.SetRootFullCanvas(r.page)
	r.page.Refresh()
	eng.Flush()
	return r
}

func (r *rig) render() *image.RGBA {
	r.eng.Flush()
	return r.eng.RenderOnce()
}

func (r *rig) click(x, y int) {
	r.page.OnMouseMove(x, y)
	r.page.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft, Pressed: true, Clicks: 1})
	r.page.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft, Pressed: false, Clicks: 1})
}

func (r *rig) doubleClick(x, y int) {
	r.page.OnMouseMove(x, y)
	r.page.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft, Pressed: true, Clicks: 1})
	r.page.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft, Pressed: false, Clicks: 1})
	r.page.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft, Pressed: true, Clicks: 2})
	r.page.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft, Pressed: false, Clicks: 2})
}

func (r *rig) rightClick(x, y int) {
	r.page.OnMouseMove(x, y)
	r.page.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseRight, Pressed: true, Clicks: 1})
	r.page.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseRight, Pressed: false, Clicks: 1})
}

func (r *rig) key(code widget.KeyCode, mod widget.KeyMod) bool {
	return r.page.HandleKey(widget.KeyEvent{Code: code, Mod: mod, Pressed: true})
}

func (r *rig) typeRune(ch rune) bool {
	return r.page.HandleKey(widget.KeyEvent{Code: widget.KeyUnknown, Rune: ch, Pressed: true})
}

func (r *rig) tileCenter(i int) (int, int) {
	r.page.ensureLayout()
	c := r.page.tileRect(i)
	return (c.Min.X + c.Max.X) / 2, c.Min.Y + 20
}

func solid(w, h int, c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, c.A
	}
	return img
}

func manyItems(n int) []model.Item {
	items := make([]model.Item, n)
	for i := range items {
		name := "img" + itoa(i) + ".jpg"
		items[i] = model.Item{Name: name, Path: `D:\Photos\` + name, Size: int64(1000 + i)}
	}
	return items
}

func lightTheme() *theme.Theme { return theme.Light() }

func contrastTheme() *theme.Theme { return theme.HighContrast() }
