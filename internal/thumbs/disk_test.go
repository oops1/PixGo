package thumbs

import (
	"bytes"
	"fmt"
	"image/color"
	stdjpeg "image/jpeg"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/oops1/PixGo/internal/codec"
)

func cacheFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		if isCacheName(e.Name()) {
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(out)
	return out
}

func generateOnDisk(t *testing.T, dir string, names ...string) []string {
	t.Helper()
	reg, _ := fakeRegistry()
	hn := newHarness(t, reg, Options{DiskDir: dir})
	paths := hn.writeFake(names...)
	for _, p := range paths {
		if th := hn.get(p, 128); th.Err != nil {
			t.Fatal(th.Err)
		}
	}
	hn.svc.Close()
	return paths
}

func TestKeyFileName(t *testing.T) {
	base := Key{Path: "/a/b.jpg", Size: 100, ModTime: time.Unix(1700000000, 5), Side: 128}
	name := base.File()
	if !isCacheName(name) || len(name) != 20 || !strings.HasSuffix(name, ".jpg") {
		t.Fatalf("name %q", name)
	}
	if base.File() != name {
		t.Fatal("not deterministic")
	}
	variants := map[string]Key{
		"path":  {Path: "/a/c.jpg", Size: 100, ModTime: base.ModTime, Side: 128},
		"size":  {Path: base.Path, Size: 101, ModTime: base.ModTime, Side: 128},
		"mtime": {Path: base.Path, Size: 100, ModTime: base.ModTime.Add(time.Nanosecond), Side: 128},
		"side":  {Path: base.Path, Size: 100, ModTime: base.ModTime, Side: 256},
	}
	for what, k := range variants {
		if k.File() == name {
			t.Errorf("changing the %s did not change the file name", what)
		}
	}
	if same := (Key{Path: base.Path, Size: 100, ModTime: base.ModTime.UTC(), Side: 128}); same.File() != name {
		t.Error("the file name must not depend on the time zone")
	}
	if isCacheName("short.jpg") || isCacheName("0123456789abcdeg.jpg") || isCacheName("0123456789abcdef.png") || isCacheName("0123456789ABCDEF.jpg") {
		t.Error("isCacheName accepts a foreign name")
	}
}

func TestSecondInstanceReadsFromDisk(t *testing.T) {
	dir := t.TempDir()
	reg1, f1 := fakeRegistry()
	f1.orient = codec.OrientRotate90
	hn1 := newHarness(t, reg1, Options{DiskDir: dir})
	p := hn1.writeFake("pic")[0]
	first := hn1.get(p, 128)
	if first.Err != nil {
		t.Fatal(first.Err)
	}
	hn1.svc.Close()
	if n := len(cacheFiles(t, dir)); n != 1 {
		t.Fatalf("%d cache files after the first run, want 1", n)
	}

	reg2, f2 := fakeRegistry()
	hn2 := newHarness(t, reg2, Options{DiskDir: dir})
	hn2.dir = hn1.dir
	second := hn2.get(p, 128)
	if second.Err != nil {
		t.Fatal(second.Err)
	}
	if f2.decodeCount("pic") != 0 || f2.probes["pic"] != 0 {
		t.Fatalf("second instance touched the codec: decodes=%d probes=%d", f2.decodeCount("pic"), f2.probes["pic"])
	}
	if second.Img.Bounds() != first.Img.Bounds() {
		t.Fatalf("size %v vs %v", second.Img.Bounds(), first.Img.Bounds())
	}
	if second.SrcW != 600 || second.SrcH != 400 || second.Orientation != codec.OrientRotate90 {
		t.Fatalf("metadata lost on disk: %+v", second)
	}
	a, b := pixelAt(first.Img, 20, 40), pixelAt(second.Img, 20, 40)
	if !near(a.R, b.R, 6) || !near(a.G, b.G, 6) || !near(a.B, b.B, 6) || b.A != 255 {
		t.Fatalf("pixel differs after the round trip: %v vs %v", a, b)
	}
}

