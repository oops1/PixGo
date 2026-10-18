package printdlg

import (
	"image"
	"strconv"
	"strings"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/print"
)

var partIDs = map[Part]string{
	PartPrinter:     "printer",
	PartSystem:      "system",
	PartCopies:      "copies",
	PartCopiesDec:   "copies.dec",
	PartCopiesInc:   "copies.inc",
	PartPaper:       "paper",
	PartOrientation: "orientation",
	PartPlacement:   "placement",
	PartWidth:       "width",
	PartHeight:      "height",
	PartCm:          "unit.cm",
	PartInch:        "unit.in",
	PartKeep:        "keep",
	PartMargins:     "margins",
	PartPrint:       "print",
	PartCancel:      "cancel",
}

const itemIDPrefix = "item."

func partOfID(id string) Part {
	for p, s := range partIDs {
		if s == id {
			return p
		}
	}
	return PartNone
}

func (f *form) title() string {
	if f.lay.export {
		return i18n.T("print.titleExport")
	}
	return i18n.T("print.title")
}

func (f *form) AccessInfo() widget.AccessInfo {
	return widget.AccessInfo{Role: widget.RoleDialog, Name: f.title(), Bounds: f.Bounds()}
}

func (f *form) states(p Part, checked bool) []string {
	var out []string
	if checked {
		out = append(out, widget.StateChecked)
	}
	if !f.enabled(p) {
		out = append(out, widget.StateDisabled)
	}
	if f.focused && f.stop == p {
		out = append(out, widget.StateFocused)
	}
	if isDropdown(p) {
		out = append(out, widget.StateHasPopup)
		if f.list != nil && f.list.part == p {
			out = append(out, widget.StateExpanded)
		}
	}
	return out
}

func (f *form) partInfo(p Part) (widget.AccessInfo, bool) {
	if !f.shown(p) {
		return widget.AccessInfo{}, false
	}
	o := f.model.Options()
	info := widget.AccessInfo{ID: partIDs[p], Bounds: f.rect(p), States: f.states(p, false)}
	switch p {
	case PartPrinter:
		info.Role, info.Name, info.Value = widget.RoleComboBox, i18n.T("print.printer"), f.valueText(p)
	case PartPaper:
		info.Role, info.Name, info.Value = widget.RoleComboBox, i18n.T("print.paper"), f.valueText(p)
	case PartOrientation:
		info.Role, info.Name, info.Value = widget.RoleComboBox, i18n.T("print.orientation"), f.valueText(p)
	case PartPlacement:
		info.Role, info.Name, info.Value = widget.RoleComboBox, i18n.T("print.placement"), f.valueText(p)
	case PartMargins:
		info.Role, info.Name, info.Value = widget.RoleComboBox, i18n.T("print.margins"), f.valueText(p)
	case PartSystem:
		info.Role, info.Name = widget.RoleButton, i18n.T("print.system")
	case PartCopies:
		info.Role, info.Name, info.Value = widget.RoleSpinner, i18n.T("print.copies"), f.model.CopiesText()
	case PartCopiesDec:
		info.Role, info.Name = widget.RoleButton, i18n.T("print.copies.dec")
	case PartCopiesInc:
		info.Role, info.Name = widget.RoleButton, i18n.T("print.copies.inc")
	case PartWidth:
		info.Role, info.Name, info.Value = widget.RoleTextInput, i18n.T("print.width"), f.model.WidthText()
	case PartHeight:
		info.Role, info.Name, info.Value = widget.RoleTextInput, i18n.T("print.height"), f.model.HeightText()
	case PartCm:
		info.Role, info.Name = widget.RoleRadioButton, i18n.T("print.unit.cm")
		info.States = f.states(p, o.Unit == print.UnitCM)
	case PartInch:
		info.Role, info.Name = widget.RoleRadioButton, i18n.T("print.unit.in")
		info.States = f.states(p, o.Unit == print.UnitInch)
	case PartKeep:
		info.Role, info.Name = widget.RoleCheckBox, i18n.T("print.keep")
		info.States = f.states(p, o.Keep)
	case PartPrint:
		info.Role, info.Name = widget.RoleButton, f.PrintLabel()
	case PartCancel:
		info.Role, info.Name = widget.RoleButton, i18n.T("dialog.cancel")
	}
	return info, true
}

func (f *form) AccessChildren() []widget.AccessInfo {
	var out []widget.AccessInfo
	for _, p := range []Part{
		PartPrinter, PartSystem, PartCopies, PartCopiesDec, PartCopiesInc, PartPaper, PartOrientation, PartPlacement,
		PartWidth, PartHeight, PartCm, PartInch, PartKeep, PartMargins, PartPrint, PartCancel,
	} {
		if info, ok := f.partInfo(p); ok {
			out = append(out, info)
		}
	}
	out = append(out, widget.AccessInfo{
		ID: "preview", Role: widget.RoleImage, Name: i18n.T("print.preview"),
		Value: strings.Join(f.InfoLines(), "; "), Bounds: f.previewRect(),
	})
	for i, l := range f.InfoLines() {
		out = append(out, widget.AccessInfo{ID: "info." + strconv.Itoa(i), Role: widget.RoleLabel, Name: l, Bounds: f.infoRect()})
	}
	if ls := f.list; ls != nil {
		for i, it := range ls.items {
			states := []string{widget.StatePopup}
			if i == f.currentIndex(ls.part) {
				states = append(states, widget.StateSelected)
			}
			r := f.itemRect(i)
			out = append(out, widget.AccessInfo{ID: itemIDPrefix + strconv.Itoa(i), Role: widget.RoleMenuItem, Name: it, States: states, Bounds: r})
		}
	}
	return out
}

func (f *form) itemRect(i int) image.Rectangle {
	ls := f.list
	if ls == nil || i < ls.top || i >= ls.top+ls.rows {
		return image.Rectangle{}
	}
	m := f.lay.m
	row := i - ls.top
	return image.Rect(ls.rect.Min.X+listPad, ls.rect.Min.Y+listPad+row*m.itemH, ls.rect.Max.X-listPad, ls.rect.Min.Y+listPad+(row+1)*m.itemH)
}

func (f *form) AccessAction(id string, action widget.AccessActionKind) bool {
	if rest, ok := strings.CutPrefix(id, itemIDPrefix); ok {
		i, err := strconv.Atoi(rest)
		if f.list == nil || err != nil || i < 0 || i >= len(f.list.items) {
			return false
		}
		switch action {
		case widget.AccessInvoke, widget.AccessSelect:
			f.pickList(i)
			return true
		}
		return false
	}
	p := partOfID(id)
	if p == PartNone || !f.enabled(p) {
		return false
	}
	switch action {
	case widget.AccessFocus:
		f.enter(p)
		f.keyNav = true
		f.Invalidate()
		return true
	case widget.AccessToggle:
		if p != PartKeep {
			return false
		}
	case widget.AccessSelect:
		if p != PartCm && p != PartInch {
			return false
		}
	case widget.AccessExpand:
		if !isDropdown(p) {
			return false
		}
		f.openList(p)
		return f.list != nil
	case widget.AccessCollapse:
		if !isDropdown(p) {
			return false
		}
		f.closeList()
		return true
	case widget.AccessInvoke:
		if isField(p) {
			return false
		}
	default:
		return false
	}
	f.closeList()
	f.stop = p
	f.activate(p)
	return true
}
