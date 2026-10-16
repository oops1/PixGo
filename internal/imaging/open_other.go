//go:build !windows

package imaging

import "os"

func openShared(path string) (*os.File, error) { return os.Open(path) }
