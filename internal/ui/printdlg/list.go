package printdlg

import (
	"image"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/print"
)

const listPad = 4

func PaperLabel(p print.Paper) string {
	if p.Photo {
		return i18n.T("print.paper.photo", int(p.ShortMM/10+.5), int(p.LongMM/10+.5))
	}
	return p.Name
}

func OrientationLabel(o print.Orientation) string {
	return i18n.T("print.orient." + o.ID())
}

func PlacementLabel(p print.Placement) string {
	return i18n.T("print.place." + p.ID())
}

func MarginLabel(g print.Margin) string {
	return i18n.T("print.margin." + g.ID())
}

func (f *form) itemsOf(p Part) ([]string, int) {
	o := f.model.Options()
	switch p {
	case PartPrinter:
		return f.model.Destinations(), f.model.DestIndex()
	case PartPaper:
		items := make([]string, len(print.Papers))
		sel := 0
		for i, pp := range print.Papers {
			items[i] = PaperLabel(pp)
			if pp.ID == o.Paper {
				sel = i
			}
		}
		return items, sel
	case PartOrientation:
		items := make([]string, len(print.Orientations))
		for i, v := range print.Orientations {
			items[i] = OrientationLabel(v)
		}
		return items, int(o.Orientation)
	case PartPlacement:
		items := make([]string, len(print.Placements))
		for i, v := range print.Placements {
			items[i] = PlacementLabel(v)
		}
		return items, int(o.Placement)
	case PartMargins:
		items := make([]string, len(print.Margins))
		for i, v := range print.Margins {
			items[i] = MarginLabel(v)
		}
		return items, int(o.Margin)
	}
	return nil, 0
}

func (f *form) valueText(p Part) string {
	items, sel := f.itemsOf(p)
	if sel < 0 || sel >= len(items) {
		return ""
	}
	return items[sel]
}

func (f *form) choose(p Part, i int) {
	items, sel := f.itemsOf(p)
	if i < 0 || i >= len(items) || i == sel {
		return
	}
	switch p {
	case PartPrinter:
		f.model.SetDestIndex(i)
		f.clampCarets()
	case PartPaper:
		f.model.SetPaper(print.Papers[i].ID)
	case PartOrientation:
		f.model.SetOrientation(print.Orientations[i])
	case PartPlacement:
		f.model.SetPlacement(print.Placements[i])
		f.resetFields()
	case PartMargins:
		f.model.SetMargin(print.Margins[i])
	}
	f.Invalidate()
}

func (f *form) openList(p Part) {
	if !f.enabled(p) {
		return
	}
	items, sel := f.itemsOf(p)
	if len(items) == 0 {
		return
	}
	f.reveal(p)
	m := f.lay.m
	field := f.rect(p)
	b := f.Bounds()
	rows := min(len(items), listRows)
	longest := 0
	for _, it := range items {
		longest = max(longest, bodyFont.Measure(nil, it))
	}
	w := max(field.Dx(), longest+2*m.s(12)+2*listPad)
	w = min(w, b.Dx()-8)
	x := min(field.Min.X, b.Max.X-4-w)
	x = max(x, b.Min.X+4)
	below := b.Max.Y - 4 - (field.Max.Y + 2) - 2*listPad
	above := field.Min.Y - 2 - (b.Min.Y + 4) - 2*listPad
	fitRows := func(room int) int { return max(1, room/m.itemH) }
	var y, h int
	switch {
	case rows*m.itemH <= below || below >= above:
		rows = min(rows, fitRows(below))
		h = rows*m.itemH + 2*listPad
		y = field.Max.Y + 2
	default:
		rows = min(rows, fitRows(above))
		h = rows*m.itemH + 2*listPad
		y = field.Min.Y - 2 - h
	}
	ls := &listState{part: p, items: items, sel: sel, hover: -1, rows: rows, rect: image.Rect(x, y, x+w, y+h)}
	ls.top = min(max(sel-rows/2, 0), len(items)-rows)
	f.list = ls
	f.stop = p
	f.Invalidate()
}

func (f *form) closeList() bool {
	if f.list == nil {
		return false
	}
	f.list = nil
	f.Invalidate()
	return true
}

func (f *form) toggleList(p Part) {
	if f.list != nil && f.list.part == p {
		f.closeList()
		return
	}
	f.openList(p)
}

func (f *form) pickList(i int) {
	if f.list == nil {
		return
	}
	p := f.list.part
	f.list = nil
	f.choose(p, i)
	f.Invalidate()
}

func (f *form) listIndexAt(x, y int) int {
	ls := f.list
	if ls == nil || !image.Pt(x, y).In(ls.rect) {
		return -1
	}
	row := (y - ls.rect.Min.Y - listPad) / f.lay.m.itemH
	if row < 0 || row >= ls.rows {
		return -1
	}
	return ls.top + row
}

func (f *form) listScroll(d int) {
	ls := f.list
	if ls == nil {
		return
	}
	ls.top = min(max(ls.top+d, 0), len(ls.items)-ls.rows)
	f.Invalidate()
}

func (f *form) listMove(to int) {
	ls := f.list
	to = min(max(to, 0), len(ls.items)-1)
	ls.sel = to
	if to < ls.top {
		ls.top = to
	}
	if to >= ls.top+ls.rows {
		ls.top = to - ls.rows + 1
	}
}

func (f *form) listKey(e widget.KeyEvent) {
	ls := f.list
	switch e.Code {
	case widget.KeyDown:
		f.listMove(ls.sel + 1)
	case widget.KeyUp:
		f.listMove(ls.sel - 1)
	case widget.KeyHome:
		f.listMove(0)
	case widget.KeyEnd:
		f.listMove(len(ls.items) - 1)
	case widget.KeyPageDown:
		f.listMove(ls.sel + ls.rows)
	case widget.KeyPageUp:
		f.listMove(ls.sel - ls.rows)
	case widget.KeyEnter, widget.KeySpace:
		f.pickList(ls.sel)
	case widget.KeyTab:
		f.closeList()
		if e.Mod&widget.ModShift != 0 {
			f.moveStop(-1)
		} else {
			f.moveStop(1)
		}
	case widget.KeyF4:
		f.closeList()
	}
}
