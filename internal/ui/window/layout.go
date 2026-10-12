package window

import "image"

type ZoneKind uint8

const (
	ZoneNone ZoneKind = iota
	ZoneDrag
	ZoneBack
	ZoneInfo
	ZoneFavorite
	ZoneMore
	ZoneMin
	ZoneMax
	ZoneClose
	ZonePrev
	ZoneNext
	ZoneToolGallery
	ZoneToolDelete
	ZoneToolRotateLeft
	ZoneToolZoomOut
	ZoneToolZoomLabel
	ZoneToolZoomIn
	ZoneToolRotateRight
	ZoneToolMore
	ZoneToolFullscreen
	ZoneExit
	ZoneResizeTop
	ZoneResizeBottom
	ZoneResizeLeft
	ZoneResizeRight
	ZoneResizeTopLeft
	ZoneResizeTopRight
	ZoneResizeBottomLeft
	ZoneResizeBottomRight
)

type Zone struct {
	Kind ZoneKind
	Rect image.Rectangle
}

const (
	TitleHeight     = 48
	SeparatorY      = 48
	ContentTop      = 49
	FrameWidth      = 1
	CornerRadius    = 8
	CaptionWidth    = 48
	CaptionClose    = 29
	CaptionStep     = 48
	TitleButtonW    = 40
	TitleButtonH    = 36
	TitleButtonTop  = 6
	TitleTextX      = 64
	BackCenterX     = 29
	InfoFromRight   = 268
	FavFromRight    = 223
	MoreFromRight   = 178
	ArrowDiameter   = 41
	ArrowLeftCX     = 37
	ArrowRightFromR = 38
	ToolbarWidth    = 539
	ToolbarHeight   = 56
	ToolbarBottom   = 9
	ToolbarRadius   = 12
	ToolButtonSize  = 40
	ToolZoomLabelW  = 56
	ResizeBand      = 5
	ResizeCorner    = 10
	PanelWidth      = 326
	ExitSize        = 40
	ExitMargin      = 8
)

var toolbarOffsets = [...]struct {
	kind   ZoneKind
	center int
	width  int
}{
	{ZoneToolGallery, 32, ToolButtonSize},
	{ZoneToolDelete, 91, ToolButtonSize},
	{ZoneToolRotateLeft, 158, ToolButtonSize},
	{ZoneToolZoomOut, 214, ToolButtonSize},
	{ZoneToolZoomLabel, 268, ToolZoomLabelW},
	{ZoneToolZoomIn, 323, ToolButtonSize},
	{ZoneToolRotateRight, 388, ToolButtonSize},
	{ZoneToolMore, 443, ToolButtonSize},
	{ZoneToolFullscreen, 504, ToolButtonSize},
}

type Options struct {
	Maximized  bool
	Fullscreen bool
	PanelOpen  bool
	PanelWidth int
	Toolbar    bool
	Arrows     bool
	Resizable  bool
}

type Layout struct {
	W, H      int
	Opt       Options
	Title     image.Rectangle
	Content   image.Rectangle
	View      image.Rectangle
	Panel     image.Rectangle
	Toolbar   image.Rectangle
	Prev      image.Rectangle
	Next      image.Rectangle
	Zones     []Zone
	TitleText image.Point
}

func (o Options) frame() int {
	if o.Maximized || o.Fullscreen {
		return 0
	}
	return FrameWidth
}

