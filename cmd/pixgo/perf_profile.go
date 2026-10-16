package main

import (
	"fmt"
	"os"
	"runtime"
	"runtime/pprof"
)

func startMemProfile(path string) func() {
	if path == "" {
		return func() {}
	}
	previous := runtime.MemProfileRate
	runtime.MemProfileRate = perfMemProfileRate
	return func() {
		f, err := os.Create(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, "pixgo: perf:", err)
			return
		}
		defer f.Close()
		if err := pprof.Lookup("allocs").WriteTo(f, 0); err != nil {
			fmt.Fprintln(os.Stderr, "pixgo: perf:", err)
		}
		runtime.MemProfileRate = previous
	}
}

func startCPUProfile(path string) func() {
	if path == "" {
		return func() {}
	}
	f, err := os.Create(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "pixgo: perf:", err)
		return func() {}
	}
	runtime.SetCPUProfileRate(perfProfileHz)
	if err := pprof.StartCPUProfile(f); err != nil {
		f.Close()
		fmt.Fprintln(os.Stderr, "pixgo: perf:", err)
		return func() {}
	}
	return func() {
		pprof.StopCPUProfile()
		f.Close()
	}
}
