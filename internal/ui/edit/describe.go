package edit

import (
	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/i18n"
)

const (
	idCrop = "crop"
	idSize = "size"
)

var partIDs = map[Part]string{
	PartAspect:      "bar.aspect",
	PartSwap:        "bar.swap",
	PartRotateLeft:  "bar.rotateLeft",
	PartRotateRight: "bar.rotateRight",
	PartFlipH:       "bar.flipH",
	PartFlipV:       "bar.flipV",
	PartResize:      "bar.resize",
	PartReset:       "bar.reset",
	PartSaveCopy:    "bar.saveCopy",
	PartSaveMore:    "bar.saveMore",
	PartCancel:      "bar.cancel",
}

func PartID(p Part) string { return partIDs[p] }

func (e *Editor) PartName(p Part) string {
	switch p {
	case PartAspect:
		return i18n.T("edit.aspect")
	case PartSwap:
		return i18n.T("edit.swap")
	case PartRotateLeft:
		return i18n.T("cmd.rotateLeft")
	case PartRotateRight:
		return i18n.T("cmd.rotateRight")
	case PartFlipH:
		return i18n.T("cmd.flipH")
	case PartFlipV:
		return i18n.T("cmd.flipV")
	case PartResize:
		return i18n.T("edit.resize")
	case PartReset:
		return i18n.T("edit.reset")
	case PartSaveCopy:
		return i18n.T("edit.saveCopy")
	case PartSaveMore:
		return i18n.T("edit.saveMenu")
	case PartCancel:
		return i18n.T("edit.cancel")
	}
	return ""
}

func (e *Editor) CropValue() string {
	r := e.crop.Rect()
	return i18n.T("edit.cropValue", r.Min.X, r.Min.Y, r.Dx(), r.Dy())
}

func (e *Editor) AccessInfo() widget.AccessInfo {
	return widget.AccessInfo{Role: widget.RoleGroup, Name: i18n.T("cmd.edit"), Bounds: e.Bounds()}
}

func (e *Editor) partStates(p Part) []string {
	var out []string
	if !e.Enabled(p) {
		out = append(out, widget.StateDisabled)
	}
	if e.kbd && e.focus == p {
		out = append(out, widget.StateFocused)
	}
	if e.menuPart == p {
		out = append(out, widget.StateExpanded)
	}
	return out
}

func (e *Editor) AccessChildren() []widget.AccessInfo {
	if !e.active {
		return nil
	}
	var out []widget.AccessInfo
	for _, p := range barStops {
		r := e.bar.rect(p)
		if r.Empty() {
			continue
		}
		info := widget.AccessInfo{ID: partIDs[p], Role: widget.RoleButton, Name: e.PartName(p), States: e.partStates(p), Bounds: r}
		switch p {
		case PartAspect:
			info.Role = widget.RoleComboBox
			info.Value = e.aspectLabel()
			info.States = append(info.States, widget.StateHasPopup)
		case PartSaveMore:
			info.States = append(info.States, widget.StateHasPopup)
		}
		out = append(out, info)
	}
	crop := widget.AccessInfo{ID: idCrop, Role: widget.RoleGroup, Name: i18n.T("edit.cropArea"), Value: e.CropValue(), Description: i18n.T("edit.handle.hint"), Bounds: e.CropScreen()}
	if e.kbd && e.focus == PartCrop {
		crop.States = append(crop.States, widget.StateFocused)
	}
	out = append(out,
		crop,
		widget.AccessInfo{ID: idSize, Role: widget.RoleLabel, Name: e.sizeText(), Bounds: e.SizePill()})
	return out
}

func (e *Editor) partOfID(id string) (Part, bool) {
	for p, s := range partIDs {
		if s == id {
			return p, true
		}
	}
	return PartNone, false
}

func (e *Editor) AccessAction(id string, action widget.AccessActionKind) bool {
	if !e.active {
		return false
	}
	if id == idCrop {
		if action != widget.AccessFocus {
			return false
		}
		e.SetFocusPart(PartCrop)
		return true
	}
	p, ok := e.partOfID(id)
	if !ok || e.bar.rect(p).Empty() || !e.Enabled(p) {
		return false
	}
	switch action {
	case widget.AccessFocus:
		e.SetFocusPart(p)
		return true
	case widget.AccessInvoke:
		e.activate(p)
		return true
	case widget.AccessExpand, widget.AccessSelect:
		if p != PartAspect && p != PartSaveMore {
			return false
		}
		e.activate(p)
		return true
	}
	return false
}
