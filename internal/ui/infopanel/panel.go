package infopanel

import (
	"image"
	"image/color"
	"strings"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/fonts"
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
	cacheK float64

	active bool
	cur    int
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
	k := fonts.TextScale()
	if p.cacheW == w && p.cacheK == k {
		return p.cache
	}
	p.cache, p.cacheW, p.cacheK = layoutModel(p.model, w, k), w, k
	return p.cache
}

func (p *Panel) step() int { return max(scaled(scrollStep, fonts.TextScale()), 1) }

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
	cl := image.Rect(o.X+closeBoxX-8, p.Bounds().Min.Y+closeBoxY-8, o.X+closeBoxX-8+closeHit, p.Bounds().Min.Y+closeBoxY-8+closeHit)
	hs := []hit{{kind: hitClose, rect: cl}}
	for _, cp := range p.place().cards {
		at := o.Add(cp.rect.Min)
		for _, rp := range cp.rows {
			if rp.row.Copy != "" {
				hs = append(hs, hit{kind: hitCopy, rect: rp.copyRect.Add(at), text: rp.row.Copy})
			}
			if rp.row.Link {
				hs = append(hs, hit{kind: hitLink, rect: rp.linkRect.Add(at), url: rp.row.URL})
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

	pl := p.place()
	headerFont.DrawFit(c, i18n.T("info.title"), o.X+headerX, o.Y+pl.headerBase, closeBoxX-8-headerX, t.PanelValue)
	p.drawClose(c, o)
	p.drawThumb(c, o)

	tx := o.X + textX
	drawLines(c, nameFont, pl.nameLines, tx, o.Y+pl.nameBase, pl.nameStep, pl.textW, t.PanelValue)
	subFont.DrawFit(c, fitFormat(subFont, p.model.Format, p.model.FormatNote, pl.textW), tx, o.Y+pl.formatBase, pl.textW, t.PanelLabel)
	subFont.DrawFit(c, p.model.FileSize, tx, o.Y+pl.sizeBase, pl.textW, t.PanelLabel)

	for _, cp := range pl.cards {
		p.drawCard(c, o, cp)
	}
	p.drawFocus(c)
	p.drawBar(c)
}

func fitFormat(f gfx.Font, format, note string, width int) string {
	if note == "" || !strings.HasSuffix(format, note) || f.Measure(nil, format) <= width {
		return format
	}
	head := strings.TrimSpace(strings.TrimSuffix(format, note))
	room := width - f.Measure(nil, " "+note)
	if head == "" || room < f.Measure(nil, "…") {
		return format
	}
	return f.Fit(nil, head, room) + " " + note
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

func drawLines(c widget.DrawContext, f gfx.Font, lines []string, x, base, step, w int, col color.RGBA) {
	for i, s := range lines {
		f.DrawFit(c, s, x, base+i*step, w+textSlack, col)
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
		tf.DrawFit(c, cp.card.Title, x0, r.Min.Y+cp.title, cp.innerW, t.PanelValue)
	}
	for _, rp := range cp.rows {
		p.drawRow(c, r, cp, rp, x0)
	}
}

func (p *Panel) drawRow(c widget.DrawContext, card image.Rectangle, cp cardPos, rp rowPos, x0 int) {
	t := p.Theme
	base := card.Min.Y + rp.baseline
	if cp.card.Kind == CardNote {
		drawLines(c, labelFont, rp.labelLines, x0, base, rp.labelStep, cp.innerW, t.PanelLabel)
		return
	}
	drawLines(c, labelFont, rp.labelLines, x0, base, rp.labelStep, rp.valueX-cardPad-valuePad, t.PanelLabel)
	vx := card.Min.X + rp.valueX - 2
	col := t.PanelValue
	if rp.row.Link {
		col = t.Link
	}
	if rp.row.Copy != "" {
		ic := rp.copyRect.Add(card.Min)
		if p.hover.kind == hitCopy && p.hover.text == rp.row.Copy {
			gfx.FillRound(c, ic, 4, t.CaptionHover)
		}
		icons.Draw(c, "copy", ic.Min.X+copyBoxLeft+copyIconDX, base-copyIconDY-rp.iconLift, t.PanelValue)
	}
	drawLines(c, rp.valueFont, rp.valueLines, vx, base, rp.valueStep, rp.valueW, col)
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
		p.scrollBy(-p.step())
		return true
	case widget.MouseWheelDown:
		p.scrollBy(p.step())
		return true
	case widget.MouseLeft:
	default:
		return false
	}
	h := p.hitAt(e.X, e.Y)
	if e.Pressed {
		return h.kind != hitNone
	}
	return p.activate(h)
}

func (p *Panel) activate(h hit) bool {
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
