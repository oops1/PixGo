package platform

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

var ErrProcInfoUnavailable = errors.New("platform: process information is not available")

const (
	procStatStartField = 22
	linuxClockTicks    = 100
)

type MemoryUsage struct {
	Current uint64
	Peak    uint64
}

func parseProcStatStart(stat string) (ticks uint64, err error) {
	end := strings.LastIndexByte(stat, ')')
	if end < 0 {
		return 0, fmt.Errorf("platform: malformed /proc stat line")
	}
	fields := strings.Fields(stat[end+1:])
	idx := procStatStartField - 3
	if len(fields) <= idx {
		return 0, fmt.Errorf("platform: /proc stat line has %d fields", len(fields)+2)
	}
	return strconv.ParseUint(fields[idx], 10, 64)
}

func parseBootTime(stat string) (int64, error) {
	for _, line := range strings.Split(stat, "\n") {
		if rest, ok := strings.CutPrefix(line, "btime "); ok {
			return strconv.ParseInt(strings.TrimSpace(rest), 10, 64)
		}
	}
	return 0, fmt.Errorf("platform: btime is missing")
}

func linuxStartTime(boot int64, ticks uint64) time.Time {
	nanos := int64(ticks) * int64(time.Second) / linuxClockTicks
	return time.Unix(boot, 0).Add(time.Duration(nanos))
}

func parseStatusKB(status, key string) (uint64, bool) {
	for _, line := range strings.Split(status, "\n") {
		rest, ok := strings.CutPrefix(line, key+":")
		if !ok {
			continue
		}
		f := strings.Fields(rest)
		if len(f) == 0 {
			return 0, false
		}
		kb, err := strconv.ParseUint(f[0], 10, 64)
		if err != nil {
			return 0, false
		}
		return kb << 10, true
	}
	return 0, false
}

func parseMemoryStatus(status string) (MemoryUsage, bool) {
	cur, ok1 := parseStatusKB(status, "VmRSS")
	peak, ok2 := parseStatusKB(status, "VmHWM")
	if !ok1 && !ok2 {
		return MemoryUsage{}, false
	}
	return MemoryUsage{Current: cur, Peak: max(peak, cur)}, true
}
