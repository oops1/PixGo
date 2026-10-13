package infopanel

import (
	"image"
	"math"
	"strings"

	"github.com/oops1/PixGo/internal/fonts"
	"github.com/oops1/PixGo/internal/ui/gfx"
)

const (
	valueShare     = 0.55
	wordShare      = 0.6
	maxNameLines   = 2
	maxLabelLines  = 2
	maxValueLines  = 5
	maxPathLines   = 6
	maxNoteLines   = 3
	textSlack      = 2
	minColumn      = 24
	nameDrop       = 10
	formatDrop     = 21
	sizeDrop       = 19
	blockDrop      = 17
	thumbToCards   = 15
	emptyCardExtra = 8
	headerLift     = 4.9
	iconLift       = 3.7
	copyInset      = 27
	copyBoxLeft    = 4
	copyBoxRight   = 18
	copyBoxTop     = 15
	copyBoxBottom  = 7
	copyIconDX     = 2
	copyIconDY     = 11
	copyGap        = 8
	linkAbove      = 14
	linkBelow      = 6
)

type rowPos struct {
	row        Row
	baseline   int
	valueX     int
	labelLines []string
	valueLines []string
	valueFont  gfx.Font
	labelStep  int
	valueStep  int
	valueW     int
	iconLift   int
	copyRect   image.Rectangle
	linkRect   image.Rectangle
}

type cardPos struct {
	card   Card
	rect   image.Rectangle
	title  int
	innerW int
	rows   []rowPos
}

type placed struct {
	height     int
	headerBase int
	textW      int
	nameLines  []string
	nameBase   int
	nameStep   int
	formatBase int
	sizeBase   int
	cards      []cardPos
}

func scaled(v int, k float64) int { return int(math.Round(float64(v) * k)) }

func lineHeight(f gfx.Font) int {
	m := fonts.MetricsOf(f.Name, f.Size(), 96)
	return m.Ascent + m.Descent
}

func layoutModel(m Model, w int, k float64) placed {
	var out placed
	out.headerBase = headerBase + int(math.Round(headerLift*(k-1)))
	out.textW = max(w-textX-cardPad, minColumn)
	out.nameLines = wrapText(nameFont, m.Name, out.textW, maxNameLines, false)
	out.nameStep = lineHeight(nameFont)
	out.nameBase = thumbY + scaled(nameDrop, k)
	lastName := out.nameBase + max(len(out.nameLines)-1, 0)*out.nameStep
	out.formatBase = lastName + scaled(formatDrop, k)
	out.sizeBase = out.formatBase + scaled(sizeDrop, k)
	y := max(thumbY+thumbH+thumbToCards, out.sizeBase+scaled(blockDrop, k))
	cw := max(min(cardW, w-2*cardX), 2*cardPad+2*minColumn)
	for _, c := range m.Cards {
		cp := layoutCard(c, cw, k)
		cp.rect = image.Rect(cardX, y, cardX+cw, y+cp.rect.Dy())
		y += cp.rect.Dy() + cardGap
		out.cards = append(out.cards, cp)
	}
	out.height = y - cardGap + bottomMargin
	return out
}

func valueSpec(r Row) (gfx.Font, int, bool) {
	f, lines := valueFont, maxValueLines
	if r.Link {
		f, lines = pathFont, maxPathLines
	}
	if r.Copy != "" {
		lines = maxPathLines
	}
	return f, lines, r.Copy != ""
}

