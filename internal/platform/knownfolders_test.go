package platform

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestParseUserDirs(t *testing.T) {
	data := `# comment
XDG_DESKTOP_DIR="$HOME/Рабочий стол"
XDG_DOWNLOAD_DIR="$HOME/Загрузки"
XDG_DOCUMENTS_DIR="/data/docs/"
XDG_PICTURES_DIR="$HOME/Pic\"tures"
XDG_VIDEOS_DIR="$HOME/"
XDG_MUSIC_DIR="$HOME/Music"
`
	got := parseUserDirs(data, "/home/u")
	want := map[string]string{
		"DESKTOP":   "/home/u/Рабочий стол",
		"DOWNLOAD":  "/home/u/Загрузки",
		"DOCUMENTS": "/data/docs",
		"PICTURES":  `/home/u/Pic"tures`,
		"VIDEOS":    "/home/u",
		"MUSIC":     "/home/u/Music",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
}

func TestParseUserDirsRejectsUnsafe(t *testing.T) {
	for _, line := range []string{
		`XDG_DESKTOP_DIR="$(touch /tmp/x)/Desktop"`,
		"XDG_DESKTOP_DIR=\"`id`/Desktop\"",
		`XDG_DESKTOP_DIR="$HOMEDIR/Desktop"`,
		`XDG_DESKTOP_DIR="${HOME}/Desktop"`,
		`XDG_DESKTOP_DIR=$HOME/Desktop`,
		`XDG_DESKTOP_DIR="relative/Desktop"`,
		`XDG_DESKTOP_DIR="/unterminated`,
		`XDG_DESKTOP_DIR="/trailing\`,
		`DESKTOP_DIR="/home/u/Desktop"`,
		`XDG_DESKTOP="/home/u/Desktop"`,
		`export XDG_DESKTOP_DIR="/home/u/Desktop"`,
	} {
		if got := parseUserDirs(line+"\n", "/home/u"); len(got) != 0 {
			t.Errorf("%q accepted: %v", line, got)
		}
	}
}

func TestFoldersFromUserDirs(t *testing.T) {
	home := "/home/u"
	data := "XDG_PICTURES_DIR=\"$HOME/Фото\"\nXDG_VIDEOS_DIR=\"$HOME/\"\n"
	f := foldersFromUserDirs(home, data)
	if f.Home != home {
		t.Fatal(f.Home)
	}
	if f.Pictures != filepath.FromSlash(home+"/Фото") {
		t.Errorf("pictures %q", f.Pictures)
	}
	if f.Videos != home {
		t.Errorf("a disabled folder falls back to home, got %q", f.Videos)
	}
	for name, got := range map[string]string{
		"Desktop": f.Desktop, "Documents": f.Documents, "Downloads": f.Downloads,
	} {
		if got != filepath.Join(home, name) {
			t.Errorf("%s default %q", name, got)
		}
	}
}

func TestLoadUserDirsFolders(t *testing.T) {
	home := t.TempDir()
	cfg := filepath.Join(home, "cfg")
	os.MkdirAll(cfg, 0o755)
	os.WriteFile(filepath.Join(cfg, "user-dirs.dirs"), []byte("XDG_DOWNLOAD_DIR=\"$HOME/dl\"\n"), 0o644)
	if f := loadUserDirsFolders(home, cfg); f.Downloads != filepath.Join(home, "dl") {
		t.Errorf("explicit config home: %q", f.Downloads)
	}
	if f := loadUserDirsFolders(home, "relative"); f.Downloads != filepath.Join(home, "Downloads") {
		t.Errorf("relative config home must be ignored: %q", f.Downloads)
	}
	os.MkdirAll(filepath.Join(home, ".config"), 0o755)
	os.WriteFile(filepath.Join(home, ".config", "user-dirs.dirs"), []byte("XDG_DOWNLOAD_DIR=\"$HOME/dl2\"\n"), 0o644)
	if f := loadUserDirsFolders(home, ""); f.Downloads != filepath.Join(home, "dl2") {
		t.Errorf("default config home: %q", f.Downloads)
	}
}
