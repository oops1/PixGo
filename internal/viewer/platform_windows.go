//go:build windows

package viewer

import (
	"io/fs"
	"syscall"
)

const foldPaths = true

func isHidden(info fs.FileInfo) bool {
	attrs, ok := info.Sys().(*syscall.Win32FileAttributeData)
	if !ok {
		return false
	}
	return attrs.FileAttributes&(syscall.FILE_ATTRIBUTE_HIDDEN|syscall.FILE_ATTRIBUTE_SYSTEM) != 0
}

func isFileMode(mode fs.FileMode) bool {
	if mode.IsRegular() {
		return true
	}
	const notFile = fs.ModeDir | fs.ModeSymlink | fs.ModeDevice | fs.ModeCharDevice | fs.ModeNamedPipe | fs.ModeSocket
	return mode&notFile == 0
}
