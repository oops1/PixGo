package infopanel

import (
	"image"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/render"
	"github.com/oops1/PixGo/internal/ui/gfx"
	"github.com/oops1/PixGo/internal/ui/icons"
	"github.com/oops1/PixGo/internal/ui/theme"
)

const (
	headerX      = 14
	headerBase   = 26
	closeBoxX    = 294
	closeBoxY    = 15
	closeHit     = 28
	thumbX       = 14
	thumbY       = 49
	thumbW       = 80
	thumbH       = 52
	thumbRadius  = 4
	textX        = 111
	nameBase     = 59
	formatBase   = 80
	sizeBase     = 99
	cardX        = 9
	cardW        = 310
	cardTop      = 116
	cardGap      = 2
	cardRadius   = 8
	cardPad      = 12
	titleBase    = 21
	valuePad     = 14
	bottomMargin = 12
	scrollStep   = 60
	barWidth     = 3
	barInset     = 4
)

var (
	headerFont       = gfx.UISemi(10.25)
	nameFont         = gfx.UI(9.5)
	subFont          = gfx.UI(9)
	titleFont        = gfx.UISemi(9.75)
	compactTitleFont = gfx.UISemi(9.25)
	labelFont        = gfx.UI(8)
	valueFont        = gfx.UI(8.5)
	pathFont         = gfx.UI(7.75)
)

type kindMetrics struct {
	title    int
	firstRow int
	pitch    float64
	bottom   int
	valueX   int
}

var metricsByKind = map[CardKind]kindMetrics{
	CardSizes:  {21, 39, 18, 13, 109},
	CardFile:   {21, 40, 23, 12, 109},
	CardCamera: {20, 40, 19.4, 12, 137},
	CardColor:  {20, 40, 19.4, 12, 137},
	CardGPS:    {20, 40, 19.4, 12, 137},
	CardNote:   {20, 30, 19.4, 12, 12},
}

type Engine interface {
	Invalidate()
	InvalidateRect(r image.Rectangle)
}

type hitKind uint8

const (
	hitNone hitKind = iota
	hitClose
	hitCopy
	hitLink
)

type hit struct {
	kind hitKind
	rect image.Rectangle
	text string
	url  string
}

type rowPos struct {
	row      Row
	baseline int
	valueX   int
	copyRect image.Rectangle
}

type cardPos struct {
	card  Card
	rect  image.Rectangle
	title int
	rows  []rowPos
}

type placed struct {
	height int
	cards  []cardPos
}

type Panel struct {
	widget.Base
	eng   Engine
	Theme *theme.Theme

	OnClose   func()
	OnCopy    func(text string)
	OnReveal  func()
	OnOpenURL func(url string)

	model Model

	thumbSrc    render.Source
	thumbOrient codec.Orientation
	thumbBuf    *image.RGBA
	thumbDirty  bool
	scale       float64

	scroll int
	hover  hit
	cache  placed
	cacheW int
}

func New(eng Engine, t *theme.Theme) *Panel {
	return &Panel{eng: eng, Theme: t, scale: 1}
}

func (p *Panel) SetModel(m Model) {
	p.model = m
	p.cacheW = -1
	p.scroll = 0
	p.invalidate()
}

func (p *Panel) Model() Model { return p.model }

func (p *Panel) SetThumbnail(src render.Source, o codec.Orientation) {
	p.thumbSrc = src
	p.thumbOrient = o
	p.thumbDirty = true
	p.invalidate()
}

func (p *Panel) invalidate() {
	if p.eng != nil {
		p.eng.InvalidateRect(p.Bounds())
	}
}

