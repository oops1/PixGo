package settings

import (
	"image"
	"math"
	"time"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/fonts"
	"github.com/oops1/PixGo/internal/i18n"
	data "github.com/oops1/PixGo/internal/settings"
	"github.com/oops1/PixGo/internal/ui/theme"
)

const (
	barHideDelay = 1200 * time.Millisecond
	noteDelay    = 3 * time.Second
	focusBack    = "back"
	focusNav     = "nav"
)

type dragState struct {
	bar  bool
	id   string
	grab int
}

type scrollBar struct {
	shown bool
	stop  func()
}

type Page struct {
	widget.Base
	eng    *engine.Engine
	deps   Deps
	Chrome func(x, y int) (widget.Cursor, bool)

	cfg        data.Settings
	langs      []Language
	formats    []Format
	about      About
	diskBytes  int64
	lastCheck  time.Time
	taken      bool
	builtLang  string
	builtScale float64

	sec      Section
	geo      geometry
	items    []item
	contentH int
	scroll   int
	laidOut  image.Rectangle
	dirty    bool

	hover     hit
	pressed   hit
	focus     string
	keyNav    bool
	drag      *dragState
	bar       scrollBar
	wheelFrac float64

	pics    map[picKey]*image.RGBA
	noted   bool
	stopNot func()
}

func New(eng *engine.Engine, d Deps) *Page {
	return &Page{eng: eng, deps: d, dirty: true, pics: map[picKey]*image.RGBA{}}
}

func (p *Page) theme() *theme.Theme {
	if p.deps.Theme != nil {
		if t := p.deps.Theme(); t != nil {
			return t
		}
	}
	return theme.Dark()
}

func (p *Page) after(d time.Duration, fn func()) func() {
	if p.deps.After == nil {
		return func() {}
	}
	return p.deps.After(d, fn)
}

func (p *Page) invalidate() {
	if p.eng != nil {
		p.eng.InvalidateRect(p.Bounds())
	}
}

func (p *Page) invalidateRect(r image.Rectangle) {
	if p.eng != nil && !r.Empty() {
		p.eng.InvalidateRect(r.Inset(-2).Intersect(p.Bounds()))
	}
}

func (p *Page) take() {
	s := p.deps.Source
	if s == nil {
		return
	}
	p.cfg = s.Values()
	p.langs = s.Languages()
	p.formats = s.Formats()
	p.about = s.About()
	p.diskBytes = s.ThumbCacheBytes()
	p.lastCheck = s.LastUpdateCheck()
	p.taken = true
}

func (p *Page) resync() {
	p.take()
	p.rebuild()
}

func (p *Page) Refresh() {
	p.resync()
	p.invalidate()
}

func (p *Page) SetBounds(r image.Rectangle) {
	p.Base.SetBounds(r)
	p.dirty = true
	if p.taken {
		p.rebuild()
	}
}

func (p *Page) ensureLayout() {
	if !p.taken {
		p.take()
	}
	if p.dirty || p.laidOut != p.Bounds() || p.builtLang != i18n.Lang() || p.builtScale != fonts.TextScale() {
		p.rebuild()
	}
}

func (p *Page) rebuild() {
	syncDims()
	p.geo = computeGeometry(p.Bounds())
	p.builtLang = i18n.Lang()
	p.builtScale = dim.k
	b := &builder{p: p, x0: p.geo.x0, x1: p.geo.x1, xt: p.geo.xt, y: float64(dim.titleTop)}
	b.section(p.sec)
	p.items = b.items
	p.contentH = int(math.Ceil(b.y)) + dim.bottomPad
	p.laidOut = p.Bounds()
	p.dirty = false
	p.scroll = min(max(p.scroll, 0), p.maxScroll())
	p.checkFocus()
}

func (p *Page) checkFocus() {
	if p.focus == "" || p.focus == focusBack || p.focus == focusNav {
		return
	}
	if p.indexOf(p.focus) < 0 {
		p.focus = ""
	}
}

func (p *Page) indexOf(id string) int {
	if id == "" {
		return -1
	}
	for i := range p.items {
		if p.items[i].id == id {
			return i
		}
	}
	return -1
}

func (p *Page) update(change func(*data.Settings)) {
	if f := p.deps.Actions.Update; f != nil {
		f(change)
	}
	p.resync()
	p.invalidate()
}

func (p *Page) Section() Section { return p.sec }

func (p *Page) SetSection(s Section) {
	if s >= sectionCount || (s == p.sec && p.taken) {
		return
	}
	p.sec = s
	p.scroll = 0
	p.wheelFrac = 0
	p.drag = nil
	p.hover, p.pressed = noHit, noHit
	if p.focus != focusBack && p.focus != focusNav {
		p.focus = ""
		if p.keyNav {
			p.focus = focusNav
		}
	}
	if !p.taken {
		p.take()
	}
	p.rebuild()
	p.invalidate()
}

func (p *Page) HeaderHoles() []image.Rectangle {
	return []image.Rectangle{computeGeometry(p.Bounds()).back}
}

func (p *Page) back() {
	if f := p.deps.Actions.Back; f != nil {
		f()
	}
}

func (p *Page) Children() []widget.Widget { return nil }
