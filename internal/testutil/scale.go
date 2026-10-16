package testutil

import "github.com/oops1/headless-gui/v3/engine"

func NewEngineScaled(w, h int, k float64) *engine.Engine {
	eng := NewEngine(w, h)
	eng.SetScale(k)
	return eng
}
