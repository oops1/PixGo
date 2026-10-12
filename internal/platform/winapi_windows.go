//go:build windows

package platform

import "golang.org/x/sys/windows"

var (
	modkernel32 = windows.NewLazySystemDLL("kernel32.dll")
	modole32    = windows.NewLazySystemDLL("ole32.dll")
	modshell32  = windows.NewLazySystemDLL("shell32.dll")
	moduser32   = windows.NewLazySystemDLL("user32.dll")

	procCoCreateInstance            = modole32.NewProc("CoCreateInstance")
	procSHCreateItemFromParsingName = modshell32.NewProc("SHCreateItemFromParsingName")
	procSystemParametersInfoW       = moduser32.NewProc("SystemParametersInfoW")
	procSHFileOperationW            = modshell32.NewProc("SHFileOperationW")

	procGetWindowRect            = moduser32.NewProc("GetWindowRect")
	procGetWindow                = moduser32.NewProc("GetWindow")
	procIsIconic                 = moduser32.NewProc("IsIconic")
	procShowWindow               = moduser32.NewProc("ShowWindow")
	procBringWindowToTop         = moduser32.NewProc("BringWindowToTop")
	procSetForegroundWindow      = moduser32.NewProc("SetForegroundWindow")
	procAllowSetForegroundWindow = moduser32.NewProc("AllowSetForegroundWindow")
)