func (p *Panel) place() placed {
	w := p.Bounds().Dx()
	if p.cacheW == w {
		return p.cache
	}
	y := cardTop
	var out placed
	for _, c := range p.model.Cards {
		km := metricsByKind[c.Kind]
		valueX := km.valueX
		for _, r := range c.Rows {
			if c.Kind != CardNote {
				if lw := labelFont.Measure(nil, r.Label) + cardPad + valuePad; lw > valueX {
					valueX = lw
				}
			}
		}
		cp := cardPos{card: c, title: km.title}
		last := km.firstRow
		if c.Kind == CardNote {
			last = km.firstRow
		}
		for i, r := range c.Rows {
			base := km.firstRow + int(float64(i)*km.pitch+.5)
			last = base
			cp.rows = append(cp.rows, rowPos{row: r, baseline: base, valueX: valueX})
		}
		height := last + km.bottom
		if len(c.Rows) == 0 {
			height = titleBase + km.bottom + 8
		}
		cp.rect = image.Rect(cardX, y, cardX+min(cardW, w-2*cardX), y+height)
		y += height + cardGap
		out.cards = append(out.cards, cp)
	}
	out.height = y - cardGap + bottomMargin
	p.cache, p.cacheW = out, w
	return out
}

func (p *Panel) maxScroll() int {
	return max(0, p.place().height-p.Bounds().Dy())
}

func (p *Panel) clampScroll() {
	p.scroll = max(0, min(p.scroll, p.maxScroll()))
}

func (p *Panel) origin() image.Point {
	b := p.Bounds()
	return image.Pt(b.Min.X, b.Min.Y-p.scroll)
}

func (p *Panel) hits() []hit {
	o := p.origin()
	var hs []hit
	cl := image.Rect(o.X+closeBoxX-8, p.Bounds().Min.Y+closeBoxY-8, o.X+closeBoxX-8+closeHit, p.Bounds().Min.Y+closeBoxY-8+closeHit)
	hs = append(hs, hit{kind: hitClose, rect: cl})
	pl := p.place()
	for _, cp := range pl.cards {
		for _, rp := range cp.rows {
			rowTop := o.Y + cp.rect.Min.Y + rp.baseline - 14
			rect := image.Rect(o.X+cp.rect.Min.X+rp.valueX, rowTop, o.X+cp.rect.Max.X-cardPad, rowTop+20)
			if rp.row.Copy != "" {
				cr := image.Rect(o.X+cp.rect.Max.X-27-4, o.Y+cp.rect.Min.Y+rp.baseline-15, o.X+cp.rect.Max.X-27+18, o.Y+cp.rect.Min.Y+rp.baseline+7)
				hs = append(hs, hit{kind: hitCopy, rect: cr, text: rp.row.Copy})
				rect.Max.X = cr.Min.X
			}
			if rp.row.Link {
				hs = append(hs, hit{kind: hitLink, rect: rect, url: rp.row.URL})
			}
		}
	}
	return hs
}

func (p *Panel) hitAt(x, y int) hit {
	if !image.Pt(x, y).In(p.Bounds()) {
		return hit{}
	}
	for _, h := range p.hits() {
		if image.Pt(x, y).In(h.rect) {
			return h
		}
	}
	return hit{}
}

func (p *Panel) Draw(c widget.DrawContext) {
	b := p.Bounds()
	if b.Empty() {
		return
	}
	p.scale = widget.ContextScale(c)
	p.clampScroll()
	t := p.Theme
	c.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), t.Panel)
	restore := gfx.Clip(c, b)
	defer restore()
	o := p.origin()

	headerFont.Draw(c, i18n.T("info.title"), o.X+headerX, o.Y+headerBase, t.PanelValue)
	p.drawClose(c, o)
	p.drawThumb(c, o)

	tx := o.X + textX
	tw := b.Max.X - tx - cardPad
	nameFont.DrawFit(c, p.model.Name, tx, o.Y+nameBase, tw, t.PanelValue)
	subFont.DrawFit(c, p.model.Format, tx, o.Y+formatBase, tw, t.PanelLabel)
	subFont.DrawFit(c, p.model.FileSize, tx, o.Y+sizeBase, tw, t.PanelLabel)

	for _, cp := range p.place().cards {
		p.drawCard(c, o, cp)
	}
	p.drawBar(c)
}

