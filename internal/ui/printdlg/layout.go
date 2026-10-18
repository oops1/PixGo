package printdlg

import (
	"image"
	"math"

	"github.com/oops1/PixGo/internal/fonts"
	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/ui/gfx"
)

type Part uint8

const (
	PartNone Part = iota
	PartPrinter
	PartSystem
	PartCopies
	PartCopiesDec
	PartCopiesInc
	PartPaper
	PartOrientation
	PartPlacement
	PartWidth
	PartHeight
	PartCm
	PartInch
	PartKeep
	PartMargins
	PartPrint
	PartCancel
	partCount
)

var (
	bodyFont = gfx.UI(7.5)
	hintFont = gfx.UI(6.7)
)

const (
	minGapShare = 0.25
	infoLines   = 3
	listRows    = 9
)

type metrics struct {
	k, ks     float64
	padX      int
	padTop    int
	padBottom int
	fieldH    int
	rowGap    int
	labelGap  int
	boxSize   int
	boxGap    int
	btnGap    int
	btnPad    int
	btnMinW   int
	btnMinH   int
	colGap    int
	lineGap   int
	stepW     int
	itemH     int
	timesW    int
}

func newMetrics(k float64) metrics {
	ks := 1 + (k-1)/2
	s := func(v float64) int { return int(math.Round(v * k)) }
	ss := func(v float64) int { return int(math.Round(v * ks)) }
	return metrics{
		k: k, ks: ks,
		padX: s(18), padTop: s(16), padBottom: s(14),
		fieldH: s(32), rowGap: s(10), labelGap: s(12),
		boxSize: ss(16), boxGap: s(8),
		btnGap: s(8), btnPad: s(18), btnMinW: s(88), btnMinH: s(30),
		colGap: s(20), lineGap: s(2), stepW: s(32), itemH: s(30), timesW: s(22),
	}
}

func (m metrics) s(v float64) int { return int(math.Round(v * m.k)) }

type layout struct {
	m    metrics
	size image.Point
	wide bool

	view      image.Rectangle
	contentH  int
	scrollMax int

	ctl [partCount]image.Rectangle
	lbl [partCount]image.Rectangle

	times   image.Rectangle
	preview image.Rectangle
	info    image.Rectangle
	rule    image.Rectangle

	export bool
	system bool
	gq     float64
}

func textHeight(f gfx.Font) int {
	mt := fonts.MetricsOf(f.Name, f.Size(), 96)
	return mt.Ascent + mt.Descent
}

var labelKeys = map[Part]string{
	PartPrinter:     "print.printer",
	PartCopies:      "print.copies",
	PartPaper:       "print.paper",
	PartOrientation: "print.orientation",
	PartPlacement:   "print.placement",
	PartWidth:       "print.size",
	PartMargins:     "print.margins",
}

func compute(maxW, maxH int, export, system bool) layout {
	for q := 1.0; ; q -= 0.05 {
		l := build(maxW, maxH, export, system, q)
		if maxH <= 0 || l.size.Y <= maxH || q <= minGapShare+0.001 {
			return l
		}
	}
}

