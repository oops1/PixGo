package gallery

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oops1/PixGo/internal/viewer"
)

var images = byExt(".jpg", ".png")

func folderFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, d := range []string{"b-dir", "a-dir", "Dir10", "Dir2", ".hidden-dir"} {
		mkdir(t, filepath.Join(root, d))
	}
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	touch(t, filepath.Join(root, "img10.jpg"), 300, base.Add(1*time.Hour))
	touch(t, filepath.Join(root, "img2.jpg"), 100, base.Add(3*time.Hour))
	touch(t, filepath.Join(root, "a.png"), 200, base.Add(2*time.Hour))
	touch(t, filepath.Join(root, "notes.txt"), 1, time.Time{})
	touch(t, filepath.Join(root, ".dot.jpg"), 1, time.Time{})
	return root
}

func TestListFolderPutsFoldersFirstThenFilesInOrder(t *testing.T) {
	root := folderFixture(t)
	l := &Lister{Supports: images}
	cases := []struct {
		name  string
		order Order
		want  []string
	}{
		{"by name", Order{}, []string{"a-dir", "b-dir", "Dir2", "Dir10", "a.png", "img2.jpg", "img10.jpg"}},
		{"by name descending reverses folders too", Order{Desc: true}, []string{"Dir10", "Dir2", "b-dir", "a-dir", "img10.jpg", "img2.jpg", "a.png"}},
		{"by date", Order{Mode: viewer.SortDate}, []string{"a-dir", "b-dir", "Dir2", "Dir10", "img10.jpg", "a.png", "img2.jpg"}},
		{"by date descending keeps folders by name", Order{Mode: viewer.SortDate, Desc: true}, []string{"a-dir", "b-dir", "Dir2", "Dir10", "img2.jpg", "a.png", "img10.jpg"}},
		{"by size", Order{Mode: viewer.SortSize}, []string{"a-dir", "b-dir", "Dir2", "Dir10", "img2.jpg", "a.png", "img10.jpg"}},
		{"by size descending", Order{Mode: viewer.SortSize, Desc: true}, []string{"a-dir", "b-dir", "Dir2", "Dir10", "img10.jpg", "a.png", "img2.jpg"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := l.List(context.Background(), Folder(root), tc.order)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(names(got.Items), tc.want) {
				t.Fatalf("got %v, want %v", names(got.Items), tc.want)
			}
			if got.Loc != Folder(root) || got.Truncated {
				t.Fatalf("loc %+v, truncated %v", got.Loc, got.Truncated)
			}
		})
	}
}

func TestListFolderFillsItemFields(t *testing.T) {
	root := t.TempDir()
	mtime := time.Date(2023, 5, 6, 7, 8, 9, 0, time.UTC)
	file := touch(t, filepath.Join(root, "p.jpg"), 321, mtime)
	dir := mkdir(t, filepath.Join(root, "sub"))
	l := &Lister{Supports: images}
	got, err := l.List(context.Background(), Folder(root), Order{})
	if err != nil || len(got.Items) != 2 {
		t.Fatalf("items %v, err %v", got.Items, err)
	}
	folder, pic := got.Items[0], got.Items[1]
	if !folder.Dir || folder.Name != "sub" || folder.Path != dir || folder.Size != 0 {
		t.Fatalf("folder item %+v", folder)
	}
	if pic.Dir || pic.Name != "p.jpg" || pic.Path != file || pic.Size != 321 || !pic.ModTime.Equal(mtime) {
		t.Fatalf("file item %+v", pic)
	}
}

func TestListFolderWithoutSupportsShowsEveryFile(t *testing.T) {
	root := folderFixture(t)
	got, err := (&Lister{}).List(context.Background(), Folder(root), Order{})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(names(got.Items), "notes.txt") || slices.Contains(names(got.Items), ".dot.jpg") {
		t.Fatalf("got %v", names(got.Items))
	}
}

func TestListFolderHidesDotEntries(t *testing.T) {
	root := folderFixture(t)
	got, err := (&Lister{Supports: images}).List(context.Background(), Folder(root), Order{})
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range names(got.Items) {
		if n == ".hidden-dir" || n == ".dot.jpg" {
			t.Fatalf("hidden entry %q listed", n)
		}
	}
}

