//go:build !windows

package i18n

import "os"

func systemLanguage() string { return envLanguage(os.Getenv) }
