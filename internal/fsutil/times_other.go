//go:build !windows && !linux

package fsutil

import "time"

func CreationTime(path string) (time.Time, bool) { return time.Time{}, false }
