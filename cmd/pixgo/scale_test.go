package main

import (
	"math"
	"testing"
)

func TestParseScaleAcceptsTheSupportedSteps(t *testing.T) {
	for _, v := range supportedScales {
		got, err := parseScale(v)
		if err != nil || got != v {
			t.Errorf("parseScale(%g) = %g, %v", v, got, err)
		}
	}
}

func TestParseScaleZeroFollowsTheDisplay(t *testing.T) {
	got, err := parseScale(0)
	if err != nil || got != 0 {
		t.Fatalf("parseScale(0) = %g, %v", got, err)
	}
}

func TestParseScaleRejectsOtherValues(t *testing.T) {
	for _, v := range []float64{-1, 0.5, 1.1, 2.5, 3, 4} {
		if _, err := parseScale(v); err == nil {
			t.Errorf("parseScale(%g) accepted", v)
		}
	}
}

func TestSavedWindowSizeIsLogicalSoItDoesNotGrowOnEveryLaunch(t *testing.T) {
	for _, k := range supportedScales {
		physicalW, physicalH := int(math.Round(1100*k)), int(math.Round(700*k))
		w, h := logicalSize(physicalW, physicalH, k)
		if w != 1100 || h != 700 {
			t.Errorf("scale %v: %dx%d physical saved as %dx%d, want 1100x700", k, physicalW, physicalH, w, h)
		}
	}
}

func TestLogicalSizeIgnoresAnInvalidScale(t *testing.T) {
	for _, k := range []float64{0, -1} {
		if w, h := logicalSize(800, 600, k); w != 800 || h != 600 {
			t.Errorf("scale %v: %dx%d", k, w, h)
		}
	}
}
