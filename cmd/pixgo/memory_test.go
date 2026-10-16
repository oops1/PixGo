package main

import (
	"runtime/debug"
	"testing"
)

func TestTuneGCLowersTheCollectionThreshold(t *testing.T) {
	t.Setenv("GOGC", "")
	previous := debug.SetGCPercent(100)
	defer debug.SetGCPercent(previous)
	if got := tuneGC(); got != 100 {
		t.Fatalf("previous setting %d, want 100", got)
	}
	if now := debug.SetGCPercent(100); now != steadyGCPercent {
		t.Fatalf("GC percent %d, want %d", now, steadyGCPercent)
	}
}

func TestTuneGCRespectsAnExplicitEnvironmentSetting(t *testing.T) {
	t.Setenv("GOGC", "200")
	previous := debug.SetGCPercent(200)
	defer debug.SetGCPercent(previous)
	if got := tuneGC(); got != -1 {
		t.Fatalf("tuneGC returned %d, want -1 when GOGC is set", got)
	}
	if now := debug.SetGCPercent(200); now != 200 {
		t.Fatalf("GC percent changed to %d", now)
	}
}
