//go:build windows

package platform

import "golang.org/x/sys/windows"

type winAssociations struct{}

func NewAssociations() Associations { return winAssociations{} }

func (winAssociations) OpenDefaultsSettings() error {
	return onSTA(func() error {
		return windows.ShellExecute(0, utf16("open"), utf16("ms-settings:defaultapps"), nil, nil, windows.SW_SHOWNORMAL)
	})
}
