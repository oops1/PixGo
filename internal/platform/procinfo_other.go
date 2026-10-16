//go:build !windows && !linux

package platform

import "time"

func ProcessStart() (time.Time, error) { return time.Time{}, ErrProcInfoUnavailable }

func ProcessMemory() (MemoryUsage, error) { return MemoryUsage{}, ErrProcInfoUnavailable }
