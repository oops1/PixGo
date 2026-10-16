package app

import (
	"image/color"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func (h *harness) settleScan() {
	h.t.Helper()
	h.pump(func() bool { return !h.c.Scanning() })
}

func TestOpenShowsTheFileBeforeTheFolderIsScanned(t *testing.T) {
	h := newHarness(t, nil)
	_, paths := makeFolder(t, 5)

	if err := h.c.Open(paths[2]); err != nil {
		t.Fatal(err)
	}

	if h.c.Name() != "img03.png" || h.c.Status() != StatusLoading {
		t.Fatalf("name %q, status %d", h.c.Name(), h.c.Status())
	}
	if h.c.Title() != "img03.png" {
		t.Fatalf("title %q", h.c.Title())
	}
	if _, ok := h.c.loader.Entry(paths[2]); !ok {
		t.Fatal("the requested file was not handed to the loader before the scan")
	}
	if !h.c.Scanning() {
		t.Fatal("the folder must be scanned in the background")
	}
	if i, n := h.c.Position(); i != 1 || n != 1 {
		t.Fatalf("position %d/%d while scanning", i, n)
	}
	if h.c.Step(1) || h.c.Step(-1) || h.c.First() || h.c.Last() {
		t.Fatal("navigation must be refused until the folder is known")
	}
	if h.c.Name() != "img03.png" {
		t.Fatalf("navigation moved to %s", h.c.Name())
	}
}

func TestOpenedFolderAppearsAfterTheScan(t *testing.T) {
	h := newHarness(t, nil)
	_, paths := makeFolder(t, 5)
	h.c.Open(paths[2])
	before := h.events

	h.settleScan()

	if i, n := h.c.Position(); i != 3 || n != 5 {
		t.Fatalf("position %d/%d, want 3/5", i, n)
	}
	if h.c.Name() != "img03.png" || h.c.Scanning() {
		t.Fatalf("name %q, scanning %v", h.c.Name(), h.c.Scanning())
	}
	if h.events == before {
		t.Fatal("the scan result must be announced with OnChange")
	}
	h.pump(func() bool { return h.c.Status() == StatusReady })
	for _, p := range []string{paths[1], paths[3]} {
		p := p
		h.pump(func() bool {
			e, ok := h.c.loader.Entry(p)
			return ok && e.State == 2
		})
	}
	if !h.c.Step(1) || h.c.Name() != "img04.png" {
		t.Fatalf("next after the scan: %s", h.c.Name())
	}
}

func TestScanKeepsTheShownPictureAndTheViewport(t *testing.T) {
	h := newHarness(t, nil)
	_, paths := makeFolder(t, 4)
	h.c.Open(paths[1])
	h.pump(func() bool { return h.c.Status() == StatusReady })
	shown := h.c.Shown()
	h.settleScan()

	if h.c.Shown() != shown || h.c.Status() != StatusReady || h.c.Name() != "img02.png" {
		t.Fatal("the scan replaced the picture that was already shown")
	}
}

func TestSecondOpenCancelsThePendingScan(t *testing.T) {
	h := newHarness(t, nil)
	_, first := makeFolder(t, 3)
	_, second := makeFolder(t, 6)
	h.c.Open(first[1])
	h.c.Open(second[4])

	h.settleScan()

	if i, n := h.c.Position(); i != 5 || n != 6 {
		t.Fatalf("position %d/%d, want 5/6", i, n)
	}
	if filepath.Dir(h.c.Path()) != filepath.Dir(second[0]) || h.c.Folder().Dir() != filepath.Dir(second[0]) {
		t.Fatalf("path %q, folder %q", h.c.Path(), h.c.Folder().Dir())
	}
	h.pump(func() bool { return h.c.Status() == StatusReady })
	if h.c.Name() != "img05.png" {
		t.Fatalf("name %q", h.c.Name())
	}
}

func TestOpeningADirectoryCancelsThePendingFileScan(t *testing.T) {
	h := newHarness(t, nil)
	_, first := makeFolder(t, 3)
	dir, other := makeFolder(t, 4)
	h.c.Open(first[0])

	if err := h.c.Open(dir); err != nil {
		t.Fatal(err)
	}

	if h.c.Scanning() {
		t.Fatal("a directory is listed at once, nothing should be pending")
	}
	h.pump(func() bool { return h.c.Status() == StatusReady })
	if _, n := h.c.Position(); n != 4 || h.c.Path() != other[0] {
		t.Fatalf("count %d, path %q", n, h.c.Path())
	}
}

func TestFileDeletedWhileScanningFallsBackToANeighbour(t *testing.T) {
	h := newHarness(t, nil)
	_, paths := makeFolder(t, 3)
	h.c.Open(paths[1])
	if err := os.Remove(paths[1]); err != nil {
		t.Fatal(err)
	}

	h.settleScan()
	if _, n := h.c.Position(); n == 3 {
		h.c.RescanAsync()
		h.settleScan()
	}

	if _, n := h.c.Position(); n != 2 {
		t.Fatalf("count %d, want 2", n)
	}
	h.pump(func() bool { return h.c.Status() == StatusReady })
	if name := h.c.Name(); name != "img01.png" && name != "img03.png" {
		t.Fatalf("shown %q", name)
	}
}

func TestOpenMissingFileDoesNotStartAScan(t *testing.T) {
	h := newHarness(t, nil)
	_, paths := makeFolder(t, 3)
	h.c.Open(paths[0])

	if err := h.c.Open(filepath.Join(filepath.Dir(paths[0]), "none.png")); err == nil {
		t.Fatal("expected an error")
	}
	h.pump(func() bool { return true })

	if h.c.Status() != StatusFailed {
		t.Fatalf("status %d: a scan of the previous folder replaced the failure", h.c.Status())
	}
}

func TestActivationRescansTheFolder(t *testing.T) {
	h := newHarness(t, nil)
	dir, paths := makeFolder(t, 3)
	h.open(paths[0])
	h.pump(func() bool { return h.c.Status() == StatusReady })
	h.c.SetActive(false)
	writePNG(t, filepath.Join(dir, "img99.png"), 10, 10, color.NRGBA{9, 9, 9, 255})
	if err := os.Remove(paths[2]); err != nil {
		t.Fatal(err)
	}
	h.advance(5 * time.Second)

	h.c.SetActive(true)
	h.settleScan()

	var names []string
	for _, e := range h.c.Folder().Entries() {
		names = append(names, e.Name)
	}
	if len(names) != 3 || names[0] != "img01.png" || names[1] != "img02.png" || names[2] != "img99.png" {
		t.Fatalf("entries after activation: %v", names)
	}
	if h.c.Name() != "img01.png" {
		t.Fatalf("the shown file changed to %s", h.c.Name())
	}
}

func TestActivationRescanIsThrottled(t *testing.T) {
	h := newHarness(t, nil)
	dir, paths := makeFolder(t, 2)
	h.open(paths[0])
	h.advance(10 * time.Second)
	h.c.SetActive(true)
	h.settleScan()
	writePNG(t, filepath.Join(dir, "img50.png"), 10, 10, color.NRGBA{9, 9, 9, 255})

	h.advance(1500 * time.Millisecond)
	h.c.SetActive(false)
	h.c.SetActive(true)
	if h.c.Scanning() {
		t.Fatal("a second activation within 2 s must not rescan")
	}
	h.pump(func() bool { return true })
	if _, n := h.c.Position(); n != 2 {
		t.Fatalf("count %d: the folder was rescanned too early", n)
	}

	h.advance(600 * time.Millisecond)
	h.c.SetActive(true)
	h.settleScan()
	if _, n := h.c.Position(); n != 3 {
		t.Fatalf("count %d, want 3 after the interval", n)
	}
}

func TestActivationWithoutChangesLeavesThePictureAlone(t *testing.T) {
	h := newHarness(t, nil)
	_, paths := makeFolder(t, 3)
	h.open(paths[1])
	h.pump(func() bool { return h.c.Status() == StatusReady })
	shown, events := h.c.Shown(), h.events
	h.advance(10 * time.Second)

	h.c.SetActive(true)
	h.settleScan()

	if h.events != events {
		t.Fatalf("OnChange fired %d times for an unchanged folder", h.events-events)
	}
	if h.c.Shown() != shown || h.c.Status() != StatusReady || h.c.Name() != "img02.png" {
		t.Fatal("the shown picture was disturbed")
	}
}

func TestActivationWhenTheShownFileWasDeletedMovesOn(t *testing.T) {
	h := newHarness(t, nil)
	_, paths := makeFolder(t, 3)
	h.open(paths[1])
	h.pump(func() bool { return h.c.Status() == StatusReady })
	h.c.SetActive(false)
	if err := os.Remove(paths[1]); err != nil {
		t.Fatal(err)
	}
	h.advance(10 * time.Second)

	h.c.SetActive(true)
	h.settleScan()
	h.pump(func() bool { return h.c.Status() == StatusReady && h.c.Name() != "img02.png" })

	if _, n := h.c.Position(); n != 2 {
		t.Fatalf("count %d", n)
	}
}

func TestActivationWithoutAFolderIsHarmless(t *testing.T) {
	h := newHarness(t, nil)

	h.c.SetActive(false)
	h.c.SetActive(true)

	if h.c.Scanning() || h.c.Folder() != nil {
		t.Fatal("nothing is open, nothing to scan")
	}
}

func TestSaveRescanIsNotThrottled(t *testing.T) {
	h := newHarness(t, nil)
	dir, paths := makeFolder(t, 2)
	h.open(paths[0])
	writePNG(t, filepath.Join(dir, "img77.png"), 10, 10, color.NRGBA{9, 9, 9, 255})

	h.c.RescanAsync()
	h.settleScan()

	if _, n := h.c.Position(); n != 3 {
		t.Fatalf("count %d: an explicit rescan must run at once", n)
	}
}

func TestPrevAndNextAreDisabledUntilTheScanFinishes(t *testing.T) {
	a, _ := goldenApp(t, 770, 504, nil)
	a.Root.ForceEnabled = false
	first, _ := twoImages(t)

	a.Ctl.Open(first)

	if a.CanPrev() || a.CanNext() || !a.Ctl.Scanning() {
		t.Fatalf("prev %v, next %v, scanning %v", a.CanPrev(), a.CanNext(), a.Ctl.Scanning())
	}
	deadline := time.Now().Add(5 * time.Second)
	for a.Ctl.Scanning() && time.Now().Before(deadline) {
		a.Eng.Flush()
		time.Sleep(2 * time.Millisecond)
	}
	if !a.CanNext() || a.CanPrev() {
		t.Fatalf("after the scan: prev %v, next %v", a.CanPrev(), a.CanNext())
	}
}

func TestWindowActivationRescansTheFolderThroughTheApp(t *testing.T) {
	a, _ := goldenApp(t, 770, 504, nil)
	first, _ := twoImages(t)
	a.Ctl.Open(first)
	waitName(t, a, "a.jpg")
	for a.Ctl.Scanning() {
		a.Eng.Flush()
		time.Sleep(2 * time.Millisecond)
	}
	a.SetActive(false)
	writeBigJPEG(t, filepath.Join(filepath.Dir(first), "c.jpg"))
	a.Ctl.lastScan = time.Time{}

	a.SetActive(true)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		a.Eng.Flush()
		if _, n := a.Ctl.Position(); n == 3 {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("the new file did not appear after the window was activated")
}