func TestDiskFileIsAJPEGWithMarker(t *testing.T) {
	dir := t.TempDir()
	generateOnDisk(t, dir, "m")
	files := cacheFiles(t, dir)
	if len(files) != 1 {
		t.Fatalf("files %v", files)
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(data, []byte{0xff, 0xd8, 0xff, 0xfe}) {
		t.Fatalf("file starts with %x, want SOI followed by COM", data[:4])
	}
	if !bytes.Contains(data[:64], []byte("pixgo-thumb v1 600 400 1")) {
		t.Fatalf("marker not found in %q", data[:64])
	}
	im, err := stdjpeg.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if im.Bounds().Dx() != 128 || im.Bounds().Dy() != 85 {
		t.Fatalf("size %v", im.Bounds())
	}
	if w, h, o, ok := parseCom(data); !ok || w != 600 || h != 400 || o != codec.OrientNormal {
		t.Fatalf("parseCom = %d %d %d %v", w, h, o, ok)
	}
}

func TestParseCom(t *testing.T) {
	build := func(text string) []byte {
		seg := []byte{0xff, 0xd8, 0xff, 0xfe, byte((len(text) + 2) >> 8), byte(len(text) + 2)}
		seg = append(seg, text...)
		return append(seg, 0xff, 0xd9)
	}
	cases := []struct {
		name string
		data []byte
		ok   bool
		w, h int
		o    codec.Orientation
	}{
		{"with orientation", build("pixgo-thumb v1 6000 4000 6"), true, 6000, 4000, 6},
		{"without orientation", build("pixgo-thumb v1 6000 4000"), true, 6000, 4000, 1},
		{"bad orientation", build("pixgo-thumb v1 10 20 9"), true, 10, 20, 1},
		{"wrong version", build("pixgo-thumb v2 10 20 1"), false, 0, 0, 0},
		{"foreign comment", build("hello"), false, 0, 0, 0},
		{"zero size", build("pixgo-thumb v1 0 20 1"), false, 0, 0, 0},
		{"garbage numbers", build("pixgo-thumb v1 x y"), false, 0, 0, 0},
		{"not a jpeg", []byte("hello world"), false, 0, 0, 0},
		{"truncated segment", []byte{0xff, 0xd8, 0xff, 0xfe, 0xff, 0xff, 'a'}, false, 0, 0, 0},
		{"empty", nil, false, 0, 0, 0},
	}
	for _, tc := range cases {
		w, h, o, ok := parseCom(tc.data)
		if ok != tc.ok || w != tc.w || h != tc.h || (ok && o != tc.o) {
			t.Errorf("%s: parseCom = %d %d %d %v", tc.name, w, h, o, ok)
		}
	}
}

func TestMtimeChangeMissesTheDiskCache(t *testing.T) {
	dir := t.TempDir()
	paths := generateOnDisk(t, dir, "mt")
	before := cacheFiles(t, dir)
	st, err := os.Stat(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	mt := st.ModTime().Add(time.Hour)
	if err := os.Chtimes(paths[0], mt, mt); err != nil {
		t.Fatal(err)
	}
	reg, f := fakeRegistry()
	hn := newHarness(t, reg, Options{DiskDir: dir})
	hn.dir = filepath.Dir(paths[0])
	if th := hn.get(paths[0], 128); th.Err != nil {
		t.Fatal(th.Err)
	}
	hn.svc.Close()
	if f.decodeCount("mt") != 1 {
		t.Fatalf("decodes = %d, a changed mtime must not hit the old cache entry", f.decodeCount("mt"))
	}
	after := cacheFiles(t, dir)
	if len(after) != len(before)+1 {
		t.Fatalf("cache files %d -> %d, want a new entry next to the stale one", len(before), len(after))
	}
}

func TestOtherSideMissesTheDiskCache(t *testing.T) {
	dir := t.TempDir()
	paths := generateOnDisk(t, dir, "sd")
	reg, f := fakeRegistry()
	hn := newHarness(t, reg, Options{DiskDir: dir})
	if th := hn.get(paths[0], 192); th.Err != nil {
		t.Fatal(th.Err)
	}
	if f.decodeCount("sd") != 1 {
		t.Fatal("a different side must be generated separately")
	}
}

func TestCorruptCacheFilesAreRegenerated(t *testing.T) {
	corrupt := map[string]func(t *testing.T, path string){
		"truncated": func(t *testing.T, path string) {
			data, _ := os.ReadFile(path)
			if err := os.WriteFile(path, data[:len(data)/2], 0o600); err != nil {
				t.Fatal(err)
			}
		},
		"garbage": func(t *testing.T, path string) {
			if err := os.WriteFile(path, bytes.Repeat([]byte{0x42}, 500), 0o600); err != nil {
				t.Fatal(err)
			}
		},
		"empty": func(t *testing.T, path string) {
			if err := os.WriteFile(path, nil, 0o600); err != nil {
				t.Fatal(err)
			}
		},
		"valid jpeg without marker": func(t *testing.T, path string) {
			if err := os.WriteFile(path, jpegBytes(t, solid(128, 85, colRed), 80), 0o600); err != nil {
				t.Fatal(err)
			}
		},
		"valid jpeg larger than the side": func(t *testing.T, path string) {
			data := jpegBytes(t, solid(300, 200, colRed), 80)
			out := append([]byte{}, data[:2]...)
			out = append(out, comSegment(Thumb{SrcW: 600, SrcH: 400, Orientation: 1})...)
			out = append(out, data[2:]...)
			if err := os.WriteFile(path, out, 0o600); err != nil {
				t.Fatal(err)
			}
		},
	}
	for name, damage := range corrupt {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			paths := generateOnDisk(t, dir, "cc")
			files := cacheFiles(t, dir)
			if len(files) != 1 {
				t.Fatalf("files %v", files)
			}
			damage(t, files[0])
			reg, f := fakeRegistry()
			hn := newHarness(t, reg, Options{DiskDir: dir})
			th := hn.get(paths[0], 128)
			if th.Err != nil || th.Img == nil {
				t.Fatalf("corrupt cache entry must be ignored: %+v", th)
			}
			hn.svc.Close()
			if f.decodeCount("cc") != 1 {
				t.Fatalf("decodes = %d, want regeneration", f.decodeCount("cc"))
			}
			data, err := os.ReadFile(files[0])
			if err != nil {
				t.Fatalf("entry not rewritten: %v", err)
			}
			if _, ok := decodeCached(data, 128); !ok {
				t.Fatal("rewritten entry is not valid")
			}
		})
	}
}