func TestListFolderEmptyFolderIsNotAnError(t *testing.T) {
	got, err := (&Lister{Supports: images}).List(context.Background(), Folder(t.TempDir()), Order{})
	if err != nil || len(got.Items) != 0 {
		t.Fatalf("items %v, err %v", got.Items, err)
	}
}

func TestListFolderReturnsReadErrors(t *testing.T) {
	root := t.TempDir()
	file := touch(t, filepath.Join(root, "f.jpg"), 1, time.Time{})
	cases := []struct {
		name string
		loc  Location
	}{
		{"missing folder", Folder(filepath.Join(root, "missing"))},
		{"a file instead of a folder", Folder(file)},
		{"empty path", Folder("")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := (&Lister{}).List(context.Background(), tc.loc, Order{}); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
	_, err := (&Lister{}).List(context.Background(), Folder(filepath.Join(root, "missing")), Order{})
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("error %v must wrap fs.ErrNotExist", err)
	}
}

func TestListRejectsUnknownKind(t *testing.T) {
	_, err := (&Lister{}).List(context.Background(), Location{Kind: Kind(99)}, Order{})
	if !errors.Is(err, ErrUnknownLocation) {
		t.Fatalf("err = %v", err)
	}
}

func TestListStopsWhenContextIsAlreadyCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, loc := range []Location{Folder(t.TempDir()), {Kind: KindAll}, {Kind: KindRecent}, {Kind: KindFavorites}} {
		if _, err := (&Lister{Pictures: t.TempDir()}).List(ctx, loc, Order{}); !errors.Is(err, context.Canceled) {
			t.Fatalf("%+v: err = %v", loc, err)
		}
	}
}

func TestListFolderStopsWhenCancelledMidway(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 3*cancelCheckEvery; i++ {
		touch(t, filepath.Join(root, "f"+string(rune('a'+i%26))+string(rune('a'+i/26))+".jpg"), 1, time.Time{})
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	l := &Lister{Supports: func(string) bool { cancel(); return true }}
	if _, err := l.List(ctx, Folder(root), Order{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
}

func TestClassifySortsEntriesIntoKinds(t *testing.T) {
	root := t.TempDir()
	touch(t, filepath.Join(root, "real.jpg"), 7, time.Time{})
	mkdir(t, filepath.Join(root, "realdir"))
	pngOnly := &Lister{Supports: byExt(".png")}
	l := &Lister{Supports: images}
	info := fakeInfo{name: "x"}
	cases := []struct {
		name       string
		lister     *Lister
		entry      fakeEntry
		wantKind   entryKind
		wantLinked bool
		wantDir    bool
	}{
		{"named pipe", l, fakeEntry{name: "pipe.jpg", typ: fs.ModeNamedPipe, info: info}, kindSkip, false, false},
		{"socket", l, fakeEntry{name: "sock.jpg", typ: fs.ModeSocket, info: info}, kindSkip, false, false},
		{"device", l, fakeEntry{name: "dev.jpg", typ: fs.ModeDevice, info: info}, kindSkip, false, false},
		{"unsupported regular file", l, fakeEntry{name: "a.txt", info: info}, kindSkip, false, false},
		{"regular file whose info fails", l, fakeEntry{name: "b.jpg", infoErr: errors.New("gone")}, kindSkip, false, false},
		{"folder whose info fails", l, fakeEntry{name: "d", typ: fs.ModeDir, infoErr: errors.New("gone")}, kindSkip, false, false},
		{"dot entry", l, fakeEntry{name: ".x.jpg", info: info}, kindSkip, false, false},
		{"link to a supported file", l, fakeEntry{name: "real.jpg", typ: fs.ModeSymlink, info: info}, kindFile, true, false},
		{"link to an unsupported file", pngOnly, fakeEntry{name: "real.jpg", typ: fs.ModeSymlink, info: info}, kindSkip, false, false},
		{"link to a folder", pngOnly, fakeEntry{name: "realdir", typ: fs.ModeSymlink, info: info}, kindDir, true, true},
		{"irregular entry resolving to a file", l, fakeEntry{name: "real.jpg", typ: fs.ModeIrregular, info: info}, kindFile, true, false},
		{"broken link", l, fakeEntry{name: "nowhere.jpg", typ: fs.ModeSymlink, info: info}, kindSkip, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			item, kind, linked := tc.lister.classify(root, tc.entry)
			if kind != tc.wantKind || linked != tc.wantLinked || (kind != kindSkip && item.Dir != tc.wantDir) {
				t.Fatalf("kind %d, linked %v, item %+v; want kind %d, linked %v, dir %v", kind, linked, item, tc.wantKind, tc.wantLinked, tc.wantDir)
			}
		})
	}
}

func TestClassifyLinkedFileTakesSizeFromTarget(t *testing.T) {
	root := t.TempDir()
	touch(t, filepath.Join(root, "real.jpg"), 42, time.Time{})
	l := &Lister{Supports: images}
	item, kind, linked := l.classify(root, fakeEntry{name: "real.jpg", typ: fs.ModeSymlink, info: fakeInfo{name: "real.jpg", size: 1}})
	if kind != kindFile || !linked || item.Size != 42 {
		t.Fatalf("kind %d, linked %v, item %+v", kind, linked, item)
	}
}

func TestListFolderFollowsSymlinks(t *testing.T) {
	root := t.TempDir()
	target := touch(t, filepath.Join(root, "targets", "real.jpg"), 5, time.Time{})
	targetDir := mkdir(t, filepath.Join(root, "targets", "inner"))
	view := mkdir(t, filepath.Join(root, "view"))
	if err := os.Symlink(target, filepath.Join(view, "link.jpg")); err != nil {
		t.Skipf("symlinks are not available: %v", err)
	}
	if err := os.Symlink(targetDir, filepath.Join(view, "linkdir")); err != nil {
		t.Skipf("symlinks are not available: %v", err)
	}
	if err := os.Symlink(filepath.Join(root, "missing"), filepath.Join(view, "broken.jpg")); err != nil {
		t.Skipf("symlinks are not available: %v", err)
	}
	got, err := (&Lister{Supports: images}).List(context.Background(), Folder(view), Order{})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"linkdir", "link.jpg"}; !slices.Equal(names(got.Items), want) {
		t.Fatalf("got %v, want %v", names(got.Items), want)
	}
	if !got.Items[0].Dir || got.Items[1].Size != 5 {
		t.Fatalf("items %+v", got.Items)
	}
}

func allFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	touch(t, filepath.Join(root, "a.jpg"), 1, time.Time{})
	touch(t, filepath.Join(root, "notes.txt"), 1, time.Time{})
	touch(t, filepath.Join(root, "sub1", "b.jpg"), 1, time.Time{})
	touch(t, filepath.Join(root, "sub1", "sub2", "c.png"), 1, time.Time{})
	touch(t, filepath.Join(root, "sub1", "sub2", "sub3", "d.jpg"), 1, time.Time{})
	touch(t, filepath.Join(root, "sub1", "sub2", "sub3", "sub4", "e.jpg"), 1, time.Time{})
	touch(t, filepath.Join(root, ".hidden", "f.jpg"), 1, time.Time{})
	return root
}

