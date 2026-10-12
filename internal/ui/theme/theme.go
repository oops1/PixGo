package theme

import (
	"image/color"
	"math"
)

type Theme struct {
	Name string
	Dark bool

	Canvas       color.RGBA
	Checker1     color.RGBA
	Checker2     color.RGBA
	WindowBorder color.RGBA

	TitleBar       color.RGBA
	TitleText      color.RGBA
	TitleIcon      color.RGBA
	TitleActive    color.RGBA
	CaptionHover   color.RGBA
	CaptionPressed color.RGBA
	CloseHover     color.RGBA
	CloseText      color.RGBA
	Separator      color.RGBA
	Divider        color.RGBA

	Overlay       color.RGBA
	OverlayBorder color.RGBA
	OverlayText   color.RGBA
	OverlayHover  color.RGBA
	SlideBar      color.RGBA
	SliderTrack   color.RGBA
	ArrowFill     color.RGBA
	ArrowBorder   color.RGBA
	ArrowGlyph    color.RGBA

	Panel           color.RGBA
	PanelCard       color.RGBA
	PanelCardBorder color.RGBA
	PanelLabel      color.RGBA
	PanelValue      color.RGBA
	Link            color.RGBA

	Page            color.RGBA
	Nav             color.RGBA
	NavSelected     color.RGBA
	Status          color.RGBA
	GalleryTile     color.RGBA
	Selection       color.RGBA
	SelectionBorder color.RGBA
	Focus           color.RGBA

	Text          color.RGBA
	SecondaryText color.RGBA
	Disabled      color.RGBA
	Hover         color.RGBA
	Pressed       color.RGBA
	Accent        color.RGBA
	AccentText    color.RGBA

	Menu       color.RGBA
	MenuBorder color.RGBA
	MenuHover  color.RGBA
	Shadow     color.RGBA

	Card        color.RGBA
	CardBorder  color.RGBA
	Field       color.RGBA
	FieldBorder color.RGBA
	FieldBottom color.RGBA
	ToggleOff   color.RGBA
	ChoiceRing  color.RGBA

	ScrollThumb color.RGBA
	ScrollTrack color.RGBA
}

func rgb(v uint32) color.RGBA { return color.RGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 255} }

func rgba(v uint32, a uint8) color.RGBA {
	c := rgb(v)
	p := func(x uint8) uint8 { return uint8((uint32(x)*uint32(a) + 127) / 255) }
	return color.RGBA{p(c.R), p(c.G), p(c.B), a}
}

func Dark() *Theme {
	return &Theme{
		Name: "dark", Dark: true,

		Canvas: rgb(0x202020), Checker1: rgb(0xcbcbcb), Checker2: rgb(0x858385),
		WindowBorder: rgb(0x3d505d),

		TitleBar: rgb(0x17202a), TitleText: rgb(0xffffff), TitleIcon: rgb(0xffffff), TitleActive: rgb(0x333e47),
		CaptionHover: rgb(0x252e37), CaptionPressed: rgb(0x212933), CloseHover: rgb(0xc42b1c), CloseText: rgb(0xffffff),
		Separator: rgb(0x111720), Divider: rgb(0x242b31),

		Overlay: rgba(0x202022, 219), OverlayBorder: rgb(0x585e64), OverlayText: rgb(0xffffff),
		OverlayHover: rgba(0xffffff, 20), SlideBar: rgba(0x16181a, 184), SliderTrack: rgb(0x707274),
		ArrowFill: rgba(0x181818, 209), ArrowBorder: rgb(0x5c6260), ArrowGlyph: rgb(0xffffff),

		Panel: rgb(0x1c232a), PanelCard: rgb(0x232a31), PanelCardBorder: rgb(0x1e252b),
		PanelLabel: rgb(0xc9cfd4), PanelValue: rgb(0xffffff), Link: rgb(0x73d0ee),

		Page: rgb(0x1d232a), Nav: rgb(0x1a2027), NavSelected: rgb(0x28323b), Status: rgb(0x21272f),
		GalleryTile: rgb(0x313d46), Selection: rgb(0x1b2d41), SelectionBorder: rgb(0x1486e8), Focus: rgb(0xe6e6ea),

		Text: rgb(0xffffff), SecondaryText: rgb(0xcdd3d8), Disabled: rgb(0x6f757b),
		Hover: rgb(0x2a3139), Pressed: rgb(0x242b32), Accent: rgb(0x0b78f8), AccentText: rgb(0xffffff),

		Menu: rgb(0x242b32), MenuBorder: rgb(0x11161b), MenuHover: rgb(0x2f373f), Shadow: color.RGBA{0, 0, 0, 90},

		Card: rgb(0x232a31), CardBorder: rgb(0x1e252b), Field: rgb(0x2a3138), FieldBorder: rgb(0x333b43),
		FieldBottom: rgb(0x8f969c), ToggleOff: rgb(0x5f666b), ChoiceRing: rgb(0x50555c),

		ScrollThumb: rgb(0x8e959b), ScrollTrack: rgb(0x1d232a),
	}
}

