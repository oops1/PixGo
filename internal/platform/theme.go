package platform

import (
	"bufio"
	"bytes"
	"image/color"
	"strconv"
	"strings"
)

type themeState struct {
	dark, highContrast, reduceMotion bool
	accent                           color.RGBA
	hasAccent                        bool
}

type themeFile struct {
	family    string
	mode      string
	dark      bool
	darkSet   bool
	accent    color.RGBA
	hasAccent bool
}

type accessFile struct {
	reduceMotion, reduceSet bool
	highContrast, hcSet     bool
}

func parseKeyValues(data []byte) map[string]string {
	out := map[string]string{}
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || line[0] == '#' || line[0] == ';' || line[0] == '[' {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.ToLower(strings.TrimSpace(k))
		k = strings.NewReplacer("-", "_", " ", "_").Replace(k)
		out[k] = strings.Trim(strings.TrimSpace(v), `"'`)
	}
	return out
}

func parseThemeFile(data []byte) themeFile {
	var tf themeFile
	kv := parseKeyValues(data)
	tf.family = strings.ToLower(kv["family"])
	switch m := strings.ToLower(kv["mode"]); m {
	case "system", "light", "dark":
		tf.mode = m
	}
	if b, ok := parseBool(kv["dark"]); ok {
		tf.dark, tf.darkSet = b, true
	}
	if c, ok := parseHexColor(kv["accent"]); ok {
		tf.accent, tf.hasAccent = c, true
	}
	return tf
}

func parseAccessibilityFile(data []byte) accessFile {
	var af accessFile
	kv := parseKeyValues(data)
	if b, ok := parseBool(kv["reduce_motion"]); ok {
		af.reduceMotion, af.reduceSet = b, true
	}
	switch strings.ToLower(kv["contrast"]) {
	case "high", "more", "increased", "hc", "true", "yes", "on", "1":
		af.highContrast, af.hcSet = true, true
	case "normal", "default", "standard", "none", "false", "no", "off", "0":
		af.hcSet = true
	}
	return af
}

func parseBool(v string) (bool, bool) {
	switch strings.ToLower(v) {
	case "true", "yes", "on", "1":
		return true, true
	case "false", "no", "off", "0":
		return false, true
	}
	return false, false
}

func parseHexColor(v string) (color.RGBA, bool) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "#")
	if len(v) == 3 {
		v = string([]byte{v[0], v[0], v[1], v[1], v[2], v[2]})
	}
	if len(v) != 6 {
		return color.RGBA{}, false
	}
	n, err := strconv.ParseUint(v, 16, 32)
	if err != nil {
		return color.RGBA{}, false
	}
	return color.RGBA{R: uint8(n >> 16), G: uint8(n >> 8), B: uint8(n), A: 0xff}, true
}

func (tf themeFile) wantsSystem() bool {
	switch tf.mode {
	case "light", "dark":
		return false
	case "system":
		return true
	}
	return !tf.darkSet
}

func resolveTheme(tf themeFile, af accessFile, sysDark bool) themeState {
	var st themeState
	switch {
	case tf.mode == "dark":
		st.dark = true
	case tf.mode == "light":
		st.dark = false
	case tf.mode == "" && tf.darkSet:
		st.dark = tf.dark
	default:
		st.dark = sysDark
	}
	st.highContrast = af.highContrast
	st.reduceMotion = af.reduceMotion
	st.accent, st.hasAccent = tf.accent, tf.hasAccent
	return st
}
