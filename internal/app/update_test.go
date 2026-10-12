package app

import (
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oops1/PixGo/internal/buildinfo"
	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/settings"
	"github.com/oops1/PixGo/internal/updates"
)

type updateRig struct {
	app      *App
	release  updates.Release
	err      error
	clock    time.Time
	lookups  atomic.Int32
	notified []string
	failed   []string
	asked    []string
	yes      func()
	opened   []string
}

func newUpdateRig(t *testing.T, version string, mutate func(*settings.Settings)) *updateRig {
	t.Helper()
	prev := buildinfo.Version
	buildinfo.Version = version
	t.Cleanup(func() { buildinfo.Version = prev })
	a, _ := goldenApp(t, 770, 504, mutate)
	r := &updateRig{app: a, clock: time.Date(2026, 10, 12, 18, 0, 0, 0, time.UTC)}
	a.upd = updateHooks{
		latest: func(contextT) (updates.Release, error) {
			r.lookups.Add(1)
			return r.release, r.err
		},
		open:   func(rawURL string) { r.opened = append(r.opened, rawURL) },
		notify: func(title, message string) { r.notified = append(r.notified, title+"|"+message) },
		fail:   func(message string) { r.failed = append(r.failed, message) },
		ask: func(title, message string, yes func()) {
			r.asked = append(r.asked, title+"|"+message)
			r.yes = yes
		},
		now: func() time.Time { return r.clock },
	}
	return r
}

func (r *updateRig) settle(t *testing.T, want int32) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		r.app.Eng.Flush()
		if r.lookups.Load() >= want {
			time.Sleep(20 * time.Millisecond)
			r.app.Eng.Flush()
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("lookup did not run: %d of %d", r.lookups.Load(), want)
}

func (r *updateRig) lastCheck() time.Time { return r.app.Settings.Get().Updates.LastCheck }

func TestManualCheckSaysTheBuildIsCurrent(t *testing.T) {
	r := newUpdateRig(t, "0.2.1", nil)
	r.release = updates.Release{Tag: "v0.2.1"}

	r.app.CheckForUpdates()
	r.settle(t, 1)

	if len(r.notified) != 1 || r.notified[0] != i18n.T("update.title")+"|"+i18n.T("update.upToDate", "0.2.1") {
		t.Fatalf("notified = %q", r.notified)
	}
	if len(r.asked) != 0 || len(r.failed) != 0 {
		t.Fatalf("asked %q, failed %q", r.asked, r.failed)
	}
	if !r.lastCheck().Equal(r.clock) {
		t.Fatalf("last check = %v, want %v", r.lastCheck(), r.clock)
	}
}

func TestManualCheckOffersTheReleasePageOfANewerVersion(t *testing.T) {
	r := newUpdateRig(t, "0.2.1", nil)
	r.release = updates.Release{Tag: "v0.3.0", Page: "https://github.com/oops1/PixGo/releases/tag/v0.3.0"}

	r.app.CheckForUpdates()
	r.settle(t, 1)

	if len(r.asked) != 1 || !strings.Contains(r.asked[0], "0.3.0") || !strings.Contains(r.asked[0], "0.2.1") {
		t.Fatalf("asked = %q", r.asked)
	}
	if strings.Contains(r.asked[0], "v0.3.0") {
		t.Fatalf("the version must be shown without the tag prefix: %q", r.asked[0])
	}
	if len(r.opened) != 0 {
		t.Fatalf("nothing may open before the answer, opened %q", r.opened)
	}
	r.yes()
	if len(r.opened) != 1 || r.opened[0] != "https://github.com/oops1/PixGo/releases/tag/v0.3.0" {
		t.Fatalf("opened = %q", r.opened)
	}
	if len(r.notified) != 0 {
		t.Fatalf("notified = %q", r.notified)
	}
}

func TestOfferFallsBackToTheLatestReleasesPage(t *testing.T) {
	r := newUpdateRig(t, "0.2.1", nil)
	r.release = updates.Release{Tag: "v0.3.0"}

	r.app.CheckForUpdates()
	r.settle(t, 1)
	r.yes()

	if len(r.opened) != 1 || r.opened[0] != updates.PageURL {
		t.Fatalf("opened = %q", r.opened)
	}
}

func TestManualCheckReportsAFailureAndKeepsTheLastCheck(t *testing.T) {
	r := newUpdateRig(t, "0.2.1", nil)
	r.err = errors.New("offline")

	r.app.CheckForUpdates()
	r.settle(t, 1)

	if len(r.failed) != 1 || r.failed[0] != i18n.T("update.failed") {
		t.Fatalf("failed = %q", r.failed)
	}
	if !r.lastCheck().IsZero() {
		t.Fatalf("a failed check must not count: %v", r.lastCheck())
	}
	if len(r.notified) != 0 || len(r.asked) != 0 {
		t.Fatalf("notified %q, asked %q", r.notified, r.asked)
	}
}

func TestManualCheckExplainsTheRateLimit(t *testing.T) {
	r := newUpdateRig(t, "0.2.1", nil)
	r.err = updates.ErrRateLimited

	r.app.CheckForUpdates()
	r.settle(t, 1)

	if len(r.failed) != 1 || r.failed[0] != i18n.T("update.rateLimited") {
		t.Fatalf("failed = %q", r.failed)
	}
}

