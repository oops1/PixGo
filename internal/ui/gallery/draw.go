package gallery

import (
	"image"
	"image/color"

	"github.com/oops1/headless-gui/v3/widget"

	model "github.com/oops1/PixGo/internal/gallery"
	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/ui/gfx"
	"github.com/oops1/PixGo/internal/ui/icons"
	"github.com/oops1/PixGo/internal/ui/theme"
)

var (
	shadowRing = color.RGBA{0, 0, 0, 72}
	errorRed   = color.RGBA{232, 17, 35, 255}
)

func (p *Page) Draw(c widget.DrawContext) {
	b := p.Bounds()
	if b.Empty() || p.src() == nil {
		return
	}
	p.ensureLayout()
	t := p.theme()
	restore := gfx.Clip(c, b)
	defer restore()
	p.drawGrid(c, t)
	p.drawNav(c, t)
	p.drawHeader(c, t)
	p.drawStatus(c, t)
	p.drawBars(c, t)
	p.drawBubble(c, t)
}

func (p *Page) fill(c widget.DrawContext, r image.Rectangle, col color.RGBA) {
	r = r.Intersect(p.Bounds())
	if !r.Empty() {
		c.FillRect(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), col)
	}
}

func (p *Page) mid(t *theme.Theme) color.RGBA { return gfx.Blend(t.Disabled, t.TitleIcon, .75) }

func (p *Page) soft(t *theme.Theme) color.RGBA { return gfx.Blend(t.Disabled, t.TitleIcon, .45) }

func (p *Page) glyph(t *theme.Theme) color.RGBA { return gfx.Blend(t.TitleIcon, t.Disabled, .06) }

func (p *Page) dim(t *theme.Theme) color.RGBA { return gfx.Blend(t.Disabled, t.TitleIcon, .13) }

func (p *Page) drawHeader(c widget.DrawContext, t *theme.Theme) {
	g := p.geo
	b := p.Bounds()
	p.fill(c, g.header, t.GalleryHeader)
	c.DrawHLine(b.Min.X, headerHeight, b.Dx(), t.Separator)
	p.drawNavButtons(c, t)
	p.drawAddress(c, t)
	p.drawViews(c, t)
}

func (p *Page) buttonFill(c widget.DrawContext, t *theme.Theme, r image.Rectangle, kind hitKind, enabled bool) {
	if !enabled {
		return
	}
	switch {
	case p.pressed.kind == kind && p.hover.kind == kind:
		gfx.FillRound(c, r, navPillRadius, t.CaptionPressed)
	case p.hover.kind == kind && p.pressed.kind == hitNone:
		gfx.FillRound(c, r, navPillRadius, t.CaptionHover)
	}
}

func (p *Page) drawNavButtons(c widget.DrawContext, t *theme.Theme) {
	g := p.geo
	s := p.snap
	p.buttonFill(c, t, g.back, hitBack, s.canBack)
	p.buttonFill(c, t, g.forward, hitForward, s.canForward)
	p.buttonFill(c, t, g.up, hitUp, s.canUp)
	col := func(on bool, enabled color.RGBA) color.RGBA {
		if on {
			return enabled
		}
		return p.dim(t)
	}
	icons.Draw(c, "arrow-left", backIconX, iconTop, col(s.canBack, t.TitleIcon))
	icons.Draw(c, "arrow-right", forwardIconX, iconTop, col(s.canForward, t.TitleIcon))
	icons.Draw(c, "arrow-up", upIconX, iconTop, col(s.canUp, p.mid(t)))
}

func (p *Page) addressIcon() string {
	switch p.snap.loc.Kind {
	case model.KindAll:
		return "images"
	case model.KindRecent:
		return "recent"
	case model.KindFavorites:
		return "heart-small"
	}
	return "folder"
}

