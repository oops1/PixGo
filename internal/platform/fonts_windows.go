//go:build windows

package platform

import (
	"encoding/binary"
	"os"
	"unsafe"
)

const (
	defaultBodyPt = 9.0

	spiGetNonClientMetrics = 0x0029
	ncmSize                = 504
	messageFontOffset      = 408
	accessibilityKey       = `SOFTWARE\Microsoft\Accessibility`
)

var procGetDpiForSystem = moduser32.NewProc("GetDpiForSystem")

type osFonts struct{}

func NewSystemFonts() SystemFonts { return osFonts{} }

func systemDPI() float64 {
	if procGetDpiForSystem.Find() != nil {
		return 96
	}
	r, _, _ := procGetDpiForSystem.Call()
	if r == 0 {
		return 96
	}
	return float64(r)
}

func messageFontPt(buf []byte, dpi float64) (float64, bool) {
	if len(buf) < messageFontOffset+4 || dpi <= 0 {
		return 0, false
	}
	h := int32(binary.LittleEndian.Uint32(buf[messageFontOffset:]))
	if h < 0 {
		h = -h
	}
	if h == 0 {
		return 0, false
	}
	return float64(h) * 72 / dpi, true
}

func (osFonts) BodyPt() float64 {
	if pt, ok := envFontPt(os.Getenv); ok {
		return pt
	}
	pt := defaultBodyPt
	buf := make([]byte, ncmSize)
	binary.LittleEndian.PutUint32(buf, ncmSize)
	if r, _, _ := procSystemParametersInfoW.Call(spiGetNonClientMetrics, ncmSize, uintptr(unsafe.Pointer(&buf[0])), 0); r != 0 {
		if v, ok := messageFontPt(buf, systemDPI()); ok {
			pt = v
		}
	}
	if v, ok := readDWORD(accessibilityKey, "TextScaleFactor"); ok && v >= 100 && v <= 225 {
		pt *= float64(v) / 100
	}
	return clampBodyPt(pt)
}
