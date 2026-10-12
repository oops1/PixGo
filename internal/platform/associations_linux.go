package platform

import (
	"os"
	"path/filepath"

	"github.com/oops1/PixGo/internal/fsutil"
)

type linuxAssociations struct{}

func NewAssociations() Associations { return linuxAssociations{} }

func (linuxAssociations) OpenDefaultsSettings() error { return ErrUnsupported }

func mimeappsPath() string {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "mimeapps.list")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "mimeapps.list")
}

func (linuxAssociations) SetAsDefault() error {
	path := mimeappsPath()
	if path == "" {
		return ErrUnavailable
	}
	old, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	updated := SetDefaultInMimeapps(string(old), DesktopID(), ImageMimeTypes)
	if updated == string(old) {
		return nil
	}
	return fsutil.WriteAtomic(path, []byte(updated), 0o644)
}
