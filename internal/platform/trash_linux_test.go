//go:build linux

package platform

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func isolateTrash(t *testing.T) (dataHome, work string) {
	t.Helper()
	root := t.TempDir()
	dataHome = filepath.Join(root, "data")
	work = filepath.Join(root, "work")
	for _, d := range []string{dataHome, work} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("XDG_DATA_HOME", dataHome)
	return dataHome, work
}

func writeFile(t *testing.T, p, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readInfo(t *testing.T, trashRoot, name string) (path, date string) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(trashRoot, "info", name+".trashinfo"))
	if err != nil {
		t.Fatal(err)
	}
	if !infoRE.MatchString(string(b)) {
		t.Fatalf("bad .trashinfo: %q", b)
	}
	for _, line := range strings.Split(string(b), "\n") {
		if v, ok := strings.CutPrefix(line, "Path="); ok {
			path, _ = decodeTrashPath(v)
		}
		if v, ok := strings.CutPrefix(line, "DeletionDate="); ok {
			date = v
		}
	}
	return path, date
}

func TestLinuxTrashHome(t *testing.T) {
	dataHome, work := isolateTrash(t)
	tr := NewTrash()
	file := filepath.Join(work, "снимок 1 %20.jpg")
	writeFile(t, file, "pixels")
	if !tr.Available(file) {
		t.Fatal("home trash must be available")
	}
	if err := tr.Trash(context.Background(), file); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(file); !os.IsNotExist(err) {
		t.Fatalf("file still there: %v", err)
	}
	root := filepath.Join(dataHome, "Trash")
	got, err := os.ReadFile(filepath.Join(root, "files", filepath.Base(file)))
	if err != nil || string(got) != "pixels" {
		t.Fatalf("trashed content %q %v", got, err)
	}
	p, date := readInfo(t, root, filepath.Base(file))
	if p != file {
		t.Fatalf("Path = %q, want %q", p, file)
	}
	if _, err := time.ParseInLocation(trashDateLayout, date, time.Local); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{root, filepath.Join(root, "files"), filepath.Join(root, "info")} {
		fi, err := os.Stat(d)
		if err != nil || fi.Mode().Perm() != 0o700 {
			t.Fatalf("%s: %v %v", d, fi, err)
		}
	}
}

func TestLinuxTrashNameCollision(t *testing.T) {
	dataHome, work := isolateTrash(t)
	tr := NewTrash()
	var stored []string
	for i, dir := range []string{"a", "b", "c"} {
		f := filepath.Join(work, dir, "same.jpg")
		writeFile(t, f, "v"+strconv.Itoa(i))
		if err := tr.Trash(context.Background(), f); err != nil {
			t.Fatal(err)
		}
		stored = append(stored, f)
	}
	root := filepath.Join(dataHome, "Trash")
	for i, name := range []string{"same.jpg", "same.2.jpg", "same.3.jpg"} {
		b, err := os.ReadFile(filepath.Join(root, "files", name))
		if err != nil || string(b) != "v"+strconv.Itoa(i) {
			t.Fatalf("%s: %q %v", name, b, err)
		}
		if p, _ := readInfo(t, root, name); p != stored[i] {
			t.Fatalf("%s: Path = %q, want %q", name, p, stored[i])
		}
	}
}

func TestLinuxTrashExistingOrphanFile(t *testing.T) {
	dataHome, work := isolateTrash(t)
	tr := NewTrash()
	root := filepath.Join(dataHome, "Trash")
	writeFile(t, filepath.Join(root, "files", "x.jpg"), "orphan")
	os.MkdirAll(filepath.Join(root, "info"), 0o700)
	f := filepath.Join(work, "x.jpg")
	writeFile(t, f, "new")
	if err := tr.Trash(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(root, "files", "x.jpg")); string(b) != "orphan" {
		t.Fatalf("orphan overwritten: %q", b)
	}
	if b, _ := os.ReadFile(filepath.Join(root, "files", "x.2.jpg")); string(b) != "new" {
		t.Fatalf("new file not stored under the next name: %q", b)
	}
}

func TestLinuxTrashDirectoryAndSymlink(t *testing.T) {
	dataHome, work := isolateTrash(t)
	tr := NewTrash()
	dir := filepath.Join(work, "album")
	writeFile(t, filepath.Join(dir, "sub", "1.jpg"), "1")
	if err := tr.Trash(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(dataHome, "Trash", "files", "album", "sub", "1.jpg")); err != nil || string(b) != "1" {
		t.Fatalf("directory content: %q %v", b, err)
	}

	target := filepath.Join(work, "target.jpg")
	writeFile(t, target, "keep")
	link := filepath.Join(work, "link.jpg")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := tr.Trash(context.Background(), link); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(target); err != nil || string(b) != "keep" {
		t.Fatalf("symlink target touched: %q %v", b, err)
	}
	fi, err := os.Lstat(filepath.Join(dataHome, "Trash", "files", "link.jpg"))
	if err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("the link itself must be in the trash: %v %v", fi, err)
	}
}

func TestLinuxTrashMissingAndRoot(t *testing.T) {
	_, work := isolateTrash(t)
	tr := NewTrash()
	missing := filepath.Join(work, "nope.jpg")
	if tr.Available(missing) {
		t.Error("a missing file has no trash")
	}
	if err := tr.Trash(context.Background(), missing); err == nil {
		t.Error("trashing a missing file succeeded")
	}
	if err := tr.Trash(context.Background(), "/"); err == nil {
		t.Error("trashing the root succeeded")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	f := filepath.Join(work, "c.jpg")
	writeFile(t, f, "x")
	if err := tr.Trash(ctx, f); !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled context: %v", err)
	}
	if _, err := os.Lstat(f); err != nil {
		t.Error("file moved despite cancellation")
	}
}

