//go:build !windows && !linux

package main

import "runtime"

func cpuModel() string { return "unknown" }

func osDescription() string { return runtime.GOOS }
