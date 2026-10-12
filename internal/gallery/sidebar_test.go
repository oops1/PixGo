package gallery

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/oops1/PixGo/internal/platform"
)

type sidebarFixture struct {
	root    string
	home    string
	folders platform.Folders
	volumes []platform.Volume
}

func newSidebarFixture(t *testing.T) sidebarFixture {
	t.Helper()
	root := t.TempDir()
	home := filepath.Join(root, "c", "Users", "me")
	return sidebarFixture{
		root: root,
		home: home,
		folders: platform.Folders{
			Home:      home,
			Desktop:   filepath.Join(home, "Desktop"),
			Pictures:  filepath.Join(home, "Pictures"),
			Videos:    filepath.Join(home, "Videos"),
			Documents: filepath.Join(home, "Documents"),
			Downloads: filepath.Join(home, "Downloads"),
		},
		volumes: []platform.Volume{
			{Path: filepath.Join(root, "c"), Name: "Windows (C:)", Label: "Windows", Kind: platform.VolumeFixed},
			{Path: filepath.Join(root, "d"), Name: "Data (D:)", Label: "Data", Kind: platform.VolumeFixed},
		},
	}
}

func labels(entries []Entry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Label
	}
	return out
}

func icons(entries []Entry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Icon
	}
	return out
}

func selectedLabels(entries []Entry) []string {
	var out []string
	for _, e := range entries {
		if e.Selected {
			out = append(out, e.Label)
		}
	}
	return out
}

func TestSidebarFollowsTheMockupLayout(t *testing.T) {
	setLang(t, "ru")
	fx := newSidebarFixture(t)
	got := Sidebar(fx.folders, fx.volumes, Location{Kind: KindAll})
	wantLabels := []string{
		"Все изображения", "Недавние", "Избранные",
		"Папки", "Рабочий стол", "Изображения", "Видео", "Документы", "Загрузки",
		"Этот компьютер", "Windows (C:)", "Data (D:)",
		"Сеть",
	}
	if !slices.Equal(labels(got), wantLabels) {
		t.Fatalf("labels %v, want %v", labels(got), wantLabels)
	}
	wantIcons := []string{
		"images", "recent", "heart-small",
		"", "desktop", "pictures", "video", "documents", "download",
		"", "drive-system", "drive",
		"network",
	}
	if !slices.Equal(icons(got), wantIcons) {
		t.Fatalf("icons %v, want %v", icons(got), wantIcons)
	}
	for i, e := range got {
		wantHeader := e.Label == "Папки" || e.Label == "Этот компьютер"
		if e.Header != wantHeader {
			t.Errorf("entry %d %q: Header = %v", i, e.Label, e.Header)
		}
		if e.Disabled != (e.Label == "Сеть") {
			t.Errorf("entry %d %q: Disabled = %v", i, e.Label, e.Disabled)
		}
		if e.Indent != 0 {
			t.Errorf("entry %d %q: Indent = %d", i, e.Label, e.Indent)
		}
		if (e.Header || e.Disabled) && !e.Loc.IsZero() {
			t.Errorf("entry %d %q: non-navigable entry has a location %+v", i, e.Label, e.Loc)
		}
	}
}

func TestSidebarLocationsPointAtTheRightPlaces(t *testing.T) {
	setLang(t, "ru")
	fx := newSidebarFixture(t)
	got := Sidebar(fx.folders, fx.volumes, Location{})
	want := map[string]Location{
		"Все изображения": {Kind: KindAll},
		"Недавние":        {Kind: KindRecent},
		"Избранные":       {Kind: KindFavorites},
		"Рабочий стол":    Folder(fx.folders.Desktop),
		"Изображения":     Folder(fx.folders.Pictures),
		"Видео":           Folder(fx.folders.Videos),
		"Документы":       Folder(fx.folders.Documents),
		"Загрузки":        Folder(fx.folders.Downloads),
		"Windows (C:)":    Folder(fx.volumes[0].Path),
		"Data (D:)":       Folder(fx.volumes[1].Path),
	}
	for _, e := range got {
		if loc, ok := want[e.Label]; ok && e.Loc != loc {
			t.Errorf("%q points at %+v, want %+v", e.Label, e.Loc, loc)
		}
	}
}

