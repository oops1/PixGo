package infopanel

import (
	"image"
	"strings"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/meta"
	"github.com/oops1/PixGo/internal/testutil"
	"github.com/oops1/PixGo/internal/ui/theme"
)

func realModel(lang string) Model {
	i18n.SetLang(lang)
	md := &meta.Metadata{
		Exif: &meta.Exif{Make: "Canon", Model: "EOS R5", ISO: 100, TakenAt: time.Date(2024, 9, 14, 18, 27, 3, 0, time.UTC)},
		GPS:  &meta.GPS{Lat: 55.75, Lon: 37.61},
	}
	return BuildModel(Input{
		Path: `D:\Photos\mountain.jpg`, Name: "mountain.jpg", FileSize: 8_730_000, Meta: md,
		Info:     codec.Info{Width: 6000, Height: 4000, FormatName: "JPEG"},
		Modified: time.Date(2024, 9, 14, 18, 27, 0, 0, time.UTC),
	})
}

func panelIn(t *testing.T, lang string) *Panel {
	t.Helper()
	_, p := renderPanel(t, realModel(lang))
	i18n.SetLang(lang)
	return p
}

func panelTree(t *testing.T, lang string) (*widget.AccessNode, *Panel) {
	t.Helper()
	p := panelIn(t, lang)
	return widget.BuildAccessTree(p, nil), p
}

func TestPanelAccessTreeIsNamedInBothLanguages(t *testing.T) {
	for _, lang := range i18n.Supported() {
		tr, _ := panelTree(t, lang)
		testutil.RequireNamed(t, tr)
		if tr.Role != widget.RoleGroup || tr.Name != i18n.T("info.title") || tr.Value != "mountain.jpg" {
			t.Errorf("%s: panel node %s %q %q", lang, tr.Role, tr.Name, tr.Value)
		}
		if testutil.FindAccess(tr, widget.RoleButton, i18n.T("info.close")) == nil {
			t.Errorf("%s: close button missing\n%s", lang, testutil.DescribeAccess(tr))
		}
		if testutil.FindAccess(tr, widget.RoleButton, i18n.T("info.copyPath")) == nil {
			t.Errorf("%s: copy button missing", lang)
		}
		for _, key := range []string{"info.sizes", "info.file", "info.camera", "info.gps"} {
			if testutil.FindAccess(tr, widget.RoleLabel, i18n.T(key)) == nil {
				t.Errorf("%s: card title %q missing", lang, i18n.T(key))
			}
		}
	}
}

func TestPanelRowsPairLabelsWithValues(t *testing.T) {
	tr, _ := panelTree(t, "en")
	dims := testutil.FindAccess(tr, widget.RoleLabel, "Dimensions")
	var found *widget.AccessNode
	for _, n := range testutil.FlatAccess(tr) {
		if n.Name == "Dimensions" && n.Value == "6000 × 4000" {
			found = n
		}
	}
	if dims == nil || found == nil {
		t.Errorf("dimensions row %+v", dims)
	}
	loc := testutil.FindAccess(tr, widget.RoleButton, "Location")
	if loc == nil || loc.Value != `D:\Photos\mountain.jpg` {
		t.Errorf("location link %+v", loc)
	}
	if n := testutil.FindAccess(tr, widget.RoleButton, "Open on the map"); n == nil || n.Value != "OpenStreetMap" {
		t.Errorf("map link %+v", n)
	}
	file := testutil.FindAccess(tr, widget.RoleLabel, "mountain.jpg")
	if file == nil || !strings.Contains(file.Value, "JPEG") {
		t.Errorf("file label %+v", file)
	}
}

func TestPanelThumbnailIsAnImage(t *testing.T) {
	tr, _ := panelTree(t, "en")
	n := testutil.FindAccess(tr, widget.RoleImage, "Thumbnail")
	if n == nil || n.Value != "mountain.jpg" {
		t.Errorf("thumbnail %+v", n)
	}
}

func TestPanelAccessBoundsStayInsideThePanel(t *testing.T) {
	tr, p := panelTree(t, "ru")
	for _, n := range testutil.FlatAccess(tr) {
		if !n.Bounds.Empty() && !n.Bounds.In(p.Bounds()) && n.Role != widget.RoleGroup {
			t.Errorf("%s %q at %v outside %v", n.Role, n.Name, n.Bounds, p.Bounds())
		}
	}
}

