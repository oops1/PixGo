//go:build windows

package platform

import (
	"encoding/binary"
	"testing"
)

func metricsBuffer(height int32) []byte {
	buf := make([]byte, ncmSize)
	binary.LittleEndian.PutUint32(buf[messageFontOffset:], uint32(height))
	return buf
}

func TestMessageFontHeightBecomesPointsAtTheSystemDPI(t *testing.T) {
	cases := []struct {
		height int32
		dpi    float64
		pt     float64
	}{
		{-12, 96, 9},
		{-15, 120, 9},
		{-16, 96, 12},
		{-24, 192, 9},
		{12, 96, 9},
	}
	for _, c := range cases {
		pt, ok := messageFontPt(metricsBuffer(c.height), c.dpi)
		if !ok || pt != c.pt {
			t.Errorf("height %d at %v dpi -> %v %v, want %v", c.height, c.dpi, pt, ok, c.pt)
		}
	}
}

func TestMessageFontNeedsAHeightAndAValidBuffer(t *testing.T) {
	if _, ok := messageFontPt(metricsBuffer(0), 96); ok {
		t.Fatal("a zero height is not a size")
	}
	if _, ok := messageFontPt(make([]byte, 10), 96); ok {
		t.Fatal("a short buffer must be rejected")
	}
	if _, ok := messageFontPt(metricsBuffer(-12), 0); ok {
		t.Fatal("a zero dpi must be rejected")
	}
}

func TestSystemDPIIsPositive(t *testing.T) {
	if systemDPI() <= 0 {
		t.Fatal("dpi must be positive")
	}
}

func TestWindowsBodySizeIsTheSystemMessageFont(t *testing.T) {
	t.Setenv(EnvFontPt, "")

	pt := NewSystemFonts().BodyPt()

	if pt < 8 || pt > 14 {
		t.Fatalf("a normal Windows setup gives 9 to 12 pt, got %v", pt)
	}
}
