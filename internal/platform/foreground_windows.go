//go:build windows

package platform

func allowForeground(pid uint32) {
	procAllowSetForegroundWindow.Call(uintptr(pid))
}
