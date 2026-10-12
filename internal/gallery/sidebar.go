package gallery

import (
	"path/filepath"

	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/platform"
	"github.com/oops1/PixGo/internal/viewer"
)

type Entry struct {
	Label    string
	Icon     string
	Loc      Location
	Header   bool
	Disabled bool
	Expander bool
	Selected bool
	Indent   int
}

func Sidebar(f platform.Folders, volumes []platform.Volume, current Location) []Entry {
	out := []Entry{
		{Label: i18n.T("gallery.all"), Icon: "images", Loc: Location{Kind: KindAll}},
		{Label: i18n.T("gallery.recent"), Icon: "recent", Loc: Location{Kind: KindRecent}},
		{Label: i18n.T("gallery.favorites"), Icon: "heart-small", Loc: Location{Kind: KindFavorites}},
	}
	out = appendSection(out, "gallery.folders", knownFolderEntries(f))
	out = appendSection(out, "gallery.computer", volumeEntries(volumes, f.Home))
	out = append(out, Entry{Label: i18n.T("gallery.network"), Icon: "network", Disabled: true})
	markSelected(out, current)
	return out
}

func appendSection(out []Entry, titleKey string, entries []Entry) []Entry {
	if len(entries) == 0 {
		return out
	}
	out = append(out, Entry{Label: i18n.T(titleKey), Header: true})
	return append(out, entries...)
}

func knownFolderEntries(f platform.Folders) []Entry {
	known := []struct{ key, icon, path string }{
		{"gallery.desktop", "desktop", f.Desktop},
		{"gallery.pictures", "pictures", f.Pictures},
		{"gallery.videos", "video", f.Videos},
		{"gallery.documents", "documents", f.Documents},
		{"gallery.downloads", "download", f.Downloads},
	}
	var out []Entry
	for _, k := range known {
		if k.path == "" || (f.Home != "" && viewer.SamePath(k.path, f.Home)) {
			continue
		}
		out = append(out, Entry{Label: i18n.T(k.key), Icon: k.icon, Loc: Folder(k.path)})
	}
	return out
}

func volumeEntries(volumes []platform.Volume, home string) []Entry {
	system := systemVolume(volumes, home)
	var out []Entry
	for i, v := range volumes {
		if v.Path == "" {
			continue
		}
		out = append(out, Entry{Label: volumeLabel(v), Icon: volumeIcon(v, i == system), Loc: Folder(v.Path)})
	}
	return out
}

func systemVolume(volumes []platform.Volume, home string) int {
	best, bestLen := -1, -1
	for i, v := range volumes {
		if v.Kind == platform.VolumeRoot {
			return i
		}
		if v.Path == "" || !within(home, v.Path) {
			continue
		}
		if n := len(filepath.Clean(v.Path)); n > bestLen {
			best, bestLen = i, n
		}
	}
	return best
}

func volumeLabel(v platform.Volume) string {
	switch {
	case v.Kind == platform.VolumeRoot:
		return i18n.T("gallery.fileSystem")
	case v.Name != "":
		return v.Name
	case v.Label != "":
		return v.Label
	}
	return v.Path
}

func volumeIcon(v platform.Volume, system bool) string {
	switch {
	case system:
		return "drive-system"
	case v.Kind == platform.VolumeNetwork:
		return "network"
	}
	return "drive"
}

func markSelected(entries []Entry, current Location) {
	best, bestLen := -1, -1
	for i, e := range entries {
		if e.Header || e.Disabled {
			continue
		}
		if current.Kind != KindFolder {
			if e.Loc.Kind == current.Kind {
				best = i
			}
			continue
		}
		if e.Loc.Kind != KindFolder || !within(current.Path, e.Loc.Path) {
			continue
		}
		if n := len(filepath.Clean(e.Loc.Path)); n > bestLen {
			best, bestLen = i, n
		}
	}
	if best < 0 {
		return
	}
	entries[best].Selected = true
	entries[best].Expander = current.Kind == KindFolder && !entries[best].Loc.Equal(current)
}