func TestTranslucentThumbnailsStayOffTheDisk(t *testing.T) {
	dir := t.TempDir()
	reg, _ := jpegRegistry()
	hn := newHarness(t, reg, Options{DiskDir: dir})
	translucent := hn.write("t.png", pngBytes(t, solid(64, 64, color.NRGBA{R: 1, G: 2, B: 3, A: 200})))
	opaque := hn.write("o.png", pngBytes(t, solid(64, 64, color.NRGBA{R: 1, G: 2, B: 3, A: 255})))
	hn.get(translucent, 32)
	hn.get(opaque, 32)
	hn.svc.Close()
	if n := len(cacheFiles(t, dir)); n != 1 {
		t.Fatalf("%d cache files, want only the opaque thumbnail", n)
	}
}

func TestFailuresAreNotWrittenToDisk(t *testing.T) {
	dir := t.TempDir()
	reg, _ := jpegRegistry()
	hn := newHarness(t, reg, Options{DiskDir: dir})
	p := hn.write("bad.jpg", []byte{0xff, 0xd8, 0xff, 0x00, 0x01})
	if th := hn.get(p, 64); th.Err == nil {
		t.Fatal("expected a failure")
	}
	hn.svc.Close()
	if n := len(cacheFiles(t, dir)); n != 0 {
		t.Fatalf("%d cache files for a failed decode", n)
	}
}

func fillCache(t *testing.T, dir string, n, size int, base time.Time) []string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	var paths []string
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("%016x.jpg", uint64(i)+1)
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, bytes.Repeat([]byte{byte(i)}, size), 0o600); err != nil {
			t.Fatal(err)
		}
		mt := base.Add(time.Duration(i) * time.Minute)
		if err := os.Chtimes(p, mt, mt); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, p)
	}
	return paths
}

