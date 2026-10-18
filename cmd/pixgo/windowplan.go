package main

import (
	"image"
	"math"
	"os"

	"github.com/oops1/PixGo/internal/settings"
)

const (
	minVisibleWidth  = 120
	minVisibleHeight = 80
)

type screen struct {
	Bounds image.Rectangle
	Work   image.Rectangle
	Scale  float64
}

func (s screen) area() image.Rectangle {
	if s.Work.Empty() {
		return s.Bounds
	}
	return s.Work
}

func (s screen) scale() float64 {
	if s.Scale <= 0 {
		return 1
	}
	return s.Scale
}

func physical(v int, k float64) int { return int(math.Round(float64(v) * k)) }

func logical(v int, k float64) int { return int(math.Floor(float64(v) / k)) }

func placeWindow(saved settings.WindowState, screens []screen) (x, y, w, h int, ok bool) {
	if !saved.HasBounds() {
		return 0, 0, 0, 0, false
	}
	if len(screens) == 0 {
		return 0, 0, saved.W, saved.H, false
	}
	best, bestArea := -1, 0
	for i, s := range screens {
		k := s.scale()
		r := image.Rect(saved.X, saved.Y, saved.X+physical(saved.W, k), saved.Y+physical(saved.H, k))
		in := r.Intersect(s.Bounds)
		if in.Dx() < minVisibleWidth || in.Dy() < minVisibleHeight {
			continue
		}
		if a := in.Dx() * in.Dy(); a > bestArea {
			best, bestArea = i, a
		}
	}
	if best < 0 {
		first := screens[0]
		k := first.scale()
		area := first.area()
		return 0, 0, min(saved.W, logical(area.Dx(), k)), min(saved.H, logical(area.Dy(), k)), false
	}
	s := screens[best]
	k := s.scale()
	area := s.area()
	w, h = saved.W, saved.H
	x, y = saved.X, saved.Y
	if maxW := logical(area.Dx(), k); w > maxW {
		w, x = maxW, area.Min.X
	}
	if maxH := logical(area.Dy(), k); h > maxH {
		h, y = maxH, area.Min.Y
	}
	if y < area.Min.Y {
		y = area.Min.Y
	}
	return x, y, w, h, true
}

type windowPlan struct {
	W, H     int
	X, Y     int
	HasPos   bool
	Maximize bool
}

func planWindow(remember bool, ws settings.WindowState, screens []screen) windowPlan {
	p := windowPlan{W: defaultWidth, H: defaultHeight}
	if !remember {
		return p
	}
	p.Maximize = ws.Maximized
	if !ws.HasBounds() {
		return p
	}
	x, y, w, h, ok := placeWindow(ws, screens)
	p.W, p.H = w, h
	if ok {
		p.X, p.Y, p.HasPos = x, y, true
	}
	return p
}

type windowSnapshot struct {
	X, Y, W, H int
	Maximized  bool
	Valid      bool
}

func nextWindowState(old settings.WindowState, snap windowSnapshot, rememberWindow, restoreLast bool, lastFile string) settings.WindowState {
	ws := old
	if rememberWindow && snap.Valid {
		ws.Maximized = snap.Maximized
		if !snap.Maximized {
			ws.X, ws.Y, ws.W, ws.H = snap.X, snap.Y, snap.W, snap.H
		}
	}
	if restoreLast {
		ws.LastFile = lastFile
	} else {
		ws.LastFile = ""
	}
	return ws
}

func lastFileToRestore(restore bool, ws settings.WindowState, exists func(string) bool) string {
	if !restore || ws.LastFile == "" || !exists(ws.LastFile) {
		return ""
	}
	return ws.LastFile
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

type initialPlacer interface {
	SetInitialBounds(x, y, w, h int)
	SetInitialMaximized(v bool)
}

func applyInitialPlacement(win initialPlacer, p windowPlan, sizeForced bool, w, h int) {
	if sizeForced {
		return
	}
	if p.HasPos {
		win.SetInitialBounds(p.X, p.Y, w, h)
	}
	if p.Maximize {
		win.SetInitialMaximized(true)
	}
}
