//go:build !windows

package platform

import "os"

func listVolumes() []Volume {
	out := []Volume{{Path: "/", Name: "/", Label: "", Kind: VolumeRoot}}
	if f, err := os.Open("/proc/self/mounts"); err == nil {
		defer f.Close()
		out = append(out, parseMounts(f)...)
	}
	return out
}
