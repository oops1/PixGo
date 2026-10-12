package infopanel

import (
	"image"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/meta"
)

func click(p *Panel, x, y int) {
	p.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft, Pressed: true, Clicks: 1})
	p.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft, Pressed: false, Clicks: 1})
}

func TestCloseCopyAndLinkClicks(t *testing.T) {
	_, p := renderPanel(t, mockupModel())
	var closed, revealed int
	var copied, opened string
	p.OnClose = func() { closed++ }
	p.OnReveal = func() { revealed++ }
	p.OnCopy = func(s string) { copied = s }
	p.OnOpenURL = func(s string) { opened = s }
	click(p, 689, 70)
	click(p, 689, 273)
	click(p, 580, 274)
	if closed != 1 || revealed != 1 || copied != mockupModel().Path || opened != "" {
		t.Errorf("closed=%d revealed=%d copied=%q opened=%q", closed, revealed, copied, opened)
	}
	click(p, 450, 300)
	if closed != 1 || revealed != 1 {
		t.Errorf("a click on plain text triggered an action")
	}
}

func TestMapLinkOpensURL(t *testing.T) {
	m := mockupModel()
	m.Cards = append(m.Cards[:2:2], Card{Kind: CardGPS, Title: "GPS", Rows: []Row{
		{Label: "Широта", Value: "55°45′21.1″ N"},
		{Label: "Открыть на карте", Value: "OpenStreetMap", Link: true, URL: "https://example.org/map"},
	}})
	_, p := renderPanel(t, m)
	var opened string
	p.OnOpenURL = func(s string) { opened = s }
	card := p.place().cards[2]
	row := card.rows[1]
	y := p.Bounds().Min.Y + card.rect.Min.Y + row.baseline - 5
	click(p, p.Bounds().Min.X+card.rect.Min.X+row.valueX+10, y)
	if opened != "https://example.org/map" {
		t.Errorf("opened %q", opened)
	}
}

func TestWheelScrollsAndClamps(t *testing.T) {
	m := mockupModel()
	for i := 0; i < 3; i++ {
		m.Cards = append(m.Cards, m.Cards[2])
	}
	_, p := renderPanel(t, m)
	if p.ContentHeight() <= p.Bounds().Dy() {
		t.Fatalf("content %d does not overflow %d", p.ContentHeight(), p.Bounds().Dy())
	}
	p.OnMouseButton(widget.MouseEvent{X: 500, Y: 200, Button: widget.MouseWheelDown})
	if p.ScrollY() != scrollStep {
		t.Errorf("scroll after one tick = %d", p.ScrollY())
	}
	p.OnMouseWheelPixelsMod(500, 200, 0, 100000, 0)
	if want := p.ContentHeight() - p.Bounds().Dy(); p.ScrollY() != want {
		t.Errorf("scroll not clamped to the end: %d want %d", p.ScrollY(), want)
	}
	p.OnMouseWheelPixelsMod(500, 200, 0, -100000, 0)
	if p.ScrollY() != 0 {
		t.Errorf("scroll not clamped to the start: %d", p.ScrollY())
	}
	if p.OnMouseWheelPixelsMod(50, 200, 0, 40, 0) {
		t.Error("wheel outside the panel was consumed")
	}
}

func TestShortContentDoesNotScroll(t *testing.T) {
	_, p := renderPanel(t, Model{Name: "a.png", Format: "PNG", Cards: []Card{{Kind: CardSizes, Title: "Размеры", Rows: []Row{{Label: "Размеры", Value: "1 × 1"}}}}})
	p.OnMouseButton(widget.MouseEvent{X: 500, Y: 200, Button: widget.MouseWheelDown})
	if p.ScrollY() != 0 {
		t.Errorf("scrolled short content: %d", p.ScrollY())
	}
}

