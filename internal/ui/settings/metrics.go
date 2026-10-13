package settings

import (
	"image"

	"github.com/oops1/PixGo/internal/ui/gfx"
)

const (
	headerHeight  = 33
	contentTop    = 34
	navDividerX   = 155
	viewLeft      = 156
	backLeft      = 8
	backTop       = 4
	backRight     = 35
	backBottom    = 32
	backIconX     = 13
	backIconY     = 10
	headerTextX   = 39
	headerBase    = 22
	backPillRound = 4
)

const (
	navPillLeft    = 8.6
	navPillRight   = 150.5
	navPillUp      = 13.3
	navPillDown    = 13.35
	navPillRadius  = 4.5
	navBarLeft     = 7.5
	navBarRight    = 9.1
	navBarUp       = 8.05
	navBarDown     = 7.95
	navBarRadius   = .8
	navIconX       = 13
	navTextX       = 39
	navTextBase    = 8
	navTextWidth   = 105
	navCenterBelow = 3.65
	navHitLeft     = 4
	navHitRight    = 152
	navHitUp       = 14
	navHitDown     = 15
)

const (
	columnLeft  = 168
	columnGap   = 15
	columnMax   = 560
	columnFloor = 120
	textGap     = 6
	textExtra   = 9
)

const (
	titleTop       = 48
	titleHeight    = 17
	titleBase      = 14
	titleGap       = 15
	headingHeight  = 10
	headingBase    = 8
	groupGap       = 16.2
	rowHeight      = 22
	rowPitch       = 22.2
	lineHeight     = 13
	noteBase       = 10
	valueHeight    = 18
	valueBase      = 13
	checkBoxLeft   = .5
	checkBoxTop    = 4.3
	checkBoxSize   = 13.3
	checkBoxRadius = 3
	checkLabelX    = 21
	checkBase      = 15
	checkMarkX     = 0
	checkMarkY     = 1
	bottomPad      = 24
)

const (
	dropdownHeight = 25
	dropdownRadius = 4.5
	dropdownTextX  = 10
	dropdownBase   = 16
	dropdownAfter  = 10
	chevronFromR   = 18
	chevronTop     = 6
	headingToField = 7
	headingToRows  = 4
)

const (
	themeCardW      = 84
	themeCardPitch  = 95.25
	themeItemH      = 66
	themeLabelBase  = 63
	themePicDX      = 5
	themePicDY      = 4
	themePicW       = 74
	themePicH       = 42
	themePicRadius  = 3.5
	themeRingRadius = 6.5
	themeRingW      = 82.7
	themeRingH      = 49.2
	themeRingIdle   = 2.4
	themeRingOn     = 2.3
	cardRowGap      = 8
	cardSlack       = 1
	headingToTheme  = 4
	afterCards      = 19
)

const (
	bgItemW       = 59
	bgItemH       = 60
	bgPitch       = 71.4
	bgPicDX       = 8
	bgPicDY       = 6
	bgPicW        = 42
	bgPicH        = 34
	bgPicRadius   = 5
	bgRingRadius  = 8
	bgRingWidth   = 3.2
	bgRingOffsetX = .4
	bgRingOffsetY = .3
	bgItemRingW   = 58.6
	bgItemRingH   = 46
	bgSolidDX     = 8.2
	bgSolidDY     = 6.2
	bgSolidW      = 41.4
	bgSolidH      = 34.3
	bgLabelBase   = 57
	headingToBg   = 2
	afterBgCards  = 24
)

const (
	sliderThumb   = 14
	sliderTrack   = 3
	sliderValueW  = 92
	sliderMinLen  = 80
	sliderMaxLen  = 300
	sliderInset   = 7
	buttonHeight  = 25
	buttonPadding = 14
	buttonMin     = 80
	buttonRadius  = 4.5
	linkHeight    = 18
	linkBase      = 13

	formatRowHeight = 34
	formatSubBase   = 28
	formatGap       = 4
)

const (
	barInset      = 16
	barThin       = 3
	barWide       = 7
	barRightInset = 2

	wheelNotchPixels = 40.0
	wheelStep        = 48.0
	pageStepKeep     = 40
	scrollKeyStep    = 44
)

var bgOffsets = []int{0, 72, 143, 214}

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

func computeGeometry(b image.Rectangle) geometry {
	w, h := b.Max.X+b.Min.X, b.Max.Y+b.Min.Y
	g := geometry{win: image.Rect(0, 0, w, h)}
	g.header = image.Rect(b.Min.X, b.Min.Y, b.Max.X, headerHeight)
	g.back = image.Rect(backLeft, backTop, backRight, backBottom)
	g.nav = image.Rect(b.Min.X, contentTop, navDividerX, max(b.Max.Y, contentTop))
	g.view = image.Rect(viewLeft, contentTop, max(b.Max.X, viewLeft), max(b.Max.Y, contentTop))
	g.x0 = columnLeft
	g.x1 = max(min(w-columnGap, columnLeft+columnMax), columnLeft+columnFloor)
	g.xt = max(min(w-textGap, g.x1+textExtra), g.x1)
	return g
}
