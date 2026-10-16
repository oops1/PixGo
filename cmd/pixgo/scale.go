package main

import (
	"fmt"
	"math"
	"slices"
)

const scaleEnv = "HEADLESS_GUI_SCALE"

var supportedScales = []float64{1, 1.25, 1.5, 1.75, 2}

func parseScale(v float64) (float64, error) {
	if v == 0 {
		return 0, nil
	}
	if !slices.Contains(supportedScales, v) {
		return 0, fmt.Errorf("pixgo: -scale %g is not supported, use 1, 1.25, 1.5, 1.75 or 2", v)
	}
	return v, nil
}

func logicalSize(w, h int, k float64) (int, int) {
	if k <= 0 {
		k = 1
	}
	return int(math.Round(float64(w) / k)), int(math.Round(float64(h) / k))
}
