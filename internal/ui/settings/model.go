package settings

import (
	"image"

	data "github.com/oops1/PixGo/internal/settings"
)

type Section uint8

const (
	SectionGeneral Section = iota
	SectionView
	SectionFormats
	SectionSlideshow
	SectionPerformance
	SectionLanguage
	SectionAbout
	sectionCount
)

var sectionIcons = [sectionCount]string{"settings", "view", "formats", "slideshow", "performance", "language", "about"}

var sectionKeys = [sectionCount]string{
	"settings.section.general", "settings.section.view", "settings.section.formats",
	"settings.section.slideshow", "settings.section.performance", "settings.section.language",
	"settings.section.about",
}

type kind uint8

const (
	kindTitle kind = iota
	kindHeading
	kindNote
	kindCheck
	kindRadio
	kindCard
	kindDropdown
	kindSlider
	kindButton
	kindLink
	kindFormat
	kindValue
)

type picture uint8

const (
	picNone picture = iota
	picThemeSystem
	picThemeLight
	picThemeDark
	picBgSystem
	picBgWhite
	picBgBlack
	picBgChecker
)

type choice struct {
	id     string
	text   string
	on     bool
	change func(*data.Settings)
}

type item struct {
	kind      kind
	id        string
	r         image.Rectangle
	text      string
	lines     []string
	detail    string
	on        bool
	disabled  bool
	group     int
	pic       picture
	indent    int
	value     int
	lo, hi    int
	step      int
	valueText string
	labelW    int
	stacked   bool
	choices   []choice
	change    func(*data.Settings)
	slide     func(v int) func(*data.Settings)
	run       func()
	box       image.Rectangle
	name      string
	fy        float64
}

func (it *item) focusable() bool {
	switch it.kind {
	case kindCheck, kindRadio, kindCard, kindDropdown, kindSlider, kindButton, kindLink:
		return !it.disabled
	case kindFormat:
		return !it.disabled && it.change != nil
	}
	return false
}

func (it *item) grouped() bool { return it.group != 0 }

type hitKind uint8

const (
	hitNone hitKind = iota
	hitBack
	hitNav
	hitItem
	hitBar
)

type hit struct {
	kind  hitKind
	index int
}

var noHit = hit{}
