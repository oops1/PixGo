package settings

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/oops1/PixGo/internal/fsutil"
)

type WindowState struct {
	X         int    `json:"x"`
	Y         int    `json:"y"`
	W         int    `json:"w"`
	H         int    `json:"h"`
	Maximized bool   `json:"maximized"`
	LastFile  string `json:"lastFile"`
}

func DefaultWindowState() WindowState {
	return WindowState{}
}

func WindowStatePath() string { return filepath.Join(fsutil.StateDir(), "window.json") }

func (w WindowState) HasBounds() bool { return w.W > 0 && w.H > 0 }

func (w *WindowState) Normalize() {
	if w.W < 0 || w.H < 0 || w.W > 20000 || w.H > 20000 {
		w.W, w.H = 0, 0
	}
	if w.W == 0 || w.H == 0 {
		w.W, w.H, w.X, w.Y = 0, 0, 0, 0
	}
	if w.X < -20000 || w.X > 40000 || w.Y < -20000 || w.Y > 40000 {
		w.X, w.Y = 0, 0
	}
}

func DecodeWindowState(data []byte) (WindowState, error) {
	w := DefaultWindowState()
	if err := json.Unmarshal(data, &w); err != nil {
		return DefaultWindowState(), err
	}
	w.Normalize()
	return w, nil
}

func LoadWindowState() (WindowState, error) { return LoadWindowStateFrom(WindowStatePath()) }

func LoadWindowStateFrom(path string) (WindowState, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return DefaultWindowState(), nil
	}
	if err != nil {
		return DefaultWindowState(), err
	}
	w, err := DecodeWindowState(data)
	if err != nil {
		os.Rename(path, path+".broken")
		return DefaultWindowState(), err
	}
	return w, nil
}

func (w WindowState) Save() error { return w.SaveTo(WindowStatePath()) }

func (w WindowState) SaveTo(path string) error {
	w.Normalize()
	b, err := json.MarshalIndent(w, "", "  ")
	if err != nil {
		return err
	}
	return fsutil.WriteAtomic(path, append(b, '\n'), 0o600)
}
