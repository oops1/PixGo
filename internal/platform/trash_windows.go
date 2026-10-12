//go:build windows

package platform

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	foDelete          = 0x0003
	fofSilent         = 0x0004
	fofNoConfirmation = 0x0010
	fofAllowUndo      = 0x0040
	fofNoErrorUI      = 0x0400
	fofWantNukeWarn   = 0x4000

	maxPathChars = 260
)

type shFileOpStruct struct {
	hwnd                  uintptr
	wFunc                 uint32
	pFrom                 *uint16
	pTo                   *uint16
	fFlags                uint16
	fAnyOperationsAborted int32
	hNameMappings         uintptr
	lpszProgressTitle     *uint16
}

type winTrash struct{}

func NewTrash() Trash { return winTrash{} }

func recyclablePath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if strings.ContainsAny(abs, "*?") || strings.ContainsRune(abs, 0) {
		return "", fmt.Errorf("platform: wildcard in path %q", abs)
	}
	if len(abs) >= maxPathChars {
		return "", fmt.Errorf("platform: path longer than %d characters: %w", maxPathChars-1, ErrUnavailable)
	}
	vol := filepath.VolumeName(abs)
	if len(vol) != 2 || vol[1] != ':' || len(abs) <= len(vol)+1 {
		return "", fmt.Errorf("platform: %q has no recycle bin: %w", abs, ErrUnavailable)
	}
	return abs, nil
}

func (winTrash) Available(path string) bool {
	abs, err := recyclablePath(path)
	if err != nil {
		return false
	}
	if _, err := os.Lstat(abs); err != nil {
		return false
	}
	root, err := windows.UTF16PtrFromString(filepath.VolumeName(abs) + `\`)
	if err != nil {
		return false
	}
	return windows.GetDriveType(root) == windows.DRIVE_FIXED
}

func doubleNulPath(p string) ([]uint16, error) {
	u, err := windows.UTF16FromString(p)
	if err != nil {
		return nil, err
	}
	return append(u, 0), nil
}

func (t winTrash) Trash(ctx context.Context, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	abs, err := recyclablePath(path)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(abs); err != nil {
		return err
	}
	if !t.Available(abs) {
		return fmt.Errorf("platform: no recycle bin for %q: %w", abs, ErrUnavailable)
	}
	from, err := doubleNulPath(abs)
	if err != nil {
		return err
	}
	return onSTA(func() error {
		op := shFileOpStruct{
			wFunc:  foDelete,
			pFrom:  &from[0],
			fFlags: fofAllowUndo | fofNoConfirmation | fofNoErrorUI | fofSilent | fofWantNukeWarn,
		}
		r, _, _ := procSHFileOperationW.Call(uintptr(unsafe.Pointer(&op)))
		if op.fAnyOperationsAborted != 0 {
			return ErrCancelled
		}
		if r != 0 {
			return shellOpError(uint32(r))
		}
		if _, err := os.Lstat(abs); err == nil {
			return fmt.Errorf("platform: the item is still there after the shell operation: %w", ErrUnavailable)
		}
		return nil
	})
}

func shellOpError(code uint32) error {
	switch code {
	case 0x2, 0x3, 0x7C, 0x402:
		return fmt.Errorf("platform: trash: code 0x%X: %w", code, os.ErrNotExist)
	case 0x5, 0x78:
		return fmt.Errorf("platform: trash: code 0x%X: %w", code, os.ErrPermission)
	}
	return errors.New("platform: trash: SHFileOperation failed with code 0x" + strings.ToUpper(fmt.Sprintf("%X", code)))
}
