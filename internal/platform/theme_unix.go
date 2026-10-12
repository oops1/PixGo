//go:build !windows

package platform

import (
	"image/color"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/oops1/headless-gui/v3/window"
)

const (
	themePoll   = 1200 * time.Millisecond
	engineEvery = 10
)

type fileTheme struct {
	themePath  string
	accessPath string
	detect     func() (dark, known bool)
	poll       time.Duration

	mu         sync.Mutex
	loaded     bool
	checked    time.Time
	themeStamp fileStamp
	accStamp   fileStamp
	st         themeState
	engAsked   bool
	engDark    bool
}

type fileStamp struct {
	exists bool
	mod    time.Time
	size   int64
}

func engineDetect() (bool, bool) {
	switch window.DetectSystemTheme() {
	case window.SystemThemeDark:
		return true, true
	case window.SystemThemeLight:
		return false, true
	}
	return false, false
}

func NewSystemTheme() SystemTheme {
	var themePath, accessPath string
	if h, err := os.UserHomeDir(); err == nil && h != "" {
		themePath = filepath.Join(h, ".winline", "theme")
		accessPath = filepath.Join(h, ".winline", "accessibility")
	}
	t := &fileTheme{themePath: themePath, accessPath: accessPath, detect: engineDetect, poll: themePoll}
	t.refresh(false)
	return t
}

func (t *fileTheme) state() themeState {
	t.mu.Lock()
	stale := time.Since(t.checked) >= t.poll
	t.mu.Unlock()
	if stale {
		t.refresh(false)
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.st
}

func (t *fileTheme) Dark() bool         { return t.state().dark }
func (t *fileTheme) HighContrast() bool { return t.state().highContrast }
func (t *fileTheme) ReduceMotion() bool { return t.state().reduceMotion }

func (t *fileTheme) Accent() (color.RGBA, bool) {
	st := t.state()
	return st.accent, st.hasAccent
}

func (t *fileTheme) Watch(changed func()) func() {
	done := make(chan struct{})
	var once sync.Once
	go func() {
		tk := time.NewTicker(t.poll)
		defer tk.Stop()
		n := 0
		for {
			select {
			case <-done:
				return
			case <-tk.C:
			}
			n++
			if t.refresh(n%engineEvery == 0) {
				changed()
			}
		}
	}()
	return func() { once.Do(func() { close(done) }) }
}

func statStamp(path string) fileStamp {
	if path == "" {
		return fileStamp{}
	}
	fi, err := os.Stat(path)
	if err != nil || !fi.Mode().IsRegular() {
		return fileStamp{}
	}
	return fileStamp{exists: true, mod: fi.ModTime(), size: fi.Size()}
}

func readIf(path string, s fileStamp) []byte {
	if !s.exists {
		return nil
	}
	data, _ := os.ReadFile(path)
	return data
}

func (t *fileTheme) refresh(reaskEngine bool) bool {
	ts, as := statStamp(t.themePath), statStamp(t.accessPath)
	t.mu.Lock()
	t.checked = time.Now()
	filesSame := t.loaded && ts == t.themeStamp && as == t.accStamp
	if filesSame && !(reaskEngine && t.engAsked) {
		t.mu.Unlock()
		return false
	}
	asked := t.engAsked
	t.mu.Unlock()

	tf := parseThemeFile(readIf(t.themePath, ts))
	af := parseAccessibilityFile(readIf(t.accessPath, as))
	needEngine := tf.wantsSystem() && t.detect != nil && (!asked || reaskEngine)
	engDark, engKnown := false, false
	if needEngine {
		engDark, engKnown = t.detect()
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	if needEngine {
		t.engAsked = true
		t.engDark = engKnown && engDark
	}
	t.loaded, t.themeStamp, t.accStamp = true, ts, as
	old := t.st
	t.st = resolveTheme(tf, af, t.engDark)
	return old != t.st
}
