package settings

import (
	"os"
	"strconv"
	"strings"
)

const (
	EnvNoUpdateCheck = "PIXGO_NO_UPDATE_CHECK"
	EnvThreads       = "PIXGO_THREADS"
	maxEnvThreads    = 64
)

func UpdateCheckDisabled() bool { return envFlag(os.Getenv(EnvNoUpdateCheck)) }

func ThreadsFromEnv() int { return parseThreads(os.Getenv(EnvThreads)) }

func envFlag(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "0", "false", "no", "off":
		return false
	}
	return true
}

func parseThreads(value string) int {
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || n < 1 || n > maxEnvThreads {
		return 0
	}
	return n
}
