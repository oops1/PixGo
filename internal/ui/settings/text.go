package settings

import (
	"image"
	"image/color"
	"strings"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/ui/gfx"
)

func measure(f gfx.Font, s string) int { return f.Measure(nil, s) }

func wrapLines(f gfx.Font, s string, width, maxLines int) []string {
	words := i18n.Words(s)
	if len(words) == 0 {
		return []string{""}
	}
	maxLines = max(maxLines, 1)
	var lines []string
	start := 0
	for start < len(words) {
		if len(lines) == maxLines-1 {
			lines = append(lines, fitText(f, strings.Join(words[start:], " "), width))
			break
		}
		end := start + 1
		for end < len(words) && measure(f, strings.Join(words[start:end+1], " ")) <= width {
			end++
		}
		lines = append(lines, fitText(f, strings.Join(words[start:end], " "), width))
		start = end
	}
	return lines
}

func fitText(f gfx.Font, s string, width int) string {
	if measure(f, s) <= width {
		return s
	}
	r := []rune(s)
	lo, hi := 0, len(r)
	for lo < hi {
		m := (lo + hi + 1) / 2
		if measure(f, string(r[:m])+"…") <= width {
			lo = m
		} else {
			hi = m - 1
		}
	}
	return strings.TrimRight(string(r[:lo]), " ") + "…"
}

var textWeights = map[gfx.Font]float64{
	bodyFont:     .5,
	navFont:      .5,
	headerFont:   .65,
	captionFont:  .3,
	smallCapFont: .15,
	noteFont:     .5,
}

func drawText(c widget.DrawContext, f gfx.Font, s string, x, baseline int, col color.RGBA) {
	f.Draw(c, s, x, baseline, col)
	if w := textWeights[f]; w > 0 && s != "" {
		f.Draw(c, s, x, baseline, gfx.Alpha(col, w))
	}
}

func drawFit(c widget.DrawContext, f gfx.Font, s string, x, baseline, width int, col color.RGBA) {
	s = f.Fit(c, s, width)
	restore := gfx.Clip(c, image.Rect(x, baseline-f.Ascent()-4, x+width, baseline+f.Ascent()))
	drawText(c, f, s, x, baseline, col)
	restore()
}
