package theme

import (
	"image"
	"image/color"
	"reflect"
	"testing"

	"github.com/oops1/PixGo/internal/testutil"
)

func eachToken(t *Theme, f func(name string, c color.RGBA)) {
	v := reflect.ValueOf(t).Elem()
	for i := 0; i < v.NumField(); i++ {
		if c, ok := v.Field(i).Interface().(color.RGBA); ok {
			f(v.Type().Field(i).Name, c)
		}
	}
}

func TestEveryTokenSetAndPremultiplied(t *testing.T) {
	for _, th := range []*Theme{Dark(), Light(), HighContrast()} {
		eachToken(th, func(name string, c color.RGBA) {
			if c == (color.RGBA{}) {
				t.Errorf("%s: %s is unset", th.Name, name)
			}
			if c.R > c.A || c.G > c.A || c.B > c.A {
				t.Errorf("%s: %s = %v is not a valid premultiplied colour", th.Name, name, c)
			}
		})
	}
}

func TestTranslucentTokens(t *testing.T) {
	translucent := map[string]bool{
		"Overlay": true, "OverlayHover": true, "SlideBar": true, "ArrowFill": true, "Shadow": true,
	}
	for _, th := range []*Theme{Dark(), Light()} {
		eachToken(th, func(name string, c color.RGBA) {
			if translucent[name] != (c.A != 255) {
				t.Errorf("%s: %s alpha = %d", th.Name, name, c.A)
			}
		})
	}
	hc := HighContrast()
	if hc.Overlay.A != 255 || hc.ArrowFill.A != 255 || hc.SlideBar.A != 255 {
		t.Error("high contrast overlays must be opaque")
	}
}

func TestRGBAPremultiplies(t *testing.T) {
	if c := rgba(0xffffff, 20); c != (color.RGBA{20, 20, 20, 20}) {
		t.Errorf("%v", c)
	}
	if c := rgba(0x202022, 219); c != (color.RGBA{27, 27, 29, 219}) {
		t.Errorf("%v", c)
	}
}

func TestOver(t *testing.T) {
	if c := Over(rgba(0x202022, 219), color.RGBA{142, 152, 162, 255}); c != (color.RGBA{47, 48, 52, 255}) {
		t.Errorf("%v", c)
	}
	if c := Over(color.RGBA{1, 2, 3, 255}, color.RGBA{200, 200, 200, 255}); c != (color.RGBA{1, 2, 3, 255}) {
		t.Errorf("%v", c)
	}
}

func TestContrast(t *testing.T) {
	for _, th := range []*Theme{Dark(), Light(), HighContrast()} {
		min := 4.5
		if th.Name == "contrast" {
			min = 7
		}
		for _, p := range []struct {
			name   string
			fg, bg color.RGBA
		}{
			{"Text/Page", th.Text, th.Page},
			{"Text/Nav", th.Text, th.Nav},
			{"TitleText/TitleBar", th.TitleText, th.TitleBar},
			{"TitleIcon/TitleBar", th.TitleIcon, th.TitleBar},
			{"PanelLabel/PanelCard", th.PanelLabel, th.PanelCard},
			{"PanelValue/PanelCard", th.PanelValue, th.PanelCard},
			{"SecondaryText/Page", th.SecondaryText, th.Page},
			{"Link/PanelCard", th.Link, th.PanelCard},
			{"Text/Menu", th.Text, th.Menu},
			{"Text/Field", th.Text, th.Field},
		} {
			if got := Contrast(p.fg, p.bg); got < min {
				t.Errorf("%s: %s contrast %.2f < %.1f", th.Name, p.name, got, min)
			}
		}
		if got := Contrast(th.AccentText, th.Accent); got < 3.5 {
			t.Errorf("%s: AccentText/Accent contrast %.2f", th.Name, got)
		}
		if got := Contrast(th.CloseText, th.CloseHover); got < 3.5 {
			t.Errorf("%s: CloseText/CloseHover contrast %.2f", th.Name, got)
		}
	}
	dark := Dark()
	over := Over(dark.Overlay, color.RGBA{255, 255, 255, 255})
	if got := Contrast(dark.OverlayText, over); got < 7 {
		t.Errorf("overlay text over a white photo: %.2f", got)
	}
	light := Light()
	over = Over(light.Overlay, color.RGBA{0, 0, 0, 255})
	if got := Contrast(light.OverlayText, over); got < 7 {
		t.Errorf("light overlay text over a black photo: %.2f", got)
	}
}

func TestByName(t *testing.T) {
	if ByName("dark").Name != "dark" || !ByName("dark").Dark || ByName("contrast").Name != "contrast" ||
		ByName("light").Name != "light" || ByName("").Name != "light" || ByName("nonsense").Name != "light" {
		t.Fatal("ByName")
	}
	if Light().Dark || !HighContrast().Dark {
		t.Fatal("Dark flag")
	}
	if Dark().Canvas == Light().Canvas || Dark().TitleBar == Light().TitleBar {
		t.Fatal("themes are identical")
	}
}

