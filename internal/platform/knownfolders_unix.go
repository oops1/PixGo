//go:build !windows

package platform

import "os"

func KnownFolderPaths() Folders {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = os.TempDir()
	}
	return loadUserDirsFolders(home, os.Getenv("XDG_CONFIG_HOME"))
}
