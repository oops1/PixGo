//go:build windows

package imaging

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

const (
	shareAll       = syscall.FILE_SHARE_READ | syscall.FILE_SHARE_WRITE | syscall.FILE_SHARE_DELETE
	longPathLimit  = 248
	extendedPrefix = `\\?\`
	uncPrefix      = `\\`
	uncExtended    = `\\?\UNC\`
)

func extendedPath(path string) string {
	if len(path) < longPathLimit || strings.HasPrefix(path, extendedPrefix) || !filepath.IsAbs(path) {
		return path
	}
	path = filepath.Clean(path)
	if strings.HasPrefix(path, uncPrefix) {
		return uncExtended + path[len(uncPrefix):]
	}
	return extendedPrefix + path
}

func openShared(path string) (*os.File, error) {
	p, err := syscall.UTF16PtrFromString(extendedPath(path))
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	h, err := syscall.CreateFile(p, syscall.GENERIC_READ, shareAll, nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	return os.NewFile(uintptr(h), path), nil
}
