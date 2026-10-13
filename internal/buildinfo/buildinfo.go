package buildinfo

import "runtime/debug"

var Version = "dev"

func Revision() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	for _, s := range info.Settings {
		if s.Key == "vcs.revision" {
			if len(s.Value) > 7 {
				return s.Value[:7]
			}
			return s.Value
		}
	}
	return ""
}

func String() string {
	if rev := Revision(); rev != "" && Version == "dev" {
		return Version + "+" + rev
	}
	return Version
}

func Engine() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	for _, dep := range info.Deps {
		if dep.Path == "github.com/oops1/headless-gui/v3" {
			return dep.Version
		}
	}
	return ""
}