func TestPanelFocusMovesThroughCloseCopyAndLinks(t *testing.T) {
	p := panelIn(t, "en")
	if p.FocusActive() {
		t.Fatal("focus active before entering")
	}
	p.FocusEnter(1)
	var names []string
	collect := func() {
		for _, n := range testutil.FlatAccess(widget.BuildAccessTree(p, nil)) {
			if testutil.HasState(n, widget.StateFocused) {
				names = append(names, n.Name)
			}
		}
	}
	collect()
	for p.FocusStep(1) {
		collect()
	}
	want := []string{"Close the info panel", "Copy path", "Location", "Open on the map"}
	if strings.Join(names, "|") != strings.Join(want, "|") {
		t.Errorf("focus visited %v, want %v", names, want)
	}
	if p.FocusActive() {
		t.Error("panel kept focus after the last stop")
	}
}

func TestPanelFocusFromTheEndStartsOnTheLastStop(t *testing.T) {
	p := panelIn(t, "en")
	p.FocusEnter(-1)
	h, ok := p.focusedHit()
	if !ok || h.kind != hitLink || h.url == "" {
		t.Errorf("last stop %+v", h)
	}
	for p.FocusStep(-1) {
	}
	if p.FocusActive() {
		t.Error("shift-tab past the first stop kept the focus")
	}
}

func TestPanelEnterActivatesTheFocusedStop(t *testing.T) {
	p := panelIn(t, "en")
	var closed, revealed int
	var copied, opened string
	p.OnClose = func() { closed++ }
	p.OnReveal = func() { revealed++ }
	p.OnCopy = func(s string) { copied = s }
	p.OnOpenURL = func(s string) { opened = s }
	press := func(code widget.KeyCode) bool {
		return p.HandleKey(widget.KeyEvent{Code: code, Pressed: true})
	}
	if press(widget.KeyEnter) {
		t.Error("keys consumed while the panel has no focus")
	}
	p.FocusEnter(1)
	press(widget.KeyEnter)
	p.FocusStep(1)
	press(widget.KeySpace)
	p.FocusStep(1)
	press(widget.KeyEnter)
	p.FocusStep(1)
	press(widget.KeyEnter)
	if closed != 1 || copied != `D:\Photos\mountain.jpg` || revealed != 1 || !strings.HasPrefix(opened, "https://www.openstreetmap.org/") {
		t.Errorf("closed=%d copied=%q revealed=%d opened=%q", closed, copied, revealed, opened)
	}
}

func TestPanelEscapeClosesIt(t *testing.T) {
	p := panelIn(t, "en")
	closed := 0
	p.OnClose = func() { closed++ }
	p.FocusEnter(1)
	p.FocusStep(1)
	if !p.HandleKey(widget.KeyEvent{Code: widget.KeyEscape, Pressed: true}) || closed != 1 {
		t.Errorf("escape: closed=%d", closed)
	}
}

func TestPanelArrowKeysScrollWhileFocused(t *testing.T) {
	m := realModel("en")
	for i := 0; i < 4; i++ {
		m.Cards = append(m.Cards, m.Cards[2])
	}
	_, p := renderPanel(t, m)
	p.FocusEnter(1)
	if !p.HandleKey(widget.KeyEvent{Code: widget.KeyDown, Pressed: true}) || p.ScrollY() == 0 {
		t.Errorf("down did not scroll: %d", p.ScrollY())
	}
	p.HandleKey(widget.KeyEvent{Code: widget.KeyUp, Pressed: true})
	if p.ScrollY() != 0 {
		t.Errorf("up did not scroll back: %d", p.ScrollY())
	}
}

func TestPanelFocusRevealsStopsBelowTheFold(t *testing.T) {
	m := realModel("en")
	for i := 0; i < 5; i++ {
		m.Cards = append(m.Cards, Card{Kind: CardGPS, Title: "GPS", Rows: []Row{{Label: "Map", Value: "OSM", Link: true, URL: "https://example.org"}}})
	}
	_, p := renderPanel(t, m)
	p.FocusEnter(-1)
	h, ok := p.focusedHit()
	if !ok || !h.rect.In(p.Bounds()) {
		t.Errorf("last stop %v is outside the panel %v (scroll %d)", h.rect, p.Bounds(), p.ScrollY())
	}
}

func TestPanelRingIsDrawnForKeyboardFocusOnly(t *testing.T) {
	p := panelIn(t, "en")
	eng := p.eng.(*engine.Engine)
	render := func() *image.RGBA {
		eng.Invalidate()
		eng.Flush()
		return eng.RenderOnce()
	}
	p.FocusEnter(1)
	h, ok := p.focusedHit()
	if !ok {
		t.Fatal("no focused stop")
	}
	probe := image.Pt(h.rect.Min.X-1, (h.rect.Min.Y+h.rect.Max.Y)/2)
	if got := render().RGBAAt(probe.X, probe.Y); got != theme.Dark().Focus {
		t.Errorf("ring pixel %v, want %v", got, theme.Dark().Focus)
	}
	p.FocusLeave()
	if got := render().RGBAAt(probe.X, probe.Y); got == theme.Dark().Focus {
		t.Error("ring stayed after the focus left the panel")
	}
}
