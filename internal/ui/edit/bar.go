package edit

import (
	"image"

	"github.com/oops1/PixGo/internal/fonts"
	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/ui/gfx"
)

type Part uint8

const (
	PartNone Part = iota
	PartCrop
	PartAspect
	PartSwap
	PartRotateLeft
	PartRotateRight
	PartFlipH
	PartFlipV
	PartResize
	PartReset
	PartSaveCopy
	PartSaveMore
	PartCancel
	PartSize
)

var barStops = []Part{PartAspect, PartSwap, PartRotateLeft, PartRotateRight, PartFlipH, PartFlipV, PartResize, PartReset, PartSaveCopy, PartSaveMore, PartCancel}

const (
	barPad      = 8
	barGap      = 4
	barBtn      = 36
	barIconText = 8
	barRadius   = 12
	barTop      = 12
	barSide     = 12
	barRowGap   = 4
	barSepGap   = 6
	areaSide    = 28
	areaGap     = 12
	areaBottom  = 56
	splitW      = 28
	textPadX    = 14
)

var (
	labelFont = gfx.UI(9)
	smallFont = gfx.UI(8.5)
)

func textHeightOf(f gfx.Font) int {
	m := fonts.MetricsOf(f.Name, f.Size(), 96)
	return m.Ascent + m.Descent
}

type barLayout struct {
	bar   image.Rectangle
	rects map[Part]image.Rectangle
	sep   image.Rectangle
	rows  int
	level int
}

func (l barLayout) rect(p Part) image.Rectangle { return l.rects[p] }

type barItem struct {
	part  Part
	w     int
	right bool
}

func (e *Editor) aspectLabel() string { return i18n.T(aspectKey(e.crop.Aspect())) }

func aspectKey(a Aspect) string {
	switch a {
	case AspectOriginal:
		return "edit.aspect.orig"
	case AspectSquare:
		return "edit.aspect.1x1"
	case Aspect43:
		return "edit.aspect.4x3"
	case Aspect32:
		return "edit.aspect.3x2"
	case Aspect169:
		return "edit.aspect.16x9"
	}
	return "edit.aspect.free"
}

func AspectKey(a Aspect) string { return aspectKey(a) }

func iconTextWidth(label string) int {
	return textPadX + 20 + barIconText + labelFont.Measure(nil, label) + textPadX
}

func plainTextWidth(label string) int {
	return 2*textPadX + labelFont.Measure(nil, label)
}

const (
	levelFull = iota
	levelIcons
	levelTight
)

func (e *Editor) barItems(level int) []barItem {
	aspectW := textPadX - 2 + 20 + barIconText + labelFont.Measure(nil, e.widestAspect()) + barIconText + 12 + textPadX - 2
	if level == levelTight {
		aspectW = textPadX - 2 + 20 + barIconText + 12 + textPadX - 2
	}
	items := []barItem{
		{PartAspect, aspectW, false},
		{PartSwap, barBtn, false},
		{PartRotateLeft, barBtn, false},
		{PartRotateRight, barBtn, false},
		{PartFlipH, barBtn, false},
		{PartFlipV, barBtn, false},
	}
	if level >= levelIcons {
		items = append(items, barItem{PartResize, barBtn, false}, barItem{PartReset, barBtn, false})
	} else {
		items = append(items,
			barItem{PartResize, iconTextWidth(i18n.T("edit.resize")), false},
			barItem{PartReset, iconTextWidth(i18n.T("edit.reset")), false})
	}
	items = append(items,
		barItem{PartSaveCopy, plainTextWidth(i18n.T("edit.saveCopy")), true},
		barItem{PartSaveMore, splitW, true},
		barItem{PartCancel, plainTextWidth(i18n.T("edit.cancel")), true})
	return items
}

func (e *Editor) widestAspect() string {
	best := ""
	for _, a := range Aspects {
		if s := i18n.T(aspectKey(a)); labelFont.Measure(nil, s) > labelFont.Measure(nil, best) {
			best = s
		}
	}
	return best
}

func gapAfter(p Part) int {
	if p == PartSaveCopy {
		return 1
	}
	return barGap
}

func rowWidth(items []barItem, right bool) int {
	w, last := 0, PartNone
	for _, it := range items {
		if it.right != right {
			continue
		}
		if last != PartNone {
			w += gapAfter(last)
		}
		w += it.w
		last = it.part
	}
	return w
}

func (e *Editor) computeBar(bounds image.Rectangle) barLayout {
	avail := bounds.Dx() - 2*barSide
	l := barLayout{rects: map[Part]image.Rectangle{}}
	sepW := 2*barSepGap + 1
	var items []barItem
	fits := false
	for l.level = levelFull; l.level <= levelTight && !fits; l.level++ {
		items = e.barItems(l.level)
		need := 2*barPad + rowWidth(items, false) + rowWidth(items, true) + sepW + 2*barPad
		fits = need <= avail
	}
	l.level--
	h := barBtn
	if fits {
		l.rows = 1
		l.bar = image.Rect(bounds.Min.X+barSide, bounds.Min.Y+barTop, bounds.Max.X-barSide, bounds.Min.Y+barTop+h+2*barPad)
		e.place(&l, items, false, l.bar.Min.X+barPad, l.bar.Min.Y+barPad, true)
		e.place(&l, items, true, l.bar.Max.X-barPad-rowWidth(items, true), l.bar.Min.Y+barPad, false)
		return l
	}
	l.rows = 2
	height := 2*h + barRowGap + 2*barPad
	l.bar = image.Rect(bounds.Min.X+barSide, bounds.Min.Y+barTop, bounds.Max.X-barSide, bounds.Min.Y+barTop+height)
	e.place(&l, items, false, l.bar.Min.X+barPad, l.bar.Min.Y+barPad, true)
	e.place(&l, items, true, l.bar.Max.X-barPad-rowWidth(items, true), l.bar.Min.Y+barPad+h+barRowGap, false)
	return l
}

func (e *Editor) place(l *barLayout, items []barItem, right bool, x, y int, sep bool) {
	for _, it := range items {
		if it.right != right {
			continue
		}
		if sep && it.part == PartResize {
			l.sep = image.Rect(x+barSepGap-barGap/2, y+6, x+barSepGap-barGap/2+1, y+barBtn-6)
			x += 2*barSepGap + 1 - barGap
		}
		l.rects[it.part] = image.Rect(x, y, x+it.w, y+barBtn)
		x += it.w + gapAfter(it.part)
	}
}