func TestSidebarIsLocalized(t *testing.T) {
	setLang(t, "en")
	fx := newSidebarFixture(t)
	got := Sidebar(fx.folders, fx.volumes, Location{})
	want := []string{
		"All images", "Recent", "Favorites",
		"Folders", "Desktop", "Pictures", "Videos", "Documents", "Downloads",
		"This PC", "Windows (C:)", "Data (D:)",
		"Network",
	}
	if !slices.Equal(labels(got), want) {
		t.Fatalf("labels %v, want %v", labels(got), want)
	}
}

func TestSidebarMarksTheCurrentPlace(t *testing.T) {
	setLang(t, "ru")
	fx := newSidebarFixture(t)
	cases := []struct {
		name         string
		current      Location
		wantSelected []string
		wantExpander bool
	}{
		{"all images", Location{Kind: KindAll}, []string{"Все изображения"}, false},
		{"recent", Location{Kind: KindRecent}, []string{"Недавние"}, false},
		{"favorites", Location{Kind: KindFavorites}, []string{"Избранные"}, false},
		{"known folder itself", Folder(fx.folders.Pictures), []string{"Изображения"}, false},
		{"inside a known folder", Folder(filepath.Join(fx.folders.Pictures, "Trip", "Day 1")), []string{"Изображения"}, true},
		{"the volume itself", Folder(fx.volumes[1].Path), []string{"Data (D:)"}, false},
		{"inside a volume as in the mockup", Folder(filepath.Join(fx.volumes[1].Path, "Photos", "Nature")), []string{"Data (D:)"}, true},
		{"inside the system volume outside known folders", Folder(filepath.Join(fx.root, "c", "Games")), []string{"Windows (C:)"}, true},
		{"home folder falls back to its volume", Folder(fx.home), []string{"Windows (C:)"}, true},
		{"unrelated place", Folder(filepath.Join(fx.root, "elsewhere")), nil, false},
		{"zero location", Location{}, nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Sidebar(fx.folders, fx.volumes, tc.current)
			if sel := selectedLabels(got); !slices.Equal(sel, tc.wantSelected) {
				t.Fatalf("selected %v, want %v", sel, tc.wantSelected)
			}
			for _, e := range got {
				if e.Expander != (e.Selected && tc.wantExpander) {
					t.Errorf("%q: Expander = %v (selected %v, want expander %v)", e.Label, e.Expander, e.Selected, tc.wantExpander)
				}
			}
		})
	}
}

func TestSidebarNeverSelectsHeadersOrTheDisabledNetwork(t *testing.T) {
	setLang(t, "ru")
	fx := newSidebarFixture(t)
	for _, cur := range []Location{{}, Folder(""), {Kind: KindAll}, Folder(fx.home)} {
		for _, e := range Sidebar(fx.folders, fx.volumes, cur) {
			if (e.Header || e.Disabled) && (e.Selected || e.Expander) {
				t.Fatalf("%q is selected for %+v", e.Label, cur)
			}
		}
	}
}

