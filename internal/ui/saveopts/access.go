package saveopts

import (
	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/i18n"
)

var partIDs = map[Part]string{
	PartCheck:    "lossless",
	PartStripGPS: "stripGPS",
	PartSlider:   "quality",
	PartSave:     "save",
	PartCancel:   "cancel",
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
	return widget.AccessInfo{Role: widget.RoleGroup, Name: i18n.T("save.options.title"), Bounds: f.Bounds()}
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
	var out []widget.AccessInfo
	if f.lay.hasCheck {
		out = append(out, widget.AccessInfo{ID: partIDs[PartCheck], Role: widget.RoleCheckBox, Name: i18n.T("save.lossless"), States: f.states(PartCheck, f.lossless), Bounds: f.rect(PartCheck)})
	}
	if f.lay.hasGPS {
		out = append(out, widget.AccessInfo{ID: partIDs[PartStripGPS], Role: widget.RoleCheckBox, Name: i18n.T("save.stripGPS"), States: f.states(PartStripGPS, f.stripGPS), Bounds: f.rect(PartStripGPS)})
	}
	out = append(out,
		widget.AccessInfo{ID: partIDs[PartSlider], Role: widget.RoleSlider, Name: i18n.T("save.quality"), Value: f.valueText(), States: f.states(PartSlider, false), Bounds: f.rect(PartSlider)},
		widget.AccessInfo{ID: partIDs[PartSave], Role: widget.RoleButton, Name: i18n.T("save.options.save"), States: f.states(PartSave, false), Bounds: f.rect(PartSave)},
		widget.AccessInfo{ID: partIDs[PartCancel], Role: widget.RoleButton, Name: i18n.T("dialog.cancel"), States: f.states(PartCancel, false), Bounds: f.rect(PartCancel)})
	return out
}

func (f *form) AccessAction(id string, action widget.AccessActionKind) bool {
	p := partOfID(id)
	if p == PartNone || !f.enabled(p) {
		return false
	}
	switch action {
	case widget.AccessFocus:
		f.focusPart(p)
		return true
	case widget.AccessToggle:
		if p != PartCheck && p != PartStripGPS {
			return false
		}
	case widget.AccessInvoke:
		if p == PartSlider {
			return false
		}
	default:
		return false
	}
	f.stop = p
	f.activate(p)
	return true
}

func (f *form) focusPart(p Part) {
	f.stop = p
	f.keyNav = true
	f.Invalidate()
}
