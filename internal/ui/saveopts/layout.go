package saveopts

import (
	"image"
	"math"
	"strings"

	"github.com/oops1/PixGo/internal/fonts"
	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/ui/gfx"
)

const (
	MinQuality = 1
	MaxQuality = 100

	hintMaxLines = 5
	minGapShare  = 0.25
	squeezeStep  = 0.02
)

var (
	bodyFont = gfx.UI(7.5)
	hintFont = gfx.UI(6.7)
)

type Kind uint8

const (
	JPEG Kind = iota + 1
	WebP
	JPEGLossless
)

const GPS Kind = 1 << 7

func (k Kind) base() Kind { return k &^ GPS }

func (k Kind) hasGPS() bool { return k&GPS != 0 }

func (k Kind) lossless() bool { return k.base() == WebP || k.base() == JPEGLossless }

func (k Kind) hintKey() string {
	switch k.base() {
	case WebP:
		return "save.options.webpHint"
	case JPEGLossless:
		return "save.options.jpegLosslessHint"
	}
	return "save.options.jpegHint"
}

type metrics struct {
	k, ks     float64
	padX      int
	padTop    int
	padBottom int
	boxSize   int
	boxGap    int
	thumbR    int
	trackH    int
	btnGap    int
	btnPad    int
	btnMinW   int
	btnMinH   int
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
		thumbR: ss(8), trackH: max(ss(4), 2),
		btnGap: s(8), btnPad: s(18), btnMinW: s(88), btnMinH: s(30),
		lineGap: s(2),
	}
}

func (m metrics) s(v float64) int { return int(math.Round(v * m.k)) }

type layout struct {
	size    image.Point
	check   image.Rectangle
	gps     image.Rectangle
	caption image.Rectangle
	value   image.Rectangle
	slider  image.Rectangle
	hint    image.Rectangle
	save    image.Rectangle
	cancel  image.Rectangle

	m        metrics
	hintText []string
	hasCheck bool
	hasGPS   bool
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
	flush := func() {
		if cur != "" {
			lines = append(lines, cur)
			cur = ""
		}
	}
	for _, word := range i18n.Words(text) {
		for f.Measure(nil, word) > width && len([]rune(word)) > 1 {
			head := []rune(word)
			cut := len(head) - 1
			for cut > 1 && f.Measure(nil, string(head[:cut])) > width {
				cut--
			}
			flush()
			lines = append(lines, string(head[:cut]))
			word = string(head[cut:])
		}
		switch {
		case cur == "":
			cur = word
		case f.Measure(nil, cur+" "+word) <= width:
			cur += " " + word
		default:
			flush()
			cur = word
		}
	}
	flush()
	if len(lines) > maxLines {
		lines = append(lines[:maxLines-1], f.Fit(nil, strings.Join(lines[maxLines-1:], " "), width))
	}
	return lines
}

func compute(kind Kind, maxW, maxH int) layout {
	l := buildLines(kind, maxW, 1, hintMaxLines)
	if maxH <= 0 || l.size.Y <= maxH || l.gapTotal <= 0 {
		return l
	}
	fixed := l.size.Y - l.gapTotal
	q := min(max(float64(maxH-fixed)/float64(l.gapTotal), minGapShare), 1)
	for {
		l = buildLines(kind, maxW, q, hintMaxLines)
		if l.size.Y <= maxH || q <= minGapShare {
			break
		}
		q = max(q-squeezeStep, minGapShare)
	}
	for lines := hintMaxLines - 1; l.size.Y > maxH && lines >= 1 && kind.hasGPS(); lines-- {
		l = buildLines(kind, maxW, minGapShare, lines)
	}
	return l
}

func build(kind Kind, maxW int, gq float64) layout { return buildLines(kind, maxW, gq, hintMaxLines) }

func buildLines(kind Kind, maxW int, gq float64, hintLines int) layout {
	m := newMetrics(fonts.TextScale())
	g := func(v int) int { return int(math.Round(float64(v) * gq)) }
	lineH, hintH := textHeight(bodyFont), textHeight(hintFont)
	l := layout{m: m, hasCheck: kind.lossless(), hasGPS: kind.hasGPS()}

	checkText := i18n.T("save.lossless")
	gpsText := i18n.T("save.stripGPS")
	qualityText := i18n.T("save.quality")
	saveText := i18n.T("save.options.save")
	cancelText := i18n.T("dialog.cancel")

	checkW := m.boxSize + m.boxGap + bodyFont.Measure(nil, checkText)
	valueW := bodyFont.Measure(nil, "100")
	captionW := bodyFont.Measure(nil, qualityText) + m.s(16) + valueW
	saveW := max(m.btnMinW, bodyFont.Measure(nil, saveText)+2*m.btnPad)
	cancelW := max(m.btnMinW, bodyFont.Measure(nil, cancelText)+2*m.btnPad)
	buttonsW := saveW + m.btnGap + cancelW

	gpsW := 0
	if l.hasGPS {
		gpsW = m.boxSize + m.boxGap + bodyFont.Measure(nil, gpsText)
	}
	cw := max(m.s(300), captionW, buttonsW, m.s(220), gpsW)
	if l.hasCheck {
		cw = max(cw, checkW)
	}
	w := cw + 2*m.padX
	if maxW > 0 && w > maxW {
		w = max(maxW, 2*m.padX+m.s(160))
		cw = w - 2*m.padX
	}
	if buttonsW > cw {
		each := (cw - m.btnGap) / 2
		saveW, cancelW = each, each
	}

	y := g(m.padTop)
	gapTotal := y
	step := func(v int) int {
		gapTotal += g(v)
		return g(v)
	}
	if l.hasCheck {
		rowH := max(m.boxSize, lineH) + m.s(8)
		l.check = image.Rect(m.padX, y, m.padX+min(cw, checkW+m.s(8)), y+rowH)
		y += rowH + step(m.s(8))
	}
	if l.hasGPS {
		gpsRow := max(m.boxSize, lineH) + m.s(8)
		l.gps = image.Rect(m.padX, y, m.padX+min(cw, gpsW+m.s(8)), y+gpsRow)
		y += gpsRow + step(m.s(8))
	}
	capH := lineH + m.lineGap
	l.caption = image.Rect(m.padX, y, m.padX+cw, y+capH)
	l.value = image.Rect(m.padX+cw-valueW, y, m.padX+cw, y+capH)
	y += capH + step(m.s(2))
	sliderH := 2*m.thumbR + m.s(8)
	l.slider = image.Rect(m.padX, y, m.padX+cw, y+sliderH)
	y += sliderH + step(m.s(6))
	l.hintText = wrap(hintFont, i18n.T(kind.hintKey()), cw, hintLines)
	hintStep := hintH + m.lineGap
	l.hint = image.Rect(m.padX, y, m.padX+cw, y+len(l.hintText)*hintStep)
	y += len(l.hintText)*hintStep + step(m.s(18))
	btnH := max(m.btnMinH, lineH+m.s(14))
	right := m.padX + cw
	l.cancel = image.Rect(right-cancelW, y, right, y+btnH)
	l.save = image.Rect(l.cancel.Min.X-m.btnGap-saveW, y, l.cancel.Min.X-m.btnGap, y+btnH)
	y += btnH + step(m.padBottom)
	l.size = image.Pt(w, y)
	l.gapTotal = gapTotal
	l.squeeze = gq
	return l
}