func Compute(w, h int, o Options) Layout {
	l := Layout{W: w, H: h, Opt: o}
	fr := o.frame()
	titleH := TitleHeight
	if o.Fullscreen {
		titleH = 0
	}
	l.Title = image.Rect(0, 0, w, titleH)
	top := ContentTop
	if o.Fullscreen {
		top = 0
	}
	l.Content = image.Rect(fr, top, w-fr, h-fr)
	l.View = l.Content
	if o.PanelOpen && !o.Fullscreen {
		pw := o.PanelWidth
		if pw <= 0 {
			pw = PanelWidth
		}
		pw = min(pw, max(0, l.Content.Dx()-120))
		l.Panel = image.Rect(l.Content.Max.X-pw, l.Content.Min.Y, l.Content.Max.X, l.Content.Max.Y)
		l.View = image.Rect(l.Content.Min.X, l.Content.Min.Y, l.Panel.Min.X, l.Content.Max.Y)
	}

	var zones []Zone
	if o.Resizable && !o.Maximized && !o.Fullscreen {
		zones = append(zones, resizeZones(w, h)...)
	}

	tbW := min(ToolbarWidth, max(0, l.View.Dx()-16))
	tbLeft := l.View.Min.X + (l.View.Dx()-tbW)/2
	tbTop := l.View.Max.Y - (ToolbarHeight + ToolbarBottom - fr)
	l.Toolbar = image.Rect(tbLeft, tbTop, tbLeft+tbW, tbTop+ToolbarHeight)
	toolbarCY := tbTop + ToolbarHeight/2

	if o.Toolbar {
		scale := float64(tbW) / float64(ToolbarWidth)
		for _, t := range toolbarOffsets {
			cx := tbLeft + int(float64(t.center)*scale+.5)
			bw := t.width
			zones = append(zones, Zone{t.kind, image.Rect(cx-bw/2, toolbarCY-ToolButtonSize/2, cx-bw/2+bw, toolbarCY-ToolButtonSize/2+ToolButtonSize)})
		}
	}

	arrowCY := (l.View.Min.Y + toolbarCY) / 2
	if o.Fullscreen {
		arrowCY = (l.View.Min.Y + l.View.Max.Y) / 2
	}
	r := ArrowDiameter / 2
	pcx := l.View.Min.X + ArrowLeftCX - fr
	ncx := l.View.Max.X - ArrowRightFromR + fr
	l.Prev = image.Rect(pcx-r, arrowCY-r, pcx-r+ArrowDiameter, arrowCY-r+ArrowDiameter)
	l.Next = image.Rect(ncx-r-1, arrowCY-r, ncx-r-1+ArrowDiameter, arrowCY-r+ArrowDiameter)
	if o.Arrows {
		zones = append(zones, Zone{ZonePrev, l.Prev}, Zone{ZoneNext, l.Next})
	}

	if o.Fullscreen && o.Toolbar {
		ex := image.Rect(w-ExitMargin-ExitSize, ExitMargin, w-ExitMargin, ExitMargin+ExitSize)
		zones = append(zones, Zone{ZoneExit, ex})
	}

	if !o.Fullscreen {
		for i, k := range []ZoneKind{ZoneMin, ZoneMax, ZoneClose} {
			cx := w - CaptionClose - (2-i)*CaptionStep
			zones = append(zones, Zone{k, image.Rect(cx-CaptionWidth/2, 0, cx-CaptionWidth/2+CaptionWidth, TitleHeight)})
		}
		for _, b := range []struct {
			k    ZoneKind
			from int
		}{{ZoneInfo, InfoFromRight}, {ZoneFavorite, FavFromRight}, {ZoneMore, MoreFromRight}} {
			cx := w - b.from
			zones = append(zones, Zone{b.k, titleButtonRect(cx)})
		}
		zones = append(zones, Zone{ZoneBack, titleButtonRect(BackCenterX)})
		capLeft := w - CaptionClose - 2*CaptionStep - CaptionWidth/2
		zones = append(zones, Zone{ZoneDrag, image.Rect(0, 0, capLeft, TitleHeight)})
		l.TitleText = image.Pt(TitleTextX, TitleHeight/2)
	}
	l.Zones = zones
	return l
}

func titleButtonRect(cx int) image.Rectangle {
	return image.Rect(cx-TitleButtonW/2, TitleButtonTop, cx-TitleButtonW/2+TitleButtonW, TitleButtonTop+TitleButtonH)
}

func resizeZones(w, h int) []Zone {
	b, c := ResizeBand, ResizeCorner
	return []Zone{
		{ZoneResizeTopLeft, image.Rect(0, 0, c, c)},
		{ZoneResizeTopRight, image.Rect(w-c, 0, w, c)},
		{ZoneResizeBottomLeft, image.Rect(0, h-c, c, h)},
		{ZoneResizeBottomRight, image.Rect(w-c, h-c, w, h)},
		{ZoneResizeTop, image.Rect(0, 0, w, b)},
		{ZoneResizeBottom, image.Rect(0, h-b, w, h)},
		{ZoneResizeLeft, image.Rect(0, 0, b, h)},
		{ZoneResizeRight, image.Rect(w-b, 0, w, h)},
	}
}

func (l Layout) ZoneAt(p image.Point) Zone {
	for _, z := range l.Zones {
		if p.In(z.Rect) {
			return z
		}
	}
	return Zone{}
}

func (l Layout) Rect(k ZoneKind) image.Rectangle {
	for _, z := range l.Zones {
		if z.Kind == k {
			return z.Rect
		}
	}
	return image.Rectangle{}
}

func (l Layout) Center(k ZoneKind) (int, int) {
	r := l.Rect(k)
	return (r.Min.X + r.Max.X) / 2, (r.Min.Y + r.Max.Y) / 2
}