func TestLinuxTrashRefusesTrashItself(t *testing.T) {
	dataHome, work := isolateTrash(t)
	tr := NewTrash()
	f := filepath.Join(work, "a.jpg")
	writeFile(t, f, "x")
	if err := tr.Trash(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(dataHome, "Trash")
	for _, p := range []string{root, filepath.Join(root, "files"), filepath.Join(root, "files", "a.jpg"), dataHome} {
		if tr.Available(p) {
			t.Errorf("%s reported as trashable", p)
		}
		if err := tr.Trash(context.Background(), p); err == nil {
			t.Errorf("%s was moved into the trash", p)
		}
	}
}

func TestLinuxTrashRejectsSymlinkedTrashDir(t *testing.T) {
	dataHome, work := isolateTrash(t)
	elsewhere := filepath.Join(work, "elsewhere")
	os.MkdirAll(elsewhere, 0o700)
	if err := os.Symlink(elsewhere, filepath.Join(dataHome, "Trash")); err != nil {
		t.Fatal(err)
	}
	tr := NewTrash()
	f := filepath.Join(work, "a.jpg")
	writeFile(t, f, "x")
	if tr.Available(f) {
		t.Error("a symlinked trash directory must not be used")
	}
	err := tr.Trash(context.Background(), f)
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("got %v, want ErrUnavailable", err)
	}
	if _, err := os.Lstat(f); err != nil {
		t.Fatal("file moved into an unsafe trash")
	}
}

func TestLinuxTrashCrossDeviceReportsUnavailable(t *testing.T) {
	dataHome, work := isolateTrash(t)
	tr := &linuxTrash{now: time.Now, rename: func(string, string) error { return unix.EXDEV }}
	f := filepath.Join(work, "big.jpg")
	writeFile(t, f, "x")
	err := tr.Trash(context.Background(), f)
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("got %v, want ErrUnavailable", err)
	}
	if _, err := os.Lstat(f); err != nil {
		t.Fatal("source lost")
	}
	ents, _ := os.ReadDir(filepath.Join(dataHome, "Trash", "info"))
	if len(ents) != 0 {
		t.Fatalf("stale .trashinfo left: %v", ents)
	}
}

func TestLinuxPerVolumeTrashChoice(t *testing.T) {
	top := t.TempDir()
	uid := strconv.Itoa(os.Getuid())

	if loc := perVolumeTrash(top); loc.root != filepath.Join(top, ".Trash-"+uid) || loc.top != top {
		t.Fatalf("no .Trash: %+v", loc)
	}

	shared := filepath.Join(top, ".Trash")
	os.Mkdir(shared, 0o755)
	if loc := perVolumeTrash(top); loc.root != filepath.Join(top, ".Trash-"+uid) {
		t.Fatalf(".Trash without the sticky bit must be ignored: %+v", loc)
	}
	if err := os.Chmod(shared, 0o777|os.ModeSticky); err != nil {
		t.Fatal(err)
	}
	if loc := perVolumeTrash(top); loc.root != filepath.Join(shared, uid) {
		t.Fatalf("sticky .Trash must be used: %+v", loc)
	}

	os.Remove(shared)
	target := filepath.Join(top, "real")
	os.Mkdir(target, 0o777|os.ModeSticky)
	os.Chmod(target, 0o777|os.ModeSticky)
	if err := os.Symlink(target, shared); err != nil {
		t.Fatal(err)
	}
	if loc := perVolumeTrash(top); loc.root != filepath.Join(top, ".Trash-"+uid) {
		t.Fatalf("a symlinked .Trash must be ignored: %+v", loc)
	}
}

func TestLinuxTrashOtherDevice(t *testing.T) {
	const shm = "/dev/shm"
	shmDev, err := devOf(shm)
	if err != nil {
		t.Skip("no /dev/shm")
	}
	dataHome, _ := isolateTrash(t)
	homeDev, err := devOf(dataHome)
	if err != nil || homeDev == shmDev {
		t.Skip("/dev/shm is on the same filesystem as the temp dir")
	}
	work, err := os.MkdirTemp(shm, "pixgo-trash-test-")
	if err != nil {
		t.Skip("/dev/shm is not writable")
	}
	uid := strconv.Itoa(os.Getuid())
	volTrash := filepath.Join(shm, ".Trash-"+uid)
	_, existed := os.Lstat(volTrash)
	t.Cleanup(func() {
		os.RemoveAll(work)
		if existed != nil {
			os.RemoveAll(volTrash)
		}
	})
	if fi, err := os.Lstat(filepath.Join(shm, ".Trash")); err == nil && fi.IsDir() {
		t.Skip("/dev/shm already has a .Trash directory")
	}

	f := filepath.Join(work, "pic.jpg")
	writeFile(t, f, "shm")
	tr := NewTrash()
	if !tr.Available(f) {
		t.Fatal("per-volume trash must be available")
	}
	if err := tr.Trash(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(dataHome, "Trash")); err == nil {
		t.Fatal("home trash used for a file on another filesystem")
	}
	if b, err := os.ReadFile(filepath.Join(volTrash, "files", "pic.jpg")); err != nil || string(b) != "shm" {
		t.Fatalf("content in the volume trash: %q %v", b, err)
	}
	p, _ := readInfo(t, volTrash, "pic.jpg")
	rel, _ := filepath.Rel(shm, f)
	if p != rel {
		t.Fatalf("Path = %q, want path relative to the volume top %q", p, rel)
	}
}
