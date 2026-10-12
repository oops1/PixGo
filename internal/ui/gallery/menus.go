package gallery

import (
	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/thumbs"
)

func (p *Page) showMenu(items []widget.MenuItem, x, y int) {
	if f := p.deps.Actions.ShowMenu; f != nil && len(items) > 0 {
		f(items, x, y)
	}
}

func (p *Page) showFilterMenu(x, y int) {
	set := p.deps.Actions.SetFormat
	if set == nil {
		return
	}
	choose := func(format string) func() {
		return func() {
			set(format)
			p.resync()
			p.setScrollRaw(0)
			p.invalidate()
		}
	}
	current := p.snap.format
	items := []widget.MenuItem{{Text: i18n.T("gallery.filterAll"), Checkable: true, Checked: current == "", OnClick: choose("")}}
	for _, f := range p.src().Formats() {
		items = append(items, widget.MenuItem{Text: f, Checkable: true, Checked: f == current, OnClick: choose(f)})
	}
	p.showMenu(items, x, y)
}

func (p *Page) showSizeMenu(x, y int) {
	set := p.deps.Actions.SetThumbSize
	if set == nil {
		return
	}
	var items []widget.MenuItem
	for _, s := range thumbs.Sizes() {
		px := s.Px()
		items = append(items, widget.MenuItem{
			Text: i18n.T("gallery.thumbPx", px), Checkable: true, Checked: px == p.snap.side,
			OnClick: func() {
				set(px)
				p.resync()
				p.invalidate()
			},
		})
	}
	p.showMenu(items, x, y)
}

func (p *Page) showMoreMenu(i int) {
	g := p.geo
	p.showMenu(p.openItems(i), g.open.Min.X, g.open.Min.Y)
}

func (p *Page) openItems(i int) []widget.MenuItem {
	a := p.deps.Actions
	call := func(f func(int)) func() {
		return func() {
			if f != nil {
				f(i)
			}
		}
	}
	dir := i < len(p.snap.items) && p.snap.items[i].Dir
	return []widget.MenuItem{
		{Text: i18n.T("cmd.open"), Shortcut: "Enter", OnClick: call(a.Open)},
		{Text: i18n.T("cmd.openNewWindow"), Disabled: dir, OnClick: call(a.OpenNewWindow)},
		{Text: i18n.T("cmd.reveal"), OnClick: call(a.Reveal)},
	}
}

func (p *Page) showItemMenu(i, x, y int) {
	if i < 0 || i >= len(p.snap.items) {
		return
	}
	a := p.deps.Actions
	item := p.snap.items[i]
	items := p.openItems(i)
	if item.Dir {
		items = items[:1]
		p.showMenu(items, x, y)
		return
	}
	favorite := i18n.T("cmd.favorite.add")
	if p.src().Favorite(item.Path) {
		favorite = i18n.T("cmd.favorite.remove")
	}
	items = append(items,
		widget.MenuItem{Separator: true},
		widget.MenuItem{Text: favorite, Shortcut: "Ctrl+D", OnClick: func() {
			if a.ToggleFavorite != nil {
				a.ToggleFavorite(i)
				p.resync()
				p.invalidate()
			}
		}},
		widget.MenuItem{Separator: true},
		widget.MenuItem{Text: i18n.T("cmd.delete"), Shortcut: "Del", OnClick: func() {
			if a.Delete != nil {
				a.Delete(i)
				p.resync()
				p.invalidate()
			}
		}},
	)
	p.showMenu(items, x, y)
}
