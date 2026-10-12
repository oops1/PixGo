package gallery

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/oops1/PixGo/internal/i18n"
	"github.com/oops1/PixGo/internal/viewer"
)

var (
	ErrEmpty        = errors.New("gallery: empty address")
	ErrNotFound     = errors.New("gallery: path not found")
	ErrNotDirectory = errors.New("gallery: neither a folder nor a file")
)

type AddressError struct {
	Path string
	Err  error
}

func (e *AddressError) Error() string { return e.Path + ": " + e.Err.Error() }

func (e *AddressError) Unwrap() error { return e.Err }

type Address struct {
	Loc  Location
	File string
}

func ParseAddress(text, home string) (Address, error) {
	text = trimAddress(text)
	if text == "" {
		return Address{}, ErrEmpty
	}
	if loc, ok := specialLocation(text); ok {
		return Address{Loc: loc}, nil
	}
	path := driveRoot(expandHome(text, home))
	if !filepath.IsAbs(path) {
		return Address{}, &AddressError{Path: text, Err: ErrNotFound}
	}
	path = filepath.Clean(path)
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			err = ErrNotFound
		}
		return Address{}, &AddressError{Path: path, Err: err}
	}
	return addressFor(path, info.Mode())
}

func AddressErrorText(err error) string {
	var ae *AddressError
	switch {
	case err == nil || errors.Is(err, ErrEmpty):
		return ""
	case !errors.As(err, &ae):
		return i18n.T("error.generic")
	case errors.Is(err, ErrNotFound):
		return i18n.T("gallery.addressNotFound", ae.Path)
	}
	return i18n.T("error.openNamed", ae.Path)
}

func addressFor(path string, mode fs.FileMode) (Address, error) {
	switch {
	case mode.IsDir():
		return Address{Loc: Folder(path)}, nil
	case viewer.IsFile(mode):
		return Address{Loc: Folder(filepath.Dir(path)), File: path}, nil
	}
	return Address{}, &AddressError{Path: path, Err: ErrNotDirectory}
}

func trimAddress(text string) string {
	text = strings.Trim(strings.TrimSpace(text), `"`)
	if n := len(text); n >= 2 && text[0] == '\'' && text[n-1] == '\'' {
		text = text[1 : n-1]
	}
	return strings.TrimSpace(text)
}

func specialLocation(text string) (Location, bool) {
	for _, loc := range []Location{{Kind: KindAll}, {Kind: KindRecent}, {Kind: KindFavorites}} {
		if strings.EqualFold(text, loc.Display()) {
			return loc, true
		}
	}
	return Location{}, false
}

func expandHome(text, home string) string {
	if home == "" || text == "" || text[0] != '~' {
		return text
	}
	if text == "~" {
		return home
	}
	if os.IsPathSeparator(text[1]) {
		return filepath.Join(home, text[2:])
	}
	return text
}

func driveRoot(path string) string {
	if vol := filepath.VolumeName(path); vol != "" && len(vol) == len(path) {
		return path + string(filepath.Separator)
	}
	return path
}