func TestListAllWalksDownToMaxDepthWithoutFolders(t *testing.T) {
	root := allFixture(t)
	cases := []struct {
		name  string
		depth int
		want  []string
	}{
		{"default depth is three levels", 0, []string{"a.jpg", "sub1/b.jpg", "sub1/sub2/c.png", "sub1/sub2/sub3/d.jpg"}},
		{"depth one", 1, []string{"a.jpg", "sub1/b.jpg"}},
		{"depth four reaches the deepest file", 4, []string{"a.jpg", "sub1/b.jpg", "sub1/sub2/c.png", "sub1/sub2/sub3/d.jpg", "sub1/sub2/sub3/sub4/e.jpg"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			l := &Lister{Supports: images, Pictures: root, MaxDepth: tc.depth}
			got, err := l.List(context.Background(), Location{Kind: KindAll}, Order{})
			if err != nil {
				t.Fatal(err)
			}
			if rel := relPaths(t, root, got.Items); !slices.Equal(rel, tc.want) {
				t.Fatalf("got %v, want %v", rel, tc.want)
			}
			for _, it := range got.Items {
				if it.Dir {
					t.Fatalf("folder %q in the result", it.Name)
				}
			}
			if got.Truncated || got.Loc.Kind != KindAll {
				t.Fatalf("truncated %v, loc %+v", got.Truncated, got.Loc)
			}
		})
	}
}

