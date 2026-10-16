package settings

import (
	"image"
	"image/color"
	"math"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/ui/gfx"
	"github.com/oops1/PixGo/internal/ui/icons"
	"github.com/oops1/PixGo/internal/ui/theme"
)

var (
	shadowRing = color.RGBA{0, 0, 0, 26}
	haloRing   = color.RGBA{0, 0, 0, 56}
)

func (p *Page) Draw(c widget.DrawContext) {
	b := p.Bounds()
	if b.Empty() || p.deps.Source == nil {
		return
	}
	p.ensureLayout()
	t := p.theme()
	restore := gfx.Clip(c, b)
	defer restore()
	p.drawBackground(c, t)
	p.drawItems(c, t)
	p.drawNav(c, t)
	p.drawHeader(c, t)
	p.drawBar(c, t)
}

func (p *Page) fill(c widget.DrawContext, r image.Rectangle, col color.RGBA) {
	r = r.Intersect(p.Bounds())
	if !r.Empty() {
		c.FillRect(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), col)
	}
}

func (p *Page) drawBackground(c widget.DrawContext, t *theme.Theme) {
	g := p.geo
	b := p.Bounds()
	p.fill(c, g.header, t.Nav)
	c.DrawHLine(b.Min.X, headerHeight, b.Dx(), t.Separator)
	p.fill(c, g.nav, t.Nav)
	if g.nav.Dy() > 0 {
		c.DrawVLine(dim.navW, contentTop, g.nav.Dy(), t.GalleryRule)
	}
	p.fill(c, g.view, t.Page)
}

func (p *Page) secondary(t *theme.Theme) color.RGBA {
	return gfx.Blend(t.SecondaryText, t.Page, .25)
}

func (p *Page) outline(c widget.DrawContext, t *theme.Theme, r frect, radius float64) {
	if !t.Dark || t.Name == "contrast" {
		strokeRound(c, r, radius, 1, t.FieldBorder)
	}
}

func (p *Page) ring(c widget.DrawContext, t *theme.Theme, r frect, radius float64) {
	gfx.RingOf(t).RectF(c, r.gfx(), radius)
}

func (p *Page) discRing(c widget.DrawContext, t *theme.Theme, cx, cy, rad float64) {
	if t.Name == "contrast" {
		strokeDisc(c, cx, cy, rad+1.5, 1, t.Canvas)
	}
	strokeDisc(c, cx, cy, rad+3.5, 2, t.Focus)
}

func (p *Page) drawHeader(c widget.DrawContext, t *theme.Theme) {
	g := p.geo
	back := g.back
	switch {
	case p.pressed.kind == hitBack && p.hover.kind == hitBack:
		gfx.FillRound(c, back, backPillRound, t.CaptionPressed)
	case p.hover.kind == hitBack && p.pressed.kind == hitNone:
		gfx.FillRound(c, back, backPillRound, t.CaptionHover)
	}
	icons.Draw(c, "arrow-left", backIconX, backIconY, t.TitleIcon)
	drawText(c, headerFont, i18n.T("settings.title"), headerTextX, dim.headerBase, t.TitleText)
	if p.keyNav && p.focus == focusBack {
		p.ring(c, t, fr(back), backPillRound)
	}
}

func navCenter(s Section) float64 { return dim.navCenters[s] }

func navRowRect(s Section) image.Rectangle {
	cy := navCenter(s)
	return image.Rect(navHitLeft, int(cy-dim.navHitUp+.5), dim.navHitRight, int(cy+dim.navHitDown+.5))
}

func (p *Page) drawNav(c widget.DrawContext, t *theme.Theme) {
	restore := gfx.Clip(c, p.geo.nav)
	defer restore()
	for s := Section(0); s < sectionCount; s++ {
		cy := navCenter(s)
		pill := frect{navPillLeft, cy - dim.navPillUp, dim.navPillRight, cy + dim.navPillDown}
		selected := s == p.sec
		col := t.Text
		hovered := p.hover.kind == hitNav && p.hover.index == int(s)
		switch {
		case selected:
			fillRound(c, pill, dim.navPillRound, t.NavSelected)
			indicator := t.NavIndicator
			if theme.Contrast(indicator, t.NavSelected) < 1.5 {
				indicator = t.AccentText
			}
			fillRound(c, frect{navBarLeft, cy - dim.navBarUp, navBarRight, cy + dim.navBarDown}, navBarRadius, indicator)
			if theme.Contrast(t.Text, t.NavSelected) < 3 {
				col = t.AccentText
			}
		case hovered:
			fillRound(c, pill, dim.navPillRound, gfx.Alpha(t.Text, .05))
		}
		icons.Draw(c, sectionIcons[s], navIconX, dim.navIconTops[s], col)
		base := dim.navCapTops[s] + dim.navTextBase
		drawFit(c, navFont, i18n.T(sectionKeys[s]), navTextX, base, dim.navTextWidth, col)
		if selected && p.keyNav && p.focus == focusNav {
			p.ring(c, t, pill, dim.navPillRound)
		}
	}
}

