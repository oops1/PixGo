//go:build windows

package platform

import (
	"errors"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	swRestore = 9
	gwOwner   = 4
)

var (
	errForeground = errors.New("platform: windows kept the window in the background")

	enumOnce     sync.Once
	enumCallback uintptr
	enumMu       sync.Mutex
	enumFound    []windowCandidate
)

type winRect struct{ left, top, right, bottom int32 }

func ownWindowCallback(hwnd, _ uintptr) uintptr {
	var pid uint32
	_, _ = windows.GetWindowThreadProcessId(windows.HWND(hwnd), &pid)
	if pid != windows.GetCurrentProcessId() {
		return 1
	}
	var r winRect
	procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	owner, _, _ := procGetWindow.Call(hwnd, gwOwner)
	enumFound = append(enumFound, windowCandidate{
		handle:  hwnd,
		visible: windows.IsWindowVisible(windows.HWND(hwnd)),
		owned:   owner != 0,
		area:    int(r.right-r.left) * int(r.bottom-r.top),
	})
	return 1
}

func mainWindowHandle() (uintptr, bool) {
	enumOnce.Do(func() { enumCallback = windows.NewCallback(ownWindowCallback) })
	enumMu.Lock()
	defer enumMu.Unlock()
	enumFound = enumFound[:0]
	if err := windows.EnumWindows(enumCallback, nil); err != nil {
		return 0, false
	}
	return pickMainWindow(enumFound)
}

func RaiseWindow(any) error {
	hwnd, ok := mainWindowHandle()
	if !ok {
		return ErrNoWindow
	}
	if iconic, _, _ := procIsIconic.Call(hwnd); iconic != 0 {
		procShowWindow.Call(hwnd, swRestore)
	}
	procBringWindowToTop.Call(hwnd)
	if r, _, _ := procSetForegroundWindow.Call(hwnd); r == 0 {
		return errForeground
	}
	return nil
}

func allowForeground(pid uint32) {
	procAllowSetForegroundWindow.Call(uintptr(pid))
}
