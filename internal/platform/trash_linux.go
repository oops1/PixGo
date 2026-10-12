//go:build linux

package platform

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

type trashLocation struct {
	root string
	top  string
}

type linuxTrash struct {
	now    func() time.Time
	rename func(from, to string) error
}

func NewTrash() Trash { return &linuxTrash{now: time.Now, rename: renameNoReplace} }

func homeTrashDir() string {
	data := os.Getenv("XDG_DATA_HOME")
	if !filepath.IsAbs(data) {
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			return ""
		}
		data = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(data, "Trash")
}

func renameNoReplace(from, to string) error {
	err := unix.Renameat2(unix.AT_FDCWD, from, unix.AT_FDCWD, to, unix.RENAME_NOREPLACE)
	if errors.Is(err, unix.ENOSYS) || errors.Is(err, unix.EINVAL) {
		if _, serr := os.Lstat(to); serr == nil {
			return unix.EEXIST
		}
		return os.Rename(from, to)
	}
	return err
}

func existingAncestor(p string) string {
	for {
		if _, err := os.Lstat(p); err == nil {
			return p
		}
		parent := filepath.Dir(p)
		if parent == p {
			return p
		}
		p = parent
	}
}

func devOf(p string) (uint64, error) {
	var st unix.Stat_t
	if err := unix.Stat(p, &st); err != nil {
		return 0, err
	}
	return uint64(st.Dev), nil
}

func mountTop(dir string) (string, error) {
	cur := dir
	dev, err := devOf(cur)
	if err != nil {
		return "", err
	}
	for {
		parent := filepath.Dir(cur)
		if parent == cur {
			return cur, nil
		}
		pd, err := devOf(parent)
		if err != nil {
			return "", err
		}
		if pd != dev {
			return cur, nil
		}
		cur = parent
	}
}

func perVolumeTrash(top string) trashLocation {
	uid := strconv.Itoa(os.Getuid())
	shared := filepath.Join(top, ".Trash")
	if fi, err := os.Lstat(shared); err == nil && fi.IsDir() && fi.Mode()&os.ModeSymlink == 0 && fi.Mode()&os.ModeSticky != 0 {
		return trashLocation{root: filepath.Join(shared, uid), top: top}
	}
	return trashLocation{root: filepath.Join(top, ".Trash-"+uid), top: top}
}

func (t *linuxTrash) locate(path string) (trashLocation, string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return trashLocation{}, "", err
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(abs))
	if err != nil {
		return trashLocation{}, "", err
	}
	full := filepath.Join(parent, filepath.Base(abs))
	if filepath.Dir(full) == full {
		return trashLocation{}, "", errors.New("platform: the root cannot be moved to the trash")
	}
	var src unix.Stat_t
	if err := unix.Lstat(full, &src); err != nil {
		return trashLocation{}, "", err
	}
	home := homeTrashDir()
	if home != "" {
		if hd, err := devOf(existingAncestor(home)); err == nil && hd == uint64(src.Dev) {
			return trashLocation{root: home}, full, nil
		}
	}
	top, err := mountTop(filepath.Dir(full))
	if err != nil {
		return trashLocation{}, "", err
	}
	return perVolumeTrash(top), full, nil
}

func trashDirsValid(root string) bool {
	for _, p := range []string{root, filepath.Join(root, "files"), filepath.Join(root, "info")} {
		if !trashDirOK(p) {
			return false
		}
	}
	return true
}

func trashDirOK(p string) bool {
	fi, err := os.Lstat(p)
	if err != nil || !fi.IsDir() || fi.Mode()&os.ModeSymlink != 0 {
		return false
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	return ok && int(st.Uid) == os.Getuid()
}

func ensureTrashDirs(root string) error {
	for _, p := range []string{root, filepath.Join(root, "files"), filepath.Join(root, "info")} {
		if err := os.MkdirAll(p, 0o700); err != nil {
			return fmt.Errorf("platform: trash directory %s: %w", p, ErrUnavailable)
		}
		if !trashDirOK(p) {
			return fmt.Errorf("platform: unsafe trash directory %s: %w", p, ErrUnavailable)
		}
	}
	return nil
}

func canCreate(p string) bool {
	anc := existingAncestor(p)
	fi, err := os.Stat(anc)
	if err != nil || !fi.IsDir() {
		return false
	}
	return unix.Access(anc, unix.W_OK) == nil
}

func (t *linuxTrash) Available(path string) bool {
	loc, full, err := t.locate(path)
	if err != nil || loc.root == "" {
		return false
	}
	if within(loc.root, full) || within(full, loc.root) {
		return false
	}
	if trashDirsValid(loc.root) {
		return true
	}
	if _, err := os.Lstat(loc.root); err == nil {
		return false
	}
	return canCreate(loc.root)
}

func (t *linuxTrash) Trash(ctx context.Context, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	loc, full, err := t.locate(path)
	if err != nil {
		return err
	}
	if loc.root == "" {
		return fmt.Errorf("platform: no trash directory: %w", ErrUnavailable)
	}
	if within(loc.root, full) || within(full, loc.root) {
		return errors.New("platform: the trash or its parent cannot be moved to the trash")
	}
	if err := ensureTrashDirs(loc.root); err != nil {
		return err
	}
	stored := full
	if loc.top != "" {
		if stored, err = filepath.Rel(loc.top, full); err != nil {
			return err
		}
	}
	info := trashInfoText(stored, t.now())
	base := filepath.Base(full)
	for n := 1; n <= 10000; n++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		name := trashCandidate(base, n)
		infoPath := filepath.Join(loc.root, "info", name+".trashinfo")
		f, err := os.OpenFile(infoPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return err
		}
		_, werr := f.WriteString(info)
		if werr == nil {
			werr = f.Sync()
		}
		if cerr := f.Close(); werr == nil {
			werr = cerr
		}
		if werr != nil {
			os.Remove(infoPath)
			return werr
		}
		err = t.rename(full, filepath.Join(loc.root, "files", name))
		if err == nil {
			return nil
		}
		os.Remove(infoPath)
		switch {
		case errors.Is(err, unix.EEXIST), errors.Is(err, unix.ENOTEMPTY):
			continue
		case errors.Is(err, unix.EXDEV):
			return fmt.Errorf("platform: %s is on another filesystem than its trash: %w", full, ErrUnavailable)
		case errors.Is(err, unix.EROFS):
			return fmt.Errorf("platform: %s is read-only: %w", full, ErrUnavailable)
		}
		return err
	}
	return errors.New("platform: no free name in the trash")
}
