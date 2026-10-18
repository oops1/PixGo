package app

import (
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
	codecall "github.com/oops1/PixGo/internal/codec/all"
	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/settings"
	"github.com/oops1/PixGo/internal/testutil"
	"github.com/oops1/PixGo/internal/ui/saveopts"
	uiset "github.com/oops1/PixGo/internal/ui/settings"
)

const (
	shotW = 900
	shotH = 600
)

func writeScene(t *testing.T, path string, hue int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 640, 420))
	for y := 0; y < 420; y++ {
		for x := 0; x < 640; x++ {
			h := float64(hue) + float64(x)*0.25
			r := 128 + 110*math.Sin(h*math.Pi/180)
			g := 128 + 110*math.Sin((h+120)*math.Pi/180)
			b := 128 + 110*math.Sin((h+240)*math.Pi/180)
			shade := 1 - float64(y)/700
			if (x/40+y/40)%2 == 0 {
				shade *= 0.8
			}
			img.SetRGBA(x, y, color.RGBA{uint8(r * shade), uint8(g * shade), uint8(b * shade), 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := jpeg.Encode(f, img, &jpeg.Options{Quality: 88}); err != nil {
		t.Fatal(err)
	}
}

type shotRig struct {
	t     *testing.T
	a     *App
	dir   string
	files []string
	k     float64
}

func newShotRig(t *testing.T, k float64, themeName string, lang string) *shotRig {
	t.Helper()
	i18n.SetLang(lang)
	t.Setenv("PIXGO_CONFIG_DIR", t.TempDir())
	t.Setenv("PIXGO_STATE_DIR", t.TempDir())
	t.Setenv("PIXGO_CACHE_DIR", t.TempDir())
	st := settings.Defaults()
	st.Theme = themeName
	st.AutoHidePanels = false
	eng := testutil.NewEngineScaled(shotW, shotH, k)
	t.Cleanup(eng.Stop)
	reg := codec.NewRegistry()
	codecall.Register(reg)
	a := New(eng, Options{Settings: settings.NewMemory(st), Registry: reg})
	eng.SetRootFullCanvas(a.Root)
	t.Cleanup(a.Shutdown)
	r := &shotRig{t: t, a: a, dir: t.TempDir(), k: k}
	for i := 0; i < 8; i++ {
		p := filepath.Join(r.dir, fmt.Sprintf("scene%02d.jpg", i+1))
		writeScene(t, p, i*45)
		r.files = append(r.files, p)
	}
	return r
}

func (r *shotRig) openImage() {
	r.t.Helper()
	if err := r.a.Ctl.Open(r.files[2]); err != nil {
		r.t.Fatal(err)
	}
	waitReady(r.t, r.a)
	r.a.Root.Refresh()
}

func (r *shotRig) openGallery(list bool) {
	r.t.Helper()
	if list {
		r.a.Settings.Update(func(s *settings.Settings) { s.GalleryView = settings.GalleryList })
	}
	g := newGalleryRigFor(r)
	g.open(r.t)
}

func newGalleryRigFor(r *shotRig) *galleryRig {
	return &galleryRig{app: r.a, root: r.dir}
}

func (r *shotRig) frame() *image.RGBA {
	r.t.Helper()
	for i := 0; i < 3; i++ {
		r.a.Eng.Flush()
		r.a.Eng.RenderOnce()
	}
	r.a.Root.Refresh()
	r.a.Eng.Flush()
	return r.a.Eng.RenderOnce()
}

func (r *shotRig) check(img *image.RGBA) {
	r.t.Helper()
	if want := image.Rect(0, 0, int(math.Round(shotW*r.k)), int(math.Round(shotH*r.k))); img.Bounds() != want {
		r.t.Fatalf("frame is %v at scale %v, want %v", img.Bounds(), r.k, want)
	}
	var flat = true
	first := img.RGBAAt(0, 0)
	for y := 0; y < img.Bounds().Dy() && flat; y += 7 {
		for x := 0; x < img.Bounds().Dx(); x += 7 {
			if img.RGBAAt(x, y) != first {
				flat = false
				break
			}
		}
	}
	if flat {
		r.t.Fatal("blank frame")
	}
}

func (r *shotRig) save(name string, img *image.RGBA) {
	r.t.Helper()
	r.check(img)
	if dir := testutil.OutDir(); dir != "" {
		file := filepath.Join(dir, fmt.Sprintf("%s_%s_%s_%.2f.png", name, i18n.Lang(), r.a.CurrentTheme().Name, r.k))
		if err := testutil.SavePNG(file, img); err != nil {
			r.t.Fatal(err)
		}
	}
}

type shotState struct {
	name string
	show func(r *shotRig)
}

func shotStates() []shotState {
	return []shotState{
		{"viewer", func(r *shotRig) { r.openImage() }},
		{"empty", func(r *shotRig) {}},
		{"loading", func(r *shotRig) {
			r.openImage()
			r.a.Ctl.status = StatusLoading
			r.a.Ctl.shown.Reduced = true
			r.a.Root.Refresh()
		}},
		{"focus_viewer", func(r *shotRig) {
			r.openImage()
			k := newTypist(r.t, r.a)
			k.tab(2)
		}},
		{"gallery_grid", func(r *shotRig) { r.openGallery(false) }},
		{"gallery_list", func(r *shotRig) { r.openGallery(true) }},
		{"focus_gallery", func(r *shotRig) {
			r.openGallery(false)
			k := newTypist(r.t, r.a)
			k.tab(3)
		}},
		{"info", func(r *shotRig) {
			r.openImage()
			r.a.toggleInfo()
		}},
		{"dialog", func(r *shotRig) {
			r.openImage()
			r.a.confirm(i18n.T("dialog.delete.title"), i18n.T("dialog.delete.message", "scene03.jpg"), func() {})
		}},
		{"saveopts", func(r *shotRig) {
			r.openImage()
			saveopts.Show(r.a.Eng, saveopts.JPEG, saveopts.Values{Quality: 85}, func(saveopts.Values, bool) {})
		}},
		{"menu", func(r *shotRig) {
			r.openImage()
			r.a.showMenuFromKeyboard()
		}},
		{"toast", func(r *shotRig) {
			r.openImage()
			r.a.Root.Toast(i18n.T("toast.saved"))
		}},
		{"slideshow", func(r *shotRig) {
			r.openImage()
			r.a.SetFullscreen(true)
			r.a.startSlideshow()
		}},
	}
}

func settingsStates() []shotState {
	var out []shotState
	for s := uiset.Section(0); s < 7; s++ {
		s := s
		out = append(out, shotState{fmt.Sprintf("settings%d", s), func(r *shotRig) {
			r.a.OpenSettings()
			r.a.set.page.SetSection(s)
		}})
	}
	return out
}

func shotLang() string {
	if l := os.Getenv("PIXGO_SNAPSHOT_LANG"); l != "" {
		return l
	}
	return "en"
}

func TestEveryStateRendersAtEveryScale(t *testing.T) {
	scales := []float64{1, 1.25, 1.5, 2}
	themes := []string{settings.ThemeDark, settings.ThemeLight}
	if testing.Short() || testutil.OutDir() == "" {
		scales, themes = []float64{1.5}, []string{settings.ThemeDark}
	}
	for _, k := range scales {
		for _, th := range themes {
			for _, st := range append(shotStates(), settingsStates()...) {
				st := st
				t.Run(fmt.Sprintf("%s/%s/%.2f", st.name, th, k), func(t *testing.T) {
					r := newShotRig(t, k, th, shotLang())
					st.show(r)
					r.save(st.name, r.frame())
				})
			}
		}
	}
}
