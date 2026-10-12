package platform

import "errors"

var ErrNoWindow = errors.New("platform: the process has no window to raise")

type windowCandidate struct {
	handle  uintptr
	visible bool
	owned   bool
	area    int
}

func pickMainWindow(candidates []windowCandidate) (uintptr, bool) {
	var best windowCandidate
	for _, c := range candidates {
		if !c.visible || c.owned || c.area <= 0 {
			continue
		}
		if best.handle == 0 || c.area > best.area {
			best = c
		}
	}
	return best.handle, best.handle != 0
}