func TestListAllAppliesOrderAcrossFolders(t *testing.T) {
	root := t.TempDir()
	base := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
	touch(t, filepath.Join(root, "x", "old.jpg"), 1, base)
	touch(t, filepath.Join(root, "y", "new.jpg"), 1, base.Add(2*time.Hour))
	touch(t, filepath.Join(root, "mid.jpg"), 1, base.Add(time.Hour))
	l := &Lister{Supports: images, Pictures: root}
	got, err := l.List(context.Background(), Location{Kind: KindAll}, Order{Mode: viewer.SortDate, Desc: true})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"new.jpg", "mid.jpg", "old.jpg"}; !slices.Equal(names(got.Items), want) {
		t.Fatalf("got %v, want %v", names(got.Items), want)
	}
}

func TestListAllTruncatesAtMaxFiles(t *testing.T) {
	root := t.TempDir()
	for _, n := range []string{"a.jpg", "b.jpg", "c.jpg"} {
		touch(t, filepath.Join(root, n), 1, time.Time{})
	}
	touch(t, filepath.Join(root, "sub", "d.jpg"), 1, time.Time{})
	cases := []struct {
		name      string
		maxFiles  int
		want      []string
		truncated bool
	}{
		{"fewer files than the limit", 10, []string{"a.jpg", "b.jpg", "c.jpg", "d.jpg"}, false},
		{"exactly the limit is not truncated", 4, []string{"a.jpg", "b.jpg", "c.jpg", "d.jpg"}, false},
		{"one over the limit", 3, []string{"a.jpg", "b.jpg", "c.jpg"}, true},
		{"shallow files win over deep ones", 2, []string{"a.jpg", "b.jpg"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			l := &Lister{Supports: images, Pictures: root, MaxFiles: tc.maxFiles}
			got, err := l.List(context.Background(), Location{Kind: KindAll}, Order{})
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(names(got.Items), tc.want) || got.Truncated != tc.truncated || got.Limit != tc.maxFiles {
				t.Fatalf("names %v, truncated %v, limit %d; want %v, %v, %d", names(got.Items), got.Truncated, got.Limit, tc.want, tc.truncated, tc.maxFiles)
			}
		})
	}
}

func TestListAllUsesDefaultLimits(t *testing.T) {
	root := allFixture(t)
	got, err := (&Lister{Supports: images, Pictures: root}).List(context.Background(), Location{Kind: KindAll}, Order{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Limit != DefaultMaxFiles {
		t.Fatalf("limit %d, want %d", got.Limit, DefaultMaxFiles)
	}
}

func TestListerLimitsFallBackToDefaults(t *testing.T) {
	cases := []struct {
		name         string
		depth, files int
		wantD, wantF int
	}{
		{"zero values", 0, 0, DefaultMaxDepth, DefaultMaxFiles},
		{"negative values", -2, -9, DefaultMaxDepth, DefaultMaxFiles},
		{"explicit values", 5, 20, 5, 20},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, f := (&Lister{MaxDepth: tc.depth, MaxFiles: tc.files}).limits()
			if d != tc.wantD || f != tc.wantF {
				t.Fatalf("limits = %d, %d; want %d, %d", d, f, tc.wantD, tc.wantF)
			}
		})
	}
	if DefaultMaxDepth != 3 || DefaultMaxFiles != 5000 {
		t.Fatal("documented defaults changed")
	}
}

func TestListAllRequiresReadablePicturesFolder(t *testing.T) {
	ctx := context.Background()
	if _, err := (&Lister{}).List(ctx, Location{Kind: KindAll}, Order{}); !errors.Is(err, ErrNoPictures) {
		t.Fatalf("unset root: %v", err)
	}
	missing := filepath.Join(t.TempDir(), "missing")
	if _, err := (&Lister{Pictures: missing}).List(ctx, Location{Kind: KindAll}, Order{}); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing root: %v", err)
	}
}

func TestListAllCachesUntilInvalidated(t *testing.T) {
	root := t.TempDir()
	touch(t, filepath.Join(root, "a.jpg"), 1, time.Time{})
	var scans atomic.Int32
	l := &Lister{Pictures: root, Supports: func(path string) bool { scans.Add(1); return images(path) }}
	list := func() []string {
		got, err := l.List(context.Background(), Location{Kind: KindAll}, Order{})
		if err != nil {
			t.Fatal(err)
		}
		return names(got.Items)
	}
	if got := list(); !slices.Equal(got, []string{"a.jpg"}) {
		t.Fatalf("first: %v", got)
	}
	first := scans.Load()
	touch(t, filepath.Join(root, "b.jpg"), 1, time.Time{})
	if got := list(); !slices.Equal(got, []string{"a.jpg"}) || scans.Load() != first {
		t.Fatalf("cached listing changed: %v, scans %d -> %d", got, first, scans.Load())
	}
	l.InvalidateAll()
	if got := list(); !slices.Equal(got, []string{"a.jpg", "b.jpg"}) {
		t.Fatalf("after invalidation: %v", got)
	}
}

