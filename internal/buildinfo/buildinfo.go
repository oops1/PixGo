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
