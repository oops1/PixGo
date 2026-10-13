package settings

import (
	"image"
	"math"

	"github.com/oops1/PixGo/internal/fonts"
	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/ui/gfx"
)

const (
	headerHeight  = 33
	contentTop    = 34
	backLeft      = 8
	backTop       = 4
	backRight     = 35
	backBottom    = 32
	backIconX     = 13
	backIconY     = 10
	headerTextX   = 39
	backPillRound = 4

	navPillLeft  = 8.6
	navBarLeft   = 7.5
	navBarRight  = 9.1
	navBarRadius = .8
	navIconX     = 13
	navTextX     = 39
	navHitLeft   = 4

	columnGap = 15
	textGap   = 6
	textExtra = 9

	checkBoxLeft  = .5
	chevronFromR  = 18
	cardLabelPad  = 6
	cardSlack     = 1
	barInset      = 16
	barRightInset = 2

	wheelNotchPixels = 40.0
)

type dims struct {
	k, ks float64

	headerBase int

	navW         int
	viewLeft     int
	navPillRight float64
	navPillUp    float64
	navPillDown  float64
	navPillRound float64
	navBarUp     float64
	navBarDown   float64
	navTextBase  int
	navTextWidth int
	navHitRight  int
	navHitUp     float64
	navHitDown   float64
	navCapTops   [sectionCount]int
	navIconTops  [sectionCount]int
	navCenters   [sectionCount]float64

	columnLeft  int
	columnMax   int
	columnFloor int

	titleTop      int
	titleHeight   int
	titleBase     int
	titleGap      int
	titleStep     int
	headingHeight int
	headingBase   int
	groupGap      float64
	rowHeight     int
	rowPitch      float64
	lineHeight    int
	noteBase      int
	valueHeight   int
	valueBase     int
	bottomPad     int

	checkBoxTop    float64
	checkBoxSize   float64
	checkBoxRadius float64
	checkRingW     float64
	radioDot       float64
	checkLabelX    int
	checkBase      int

	dropdownHeight int
	dropdownRadius float64
	dropdownTextX  int
	dropdownBase   int
	dropdownAfter  int
	chevronTop     int
	headingToField int
	headingToRows  int

	themeCardW      int
	themeCardPitch  float64
	themeItemH      int
	themeLabelBase  int
	themePicDX      int
	themePicDY      int
	themePicW       int
	themePicH       int
	themePicRadius  float64
	themeRingRadius float64
	themeRingW      float64
	themeRingH      float64
	themeRingIdle   float64
	themeRingOn     float64
	cardRowGap      int
	headingToTheme  int
	afterCards      int

	bgItemW      int
	bgItemH      int
	bgPitch      float64
	bgPicDX      int
	bgPicDY      int
	bgPicW       int
	bgPicH       int
	bgPicRadius  float64
	bgRingRadius float64
	bgRingWidth  float64
	bgItemRingW  float64
	bgItemRingH  float64
	bgSolidDX    float64
	bgSolidDY    float64
	bgSolidW     float64
	bgSolidH     float64
	bgLabelBase  int
	headingToBg  int
	afterBgCards int
	bgOffsets    []int
	checkerCell  int

	sliderThumb     float64
	sliderTrack     float64
	sliderCore      float64
	sliderCoreHeld  float64
	sliderCoreHover float64
	sliderLabelGap  int
	sliderMinLen    int
	sliderMaxLen    int
	sliderInset     int
	buttonHeight    int
	buttonPadding   int
	buttonMin       int
	buttonRadius    float64
	linkHeight      int
	linkBase        int

	formatRowHeight int
	formatSubBase   int
	formatGap       int

	barThin int
	barWide int

	wheelStep     float64
	pageStepKeep  int
	scrollKeyStep int
}

