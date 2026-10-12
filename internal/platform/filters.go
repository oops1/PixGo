package platform

import "strings"

type filterSpec struct {
	Name, Spec string
}

func buildFilterSpecs(fs []FileFilter) []filterSpec {
	out := make([]filterSpec, 0, len(fs))
	for _, f := range fs {
		var pats []string
		for _, p := range f.Patterns {
			if p = strings.TrimSpace(p); p != "" {
				pats = append(pats, p)
			}
		}
		if len(pats) == 0 {
			pats = []string{"*.*"}
		}
		out = append(out, filterSpec{Name: f.Name, Spec: strings.Join(pats, ";")})
	}
	return out
}

func defaultExt(fs []FileFilter, idx int) string {
	if idx < 0 || idx >= len(fs) || len(fs[idx].Patterns) == 0 {
		return ""
	}
	p := strings.TrimSpace(fs[idx].Patterns[0])
	ext, ok := strings.CutPrefix(p, "*.")
	if !ok || ext == "" || strings.ContainsAny(ext, "*?") {
		return ""
	}
	return ext
}

func clampIndex(i, n int) int {
	if i < 0 || i >= n {
		return 0
	}
	return i
}
