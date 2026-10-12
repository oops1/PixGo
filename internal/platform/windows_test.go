//go:build windows

package platform

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unsafe"
)

func TestSaveDialogSetup(t *testing.T) {
	fs := []FileFilter{
		{Name: "JPEG", Patterns: []string{"*.jpg", "*.jpeg"}},
		{Name: "PNG", Patterns: []string{"*.png"}},
		{Name: "Все файлы", Patterns: []string{"*.*"}},
	}
	err := onSTA(func() error {
		d, err := coCreate(&clsidFileSaveDialog, &iidIFileSaveDialog)
		if err != nil {
			return err
		}
		defer d.release()
		keep, err := setupDialog(d, "T", os.TempDir(), "a.png", fs, 1, fosOverwritePrompt)
		if err != nil {
			return err
		}
		_ = keep
		var ti uint32
		if hr := d.call(fdGetFileTypeIndex, uintptr(unsafe.Pointer(&ti))); failed(hr) || ti != 2 {
			t.Errorf("file type index %d hr %x", ti, hr)
		}
		var opts uint32
		d.call(fdGetOptions, uintptr(unsafe.Pointer(&opts)))
		if opts&fosOverwritePrompt == 0 {
			t.Errorf("options %x", opts)
		}
		dir, _ := filepath.Abs(os.TempDir())
		if long, err := filepath.EvalSymlinks(dir); err == nil {
			dir = long
		}
		it, err := shellItem(dir)
		if err != nil {
			return err
		}
		defer it.release()
		if p, err := itemPath(it); err != nil || !strings.EqualFold(filepath.Clean(p), filepath.Clean(dir)) {
			t.Errorf("itemPath %q %v, want %q", p, err, dir)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSaveDialogEnvOverride(t *testing.T) {
	t.Setenv("PIXGO_OWN_DIALOGS", "1")
	if _, ok := NewSaveDialog().(unsupportedSaveDialog); !ok {
		t.Fatal("PIXGO_OWN_DIALOGS=1 must hand the dialog to the engine")
	}
	t.Setenv("PIXGO_OWN_DIALOGS", "")
	if _, ok := NewSaveDialog().(winSaveDialog); !ok {
		t.Fatal("native dialog expected")
	}
}

func TestRegThemeSmoke(t *testing.T) {
	th := NewSystemTheme()
	c, ok := th.Accent()
	t.Logf("dark=%v hc=%v reduceMotion=%v accent=%v/%v", th.Dark(), th.HighContrast(), th.ReduceMotion(), c, ok)
	stop := th.Watch(func() {})
	stop()
	stop()
}

func TestWindowsFolders(t *testing.T) {
	f := KnownFolderPaths()
	for name, p := range map[string]string{
		"Home": f.Home, "Desktop": f.Desktop, "Documents": f.Documents,
		"Downloads": f.Downloads, "Pictures": f.Pictures, "Videos": f.Videos,
	} {
		if !filepath.IsAbs(p) {
			t.Errorf("%s = %q is not absolute", name, p)
		}
	}
	t.Logf("%+v", f)
}

func TestWindowsVolumes(t *testing.T) {
	vs := NewVolumes().List()
	t.Logf("%+v", vs)
	var found bool
	for _, v := range vs {
		if strings.EqualFold(v.Path, `C:\`) {
			found = true
			if !strings.Contains(v.Name, "C:") {
				t.Errorf("name %q lacks the drive letter", v.Name)
			}
		}
	}
	if !found {
		t.Error("C: not listed")
	}
}

func TestWindowsTrashPathChecks(t *testing.T) {
	for _, bad := range []string{
		`\\server\share\a.jpg`, `C:\`, `C:\dir\*.jpg`, `C:\dir\a?.jpg`,
		`C:\` + strings.Repeat("d", 300) + `\a.jpg`,
	} {
		if _, err := recyclablePath(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	if p, err := recyclablePath(`C:\Users\x\a.jpg`); err != nil || p != `C:\Users\x\a.jpg` {
		t.Errorf("%q %v", p, err)
	}
	u, err := doubleNulPath(`C:\a`)
	if err != nil || len(u) < 3 || u[len(u)-1] != 0 || u[len(u)-2] != 0 {
		t.Errorf("not double-NUL terminated: %v %v", u, err)
	}
	tr := NewTrash()
	if tr.Available(filepath.Join(t.TempDir(), "missing.jpg")) {
		t.Error("a missing file must not be reported as recyclable")
	}
	f := filepath.Join(t.TempDir(), "present.jpg")
	os.WriteFile(f, []byte("x"), 0o644)
	if !tr.Available(f) {
		t.Skip("the temp directory is not on a fixed drive with a recycle bin")
	}
	if err := tr.Trash(context.Background(), `\\server\share\a.jpg`); err == nil {
		t.Error("UNC path accepted")
	}
	if _, err := os.Stat(f); err != nil {
		t.Error("untouched file vanished")
	}
}

func manual(t *testing.T) {
	t.Helper()
	if os.Getenv("PIXGO_MANUAL") != "1" {
		t.Skip("set PIXGO_MANUAL=1")
	}
}

func TestManualRecycleBin(t *testing.T) {
	manual(t)
	f := filepath.Join(t.TempDir(), "pixgo-recycle-test.txt")
	if err := os.WriteFile(f, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	tr := NewTrash()
	if !tr.Available(f) {
		t.Fatal("recycle bin not available for the temp dir")
	}
	if err := tr.Trash(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(f); !os.IsNotExist(err) {
		t.Fatalf("file still exists: %v", err)
	}
	t.Log("check the Recycle Bin for pixgo-recycle-test.txt")
}

func TestManualSaveDialog(t *testing.T) {
	manual(t)
	path, ok, err := NewSaveDialog().Save(SaveOptions{
		Title: "Сохранить как", Dir: os.TempDir(), Name: "picture.png", FilterIndex: 1,
		Filters: []FileFilter{
			{Name: "JPEG", Patterns: []string{"*.jpg", "*.jpeg"}},
			{Name: "PNG", Patterns: []string{"*.png"}},
			{Name: "Все файлы", Patterns: []string{"*.*"}},
		},
	})
	t.Logf("Save: %q %v %v", path, ok, err)
	if err != nil {
		t.Fatal(err)
	}
}

func TestManualAssociations(t *testing.T) {
	manual(t)
	if err := NewAssociations().OpenDefaultsSettings(); err != nil {
		t.Fatal(err)
	}
}
