package app

import (
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
	"time"

	codecall "github.com/oops1/PixGo/internal/codec/all"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/settings"
	"github.com/oops1/PixGo/internal/testutil"
)

func writeBigJPEG(t *testing.T, path string) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 600, 400))
	for y := 0; y < 400; y++ {
		for x := 0; x < 600; x++ {
			img.SetRGBA(x, y, color.RGBA{uint8(x * 255 / 600), uint8(y * 255 / 400), 140, 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := jpeg.Encode(f, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
}

func goldenApp(t *testing.T, w, h int, mutate func(*settings.Settings)) (*App, *harness) {
	t.Helper()
	i18n.SetLang("ru")
	t.Setenv("PIXGO_CONFIG_DIR", t.TempDir())
	t.Setenv("PIXGO_STATE_DIR", t.TempDir())
	t.Setenv("PIXGO_CACHE_DIR", t.TempDir())
	st := settings.Defaults()
	st.Theme = settings.ThemeDark
	if mutate != nil {
		mutate(&st)
	}
	eng := testutil.NewEngine(w, h)
	reg := codec.NewRegistry()
	codecall.Register(reg)
	a := New(eng, Options{Settings: settings.NewMemory(st), Registry: reg})
	a.Root.ForceEnabled = true
	eng.SetRootFullCanvas(a.Root)
	t.Cleanup(a.Ctl.Close)
	hh := &harness{t: t}
	_ = hh
	return a, nil
}

func waitReady(t *testing.T, a *App) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		a.Eng.Flush()
		if a.Ctl.Status() == StatusReady && !a.Ctl.Scanning() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("image not ready: status=%d err=%v", a.Ctl.Status(), a.Ctl.Err())
}

func TestMainWindowTitleBarMatchesMockup(t *testing.T) {
	ref, err := testutil.LoadRef("01_main_viewer.png")
	if err != nil {
		t.Skip(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "mountain.jpg")
	writeBigJPEG(t, path)
	a, _ := goldenApp(t, 770, 504, nil)
	if err := a.Ctl.Open(path); err != nil {
		t.Fatal(err)
	}
	waitReady(t, a)
	a.Root.Refresh()
	a.Eng.Flush()
	got := a.Eng.RenderOnce()

	title := image.Rect(0, 0, 770, 49)
	e := testutil.MeanError(got, ref, title)
	if out := testutil.OutDir(); out != "" {
		testutil.SavePNG(filepath.Join(out, "main.png"), got)
		testutil.SaveComparison(filepath.Join(out, "main_compare.png"), got.SubImage(title).(*image.RGBA), ref.SubImage(title).(*image.RGBA))
	}
	t.Logf("title bar mean error vs mockup: %.2f", e)
	if e > 7 {
		t.Errorf("title bar mean error %.2f exceeds 7", e)
	}
}

func goldenAppSharingState(t *testing.T, prev *App) (*App, *harness) {
	t.Helper()
	cfg, state, cache := os.Getenv("PIXGO_CONFIG_DIR"), os.Getenv("PIXGO_STATE_DIR"), os.Getenv("PIXGO_CACHE_DIR")
	eng := testutil.NewEngine(770, 504)
	reg := codec.NewRegistry()
	codecall.Register(reg)
	t.Setenv("PIXGO_CONFIG_DIR", cfg)
	t.Setenv("PIXGO_STATE_DIR", state)
	t.Setenv("PIXGO_CACHE_DIR", cache)
	a := New(eng, Options{Settings: settings.NewMemory(settings.Defaults()), Registry: reg})
	t.Cleanup(a.Ctl.Close)
	return a, nil
}
