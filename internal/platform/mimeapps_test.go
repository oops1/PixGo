package platform

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestSetDefaultCreatesGroupInEmptyFile(t *testing.T) {
	got := SetDefaultInMimeapps("", "pixgo.desktop", []string{"image/png", "image/jpeg"})
	want := "[Default Applications]\nimage/png=pixgo.desktop;\nimage/jpeg=pixgo.desktop;\n"
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestSetDefaultPutsPixGoFirstAndKeepsOthers(t *testing.T) {
	in := "[Default Applications]\nimage/png=eog.desktop;gimp.desktop;\ntext/plain=gedit.desktop;\n"
	got := SetDefaultInMimeapps(in, "pixgo.desktop", []string{"image/png", "image/gif"})
	for _, want := range []string{"image/png=pixgo.desktop;eog.desktop;gimp.desktop;", "image/gif=pixgo.desktop;", "text/plain=gedit.desktop;"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestSetDefaultMovesExistingEntryToFront(t *testing.T) {
	in := "[Default Applications]\nimage/png=eog.desktop;pixgo.desktop;\n"
	got := SetDefaultInMimeapps(in, "pixgo.desktop", []string{"image/png"})
	if !strings.Contains(got, "image/png=pixgo.desktop;eog.desktop;\n") || strings.Count(got, "pixgo.desktop") != 1 {
		t.Fatalf("got:\n%s", got)
	}
}

func TestSetDefaultIsIdempotent(t *testing.T) {
	mimes := []string{"image/png", "image/jpeg"}
	once := SetDefaultInMimeapps("[Added Associations]\nimage/png=eog.desktop;\n", "pixgo.desktop", mimes)
	twice := SetDefaultInMimeapps(once, "pixgo.desktop", mimes)
	if once != twice {
		t.Fatalf("second run changed the file:\n%s\n---\n%s", once, twice)
	}
}

func TestSetDefaultPreservesCommentsAndOtherGroups(t *testing.T) {
	in := "# my settings\n[Added Associations]\nimage/png=eog.desktop;\n\n[Default Applications]\n# keep\nvideo/mp4=vlc.desktop;\n\n[Other]\nkey=value\n"
	got := SetDefaultInMimeapps(in, "pixgo.desktop", []string{"image/png"})
	for _, want := range []string{"# my settings\n", "[Added Associations]\nimage/png=eog.desktop;\n", "# keep\n", "video/mp4=vlc.desktop;\n", "[Other]\nkey=value\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("lost %q in:\n%s", want, got)
		}
	}
	i := strings.Index(got, "image/png=pixgo.desktop;")
	j := strings.Index(got, "[Other]")
	if i < 0 || j < 0 || i > j {
		t.Errorf("the new entry is not inside [Default Applications]:\n%s", got)
	}
}

func TestSetDefaultClearsRemovedAssociation(t *testing.T) {
	in := "[Removed Associations]\nimage/png=pixgo.desktop;eog.desktop;\nimage/gif=pixgo.desktop;\n"
	got := SetDefaultInMimeapps(in, "pixgo.desktop", []string{"image/png", "image/gif"})
	if strings.Contains(got, "image/gif=pixgo.desktop;\n[") || strings.Count(got, "[Removed Associations]\nimage/png=eog.desktop;") != 1 {
		t.Fatalf("got:\n%s", got)
	}
	removed := got[strings.Index(got, "[Removed Associations]"):strings.Index(got, "[Default Applications]")]
	if strings.Contains(removed, "pixgo.desktop") {
		t.Errorf("pixgo still listed as removed:\n%s", removed)
	}
}

func TestSetDefaultHandlesCRLF(t *testing.T) {
	got := SetDefaultInMimeapps("[Default Applications]\r\nimage/png=eog.desktop;\r\n", "pixgo.desktop", []string{"image/png"})
	if strings.Contains(got, "\r") || !strings.Contains(got, "image/png=pixgo.desktop;eog.desktop;") {
		t.Fatalf("got %q", got)
	}
}

func TestDesktopFileMatchesSupportedMimeTypes(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "packaging", "linux", "pixgo.desktop"))
	if err != nil {
		t.Skip(err)
	}
	var listed []string
	for _, line := range strings.Split(string(data), "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), "MimeType="); ok {
			listed = splitList(v)
		}
	}
	want := append(append([]string(nil), ImageMimeTypes...), ModelMimeTypes...)
	sort.Strings(listed)
	sort.Strings(want)
	if strings.Join(listed, ",") != strings.Join(want, ",") {
		t.Fatalf("desktop file MimeType\n  %v\nplatform.ImageMimeTypes\n  %v", listed, want)
	}
}
