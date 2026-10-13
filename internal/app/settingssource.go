package app

import (
	"errors"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/buildinfo"
	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/platform"
	"github.com/oops1/PixGo/internal/settings"
	"github.com/oops1/PixGo/internal/thumbs"
	uisettings "github.com/oops1/PixGo/internal/ui/settings"
	uiwindow "github.com/oops1/PixGo/internal/ui/window"
)

const repositoryURL = "https://github.com/oops1/PixGo"

type settingsState struct {
	a    *App
	page *uisettings.Page
	from uiwindow.Page
}

func (s *settingsState) Values() settings.Settings { return s.a.Settings.Get() }

func (s *settingsState) Languages() []uisettings.Language {
	out := []uisettings.Language{{ID: settings.LanguageSystem, Name: i18n.T("settings.language.system", systemLanguageName())}}
	for _, id := range i18n.Supported() {
		out = append(out, uisettings.Language{ID: id, Name: i18n.T("settings.language." + id)})
	}
	return out
}

func systemLanguageName() string {
	base := strings.ToLower(strings.SplitN(i18n.SystemLanguage(), "-", 2)[0])
	for _, id := range i18n.Supported() {
		if id == base {
			return i18n.T("settings.language." + id)
		}
	}
	return i18n.T("settings.language.en")
}

func (s *settingsState) Formats() []uisettings.Format {
	decoders := s.a.Ctl.env.Registry.Decoders()
	out := make([]uisettings.Format, 0, len(decoders))
	for _, d := range decoders {
		exts := make([]string, 0, len(d.Extensions()))
		for _, e := range d.Extensions() {
			exts = append(exts, strings.ToUpper(strings.TrimPrefix(e, ".")))
		}
		out = append(out, uisettings.Format{ID: string(d.ID()), Name: d.Name(), Extensions: exts, Available: true, Enabled: true})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (s *settingsState) About() uisettings.About {
	return uisettings.About{
		Version:    buildinfo.Version,
		Revision:   buildinfo.Revision(),
		Engine:     buildinfo.Engine(),
		Go:         runtime.Version(),
		Platform:   runtime.GOOS + "/" + runtime.GOARCH,
		Repository: repositoryURL,
		License:    i18n.T("settings.about.license"),
		Components: []uisettings.Component{
			{Name: "headless-gui", License: "MIT", URL: "https://github.com/oops1/headless-gui"},
			{Name: "Open Sans", License: "SIL OFL 1.1", URL: "https://github.com/googlefonts/opensans"},
			{Name: "golang.org/x/image", License: "BSD-3-Clause", URL: "https://pkg.go.dev/golang.org/x/image"},
			{Name: "golang.org/x/sys", License: "BSD-3-Clause", URL: "https://pkg.go.dev/golang.org/x/sys"},
			{Name: "golang.org/x/text", License: "BSD-3-Clause", URL: "https://pkg.go.dev/golang.org/x/text"},
			{Name: "go-text/typesetting", License: "Unlicense / BSD-3-Clause", URL: "https://github.com/go-text/typesetting"},
		},
	}
}

func (s *settingsState) ThumbCacheBytes() int64 {
	return thumbs.DirectoryUsage(thumbs.DefaultDiskDir())
}

func (s *settingsState) LastUpdateCheck() time.Time {
	return s.a.Settings.Get().Updates.LastCheck
}

func (s *settingsState) actions() uisettings.Actions {
	a := s.a
	return uisettings.Actions{
		Update: func(change func(*settings.Settings)) {
			a.Settings.Update(change)
			s.page.Refresh()
		},
		Back:            a.CloseSettings,
		ClearThumbCache: s.clearThumbCache,
		OpenDefaultApps: s.openDefaultApps,
		CheckUpdates: func() {
			a.CheckForUpdates()
			s.page.Refresh()
		},
		OpenURL: func(url string) { a.upd.open(url) },
		ShowMenu: func(items []widget.MenuItem, x, y int) {
			if a.menu == nil || len(items) == 0 {
				return
			}
			a.menu.SetItems(items)
			a.menu.Show(x, y)
		},
	}
}

func (s *settingsState) clearThumbCache() {
	a := s.a
	a.Ctl.Async("thumbs-clear", func(ctx contextT) (any, error) {
		return nil, thumbs.ClearDirectory(thumbs.DefaultDiskDir())
	}, func(_ any, err error) {
		if err != nil {
			a.errorBox(i18n.T("error.generic"))
		} else {
			a.Root.Toast(i18n.T("toast.thumbCacheCleared"))
		}
		s.page.Refresh()
	})
}

func (s *settingsState) openDefaultApps() {
	a := s.a
	if a.Platform == nil || a.Platform.Associations == nil {
		return
	}
	assoc := a.Platform.Associations
	go func() {
		err := assoc.OpenDefaultsSettings()
		if errors.Is(err, platform.ErrUnsupported) {
			err = assoc.SetAsDefault()
			if err == nil {
				a.Eng.Post(func() { a.Root.Toast(i18n.T("toast.defaultViewerSet")) })
				return
			}
		}
		if err != nil && !errors.Is(err, platform.ErrUnsupported) {
			a.Eng.Post(func() { a.errorBox(i18n.T("error.generic")) })
		}
	}()
}
