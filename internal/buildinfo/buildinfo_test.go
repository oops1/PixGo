package buildinfo

import "testing"

func TestStringIsNeverEmpty(t *testing.T) {
	if String() == "" {
		t.Fatal("empty version string")
	}
}

func TestReleaseVersionIsReportedAsIs(t *testing.T) {
	old := Version
	defer func() { Version = old }()
	Version = "1.2.3"
	if got := String(); got != "1.2.3" {
		t.Fatalf("got %q", got)
	}
}

func TestEngineVersionLooksLikeAModuleVersionWhenKnown(t *testing.T) {
	v := Engine()

	if v != "" && v[0] != 'v' {
		t.Fatalf("engine version = %q", v)
	}
}
