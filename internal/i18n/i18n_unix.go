//go:build !windows

package i18n

import (
	"os"
	"path/filepath"
)

const sessionChoiceLimit = 4096

func systemLanguages() []string {
	if tag := sessionLanguage(os.Getenv, readSessionChoice); tag != "" {
		return []string{tag}
	}
	if tag := envLanguage(os.Getenv); tag != "" {
		return []string{tag}
	}
	return nil
}

func readSessionChoice() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	f, err := os.Open(filepath.Join(home, ".winline", "language"))
	if err != nil {
		return ""
	}
	defer f.Close()
	buf := make([]byte, sessionChoiceLimit)
	n, _ := f.Read(buf)
	return string(buf[:n])
}
