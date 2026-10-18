package gallery

import (
	"image"
	"strings"

	"github.com/oops1/headless-gui/v3/widget"

	model "github.com/oops1/PixGo/internal/gallery"
	"github.com/oops1/PixGo/internal/i18n"
)

func (p *Page) AccessInfo() widget.AccessInfo {
	info := widget.AccessInfo{Role: widget.RoleGroup, Name: i18n.T("gallery.title"), Bounds: p.Bounds()}
	if p.src() != nil {
		p.ensureLayout()
		info.Value = p.snap.loc.Display()
	}
	return info
}

func (a *addressField) AccessInfo() widget.AccessInfo {
	return widget.AccessInfo{Role: widget.RoleTextInput, Name: i18n.T("gallery.address"), Value: a.GetText(), Bounds: a.Bounds()}
}

func (p *Page) control(id string, role widget.AccessRole, name string, bounds image.Rectangle, s stop, states ...string) widget.AccessInfo {
	if p.focus == s {
		states = append(states, widget.StateFocused)
	}
	return widget.AccessInfo{ID: id, Role: role, Name: name, Bounds: bounds, States: states}
}

func disabledUnless(on bool) []string {
	if on {
		return nil
	}
	return []string{widget.StateDisabled}
}

func (p *Page) AccessChildren() []widget.AccessInfo {
	if p.src() == nil {
		return nil
	}
	p.ensureLayout()
	out := p.accessHeader()
	out = append(out, p.accessPlaces()...)
	out = append(out, p.accessItems()...)
	out = append(out, p.accessFooter()...)
	return out
}

func (p *Page) accessHeader() []widget.AccessInfo {
	g, s := p.geo, p.snap
	out := []widget.AccessInfo{
		p.control(idBack, widget.RoleButton, i18n.T("cmd.historyBack"), g.back, stopBack, disabledUnless(s.canBack)...),
		p.control(idForward, widget.RoleButton, i18n.T("cmd.historyForward"), g.forward, stopForward, disabledUnless(s.canForward)...),
		p.control(idUp, widget.RoleButton, i18n.T("cmd.historyUp"), g.up, stopUp, disabledUnless(s.canUp)...),
	}
	if !p.editing {
		address := p.control(idAddress, widget.RoleTextInput, i18n.T("gallery.address"), g.address, stopAddress, widget.StateReadOnly)
		address.Value = s.loc.Display()
		out = append(out, address)
	}
	filter := p.control(idFilter, widget.RoleButton, i18n.T("gallery.filter"), g.filter, stopFilter)
	filter.Value = i18n.T("gallery.filterAll")
	if s.format != "" {
		filter.Value = s.format
	}
	refresh := i18n.T("access.refresh")
	if p.editing {
		refresh = i18n.T("access.go")
	}
	out = append(out, filter, p.control(idRefresh, widget.RoleButton, refresh, g.refresh, stopRefresh))
	list := widget.AccessInfo{ID: idViewList, Role: widget.RoleRadioButton, Name: i18n.T("gallery.viewList"), Bounds: g.viewList, States: p.viewStates(s.mode == ViewList)}
	grid := widget.AccessInfo{ID: idViewGrid, Role: widget.RoleRadioButton, Name: i18n.T("gallery.viewGrid"), Bounds: g.viewGrid, States: p.viewStates(s.mode == ViewGrid)}
	thumbs := p.control(idThumbs, widget.RoleButton, i18n.T("gallery.thumbSize"), g.thumbs, stopThumbs)
	thumbs.Value = i18n.T("gallery.thumbPx", s.side)
	return append(out, list, grid, thumbs)
}

func (p *Page) viewStates(selected bool) []string {
	if !selected {
		return nil
	}
	states := []string{widget.StateSelected}
	if p.focus == stopViews {
		states = append(states, widget.StateFocused)
	}
	return states
}

func (p *Page) accessPlaces() []widget.AccessInfo {
	g := p.geo
	out := []widget.AccessInfo{{Role: widget.RoleList, Name: i18n.T("access.places"), Bounds: g.sidebar}}
	top, right := g.sidebar.Min.Y-p.sideY, g.sidebar.Max.X
	cursor := p.navCurrent()
	for _, r := range p.rows {
		rect := r.rect(top, right).Intersect(g.sidebar)
		if rect.Empty() {
			continue
		}
		e := r.entry
		if e.Header {
			out = append(out, widget.AccessInfo{Role: widget.RoleLabel, Name: e.Label, Bounds: rect})
			continue
		}
		info := widget.AccessInfo{ID: placeID(r.index), Role: widget.RoleRadioButton, Name: e.Label, Bounds: rect}
		if e.Selected {
			info.States = append(info.States, widget.StateSelected)
		}
		if e.Disabled {
			info.States = append(info.States, widget.StateDisabled)
		}
		if p.focus == stopNav && r.index == cursor {
			info.States = append(info.States, widget.StateFocused)
		}
		out = append(out, info)
	}
	return out
}

func (p *Page) accessItems() []widget.AccessInfo {
	g, s := p.geo, p.snap
	list := widget.AccessInfo{Role: widget.RoleList, Name: i18n.T("access.items"), Bounds: g.grid}
	list.Value = i18n.T("gallery.items", i18n.FormatInt(int64(len(s.items))))
	out := []widget.AccessInfo{list}
	if len(s.items) == 0 {
		text := model.EmptyText(s.loc, s.failed)
		if s.loading {
			text = i18n.T("gallery.loading")
		}
		return append(out, widget.AccessInfo{Role: widget.RoleLabel, Name: text, Bounds: g.grid})
	}
	first, last := p.grid.Visible(p.scroll)
	sel := p.Selected()
	for i := first; i < last && i < len(s.items); i++ {
		rect := p.tileRect(i).Intersect(g.grid)
		if rect.Empty() {
			continue
		}
		item := s.items[i]
		info := widget.AccessInfo{ID: itemID(i), Role: widget.RoleRadioButton, Name: item.Name, Bounds: rect}
		if item.Dir {
			info.Value = i18n.T("gallery.folder")
		} else {
			info.Value = i18n.FormatBytes(item.Size)
		}
		if i == sel {
			info.States = append(info.States, widget.StateSelected)
			if p.focus == stopGrid {
				info.States = append(info.States, widget.StateFocused)
			}
		}
		out = append(out, info)
	}
	return out
}

func (p *Page) accessFooter() []widget.AccessInfo {
	g, s := p.geo, p.snap
	selected := p.Selected() >= 0
	var out []widget.AccessInfo
	status := strings.Join(s.status, ", ")
	if s.truncated != "" {
		status += ". " + s.truncated
	}
	if status != "" {
		out = append(out, widget.AccessInfo{Role: widget.RoleLabel, Name: status, Description: i18n.T("access.statusBar"), Bounds: g.status})
	}
	out = append(out,
		p.control(idOpen, widget.RoleButton, i18n.T("cmd.open"), g.open, stopOpen, disabledUnless(selected)...),
		p.control(idMore, widget.RoleButton, i18n.T("gallery.openMenu"), g.more, stopMore, disabledUnless(selected)...),
	)
	if p.bubble != "" {
		out = append(out, widget.AccessInfo{Role: widget.RoleLabel, Name: p.bubble, Bounds: p.bubbleArea()})
	}
	return out
}