func TestBuildModelFromMetadata(t *testing.T) {
	i18n.SetLang("ru")
	taken := time.Date(2024, 9, 14, 18, 27, 3, 0, time.UTC)
	md := &meta.Metadata{
		Exif: &meta.Exif{
			Make: "Canon", Model: "EOS R5", Lens: "RF 24-70mm F2.8 L",
			Exposure: meta.Rational{Num: 1, Den: 250}, FNumber: meta.Rational{Num: 40, Den: 10}, ISO: 100,
			FocalLength: meta.Rational{Num: 24, Den: 1}, TakenAt: taken,
		},
		GPS: &meta.GPS{Lat: 55.755861, Lon: 37.617361, Alt: 156, HasAlt: true},
		ICC: &meta.ICC{Description: "sRGB IEC61966-2.1", ColorSpace: "RGB"},
	}
	in := Input{
		Path: "/photos/mountain.jpg", Name: "mountain.jpg", FileSize: 9_160_000, Modified: taken, Created: taken, HasCreated: true,
		Info: codec.Info{FormatName: "JPEG", Width: 6000, Height: 4000, Orientation: codec.OrientNormal}, Meta: md,
	}
	m := BuildModel(in)
	if m.Format != "JPEG" || m.FileSize != "8,73 МБ" {
		t.Errorf("format=%q size=%q", m.Format, m.FileSize)
	}
	kinds := []CardKind{CardSizes, CardFile, CardCamera, CardColor, CardGPS}
	if len(m.Cards) != len(kinds) {
		t.Fatalf("%d cards, want %d", len(m.Cards), len(kinds))
	}
	for i, k := range kinds {
		if m.Cards[i].Kind != k {
			t.Errorf("card %d kind %d, want %d", i, m.Cards[i].Kind, k)
		}
	}
	rows := map[string]string{}
	for _, c := range m.Cards {
		for _, r := range c.Rows {
			rows[r.Label] = r.Value
		}
	}
	want := map[string]string{
		"Размеры": "6000 × 4000", "Соотношение сторон": "3:2", "Производитель": "Canon", "Модель": "EOS R5",
		"Выдержка": "1/250 с", "Диафрагма": "f/4.0", "ISO": "100", "Фокусное расстояние": "24 мм",
		"Дата создания": "14.09.2024 18:27", "Профиль ICC": "sRGB IEC61966-2.1",
	}
	for k, v := range want {
		if rows[k] != v {
			t.Errorf("%s = %q, want %q", k, rows[k], v)
		}
	}
	if rows["Высота"] != "156 м" {
		t.Errorf("altitude = %q", rows["Высота"])
	}
}

func TestBuildModelRotatedImageSwapsDimensions(t *testing.T) {
	i18n.SetLang("ru")
	m := BuildModel(Input{Path: "p.jpg", Name: "p.jpg", Info: codec.Info{Width: 6000, Height: 4000, Orientation: codec.OrientRotate90}})
	if got := m.Cards[0].Rows[0].Value; got != "4000 × 6000" {
		t.Errorf("dimensions = %q", got)
	}
}

func TestBuildModelWithoutMetadataShowsNote(t *testing.T) {
	i18n.SetLang("ru")
	m := BuildModel(Input{Path: "p.png", Name: "p.png", Info: codec.Info{Width: 2, Height: 2}, Meta: &meta.Metadata{}})
	last := m.Cards[len(m.Cards)-1]
	if last.Kind != CardNote || last.Rows[0].Label != "Метаданные отсутствуют" {
		t.Errorf("last card %+v", last)
	}
	pending := BuildModel(Input{Path: "p.png", Name: "p.png", Info: codec.Info{Width: 2, Height: 2}})
	if pending.Cards[len(pending.Cards)-1].Kind == CardNote {
		t.Error("note shown before metadata was read")
	}
}

func TestPlacementKeepsCardsOrderedAndInside(t *testing.T) {
	_, p := renderPanel(t, mockupModel())
	prev := 0
	for _, c := range p.place().cards {
		if c.rect.Min.Y < prev || c.rect.Max.X > p.Bounds().Dx() {
			t.Errorf("card %v misplaced after y=%d", c.rect, prev)
		}
		prev = c.rect.Max.Y
	}
	_ = image.Pt
}
