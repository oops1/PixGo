package platform

import "strings"

const (
	groupDefault = "[Default Applications]"
	groupRemoved = "[Removed Associations]"
)

func SetDefaultInMimeapps(content, desktopID string, mimes []string) string {
	wanted := map[string]bool{}
	for _, m := range mimes {
		wanted[m] = true
	}
	lines := splitLines(content)
	var out []string
	group := ""
	seen := map[string]bool{}
	hasDefaultGroup := false
	flushMissing := func() {
		if group != groupDefault {
			return
		}
		var blanks []string
		for len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
			blanks = append(blanks, out[len(out)-1])
			out = out[:len(out)-1]
		}
		for _, m := range mimes {
			if !seen[m] {
				out = append(out, m+"="+desktopID+";")
				seen[m] = true
			}
		}
		out = append(out, blanks...)
	}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			flushMissing()
			group = trimmed
			if group == groupDefault {
				hasDefaultGroup = true
			}
			out = append(out, line)
			continue
		}
		key, value, isPair := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !isPair || !wanted[key] || (group != groupDefault && group != groupRemoved) {
			out = append(out, line)
			continue
		}
		ids := splitList(value)
		switch group {
		case groupDefault:
			ids = append([]string{desktopID}, without(ids, desktopID)...)
			seen[key] = true
			out = append(out, key+"="+joinList(ids))
		case groupRemoved:
			ids = without(ids, desktopID)
			if len(ids) > 0 {
				out = append(out, key+"="+joinList(ids))
			}
		}
	}
	flushMissing()
	if !hasDefaultGroup {
		if len(out) > 0 && strings.TrimSpace(out[len(out)-1]) != "" {
			out = append(out, "")
		}
		out = append(out, groupDefault)
		for _, m := range mimes {
			out = append(out, m+"="+desktopID+";")
		}
	}
	return strings.Join(out, "\n") + "\n"
}

func splitLines(content string) []string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.TrimRight(content, "\n")
	if content == "" {
		return nil
	}
	return strings.Split(content, "\n")
}

func splitList(value string) []string {
	var ids []string
	for _, p := range strings.Split(value, ";") {
		if p = strings.TrimSpace(p); p != "" {
			ids = append(ids, p)
		}
	}
	return ids
}

func joinList(ids []string) string { return strings.Join(ids, ";") + ";" }

func without(ids []string, id string) []string {
	var out []string
	for _, v := range ids {
		if v != id {
			out = append(out, v)
		}
	}
	return out
}
