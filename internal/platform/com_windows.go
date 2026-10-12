//go:build windows

package platform

import (
	"fmt"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	coinitApartmentThreaded = 0x2
	coinitDisableOLE1DDE    = 0x4
	clsctxInprocServer      = 0x1

	hrCancelled = 0x800704C7
)

type comObj struct {
	vtbl *[64]uintptr
}

func (o *comObj) call(i int, args ...uintptr) uint32 {
	a := append([]uintptr{uintptr(unsafe.Pointer(o))}, args...)
	r, _, _ := syscall.SyscallN(o.vtbl[i], a...)
	return uint32(r)
}

func (o *comObj) release() {
	if o != nil {
		o.call(2)
	}
}

func (o *comObj) queryInterface(iid *windows.GUID) (*comObj, error) {
	var out *comObj
	if hr := o.call(0, uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&out))); failed(hr) {
		return nil, hrError("QueryInterface", hr)
	}
	return out, nil
}

func failed(hr uint32) bool { return int32(hr) < 0 }

func hrError(op string, hr uint32) error {
	if hr == hrCancelled {
		return ErrCancelled
	}
	return fmt.Errorf("platform: %s: HRESULT 0x%08X", op, hr)
}

func coCreate(clsid, iid *windows.GUID) (*comObj, error) {
	var out *comObj
	r, _, _ := procCoCreateInstance.Call(uintptr(unsafe.Pointer(clsid)), 0,
		clsctxInprocServer, uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&out)))
	if failed(uint32(r)) {
		return nil, hrError("CoCreateInstance", uint32(r))
	}
	return out, nil
}

func onSTA(f func() error) error {
	done := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		err := windows.CoInitializeEx(0, coinitApartmentThreaded|coinitDisableOLE1DDE)
		if err != nil && err != syscall.Errno(1) {
			done <- fmt.Errorf("platform: CoInitializeEx: %w", err)
			return
		}
		defer windows.CoUninitialize()
		done <- f()
	}()
	return <-done
}

func utf16(s string) *uint16 {
	p, err := windows.UTF16PtrFromString(s)
	if err != nil {
		p, _ = windows.UTF16PtrFromString(s[:indexNUL(s)])
	}
	return p
}

func indexNUL(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] == 0 {
			return i
		}
	}
	return len(s)
}
