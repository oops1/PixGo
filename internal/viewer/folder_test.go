package viewer

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"
)

func TestOpenFolderFromFileFindsIndex(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"a.jpg", "img2.jpg", "img10.jpg", "z.txt"} {
		writeFile(t, dir, n, 1, time.Time{})
	}
	f, idx, err := OpenFolder(filepath.Join(dir, "img10.jpg"), byExt(".jpg"), SortName, false)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"a.jpg", "img2.jpg", "img10.jpg"}; !slices.Equal(names(f.Entries()), want) {
		t.Fatalf("got %v", names(f.Entries()))
	}
	if idx != 2 || f.Len() != 3 || f.At(idx).Name != "img10.jpg" || f.Dir() != dir {
		t.Fatalf("idx %d, len %d, dir %q", idx, f.Len(), f.Dir())
	}
}

func TestOpenFolderFromDirectoryStartsAtZero(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "b.jpg", 1, time.Time{})
	writeFile(t, dir, "a.jpg", 1, time.Time{})
	f, idx, err := OpenFolder(dir, byExt(".jpg"), SortName, false)
	if err != nil || idx != 0 || f.At(0).Name != "a.jpg" {
		t.Fatalf("idx %d, err %v", idx, err)
	}
}

func TestOpenFolderEmptyDirectory(t *testing.T) {
	f, idx, err := OpenFolder(t.TempDir(), byExt(".jpg"), SortName, false)
	if err != nil || idx != 0 || f.Len() != 0 {
		t.Fatalf("idx %d, len %d, err %v", idx, f.Len(), err)
	}
	if _, ok := NewNavigator(f, idx).Current(); ok {
		t.Fatal("empty folder has no current entry")
	}
}

func TestOpenFolderIncludesRejectedFileAndKeepsItAcrossRescan(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.jpg", 1, time.Time{})
	odd := writeFile(t, dir, "scan.dat", 1, time.Time{})
	writeFile(t, dir, "z.jpg", 1, time.Time{})
	f, idx, err := OpenFolder(odd, byExt(".jpg"), SortName, false)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"a.jpg", "scan.dat", "z.jpg"}; !slices.Equal(names(f.Entries()), want) || idx != 1 {
		t.Fatalf("got %v at %d", names(f.Entries()), idx)
	}
	writeFile(t, dir, "m.jpg", 1, time.Time{})
	changed, err := f.Rescan()
	if err != nil || !changed {
		t.Fatalf("changed %v, err %v", changed, err)
	}
	if want := []string{"a.jpg", "m.jpg", "scan.dat", "z.jpg"}; !slices.Equal(names(f.Entries()), want) {
		t.Fatalf("after rescan: %v", names(f.Entries()))
	}
	if !f.Remove(odd) {
		t.Fatal("Remove failed")
	}
	if _, err := f.Rescan(); err != nil {
		t.Fatal(err)
	}
	if f.IndexOf(odd) != -1 {
		t.Fatal("removed file must not come back through the pinned path")
	}
}

func TestOpenFolderErrors(t *testing.T) {
	if _, _, err := OpenFolder(filepath.Join(t.TempDir(), "missing.jpg"), nil, SortName, false); err == nil {
		t.Fatal("expected an error for a missing path")
	}
}

func TestOpenFolderRelativePath(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.jpg", 1, time.Time{})
	writeFile(t, dir, "b.jpg", 1, time.Time{})
	old, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Skip(err)
	}
	defer os.Chdir(old)
	f, idx, err := OpenFolder("b.jpg", nil, SortName, false)
	if err != nil || idx != 1 || !filepath.IsAbs(f.At(idx).Path) {
		t.Fatalf("idx %d, err %v", idx, err)
	}
}

func TestIndexOfNormalisesPath(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "a.jpg", 1, time.Time{})
	f, _, _ := OpenFolder(dir, nil, SortName, false)
	if f.IndexOf(filepath.Join(dir, "sub", "..", "a.jpg")) != 0 || f.IndexOf(path) != 0 {
		t.Fatal("IndexOf must clean the path")
	}
	if f.IndexOf(filepath.Join(dir, "other.jpg")) != -1 || f.IndexOf("") != -1 {
		t.Fatal("unknown path must give -1")
	}
}

