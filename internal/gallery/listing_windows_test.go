//go:build windows

package gallery

import (
	"context"
	"os/exec"
	"path/filepath"
	"slices"
	"syscall"
	"testing"
	"time"
)

func setAttributes(t *testing.T, path string, attrs uint32) {
	t.Helper()
	ptr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.SetFileAttributes(ptr, attrs); err != nil {
		t.Fatal(err)
	}
}

func TestListSkipsHiddenAndSystemEntriesOnWindows(t *testing.T) {
	root := t.TempDir()
	touch(t, filepath.Join(root, "visible.jpg"), 1, time.Time{})
	hidden := touch(t, filepath.Join(root, "hidden.jpg"), 1, time.Time{})
	system := touch(t, filepath.Join(root, "system.jpg"), 1, time.Time{})
	hiddenDir := mkdir(t, filepath.Join(root, "hiddendir"))
	touch(t, filepath.Join(hiddenDir, "inside.jpg"), 1, time.Time{})
	mkdir(t, filepath.Join(root, "shown"))
	setAttributes(t, hidden, syscall.FILE_ATTRIBUTE_HIDDEN)
	setAttributes(t, system, syscall.FILE_ATTRIBUTE_SYSTEM)
	setAttributes(t, hiddenDir, syscall.FILE_ATTRIBUTE_HIDDEN)
	l := &Lister{Supports: images, Pictures: root}
	got, err := l.List(context.Background(), Folder(root), Order{})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"shown", "visible.jpg"}; !slices.Equal(names(got.Items), want) {
		t.Fatalf("folder: got %v, want %v", names(got.Items), want)
	}
	all, err := l.List(context.Background(), Location{Kind: KindAll}, Order{})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"visible.jpg"}; !slices.Equal(names(all.Items), want) {
		t.Fatalf("all images: got %v, want %v", names(all.Items), want)
	}
}

func makeJunction(t *testing.T, link, target string) {
	t.Helper()
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput(); err != nil {
		t.Skipf("junctions are not available: %v: %s", err, out)
	}
}

func TestListTreatsJunctionsAsFoldersButDoesNotWalkIntoThem(t *testing.T) {
	root := t.TempDir()
	touch(t, filepath.Join(root, "a.jpg"), 1, time.Time{})
	outside := t.TempDir()
	touch(t, filepath.Join(outside, "out.jpg"), 1, time.Time{})
	makeJunction(t, filepath.Join(root, "linked"), outside)
	l := &Lister{Supports: images, Pictures: root}
	folder, err := l.List(context.Background(), Folder(root), Order{})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"linked", "a.jpg"}; !slices.Equal(names(folder.Items), want) || !folder.Items[0].Dir {
		t.Fatalf("folder: got %v, want %v", names(folder.Items), want)
	}
	all, err := l.List(context.Background(), Location{Kind: KindAll}, Order{})
	if err != nil || !slices.Equal(names(all.Items), []string{"a.jpg"}) {
		t.Fatalf("all images: got %v, %v", names(all.Items), err)
	}
}
