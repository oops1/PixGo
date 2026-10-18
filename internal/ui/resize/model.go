package resize

import (
	"image"
	"math"
	"strconv"
)

type Unit uint8

const (
	UnitPixels Unit = iota
	UnitPercent
)

type Status uint8

const (
	StatusOK Status = iota
	StatusEmpty
	StatusTooBig
)

const (
	maxDigits  = 6
	minPercent = 1
)

type Limits struct {
	MaxSide   int
	MaxPixels int64
}

func (l Limits) allows(w, h int) bool {
	if w < 1 || h < 1 {
		return false
	}
	if l.MaxSide > 0 && (w > l.MaxSide || h > l.MaxSide) {
		return false
	}
	return l.MaxPixels <= 0 || int64(w)*int64(h) <= l.MaxPixels
}

type Model struct {
	orig   image.Point
	limits Limits
	unit   Unit
	keep   bool
	wText  string
	hText  string
}

func NewModel(orig, initial image.Point, limits Limits) *Model {
	m := &Model{orig: orig, limits: limits, keep: true}
	if initial.X < 1 || initial.Y < 1 {
		initial = orig
	}
	m.wText, m.hText = strconv.Itoa(initial.X), strconv.Itoa(initial.Y)
	return m
}

func (m *Model) Orig() image.Point { return m.orig }

func (m *Model) Limits() Limits { return m.limits }

func (m *Model) Unit() Unit { return m.unit }

func (m *Model) Keep() bool { return m.keep }

func (m *Model) WidthText() string { return m.wText }

func (m *Model) HeightText() string { return m.hText }

func parse(s string) (int, bool) {
	if s == "" || len(s) > maxDigits {
		return 0, false
	}
	n, err := strconv.Atoi(s)
	return n, err == nil && n > 0
}

func (m *Model) Result() (image.Point, Status) {
	w, okW := parse(m.wText)
	h, okH := parse(m.hText)
	if !okW || !okH {
		return image.Point{}, StatusEmpty
	}
	if m.unit == UnitPercent {
		w = scale(m.orig.X, w)
		h = scale(m.orig.Y, h)
	}
	if !m.limits.allows(w, h) {
		return image.Pt(w, h), StatusTooBig
	}
	return image.Pt(w, h), StatusOK
}

func scale(v, percent int) int {
	return max(1, int(math.Round(float64(v)*float64(percent)/100)))
}

func (m *Model) otherFor(text string, fromWidth bool) string {
	n, ok := parse(text)
	if !ok {
		return ""
	}
	if m.unit == UnitPercent {
		return text
	}
	var v int
	if fromWidth {
		v = int(math.Round(float64(n) * float64(m.orig.Y) / float64(m.orig.X)))
	} else {
		v = int(math.Round(float64(n) * float64(m.orig.X) / float64(m.orig.Y)))
	}
	return strconv.Itoa(max(1, v))
}

func clean(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s) && len(out) < maxDigits; i++ {
		if s[i] >= '0' && s[i] <= '9' {
			out = append(out, s[i])
		}
	}
	return string(out)
}

func (m *Model) SetWidth(s string) {
	m.wText = clean(s)
	if m.keep && m.wText != "" {
		m.hText = m.otherFor(m.wText, true)
	}
}

func (m *Model) SetHeight(s string) {
	m.hText = clean(s)
	if m.keep && m.hText != "" {
		m.wText = m.otherFor(m.hText, false)
	}
}

func (m *Model) SetKeep(on bool) {
	if m.keep == on {
		return
	}
	m.keep = on
	if on && m.wText != "" {
		m.hText = m.otherFor(m.wText, true)
	}
}

func (m *Model) SetUnit(u Unit) {
	if u == m.unit {
		return
	}
	cur, st := m.Result()
	if st == StatusEmpty {
		cur = m.orig
	}
	m.unit = u
	if u == UnitPercent {
		pw := max(minPercent, int(math.Round(float64(cur.X)*100/float64(m.orig.X))))
		ph := max(minPercent, int(math.Round(float64(cur.Y)*100/float64(m.orig.Y))))
		if m.keep {
			ph = pw
		}
		m.wText, m.hText = strconv.Itoa(pw), strconv.Itoa(ph)
		return
	}
	m.wText, m.hText = strconv.Itoa(cur.X), strconv.Itoa(cur.Y)
}