func (p *Page) drawAddress(c widget.DrawContext, t *theme.Theme) {
	g := p.geo
	a := g.address
	field := t.GalleryField
	if p.hover.kind == hitAddress && !p.editing {
		field = gfx.Blend(field, t.Text, .04)
	}
	gfx.FillRound(c, a, controlRadius, field)
	if p.editing {
		p.input.Draw(c)
	}
	icons.Draw(c, p.addressIcon(), a.Min.X+addressIconX, a.Min.Y+addressIconY, t.Text)
	if !p.editing {
		textX := a.Min.X + addressTextX
		limit := g.filter.Min.X - 4
		addressFont.DrawFit(c, p.snap.loc.Display(), textX, a.Min.Y+18, limit-textX, t.Text)
	}
	if p.hover.kind == hitFilter || p.hover.kind == hitRefresh {
		r := g.filter
		if p.hover.kind == hitRefresh {
			r = g.refresh
		}
		a := .06
		if p.pressed.kind == p.hover.kind {
			a = .11
		}
		gfx.FillRound(c, r.Inset(3), navPillRadius, gfx.Alpha(t.Text, a))
	}
	fcol := gfx.Blend(t.TitleIcon, t.Disabled, .08)
	if p.snap.format != "" {
		fcol = t.SelectionBorder
	}
	icons.Draw(c, "filter", a.Max.X-filterIconFromR, iconTop, fcol)
	icons.Draw(c, "sort", a.Max.X-refreshIconFromR, refreshIconTop, p.soft(t))
}

func (p *Page) viewPill(half image.Rectangle) image.Rectangle {
	g := p.geo
	cx := half.Min.X + 15
	if half == g.viewGrid {
		cx = half.Min.X + 14
	}
	return image.Rect(cx-13, g.views.Min.Y+2, cx+13, g.views.Max.Y-2)
}

func (p *Page) drawViews(c widget.DrawContext, t *theme.Theme) {
	g := p.geo
	gfx.FillRound(c, g.views, controlRadius, t.GalleryViews)
	active := g.viewGrid
	if p.snap.mode == ViewList {
		active = g.viewList
	}
	gfx.FillRound(c, p.viewPill(active), navPillRadius, gfx.Alpha(t.Text, .045))
	if k := p.hover.kind; (k == hitViewList || k == hitViewGrid) && p.pressed.kind == hitNone {
		hov := g.viewList
		if k == hitViewGrid {
			hov = g.viewGrid
		}
		if hov != active {
			gfx.FillRound(c, p.viewPill(hov), navPillRadius, gfx.Alpha(t.Text, .05))
		}
	}
	icons.Draw(c, "view-details", g.views.Min.X+viewListIconX, iconTop, p.glyph(t))
	icons.Draw(c, "view-tiles", g.views.Min.X+viewGridIconX, iconTop, p.glyph(t))
	p.buttonFill(c, t, g.thumbs, hitThumbs, true)
	icons.Draw(c, "view-grid", g.thumbs.Min.X+5, iconTop, p.glyph(t))
}

func (p *Page) drawNav(c widget.DrawContext, t *theme.Theme) {
	g := p.geo
	p.fill(c, g.sidebar, t.GalleryNav)
	c.DrawVLine(sidebarWidth, g.sidebar.Min.Y, g.sidebar.Dy(), t.Status)
	restore := gfx.Clip(c, g.sidebar)
	defer restore()
	top := g.sidebar.Min.Y - p.sideY
	right := g.sidebar.Max.X
	for _, r := range p.rows {
		y := top + int(r.center)
		if y < g.sidebar.Min.Y-24 || y > g.sidebar.Max.Y+24 {
			continue
		}
		p.drawNavRow(c, t, r, top, right)
	}
}

