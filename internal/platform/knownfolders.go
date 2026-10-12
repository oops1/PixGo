package platform

import (
	"os"
	"path"
	"path/filepath"
	"strings"
)

type osKnownFolders struct{}

func NewKnownFolders() KnownFolders { return osKnownFolders{} }

func (osKnownFolders) Folders() Folders { return KnownFolderPaths() }

var xdgDefaults = []struct {
	key, dir string
	field    func(*Folders) *string
}{
	{"DESKTOP", "Desktop", func(f *Folders) *string { return &f.Desktop }},
	{"DOCUMENTS", "Documents", func(f *Folders) *string { return &f.Documents }},
	{"DOWNLOAD", "Downloads", func(f *Folders) *string { return &f.Downloads }},
	{"PICTURES", "Pictures", func(f *Folders) *string { return &f.Pictures }},
	{"VIDEOS", "Videos", func(f *Folders) *string { return &f.Videos }},
}

func parseUserDirs(data, home string) map[string]string {
	values := map[string]string{}
	for _, line := range strings.Split(data, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		key = strings.TrimSpace(key)
		if !ok || !strings.HasPrefix(key, "XDG_") || !strings.HasSuffix(key, "_DIR") {
			continue
		}
		value = strings.TrimSpace(value)
		if !strings.HasPrefix(value, `"`) {
			continue
		}
		value = value[1:]
		var b strings.Builder
		valid, closed := true, false
		for i := 0; i < len(value) && !closed; i++ {
			switch value[i] {
			case '"':
				closed = true
			case '\\':
				if i+1 < len(value) {
					i++
					b.WriteByte(value[i])
				} else {
					valid = false
				}
			case '$':
				if strings.HasPrefix(value[i:], "$HOME/") || strings.HasPrefix(value[i:], "$HOME\"") {
					b.WriteString(home)
					i += 4
				} else {
					valid = false
				}
			case '`':
				valid = false
			default:
				b.WriteByte(value[i])
			}
		}
		if valid && closed && (path.IsAbs(b.String()) || filepath.IsAbs(b.String())) {
			values[strings.TrimSuffix(strings.TrimPrefix(key, "XDG_"), "_DIR")] = path.Clean(b.String())
		}
	}
	return values
}

func foldersFromUserDirs(home, data string) Folders {
	f := Folders{Home: home}
	values := parseUserDirs(data, home)
	for _, d := range xdgDefaults {
		p := values[d.key]
		switch {
		case p == "":
			p = filepath.Join(home, d.dir)
		case p == path.Clean(home):
			p = home
		default:
			p = filepath.FromSlash(p)
		}
		*d.field(&f) = p
	}
	return f
}

func loadUserDirsFolders(home, configHome string) Folders {
	if !filepath.IsAbs(configHome) {
		configHome = filepath.Join(home, ".config")
	}
	data, _ := os.ReadFile(filepath.Join(configHome, "user-dirs.dirs"))
	return foldersFromUserDirs(home, string(data))
}