func TestStartupCheckRunsWhenDueAndRemembersTheTime(t *testing.T) {
	r := newUpdateRig(t, "0.2.1", nil)
	r.release = updates.Release{Tag: "v0.2.1"}

	r.app.ScheduleUpdateCheck()
	r.settle(t, 1)

	if !r.lastCheck().Equal(r.clock) {
		t.Fatalf("last check = %v", r.lastCheck())
	}
	if len(r.notified) != 0 || len(r.asked) != 0 || len(r.failed) != 0 {
		t.Fatalf("a quiet check must stay quiet: %q %q %q", r.notified, r.asked, r.failed)
	}
}

func TestStartupCheckAnnouncesANewerRelease(t *testing.T) {
	r := newUpdateRig(t, "0.2.1", nil)
	r.release = updates.Release{Tag: "v0.2.2"}

	r.app.ScheduleUpdateCheck()
	r.settle(t, 1)

	if len(r.asked) != 1 || !strings.Contains(r.asked[0], "0.2.2") {
		t.Fatalf("asked = %q", r.asked)
	}
}

func TestStartupCheckStaysSilentOnFailureAndTriesAgainNextTime(t *testing.T) {
	r := newUpdateRig(t, "0.2.1", nil)
	r.err = errors.New("offline")

	r.app.ScheduleUpdateCheck()
	r.settle(t, 1)

	if len(r.failed) != 0 || len(r.notified) != 0 || len(r.asked) != 0 {
		t.Fatalf("failed %q, notified %q, asked %q", r.failed, r.notified, r.asked)
	}
	if !r.lastCheck().IsZero() {
		t.Fatalf("last check = %v", r.lastCheck())
	}
}

func TestStartupCheckHonoursTheSwitchInSettings(t *testing.T) {
	r := newUpdateRig(t, "0.2.1", func(s *settings.Settings) { s.Updates.Check = false })
	r.release = updates.Release{Tag: "v9.9.9"}

	r.app.ScheduleUpdateCheck()
	time.Sleep(60 * time.Millisecond)
	r.app.Eng.Flush()

	if r.lookups.Load() != 0 || len(r.asked) != 0 {
		t.Fatalf("lookups %d, asked %q", r.lookups.Load(), r.asked)
	}
}

func TestStartupCheckWaitsForTheInterval(t *testing.T) {
	r := newUpdateRig(t, "0.2.1", func(s *settings.Settings) {
		s.Updates.LastCheck = time.Date(2026, 10, 12, 17, 0, 0, 0, time.UTC)
	})
	r.release = updates.Release{Tag: "v9.9.9"}

	r.app.ScheduleUpdateCheck()
	time.Sleep(60 * time.Millisecond)
	r.app.Eng.Flush()

	if r.lookups.Load() != 0 {
		t.Fatalf("lookups = %d, an hour is not enough", r.lookups.Load())
	}
}

func TestStartupCheckRunsAgainAfterTheInterval(t *testing.T) {
	r := newUpdateRig(t, "0.2.1", func(s *settings.Settings) {
		s.Updates.LastCheck = time.Date(2026, 10, 8, 17, 0, 0, 0, time.UTC)
	})
	r.release = updates.Release{Tag: "v0.2.1"}

	r.app.ScheduleUpdateCheck()
	r.settle(t, 1)

	if !r.lastCheck().Equal(r.clock) {
		t.Fatalf("last check = %v", r.lastCheck())
	}
}

func TestStartupCheckSkipsDevelopmentBuilds(t *testing.T) {
	r := newUpdateRig(t, "dev", nil)
	r.release = updates.Release{Tag: "v9.9.9"}

	r.app.ScheduleUpdateCheck()
	time.Sleep(60 * time.Millisecond)
	r.app.Eng.Flush()

	if r.lookups.Load() != 0 || len(r.asked) != 0 {
		t.Fatalf("lookups %d, asked %q", r.lookups.Load(), r.asked)
	}
}

func TestManualCheckWorksOnDevelopmentBuilds(t *testing.T) {
	r := newUpdateRig(t, "dev", nil)
	r.release = updates.Release{Tag: "v0.2.1"}

	r.app.CheckForUpdates()
	r.settle(t, 1)

	if len(r.asked) != 1 {
		t.Fatalf("asked = %q", r.asked)
	}
}

func TestCheckForUpdatesIsAMenuCommand(t *testing.T) {
	r := newUpdateRig(t, "0.2.1", nil)
	r.release = updates.Release{Tag: "v0.2.1"}
	cmd := r.app.Commands.Get(CmdCheckUpdates)

	if cmd == nil || !cmd.IsEnabled() {
		t.Fatal("the command must exist and work without an open image")
	}
	found := false
	for _, it := range r.app.menuItems() {
		if it.Text == i18n.T("cmd.checkUpdates") {
			found = true
			it.OnClick()
		}
	}
	if !found {
		t.Fatal("the menu has no item for the update check")
	}
	r.settle(t, 1)
	if len(r.notified) != 1 {
		t.Fatalf("notified = %q", r.notified)
	}
}

func TestFailureTextDistinguishesTheRateLimit(t *testing.T) {
	if updateFailureText(updates.ErrRateLimited) == updateFailureText(errors.New("x")) {
		t.Fatal("the quota answer needs its own text")
	}
}

func TestDefaultHooksAreWired(t *testing.T) {
	a, _ := goldenApp(t, 770, 504, nil)

	h := a.upd
	if h.latest == nil || h.open == nil || h.notify == nil || h.fail == nil || h.ask == nil || h.now == nil {
		t.Fatalf("hooks = %+v", h)
	}
	h.notify(i18n.T("update.title"), "x")
	h.fail("x")
	h.ask(i18n.T("update.title"), "x", func() {})
	a.Eng.Flush()
}
