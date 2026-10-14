//go:build windows

package platform

import (
	"testing"

	"golang.org/x/sys/windows"
)

func TestAllowingForegroundForOurOwnProcessIsHarmless(t *testing.T) {
	allowForeground(windows.GetCurrentProcessId())
	allowForeground(0)
}
