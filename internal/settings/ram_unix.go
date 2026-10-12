//go:build !windows

package settings

import "os"

func readTotalRAM() uint64 {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0
	}
	defer f.Close()
	return parseMemTotal(f)
}