func TestPruneRemovesOldestFirstAndKeepsUsageUnderLimit(t *testing.T) {
	dir := t.TempDir()
	d := &diskCache{dir: dir}
	paths := fillCache(t, dir, 40, 1000, time.Now().Add(-24*time.Hour))
	other := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(other, bytes.Repeat([]byte{1}, 5000), 0o600); err != nil {
		t.Fatal(err)
	}
	const limit = 25000
	if got := d.usage(); got != 40000 {
		t.Fatalf("usage = %d, want 40000", got)
	}
	d.prune(limit, time.Now())
	if got := d.usage(); got > limit {
		t.Fatalf("usage %d exceeds the limit %d", got, limit)
	}
	if got := d.usage(); got < limit/2 {
		t.Fatalf("usage %d: pruned far more than necessary", got)
	}
	if _, err := os.Stat(paths[0]); err == nil {
		t.Fatal("the oldest file survived")
	}
	if _, err := os.Stat(paths[len(paths)-1]); err != nil {
		t.Fatal("the newest file was removed")
	}
	if _, err := os.Stat(other); err != nil {
		t.Fatal("a foreign file was removed")
	}
	removed := 0
	for i, p := range paths {
		if _, err := os.Stat(p); err != nil {
			removed++
			if i != removed-1 {
				t.Fatalf("file %d removed out of order", i)
			}
		}
	}
	if removed == 0 {
		t.Fatal("nothing pruned")
	}
}

func TestPruneUnderLimitDoesNothing(t *testing.T) {
	dir := t.TempDir()
	d := &diskCache{dir: dir}
	fillCache(t, dir, 5, 100, time.Now())
	d.prune(10000, time.Now())
	if d.usage() != 500 {
		t.Fatalf("usage = %d", d.usage())
	}
}

func TestPruneSweepsStaleTempFiles(t *testing.T) {
	dir := t.TempDir()
	d := &diskCache{dir: dir}
	old := filepath.Join(dir, ".0000000000000001.jpg.tmp-123")
	fresh := filepath.Join(dir, ".0000000000000002.jpg.tmp-456")
	for _, p := range []string{old, fresh} {
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	past := time.Now().Add(-3 * time.Hour)
	if err := os.Chtimes(old, past, past); err != nil {
		t.Fatal(err)
	}
	d.prune(1<<20, time.Now())
	if _, err := os.Stat(old); err == nil {
		t.Fatal("stale temp file survived")
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Fatal("a fresh temp file may belong to a running write and must stay")
	}
}

func TestStartupPruneRunsInTheBackground(t *testing.T) {
	dir := t.TempDir()
	fillCache(t, dir, 30, 1000, time.Now().Add(-time.Hour))
	reg, _ := fakeRegistry()
	hn := newHarness(t, reg, Options{DiskDir: dir, DiskLimitBytes: 10000})
	deadline := time.Now().Add(10 * time.Second)
	for hn.svc.DiskUsage() > 10000 {
		if time.Now().After(deadline) {
			t.Fatalf("usage still %d", hn.svc.DiskUsage())
		}
		time.Sleep(time.Millisecond)
	}
}

func TestPruneIsThrottled(t *testing.T) {
	dir := t.TempDir()
	reg, _ := fakeRegistry()
	hn := newHarness(t, reg, Options{DiskDir: dir, DiskLimitBytes: 10000})
	deadline := time.Now().Add(10 * time.Second)
	for hn.svc.lastPrune.Load() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("startup prune never ran")
		}
		time.Sleep(time.Millisecond)
	}
	fillCache(t, dir, 30, 1000, time.Now().Add(-time.Hour))
	hn.clock.advance(10 * time.Second)
	hn.svc.maybePrune(false)
	if hn.svc.DiskUsage() != 30000 {
		t.Fatalf("pruned within 30 s of the previous check: usage %d", hn.svc.DiskUsage())
	}
	hn.clock.advance(25 * time.Second)
	hn.svc.maybePrune(false)
	if got := hn.svc.DiskUsage(); got > 10000 {
		t.Fatalf("usage %d after the throttle window", got)
	}
}

func TestWritesTriggerPruneAfterTheWindow(t *testing.T) {
	dir := t.TempDir()
	reg, _ := fakeRegistry()
	hn := newHarness(t, reg, Options{DiskDir: dir, DiskLimitBytes: 4000, Workers: 1})
	deadline := time.Now().Add(10 * time.Second)
	for hn.svc.lastPrune.Load() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("startup prune never ran")
		}
		time.Sleep(time.Millisecond)
	}
	var names []string
	for i := 0; i < 40; i++ {
		names = append(names, fmt.Sprintf("img%02d", i))
	}
	paths := hn.writeFake(names...)
	for _, p := range paths[:20] {
		hn.get(p, 128)
	}
	hn.clock.advance(time.Minute)
	for _, p := range paths[20:] {
		hn.get(p, 128)
	}
	hn.svc.Close()
	st, err := os.Stat(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	oldest := filepath.Join(dir, Key{Path: paths[0], Size: st.Size(), ModTime: st.ModTime(), Side: 128}.File())
	if _, err := os.Stat(oldest); err == nil {
		t.Fatal("the oldest thumbnail survived although the limit was exceeded after the window")
	}
	if n := len(cacheFiles(t, dir)); n >= len(paths) {
		t.Fatalf("%d cache files for %d images: nothing was pruned", n, len(paths))
	}
}

