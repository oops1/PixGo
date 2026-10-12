//go:build windows

package platform

import (
	"image/color"
	"runtime"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const (
	personalizeKey = `Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`
	dwmKey         = `Software\Microsoft\Windows\DWM`
	hcKey          = `Control Panel\Accessibility\HighContrast`
	desktopKey     = `Control Panel\Desktop`

	spiGetHighContrast        = 0x0042
	spiGetClientAreaAnimation = 0x1042
	hcfHighContrastOn         = 0x1

	regNotifyLastSet = 0x4
	regNotifyName    = 0x1
)

type regTheme struct{}

func NewSystemTheme() SystemTheme { return regTheme{} }

func readDWORD(path, name string) (uint32, bool) {
	k, err := registry.OpenKey(registry.CURRENT_USER, path, registry.QUERY_VALUE)
	if err != nil {
		return 0, false
	}
	defer k.Close()
	v, _, err := k.GetIntegerValue(name)
	if err != nil {
		return 0, false
	}
	return uint32(v), true
}

func (regTheme) Dark() bool {
	v, ok := readDWORD(personalizeKey, "AppsUseLightTheme")
	return ok && v == 0
}

type highContrastW struct {
	size   uint32
	flags  uint32
	scheme *uint16
}

func (regTheme) HighContrast() bool {
	hc := highContrastW{size: uint32(unsafe.Sizeof(highContrastW{}))}
	r, _, _ := procSystemParametersInfoW.Call(spiGetHighContrast,
		uintptr(hc.size), uintptr(unsafe.Pointer(&hc)), 0)
	return r != 0 && hc.flags&hcfHighContrastOn != 0
}

func (regTheme) ReduceMotion() bool {
	var enabled int32 = 1
	r, _, _ := procSystemParametersInfoW.Call(spiGetClientAreaAnimation, 0, uintptr(unsafe.Pointer(&enabled)), 0)
	return r != 0 && enabled == 0
}

func (regTheme) Accent() (color.RGBA, bool) {
	if v, ok := readDWORD(dwmKey, "AccentColor"); ok {
		return color.RGBA{R: uint8(v), G: uint8(v >> 8), B: uint8(v >> 16), A: 0xff}, true
	}
	if v, ok := readDWORD(dwmKey, "ColorizationColor"); ok {
		return color.RGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 0xff}, true
	}
	return color.RGBA{}, false
}

func (t regTheme) snapshot() themeState {
	var s themeState
	s.dark, s.highContrast, s.reduceMotion = t.Dark(), t.HighContrast(), t.ReduceMotion()
	s.accent, s.hasAccent = t.Accent()
	return s
}

func (t regTheme) Watch(changed func()) func() {
	stop, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		return func() {}
	}
	var mu sync.Mutex
	closed := false
	go func() {
		defer func() {
			mu.Lock()
			closed = true
			windows.CloseHandle(stop)
			mu.Unlock()
		}()
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		var keys []registry.Key
		var events []windows.Handle
		defer func() {
			for _, k := range keys {
				k.Close()
			}
			for _, e := range events {
				windows.CloseHandle(e)
			}
		}()
		for _, p := range []string{personalizeKey, dwmKey, hcKey, desktopKey} {
			k, err := registry.OpenKey(registry.CURRENT_USER, p, registry.NOTIFY)
			if err != nil {
				continue
			}
			ev, err := windows.CreateEvent(nil, 0, 0, nil)
			if err != nil {
				k.Close()
				continue
			}
			keys, events = append(keys, k), append(events, ev)
		}
		if len(keys) == 0 {
			return
		}
		arm := func(i int) {
			windows.RegNotifyChangeKeyValue(windows.Handle(keys[i]), false,
				regNotifyLastSet|regNotifyName, events[i], true)
		}
		for i := range keys {
			arm(i)
		}
		last := t.snapshot()
		wait := append([]windows.Handle{stop}, events...)
		for {
			r, err := windows.WaitForMultipleObjects(wait, false, windows.INFINITE)
			if err != nil || r == windows.WAIT_OBJECT_0 {
				return
			}
			i := int(r - windows.WAIT_OBJECT_0 - 1)
			if i < 0 || i >= len(keys) {
				return
			}
			arm(i)
			if s := t.snapshot(); s != last {
				last = s
				changed()
			}
		}
	}()
	return func() {
		mu.Lock()
		if !closed {
			windows.SetEvent(stop)
		}
		mu.Unlock()
	}
}