var mock = dims{
	k: 1, ks: 1,
	headerBase: 22,

	navW: 155, viewLeft: 156, navPillRight: 150.5, navPillUp: 13.3, navPillDown: 13.35, navPillRound: 4.5,
	navBarUp: 8.05, navBarDown: 7.95, navTextBase: 8, navTextWidth: 105, navHitRight: 152, navHitUp: 14, navHitDown: 15,
	navCapTops:  [sectionCount]int{52, 82, 111, 141, 171, 201, 231},
	navIconTops: [sectionCount]int{48, 77, 106, 136, 165, 195, 225},

	columnLeft: 168, columnMax: 560, columnFloor: 80,

	titleTop: 48, titleHeight: 17, titleBase: 14, titleGap: 15, titleStep: 20, headingHeight: 10, headingBase: 8, groupGap: 16.2,
	rowHeight: 22, rowPitch: 22.2, lineHeight: 13, noteBase: 10, valueHeight: 18, valueBase: 13, bottomPad: 24,

	checkBoxTop: 4.3, checkBoxSize: 13.3, checkBoxRadius: 3, checkRingW: 1.3, radioDot: 2.7, checkLabelX: 21, checkBase: 15,

	dropdownHeight: 25, dropdownRadius: 4.5, dropdownTextX: 10, dropdownBase: 16, dropdownAfter: 10, chevronTop: 6,
	headingToField: 7, headingToRows: 4,

	themeCardW: 84, themeCardPitch: 95.25, themeItemH: 66, themeLabelBase: 63, themePicDX: 5, themePicDY: 4,
	themePicW: 74, themePicH: 42, themePicRadius: 3.5, themeRingRadius: 6.5, themeRingW: 82.7, themeRingH: 49.2,
	themeRingIdle: 2.4, themeRingOn: 2.3, cardRowGap: 8, headingToTheme: 4, afterCards: 19,

	bgItemW: 59, bgItemH: 60, bgPitch: 71.4, bgPicDX: 8, bgPicDY: 6, bgPicW: 42, bgPicH: 34, bgPicRadius: 5,
	bgRingRadius: 8, bgRingWidth: 3.2, bgItemRingW: 58.6, bgItemRingH: 46, bgSolidDX: 8.2, bgSolidDY: 6.2,
	bgSolidW: 41.4, bgSolidH: 34.3, bgLabelBase: 57, headingToBg: 2, afterBgCards: 24,
	bgOffsets: []int{0, 72, 143, 214}, checkerCell: 5,

	sliderThumb: 14, sliderTrack: 3, sliderCore: 4.5, sliderCoreHeld: 3.5, sliderCoreHover: 5.5, sliderLabelGap: 8,
	sliderMinLen: 80, sliderMaxLen: 300, sliderInset: 7, buttonHeight: 25, buttonPadding: 14, buttonMin: 80,
	buttonRadius: 4.5, linkHeight: 18, linkBase: 13,

	formatRowHeight: 34, formatSubBase: 28, formatGap: 4,

	barThin: 3, barWide: 7,

	wheelStep: 48, pageStepKeep: 40, scrollKeyStep: 44,
}

var (
	dim     = newDims(1, 0)
	dimLang string
)

