package infopanel

import (
	"image"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/i18n"
)

func (p *Panel) AccessInfo() widget.AccessInfo {
	return widget.AccessInfo{Role: widget.RoleGroup, Name: i18n.T("info.title"), Value: p.model.Name, Bounds: p.Bounds()}
}

func (p *Panel) AccessChildren() []widget.AccessInfo {
	b := p.Bounds()
	o := p.origin()
	pl := p.place()
	stop := 0
	focusedStates := func() []string {
		defer func() { stop++ }()
		if p.active && p.cur == stop {
			return []string{widget.StateFocused}
		}
		return nil
	}
	closeRect := image.Rect(o.X+closeBoxX-8, b.Min.Y+closeBoxY-8, o.X+closeBoxX-8+closeHit, b.Min.Y+closeBoxY-8+closeHit)
	out := []widget.AccessInfo{{Role: widget.RoleButton, Name: i18n.T("info.close"), Bounds: closeRect, States: focusedStates()}}
	thumb := image.Rect(o.X+thumbX, o.Y+thumbY, o.X+thumbX+thumbW, o.Y+thumbY+thumbH)
	if len(p.thumbSrc.Levels) > 0 {
		out = append(out, widget.AccessInfo{Role: widget.RoleImage, Name: i18n.T("access.thumbnail"), Value: p.model.Name, Bounds: thumb.Intersect(b)})
	}
	if p.model.Name != "" {
		text := image.Rect(o.X+textX, o.Y+thumbY, b.Max.X-cardPad, o.Y+pl.sizeBase+2).Intersect(b)
		value := p.model.Format
		if p.model.FileSize != "" {
			value += ", " + p.model.FileSize
		}
		out = append(out, widget.AccessInfo{Role: widget.RoleLabel, Name: p.model.Name, Value: value, Bounds: text})
	}
	for _, cp := range pl.cards {
		card := cp.rect.Add(o)
		if !card.Overlaps(b) {
			for _, rp := range cp.rows {
				if rp.row.Copy != "" {
					stop++
				}
				if rp.row.Link {
					stop++
				}
			}
			continue
		}
		if cp.card.Title != "" {
			title := image.Rect(card.Min.X+cardPad, card.Min.Y, card.Max.X-cardPad, card.Min.Y+cp.title+2)
			out = append(out, widget.AccessInfo{Role: widget.RoleLabel, Name: cp.card.Title, Bounds: title.Intersect(b)})
		}
		for _, rp := range cp.rows {
			out = append(out, p.accessRow(card, cp, rp, b, focusedStates)...)
		}
	}
	return out
}

func (p *Panel) accessRow(card image.Rectangle, cp cardPos, rp rowPos, view image.Rectangle, focused func() []string) []widget.AccessInfo {
	lines := max(len(rp.labelLines), len(rp.valueLines), 1)
	step := max(rp.labelStep, rp.valueStep, 1)
	lh := lineHeight(labelFont)
	base := card.Min.Y + rp.baseline
	rect := image.Rect(card.Min.X+cardPad, base-lh, card.Max.X-cardPad, base+(lines-1)*step+lh/4).Intersect(view)
	info := widget.AccessInfo{Role: widget.RoleLabel, Name: rp.row.Label, Value: rp.row.Value, Bounds: rect}
	if cp.card.Kind == CardNote {
		info.Value = ""
	}
	var out []widget.AccessInfo
	if rp.row.Copy != "" {
		out = append(out, widget.AccessInfo{Role: widget.RoleButton, Name: i18n.T("info.copyPath"), Bounds: rp.copyRect.Add(card.Min).Intersect(view), States: focused()})
	}
	if rp.row.Link {
		info.Role = widget.RoleButton
		info.States = focused()
	}
	return append([]widget.AccessInfo{info}, out...)
}
