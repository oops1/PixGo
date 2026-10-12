//go:build !windows

package viewer

import "io/fs"

const foldPaths = false

func isHidden(info fs.FileInfo) bool { return false }

func isFileMode(mode fs.FileMode) bool { return mode.IsRegular() }
