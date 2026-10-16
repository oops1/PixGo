package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/settings"
	uiwindow "github.com/oops1/PixGo/internal/ui/window"
)

func settleSettings(a *App) {
	for i := 0; i < 5; i++ {
		a.Eng.Flush()
		time.Sleep(5 * time.Millisecond)
	}
}

func TestSettingsPageOpensAndReturnsToTheViewer(t *testing.T) {
	a, _ := goldenApp(t, 770, 504, nil)
	t.Cleanup(a.Shutdown)

	a.OpenSettings()

	if !a.SettingsOpen() || a.Page() != uiwindow.PageSettings || a.Content() != a.set.page {
		t.Fatal("the settings page is not shown")
	}
	if a.Panel() != nil {
		t.Fatal("the info panel must not show on the settings page")
	}
	a.CloseSettings()
	if a.SettingsOpen() || a.Content() != a.Canvas {
		t.Fatal("the viewer must be back")
	}
}

func TestSettingsPageReturnsToTheGalleryItCameFrom(t *testing.T) {
	r := newGalleryRig(t, nil)
	r.open(t)

	r.app.OpenSettings()
	r.app.CloseSettings()

	if !r.app.GalleryOpen() || r.app.Content() != r.app.gal.page {
		t.Fatal("the gallery must be back")
	}
}

func TestSettingsCommandAndMenuOpenThePage(t *testing.T) {
	a, _ := goldenApp(t, 770, 504, nil)
	t.Cleanup(a.Shutdown)

	found := false
	for _, it := range a.menuItems() {
		if it.Text == i18n.T("cmd.settings") {
			found = true
		}
	}
	a.Commands.Run(CmdSettings)

	if !found || !a.SettingsOpen() {
		t.Fatalf("menu entry %v, page open %v", found, a.SettingsOpen())
	}
}

func TestOpeningSettingsTwiceKeepsTheFirstReturnTarget(t *testing.T) {
	r := newGalleryRig(t, nil)
	r.open(t)
	r.app.OpenSettings()

	r.app.OpenSettings()
	r.app.CloseSettings()

	if !r.app.GalleryOpen() {
		t.Fatal("the second open overwrote where to return")
	}
}

func TestSourceDescribesLanguagesFormatsAndAbout(t *testing.T) {
	a, _ := goldenApp(t, 770, 504, nil)
	t.Cleanup(a.Shutdown)
	s := a.ensureSettings()

	langs := s.Languages()
	if len(langs) < 3 || langs[0].ID != settings.LanguageSystem || langs[1].ID != "ru" && langs[2].ID != "ru" {
		t.Fatalf("languages = %+v", langs)
	}
	formats := s.Formats()
	if len(formats) < 8 {
		t.Fatalf("formats = %d", len(formats))
	}
	registered := map[string]bool{}
	for _, d := range a.Ctl.env.Registry.Registered() {
		registered[string(d.ID())] = true
	}
	for _, f := range formats {
		if f.Name == "" || len(f.Extensions) == 0 || f.Available != (registered[f.ID] || !f.Optional) {
			t.Fatalf("format %+v", f)
		}
	}
	about := s.About()
	if about.Repository == "" || about.Platform == "" || len(about.Components) < 5 || about.License == "" {
		t.Fatalf("about = %+v", about)
	}
	if !s.LastUpdateCheck().IsZero() {
		t.Fatal("no update check ran yet")
	}
	_ = s.ThumbCacheBytes()
}

func TestUpdateActionChangesSettingsAndAppliesThemLive(t *testing.T) {
	a, _ := goldenApp(t, 770, 504, nil)
	t.Cleanup(a.Shutdown)
	act := a.ensureSettings().actions()

	act.Update(func(s *settings.Settings) {
		s.Wheel = settings.WheelZoom
		s.SmoothZoom = true
		s.LoopFolder = true
		s.CacheMB = 512
	})
	settleSettings(a)

	if a.Settings.Get().Wheel != settings.WheelZoom || a.Canvas.Wheel != 0 {
		t.Fatalf("the wheel setting was not applied: %v", a.Canvas.Wheel)
	}
	if !a.Canvas.Smooth {
		t.Fatal("smooth zoom was not switched on")
	}
}

func TestLanguageChangeSwitchesTheInterfaceAtOnce(t *testing.T) {
	a, _ := goldenApp(t, 770, 504, nil)
	t.Cleanup(a.Shutdown)
	t.Cleanup(func() { i18n.SetLang("ru") })
	act := a.ensureSettings().actions()

	act.Update(func(s *settings.Settings) { s.Language = "en" })
	settleSettings(a)

	if i18n.Lang() != "en" || i18n.T("cmd.settings") != "Settings" {
		t.Fatalf("language is %q: %q", i18n.Lang(), i18n.T("cmd.settings"))
	}
}

