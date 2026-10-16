package main

import (
	"os"
	"runtime/debug"
)

const steadyGCPercent = 30

func tuneGC() int {
	if os.Getenv("GOGC") != "" {
		return -1
	}
	return debug.SetGCPercent(steadyGCPercent)
}
