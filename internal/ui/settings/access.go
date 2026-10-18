package settings

import (
	"image"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/i18n"
)

func (p *Page) AccessInfo() widget.AccessInfo {
	return widget.AccessInfo{Role: widget.RoleGroup, Name: i18n.T("settings.title"), Bounds: p.Bounds()}
}

func (p *Page) AccessChildren() []widget.AccessInfo {
	if p.deps.Source == nil {
		return nil
	}
	p.ensureLayout()
	g := p.geo
	back := widget.AccessInfo{ID: idBack, Role: widget.RoleButton, Name: i18n.T("cmd.back"), Bounds: g.back}
	if p.focus == focusBack {
		back.States = []string{widget.StateFocused}
	}
	out := []widget.AccessInfo{back}
	for s := Section(0); s < sectionCount; s++ {
		info := widget.AccessInfo{ID: navID(s), Role: widget.RoleRadioButton, Name: i18n.T(sectionKeys[s]), Bounds: navRowRect(s)}
		if s == p.sec {
			info.States = []string{widget.StateSelected}
			if p.focus == focusNav {
				info.States = append(info.States, widget.StateFocused)
			}
		}
		out = append(out, info)
	}
	off := image.Pt(0, -p.scroll)
	for i := range p.items {
		if info, ok := p.itemAccess(&p.items[i], off); ok {
			out = append(out, info)
		}
	}
	return out
}

func (p *Page) itemAccess(it *item, off image.Point) (widget.AccessInfo, bool) {
	info := widget.AccessInfo{ID: itemID(it), Bounds: it.r.Add(off).Intersect(p.geo.view)}
	var states []string
	if it.on {
		states = append(states, widget.StateChecked)
	}
	if it.disabled {
		states = append(states, widget.StateDisabled)
	}
	if p.focus == it.id && it.id != "" {
		states = append(states, widget.StateFocused)
	}
	info.States = states
	switch it.kind {
	case kindTitle, kindHeading:
		info.Role, info.Name = widget.RoleLabel, it.text
	case kindNote, kindValue:
		info.Role, info.Name = widget.RoleLabel, joinLines(it.lines)
	case kindCheck:
		info.Role, info.Name = widget.RoleCheckBox, it.text
	case kindFormat:
		info.Role, info.Name, info.Value = widget.RoleLabel, it.name, it.detail
		if it.change != nil {
			info.Role = widget.RoleCheckBox
		}
	case kindRadio, kindCard:
		info.Role, info.Name = widget.RoleRadioButton, it.text
		if it.on {
			info.States = append(info.States, widget.StateSelected)
		}
	case kindDropdown:
		info.Role, info.Name, info.Value = widget.RoleComboBox, it.name, it.text
	case kindSlider:
		info.Role, info.Name, info.Value = widget.RoleSlider, it.name, it.valueText
	case kindButton, kindLink:
		info.Role, info.Name = widget.RoleButton, it.text
	default:
		return info, false
	}
	return info, true
}

func joinLines(lines []string) string {
	out := ""
	for i, l := range lines {
		if i > 0 {
			out += " "
		}
		out += l
	}
	return out
}