func TestSidebarSkipsMissingAndHomeKnownFolders(t *testing.T) {
	setLang(t, "ru")
	fx := newSidebarFixture(t)
	cases := []struct {
		name    string
		folders platform.Folders
		want    []string
	}{
		{"no known folders hides the section", platform.Folders{}, []string{"Все изображения", "Недавние", "Избранные", "Этот компьютер", "Windows (C:)", "Data (D:)", "Сеть"}},
		{"empty paths are skipped", platform.Folders{Pictures: fx.folders.Pictures, Downloads: fx.folders.Downloads}, []string{"Все изображения", "Недавние", "Избранные", "Папки", "Изображения", "Загрузки", "Этот компьютер", "Windows (C:)", "Data (D:)", "Сеть"}},
		{"folders equal to home mean unset", platform.Folders{Home: fx.home, Desktop: fx.home, Videos: fx.home, Pictures: fx.folders.Pictures}, []string{"Все изображения", "Недавние", "Избранные", "Папки", "Изображения", "Этот компьютер", "Windows (C:)", "Data (D:)", "Сеть"}},
		{"home is optional", platform.Folders{Desktop: fx.folders.Desktop}, []string{"Все изображения", "Недавние", "Избранные", "Папки", "Рабочий стол", "Этот компьютер", "Windows (C:)", "Data (D:)", "Сеть"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := labels(Sidebar(tc.folders, fx.volumes, Location{})); !slices.Equal(got, tc.want) {
				t.Fatalf("labels %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSidebarWithoutVolumesDropsTheComputerSection(t *testing.T) {
	setLang(t, "ru")
	fx := newSidebarFixture(t)
	got := labels(Sidebar(fx.folders, nil, Location{}))
	if slices.Contains(got, "Этот компьютер") || got[len(got)-1] != "Сеть" {
		t.Fatalf("labels %v", got)
	}
	got = labels(Sidebar(fx.folders, []platform.Volume{{Path: "", Name: "ghost"}}, Location{}))
	if slices.Contains(got, "ghost") {
		t.Fatalf("a volume without a path must be skipped: %v", got)
	}
}

func TestSidebarVolumeLabelsAndIcons(t *testing.T) {
	setLang(t, "ru")
	root := t.TempDir()
	home := filepath.Join(root, "sys", "home")
	volumes := []platform.Volume{
		{Path: filepath.Join(root, "sys"), Name: "System", Kind: platform.VolumeFixed},
		{Path: filepath.Join(root, "stick"), Name: "", Label: "Stick", Kind: platform.VolumeRemovable},
		{Path: filepath.Join(root, "bare"), Kind: platform.VolumeFixed},
		{Path: filepath.Join(root, "nas"), Name: "NAS", Kind: platform.VolumeNetwork},
		{Path: filepath.Join(root, "dvd"), Name: "DVD", Kind: platform.VolumeOptical},
	}
	got := Sidebar(platform.Folders{Home: home}, volumes, Location{})
	computer := got[slices.IndexFunc(got, func(e Entry) bool { return e.Header })+1 : len(got)-1]
	wantLabels := []string{"System", "Stick", filepath.Join(root, "bare"), "NAS", "DVD"}
	wantIcons := []string{"drive-system", "drive", "drive", "network", "drive"}
	if !slices.Equal(labels(computer), wantLabels) || !slices.Equal(icons(computer), wantIcons) {
		t.Fatalf("labels %v, icons %v", labels(computer), icons(computer))
	}
}

func TestSidebarSystemVolumeChoice(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "c", "Users", "me")
	cases := []struct {
		name    string
		home    string
		volumes []platform.Volume
		want    int
	}{
		{"no volumes", home, nil, -1},
		{"unix root wins", home, []platform.Volume{{Path: filepath.Join(root, "x")}, {Path: "/", Kind: platform.VolumeRoot}}, 1},
		{"volume that contains home", home, []platform.Volume{{Path: filepath.Join(root, "d")}, {Path: filepath.Join(root, "c")}}, 1},
		{"nested mounts pick the deepest", home, []platform.Volume{{Path: filepath.Join(root, "c")}, {Path: filepath.Join(root, "c", "Users")}}, 1},
		{"home outside every volume", home, []platform.Volume{{Path: filepath.Join(root, "d")}}, -1},
		{"unknown home", "", []platform.Volume{{Path: filepath.Join(root, "c")}}, -1},
		{"volume without a path is ignored", home, []platform.Volume{{}, {Path: filepath.Join(root, "c")}}, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := systemVolume(tc.volumes, tc.home); got != tc.want {
				t.Fatalf("systemVolume = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestSidebarFileSystemRootIsNamedAndMarkedAsSystem(t *testing.T) {
	setLang(t, "ru")
	root := t.TempDir()
	volumes := []platform.Volume{
		{Path: "/", Name: "/", Kind: platform.VolumeRoot},
		{Path: filepath.Join(root, "usb"), Name: "usb", Label: "usb", Kind: platform.VolumeRemovable},
	}
	got := Sidebar(platform.Folders{}, volumes, Location{})
	if got[4].Label != "Файловая система" || got[4].Icon != "drive-system" || got[5].Icon != "drive" {
		t.Fatalf("entries %v / %v", labels(got), icons(got))
	}
}
