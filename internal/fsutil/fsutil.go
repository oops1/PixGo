package fsutil

import (
	"os"
	"path/filepath"
	"runtime"
)

const appWindows = "PixGo"

const appUnix = "pixgo"

func appName() string {
	if runtime.GOOS == "windows" {
		return appWindows
	}
	return appUnix
}

func WriteAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	ok := false
	defer func() {
		if !ok {
			f.Close()
			os.Remove(tmp)
		}
	}()
	if _, err = f.Write(data); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if runtime.GOOS != "windows" {
		if err = os.Chmod(tmp, perm); err != nil {
			return err
		}
	}
	if err = os.Rename(tmp, path); err != nil {
		return err
	}
	ok = true
	syncDir(dir)
	return nil
}

func syncDir(dir string) {
	if runtime.GOOS == "windows" {
		return
	}
	if d, err := os.Open(dir); err == nil {
		d.Sync()
		d.Close()
	}
}

func ConfigDir() string {
	if d := os.Getenv("PIXGO_CONFIG_DIR"); d != "" {
		return d
	}
	if runtime.GOOS == "windows" {
		if d := os.Getenv("APPDATA"); d != "" {
			return filepath.Join(d, appName())
		}
	}
	if d := absEnv("XDG_CONFIG_HOME"); d != "" {
		return filepath.Join(d, appName())
	}
	if d, err := os.UserConfigDir(); err == nil {
		return filepath.Join(d, appName())
	}
	return filepath.Join(os.TempDir(), appName())
}

func StateDir() string {
	if d := os.Getenv("PIXGO_STATE_DIR"); d != "" {
		return d
	}
	if runtime.GOOS == "windows" {
		if d := os.Getenv("LOCALAPPDATA"); d != "" {
			return filepath.Join(d, appName())
		}
	}
	if d := absEnv("XDG_STATE_HOME"); d != "" {
		return filepath.Join(d, appName())
	}
	if h, err := os.UserHomeDir(); err == nil {
		return filepath.Join(h, ".local", "state", appName())
	}
	return filepath.Join(os.TempDir(), appName())
}

func CacheDir() string {
	if d := os.Getenv("PIXGO_CACHE_DIR"); d != "" {
		return d
	}
	if runtime.GOOS == "windows" {
		if d := os.Getenv("LOCALAPPDATA"); d != "" {
			return filepath.Join(d, appName(), "cache")
		}
	}
	if d := absEnv("XDG_CACHE_HOME"); d != "" {
		return filepath.Join(d, appName())
	}
	if h, err := os.UserHomeDir(); err == nil {
		return filepath.Join(h, ".cache", appName())
	}
	return filepath.Join(os.TempDir(), appName(), "cache")
}

func absEnv(name string) string {
	d := os.Getenv(name)
	if d != "" && filepath.IsAbs(d) {
		return d
	}
	return ""
}
