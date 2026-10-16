package window

import (
	"image"
	"strconv"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/i18n"
)

var (
	titleZones   = []ZoneKind{ZoneBack, ZoneInfo, ZoneFavorite, ZoneMore}
	toolZones    = []ZoneKind{ZoneToolGallery, ZoneToolDelete, ZoneToolRotateLeft, ZoneToolZoomOut, ZoneToolZoomLabel, ZoneToolZoomIn, ZoneToolRotateRight, ZoneToolMore, ZoneToolFullscreen}
	arrowZones   = []ZoneKind{ZonePrev, ZoneNext}
	captionZones = []ZoneKind{ZoneMin, ZoneMax, ZoneClose}
	slideZones   = []ZoneKind{ZoneSlidePlay, ZoneSlideSettings, ZoneSlideExit, ZoneExit}
)

func (r *Root) AccessInfo() widget.AccessInfo {
	return widget.AccessInfo{Role: widget.RoleWindow, Name: i18n.T("app.name"), Bounds: r.Bounds()}
}

func (r *Root) AccessChildren() []widget.AccessInfo {
	l := r.fullOverlayLayout()
	var out []widget.AccessInfo
	switch {
	case l.Opt.Page != PageViewer:
	case l.Opt.Fullscreen:
		out = r.accessSlideshow(l)
	default:
		out = r.accessViewer(l)
	}
	out = r.appendZones(out, l, captionZones)
	return r.appendTransient(out, l)
}

func (r *Root) appendZones(out []widget.AccessInfo, l Layout, kinds []ZoneKind) []widget.AccessInfo {
	for _, k := range kinds {
		if info, ok := r.zoneAccess(l, k); ok {
			out = append(out, info)
		}
	}
	return out
}

func (r *Root) accessViewer(l Layout) []widget.AccessInfo {
	out := r.appendZones(nil, l, titleZones[:1])
	if rect := r.titleTextRect(l); !rect.Empty() {
		out = append(out, widget.AccessInfo{Role: widget.RoleLabel, Name: r.host.Title(), Bounds: rect})
	}
	out = r.appendZones(out, l, titleZones[1:])
	out = r.appendZones(out, l, toolZones)
	return r.appendZones(out, l, arrowZones)
}

func (r *Root) accessSlideshow(l Layout) []widget.AccessInfo {
	out := r.appendZones(nil, l, slideZones)
	out = r.appendZones(out, l, arrowZones)
	v := r.host.Slideshow()
	sl := l.Slide
	if sl.Panel.Empty() {
		return out
	}
	counter := image.Rect(sl.CounterX, sl.Panel.Min.Y, max(sl.CounterX, int(sl.TrackX0)-slideCounterGap), sl.Panel.Max.Y)
	out = append(out, widget.AccessInfo{Role: widget.RoleLabel, Name: v.CounterText(), Bounds: counter})
	if v.Active && sl.HasTrack() {
		out = append(out, widget.AccessInfo{
			Role:   widget.RoleProgressBar,
			Name:   i18n.T("access.slideshow"),
			Value:  strconv.FormatFloat(min(max(v.Progress, 0), 1), 'f', 2, 64),
			Bounds: sl.Damage,
		})
	}
	return out
}

func (r *Root) appendTransient(out []widget.AccessInfo, l Layout) []widget.AccessInfo {
	if l.Opt.Page != PageViewer {
		if r.toastText != "" {
			out = append(out, widget.AccessInfo{Role: widget.RoleLabel, Name: r.toastText, Bounds: r.toastRect(l)})
		}
		return out
	}
	if head, body, ok := r.stateText(); ok {
		out = append(out, widget.AccessInfo{Role: widget.RoleLabel, Name: head, Description: body, Bounds: l.View})
	}
	if r.loadingShown() {
		out = append(out, widget.AccessInfo{Role: widget.RoleProgressBar, Name: r.loadingName(), Bounds: loadingRect(l.View)})
	}
	if rect := r.PreviewBadge(); !rect.Empty() {
		out = append(out, widget.AccessInfo{Role: widget.RoleLabel, Name: i18n.T("viewer.preview"), Bounds: rect})
	}
	if r.toastText != "" {
		out = append(out, widget.AccessInfo{Role: widget.RoleLabel, Name: r.toastText, Bounds: r.toastRect(l)})
	}
	return out
}

func (r *Root) zoneEnabled(k ZoneKind) bool {
	switch k {
	case ZoneMin, ZoneMax, ZoneClose, ZoneSlideSettings:
		return true
	}
	return r.enabled(k)
}

func (r *Root) zoneAccess(l Layout, k ZoneKind) (widget.AccessInfo, bool) {
	rect := l.Rect(k)
	if rect.Empty() {
		return widget.AccessInfo{}, false
	}
	info := widget.AccessInfo{Role: widget.RoleButton, Name: r.zoneLabel(k), Description: r.zoneShortcut(k), Bounds: rect}
	var states []string
	switch k {
	case ZoneInfo:
		info.Role = widget.RoleCheckBox
		if r.host.InfoOpen() {
			states = append(states, widget.StateChecked)
		}
	case ZoneFavorite:
		info.Role, info.Name = widget.RoleCheckBox, i18n.T("access.favorite")
		if r.host.Favorite() {
			states = append(states, widget.StateChecked)
		}
	case ZoneToolZoomLabel:
		info.Value = r.host.ZoomText()
	}
	if !r.zoneEnabled(k) {
		states = append(states, widget.StateDisabled)
	}
	if r.kbd && r.focus == k {
		states = append(states, widget.StateFocused)
	}
	info.States = states
	return info, true
}

func (r *Root) titleTextRect(l Layout) image.Rectangle {
	if l.Title.Empty() || l.Opt.Page != PageViewer {
		return image.Rectangle{}
	}
	limit := l.Rect(ZoneInfo).Min.X - titleGap
	if limit <= TitleTextX {
		limit = l.Rect(ZoneDrag).Max.X - titleGap
	}
	if limit <= TitleTextX {
		return image.Rectangle{}
	}
	return image.Rect(TitleTextX, 0, limit, TitleHeight)
}
