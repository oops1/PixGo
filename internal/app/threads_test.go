package app

import (
	"testing"

	model "github.com/oops1/PixGo/internal/gallery"
	"github.com/oops1/PixGo/internal/par"
	"github.com/oops1/PixGo/internal/settings"
)

func setThreads(t *testing.T, a *App, n int) {
	t.Helper()
	a.Settings.Update(func(s *settings.Settings) { s.DecodeThreads = n })
	a.Eng.Flush()
}

func TestDecodeThreadsApplyToEveryPoolWithoutRestart(t *testing.T) {
	t.Cleanup(func() { par.Set(0) })
	a, _ := goldenApp(t, 900, 600, nil)
	t.Cleanup(a.Shutdown)
	a.OpenGalleryAt(model.Folder(t.TempDir()), "")
	setThreads(t, a, 16)
	if par.Count() != 16 {
		t.Fatalf("shared thread budget %d", par.Count())
	}
	if a.Ctl.LoaderWorkers() != 8 {
		t.Fatalf("loader workers %d", a.Ctl.LoaderWorkers())
	}
	if a.gal.svc.Workers() != 4 {
		t.Fatalf("thumbnail workers %d", a.gal.svc.Workers())
	}
	setThreads(t, a, 4)
	if par.Count() != 4 || a.Ctl.LoaderWorkers() != 2 || a.gal.svc.Workers() != 1 {
		t.Fatalf("after lowering: budget %d loader %d thumbs %d", par.Count(), a.Ctl.LoaderWorkers(), a.gal.svc.Workers())
	}
	setThreads(t, a, 0)
	if par.Configured() != 0 || par.Count() != par.Auto() {
		t.Fatalf("automatic mode: configured %d count %d", par.Configured(), par.Count())
	}
}

func TestStartupHonoursTheSavedThreadCount(t *testing.T) {
	t.Cleanup(func() { par.Set(0) })
	a, _ := goldenApp(t, 600, 400, func(s *settings.Settings) { s.DecodeThreads = 6 })
	if par.Count() != 6 || a.Ctl.LoaderWorkers() != 3 {
		t.Fatalf("budget %d loader %d", par.Count(), a.Ctl.LoaderWorkers())
	}
}

func TestControllerExposesLimitsAndRegistry(t *testing.T) {
	a, _ := goldenApp(t, 600, 400, nil)
	if a.Ctl.Registry() == nil || !a.Ctl.Supports("x.jpg") || a.Ctl.Supports("x.unknown") {
		t.Fatal("registry accessors")
	}
	if l := a.Ctl.Limits(); l.TiledAbove <= 0 || l.MaxTiledPixels < l.MaxPixels {
		t.Fatalf("limits %+v", l)
	}
	if a.Ctl.LoaderPending() != 0 {
		t.Fatalf("pending %d", a.Ctl.LoaderPending())
	}
}

func TestThreadSettingFallsBackToTheEnvironment(t *testing.T) {
	t.Setenv(settings.EnvThreads, "3")
	if got := threadSetting(0); got != 3 {
		t.Fatalf("automatic with PIXGO_THREADS=3: %d", got)
	}
	if got := threadSetting(5); got != 5 {
		t.Fatalf("an explicit setting must win: %d", got)
	}
	t.Setenv(settings.EnvThreads, "")
	if got := threadSetting(0); got != 0 {
		t.Fatalf("no setting and no variable: %d", got)
	}
}
