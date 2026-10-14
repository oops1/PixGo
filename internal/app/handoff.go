package app

import "strings"

const activationMark = "\x00activation:"

func EncodeHandoff(token string, paths []string) []string {
	if token == "" {
		return paths
	}
	out := make([]string, 0, len(paths)+1)
	out = append(out, activationMark+token)
	return append(out, paths...)
}

func DecodeHandoff(args []string) (token string, paths []string) {
	for _, a := range args {
		if t, ok := strings.CutPrefix(a, activationMark); ok {
			token = t
			continue
		}
		paths = append(paths, a)
	}
	return token, paths
}
