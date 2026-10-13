package platform

import (
	"context"
	"errors"
	"image/color"

	"github.com/oops1/PixGo/internal/fsutil"
)

const InstanceName = "PixGo"

var (
	ErrCancelled   = errors.New("platform: cancelled")
	ErrUnavailable = errors.New("platform: unavailable")
	ErrUnsupported = errors.New("platform: unsupported on this platform")
)

type Trash interface {
	Available(path string) bool
	Trash(ctx context.Context, path string) error
}

type SystemTheme interface {
	Dark() bool
	HighContrast() bool
	Accent() (color.RGBA, bool)
	ReduceMotion() bool
	Watch(changed func()) (stop func())
}

type Locale interface {
	UILanguage() string
	Region() string
}

type Folders struct {
	Home      string
	Desktop   string
	Documents string
	Downloads string
	Pictures  string
	Videos    string
}

type KnownFolders interface {
	Folders() Folders
}

type VolumeKind int

const (
	VolumeUnknown VolumeKind = iota
	VolumeFixed
	VolumeRemovable
	VolumeNetwork
	VolumeOptical
	VolumeRAM
	VolumeRoot
)

type Volume struct {
	Path  string
	Name  string
	Label string
	Kind  VolumeKind
}

type Volumes interface {
	List() []Volume
	Watch(changed func()) (stop func())
}

type FileFilter struct {
	Name     string
	Patterns []string
}

type SaveOptions struct {
	Title       string
	Dir         string
	Name        string
	Filters     []FileFilter
	FilterIndex int
	Owner       uintptr
}

type SaveDialog interface {
	Save(SaveOptions) (path string, ok bool, err error)
}

type Instance interface {
	Acquire() (primary bool, err error)
	Send(args []string) error
	Listen(handle func(args []string)) error
	Close() error
}

type Associations interface {
	OpenDefaultsSettings() error
	SetAsDefault() error
}

type Paths interface {
	ConfigDir() string
	StateDir() string
	CacheDir() string
}

type Platform struct {
	Trash        Trash
	Theme        SystemTheme
	Fonts        SystemFonts
	Locale       Locale
	Folders      KnownFolders
	Volumes      Volumes
	SaveDialog   SaveDialog
	Instance     Instance
	Associations Associations
	Paths        Paths
}

func New() *Platform {
	return &Platform{
		Trash:        NewTrash(),
		Theme:        NewSystemTheme(),
		Fonts:        NewSystemFonts(),
		Locale:       NewLocale(),
		Folders:      NewKnownFolders(),
		Volumes:      NewVolumes(),
		SaveDialog:   NewSaveDialog(),
		Instance:     NewInstance(InstanceName),
		Associations: NewAssociations(),
		Paths:        NewPaths(),
	}
}

type osPaths struct{}

func NewPaths() Paths { return osPaths{} }

func (osPaths) ConfigDir() string { return fsutil.ConfigDir() }
func (osPaths) StateDir() string  { return fsutil.StateDir() }
func (osPaths) CacheDir() string  { return fsutil.CacheDir() }

func WindowsHandle(n any) uintptr {
	switch w := n.(type) {
	case interface{ HWND() uintptr }:
		return w.HWND()
	case interface{ Handle() uintptr }:
		return w.Handle()
	}
	return 0
}
