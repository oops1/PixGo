package app

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/oops1/headless-gui/v3/widget"
	"github.com/oops1/headless-gui/v3/window"

	"github.com/oops1/PixGo/internal/buildinfo"
	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/settings"
	"github.com/oops1/PixGo/internal/updates"
)

type updateHooks struct {
	latest func(ctx contextT) (updates.Release, error)
	open   func(rawURL string)
	notify func(title, message string)
	fail   func(message string)
	ask    func(title, message string, yes func())
	now    func() time.Time
}

func (a *App) initUpdates() {
	source := updates.NewSource("PixGo/" + buildinfo.String())
	a.upd = updateHooks{
		latest: source.Latest,
		open: func(rawURL string) {
			go func() {
				if err := window.OpenURL(rawURL); err != nil {
					slog.Warn("open release page", "err", err)
				}
			}()
		},
		notify: func(title, message string) { widget.NewMessageBox(a.Eng).ShowInfo(title, message) },
		fail:   a.errorBox,
		ask:    a.confirm,
		now:    time.Now,
	}
}

func (a *App) CheckForUpdates() { a.checkUpdates(true) }

func (a *App) ScheduleUpdateCheck() {
	st := a.Settings.Get().Updates
	if settings.UpdateCheckDisabled() || !st.Check || !updates.Comparable(buildinfo.Version) || !updates.Due(st.LastCheck, a.upd.now()) {
		return
	}
	a.checkUpdates(false)
}

func (a *App) checkUpdates(manual bool) {
	if settings.UpdateCheckDisabled() {
		if manual {
			a.upd.notify(i18n.T("update.title"), i18n.T("update.disabled"))
		}
		return
	}
	current := buildinfo.String()
	a.Ctl.Async("updates", func(ctx contextT) (any, error) {
		ctx, cancel := context.WithTimeout(ctx, updates.Timeout)
		defer cancel()
		return a.upd.latest(ctx)
	}, func(v any, err error) {
		release, _ := v.(updates.Release)
		a.reportUpdate(current, release, err, manual)
	})
}

func (a *App) reportUpdate(current string, latest updates.Release, err error, manual bool) {
	if err != nil {
		slog.Warn("update check", "err", err)
		if manual {
			a.upd.fail(updateFailureText(err))
		}
		return
	}
	checked := a.upd.now()
	a.Settings.Update(func(s *settings.Settings) { s.Updates.LastCheck = checked })
	title := i18n.T("update.title")
	if !updates.Newer(current, latest.Tag) {
		if manual {
			a.upd.notify(title, i18n.T("update.upToDate", current))
		}
		return
	}
	page := latest.Page
	if page == "" {
		page = updates.PageURL
	}
	a.upd.ask(title, i18n.T("update.available", latest.Version(), current), func() { a.upd.open(page) })
}

func updateFailureText(err error) string {
	if errors.Is(err, updates.ErrRateLimited) {
		return i18n.T("update.rateLimited")
	}
	return i18n.T("update.failed")
}