func (p *Page) drawNavRow(c widget.DrawContext, t *theme.Theme, r navRow, top, right int) {
	e := r.entry
	base := r.baseline(top)
	x := r.textX()
	if e.Header {
		sectionFont.DrawFit(c, e.Label, x, base, right-x-6, t.Text)
		return
	}
	pill := r.rect(top, right)
	switch {
	case e.Selected:
		gfx.FillRound(c, pill, 5, t.NavSelected)
	case p.hover.kind == hitNav && p.hover.index == r.index && !e.Disabled:
		gfx.FillRound(c, pill, 5, gfx.Alpha(t.Text, .05))
	}
	if e.Expander {
		icons.Draw(c, "expander", navExpanderX, top+int(r.center+.5)-4, p.mid(t))
	}
	col := t.Text
	switch {
	case e.Disabled:
		col = gfx.Blend(t.Text, t.SecondaryText, .22)
	case e.Selected && theme.Contrast(t.Text, t.NavSelected) < 3:
		col = t.AccentText
	}
	pos := r.iconPos(top)
	icons.Draw(c, e.Icon, pos.X, pos.Y, col)
	navFont.DrawFit(c, e.Label, x, base, right-x-6, col)
}

func (p *Page) drawGrid(c widget.DrawContext, t *theme.Theme) {
	vp := p.geo.grid
	p.fill(c, vp, t.GalleryPage)
	restore := gfx.Clip(c, vp)
	defer restore()
	if len(p.snap.items) == 0 {
		p.drawEmpty(c, t)
		return
	}
	first, last := p.grid.Visible(p.scroll)
	sel := p.Selected()
	for i := first; i < last; i++ {
		if p.snap.mode == ViewList {
			p.drawRow(c, t, i, sel)
		} else {
			p.drawTile(c, t, i, sel)
		}
	}
	p.prefetch(first, last)
}

func (p *Page) drawEmpty(c widget.DrawContext, t *theme.Theme) {
	s := p.snap
	text := model.EmptyText(s.loc, s.failed)
	if s.loading {
		text = i18n.T("gallery.loading")
	}
	vp := p.geo.grid
	w := emptyFont.Measure(c, text)
	x := max(vp.Min.X+(vp.Dx()-w)/2, vp.Min.X+8)
	emptyFont.DrawFit(c, text, x, vp.Min.Y+vp.Dy()/2, vp.Dx()-16, t.SecondaryText)
}

func (p *Page) fitted(c widget.DrawContext, f gfx.Font, name string, w int) (string, int) {
	k := labelKey{name, w}
	if v, ok := p.labels[k]; ok {
		return v.text, v.w
	}
	s := f.Fit(c, name, w)
	v := labelVal{s, f.Measure(c, s)}
	if len(p.labels) > 8192 {
		clear(p.labels)
	}
	p.labels[k] = v
	return v.text, v.w
}

func (p *Page) drawTile(c widget.DrawContext, t *theme.Theme, i, sel int) {
	cell := p.tileRect(i)
	if !cell.Overlaps(p.geo.grid) {
		return
	}
	item := p.snap.items[i]
	photo := p.photoRect(i)
	selected := i == sel
	hovered := p.hover.kind == hitTile && p.hover.index == i
	if selected {
		gfx.StrokeRound(c, cell.Inset(-1), cellRadius+1, 1, shadowRing)
		gfx.FillRound(c, cell, cellRadius, t.Selection)
		gfx.StrokeRound(c, cell, cellRadius, cellFrame, t.SelectionBorder)
	}
	gfx.StrokeRound(c, photo.Inset(-1), photoRadius+1, 1, shadowRing)
	p.drawPhoto(c, t, item, photo, p.tile.photoW, p.tile.photoH, photoRadius, p.snap.side)
	if hovered && !selected {
		gfx.StrokeRound(c, photo, photoRadius, 1, t.Focus)
	}
	text, w := p.fitted(c, labelFont, item.Name, cell.Dx()-8)
	x := photo.Min.X + (photo.Dx()-w)/2
	labelFont.Draw(c, text, x, photo.Max.Y+labelBelow-photoInsetY, t.Text)
}

func (p *Page) drawPhoto(c widget.DrawContext, t *theme.Theme, item model.Item, r image.Rectangle, w, h, radius, side int) {
	if item.Dir {
		gfx.FillRound(c, r, float64(radius), t.GalleryTile)
		drawFolder(c, r)
		return
	}
	if img := p.tileImage(item.Path, side, w, h, radius); img != nil {
		c.DrawImageScaled(img, r.Min.X, r.Min.Y, w, h)
		return
	}
	gfx.FillRound(c, r, float64(radius), t.GalleryTile)
}

