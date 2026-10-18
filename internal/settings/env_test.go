package settings

import "testing"

func TestEnvFlag(t *testing.T) {
	for value, want := range map[string]bool{
		"": false, "0": false, "false": false, "No": false, " off ": false,
		"1": true, "yes": true, "true": true, "anything": true,
	} {
		if got := envFlag(value); got != want {
			t.Errorf("envFlag(%q) = %v, want %v", value, got, want)
		}
	}
}

func TestParseThreads(t *testing.T) {
	for value, want := range map[string]int{
		"": 0, "abc": 0, "-3": 0, "0": 0, "1": 1, " 4 ": 4, "64": 64, "65": 0, "2.5": 0,
	} {
		if got := parseThreads(value); got != want {
			t.Errorf("parseThreads(%q) = %d, want %d", value, got, want)
		}
	}
}

func TestEnvironmentSwitches(t *testing.T) {
	t.Setenv(EnvNoUpdateCheck, "")
	t.Setenv(EnvThreads, "")
	if UpdateCheckDisabled() || ThreadsFromEnv() != 0 {
		t.Fatal("empty environment must change nothing")
	}
	t.Setenv(EnvNoUpdateCheck, "1")
	t.Setenv(EnvThreads, "3")
	if !UpdateCheckDisabled() || ThreadsFromEnv() != 3 {
		t.Fatal("environment switches ignored")
	}
}

func TestBuildCanSwitchTheUpdateCheckOff(t *testing.T) {
	t.Setenv(EnvNoUpdateCheck, "")
	t.Cleanup(func() { updateCheckOff.Store(false) })
	if UpdateCheckDisabled() {
		t.Fatal("on by default")
	}
	DisableUpdateCheck()
	if !UpdateCheckDisabled() {
		t.Fatal("the build switch was ignored")
	}
}
