package app

import (
	"fmt"
	"image"
	"image/color"
	"path/filepath"
	"testing"

	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/settings"
	"github.com/oops1/PixGo/internal/testutil"
	uiedit "github.com/oops1/PixGo/internal/ui/edit"
	"github.com/oops1/PixGo/internal/ui/theme"
	"github.com/oops1/PixGo/internal/ui/window"
)

func editShot(t *testing.T, lang, themeName string, w, h int, crop image.Rectangle) (*App, *image.RGBA) {
	t.Helper()
	a, _ := goldenApp(t, w, h, func(s *settings.Settings) { s.Theme = themeName })
	i18n.SetLang(lang)
	a.Root.ForceEnabled = false
	path := filepath.Join(t.TempDir(), "mountain.jpg")
	writeBigJPEG(t, path)
	if err := a.Ctl.Open(path); err != nil {
		t.Fatal(err)
	}
	waitReady(t, a)
	a.Root.Refresh()
	a.Eng.Flush()
	a.Eng.RenderOnce()
	a.Commands.Run(CmdEditStart)
	if !a.Editing() {
		t.Fatal("not editing")
	}
	a.Eng.Flush()
	a.Eng.RenderOnce()
	if !crop.Empty() {
		a.Editor.Crop().SetRect(crop)
	}
	a.Root.Refresh()
	a.Eng.Invalidate()
	a.Eng.Flush()
	src := a.Eng.RenderOnce()
	out := image.NewRGBA(src.Bounds())
	copy(out.Pix, src.Pix)
	return a, out
}

func themeOf(name string) *theme.Theme {
	if name == settings.ThemeLight {
		return theme.Light()
	}
	return theme.Dark()
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

func TestEditModeScreenAtTheReferenceSize(t *testing.T) {
	for _, name := range []string{settings.ThemeDark, settings.ThemeLight} {
		for _, lang := range i18n.Supported() {
			t.Run(fmt.Sprintf("%s/%s", name, lang), func(t *testing.T) {
				a, img := editShot(t, lang, name, 1100, 700, image.Rect(100, 60, 500, 340))
				th := themeOf(name)
				save := a.Editor.BarRect(uiedit.PartSaveCopy)
				if save.Empty() {
					t.Fatal("no save button")
				}
				if got := img.RGBAAt(save.Min.X+4, save.Min.Y+8); !near(got, th.Accent, 6) {
					t.Errorf("save button corner %v, want the accent %v", got, th.Accent)
				}
				pic := a.Editor.PictureScreen()
				cr := a.Editor.CropScreen()
				if pic.Empty() || cr.Empty() || !cr.In(pic) {
					t.Fatalf("picture %v crop %v", pic, cr)
				}
				inside := img.RGBAAt((cr.Min.X+cr.Max.X)/2, (cr.Min.Y+cr.Max.Y)/2)
				outside := img.RGBAAt(pic.Max.X-6, pic.Max.Y-6)
				lum := func(c color.RGBA) int { return int(c.R) + int(c.G) + int(c.B) }
				if lum(outside)*10 > lum(inside)*9 && lum(inside) > 60 {
					t.Errorf("outside %v is not dimmed against the crop %v", outside, inside)
				}
				if got := img.RGBAAt(cr.Min.X+cr.Dx()/2, cr.Min.Y); got.R < 200 || got.G < 200 || got.B < 200 {
					t.Errorf("crop outline pixel %v is not white", got)
				}
				if got := img.RGBAAt(a.Root.Bounds().Dx()/2, 10); got.A != 255 {
					t.Errorf("title bar pixel %v", got)
				}
				if out := testutil.OutDir(); out != "" {
					testutil.SavePNG(filepath.Join(out, fmt.Sprintf("edit_%s_%s.png", name, lang)), img)
				}
			})
		}
	}
}

func TestEditModeToolbarGoneAndViewerToolbarHasTheEditButton(t *testing.T) {
	a, img := editShot(t, "ru", settings.ThemeDark, 1100, 700, image.Rectangle{})
	if !a.Root.Layout().Rect(window.ZoneToolEdit).Empty() {
		t.Fatal("toolbar zone in the edit mode")
	}
	a.endEdit()
	a.Eng.Flush()
	view := a.Eng.RenderOnce()
	l := a.Root.Layout()
	tb := l.Toolbar
	if mid := (tb.Min.X + tb.Max.X) / 2; mid < 1100/2-2 || mid > 1100/2+2 {
		t.Errorf("toolbar centre %d, window centre 550", mid)
	}
	var kinds []window.ZoneKind
	for _, z := range l.Zones {
		if z.Kind >= window.ZoneToolGallery && z.Kind <= window.ZoneToolFullscreen {
			kinds = append(kinds, z.Kind)
		}
	}
	if len(kinds) != 10 {
		t.Fatalf("%d tool buttons", len(kinds))
	}
	if out := testutil.OutDir(); out != "" {
		testutil.SavePNG(filepath.Join(out, "viewer_toolbar_with_edit.png"), view)
		testutil.SavePNG(filepath.Join(out, "edit_bare.png"), img)
	}
}

func TestEditModeBarWrapsInANarrowWindow(t *testing.T) {
	a, img := editShot(t, "ru", settings.ThemeDark, 420, 520, image.Rectangle{})
	if a.Editor.BarRect(uiedit.PartSaveCopy).Max.X > 420 || a.Editor.BarRect(uiedit.PartCancel).Max.X > 420 {
		t.Fatalf("bar leaves the window: %v", a.Editor.Bar())
	}
	if out := testutil.OutDir(); out != "" {
		testutil.SavePNG(filepath.Join(out, "edit_narrow.png"), img)
	}
}
