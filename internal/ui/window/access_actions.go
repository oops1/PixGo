package window

import (
	"slices"
	"strconv"
	"strings"

	"github.com/oops1/headless-gui/v3/widget"
)

var zoneNames = map[ZoneKind]string{
	ZoneBack:            "back",
	ZoneInfo:            "info",
	ZoneFavorite:        "favorite",
	ZoneMore:            "more",
	ZoneMin:             "minimize",
	ZoneMax:             "maximize",
	ZoneClose:           "close",
	ZonePrev:            "prev",
	ZoneNext:            "next",
	ZoneToolGallery:     "tool.gallery",
	ZoneToolDelete:      "tool.delete",
	ZoneToolRotateLeft:  "tool.rotateLeft",
	ZoneToolZoomOut:     "tool.zoomOut",
	ZoneToolZoomLabel:   "tool.zoom",
	ZoneToolZoomIn:      "tool.zoomIn",
	ZoneToolRotateRight: "tool.rotateRight",
	ZoneToolEdit:        "tool.edit",
	ZoneToolMore:        "tool.more",
	ZoneToolFullscreen:  "tool.fullscreen",
	ZoneExit:            "exit",
	ZoneSlidePlay:       "slide.play",
	ZoneSlideSettings:   "slide.settings",
	ZoneSlideExit:       "slide.exit",
}

const zoneIDPrefix = "zone."

func zoneID(k ZoneKind) string {
	if name, ok := zoneNames[k]; ok {
		return name
	}
	return zoneIDPrefix + strconv.Itoa(int(k))
}

func zoneOfID(id string) (ZoneKind, bool) {
	if rest, ok := strings.CutPrefix(id, zoneIDPrefix); ok {
		n, err := strconv.Atoi(rest)
		_, named := zoneNames[ZoneKind(n)]
		return ZoneKind(n), err == nil && n > 0 && n < 256 && !named
	}
	for k, name := range zoneNames {
		if name == id {
			return k, true
		}
	}
	return ZoneNone, false
}

func (r *Root) publishedZones(l Layout) []ZoneKind {
	switch {
	case l.Opt.Page != PageViewer:
		return captionZones
	case l.Opt.Fullscreen:
		return slices.Concat(slideZones, arrowZones, captionZones)
	}
	return slices.Concat(titleZones, toolZones, arrowZones, captionZones)
}

func (r *Root) AccessAction(id string, action widget.AccessActionKind) bool {
	k, ok := zoneOfID(id)
	if !ok {
		return false
	}
	l := r.fullOverlayLayout()
	if !slices.Contains(r.publishedZones(l), k) || l.Rect(k).Empty() || !r.zoneEnabled(k) {
		return false
	}
	switch action {
	case widget.AccessFocus:
		if !slices.Contains(r.focusStops(), k) {
			return false
		}
		r.Wake()
		r.setFocus(k, 1)
		return true
	case widget.AccessInvoke:
	case widget.AccessToggle:
		if k != ZoneInfo && k != ZoneFavorite {
			return false
		}
	default:
		return false
	}
	r.activate(k)
	return true
}

func (r *Root) FocusPanel() {
	if r.host.Page() != PageViewer || r.panelGroup() == nil {
		return
	}
	r.Wake()
	r.setFocus(ZonePanel, 1)
}