func newDims(k float64, labelW int) dims {
	m := mock
	d := m
	ks := 1 + (k-1)/2
	d.k, d.ks = k, ks
	f := func(v float64) float64 { return v * k }
	i := func(v int) int { return int(math.Round(float64(v) * k)) }
	sf := func(v float64) float64 { return v * ks }
	si := func(v int) int { return int(math.Round(float64(v) * ks)) }

	d.headerBase = m.headerBase + int(math.Round(3.6*(k-1)))

	d.navPillRight = math.Max(m.navPillRight, math.Min(float64(navTextX+labelW)+7*ks, 190*k))
	d.navW = int(math.Ceil(d.navPillRight + 4.5))
	d.viewLeft = d.navW + 1
	d.navPillUp, d.navPillDown, d.navPillRound = f(m.navPillUp), f(m.navPillDown), sf(m.navPillRound)
	d.navBarUp, d.navBarDown = sf(m.navBarUp), sf(m.navBarDown)
	d.navTextBase = int(math.Round(7*k)) + 1
	d.navTextWidth = int(d.navPillRight) - navTextX - 6
	d.navHitRight = d.navW - 3
	d.navHitUp, d.navHitDown = f(m.navHitUp), f(m.navHitDown)
	for s := range d.navCapTops {
		d.navCapTops[s] = int(math.Round(float64(contentTop) + float64(m.navCapTops[s]-contentTop)*k))
		d.navCenters[s] = float64(d.navCapTops[s]) + 3.65 + 3.5*(k-1)
		d.navIconTops[s] = m.navIconTops[s] + int(math.Round(d.navCenters[s]-(float64(m.navCapTops[s])+3.65)))
	}

	d.columnLeft = d.navW + si(13)
	d.columnMax, d.columnFloor = i(m.columnMax), i(m.columnFloor)

	d.titleTop = contentTop + i(m.titleTop-contentTop)
	d.titleHeight, d.titleBase, d.titleGap, d.titleStep = i(m.titleHeight), i(m.titleBase), i(m.titleGap), i(m.titleStep)
	d.headingHeight, d.headingBase, d.groupGap = i(m.headingHeight), i(m.headingBase), f(m.groupGap)
	d.rowHeight, d.rowPitch, d.lineHeight, d.noteBase = i(m.rowHeight), f(m.rowPitch), i(m.lineHeight), i(m.noteBase)
	d.valueHeight, d.valueBase, d.bottomPad = i(m.valueHeight), i(m.valueBase), i(m.bottomPad)

	d.checkBoxSize, d.checkBoxRadius = sf(m.checkBoxSize), sf(m.checkBoxRadius)
	d.checkBoxTop = m.checkBoxTop + ((f(m.rowPitch)-d.checkBoxSize)-(m.rowPitch-m.checkBoxSize))/2
	d.checkRingW, d.radioDot = sf(m.checkRingW), sf(m.radioDot)
	d.checkLabelX = int(math.Round(checkBoxLeft + d.checkBoxSize + 7.2*k))
	d.checkBase = i(m.checkBase)

	d.dropdownHeight, d.dropdownRadius = i(m.dropdownHeight), sf(m.dropdownRadius)
	d.dropdownTextX, d.dropdownBase, d.dropdownAfter = i(m.dropdownTextX), i(m.dropdownBase), i(m.dropdownAfter)
	d.chevronTop = (d.dropdownHeight - 12) / 2
	d.headingToField, d.headingToRows = i(m.headingToField), i(m.headingToRows)

	d.themeCardW, d.themeCardPitch = si(m.themeCardW), sf(m.themeCardPitch)
	d.themeItemH = int(math.Round(50*ks + 16*k))
	d.themeLabelBase = int(math.Round(50*ks + 13*k))
	d.themePicDX, d.themePicDY, d.themePicW, d.themePicH = si(m.themePicDX), si(m.themePicDY), si(m.themePicW), si(m.themePicH)
	d.themePicRadius, d.themeRingRadius = sf(m.themePicRadius), sf(m.themeRingRadius)
	d.themeRingW, d.themeRingH = sf(m.themeRingW), sf(m.themeRingH)
	d.themeRingIdle, d.themeRingOn = sf(m.themeRingIdle), sf(m.themeRingOn)
	d.cardRowGap, d.headingToTheme, d.afterCards = i(m.cardRowGap), i(m.headingToTheme), i(m.afterCards)

	d.bgItemW = si(m.bgItemW)
	d.bgItemH = int(math.Round(46*ks + 14*k))
	d.bgPitch = sf(m.bgPitch)
	d.bgPicDX, d.bgPicDY, d.bgPicW, d.bgPicH = si(m.bgPicDX), si(m.bgPicDY), si(m.bgPicW), si(m.bgPicH)
	d.bgPicRadius, d.bgRingRadius, d.bgRingWidth = sf(m.bgPicRadius), sf(m.bgRingRadius), sf(m.bgRingWidth)
	d.bgItemRingW, d.bgItemRingH = sf(m.bgItemRingW), sf(m.bgItemRingH)
	d.bgSolidDX, d.bgSolidDY, d.bgSolidW, d.bgSolidH = sf(m.bgSolidDX), sf(m.bgSolidDY), sf(m.bgSolidW), sf(m.bgSolidH)
	d.bgLabelBase = int(math.Round(40*ks + 17*k))
	d.headingToBg, d.afterBgCards = i(m.headingToBg), i(m.afterBgCards)
	d.bgOffsets = make([]int, len(m.bgOffsets))
	for n, v := range m.bgOffsets {
		d.bgOffsets[n] = si(v)
	}
	d.checkerCell = max(si(m.checkerCell), 1)

	d.sliderThumb, d.sliderTrack = sf(m.sliderThumb), sf(m.sliderTrack)
	d.sliderCore, d.sliderCoreHeld, d.sliderCoreHover = sf(m.sliderCore), sf(m.sliderCoreHeld), sf(m.sliderCoreHover)
	d.sliderLabelGap, d.sliderMinLen, d.sliderMaxLen, d.sliderInset = i(m.sliderLabelGap), m.sliderMinLen, si(m.sliderMaxLen), si(m.sliderInset)
	d.buttonHeight, d.buttonPadding, d.buttonMin, d.buttonRadius = i(m.buttonHeight), i(m.buttonPadding), i(m.buttonMin), sf(m.buttonRadius)
	d.linkHeight, d.linkBase = i(m.linkHeight), i(m.linkBase)

	d.formatRowHeight, d.formatSubBase, d.formatGap = i(m.formatRowHeight), i(m.formatSubBase), i(m.formatGap)

	d.barThin, d.barWide = si(m.barThin), si(m.barWide)

	d.wheelStep, d.pageStepKeep, d.scrollKeyStep = f(m.wheelStep), i(m.pageStepKeep), i(m.scrollKeyStep)
	return d
}