func (p *Page) tileImage(path string, side, w, h, radius int) *image.RGBA {
	src, ok := p.src().Thumb(path, side, true)
	if !ok || src == nil || src.Bounds().Empty() {
		return nil
	}
	return p.tiles.get(src, w, h, radius)
}

func drawFolder(c widget.DrawContext, r image.Rectangle) {
	w, h := r.Dx(), r.Dy()
	bw, bh := w*56/100, h*50/100
	x0 := r.Min.X + (w-bw)/2
	y0 := r.Min.Y + (h-bh)/2 + h/20
	tab := image.Rect(x0, y0-bh/8, x0+bw*45/100, y0+bh/4)
	body := image.Rect(x0, y0, x0+bw, y0+bh)
	gfx.FillRound(c, tab, 3, color.RGBA{0xf0, 0xae, 0x2e, 255})
	gfx.FillRound(c, body, 4, color.RGBA{0xf6, 0xc3, 0x44, 255})
}

func (p *Page) prefetch(first, last int) {
	if p.snap.mode == ViewList || last <= first {
		return
	}
	cols := p.grid.Columns()
	lo, hi := max(first-cols, 0), min(last+cols, len(p.snap.items))
	if p.prefetched == [2]int{lo, hi} {
		return
	}
	p.prefetched = [2]int{lo, hi}
	for i := lo; i < hi; i++ {
		if i >= first && i < last {
			continue
		}
		if it := p.snap.items[i]; !it.Dir {
			p.src().Thumb(it.Path, p.snap.side, false)
		}
	}
}

func (p *Page) drawRow(c widget.DrawContext, t *theme.Theme, i, sel int) {
	row := p.tileRect(i)
	if !row.Overlaps(p.geo.grid) {
		return
	}
	item := p.snap.items[i]
	switch {
	case i == sel:
		gfx.FillRound(c, row, photoRadius, t.Selection)
		gfx.StrokeRound(c, row, photoRadius, 1, t.SelectionBorder)
	case p.hover.kind == hitTile && p.hover.index == i:
		gfx.FillRound(c, row, photoRadius, gfx.Alpha(t.Text, .05))
	}
	ty := row.Min.Y + (row.Dy()-listThumb)/2
	th := image.Rect(row.Min.X+listPad, ty, row.Min.X+listPad+listThumb, ty+listThumb)
	if item.Dir {
		icons.Draw(c, "folder", th.Min.X+8, th.Min.Y+8, t.Text)
	} else {
		p.drawPhoto(c, t, item, th, listThumb, listThumb, 4, listSideTile)
	}
	nameX := th.Max.X + 12
	base := row.Min.Y + (row.Dy()+listFont.Ascent())/2 - 2
	sizeRight := row.Max.X - 14
	nameRight := sizeRight - 240
	wide := row.Dx() >= 420
	if !wide {
		nameRight = row.Max.X - 12
	}
	listFont.DrawFit(c, item.Name, nameX, base, nameRight-nameX, t.Text)
	if !wide {
		return
	}
	if !item.ModTime.IsZero() {
		listSubFont.DrawFit(c, i18n.FormatDateTime(item.ModTime), nameRight+12, base, 138, t.SecondaryText)
	}
	if !item.Dir {
		s := i18n.FormatBytes(item.Size)
		listSubFont.Draw(c, s, sizeRight-listSubFont.Measure(c, s), base, t.SecondaryText)
	}
}

func (p *Page) drawStatus(c widget.DrawContext, t *theme.Theme) {
	g := p.geo
	b := p.Bounds()
	c.DrawHLine(b.Min.X, g.statusY, b.Dx(), t.GalleryRule)
	p.fill(c, image.Rect(b.Min.X, g.statusY+1, b.Max.X, b.Max.Y), t.Status)
	restore := gfx.Clip(c, image.Rect(b.Min.X, g.statusY, g.open.Min.X-6, b.Max.Y))
	base := g.win.Max.Y - statusBaseFromB
	x := statusTextX
	status := gfx.Blend(t.SecondaryText, t.Text, .27)
	for _, part := range p.snap.status {
		statusFont.Draw(c, part, x, base, status)
		x += statusFont.Measure(c, part) + statusGap
	}
	if s := p.snap.truncated; s != "" {
		statusFont.DrawFit(c, s, x, base, g.open.Min.X-12-x, status)
	}
	restore()
	p.drawButtons(c, t)
}