func build(maxW, maxH int, export, system bool, gq float64) layout {
	m := newMetrics(fonts.TextScale())
	g := func(v int) int { return int(math.Round(float64(v) * gq)) }
	lineH := textHeight(bodyFont)
	l := layout{m: m, export: export, system: system, gq: gq}

	labelW := 0
	for p, key := range labelKeys {
		if export && (p == PartPrinter || p == PartCopies) {
			continue
		}
		labelW = max(labelW, bodyFont.Measure(nil, i18n.T(key)))
	}
	labelW = max(labelW, m.s(70))

	printW := max(m.btnMinW, bodyFont.Measure(nil, i18n.T("print.print"))+2*m.btnPad)
	printW = max(printW, bodyFont.Measure(nil, i18n.T("print.savePdf"))+2*m.btnPad)
	cancelW := max(m.btnMinW, bodyFont.Measure(nil, i18n.T("dialog.cancel"))+2*m.btnPad)
	sysW := bodyFont.Measure(nil, i18n.T("print.system")) + 2*m.btnPad

	cw := max(m.s(380), labelW+m.labelGap+m.s(250))
	pw := m.s(300)
	total := func() int { return 2*m.padX + cw + m.colGap + pw }
	l.wide = true
	if maxW > 0 && total() > maxW {
		avail := maxW - 2*m.padX - m.colGap
		pw = m.s(300)
		cw = avail - pw
		if cw < m.s(360) {
			pw = max(m.s(220), avail-m.s(360))
			cw = avail - pw
		}
		if cw < max(m.s(340), labelW+m.labelGap+m.s(200)) {
			l.wide = false
		}
	}
	var w int
	if l.wide {
		w = total()
	} else {
		w = m.s(440)
		if maxW > 0 {
			w = min(w, maxW)
		}
		w = max(w, 2*m.padX+m.s(180))
		cw = w - 2*m.padX
	}
	labelW = min(labelW, cw*45/100)
	cx := labelW + m.labelGap
	cwc := cw - cx

	y := 0
	row := func(p Part, h int) image.Rectangle {
		lr := image.Rect(0, y, labelW, y+h)
		l.lbl[p] = lr
		r := image.Rect(cx, y, cw, y+h)
		return r
	}
	next := func() { y += m.fieldH + g(m.rowGap) }
	if !export {
		l.ctl[PartPrinter] = row(PartPrinter, m.fieldH)
		next()
		if system {
			l.ctl[PartSystem] = image.Rect(cx, y, cx+min(cwc, max(sysW, m.btnMinW)), y+m.btnMinH)
			y += m.btnMinH + g(m.rowGap)
		}
		stepW := min(m.stepW, max(m.s(20), (cwc-m.s(44)-m.s(12))/2))
		copiesW := max(m.s(44), min(m.s(70), cwc-2*stepW-m.s(12)))
		l.ctl[PartCopies] = image.Rect(cx, y, cx+copiesW, y+m.fieldH)
		l.lbl[PartCopies] = image.Rect(0, y, labelW, y+m.fieldH)
		l.ctl[PartCopiesDec] = image.Rect(l.ctl[PartCopies].Max.X+m.s(8), y, l.ctl[PartCopies].Max.X+m.s(8)+stepW, y+m.fieldH)
		l.ctl[PartCopiesInc] = image.Rect(l.ctl[PartCopiesDec].Max.X+m.s(4), y, l.ctl[PartCopiesDec].Max.X+m.s(4)+stepW, y+m.fieldH)
		next()
	}
	l.ctl[PartPaper] = row(PartPaper, m.fieldH)
	next()
	l.ctl[PartOrientation] = row(PartOrientation, m.fieldH)
	next()
	l.ctl[PartPlacement] = row(PartPlacement, m.fieldH)
	next()
	l.lbl[PartWidth] = image.Rect(0, y, labelW, y+m.fieldH)
	fw := (cwc - m.timesW) / 2
	l.ctl[PartWidth] = image.Rect(cx, y, cx+fw, y+m.fieldH)
	l.times = image.Rect(cx+fw, y, cx+fw+m.timesW, y+m.fieldH)
	l.ctl[PartHeight] = image.Rect(cx+fw+m.timesW, y, cx+2*fw+m.timesW, y+m.fieldH)
	next()
	segW := min(cwc, m.s(170))
	segH := max(m.btnMinH, lineH+m.s(12))
	l.ctl[PartCm] = image.Rect(cx, y, cx+segW/2, y+segH)
	l.ctl[PartInch] = image.Rect(cx+segW/2, y, cx+segW, y+segH)
	y += segH + g(m.rowGap)
	rowH := max(m.boxSize, lineH) + m.s(8)
	l.ctl[PartKeep] = image.Rect(cx, y, cw, y+rowH)
	y += rowH + g(m.rowGap)
	l.ctl[PartMargins] = row(PartMargins, m.fieldH)
	y += m.fieldH

	infoH := infoLines*(lineH+m.lineGap) + m.s(4)
	btnH := max(m.btnMinH, lineH+m.s(14))
	barH := g(m.s(12)) + 1 + g(m.s(12)) + btnH + m.padBottom

	top := m.padTop
	if l.wide {
		l.contentH = y
		bodyH := l.contentH
		minBody := m.s(200) + infoH + g(m.s(12))
		bodyH = max(bodyH, minBody)
		h := top + bodyH + barH
		l.size = image.Pt(w, h)
	} else {
		y += g(m.s(16))
		pvH := min(m.s(200), cw)
		l.preview = image.Rect(0, y, cw, y+pvH)
		y += pvH + g(m.s(8))
		l.info = image.Rect(0, y, cw, y+infoH)
		y += infoH
		l.contentH = y
		l.size = image.Pt(w, top+l.contentH+barH)
	}
	if maxH > 0 && l.size.Y > maxH {
		l.size.Y = max(maxH, top+m.s(60)+barH)
	}
	viewH := l.size.Y - top - barH
	l.view = image.Rect(m.padX, top, m.padX+cw, top+viewH)
	l.scrollMax = max(0, l.contentH-viewH)
	if l.wide {
		x0 := m.padX + cw + m.colGap
		pvBottom := top + viewH - infoH - g(m.s(12))
		l.preview = image.Rect(x0, top, x0+pw, max(top+m.s(60), pvBottom))
		l.info = image.Rect(x0, l.preview.Max.Y+g(m.s(12)), x0+pw, l.preview.Max.Y+g(m.s(12))+infoH)
	}

	barTop := l.size.Y - barH
	l.rule = image.Rect(0, barTop+g(m.s(12)), w, barTop+g(m.s(12))+1)
	by := l.rule.Max.Y + g(m.s(12))
	right := w - m.padX
	if room := w - 2*m.padX; printW+m.btnGap+cancelW > room {
		printW = (room - m.btnGap) * printW / (printW + cancelW)
		cancelW = room - m.btnGap - printW
	}
	l.ctl[PartCancel] = image.Rect(right-cancelW, by, right, by+btnH)
	l.ctl[PartPrint] = image.Rect(l.ctl[PartCancel].Min.X-m.btnGap-printW, by, l.ctl[PartCancel].Min.X-m.btnGap, by+btnH)
	return l
}

func fixedPart(p Part) bool { return p == PartPrint || p == PartCancel }