func TestRescanReportsChanges(t *testing.T) {
	dir := t.TempDir()
	a := writeFile(t, dir, "a.jpg", 1, time.Time{})
	writeFile(t, dir, "b.jpg", 1, time.Time{})
	f, _, _ := OpenFolder(dir, nil, SortName, false)
	if changed, err := f.Rescan(); err != nil || changed {
		t.Fatalf("untouched folder: changed %v, err %v", changed, err)
	}
	writeFile(t, dir, "c.jpg", 1, time.Time{})
	if changed, _ := f.Rescan(); !changed || f.Len() != 3 {
		t.Fatalf("added file: changed %v, len %d", changed, f.Len())
	}
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(a, later, later); err != nil {
		t.Fatal(err)
	}
	if changed, _ := f.Rescan(); !changed {
		t.Fatal("modification time change must count")
	}
	writeFile(t, dir, "a.jpg", 99, later)
	if changed, _ := f.Rescan(); !changed || f.At(0).Size != 99 {
		t.Fatalf("size change: %+v", f.At(0))
	}
	if err := os.Remove(a); err != nil {
		t.Fatal(err)
	}
	if changed, _ := f.Rescan(); !changed || f.IndexOf(a) != -1 {
		t.Fatal("deleted file must disappear")
	}
}

func TestRescanErrorKeepsList(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "pics")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, "a.jpg", 1, time.Time{})
	f, _, _ := OpenFolder(dir, nil, SortName, false)
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	changed, err := f.Rescan()
	if err == nil || changed || f.Len() != 1 {
		t.Fatalf("changed %v, err %v, len %d", changed, err, f.Len())
	}
}

func TestRemove(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.jpg", 1, time.Time{})
	b := writeFile(t, dir, "b.jpg", 1, time.Time{})
	writeFile(t, dir, "c.jpg", 1, time.Time{})
	f, _, _ := OpenFolder(dir, nil, SortName, false)
	before := f.Entries()
	if !f.Remove(b) {
		t.Fatal("Remove returned false")
	}
	if f.Remove(b) {
		t.Fatal("second Remove must return false")
	}
	if want := []string{"a.jpg", "c.jpg"}; !slices.Equal(names(f.Entries()), want) {
		t.Fatalf("got %v", names(f.Entries()))
	}
	if len(before) != 3 || before[1].Name != "b.jpg" {
		t.Fatal("previously returned slice must stay intact")
	}
	if _, err := os.Stat(b); err != nil {
		t.Fatal("Remove must not touch the disk")
	}
}

func TestSetSortReorders(t *testing.T) {
	dir := t.TempDir()
	base := time.Date(2023, 3, 3, 0, 0, 0, 0, time.UTC)
	writeFile(t, dir, "a.jpg", 30, base.Add(2*time.Hour))
	writeFile(t, dir, "b.jpg", 10, base.Add(3*time.Hour))
	writeFile(t, dir, "c.jpg", 20, base.Add(1*time.Hour))
	f, _, _ := OpenFolder(dir, nil, SortName, false)
	cases := []struct {
		mode SortMode
		desc bool
		want []string
	}{
		{SortDate, false, []string{"c.jpg", "a.jpg", "b.jpg"}},
		{SortSize, true, []string{"a.jpg", "c.jpg", "b.jpg"}},
		{SortName, true, []string{"c.jpg", "b.jpg", "a.jpg"}},
		{SortName, false, []string{"a.jpg", "b.jpg", "c.jpg"}},
	}
	for _, c := range cases {
		f.SetSort(c.mode, c.desc)
		if !slices.Equal(names(f.Entries()), c.want) {
			t.Errorf("mode %d desc %v: got %v, want %v", c.mode, c.desc, names(f.Entries()), c.want)
		}
		if mode, desc := f.Sort(); mode != c.mode || desc != c.desc {
			t.Errorf("Sort() = %d, %v", mode, desc)
		}
	}
	f.SetSort(SortSize, false)
	writeFile(t, dir, "d.jpg", 5, base)
	if _, err := f.Rescan(); err != nil {
		t.Fatal(err)
	}
	if want := []string{"d.jpg", "b.jpg", "c.jpg", "a.jpg"}; !slices.Equal(names(f.Entries()), want) {
		t.Fatalf("rescan must use the current sort: %v", names(f.Entries()))
	}
}