func (p *Page) drawButtons(c widget.DrawContext, t *theme.Theme) {
	g := p.geo
	_, _, ok := p.selectedItem()
	left, right, txt, chev := t.GalleryOpen, t.GalleryMore, t.AccentText, t.AccentText
	if !ok {
		off := gfx.Blend(t.Status, t.Text, .1)
		left, right, txt, chev = off, off, t.Disabled, t.Disabled
	} else {
		if p.hover.kind == hitOpen {
			left = gfx.Blend(left, t.AccentText, .08)
		}
		if p.hover.kind == hitMore {
			right = gfx.Blend(right, t.AccentText, .08)
		}
		if p.pressed.kind == hitOpen && p.hover.kind == hitOpen {
			left = gfx.Blend(left, t.Status, .25)
		}
		if p.pressed.kind == hitMore && p.hover.kind == hitMore {
			right = gfx.Blend(right, t.Status, .25)
		}
	}
	for _, r := range []image.Rectangle{g.open, g.more} {
		gfx.FillRound(c, r.Inset(-1), buttonRadius+1, shadowRing)
	}
	if ok {
		gap := image.Rect(g.open.Max.X, g.open.Min.Y, g.more.Min.X, g.open.Max.Y)
		c.FillRectAlpha(gap.Min.X, gap.Min.Y, gap.Dx(), gap.Dy(), gfx.Alpha(t.GalleryOpen, .28))
	}
	gfx.FillRound(c, g.open, buttonRadius, left)
	gfx.FillRound(c, g.more, buttonRadius, right)
	label := i18n.T("cmd.open")
	w := buttonFont.Measure(c, label)
	buttonFont.Draw(c, label, g.open.Min.X+(g.open.Dx()-w)/2, g.win.Max.Y-buttonBaseFromB, txt)
	icons.Draw(c, "chevron-down", g.more.Min.X+moreIconX, g.more.Min.Y+moreIconTop, chev)
}

func (p *Page) drawBars(c widget.DrawContext, t *theme.Theme) {
	if p.gridBar.shown {
		_, thumb, ok := p.gridBarRects()
		p.drawThumb(c, t, thumb, ok, p.gridBar.drag != nil || p.hover.kind == hitBar)
	}
	if p.navBar.shown {
		_, thumb, ok := p.navBarRects()
		p.drawThumb(c, t, thumb, ok, p.navBar.drag != nil || p.hover.kind == hitNavBar)
	}
}

func (p *Page) drawThumb(c widget.DrawContext, t *theme.Theme, thumb image.Rectangle, ok, active bool) {
	if !ok {
		return
	}
	a := .32
	if active {
		a = .6
	}
	gfx.FillRound(c, thumb, float64(thumb.Dx())/2, gfx.Alpha(t.ScrollThumb, a))
}

func (p *Page) drawBubble(c widget.DrawContext, t *theme.Theme) {
	if p.bubble == "" {
		return
	}
	g := p.geo
	w := min(bubbleFont.Measure(c, p.bubble)+40, g.address.Dx())
	r := image.Rect(g.address.Min.X, g.address.Max.Y+6, g.address.Min.X+w, g.address.Max.Y+36)
	gfx.FillRound(c, r.Inset(-1), 7, shadowRing)
	gfx.FillRound(c, r, 6, t.Menu)
	gfx.StrokeRound(c, r, 6, 1, t.MenuBorder)
	gfx.FillCircle(c, float64(r.Min.X+14), float64(r.Min.Y+15), 4, errorRed)
	bubbleFont.DrawFit(c, p.bubble, r.Min.X+26, r.Min.Y+19, w-36, t.Text)
}