func TestClearDisk(t *testing.T) {
	dir := t.TempDir()
	reg, _ := fakeRegistry()
	hn := newHarness(t, reg, Options{DiskDir: dir})
	paths := hn.writeFake("x1", "x2", "x3")
	for _, p := range paths {
		hn.get(p, 128)
	}
	hn.svc.Close()
	if hn.svc.DiskUsage() <= 0 {
		t.Fatal("nothing on disk to clear")
	}
	other := filepath.Join(dir, "keep.txt")
	if err := os.WriteFile(other, []byte("mine"), 0o600); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := hn.svc.ClearDisk(); err != nil {
		t.Fatal(err)
	}
	if got := hn.svc.DiskUsage(); got != 0 {
		t.Fatalf("usage after ClearDisk = %d", got)
	}
	if n := len(cacheFiles(t, dir)); n != 0 {
		t.Fatalf("%d cache files left", n)
	}
	if _, err := os.Stat(other); err != nil {
		t.Fatal("ClearDisk removed a file that is not part of the cache")
	}
	if _, err := os.Stat(sub); err != nil {
		t.Fatal("ClearDisk removed a directory")
	}
}

func TestClearDiskWithMissingDirectory(t *testing.T) {
	reg, _ := fakeRegistry()
	hn := newHarness(t, reg, Options{DiskDir: filepath.Join(t.TempDir(), "never", "created")})
	if err := hn.svc.ClearDisk(); err != nil {
		t.Fatal(err)
	}
	if hn.svc.DiskUsage() != 0 {
		t.Fatal("usage of a missing directory must be zero")
	}
}

func TestClearDiskThenRegenerate(t *testing.T) {
	dir := t.TempDir()
	reg, f := fakeRegistry()
	hn := newHarness(t, reg, Options{DiskDir: dir, MemoryEntries: 1})
	paths := hn.writeFake("a", "b")
	hn.get(paths[0], 128)
	hn.get(paths[1], 128)
	deadline := time.Now().Add(10 * time.Second)
	for len(cacheFiles(t, dir)) < 2 {
		if time.Now().After(deadline) {
			t.Fatal("cache files not written")
		}
		time.Sleep(time.Millisecond)
	}
	if err := hn.svc.ClearDisk(); err != nil {
		t.Fatal(err)
	}
	hn.get(paths[0], 128)
	if f.decodeCount("a") != 2 {
		t.Fatalf("decodes = %d, want regeneration after ClearDisk", f.decodeCount("a"))
	}
}

func TestMemoryEvictionRefillsFromDisk(t *testing.T) {
	dir := t.TempDir()
	reg, f := fakeRegistry()
	hn := newHarness(t, reg, Options{DiskDir: dir, MemoryEntries: 1})
	paths := hn.writeFake("a", "b")
	hn.get(paths[0], 128)
	hn.get(paths[1], 128)
	deadline := time.Now().Add(10 * time.Second)
	for len(cacheFiles(t, dir)) < 2 {
		if time.Now().After(deadline) {
			t.Fatal("cache files not written")
		}
		time.Sleep(time.Millisecond)
	}
	th := hn.get(paths[0], 128)
	if th.Err != nil || th.Img == nil {
		t.Fatalf("%+v", th)
	}
	if f.decodeCount("a") != 1 {
		t.Fatalf("decodes = %d, an evicted entry must come back from disk", f.decodeCount("a"))
	}
}

func TestDiskDirIsCreatedOnDemand(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "deep", "thumbs")
	generateOnDisk(t, dir, "z")
	if n := len(cacheFiles(t, dir)); n != 1 {
		t.Fatalf("%d cache files", n)
	}
}