func TestListAllCacheIsKeyedByRootAndLimits(t *testing.T) {
	first, second := allFixture(t), t.TempDir()
	touch(t, filepath.Join(second, "only.jpg"), 1, time.Time{})
	l := &Lister{Supports: images, Pictures: first}
	ctx := context.Background()
	a, err := l.List(ctx, Location{Kind: KindAll}, Order{})
	if err != nil || len(a.Items) != 4 {
		t.Fatalf("first root: %v, %v", names(a.Items), err)
	}
	l.Pictures = second
	b, err := l.List(ctx, Location{Kind: KindAll}, Order{})
	if err != nil || !slices.Equal(names(b.Items), []string{"only.jpg"}) {
		t.Fatalf("second root: %v, %v", names(b.Items), err)
	}
	l.Pictures, l.MaxDepth = first, 1
	c, err := l.List(ctx, Location{Kind: KindAll}, Order{})
	if err != nil || len(c.Items) != 2 {
		t.Fatalf("changed depth: %v, %v", names(c.Items), err)
	}
}

func TestListAllReturnsIndependentCopies(t *testing.T) {
	root := allFixture(t)
	l := &Lister{Supports: images, Pictures: root}
	ctx := context.Background()
	first, err := l.List(ctx, Location{Kind: KindAll}, Order{})
	if err != nil {
		t.Fatal(err)
	}
	first.Items[0].Name = "mutated"
	slices.Reverse(first.Items)
	second, err := l.List(ctx, Location{Kind: KindAll}, Order{})
	if err != nil || second.Items[0].Name != "a.jpg" {
		t.Fatalf("cache was corrupted: %v, %v", names(second.Items), err)
	}
}

func TestListAllDoesNotCacheCancelledWalks(t *testing.T) {
	root := allFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	l := &Lister{Pictures: root, Supports: func(path string) bool { cancel(); return images(path) }}
	if _, err := l.List(ctx, Location{Kind: KindAll}, Order{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
	l.Supports = images
	got, err := l.List(context.Background(), Location{Kind: KindAll}, Order{})
	if err != nil || len(got.Items) != 4 {
		t.Fatalf("after cancellation: %v, %v", names(got.Items), err)
	}
}

func TestListAllStopsMidDirectory(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 3*cancelCheckEvery; i++ {
		touch(t, filepath.Join(root, "f"+string(rune('a'+i%26))+string(rune('a'+i/26))+".jpg"), 1, time.Time{})
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	l := &Lister{Pictures: root, Supports: func(string) bool { cancel(); return true }}
	if _, err := l.List(ctx, Location{Kind: KindAll}, Order{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
}

func TestListAllSkipsFoldersThatVanishDuringTheWalk(t *testing.T) {
	root := t.TempDir()
	touch(t, filepath.Join(root, "a.jpg"), 1, time.Time{})
	mkdir(t, filepath.Join(root, "mdir"))
	touch(t, filepath.Join(root, "zz.jpg"), 1, time.Time{})
	l := &Lister{Pictures: root, Supports: func(path string) bool {
		if filepath.Base(path) == "zz.jpg" {
			os.RemoveAll(filepath.Join(root, "mdir"))
		}
		return images(path)
	}}
	got, err := l.List(context.Background(), Location{Kind: KindAll}, Order{})
	if err != nil || !slices.Equal(names(got.Items), []string{"a.jpg", "zz.jpg"}) {
		t.Fatalf("got %v, %v", names(got.Items), err)
	}
}

func TestListAllDoesNotDescendIntoSymlinkedFolders(t *testing.T) {
	root := t.TempDir()
	touch(t, filepath.Join(root, "a.jpg"), 1, time.Time{})
	outside := t.TempDir()
	touch(t, filepath.Join(outside, "out.jpg"), 1, time.Time{})
	if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
		t.Skipf("symlinks are not available: %v", err)
	}
	got, err := (&Lister{Supports: images, Pictures: root}).List(context.Background(), Location{Kind: KindAll}, Order{})
	if err != nil || !slices.Equal(names(got.Items), []string{"a.jpg"}) {
		t.Fatalf("got %v, %v", names(got.Items), err)
	}
}

func TestListAllIsSafeForConcurrentUse(t *testing.T) {
	root := allFixture(t)
	l := &Lister{Supports: images, Pictures: root}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				if j%7 == 0 {
					l.InvalidateAll()
				}
				got, err := l.List(context.Background(), Location{Kind: KindAll}, Order{})
				if err != nil || len(got.Items) != 4 {
					t.Errorf("got %d items, err %v", len(got.Items), err)
					return
				}
			}
		}()
	}
	wg.Wait()
}

func TestListRecentAndFavoritesFollowListOrder(t *testing.T) {
	root := t.TempDir()
	a := touch(t, filepath.Join(root, "a.jpg"), 11, time.Time{})
	b := touch(t, filepath.Join(root, "b.jpg"), 22, time.Time{})
	c := touch(t, filepath.Join(root, "c.png"), 33, time.Time{})
	missing := filepath.Join(root, "gone.jpg")
	folder := mkdir(t, filepath.Join(root, "folder.jpg"))
	txt := touch(t, filepath.Join(root, "notes.txt"), 1, time.Time{})
	recent, favorites := viewer.NewPathList(0), viewer.NewPathList(0)
	for _, p := range []string{txt, a, missing, folder, b, c} {
		recent.Add(p)
	}
	for _, p := range []string{c, a} {
		favorites.Add(p)
	}
	l := &Lister{Supports: images, Recent: recent, Favorites: favorites}
	ctx := context.Background()
	got, err := l.List(ctx, Location{Kind: KindRecent}, Order{})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"c.png", "b.jpg", "a.jpg"}; !slices.Equal(names(got.Items), want) {
		t.Fatalf("recent: got %v, want %v", names(got.Items), want)
	}
	if got.Items[0].Size != 33 || got.Items[0].Path != c || got.Items[0].Dir || got.Loc.Kind != KindRecent {
		t.Fatalf("item %+v, loc %+v", got.Items[0], got.Loc)
	}
	got, err = l.List(ctx, Location{Kind: KindFavorites}, Order{})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"a.jpg", "c.png"}; !slices.Equal(names(got.Items), want) {
		t.Fatalf("favorites: got %v, want %v", names(got.Items), want)
	}
}