func layoutCard(c Card, cw int, k float64) cardPos {
	km := metricsByKind[c.Kind]
	inner := cw - 2*cardPad
	note := c.Kind == CardNote
	cp := cardPos{card: c, title: scaled(km.title, k), innerW: inner}
	labelH := lineHeight(labelFont)
	want, word := 0, 0
	if !note {
		for _, r := range c.Rows {
			for _, w := range strings.Fields(cleanText(r.Label)) {
				word = max(word, labelFont.Measure(nil, w))
			}
			f, _, _ := valueSpec(r)
			need := f.Measure(nil, cleanText(r.Value))
			if r.Copy != "" {
				need += copyInset + copyGap - cardPad
			}
			want = max(want, min(need, int(float64(inner)*valueShare)))
		}
	}
	labelCap := max(inner-valuePad-want, min(word, int(float64(inner)*wordShare)), minColumn)
	labels := make([][]string, len(c.Rows))
	widest := 0
	for i, r := range c.Rows {
		if note {
			labels[i] = wrapText(labelFont, r.Label, inner, maxNoteLines, false)
			continue
		}
		labels[i] = wrapText(labelFont, r.Label, labelCap, maxLabelLines, false)
		for _, l := range labels[i] {
			widest = max(widest, labelFont.Measure(nil, l))
		}
	}
	valueX := km.valueX
	if !note {
		valueX = max(min(km.valueX, cardPad+valuePad+labelCap), widest+cardPad+valuePad)
	}
	first := scaled(km.firstRow, k)
	lift := int(math.Round(iconLift * (k - 1)))
	acc := 0.0
	last := first
	for i, r := range c.Rows {
		base := first + int(acc+.5)
		rp := rowPos{row: r, baseline: base, valueX: valueX, labelLines: labels[i], valueFont: valueFont, labelStep: labelH, iconLift: lift}
		valueLines := 0
		if !note {
			vx := valueX - 2
			f, lines, tail := valueSpec(r)
			rp.valueFont = f
			rp.valueW = max(cw-cardPad-vx, minColumn)
			if r.Copy != "" {
				rp.valueW = max(cw-copyInset-copyGap-vx, minColumn)
				rp.copyRect = image.Rect(cw-copyInset-copyBoxLeft, base-copyBoxTop-lift, cw-copyInset+copyBoxRight, base+copyBoxBottom-lift)
			}
			rp.valueStep = lineHeight(f)
			rp.valueLines = wrapText(f, r.Value, rp.valueW, lines, tail)
			valueLines = max(len(rp.valueLines)-1, 0)
		}
		extra := max(max(len(rp.labelLines)-1, 0)*rp.labelStep, valueLines*rp.valueStep)
		last = base + extra
		if r.Link {
			right := cw - cardPad
			if r.Copy != "" {
				right = rp.copyRect.Min.X
			}
			rp.linkRect = image.Rect(valueX, base-scaled(linkAbove, k), right, base+valueLines*rp.valueStep+scaled(linkBelow, k))
		}
		acc += km.pitch*k + float64(extra)
		cp.rows = append(cp.rows, rp)
	}
	height := last + scaled(km.bottom, k)
	if len(c.Rows) == 0 {
		height = scaled(titleBase, k) + scaled(km.bottom, k) + scaled(emptyCardExtra, k)
	}
	cp.rect = image.Rect(0, 0, cw, height)
	return cp
}

func cleanText(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case '\n', '\r', '\t', '\v', '\f':
			return ' '
		}
		return r
	}, s)
}

func softBreak(r rune) bool {
	switch r {
	case ' ', '\\', '/', '_', '-':
		return true
	}
	return false
}

func wrapText(f gfx.Font, s string, width, maxLines int, keepTail bool) []string {
	rs := []rune(strings.TrimSpace(cleanText(s)))
	maxLines = max(maxLines, 1)
	width = max(width, 1)
	var lines []string
	for len(rs) > 0 {
		if len(lines) == maxLines-1 {
			lines = append(lines, lastLine(f, rs, width, keepTail))
			break
		}
		n := breakIndex(f, rs, width)
		lines = append(lines, strings.TrimRight(string(rs[:n]), " "))
		rs = rs[n:]
		for len(rs) > 0 && rs[0] == ' ' {
			rs = rs[1:]
		}
	}
	return lines
}

func lastLine(f gfx.Font, rs []rune, width int, keepTail bool) string {
	s := string(rs)
	if f.Measure(nil, s) <= width {
		return s
	}
	if !keepTail {
		return f.Fit(nil, s, width)
	}
	tail := func(n int) string { return "…" + strings.TrimLeft(string(rs[len(rs)-n:]), " ") }
	lo, hi := 0, len(rs)
	for lo < hi {
		m := (lo + hi + 1) / 2
		if f.Measure(nil, tail(m)) <= width {
			lo = m
		} else {
			hi = m - 1
		}
	}
	return tail(lo)
}

func breakIndex(f gfx.Font, rs []rune, width int) int {
	if f.Measure(nil, string(rs)) <= width {
		return len(rs)
	}
	fit := 1
	lo, hi := 1, len(rs)-1
	for lo <= hi {
		m := (lo + hi) / 2
		if f.Measure(nil, strings.TrimRight(string(rs[:m]), " ")) <= width {
			fit, lo = m, m+1
		} else {
			hi = m - 1
		}
	}
	for i := fit; i > 0 && i*3 > fit; i-- {
		if i < len(rs) && rs[i] == ' ' {
			return i
		}
		if softBreak(rs[i-1]) {
			return i
		}
	}
	return fit
}
