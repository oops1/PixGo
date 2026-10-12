package settings

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

const (
	minCacheMB      = 256
	maxCacheMB      = 8192
	capCacheMB      = 1024
	unknownRAMCache = 512
)

var (
	ramOnce  sync.Once
	ramBytes uint64
)

func totalRAM() uint64 {
	ramOnce.Do(func() { ramBytes = readTotalRAM() })
	return ramBytes
}

func CacheMBFor(ram uint64) int {
	if ram == 0 {
		return unknownRAMCache
	}
	mb := int(min(ram/4>>20, uint64(capCacheMB)))
	return min(max(mb, minCacheMB), maxCacheMB)
}

func DefaultCacheMB() int { return CacheMBFor(totalRAM()) }

func parseMemTotal(r io.Reader) uint64 {
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		rest, ok := strings.CutPrefix(sc.Text(), "MemTotal:")
		if !ok {
			continue
		}
		f := strings.Fields(rest)
		if len(f) == 0 {
			return 0
		}
		kb, err := strconv.ParseUint(f[0], 10, 64)
		if err != nil {
			return 0
		}
		return kb << 10
	}
	return 0
}

func InWinLine() bool { return inWinLine(os.Environ(), os.UserHomeDir) }

func inWinLine(environ []string, home func() (string, error)) bool {
	for _, kv := range environ {
		if strings.HasPrefix(kv, "WINLINE_") {
			return true
		}
	}
	h, err := home()
	if err != nil || h == "" {
		return false
	}
	fi, err := os.Stat(filepath.Join(h, ".winline", "theme"))
	return err == nil && fi.Mode().IsRegular()
}

func DefaultAnimationFPS() int {
	if InWinLine() {
		return 10
	}
	return 30
}
