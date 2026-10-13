package gallery

import (
	"image"
	"time"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/fonts"
	model "github.com/oops1/PixGo/internal/gallery"
	"github.com/oops1/PixGo/internal/ui/theme"
)

const (
	typeAheadReset = time.Second
	barHideDelay   = 1200 * time.Millisecond
	bubbleDelay    = 4 * time.Second
)

type snapshot struct {
	loc        model.Location
	items      []model.Item
	loading    bool
	failed     bool
	truncated  string
	sidebar    []model.Entry
	format     string
	mode       ViewMode
	side       int
	status     []string
	canBack    bool
	canForward bool
	canUp      bool
}

type Page struct {
	widget.Base
	eng  *engine.Engine
	deps Deps

	Chrome func(x, y int) (widget.Cursor, bool)

	snap snapshot
	m    textMetrics
	geo  geometry
	tile tileGeometry
	grid model.Grid
	rows []navRow

	scroll    int
	sideY     int
	wheelFrac float64
	sideFrac  float64

	hover   hit
	pressed hit
	pressAt time.Time
	lastTap tap

	tiles  *tileCache
	labels map[labelKey]labelVal
	widths map[widthKey]int

	lastFirst, lastLast int
	prefetched          [2]int

	input   *addressField
	kids    []widget.Widget
	editing bool
	bubble  string
	stopBub func()
	gridBar scrollBar
	navBar  scrollBar

	typed    []rune
	stopType func()

	laidOut image.Rectangle
	dirty   bool
	taken   bool
	reveal  int
	navSeek bool
}

type tap struct {
	index int
	at    time.Time
}

type barDrag struct {
	grab int
}

type scrollBar struct {
	shown bool
	drag  *barDrag
	stop  func()
}

type labelKey struct {
	name string
	w    int
}

type labelVal struct {
	text string
	w    int
}

func New(eng *engine.Engine, d Deps) *Page {
	p := &Page{eng: eng, deps: d, tiles: newTileCache(), labels: map[labelKey]labelVal{}, widths: map[widthKey]int{}, m: baseMetrics(), hover: noHit, pressed: noHit, dirty: true}
	p.lastFirst, p.lastLast = -1, -1
	p.reveal = -1
	p.input = newAddressField(p)
	p.kids = []widget.Widget{p.input}
	return p
}

func (p *Page) src() Source { return p.deps.Source }

func (p *Page) theme() *theme.Theme {
	if p.deps.Theme != nil {
		if t := p.deps.Theme(); t != nil {
			return t
		}
	}
	return theme.Dark()
}

func (p *Page) now() time.Time {
	if p.deps.Now != nil {
		return p.deps.Now()
	}
	return time.Now()
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
	s := p.src()
	if s == nil {
		return
	}
	loc := s.Location()
	moved := p.taken && !p.snap.loc.Equal(loc)
	first := !p.taken
	p.snap = snapshot{
		loc:        loc,
		items:      s.Items(),
		loading:    s.Loading(),
		failed:     s.Failed(),
		truncated:  s.TruncatedText(),
		sidebar:    s.Sidebar(),
		format:     s.Format(),
		mode:       s.ViewMode(),
		side:       s.ThumbSize(),
		status:     s.Status(),
		canBack:    s.CanBack(),
		canForward: s.CanForward(),
		canUp:      s.CanUp(),
	}
	p.dirty = true
	p.taken = true
	if moved || first {
		p.navSeek = true
	}
	if moved {
		p.scroll = 0
		if p.editing {
			p.endEdit()
		}
	}
}

func (p *Page) Refresh() {
	p.resync()
	p.invalidate()
}

func (p *Page) SetBounds(r image.Rectangle) {
	p.Base.SetBounds(r)
	p.dirty = true
	if p.src() != nil {
		p.relayout()
		p.clampSide()
		p.seekNav()
		p.setScrollRaw(p.scroll)
		p.applyReveal()
		p.syncVisible()
		p.placeInput()
	}
}

func (p *Page) Selected() int {
	if s := p.src(); s != nil {
		return s.Selected()
	}
	return -1
}

func (p *Page) selectedItem() (int, model.Item, bool) {
	i := p.Selected()
	if i < 0 || i >= len(p.snap.items) {
		return -1, model.Item{}, false
	}
	return i, p.snap.items[i], true
}

func (p *Page) relayout() {
	b := p.Bounds()
	k := fonts.TextScale()
	if k != p.m.k {
		clear(p.labels)
	}
	m := newTextMetrics(k)
	p.rows = buildNavRows(p.snap.sidebar, m.nav)
	p.m = p.fitMetrics(m, b.Max.X+b.Min.X)
	p.geo = computeGeometry(b, p.m)
	if tg := tileGeometryFor(p.snap.side, p.m); tg != p.tile {
		p.tile = tg
		p.tiles.reset()
	}
	p.grid = p.makeGrid()
	p.laidOut = b
	p.dirty = false
}

func (p *Page) ensureLayout() {
	if !p.taken {
		p.take()
	}
	if p.dirty || p.laidOut != p.Bounds() || fonts.TextScale() != p.m.k {
		p.relayout()
	}
}

func (p *Page) makeGrid() model.Grid {
	vp := p.geo.grid
	g := model.Grid{Count: len(p.snap.items), Width: vp.Dx(), Height: vp.Dy()}
	if p.snap.mode == ViewList {
		g.Metrics = model.Metrics{
			CellW: max(vp.Dx()-gridPadLeft-gridPadRight, 1), CellH: p.m.listRow,
			PadLeft: gridPadLeft, PadRight: gridPadRight, PadTop: 6, PadBottom: 6,
		}
		return g
	}
	g.Metrics = model.Metrics{
		CellW: p.tile.cellW, CellH: p.tile.cellH, GapX: gridGapX, GapY: gridGapY,
		PadLeft: gridPadLeft, PadRight: gridPadRight, PadTop: gridPadTop, PadBottom: gridPadBottom,
	}
	return g
}

func (p *Page) Children() []widget.Widget {
	if p.editing {
		return p.kids
	}
	return nil
}

func (p *Page) HeaderHoles() []image.Rectangle {
	g := computeGeometry(p.Bounds(), p.m)
	nav := image.Rect(g.back.Min.X, g.back.Min.Y, g.up.Max.X, g.back.Max.Y)
	return []image.Rectangle{nav, g.address, g.views, g.thumbs}
}