func (p *Panel) drawClose(c widget.DrawContext, o image.Point) {
	t := p.Theme
	box := image.Rect(o.X+closeBoxX-8, p.Bounds().Min.Y+closeBoxY-8, o.X+closeBoxX-8+closeHit, p.Bounds().Min.Y+closeBoxY-8+closeHit)
	if p.hover.kind == hitClose {
		gfx.FillRound(c, box, 4, t.CaptionHover)
	}
	icons.Draw(c, "close", o.X+closeBoxX, p.Bounds().Min.Y+closeBoxY, t.PanelValue)
}

func (p *Panel) drawThumb(c widget.DrawContext, o image.Point) {
	r := image.Rect(o.X+thumbX, o.Y+thumbY, o.X+thumbX+thumbW, o.Y+thumbY+thumbH)
	if len(p.thumbSrc.Levels) == 0 {
		gfx.FillRound(c, r, thumbRadius, p.Theme.PanelCard)
		return
	}
	pw, ph := widget.PhysicalRect(c, r).X, widget.PhysicalRect(c, r).Y
	if p.thumbBuf == nil || p.thumbBuf.Bounds().Dx() != pw || p.thumbBuf.Bounds().Dy() != ph || p.thumbDirty {
		p.thumbBuf = image.NewRGBA(image.Rect(0, 0, pw, ph))
		render.Render(p.thumbBuf, p.thumbSrc, coverMapping(p.thumbSrc, p.thumbOrient, pw, ph), render.Background{Canvas: p.Theme.PanelCard, Kind: render.BgSolid, Fill: p.Theme.PanelCard})
		p.thumbDirty = false
	}
	if rc, ok := c.(widget.RoundClipper); ok {
		prev := c.Clip()
		rc.SetRoundClip(r.Intersect(prev), thumbRadius)
		c.DrawImageScaled(p.thumbBuf, r.Min.X, r.Min.Y, r.Dx(), r.Dy())
		rc.ClearRoundClip()
		c.SetClip(prev)
		return
	}
	c.DrawImageScaled(p.thumbBuf, r.Min.X, r.Min.Y, r.Dx(), r.Dy())
}

func coverMapping(src render.Source, o codec.Orientation, w, h int) render.Mapping {
	if !o.Valid() {
		o = codec.OrientNormal
	}
	ow, oh := o.Size(src.Width, src.Height)
	if ow <= 0 || oh <= 0 {
		return render.Mapping{Scale: 1, Orient: o}
	}
	s := max(float64(w)/float64(ow), float64(h)/float64(oh))
	return render.Mapping{
		Scale:   s,
		OriginX: (float64(ow) - float64(w)/s) / 2,
		OriginY: (float64(oh) - float64(h)/s) / 2,
		Orient:  o,
	}
}

func (p *Panel) drawCard(c widget.DrawContext, o image.Point, cp cardPos) {
	t := p.Theme
	r := cp.rect.Add(o)
	if r.Max.Y < p.Bounds().Min.Y || r.Min.Y > p.Bounds().Max.Y {
		return
	}
	gfx.FillRound(c, r, cardRadius, t.PanelCard)
	gfx.StrokeRound(c, r, cardRadius, 1, t.PanelCardBorder)
	x0 := r.Min.X + cardPad
	if cp.card.Title != "" {
		tf := titleFont
		if cp.card.Kind == CardSizes || cp.card.Kind == CardFile {
			tf = compactTitleFont
		}
		tf.Draw(c, cp.card.Title, x0, r.Min.Y+cp.title, t.PanelValue)
	}
	for _, rp := range cp.rows {
		p.drawRow(c, r, cp, rp, x0)
	}
}

