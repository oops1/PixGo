//go:build windows

package i18n

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modkernel32                  = windows.NewLazySystemDLL("kernel32.dll")
	procGetUserDefaultUILanguage = modkernel32.NewProc("GetUserDefaultUILanguage")
	procLCIDToLocaleName         = modkernel32.NewProc("LCIDToLocaleName")
)

const localeNameMaxLength = 85

func systemLanguage() string {
	r, _, _ := procGetUserDefaultUILanguage.Call()
	id := uint16(r)
	if id == 0 {
		return ""
	}
	var buf [localeNameMaxLength]uint16
	n, _, _ := procLCIDToLocaleName.Call(uintptr(id), uintptr(unsafe.Pointer(&buf[0])), localeNameMaxLength, 0)
	if n > 1 {
		return windows.UTF16ToString(buf[:n])
	}
	return langFromLangID(id)
}