func (p *Page) drawItems(c widget.DrawContext, t *theme.Theme) {
	view := p.geo.view
	restore := gfx.Clip(c, view)
	defer restore()
	off := image.Pt(0, -p.scroll)
	for i := range p.items {
		it := &p.items[i]
		r := it.r.Add(off)
		if !r.Inset(-6).Overlaps(view) {
			continue
		}
		st := state{
			hover:   p.hover.kind == hitItem && p.hover.index == i,
			pressed: p.pressed.kind == hitItem && p.pressed.index == i,
			focus:   p.keyNav && it.id != "" && p.focus == it.id,
		}
		switch it.kind {
		case kindTitle:
			for k, l := range it.lines {
				titleFont.Draw(c, l, r.Min.X+1, r.Min.Y+dim.titleBase+k*dim.titleStep, t.Text)
			}
		case kindHeading:
			p.drawHeading(c, t, it, r)
		case kindNote:
			for k, l := range it.lines {
				drawText(c, noteFont, l, r.Min.X+it.indent, r.Min.Y+dim.noteBase+k*dim.lineHeight, p.secondary(t))
			}
		case kindValue:
			for k, l := range it.lines {
				drawText(c, bodyFont, l, r.Min.X, r.Min.Y+dim.valueBase+k*dim.lineHeight, t.Text)
			}
		case kindCheck:
			p.drawCheck(c, t, it, r, st)
		case kindRadio:
			p.drawRadio(c, t, it, r, st)
		case kindCard:
			p.drawCard(c, t, it, r, st)
		case kindDropdown:
			p.drawDropdown(c, t, it, r, st)
		case kindSlider:
			p.drawSlider(c, t, it, r, st)
		case kindButton:
			p.drawButton(c, t, it, r, st)
		case kindLink:
			p.drawLink(c, t, it, r, st)
		case kindFormat:
			p.drawFormat(c, t, it, r, st)
		}
	}
}

type state struct {
	hover, pressed, focus bool
}

func (p *Page) drawHeading(c widget.DrawContext, t *theme.Theme, it *item, r image.Rectangle) {
	if it.name == "big" {
		aboutFont.Draw(c, it.lines[0], r.Min.X, r.Min.Y+dim.headingBase+int(math.Round(6*dim.k)), t.Text)
		return
	}
	for k, l := range it.lines {
		headingFont.Draw(c, l, r.Min.X+1, r.Min.Y+dim.headingBase+k*dim.lineHeight, t.Text)
	}
}

func (p *Page) labelColor(t *theme.Theme, it *item) color.RGBA {
	if it.disabled {
		return t.Disabled
	}
	return t.Text
}

func (p *Page) drawLabel(c widget.DrawContext, t *theme.Theme, it *item, r image.Rectangle) {
	col := p.labelColor(t, it)
	for k, l := range it.lines {
		drawText(c, bodyFont, l, r.Min.X+dim.checkLabelX, r.Min.Y+dim.checkBase+k*dim.lineHeight, col)
	}
}

func (p *Page) drawBox(c widget.DrawContext, t *theme.Theme, on bool, left int, top float64, st state, disabled bool) frect {
	bx, by := float64(left)+checkBoxLeft, top+dim.checkBoxTop
	box := frect{bx, by, bx + dim.checkBoxSize, by + dim.checkBoxSize}
	if on {
		fill := t.Accent
		switch {
		case disabled:
			fill = gfx.Blend(t.Page, t.Disabled, .6)
		case st.pressed && st.hover:
			fill = gfx.Blend(t.Accent, t.Page, .3)
		case st.hover:
			fill = gfx.Blend(t.Accent, t.AccentText, .1)
		}
		strokeRound(c, box.grow(1), dim.checkBoxRadius+1, 1, shadowRing)
		fillRound(c, box, dim.checkBoxRadius, fill)
		off := (dim.checkBoxSize - 12) / 2
		icons.Draw(c, "checkmark", int(bx+off+.5), int(by+off+.5), t.AccentText)
		return box
	}
	ring := t.ToggleOff
	inner := gfx.Alpha(t.Text, .02)
	if disabled {
		ring = t.Disabled
	} else if st.hover {
		ring = gfx.Blend(t.ToggleOff, t.Text, .25)
		inner = gfx.Alpha(t.Text, .06)
		if st.pressed {
			inner = gfx.Alpha(t.Text, .1)
		}
	}
	fillRound(c, box, dim.checkBoxRadius, inner)
	strokeRound(c, box, dim.checkBoxRadius, dim.checkRingW, ring)
	return box
}

