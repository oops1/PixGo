//go:build windows

package platform

import (
	"github.com/oops1/PixGo/internal/i18n"
	"golang.org/x/sys/windows"
)

func listVolumes() []Volume {
	prev := windows.SetErrorMode(windows.SEM_FAILCRITICALERRORS)
	defer windows.SetErrorMode(prev)
	mask, err := windows.GetLogicalDrives()
	if err != nil {
		return nil
	}
	var out []Volume
	for i := 0; i < 26; i++ {
		if mask&(1<<uint(i)) == 0 {
			continue
		}
		letter := string(rune('A'+i)) + ":"
		root := letter + `\`
		rootPtr, err := windows.UTF16PtrFromString(root)
		if err != nil {
			continue
		}
		kind := VolumeUnknown
		switch windows.GetDriveType(rootPtr) {
		case windows.DRIVE_FIXED:
			kind = VolumeFixed
		case windows.DRIVE_REMOVABLE:
			kind = VolumeRemovable
		case windows.DRIVE_REMOTE:
			kind = VolumeNetwork
		case windows.DRIVE_CDROM:
			kind = VolumeOptical
		case windows.DRIVE_RAMDISK:
			kind = VolumeRAM
		case windows.DRIVE_NO_ROOT_DIR:
			continue
		}
		var label [windows.MAX_PATH + 1]uint16
		var name string
		if err := windows.GetVolumeInformation(rootPtr, &label[0], uint32(len(label)), nil, nil, nil, nil, 0); err == nil {
			name = windows.UTF16ToString(label[:])
		}
		display := i18n.T("gallery.drive", letter)
		if name != "" {
			display = name + " (" + letter + ")"
		}
		out = append(out, Volume{Path: root, Name: display, Label: name, Kind: kind})
	}
	return out
}
