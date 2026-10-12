package viewer

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestCompareNaturalOrder(t *testing.T) {
	ordered := [][]string{
		{"img1", "img2", "img10", "img11", "img100"},
		{"IMG2", "img10"},
		{"a", "B", "c"},
		{"file", "file1", "file2"},
		{"_x", "1", "a"},
		{"a 1", "a1"},
		{"a1b", "a01c"},
		{"img01", "img1"},
		{"x99999999999999999999", "x100000000000000000000"},
		{"Файл2", "файл10"},
		{"е", "ё", "ж"},
		{"Ё1", "ё2"},
		{"ь", "ю", "я"},
		{"Ä", "ä"},
		{"a", "ab", "b"},
		{"1.jpg", "2.jpg", "10.jpg"},
		{"photo (2).jpg", "photo (10).jpg"},
	}
	for _, group := range ordered {
		for i := 0; i+1 < len(group); i++ {
			a, b := group[i], group[i+1]
			if got := CompareNatural(a, b); got >= 0 {
				t.Errorf("CompareNatural(%q, %q) = %d, want < 0", a, b, got)
			}
			if got := CompareNatural(b, a); got <= 0 {
				t.Errorf("CompareNatural(%q, %q) = %d, want > 0", b, a, got)
			}
		}
	}
	for _, s := range []string{"", "a", "img10", "Ёж"} {
		if got := CompareNatural(s, s); got != 0 {
			t.Errorf("CompareNatural(%q, itself) = %d", s, got)
		}
	}
}

func TestCompareNaturalIsStrictOrder(t *testing.T) {
	alphabet := []string{"a", "A", "b", "1", "2", "0", "10", "01", " ", "_", "ä", "Ä", "я", "Я", "9", "99"}
	rng := rand.New(rand.NewSource(3))
	names := make([]string, 400)
	for i := range names {
		var sb strings.Builder
		for n := rng.Intn(6); n >= 0; n-- {
			sb.WriteString(alphabet[rng.Intn(len(alphabet))])
		}
		names[i] = sb.String()
	}
	for i := 0; i < 20000; i++ {
		a, b, c := names[rng.Intn(len(names))], names[rng.Intn(len(names))], names[rng.Intn(len(names))]
		ab, ba := CompareNatural(a, b), CompareNatural(b, a)
		if (ab < 0) != (ba > 0) || (ab == 0) != (ba == 0) {
			t.Fatalf("not antisymmetric: %q %q -> %d %d", a, b, ab, ba)
		}
		if ab == 0 && a != b {
			t.Fatalf("distinct names compare equal: %q %q", a, b)
		}
		if ab <= 0 && CompareNatural(b, c) <= 0 && CompareNatural(a, c) > 0 {
			t.Fatalf("not transitive: %q <= %q <= %q but %q > %q", a, b, c, a, c)
		}
	}
}

func TestCompareNaturalInvalidUTF8(t *testing.T) {
	a, b := "a\xffb", "a\xfec"
	if CompareNatural(a, b) == 0 || CompareNatural(a, b) != -CompareNatural(b, a) {
		t.Fatal("invalid UTF-8 names must still order consistently")
	}
}

func writeFile(t *testing.T, dir, name string, size int, mtime time.Time) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}
	if !mtime.IsZero() {
		if err := os.Chtimes(path, mtime, mtime); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func names(entries []Entry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Name
	}
	return out
}

func byExt(exts ...string) func(string) bool {
	return func(path string) bool { return slices.Contains(exts, strings.ToLower(filepath.Ext(path))) }
}

func TestScanDirFiltersAndSortsByName(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"img10.jpg", "img2.jpg", "Img1.JPG", "notes.txt", ".hidden.jpg", "b.png"} {
		writeFile(t, dir, n, 1, time.Time{})
	}
	if err := os.Mkdir(filepath.Join(dir, "sub.jpg"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := ScanDir(dir, byExt(".jpg", ".png"), SortName, false)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"b.png", "Img1.JPG", "img2.jpg", "img10.jpg"}
	if !slices.Equal(names(got), want) {
		t.Fatalf("got %v, want %v", names(got), want)
	}
	for _, e := range got {
		if e.Path != filepath.Join(dir, e.Name) || e.Size != 1 || e.ModTime.IsZero() {
			t.Errorf("bad entry %+v", e)
		}
	}
	desc, err := ScanDir(dir, byExt(".jpg", ".png"), SortName, true)
	if err != nil {
		t.Fatal(err)
	}
	slices.Reverse(want)
	if !slices.Equal(names(desc), want) {
		t.Fatalf("desc got %v, want %v", names(desc), want)
	}
}

func TestScanDirNilSupportsKeepsEveryFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.txt", 1, time.Time{})
	writeFile(t, dir, "b.jpg", 1, time.Time{})
	got, err := ScanDir(dir, nil, SortName, false)
	if err != nil || len(got) != 2 {
		t.Fatalf("got %v, %v", names(got), err)
	}
}

func TestScanDirSortByDateAndSize(t *testing.T) {
	dir := t.TempDir()
	base := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	writeFile(t, dir, "c.jpg", 300, base.Add(1*time.Hour))
	writeFile(t, dir, "a.jpg", 100, base.Add(3*time.Hour))
	writeFile(t, dir, "b.jpg", 200, base.Add(2*time.Hour))
	writeFile(t, dir, "d.jpg", 200, base.Add(2*time.Hour))
	cases := []struct {
		mode SortMode
		desc bool
		want []string
	}{
		{SortDate, false, []string{"c.jpg", "b.jpg", "d.jpg", "a.jpg"}},
		{SortDate, true, []string{"a.jpg", "b.jpg", "d.jpg", "c.jpg"}},
		{SortSize, false, []string{"a.jpg", "b.jpg", "d.jpg", "c.jpg"}},
		{SortSize, true, []string{"c.jpg", "b.jpg", "d.jpg", "a.jpg"}},
	}
	for _, c := range cases {
		got, err := ScanDir(dir, nil, c.mode, c.desc)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(names(got), c.want) {
			t.Errorf("mode %d desc %v: got %v, want %v", c.mode, c.desc, names(got), c.want)
		}
	}
}

func TestScanDirDateTieBreakIsNatural(t *testing.T) {
	dir := t.TempDir()
	same := time.Date(2024, 5, 5, 5, 5, 5, 0, time.UTC)
	for _, n := range []string{"burst10.jpg", "burst2.jpg", "burst1.jpg"} {
		writeFile(t, dir, n, 1, same)
	}
	for _, desc := range []bool{false, true} {
		got, _ := ScanDir(dir, nil, SortDate, desc)
		want := []string{"burst1.jpg", "burst2.jpg", "burst10.jpg"}
		if !slices.Equal(names(got), want) {
			t.Errorf("desc=%v got %v", desc, names(got))
		}
	}
}

func TestScanDirMissingDirectory(t *testing.T) {
	if _, err := ScanDir(filepath.Join(t.TempDir(), "nope"), nil, SortName, false); err == nil {
		t.Fatal("expected an error")
	}
}

func TestScanDirFollowsFileSymlinks(t *testing.T) {
	dir := t.TempDir()
	target := writeFile(t, dir, "real.jpg", 7, time.Time{})
	sub := filepath.Join(dir, "elsewhere")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "link.jpg")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := os.Symlink(sub, filepath.Join(dir, "dirlink.jpg")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := os.Symlink(filepath.Join(dir, "missing.jpg"), filepath.Join(dir, "broken.jpg")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	got, err := ScanDir(dir, byExt(".jpg"), SortName, false)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"link.jpg", "real.jpg"}; !slices.Equal(names(got), want) {
		t.Fatalf("got %v, want %v", names(got), want)
	}
	for _, e := range got {
		if e.Size != 7 {
			t.Errorf("%s: size %d, want the target size 7", e.Name, e.Size)
		}
	}
}

func TestScanDirManyFilesSortedNaturally(t *testing.T) {
	dir := t.TempDir()
	const n = 300
	for i := 0; i < n; i++ {
		writeFile(t, dir, fmt.Sprintf("p%d.jpg", i), 0, time.Time{})
	}
	got, err := ScanDir(dir, nil, SortName, false)
	if err != nil || len(got) != n {
		t.Fatalf("got %d entries, %v", len(got), err)
	}
	for i, e := range got {
		if want := fmt.Sprintf("p%d.jpg", i); e.Name != want {
			t.Fatalf("position %d: %s, want %s", i, e.Name, want)
		}
	}
}

func BenchmarkSortNames50k(b *testing.B) {
	rng := rand.New(rand.NewSource(1))
	base := make([]Entry, 50000)
	for i := range base {
		name := fmt.Sprintf("IMG_%05d.jpg", i)
		base[i] = Entry{Path: "/p/" + name, Name: name}
	}
	rng.Shuffle(len(base), func(i, j int) { base[i], base[j] = base[j], base[i] })
	work := make([]Entry, len(base))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		copy(work, base)
		sortEntries(work, SortName, false)
	}
}
