package platform

import (
	"bufio"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	minBodyPt = 6.0
	maxBodyPt = 24.0

	EnvFontPt = "PIXGO_FONT_PT"
)

type SystemFonts interface {
	BodyPt() float64
}

func clampBodyPt(pt float64) float64 { return min(max(pt, minBodyPt), maxBodyPt) }

func usableFontPt(pt float64) bool { return pt > 0 && !math.IsNaN(pt) && !math.IsInf(pt, 0) }

func envFontPt(getenv func(string) string) (float64, bool) {
	v, err := strconv.ParseFloat(strings.TrimSpace(getenv(EnvFontPt)), 64)
	if err != nil || !usableFontPt(v) {
		return 0, false
	}
	return clampBodyPt(v), true
}

func parseGTKFontPt(settings string) (float64, bool) {
	sc := bufio.NewScanner(strings.NewReader(settings))
	for sc.Scan() {
		key, value, ok := strings.Cut(strings.TrimSpace(sc.Text()), "=")
		if !ok || strings.TrimSpace(key) != "gtk-font-name" {
			continue
		}
		fields := strings.Fields(strings.Trim(strings.TrimSpace(value), `"`))
		if len(fields) == 0 {
			return 0, false
		}
		pt, err := strconv.ParseFloat(fields[len(fields)-1], 64)
		if err != nil || !usableFontPt(pt) {
			return 0, false
		}
		return clampBodyPt(pt), true
	}
	return 0, false
}

func gtkSettingsFiles() []string {
	cfg := os.Getenv("XDG_CONFIG_HOME")
	if cfg == "" {
		if home, err := os.UserHomeDir(); err == nil {
			cfg = filepath.Join(home, ".config")
		}
	}
	if cfg == "" {
		return nil
	}
	return []string{
		filepath.Join(cfg, "gtk-4.0", "settings.ini"),
		filepath.Join(cfg, "gtk-3.0", "settings.ini"),
	}
}

func gtkFontPt(files []string) (float64, bool) {
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		if pt, ok := parseGTKFontPt(string(data)); ok {
			return pt, true
		}
	}
	return 0, false
}