func TestListRecentWithoutSupportsKeepsEveryExistingFile(t *testing.T) {
	root := t.TempDir()
	txt := touch(t, filepath.Join(root, "notes.txt"), 1, time.Time{})
	recent := viewer.NewPathList(0)
	recent.Add(txt)
	got, err := (&Lister{Recent: recent}).List(context.Background(), Location{Kind: KindRecent}, Order{})
	if err != nil || !slices.Equal(names(got.Items), []string{"notes.txt"}) {
		t.Fatalf("got %v, %v", names(got.Items), err)
	}
}

func TestListRecentAndFavoritesWithoutListsAreEmpty(t *testing.T) {
	l := &Lister{}
	for _, loc := range []Location{{Kind: KindRecent}, {Kind: KindFavorites}} {
		got, err := l.List(context.Background(), loc, Order{})
		if err != nil || len(got.Items) != 0 || got.Loc != loc {
			t.Fatalf("%+v: %+v, %v", loc, got, err)
		}
	}
}

func TestListRecentStopsWhenCancelledMidway(t *testing.T) {
	root := t.TempDir()
	recent := viewer.NewPathList(0)
	for i := 0; i < 2*cancelCheckEvery; i++ {
		recent.Add(filepath.Join(root, "f"+string(rune('a'+i%26))+string(rune('a'+i/26))+".jpg"))
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	l := &Lister{Recent: recent, Supports: func(string) bool { cancel(); return false }}
	if _, err := l.List(ctx, Location{Kind: KindRecent}, Order{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
}

func TestItemEntryCarriesFileFields(t *testing.T) {
	mtime := time.Date(2022, 2, 2, 2, 2, 2, 0, time.UTC)
	it := Item{Name: "a.jpg", Path: "/x/a.jpg", Size: 9, ModTime: mtime}
	want := viewer.Entry{Path: "/x/a.jpg", Name: "a.jpg", Size: 9, ModTime: mtime}
	if got := it.Entry(); got != want {
		t.Fatalf("Entry = %+v, want %+v", got, want)
	}
}