func (p *Page) drawCheck(c widget.DrawContext, t *theme.Theme, it *item, r image.Rectangle, st state) {
	box := p.drawBox(c, t, it.on, r.Min.X, rowTop(it, r), st, it.disabled)
	p.drawLabel(c, t, it, r)
	if st.focus {
		p.ring(c, t, box, dim.checkBoxRadius)
	}
}

func (p *Page) drawRadio(c widget.DrawContext, t *theme.Theme, it *item, r image.Rectangle, st state) {
	cx := float64(r.Min.X) + checkBoxLeft + dim.checkBoxSize/2
	cy := rowTop(it, r) + dim.checkBoxTop + dim.checkBoxSize/2
	rad := dim.checkBoxSize / 2
	if it.on {
		fill := t.Accent
		switch {
		case st.pressed && st.hover:
			fill = gfx.Blend(t.Accent, t.Page, .3)
		case st.hover:
			fill = gfx.Blend(t.Accent, t.AccentText, .1)
		}
		strokeDisc(c, cx, cy, rad+1, 1, shadowRing)
		fillDisc(c, cx, cy, rad, fill)
		fillDisc(c, cx, cy, dim.radioDot, t.AccentText)
	} else {
		ring := t.ToggleOff
		inner := gfx.Alpha(t.Text, .02)
		if st.hover {
			ring = gfx.Blend(t.ToggleOff, t.Text, .25)
			inner = gfx.Alpha(t.Text, .06)
			if st.pressed {
				inner = gfx.Alpha(t.Text, .12)
			}
		}
		fillDisc(c, cx, cy, rad, inner)
		strokeDisc(c, cx, cy, rad, dim.checkRingW, ring)
	}
	p.drawLabel(c, t, it, r)
	if st.focus {
		p.discRing(c, t, cx, cy, rad)
	}
}

func (p *Page) drawDropdown(c widget.DrawContext, t *theme.Theme, it *item, r image.Rectangle, st state) {
	f := frect{float64(r.Min.X) + .5, float64(r.Min.Y) - .4, float64(r.Max.X) - .5, float64(r.Max.Y)}
	fill := t.Field
	switch {
	case st.pressed && st.hover:
		fill = gfx.Blend(t.Field, t.Page, .35)
	case st.hover:
		fill = gfx.Blend(t.Field, t.Text, .05)
	}
	fillRound(c, f, dim.dropdownRadius, fill)
	p.outline(c, t, f, dim.dropdownRadius)
	c.DrawHLine(r.Min.X+5, r.Min.Y, r.Dx()-10, gfx.Blend(fill, t.Text, .035))
	drawText(c, bodyFont, it.text, r.Min.X+dim.dropdownTextX, r.Min.Y+dim.dropdownBase, t.Text)
	icons.Draw(c, "chevron-down-sm", r.Max.X-chevronFromR, r.Min.Y+dim.chevronTop, t.SecondaryText)
	if st.focus {
		p.ring(c, t, f, dim.dropdownRadius)
	}
}

func sliderSpan(it *item) (x0, x1, cy int) { return sliderSpanIn(it.r, it.labelW, it.stacked) }

func sliderSpanIn(r image.Rectangle, labelW int, stacked bool) (x0, x1, cy int) {
	x0 = r.Min.X + dim.sliderInset
	if stacked {
		x1 = min(r.Max.X-dim.sliderInset, x0+dim.sliderMaxLen)
	} else {
		x1 = min(r.Max.X-labelW-dim.sliderInset-dim.sliderLabelGap, x0+dim.sliderMaxLen)
	}
	x1 = max(x1, x0+dim.sliderMinLen)
	return x0, x1, r.Min.Y + dim.rowHeight/2
}

func sliderThumbX(it *item, x0, x1 int) float64 {
	span := it.hi - it.lo
	if span <= 0 {
		return float64(x0)
	}
	v := min(max(it.value, it.lo), it.hi)
	return float64(x0) + float64(v-it.lo)/float64(span)*float64(x1-x0)
}

