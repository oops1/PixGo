//go:build windows

package platform

import "golang.org/x/sys/windows"

type winAssociations struct{}

func NewAssociations() Associations { return winAssociations{} }

func openSettings(uri string) error {
	return onSTA(func() error {
		return windows.ShellExecute(0, utf16("open"), utf16(uri), nil, nil, windows.SW_SHOWNORMAL)
	})
}

func (winAssociations) OpenDefaultsSettings() error { return openSettings("ms-settings:defaultapps") }

func (winAssociations) SetAsDefault() error {
	return openSettings("ms-settings:defaultapps?registeredAppUser=PixGo")
}
