package gallery

import (
	"image"

	"github.com/oops1/PixGo/internal/ui/gfx"
)

const (
	headerHeight  = 42
	sidebarWidth  = 151
	statusFromBot = 41
	controlTop    = 8
	controlBottom = 35
	controlRadius = 6
	navPillRadius = 4

	addressLeft      = 93
	addressRightGap  = 214
	addressMinWidth  = 140
	addressIconX     = 7
	addressIconY     = 6
	addressTextX     = 32
	filterIconFromR  = 42
	refreshIconFromR = 17
	filterHitFromR   = 48
	refreshHitFromR  = 24
	viewsFromR       = 202
	viewsWidth       = 57
	viewListIconX    = 7
	viewGridIconX    = 35
	viewHalf         = 29
	thumbsIconFromR  = 136
	thumbsHitFromR   = 141
	thumbsHitWidth   = 26
	iconTop          = 14
	refreshIconTop   = 16

	backIconX    = 17
	forwardIconX = 44
	upIconX      = 69

	openRightFromR    = 51
	moreLeftFromR     = 49
	moreRightFromR    = 18
	buttonTopFromB    = 46
	buttonBotFromB    = 14
	buttonRadius      = 6
	moreIconX         = 9
	moreIconTop       = 9
	statusBaseFromB   = 17
	statusLimitGap    = 12
	statusNoticeMin   = 40
	statusTextX       = 22
	statusGap         = 16
	buttonBaseFromMid = 4

	gridPadLeft   = 8
	gridPadRight  = 8
	gridPadTop    = 10
	gridPadBottom = 10
	gridGapX      = 1
	gridGapY      = 5
	photoRadius   = 6
	cellRadius    = 8
	cellFrame     = 2
	photoInsetX   = 4
	photoInsetY   = 4
	labelBelow    = 18
	cellFooter    = 26
	cellExtraW    = 9

	listRowHeight    = 40
	listThumb        = 32
	listPad          = 8
	listSideTile     = 96
	bubbleBaseHeight = 30

	barInset      = 16
	barThin       = 3
	barWide       = 7
	barRightInset = 2

	navLeft        = 8
	navRightInset  = 5
	navPillHalf    = 12
	navFirstCenter = 21.5
	navTextTop     = 42
	navTextSection = 44
	navHeaderX     = 21
	navIconTop     = 20
	navIconSection = 23
	navExpanderX   = 12
	navHitHalf     = 11

	wheelNotchPixels = 40.0
)

var (
	navFont     = gfx.UI(7.3625)
	sectionFont = gfx.UI(7.7375)
	labelFont   = gfx.UI(7.25)
	addressFont = gfx.UI(8.0)
	statusFont  = gfx.UI(8.0)
	buttonFont  = gfx.UI(9.0)
	emptyFont   = gfx.UI(9.0)
	listFont    = gfx.UI(8.0)
	listSubFont = gfx.UI(7.5)
	bubbleFont  = gfx.UI(8.0)
)

type geometry struct {
	win      image.Rectangle
	header   image.Rectangle
	sidebar  image.Rectangle
	grid     image.Rectangle
	status   image.Rectangle
	statusY  int
	back     image.Rectangle
	forward  image.Rectangle
	up       image.Rectangle
	address  image.Rectangle
	filter   image.Rectangle
	refresh  image.Rectangle
	views    image.Rectangle
	viewList image.Rectangle
	viewGrid image.Rectangle
	thumbs   image.Rectangle
	open     image.Rectangle
	more     image.Rectangle
}

func computeGeometry(b image.Rectangle, m textMetrics) geometry {
	w, h := b.Max.X+b.Min.X, b.Max.Y+b.Min.Y
	g := geometry{win: image.Rect(0, 0, w, h)}
	g.statusY = max(h-statusFromBot-m.statusSx, headerHeight+1)
	g.header = image.Rect(b.Min.X, b.Min.Y, b.Max.X, headerHeight)
	g.sidebar = image.Rect(b.Min.X, headerHeight+1, min(m.sideW, b.Max.X), g.statusY)
	g.grid = image.Rect(m.sideW+1, headerHeight+1, max(b.Max.X, m.sideW+1), g.statusY)
	g.status = image.Rect(b.Min.X, g.statusY, b.Max.X, b.Max.Y)

	g.back = image.Rect(12, controlTop, 38, 36)
	g.forward = image.Rect(38, controlTop, 64, 36)
	g.up = image.Rect(64, controlTop, 90, 36)

	vl := w - viewsFromR
	right := min(max(w-addressRightGap, addressLeft+addressMinWidth), max(vl-8, addressLeft+60))
	g.address = image.Rect(addressLeft, controlTop, right, controlBottom)
	g.filter = image.Rect(right-filterHitFromR, controlTop, right-refreshHitFromR, controlBottom)
	g.refresh = image.Rect(right-refreshHitFromR, controlTop, right-3, controlBottom)

	g.views = image.Rect(vl, controlTop, vl+viewsWidth, controlBottom)
	g.viewList = image.Rect(vl, controlTop, vl+viewHalf, controlBottom)
	g.viewGrid = image.Rect(vl+viewHalf, controlTop, vl+viewsWidth, controlBottom)
	g.thumbs = image.Rect(w-thumbsHitFromR, controlTop, w-thumbsHitFromR+thumbsHitWidth, controlBottom)

	top := h - buttonTopFromB - m.btnExtra
	g.open = image.Rect(w-openRightFromR-m.openW, top, w-openRightFromR, h-buttonBotFromB)
	g.more = image.Rect(w-moreLeftFromR, top, w-moreRightFromR, h-buttonBotFromB)
	return g
}

type tileGeometry struct {
	photoW, photoH int
	cellW, cellH   int
}

func tileGeometryFor(side int, m textMetrics) tileGeometry {
	pw := max(side-1, 16)
	ph := (pw*3 + 2) / 4
	return tileGeometry{photoW: pw, photoH: ph, cellW: pw + cellExtraW, cellH: ph + cellFooter + m.labelExtra}
}