func syncDims() bool {
	k, lang := fonts.TextScale(), i18n.Lang()
	if dim.k == k && dimLang == lang {
		return false
	}
	labelW := 0
	for s := Section(0); s < sectionCount; s++ {
		labelW = max(labelW, measure(navFont, i18n.T(sectionKeys[s])))
	}
	dim, dimLang = newDims(k, labelW), lang
	return true
}

var (
	titleFont    = gfx.UISemi(12.725)
	headingFont  = gfx.UISemi(7.0)
	bodyFont     = gfx.UI(7.225)
	navFont      = gfx.UI(7.225)
	headerFont   = gfx.UI(7.45)
	captionFont  = gfx.UI(7.2)
	smallCapFont = gfx.UI(6.35)
	noteFont     = gfx.UI(6.7)
	aboutFont    = gfx.UISemi(10.5)
)

type geometry struct {
	win    image.Rectangle
	header image.Rectangle
	back   image.Rectangle
	nav    image.Rectangle
	view   image.Rectangle
	x0, x1 int
	xt     int
}

func backRect() image.Rectangle { return image.Rect(backLeft, backTop, backRight, backBottom) }

func computeGeometry(b image.Rectangle) geometry {
	w, h := b.Max.X+b.Min.X, b.Max.Y+b.Min.Y
	g := geometry{win: image.Rect(0, 0, w, h)}
	g.header = image.Rect(b.Min.X, b.Min.Y, b.Max.X, headerHeight)
	g.back = backRect()
	g.nav = image.Rect(b.Min.X, contentTop, dim.navW, max(b.Max.Y, contentTop))
	g.view = image.Rect(dim.viewLeft, contentTop, max(b.Max.X, dim.viewLeft), max(b.Max.Y, contentTop))
	g.x0 = dim.columnLeft
	g.x1 = max(min(w-columnGap, g.x0+dim.columnMax), g.x0+dim.columnFloor)
	g.xt = max(min(w-textGap, g.x1+textExtra), g.x1)
	return g
}

const (
	bgRingOffsetX = .4
	bgRingOffsetY = .3
)