func (p *Panel) drawRow(c widget.DrawContext, card image.Rectangle, cp cardPos, rp rowPos, x0 int) {
	t := p.Theme
	base := card.Min.Y + rp.baseline
	if cp.card.Kind == CardNote {
		labelFont.Draw(c, rp.row.Label, x0, base, t.PanelLabel)
		return
	}
	labelFont.Draw(c, rp.row.Label, x0, base, t.PanelLabel)
	vx := card.Min.X + rp.valueX - 2
	maxW := card.Max.X - cardPad - vx
	col := t.PanelValue
	font := valueFont
	if rp.row.Link {
		col = t.Link
		font = pathFont
		if rp.row.URL == "" {
			font = pathFont
		}
	}
	if rp.row.Copy != "" {
		cx := card.Max.X - 27
		maxW = cx - vx - 8
		ic := image.Rect(cx-4, base-15, cx+18, base+7)
		if p.hover.kind == hitCopy && p.hover.text == rp.row.Copy {
			gfx.FillRound(c, ic, 4, t.CaptionHover)
		}
		icons.Draw(c, "copy", cx+2, base-11, t.PanelValue)
	}
	font.DrawFit(c, rp.row.Value, vx, base, maxW, col)
}

func (p *Panel) drawBar(c widget.DrawContext) {
	ms := p.maxScroll()
	if ms <= 0 {
		return
	}
	b := p.Bounds()
	trackH := b.Dy() - 2*barInset
	total := p.place().height
	thumbH := max(24, trackH*b.Dy()/total)
	y := b.Min.Y + barInset + (trackH-thumbH)*p.scroll/ms
	x := b.Max.X - barInset - barWidth
	gfx.FillRound(c, image.Rect(x, y, x+barWidth, y+thumbH), float64(barWidth)/2, p.Theme.ScrollThumb)
}

func (p *Panel) Cursor(x, y int) widget.Cursor {
	if p.hitAt(x, y).kind != hitNone {
		return widget.CursorHand
	}
	return widget.CursorArrow
}

func (p *Panel) OnMouseMove(x, y int) {
	if widget.CursorIsNowhere(x, y) {
		p.setHover(hit{})
		return
	}
	p.setHover(p.hitAt(x, y))
}

func (p *Panel) setHover(h hit) {
	if h.kind == p.hover.kind && h.rect == p.hover.rect {
		return
	}
	p.hover = h
	p.invalidate()
}

func (p *Panel) WantsCapture(e widget.MouseEvent) bool { return false }

func (p *Panel) OnMouseButton(e widget.MouseEvent) bool {
	switch e.Button {
	case widget.MouseWheelUp:
		p.scrollBy(-scrollStep)
		return true
	case widget.MouseWheelDown:
		p.scrollBy(scrollStep)
		return true
	case widget.MouseLeft:
	default:
		return false
	}
	h := p.hitAt(e.X, e.Y)
	if e.Pressed {
		return h.kind != hitNone
	}
	switch h.kind {
	case hitClose:
		if p.OnClose != nil {
			p.OnClose()
		}
	case hitCopy:
		if p.OnCopy != nil {
			p.OnCopy(h.text)
		}
	case hitLink:
		if h.url != "" {
			if p.OnOpenURL != nil {
				p.OnOpenURL(h.url)
			}
		} else if p.OnReveal != nil {
			p.OnReveal()
		}
	default:
		return false
	}
	return true
}

func (p *Panel) OnMouseWheelPixelsMod(x, y int, dx, dy float64, mod widget.KeyMod) bool {
	if !image.Pt(x, y).In(p.Bounds()) || dy == 0 {
		return false
	}
	p.scrollBy(int(dy))
	return true
}

func (p *Panel) scrollBy(d int) {
	old := p.scroll
	p.scroll += d
	p.clampScroll()
	if p.scroll != old {
		p.invalidate()
	}
}

func (p *Panel) ScrollY() int { return p.scroll }

func (p *Panel) ContentHeight() int { return p.place().height }
