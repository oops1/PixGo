package gallery

import (
	"image"
	"math"
)

type hitKind uint8

const (
	hitNone hitKind = iota
	hitBack
	hitForward
	hitUp
	hitAddress
	hitFilter
	hitRefresh
	hitViewList
	hitViewGrid
	hitThumbs
	hitNav
	hitTile
	hitBlank
	hitBar
	hitTrack
	hitNavBar
	hitOpen
	hitMore
)

type hit struct {
	kind  hitKind
	index int
}

var noHit = hit{}

func in(r image.Rectangle, x, y int) bool { return image.Pt(x, y).In(r) }

func (p *Page) hitAt(x, y int) hit {
	if !in(p.Bounds(), x, y) {
		return noHit
	}
	p.ensureLayout()
	g := p.geo
	switch {
	case in(g.open, x, y):
		return hit{kind: hitOpen}
	case in(g.more, x, y):
		return hit{kind: hitMore}
	case y < headerHeight:
		return p.headerHit(x, y)
	case in(g.sidebar, x, y):
		return p.navHit(x, y)
	case in(g.grid, x, y):
		return p.gridHit(x, y)
	}
	return noHit
}

func (p *Page) headerHit(x, y int) hit {
	g := p.geo
	switch {
	case in(g.back, x, y):
		return hit{kind: hitBack}
	case in(g.forward, x, y):
		return hit{kind: hitForward}
	case in(g.up, x, y):
		return hit{kind: hitUp}
	case in(g.filter, x, y):
		return hit{kind: hitFilter}
	case in(g.refresh, x, y):
		return hit{kind: hitRefresh}
	case in(g.address, x, y):
		return hit{kind: hitAddress}
	case in(g.viewList, x, y):
		return hit{kind: hitViewList}
	case in(g.viewGrid, x, y):
		return hit{kind: hitViewGrid}
	case in(g.thumbs, x, y):
		return hit{kind: hitThumbs}
	}
	return noHit
}

func (p *Page) navHit(x, y int) hit {
	top := p.geo.sidebar.Min.Y - p.sideY
	right := p.geo.sidebar.Max.X
	if _, thumb, ok := p.navBarRects(); ok && p.navBar.shown && in(thumb.Inset(-6), x, y) {
		return hit{kind: hitNavBar}
	}
	if x < navLeft || x >= right-navRightInset {
		return noHit
	}
	best, dist := -1, float64(navHitHalf)+.5
	for i, r := range p.rows {
		if d := math.Abs(float64(y-top) + .5 - r.center); d < dist {
			best, dist = i, d
		}
	}
	if best < 0 || p.rows[best].entry.Header || p.rows[best].entry.Disabled {
		return noHit
	}
	return hit{kind: hitNav, index: p.rows[best].index}
}

func (p *Page) gridHit(x, y int) hit {
	if _, thumb, ok := p.gridBarRects(); ok && p.gridBar.shown && in(thumb.Inset(-6), x, y) {
		return hit{kind: hitBar}
	}
	vp := p.geo.grid
	if i := p.grid.At(x-vp.Min.X, y-vp.Min.Y, p.scroll); i >= 0 {
		return hit{kind: hitTile, index: i}
	}
	return hit{kind: hitBlank}
}

func (p *Page) tileRect(i int) image.Rectangle {
	vp := p.geo.grid
	return p.grid.Rect(i).Add(image.Pt(vp.Min.X, vp.Min.Y-p.scroll))
}

func (p *Page) photoRect(i int) image.Rectangle {
	cell := p.tileRect(i)
	return image.Rect(cell.Min.X+photoInsetX, cell.Min.Y+photoInsetY,
		cell.Min.X+photoInsetX+p.tile.photoW, cell.Min.Y+photoInsetY+p.tile.photoH)
}