func TestSortChangeReordersTheOpenFolder(t *testing.T) {
	a, _ := goldenApp(t, 770, 504, nil)
	t.Cleanup(a.Shutdown)
	dir := t.TempDir()
	for _, n := range []string{"a.jpg", "b.jpg", "c.jpg"} {
		writeBigJPEG(t, filepath.Join(dir, n))
	}
	a.OpenFiles([]string{filepath.Join(dir, "a.jpg")})
	waitName(t, a, "a.jpg")
	act := a.ensureSettings().actions()

	act.Update(func(s *settings.Settings) { s.SortDesc = true })
	settleSettings(a)
	a.Ctl.Step(-1)

	waitName(t, a, "b.jpg")
}

func TestThumbCacheCanBeClearedFromTheSettingsPage(t *testing.T) {
	a, _ := goldenApp(t, 770, 504, nil)
	t.Cleanup(a.Shutdown)
	dir := filepath.Join(os.Getenv("PIXGO_CACHE_DIR"), "thumbs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	s := a.ensureSettings()

	s.clearThumbCache()
	settleSettings(a)
	time.Sleep(50 * time.Millisecond)
	a.Eng.Flush()

	if s.ThumbCacheBytes() != 0 {
		t.Fatalf("cache usage = %d", s.ThumbCacheBytes())
	}
}

func TestCheckUpdatesActionRunsTheLookup(t *testing.T) {
	r := newUpdateRig(t, "0.2.1", nil)
	r.release.Tag = "v0.2.1"
	s := r.app.ensureSettings()

	s.actions().CheckUpdates()
	r.settle(t, 1)

	if len(r.notified) != 1 {
		t.Fatalf("notified = %q", r.notified)
	}
}

func TestOpenURLActionUsesTheSharedOpener(t *testing.T) {
	r := newUpdateRig(t, "0.2.1", nil)
	s := r.app.ensureSettings()

	s.actions().OpenURL("https://github.com/oops1/PixGo")

	if len(r.opened) != 1 || r.opened[0] != "https://github.com/oops1/PixGo" {
		t.Fatalf("opened = %q", r.opened)
	}
}

func TestDefaultAppsActionWorksWithoutPlatformSupport(t *testing.T) {
	a, _ := goldenApp(t, 770, 504, nil)
	t.Cleanup(a.Shutdown)

	a.ensureSettings().actions().OpenDefaultApps()
}

func TestSettingsShortcutWorksOnTheGalleryPage(t *testing.T) {
	r := newGalleryRig(t, nil)
	r.open(t)

	handled := r.app.Root.HandleKey(widget.KeyEvent{Code: widget.KeyOemComma, Mod: widget.ModCtrl, Pressed: true})

	if !handled || !r.app.SettingsOpen() {
		t.Fatalf("handled = %v, settings open = %v", handled, r.app.SettingsOpen())
	}
	r.app.CloseSettings()
	if !r.app.GalleryOpen() {
		t.Fatal("the gallery must be back after the settings page")
	}
}

func TestGalleryActionsOpenTheSettingsPage(t *testing.T) {
	r := newGalleryRig(t, nil)
	r.open(t)

	r.app.gal.actions().Settings()

	if !r.app.SettingsOpen() {
		t.Fatal("the gallery settings action did not open the page")
	}
}

func TestOpeningAFileClosesTheSettingsPage(t *testing.T) {
	a, _ := goldenApp(t, 770, 504, nil)
	t.Cleanup(a.Shutdown)
	first, _ := twoImages(t)
	a.OpenSettings()

	a.OpenFiles([]string{first})
	waitName(t, a, "a.jpg")

	if a.SettingsOpen() || a.Page() != uiwindow.PageViewer || a.Content() != a.Canvas {
		t.Fatalf("page = %v, the picture must be shown instead of the settings", a.Page())
	}
}

func TestOpeningAFileFromSettingsOpenedOverTheGalleryShowsTheViewer(t *testing.T) {
	r := newGalleryRig(t, nil)
	r.open(t)
	r.app.OpenSettings()

	r.app.OpenFiles([]string{r.b})
	waitName(t, r.app, "b.jpg")

	if r.app.SettingsOpen() || r.app.GalleryOpen() || r.app.Content() != r.app.Canvas {
		t.Fatalf("page = %v, want the viewer", r.app.Page())
	}
}

func TestHandoffClosesTheSettingsPageAndRaisesTheWindow(t *testing.T) {
	a, _ := goldenApp(t, 770, 504, nil)
	t.Cleanup(a.Shutdown)
	first, _ := twoImages(t)
	raised := 0
	a.SetOps(uiwindow.WindowOps{Raise: func(string) { raised++ }})
	a.OpenSettings()

	a.Handoff(EncodeHandoff("tok", []string{first}))
	waitName(t, a, "a.jpg")

	if a.SettingsOpen() || a.Page() != uiwindow.PageViewer || raised != 1 {
		t.Fatalf("page = %v, raised = %d", a.Page(), raised)
	}
}

func TestHandoffWithoutFilesKeepsTheSettingsPage(t *testing.T) {
	a, _ := goldenApp(t, 770, 504, nil)
	t.Cleanup(a.Shutdown)
	a.OpenSettings()

	a.Handoff(nil)

	if !a.SettingsOpen() {
		t.Fatal("a second launch without files must not close the settings")
	}
}