func Light() *Theme {
	return &Theme{
		Name: "light",

		Canvas: rgb(0xf3f3f3), Checker1: rgb(0xffffff), Checker2: rgb(0xcccccc),
		WindowBorder: rgb(0xc4cad0),

		TitleBar: rgb(0xfbfbfc), TitleText: rgb(0x1a1a1a), TitleIcon: rgb(0x1a1a1a), TitleActive: rgb(0xdce3ea),
		CaptionHover: rgb(0xe9ecef), CaptionPressed: rgb(0xdfe3e7), CloseHover: rgb(0xc42b1c), CloseText: rgb(0xffffff),
		Separator: rgb(0xe1e4e8), Divider: rgb(0xdadee2),

		Overlay: rgba(0xfafafb, 224), OverlayBorder: rgb(0xc9ced3), OverlayText: rgb(0x1a1a1a),
		OverlayHover: rgba(0x000000, 18), SlideBar: rgba(0xfafafb, 216), SliderTrack: rgb(0xb8bec4),
		ArrowFill: rgba(0xffffff, 214), ArrowBorder: rgb(0xbdc3c9), ArrowGlyph: rgb(0x1a1a1a),

		Panel: rgb(0xf6f7f9), PanelCard: rgb(0xffffff), PanelCardBorder: rgb(0xe3e6ea),
		PanelLabel: rgb(0x5c6269), PanelValue: rgb(0x1a1a1a), Link: rgb(0x005fb8),

		Page: rgb(0xf3f3f3), Nav: rgb(0xeef0f2), NavSelected: rgb(0xe1e5ea), Status: rgb(0xf7f8f9),
		GalleryTile: rgb(0xd0d5da), Selection: rgb(0xdcebfb), SelectionBorder: rgb(0x0078d4), Focus: rgb(0x1a1a1a),

		Text: rgb(0x1a1a1a), SecondaryText: rgb(0x5c5c5c), Disabled: rgb(0xa0a0a0),
		Hover: rgb(0xe9ecef), Pressed: rgb(0xdee2e6), Accent: rgb(0x0078d4), AccentText: rgb(0xffffff),

		Menu: rgb(0xfafafa), MenuBorder: rgb(0xd9d9d9), MenuHover: rgb(0xebebeb), Shadow: color.RGBA{0, 0, 0, 40},

		Card: rgb(0xfbfbfb), CardBorder: rgb(0xe1e5e9), Field: rgb(0xffffff), FieldBorder: rgb(0xe5e5e5),
		FieldBottom: rgb(0xcccccc), ToggleOff: rgb(0x5d5d5d), ChoiceRing: rgb(0xc4c9ce),

		ScrollThumb: rgb(0x8a8a8a), ScrollTrack: rgb(0xf3f3f3),
	}
}

func HighContrast() *Theme {
	t := Dark()
	t.Name = "contrast"
	w, b, y, c, g := rgb(0xffffff), rgb(0x000000), rgb(0xffff00), rgb(0x00ffff), rgb(0x3ff23f)
	hv := rgb(0x1aebff)
	t.Canvas, t.Checker1, t.Checker2, t.WindowBorder = b, w, rgb(0x808080), w
	t.TitleBar, t.TitleText, t.TitleIcon, t.TitleActive = b, w, w, hv
	t.CaptionHover, t.CaptionPressed, t.CloseHover, t.CloseText = hv, hv, rgb(0xff0000), w
	t.Separator, t.Divider = w, w
	t.Overlay, t.OverlayBorder, t.OverlayText, t.OverlayHover = b, w, w, hv
	t.SlideBar, t.SliderTrack = b, w
	t.ArrowFill, t.ArrowBorder, t.ArrowGlyph = b, w, w
	t.Panel, t.PanelCard, t.PanelCardBorder, t.PanelLabel, t.PanelValue, t.Link = b, b, w, w, w, y
	t.Page, t.Nav, t.NavSelected, t.Status = b, b, hv, b
	t.GalleryTile, t.Selection, t.SelectionBorder, t.Focus = w, b, c, y
	t.Text, t.SecondaryText, t.Disabled = w, w, g
	t.Hover, t.Pressed, t.Accent, t.AccentText = hv, hv, c, b
	t.Menu, t.MenuBorder, t.MenuHover = b, w, hv
	t.Card, t.CardBorder, t.Field, t.FieldBorder, t.FieldBottom, t.ToggleOff, t.ChoiceRing = b, w, b, w, w, w, w
	t.ScrollThumb, t.ScrollTrack = w, b
	return t
}

func ByName(name string) *Theme {
	switch name {
	case "dark":
		return Dark()
	case "contrast":
		return HighContrast()
	}
	return Light()
}

func (t *Theme) WithAccent(c color.RGBA) *Theme {
	if t.Name == "contrast" {
		return t
	}
	n := *t
	c.A = 255
	n.Accent = c
	n.AccentText = readableOn(c)
	n.SelectionBorder = c
	n.Selection = mix(t.Page, c, 0.12)
	return &n
}

func mix(base, over color.RGBA, f float64) color.RGBA {
	m := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*f + .5) }
	return color.RGBA{m(base.R, over.R), m(base.G, over.G), m(base.B, over.B), 255}
}

func luminance(c color.RGBA) float64 {
	lin := func(v uint8) float64 {
		s := float64(v) / 255
		if s <= 0.03928 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(c.R) + 0.7152*lin(c.G) + 0.0722*lin(c.B)
}

func Contrast(a, b color.RGBA) float64 {
	la, lb := luminance(a), luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

func readableOn(c color.RGBA) color.RGBA {
	white, black := rgb(0xffffff), rgb(0x000000)
	if Contrast(c, white) >= Contrast(c, black) {
		return white
	}
	return black
}

func Over(top, bottom color.RGBA) color.RGBA {
	k := 1 - float64(top.A)/255
	m := func(x, y uint8) uint8 { return uint8(float64(x) + float64(y)*k + .5) }
	return color.RGBA{m(top.R, bottom.R), m(top.G, bottom.G), m(top.B, bottom.B), 255}
}
