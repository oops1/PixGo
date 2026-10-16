//go:build linux

package platform

import (
	"os"
	"time"
)

func ProcessStart() (time.Time, error) {
	stat, err := os.ReadFile("/proc/self/stat")
	if err != nil {
		return time.Time{}, err
	}
	ticks, err := parseProcStatStart(string(stat))
	if err != nil {
		return time.Time{}, err
	}
	system, err := os.ReadFile("/proc/stat")
	if err != nil {
		return time.Time{}, err
	}
	boot, err := parseBootTime(string(system))
	if err != nil {
		return time.Time{}, err
	}
	return linuxStartTime(boot, ticks), nil
}

func ProcessMemory() (MemoryUsage, error) {
	status, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return MemoryUsage{}, err
	}
	usage, ok := parseMemoryStatus(string(status))
	if !ok {
		return MemoryUsage{}, ErrProcInfoUnavailable
	}
	return usage, nil
}
