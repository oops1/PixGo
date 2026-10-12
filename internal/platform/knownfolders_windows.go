//go:build windows

package platform

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

func KnownFolderPaths() Folders {
	home, err := windows.KnownFolderPath(windows.FOLDERID_Profile, windows.KF_FLAG_DONT_VERIFY)
	if err != nil || home == "" {
		if home, err = os.UserHomeDir(); err != nil || home == "" {
			home = os.TempDir()
		}
	}
	f := Folders{Home: home}
	for _, d := range []struct {
		id    *windows.KNOWNFOLDERID
		field *string
		dir   string
	}{
		{windows.FOLDERID_Desktop, &f.Desktop, "Desktop"},
		{windows.FOLDERID_Documents, &f.Documents, "Documents"},
		{windows.FOLDERID_Downloads, &f.Downloads, "Downloads"},
		{windows.FOLDERID_Pictures, &f.Pictures, "Pictures"},
		{windows.FOLDERID_Videos, &f.Videos, "Videos"},
	} {
		p, err := windows.KnownFolderPath(d.id, windows.KF_FLAG_DONT_VERIFY)
		if err != nil || p == "" {
			p = filepath.Join(home, d.dir)
		}
		*d.field = p
	}
	return f
}
