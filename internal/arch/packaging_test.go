package arch

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/all"
)

func readPackaging(t *testing.T, parts ...string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(append([]string{root(t)}, parts...)...))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func extensionsIn(t *testing.T, text string, re *regexp.Regexp) []string {
	t.Helper()
	seen := map[string]bool{}
	for _, m := range re.FindAllStringSubmatch(text, -1) {
		seen[strings.ToLower(m[1])] = true
	}
	var out []string
	for e := range seen {
		out = append(out, e)
	}
	sort.Strings(out)
	return out
}

func TestInstallerRegistersExactlyTheSupportedExtensions(t *testing.T) {
	r := codec.NewRegistry()
	all.Register(r)
	want := r.Extensions()
	sort.Strings(want)
	iss := readPackaging(t, "packaging", "windows", "pixgo.iss")
	openWith := extensionsIn(t, iss, regexp.MustCompile(`Software\\Classes\\(\.[a-z0-9]+)\\OpenWithProgids`))
	caps := extensionsIn(t, iss, regexp.MustCompile(`FileAssociations"; ValueType: string; ValueName: "(\.[a-z0-9]+)"`))
	if strings.Join(openWith, " ") != strings.Join(want, " ") {
		t.Errorf("OpenWithProgids\n  %v\nsupported\n  %v", openWith, want)
	}
	if strings.Join(caps, " ") != strings.Join(want, " ") {
		t.Errorf("Capabilities\n  %v\nsupported\n  %v", caps, want)
	}
}

func TestInstallerScriptHasAssociationTaskAndLanguages(t *testing.T) {
	iss := readPackaging(t, "packaging", "windows", "pixgo.iss")
	for _, need := range []string{
		`Name: "assoc"`, "ChangesAssociations=yes", "PrivilegesRequired=lowest",
		"RegisteredApplications", "Languages\\Russian.isl", "OpenWithProgids",
	} {
		if !strings.Contains(iss, need) {
			t.Errorf("installer script lacks %q", need)
		}
	}
	for _, line := range strings.Split(iss, "\n") {
		if strings.HasPrefix(line, "Root: HKA;") && !strings.Contains(line, "Tasks: assoc") {
			t.Errorf("registry entry outside the association task: %s", line)
		}
	}
}

func TestLinuxPackagingFilesExist(t *testing.T) {
	for _, p := range [][]string{
		{"packaging", "linux", "pixgo.desktop"},
		{"packaging", "linux", "install.sh"},
		{"packaging", "linux", "uninstall.sh"},
		{"packaging", "linux", "nfpm.yaml"},
		{"packaging", "icons", "pixgo.ico"},
		{"packaging", "icons", "hicolor", "256x256", "apps", "pixgo.png"},
	} {
		if _, err := os.Stat(filepath.Join(append([]string{root(t)}, p...)...)); err != nil {
			t.Errorf("%v: %v", p, err)
		}
	}
}

func TestShellScriptsUseLFEndings(t *testing.T) {
	for _, name := range []string{"install.sh", "uninstall.sh", "postinst.sh", "postrm.sh"} {
		if strings.Contains(readPackaging(t, "packaging", "linux", name), "\r") {
			t.Errorf("%s has CR characters", name)
		}
	}
}
