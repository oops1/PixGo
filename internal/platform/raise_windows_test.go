//go:build windows

package platform

import (
	"errors"
	"testing"

	"golang.org/x/sys/windows"
)

func TestRaiseWithoutAWindowReportsIt(t *testing.T) {
	if _, ok := mainWindowHandle(); ok {
		t.Skip("the test process owns a visible top-level window")
	}

	if err := RaiseWindow(nil); !errors.Is(err, ErrNoWindow) {
		t.Fatalf("err = %v, want ErrNoWindow", err)
	}
}

func TestAllowingForegroundForOurOwnProcessIsHarmless(t *testing.T) {
	allowForeground(windows.GetCurrentProcessId())
	allowForeground(0)
}