func (p *Page) drawSlider(c widget.DrawContext, t *theme.Theme, it *item, r image.Rectangle, st state) {
	x0, x1, cy := sliderSpanIn(r, it.labelW, it.stacked)
	tx := sliderThumbX(it, x0, x1)
	fy := float64(cy)
	half := float64(dim.sliderTrack) / 2
	fillRound(c, frect{float64(x0), fy - half, float64(x1), fy + half}, half, gfx.Blend(t.Page, t.Text, .3))
	if tx > float64(x0) {
		fillRound(c, frect{float64(x0), fy - half, tx, fy + half}, half, t.Accent)
	}
	outer := gfx.Blend(t.Field, t.Text, .2)
	inner := dim.sliderCore
	switch {
	case p.drag != nil && p.drag.id == it.id:
		inner = dim.sliderCoreHeld
	case st.hover:
		inner = dim.sliderCoreHover
	}
	strokeDisc(c, tx, fy, dim.sliderThumb/2+1, 1, shadowRing)
	fillDisc(c, tx, fy, dim.sliderThumb/2, outer)
	fillDisc(c, tx, fy, inner, t.Accent)
	if it.stacked {
		drawText(c, bodyFont, it.valueText, x0-dim.sliderInset, r.Min.Y+dim.rowHeight+dim.noteBase, p.secondary(t))
	} else {
		drawText(c, bodyFont, it.valueText, x1+dim.sliderInset+dim.sliderLabelGap, r.Min.Y+dim.checkBase, p.secondary(t))
	}
	if st.focus {
		p.discRing(c, t, tx, fy, dim.sliderThumb/2)
	}
}

func (p *Page) drawButton(c widget.DrawContext, t *theme.Theme, it *item, r image.Rectangle, st state) {
	f := frect{float64(r.Min.X) + .5, float64(r.Min.Y) - .4, float64(r.Max.X) - .5, float64(r.Max.Y)}
	fill := t.Field
	switch {
	case st.pressed && st.hover:
		fill = gfx.Blend(t.Field, t.Page, .35)
	case st.hover:
		fill = gfx.Blend(t.Field, t.Text, .05)
	}
	fillRound(c, f, dim.buttonRadius, fill)
	p.outline(c, t, f, dim.buttonRadius)
	c.DrawHLine(r.Min.X+5, r.Min.Y, r.Dx()-10, gfx.Blend(fill, t.Text, .035))
	for k, l := range it.lines {
		w := bodyFont.Measure(c, l)
		drawText(c, bodyFont, l, r.Min.X+(r.Dx()-w)/2, r.Min.Y+dim.dropdownBase+k*dim.lineHeight, t.Text)
	}
	if st.focus {
		p.ring(c, t, f, dim.buttonRadius)
	}
}

func (p *Page) drawLink(c widget.DrawContext, t *theme.Theme, it *item, r image.Rectangle, st state) {
	col := t.Link
	if st.pressed && st.hover {
		col = gfx.Blend(t.Link, t.Page, .35)
	}
	base := r.Min.Y + dim.linkBase
	drawText(c, bodyFont, it.text, r.Min.X, base, col)
	w := bodyFont.Measure(c, it.text)
	if st.hover || st.focus {
		c.DrawHLine(r.Min.X, base+2, w, col)
	}
	if it.detail != "" {
		drawText(c, bodyFont, it.detail, r.Min.X+w, base, p.secondary(t))
	}
	if st.focus {
		p.ring(c, t, fr(image.Rect(r.Min.X, r.Min.Y, r.Min.X+w, r.Max.Y)), 3)
	}
}

func (p *Page) drawFormat(c widget.DrawContext, t *theme.Theme, it *item, r image.Rectangle, st state) {
	col := t.Text
	sub := p.secondary(t)
	if it.disabled {
		col, sub = t.Disabled, t.Disabled
	}
	drawText(c, bodyFont, it.name, r.Min.X, r.Min.Y+dim.checkBase, col)
	if it.change != nil {
		box := it.box.Add(image.Pt(0, r.Min.Y-it.r.Min.Y))
		bx := p.drawBox(c, t, it.on, box.Min.X, float64(box.Min.Y), st, false)
		drawText(c, bodyFont, it.text, box.Min.X+dim.checkLabelX, box.Min.Y+dim.checkBase, t.Text)
		if st.focus {
			p.ring(c, t, bx, dim.checkBoxRadius)
		}
	}
	drawFit(c, noteFont, it.detail, r.Min.X, r.Min.Y+dim.formatSubBase, r.Dx(), sub)
}

func rowTop(it *item, r image.Rectangle) float64 {
	return float64(r.Min.Y) + it.fy - float64(it.r.Min.Y)
}
