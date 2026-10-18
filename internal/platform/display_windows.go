//go:build windows

package platform

import (
	"os"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modgdi32 = windows.NewLazySystemDLL("gdi32.dll")

	procGetICMProfileW           = modgdi32.NewProc("GetICMProfileW")
	procCreateDCW                = modgdi32.NewProc("CreateDCW")
	procDeleteDC                 = modgdi32.NewProc("DeleteDC")
	procEnumWindows              = moduser32.NewProc("EnumWindows")
	procGetWindowThreadProcessID = moduser32.NewProc("GetWindowThreadProcessId")
	procIsWindowVisible          = moduser32.NewProc("IsWindowVisible")
	procGetWindow                = moduser32.NewProc("GetWindow")
	procGetForegroundWindow      = moduser32.NewProc("GetForegroundWindow")
	procMonitorFromWindow        = moduser32.NewProc("MonitorFromWindow")
	procGetMonitorInfoW          = moduser32.NewProc("GetMonitorInfoW")
)

const (
	gwOwner                 = 4
	monitorDefaultToNearest = 2
	ccharsPerDevice         = 32
)

type monitorInfoEx struct {
	size    uint32
	monitor [4]int32
	work    [4]int32
	flags   uint32
	device  [ccharsPerDevice]uint16
}

type windowsDisplayProfile struct{}

func NewDisplayProfile() DisplayProfile { return windowsDisplayProfile{} }

func (windowsDisplayProfile) Profile() ([]byte, string) {
	hwnd := ownTopLevelWindow()
	if hwnd == 0 {
		hwnd, _, _ = procGetForegroundWindow.Call()
	}
	if hwnd == 0 {
		return nil, ""
	}
	device := monitorDevice(hwnd)
	if device == nil {
		return nil, ""
	}
	return readProfileFile(profilePathFor(device))
}

var (
	enumMu       sync.Mutex
	enumPID      uint32
	enumFound    uintptr
	enumCallback = syscall.NewCallback(func(hwnd uintptr, _ uintptr) uintptr {
		var owner uint32
		procGetWindowThreadProcessID.Call(hwnd, uintptr(unsafe.Pointer(&owner)))
		if owner != enumPID {
			return 1
		}
		if v, _, _ := procIsWindowVisible.Call(hwnd); v == 0 {
			return 1
		}
		if o, _, _ := procGetWindow.Call(hwnd, gwOwner); o != 0 {
			return 1
		}
		enumFound = hwnd
		return 0
	})
)

func ownTopLevelWindow() uintptr {
	enumMu.Lock()
	defer enumMu.Unlock()
	enumPID, enumFound = uint32(os.Getpid()), 0
	procEnumWindows.Call(enumCallback, 0)
	return enumFound
}

func monitorDevice(hwnd uintptr) *uint16 {
	mon, _, _ := procMonitorFromWindow.Call(hwnd, monitorDefaultToNearest)
	if mon == 0 {
		return nil
	}
	info := monitorInfoEx{size: uint32(unsafe.Sizeof(monitorInfoEx{}))}
	if r, _, _ := procGetMonitorInfoW.Call(mon, uintptr(unsafe.Pointer(&info))); r == 0 {
		return nil
	}
	return &info.device[0]
}

func profilePathFor(device *uint16) string {
	driver, err := windows.UTF16PtrFromString("DISPLAY")
	if err != nil {
		return ""
	}
	hdc, _, _ := procCreateDCW.Call(uintptr(unsafe.Pointer(driver)), uintptr(unsafe.Pointer(device)), 0, 0)
	if hdc == 0 {
		return ""
	}
	defer procDeleteDC.Call(hdc)
	var n uint32
	procGetICMProfileW.Call(hdc, uintptr(unsafe.Pointer(&n)), 0)
	if n == 0 || n > 32768 {
		return ""
	}
	buf := make([]uint16, n)
	if r, _, _ := procGetICMProfileW.Call(hdc, uintptr(unsafe.Pointer(&n)), uintptr(unsafe.Pointer(&buf[0]))); r == 0 {
		return ""
	}
	return windows.UTF16ToString(buf)
}
