package resize

import (
	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/i18n"
)

var partIDs = map[Part]string{
	PartWidth:   "width",
	PartHeight:  "height",
	PartPixels:  "unit.pixels",
	PartPercent: "unit.percent",
	PartKeep:    "keep",
	PartApply:   "apply",
	PartCancel:  "cancel",
}

func partOfID(id string) Part {
	for p, s := range partIDs {
		if s == id {
			return p
		}
	}
	return PartNone
}

func (f *form) AccessInfo() widget.AccessInfo {
	return widget.AccessInfo{Role: widget.RoleGroup, Name: i18n.T("resize.title"), Bounds: f.Bounds()}
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
	return out
}

func (f *form) AccessChildren() []widget.AccessInfo {
	unit := f.model.Unit()
	out := []widget.AccessInfo{
		{ID: partIDs[PartPixels], Role: widget.RoleRadioButton, Name: i18n.T("resize.unit.px"), States: f.states(PartPixels, unit == UnitPixels), Bounds: f.rect(PartPixels)},
		{ID: partIDs[PartPercent], Role: widget.RoleRadioButton, Name: i18n.T("resize.unit.pct"), States: f.states(PartPercent, unit == UnitPercent), Bounds: f.rect(PartPercent)},
		{ID: partIDs[PartWidth], Role: widget.RoleTextInput, Name: i18n.T("resize.width"), Value: f.model.WidthText(), States: f.states(PartWidth, false), Bounds: f.rect(PartWidth)},
		{ID: partIDs[PartHeight], Role: widget.RoleTextInput, Name: i18n.T("resize.height"), Value: f.model.HeightText(), States: f.states(PartHeight, false), Bounds: f.rect(PartHeight)},
		{ID: partIDs[PartKeep], Role: widget.RoleCheckBox, Name: i18n.T("resize.keep"), States: f.states(PartKeep, f.model.Keep()), Bounds: f.rect(PartKeep)},
		{Role: widget.RoleLabel, Name: f.ResultText(), Bounds: f.lay.result.Add(f.Bounds().Min), ID: "result"},
		{ID: partIDs[PartApply], Role: widget.RoleButton, Name: i18n.T("resize.apply"), States: f.states(PartApply, false), Bounds: f.rect(PartApply)},
		{ID: partIDs[PartCancel], Role: widget.RoleButton, Name: i18n.T("dialog.cancel"), States: f.states(PartCancel, false), Bounds: f.rect(PartCancel)},
	}
	if !f.lay.orig.Empty() {
		out = append(out, widget.AccessInfo{Role: widget.RoleLabel, Name: f.OrigText(), Bounds: f.lay.orig.Add(f.Bounds().Min), ID: "original"})
	}
	return out
}

func (f *form) AccessAction(id string, action widget.AccessActionKind) bool {
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
		if p != PartPixels && p != PartPercent {
			return false
		}
	case widget.AccessInvoke:
		if p == PartWidth || p == PartHeight {
			return false
		}
	default:
		return false
	}
	f.stop = p
	f.activate(p)
	return true
}
