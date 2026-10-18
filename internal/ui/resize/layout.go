package resize

import (
	"image"
	"math"
	"strings"

	"github.com/oops1/PixGo/internal/fonts"
	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/ui/gfx"
)

const (
	minGapShare = 0.25
	squeezeStep = 0.02
)

var (
	bodyFont = gfx.UI(7.5)
	hintFont = gfx.UI(6.7)
)

type metrics struct {
	k, ks     float64
	padX      int
	padTop    int
	padBottom int
	boxSize   int
	boxGap    int
	btnGap    int
	btnPad    int
	btnMinW   int
	btnMinH   int
	fieldH    int
	colGap    int
	lineGap   int
}

func newMetrics(k float64) metrics {
	ks := 1 + (k-1)/2
	s := func(v float64) int { return int(math.Round(v * k)) }
	ss := func(v float64) int { return int(math.Round(v * ks)) }
	return metrics{
		k: k, ks: ks,
		padX: s(18), padTop: s(16), padBottom: s(14),
		boxSize: ss(16), boxGap: s(8),
		btnGap: s(8), btnPad: s(18), btnMinW: s(88), btnMinH: s(30),
		fieldH: s(32), colGap: s(12), lineGap: s(2),
	}
}

func (m metrics) s(v float64) int { return int(math.Round(v * m.k)) }

type layout struct {
	size     image.Point
	pixels   image.Rectangle
	percent  image.Rectangle
	wLabel   image.Rectangle
	hLabel   image.Rectangle
	width    image.Rectangle
	height   image.Rectangle
	keep     image.Rectangle
	orig     image.Rectangle
	result   image.Rectangle
	hint     image.Rectangle
	apply    image.Rectangle
	cancel   image.Rectangle
	m        metrics
	hintText []string
	gapTotal int
	squeeze  float64
}

func textHeight(f gfx.Font) int {
	m := fonts.MetricsOf(f.Name, f.Size(), 96)
	return m.Ascent + m.Descent
}

func wrap(f gfx.Font, text string, width, maxLines int) []string {
	var lines []string
	cur := ""
	for _, word := range i18n.Words(text) {
		switch {
		case cur == "":
			cur = word
		case f.Measure(nil, cur+" "+word) <= width:
			cur += " " + word
		default:
			lines = append(lines, cur)
			cur = word
		}
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	if len(lines) > maxLines {
		lines = append(lines[:maxLines-1], f.Fit(nil, strings.Join(lines[maxLines-1:], " "), width))
	}
	return lines
}

func compute(maxW, maxH int, lim Limits) layout {
	for detail := detailFull; ; detail++ {
		l := build(maxW, 1, lim, detail)
		if maxH <= 0 || l.size.Y <= maxH {
			return l
		}
		if l.gapTotal > 0 {
			fixed := l.size.Y - l.gapTotal
			q := min(max(float64(maxH-fixed)/float64(l.gapTotal), minGapShare), 1)
			for {
				l = build(maxW, q, lim, detail)
				if l.size.Y <= maxH || q <= minGapShare {
					break
				}
				q = max(q-squeezeStep, minGapShare)
			}
		}
		if l.size.Y <= maxH || detail == detailBare {
			return l
		}
	}
}

const (
	detailFull = iota
	detailNoHint
	detailBare
)

func RangeText(lim Limits) string {
	return i18n.T("resize.range", lim.MaxSide, lim.MaxPixels/1_000_000)
}

func build(maxW int, gq float64, lim Limits, detail int) layout {
	m := newMetrics(fonts.TextScale())
	g := func(v int) int { return int(math.Round(float64(v) * gq)) }
	lineH, hintH := textHeight(bodyFont), textHeight(hintFont)
	l := layout{m: m}

	applyText := i18n.T("resize.apply")
	cancelText := i18n.T("dialog.cancel")
	applyW := max(m.btnMinW, bodyFont.Measure(nil, applyText)+2*m.btnPad)
	cancelW := max(m.btnMinW, bodyFont.Measure(nil, cancelText)+2*m.btnPad)
	buttonsW := applyW + m.btnGap + cancelW
	keepW := m.boxSize + m.boxGap + bodyFont.Measure(nil, i18n.T("resize.keep"))

	cw := max(m.s(320), buttonsW, keepW)
	w := cw + 2*m.padX
	if maxW > 0 && w > maxW {
		w = max(maxW, 2*m.padX+m.s(180))
		cw = w - 2*m.padX
	}
	if buttonsW > cw {
		each := (cw - m.btnGap) / 2
		applyW, cancelW = each, each
	}

	y := g(m.padTop)
	gapTotal := y
	step := func(v int) int {
		gapTotal += g(v)
		return g(v)
	}
	segH := max(m.btnMinH, lineH+m.s(12))
	half := cw / 2
	l.pixels = image.Rect(m.padX, y, m.padX+half, y+segH)
	l.percent = image.Rect(m.padX+half, y, m.padX+cw, y+segH)
	y += segH + step(m.s(14))

	colW := (cw - m.colGap) / 2
	capH := lineH + m.lineGap
	l.wLabel = image.Rect(m.padX, y, m.padX+colW, y+capH)
	l.hLabel = image.Rect(m.padX+cw-colW, y, m.padX+cw, y+capH)
	y += capH + step(m.s(4))
	l.width = image.Rect(m.padX, y, m.padX+colW, y+m.fieldH)
	l.height = image.Rect(m.padX+cw-colW, y, m.padX+cw, y+m.fieldH)
	y += m.fieldH + step(m.s(12))

	rowH := max(m.boxSize, lineH) + m.s(8)
	l.keep = image.Rect(m.padX, y, m.padX+min(cw, keepW+m.s(8)), y+rowH)
	y += rowH + step(m.s(10))

	infoH := lineH + m.lineGap
	if detail < detailBare {
		l.orig = image.Rect(m.padX, y, m.padX+cw, y+infoH)
		y += infoH
	}
	l.result = image.Rect(m.padX, y, m.padX+cw, y+infoH)
	y += infoH + step(m.s(4))

	if detail < detailNoHint {
		l.hintText = wrap(hintFont, RangeText(lim), cw, 2)
		hintStep := hintH + m.lineGap
		l.hint = image.Rect(m.padX, y, m.padX+cw, y+len(l.hintText)*hintStep)
		y += len(l.hintText) * hintStep
	}
	y += step(m.s(16))

	btnH := max(m.btnMinH, lineH+m.s(14))
	right := m.padX + cw
	l.cancel = image.Rect(right-cancelW, y, right, y+btnH)
	l.apply = image.Rect(l.cancel.Min.X-m.btnGap-applyW, y, l.cancel.Min.X-m.btnGap, y+btnH)
	y += btnH + step(m.padBottom)
	l.size = image.Pt(w, y)
	l.gapTotal = gapTotal
	l.squeeze = gq
	return l
}
