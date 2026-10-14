package app

import (
	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/settings"
	uisettings "github.com/oops1/PixGo/internal/ui/settings"
	uiwindow "github.com/oops1/PixGo/internal/ui/window"
)

func (a *App) applyFormats(current settings.Settings) bool {
	reg := a.Ctl.env.Registry
	changed := false
	for _, id := range settings.OptionalFormats {
		on := current.FormatEnabled(id)
		if reg.IsEnabled(codec.ID(id)) != on {
			changed = true
			reg.SetEnabled(codec.ID(id), on)
		}
	}
	return changed
}

func (a *App) formatsChanged() {
	if a.gal != nil {
		a.gal.lister.InvalidateAll()
		if a.gal.hist != nil && a.galleryBehind() {
			a.gal.reload()
		}
	}
	a.Ctl.RescanAsync()
}

func (a *App) galleryBehind() bool {
	return a.page == uiwindow.PageGallery || a.page == uiwindow.PageSettings && a.set != nil && a.set.from == uiwindow.PageGallery
}

func (a *App) settingsChanged(old, current settings.Settings) {
	if old.Language != current.Language {
		i18n.SetLang(current.Language)
		widget.SetLanguage(i18n.Lang())
	}
	formats := a.applyFormats(current)
	a.ApplySettings()
	if formats {
		a.formatsChanged()
	}
	if old.Sort != current.Sort || old.SortDesc != current.SortDesc {
		a.Ctl.ApplySort()
		if a.gal != nil && a.page == uiwindow.PageGallery {
			a.gal.reload()
		}
	}
	a.Root.Refresh()
	if a.gal != nil {
		a.gal.refresh()
	}
	if a.set != nil {
		a.set.page.Refresh()
	}
}

func (a *App) ensureSettings() *settingsState {
	if a.set != nil {
		return a.set
	}
	s := &settingsState{a: a}
	s.page = uisettings.New(a.Eng, uisettings.Deps{
		Source:  s,
		Actions: s.actions(),
		Theme:   a.CurrentTheme,
		After:   a.Root.After,
		Now:     a.Root.Now,
	})
	s.page.Chrome = a.Root.Owns
	a.set = s
	return s
}

func (a *App) OpenSettings() {
	s := a.ensureSettings()
	if a.page == uiwindow.PageSettings {
		return
	}
	if a.fullscreen {
		a.SetFullscreen(false)
	}
	s.from = a.page
	a.page = uiwindow.PageSettings
	s.page.Refresh()
	a.Root.Refresh()
	a.Eng.SetFocus(s.page)
}

func (a *App) CloseSettings() {
	if a.page != uiwindow.PageSettings || a.set == nil {
		return
	}
	a.page = a.set.from
	a.Root.Refresh()
	if a.page == uiwindow.PageGallery && a.gal != nil {
		a.gal.refresh()
		a.Eng.SetFocus(a.gal.page)
		return
	}
	a.Root.FocusContent()
}

func (a *App) SettingsOpen() bool { return a.page == uiwindow.PageSettings }
