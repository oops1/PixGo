package gallery

import (
	"time"

	"github.com/oops1/PixGo/internal/fonts"
	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/ui/gfx"
)

const (
	sidebarMaxWidth    = 280
	sidebarMaxShare    = 45
	sidebarTextMargin  = 25
	openBaseWidth      = 91
	openLabelPadding   = 41
	buttonGrowthSpare  = 4
	listDateBaseWidth  = 138
	listSizeBaseWidth  = 90
	listColumnPadding  = 16
	listColumnGap      = 12
	listWideBase       = 420
	listRowGrowthSpare = 6
	statusGapBase      = 16
	baseNavBaseline    = 5
)

type fontGrowth struct {
	line, asc, mid int
}

func growthOf(f gfx.Font, k float64) fontGrowth {
	if k <= 1 {
		return fontGrowth{}
	}
	a := fonts.MetricsOf(f.Name, f.Pt, 96)
	b := fonts.MetricsOf(f.Name, f.Pt*k, 96)
	return fontGrowth{
		line: max((b.Ascent+b.Descent)-(a.Ascent+a.Descent), 0),
		asc:  max(b.Ascent-a.Ascent, 0),
		mid:  max(((b.Ascent-b.Descent)-(a.Ascent-a.Descent)+1)/2, 0),
	}
}

type navMetrics struct {
	extra   int
	text    int
	section int
}

type textMetrics struct {
	k          float64
	nav        navMetrics
	sideW      int
	labelExtra int
	labelAsc   int
	listRow    int
	listDateW  int
	listSizeW  int
	statusSx   int
	statusMid  int
	statusGap  int
	btnExtra   int
	btnMid     int
	openW      int
	addrMid    int
	bubbleH    int
}

func navMetricsFor(k float64) navMetrics {
	return navMetrics{
		extra:   growthOf(navFont, k).line,
		text:    baseNavBaseline + growthOf(navFont, k).mid,
		section: baseNavBaseline + growthOf(sectionFont, k).mid,
	}
}

func baseMetrics() textMetrics { return newTextMetrics(1) }

func newTextMetrics(k float64) textMetrics {
	btn := growthOf(buttonFont, k)
	label := growthOf(labelFont, k)
	list := growthOf(listFont, k)
	status := growthOf(statusFont, k)
	addr := growthOf(addressFont, k)
	m := textMetrics{
		k:          k,
		nav:        navMetricsFor(k),
		sideW:      sidebarWidth,
		labelExtra: label.line,
		labelAsc:   label.asc,
		listRow:    listRowHeight + max(list.line-listRowGrowthSpare, 0),
		listDateW:  listDateBaseWidth,
		listSizeW:  listSizeBaseWidth,
		statusMid:  status.mid,
		statusGap:  int(float64(statusGapBase)*max(k, 1) + .5),
		btnExtra:   max(btn.line-buttonGrowthSpare, 0),
		btnMid:     btn.mid,
		openW:      openBaseWidth,
		addrMid:    addr.mid,
		bubbleH:    bubbleBaseHeight + addr.line,
	}
	m.statusSx = m.btnExtra
	return m
}

type widthKey struct {
	name string
	size float64
	text string
}

func (p *Page) measure(f gfx.Font, s string) int {
	key := widthKey{f.Name, f.Size(), s}
	if w, ok := p.widths[key]; ok {
		return w
	}
	w := f.Measure(nil, s)
	if len(p.widths) > 4096 {
		clear(p.widths)
	}
	p.widths[key] = w
	return w
}

func (p *Page) fitMetrics(m textMetrics, winW int) textMetrics {
	need := sidebarWidth
	for _, r := range p.rows {
		f := navFont
		if r.entry.Header {
			f = sectionFont
		}
		need = max(need, r.textX()+p.measure(f, r.entry.Label)+sidebarTextMargin)
	}
	m.sideW = min(need, max(sidebarWidth, min(sidebarMaxWidth, winW*sidebarMaxShare/100)))
	m.openW = max(openBaseWidth, p.measure(buttonFont, i18n.T("cmd.open"))+openLabelPadding)
	date := time.Date(2000, 12, 28, 23, 59, 0, 0, time.UTC)
	m.listDateW = max(listDateBaseWidth, p.measure(listSubFont, i18n.FormatDateTime(date))+listColumnPadding)
	m.listSizeW = max(listSizeBaseWidth, p.measure(listSubFont, i18n.FormatBytes(1023<<20))+listColumnPadding)
	return m
}

func (m textMetrics) listColumns() int { return m.listDateW + m.listSizeW + listColumnGap }

func (m textMetrics) listWide() int {
	return listWideBase + m.listColumns() - (listDateBaseWidth + listSizeBaseWidth + listColumnGap)
}