func TestWithAccent(t *testing.T) {
	d := Dark()
	pink := color.RGBA{0xe8, 0x3e, 0x8c, 0x80}
	n := d.WithAccent(pink)
	if n == d {
		t.Fatal("WithAccent must return a copy")
	}
	if n.Accent != (color.RGBA{0xe8, 0x3e, 0x8c, 0xff}) || n.SelectionBorder != n.Accent {
		t.Fatalf("accent %v border %v", n.Accent, n.SelectionBorder)
	}
	if d.Accent != Dark().Accent {
		t.Fatal("original changed")
	}
	if n.Selection == d.Selection || n.Selection.A != 255 {
		t.Fatalf("selection tint %v", n.Selection)
	}
	if got := Contrast(n.AccentText, n.Accent); got < 3 {
		t.Fatalf("AccentText contrast %.2f", got)
	}
	light := color.RGBA{0x60, 0xcd, 0xff, 255}
	if got := d.WithAccent(light).AccentText; got != (color.RGBA{0, 0, 0, 255}) {
		t.Fatalf("a light accent needs dark text, got %v", got)
	}
	dark := color.RGBA{0x00, 0x3a, 0x8c, 255}
	if got := Light().WithAccent(dark).AccentText; got != (color.RGBA{255, 255, 255, 255}) {
		t.Fatalf("a dark accent needs white text, got %v", got)
	}
	hc := HighContrast()
	if hc.WithAccent(pink) != hc {
		t.Fatal("high contrast must ignore the accent")
	}
}

func loadMockup(t *testing.T, name string) *image.RGBA {
	t.Helper()
	img, err := testutil.LoadRef(name)
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func average(img *image.RGBA, r image.Rectangle) color.RGBA {
	var sr, sg, sb, n int
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			c := img.RGBAAt(x, y)
			sr, sg, sb, n = sr+int(c.R), sg+int(c.G), sb+int(c.B), n+1
		}
	}
	return color.RGBA{uint8((sr + n/2) / n), uint8((sg + n/2) / n), uint8((sb + n/2) / n), 255}
}

func near(a, b color.RGBA, tol int) bool {
	d := func(x, y uint8) int {
		if x > y {
			return int(x - y)
		}
		return int(y - x)
	}
	return d(a.R, b.R) <= tol && d(a.G, b.G) <= tol && d(a.B, b.B) <= tol
}

func TestDarkTokensMatchMockups(t *testing.T) {
	dark := Dark()
	m1, m2, m3, m4 := loadMockup(t, "01_main_viewer.png"), loadMockup(t, "02_exif_panel.png"),
		loadMockup(t, "03_gallery.png"), loadMockup(t, "04_settings.png")
	for _, c := range []struct {
		name string
		img  *image.RGBA
		r    image.Rectangle
		want color.RGBA
		tol  int
	}{
		{"TitleBar", m1, image.Rect(4, 4, 18, 44), dark.TitleBar, 4},
		{"Separator", m1, image.Rect(100, 48, 400, 49), dark.Separator, 3},
		{"WindowBorder", m1, image.Rect(1, 8, 2, 40), dark.WindowBorder, 8},
		{"TitleActive", m2, image.Rect(436, 8, 442, 14), dark.TitleActive, 4},
		{"Panel", m2, image.Rect(400, 52, 690, 56), dark.Panel, 3},
		{"PanelCard", m2, image.Rect(420, 167, 700, 172), dark.PanelCard, 3},
		{"PanelCard lower", m2, image.Rect(420, 338, 700, 343), dark.PanelCard, 4},
		{"Page gallery", m3, image.Rect(153, 153, 158, 276), dark.Page, 4},
		{"Status", m3, image.Rect(3, 392, 21, 426), dark.Status, 3},
		{"NavSelected gallery", m3, image.Rect(100, 322, 140, 340), dark.NavSelected, 4},
		{"GalleryTile", m3, image.Rect(374, 90, 375, 110), dark.GalleryTile, 3},
		{"Accent", m3, image.Rect(453, 386, 465, 392), dark.Accent, 12},
		{"Nav settings", m4, image.Rect(5, 150, 140, 160), dark.Nav, 3},
		{"Page settings", m4, image.Rect(170, 62, 440, 66), dark.Page, 3},
		{"Field", m4, image.Rect(200, 198, 280, 202), dark.Field, 3},
		{"NavSelected settings", m4, image.Rect(40, 47, 140, 51), dark.NavSelected, 4},
	} {
		if got := average(c.img, c.r); !near(got, c.want, c.tol) {
			t.Errorf("%s: mockup %#02x%02x%02x, token %#02x%02x%02x (tolerance %d)",
				c.name, got.R, got.G, got.B, c.want.R, c.want.G, c.want.B, c.tol)
		}
	}
	for _, x := range []int{240, 400, 500} {
		above := average(m1, image.Rect(x, 428, x+6, 436))
		inside := average(m1, image.Rect(x, 443, x+6, 449))
		if got := Over(dark.Overlay, above); !near(got, inside, 16) {
			t.Errorf("Overlay over x=%d: composite %v, mockup %v", x, got, inside)
		}
	}
}
