package window

import (
	"image"
	"testing"
)

func center(r image.Rectangle) (float64, float64) {
	return float64(r.Min.X+r.Max.X-1) / 2, float64(r.Min.Y+r.Max.Y-1) / 2
}

func near(t *testing.T, name string, gotX, gotY, wantX, wantY, tol float64) {
	t.Helper()
	if d := gotX - wantX; d > tol || d < -tol {
		t.Errorf("%s x = %.1f, want %.1f ± %.1f", name, gotX, wantX, tol)
	}
	if d := gotY - wantY; d > tol || d < -tol {
		t.Errorf("%s y = %.1f, want %.1f ± %.1f", name, gotY, wantY, tol)
	}
}

func TestMockupMainWindow(t *testing.T) {
	l := Compute(770, 504, Options{Toolbar: true, Arrows: true, Resizable: true})
	near(t, "back", float64(l.Rect(ZoneBack).Min.X+l.Rect(ZoneBack).Max.X-1)/2, float64(l.Rect(ZoneBack).Min.Y+l.Rect(ZoneBack).Max.Y-1)/2, 29.5, 24.5, 1)
	for _, c := range []struct {
		k      ZoneKind
		cx, cy float64
	}{
		{ZoneInfo, 502.5, 24.5},
		{ZoneFavorite, 547.5, 24.5},
		{ZoneMore, 592.5, 24.5},
		{ZoneMin, 645, 24},
		{ZoneMax, 693, 24},
		{ZoneClose, 741, 24},
		{ZoneToolGallery, 119.5, 466.5},
		{ZoneToolDelete, 178.5, 466.5},
		{ZoneToolRotateLeft, 245.5, 466.5},
		{ZoneToolZoomOut, 301.5, 466.5},
		{ZoneToolZoomLabel, 355.5, 466.5},
		{ZoneToolZoomIn, 410.5, 466.5},
		{ZoneToolRotateRight, 475.5, 466.5},
		{ZoneToolEdit, 530.5, 466.5},
		{ZoneToolMore, 585.5, 466.5},
		{ZoneToolFullscreen, 646.5, 466.5},
	} {
		x, y := center(l.Rect(c.k))
		near(t, zoneName(c.k), x, y, c.cx, c.cy, 1)
	}
	px, py := center(l.Prev)
	near(t, "prev arrow", px, py, 37.5, 257.5, 1)
	nx, ny := center(l.Next)
	near(t, "next arrow", nx, ny, 731.5, 257.5, 1)
	if l.Toolbar != image.Rect(88, 439, 682, 495) {
		t.Errorf("toolbar = %v, want (88,439)-(682,495)", l.Toolbar)
	}
	if l.Content != image.Rect(1, 49, 769, 503) {
		t.Errorf("content = %v", l.Content)
	}
}

func zoneName(k ZoneKind) string {
	return map[ZoneKind]string{
		ZoneInfo: "info", ZoneFavorite: "favorite", ZoneMore: "more", ZoneMin: "min", ZoneMax: "max", ZoneClose: "close",
		ZoneToolGallery: "gallery", ZoneToolDelete: "delete", ZoneToolRotateLeft: "rotate-left", ZoneToolZoomOut: "zoom-out",
		ZoneToolZoomLabel: "zoom-label", ZoneToolZoomIn: "zoom-in", ZoneToolRotateRight: "rotate-right", ZoneToolEdit: "edit",
		ZoneToolMore: "tool-more", ZoneToolFullscreen: "fullscreen",
	}[k]
}

func TestHitPriority(t *testing.T) {
	l := Compute(770, 504, Options{Toolbar: true, Arrows: true, Resizable: true})
	cases := []struct {
		p    image.Point
		want ZoneKind
	}{
		{image.Pt(741, 24), ZoneClose},
		{image.Pt(300, 24), ZoneDrag},
		{image.Pt(29, 24), ZoneBack},
		{image.Pt(502, 24), ZoneInfo},
		{image.Pt(355, 466), ZoneToolZoomLabel},
		{image.Pt(37, 258), ZonePrev},
		{image.Pt(400, 250), ZoneNone},
		{image.Pt(0, 0), ZoneResizeTopLeft},
		{image.Pt(769, 250), ZoneResizeRight},
		{image.Pt(400, 503), ZoneResizeBottom},
	}
	for _, c := range cases {
		if got := l.ZoneAt(c.p).Kind; got != c.want {
			t.Errorf("at %v got zone %d want %d", c.p, got, c.want)
		}
	}
}

func TestPanelShrinksView(t *testing.T) {
	l := Compute(900, 600, Options{PanelOpen: true, Toolbar: true, Arrows: true})
	if l.Panel.Dx() != PanelWidth {
		t.Errorf("panel width %d", l.Panel.Dx())
	}
	if l.View.Max.X != l.Panel.Min.X || l.View.Min.X != l.Content.Min.X {
		t.Errorf("view %v panel %v", l.View, l.Panel)
	}
	if !l.Toolbar.In(l.View) && l.Toolbar.Overlaps(l.Panel) {
		t.Errorf("toolbar %v overlaps panel %v", l.Toolbar, l.Panel)
	}
}

func TestMaximizedHasNoFrameOrResize(t *testing.T) {
	l := Compute(1920, 1080, Options{Maximized: true, Toolbar: true, Arrows: true, Resizable: true})
	if l.Content.Min.X != 0 || l.Content.Max.X != 1920 || l.Content.Max.Y != 1080 {
		t.Errorf("content %v", l.Content)
	}
	for _, z := range l.Zones {
		if z.Kind >= ZoneResizeTop {
			t.Errorf("resize zone %d on a maximized window", z.Kind)
		}
	}
}

func TestFullscreenHasNoTitle(t *testing.T) {
	l := Compute(1920, 1080, Options{Fullscreen: true, Arrows: true})
	if !l.Title.Empty() || l.Content.Min.Y != 0 {
		t.Errorf("title %v content %v", l.Title, l.Content)
	}
	if l.ZoneAt(image.Pt(1900, 20)).Kind != ZoneNone {
		t.Errorf("caption zone present in fullscreen")
	}
}

func TestNarrowWindowKeepsToolbarInside(t *testing.T) {
	for _, w := range []int{320, 400, 520, 640} {
		l := Compute(w, 400, Options{Toolbar: true, Arrows: true})
		if l.Toolbar.Min.X < l.View.Min.X || l.Toolbar.Max.X > l.View.Max.X {
			t.Errorf("w=%d toolbar %v outside view %v", w, l.Toolbar, l.View)
		}
		for _, z := range l.Zones {
			if z.Kind >= ZoneToolGallery && z.Kind <= ZoneToolFullscreen && !z.Rect.In(l.Toolbar.Inset(-ToolButtonSize/2)) {
				t.Errorf("w=%d zone %d %v far outside toolbar %v", w, z.Kind, z.Rect, l.Toolbar)
			}
		}
	}
}