func TestReplaceResortsForCurrentMode(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.jpg", 1, time.Time{})
	writeFile(t, dir, "b.jpg", 1, time.Time{})
	f, _, _ := OpenFolder(dir, nil, SortName, false)
	scanned, err := f.Scan()
	if err != nil {
		t.Fatal(err)
	}
	f.SetSort(SortName, true)
	if f.Replace(scanned) {
		t.Fatal("the same files in a different input order are not a change")
	}
	if want := []string{"b.jpg", "a.jpg"}; !slices.Equal(names(f.Entries()), want) {
		t.Fatalf("got %v", names(f.Entries()))
	}
	if scanned[0].Name != "a.jpg" {
		t.Fatal("Replace must not reorder the caller's slice")
	}
}

func TestAtOutOfRange(t *testing.T) {
	f, _, _ := OpenFolder(t.TempDir(), nil, SortName, false)
	if f.At(-1) != (Entry{}) || f.At(0) != (Entry{}) || f.At(5) != (Entry{}) {
		t.Fatal("out-of-range At must give the zero Entry")
	}
}

func TestFolderConcurrentUse(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 20; i++ {
		writeFile(t, dir, fmt.Sprintf("p%02d.jpg", i), 1, time.Time{})
	}
	f, _, err := OpenFolder(dir, nil, SortName, false)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				switch (g + i) % 5 {
				case 0:
					entries, err := f.Scan()
					if err == nil {
						f.Replace(entries)
					}
				case 1:
					f.SetSort(SortMode(i%3), i%2 == 0)
				case 2:
					f.Remove(filepath.Join(dir, fmt.Sprintf("p%02d.jpg", i%20)))
				case 3:
					f.IndexOf(filepath.Join(dir, "p05.jpg"))
					f.At(i % 7)
				default:
					for range f.Entries() {
					}
					f.Len()
				}
			}
		}(g)
	}
	wg.Wait()
}

func TestSingleFolderHoldsOnlyTheRequestedFileUntilRescanned(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"a.jpg", "b.jpg", "c.jpg"} {
		writeFile(t, dir, n, 1, time.Time{})
	}
	odd := writeFile(t, dir, "scan.dat", 1, time.Time{})

	f, ok := SingleFolder(odd, byExt(".jpg"), SortName, false)

	if !ok || f.Len() != 1 || f.At(0).Name != "scan.dat" || f.Dir() != dir {
		t.Fatalf("ok %v, len %d, dir %q", ok, f.Len(), f.Dir())
	}
	if _, ok := NewNavigator(f, 0).Current(); !ok {
		t.Fatal("the single entry must be current")
	}
	changed, err := f.Rescan()
	if err != nil || !changed {
		t.Fatalf("rescan changed=%v err=%v", changed, err)
	}
	if want := []string{"a.jpg", "b.jpg", "c.jpg", "scan.dat"}; !slices.Equal(names(f.Entries()), want) {
		t.Fatalf("got %v", names(f.Entries()))
	}
	if f.IndexOf(odd) != 3 {
		t.Fatalf("the requested file moved to %d", f.IndexOf(odd))
	}
}

func TestSingleFolderRejectsMissingFilesAndDirectories(t *testing.T) {
	dir := t.TempDir()
	if _, ok := SingleFolder(filepath.Join(dir, "none.jpg"), byExt(".jpg"), SortName, false); ok {
		t.Fatal("a missing file has no folder")
	}
	if _, ok := SingleFolder(dir, byExt(".jpg"), SortName, false); ok {
		t.Fatal("a directory is not a single file")
	}
}

func TestSingleFolderScanDropsAFileDeletedMeanwhile(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "a.jpg", 1, time.Time{})
	f, _ := SingleFolder(p, byExt(".jpg"), SortName, false)
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}

	entries, err := f.Scan()

	if err != nil || len(entries) != 0 {
		t.Fatalf("entries %v err %v", entries, err)
	}
}
