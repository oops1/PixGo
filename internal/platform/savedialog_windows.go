//go:build windows

package platform

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	clsidFileSaveDialog = windows.GUID{Data1: 0xC0B4E2F3, Data2: 0xBA21, Data3: 0x4773, Data4: [8]byte{0x8D, 0xBA, 0x33, 0x5E, 0xC9, 0x46, 0xEB, 0x8B}}
	iidIFileSaveDialog  = windows.GUID{Data1: 0x84BCCD23, Data2: 0x5FDE, Data3: 0x4CDB, Data4: [8]byte{0xAE, 0xA4, 0xAF, 0x64, 0xB8, 0x3D, 0x78, 0xAB}}
	iidIShellItem       = windows.GUID{Data1: 0x43826D1E, Data2: 0xE718, Data3: 0x42EE, Data4: [8]byte{0xBC, 0x55, 0xA1, 0xE2, 0x61, 0xC3, 0x7B, 0xFE}}
)

const (
	fdShow             = 3
	fdSetFileTypes     = 4
	fdSetFileTypeIndex = 5
	fdGetFileTypeIndex = 6
	fdSetOptions       = 9
	fdGetOptions       = 10
	fdSetFolder        = 12
	fdSetFileName      = 15
	fdSetTitle         = 17
	fdGetResult        = 20
	fdSetDefaultExt    = 22

	siGetDisplayName = 5

	sigdnFileSysPath = 0x80058000

	fosOverwritePrompt = 0x2
	fosNoChangeDir     = 0x8
	fosForceFileSystem = 0x40
	fosPathMustExist   = 0x800
)

type winSaveDialog struct{}

func NewSaveDialog() SaveDialog {
	if os.Getenv("PIXGO_OWN_DIALOGS") == "1" {
		return unsupportedSaveDialog{}
	}
	return winSaveDialog{}
}

type comdlgFilterSpec struct {
	name, spec *uint16
}

func (winSaveDialog) Save(o SaveOptions) (string, bool, error) {
	var path string
	err := onSTA(func() error {
		d, err := coCreate(&clsidFileSaveDialog, &iidIFileSaveDialog)
		if err != nil {
			return err
		}
		defer d.release()
		flags := uint32(fosForceFileSystem | fosPathMustExist | fosOverwritePrompt | fosNoChangeDir)
		keep, err := setupDialog(d, o.Title, o.Dir, o.Name, o.Filters, o.FilterIndex, flags)
		if err != nil {
			return err
		}
		defer runtime.KeepAlive(keep)
		if ext := defaultExt(o.Filters, o.FilterIndex); ext != "" {
			d.call(fdSetDefaultExt, uintptr(unsafe.Pointer(utf16(ext))))
		}
		if hr := d.call(fdShow, o.Owner); failed(hr) {
			return hrError("Show", hr)
		}
		var it *comObj
		if hr := d.call(fdGetResult, uintptr(unsafe.Pointer(&it))); failed(hr) {
			return hrError("GetResult", hr)
		}
		p, err := itemPath(it)
		it.release()
		if err != nil {
			return err
		}
		if filepath.Ext(p) == "" {
			idx := clampIndex(o.FilterIndex, len(o.Filters))
			var ti uint32
			if hr := d.call(fdGetFileTypeIndex, uintptr(unsafe.Pointer(&ti))); !failed(hr) && ti >= 1 {
				idx = int(ti) - 1
			}
			if ext := defaultExt(o.Filters, idx); ext != "" {
				p = strings.TrimSuffix(p, ".") + "." + ext
			}
		}
		path = p
		return nil
	})
	if errors.Is(err, ErrCancelled) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return path, true, nil
}

func setupDialog(d *comObj, title, dir, name string, fs []FileFilter, idx int, flags uint32) (any, error) {
	var cur uint32
	if hr := d.call(fdGetOptions, uintptr(unsafe.Pointer(&cur))); failed(hr) {
		return nil, hrError("GetOptions", hr)
	}
	if hr := d.call(fdSetOptions, uintptr(cur|flags)); failed(hr) {
		return nil, hrError("SetOptions", hr)
	}
	if title != "" {
		d.call(fdSetTitle, uintptr(unsafe.Pointer(utf16(title))))
	}
	if name != "" {
		d.call(fdSetFileName, uintptr(unsafe.Pointer(utf16(filepath.Base(name)))))
	}
	if dir != "" {
		if abs, err := filepath.Abs(dir); err == nil {
			if f, err := shellItem(abs); err == nil {
				d.call(fdSetFolder, uintptr(unsafe.Pointer(f)))
				f.release()
			}
		}
	}
	specs := buildFilterSpecs(fs)
	if len(specs) == 0 {
		return nil, nil
	}
	raw := make([]comdlgFilterSpec, len(specs))
	for k, s := range specs {
		raw[k] = comdlgFilterSpec{utf16(s.Name), utf16(s.Spec)}
	}
	if hr := d.call(fdSetFileTypes, uintptr(len(raw)), uintptr(unsafe.Pointer(&raw[0]))); failed(hr) {
		return nil, hrError("SetFileTypes", hr)
	}
	d.call(fdSetFileTypeIndex, uintptr(clampIndex(idx, len(raw))+1))
	return raw, nil
}

func shellItem(path string) (*comObj, error) {
	var out *comObj
	r, _, _ := procSHCreateItemFromParsingName.Call(uintptr(unsafe.Pointer(utf16(path))), 0,
		uintptr(unsafe.Pointer(&iidIShellItem)), uintptr(unsafe.Pointer(&out)))
	if failed(uint32(r)) {
		return nil, hrError("SHCreateItemFromParsingName", uint32(r))
	}
	return out, nil
}

func itemPath(it *comObj) (string, error) {
	var p *uint16
	if hr := it.call(siGetDisplayName, sigdnFileSysPath, uintptr(unsafe.Pointer(&p))); failed(hr) {
		return "", hrError("GetDisplayName", hr)
	}
	defer windows.CoTaskMemFree(unsafe.Pointer(p))
	return windows.UTF16PtrToString(p), nil
}
