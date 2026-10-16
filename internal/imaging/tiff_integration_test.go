package imaging

import (
	"bytes"
	"encoding/binary"
	"image"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/tiff"
)

const (
	tiffTagWidth      = 256
	tiffTagHeight     = 257
	tiffTagBits       = 258
	tiffTagCompress   = 259
	tiffTagPhoto      = 262
	tiffTagOffsets    = 273
	tiffTagSamples    = 277
	tiffTagRows       = 278
	tiffTagCounts     = 279
	tiffTagPlanar     = 284
	tiffTypeShort     = 3
	tiffTypeLong      = 4
	tiffRowsPerStrip  = 128
	tiffSectionHeader = 8
)

func grayAt(x, y int) uint8 {
	return uint8(x*7 + y*13 + (x>>6)*29 + (y>>6)*17)
}

func writeGrayTIFF(t *testing.T, path string, w, h int) {
	t.Helper()
	strips := (h + tiffRowsPerStrip - 1) / tiffRowsPerStrip
	type entry struct {
		tag, typ uint16
		count    uint32
		value    uint32
	}
	entries := []entry{
		{tiffTagWidth, tiffTypeLong, 1, uint32(w)},
		{tiffTagHeight, tiffTypeLong, 1, uint32(h)},
		{tiffTagBits, tiffTypeShort, 1, 8},
		{tiffTagCompress, tiffTypeShort, 1, 1},
		{tiffTagPhoto, tiffTypeShort, 1, 1},
		{tiffTagOffsets, tiffTypeLong, uint32(strips), 0},
		{tiffTagSamples, tiffTypeShort, 1, 1},
		{tiffTagRows, tiffTypeLong, 1, tiffRowsPerStrip},
		{tiffTagCounts, tiffTypeLong, uint32(strips), 0},
		{tiffTagPlanar, tiffTypeShort, 1, 1},
	}
	ifdSize := 2 + len(entries)*12 + 4
	arrays := tiffSectionHeader + ifdSize
	dataStart := arrays + 2*strips*4
	var buf bytes.Buffer
	le := binary.LittleEndian
	buf.Write([]byte{'I', 'I', 42, 0})
	binary.Write(&buf, le, uint32(tiffSectionHeader))
	binary.Write(&buf, le, uint16(len(entries)))
	for _, e := range entries {
		switch e.tag {
		case tiffTagOffsets:
			e.value = uint32(arrays)
		case tiffTagCounts:
			e.value = uint32(arrays + strips*4)
		}
		binary.Write(&buf, le, e.tag)
		binary.Write(&buf, le, e.typ)
		binary.Write(&buf, le, e.count)
		binary.Write(&buf, le, e.value)
	}
	binary.Write(&buf, le, uint32(0))
	offset := dataStart
	for s := 0; s < strips; s++ {
		binary.Write(&buf, le, uint32(offset))
		offset += min(tiffRowsPerStrip, h-s*tiffRowsPerStrip) * w
	}
	for s := 0; s < strips; s++ {
		binary.Write(&buf, le, uint32(min(tiffRowsPerStrip, h-s*tiffRowsPerStrip)*w))
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.Write(buf.Bytes()); err != nil {
		t.Fatal(err)
	}
	row := make([]byte, w)
	for y := 0; y < h; y++ {
		for x := range row {
			row[x] = grayAt(x, y)
		}
		if _, err := f.Write(row); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLoaderServesTilesFromTheRealTIFFReader(t *testing.T) {
	const w, h = 5000, 4000
	dir := t.TempDir()
	path := filepath.Join(dir, "big.tif")
	writeGrayTIFF(t, path, w, h)
	reg := codec.NewRegistry()
	reg.RegisterDecoder(tiff.Decoder{})
	ui := &uiQueue{}
	lim := codec.DefaultLimits()
	lim.MaxPixels = 4_000_000
	cache := NewCache(1 << 30)
	loader := NewLoader(reg, cache, ui.Post, 2, func() codec.Limits { return lim })
	defer func() {
		loader.Close()
		cache.Clear()
	}()
	loader.Want([]Request{{Path: path, Prio: PrioCurrent}})
	deadline := time.Now().Add(30 * time.Second)
	var d *Decoded
	for d == nil {
		ui.Drain()
		if e, ok := loader.Entry(path); ok {
			if e.State == StateFailed {
				t.Fatalf("load failed: %v", e.Err)
			}
			if e.State == StateReady {
				d = e.Img
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("timeout")
		}
		time.Sleep(time.Millisecond)
	}
	if d.Shift != 1 || d.Detail == nil {
		t.Fatalf("shift %d detail %v", d.Shift, d.Detail)
	}
	src := d.Source()
	if src.Width != w || src.Height != h || src.Levels[0].Bounds().Dx() != 2500 || src.Levels[0].Bounds().Dy() != 2000 {
		t.Fatalf("source %dx%d level %v", src.Width, src.Height, src.Levels[0].Bounds())
	}
	for _, p := range [][2]int{{0, 0}, {10, 10}, {1234, 777}, {2499, 1999}} {
		px := src.Levels[0].NRGBAAt(p[0], p[1])
		var sum int
		for dy := 0; dy < 2; dy++ {
			for dx := 0; dx < 2; dx++ {
				sum += int(grayAt(2*p[0]+dx, 2*p[1]+dy))
			}
		}
		if want := (sum + 2) / 4; abs(int(px.R)-want) > 1 || px.R != px.G || px.A != 255 {
			t.Fatalf("overview pixel %v = %v, want about %d", p, px, want)
		}
	}
	need := image.Rect(1500, 1100, 2900, 2000)
	mgr := d.Detail.(*TileManager)
	var win = d.Detail.Window(0, need)
	for i := 0; i < 20000; i++ {
		win = d.Detail.Window(0, need)
		st := mgr.Stats()
		if st.Pending == 0 && st.Inflight == 0 {
			win = d.Detail.Window(0, need)
			break
		}
		time.Sleep(time.Millisecond)
	}
	if st := mgr.Stats(); st.Failed != 0 || st.Decoded == 0 {
		t.Fatalf("stats %+v", st)
	}
	bad := 0
	for y := 0; y < win.Img.Rect.Dy(); y++ {
		for x := 0; x < win.Img.Rect.Dx(); x++ {
			px := win.Img.NRGBAAt(x, y)
			if want := grayAt(win.X+x, win.Y+y); px.R != want || px.G != want || px.B != want || px.A != 255 {
				bad++
			}
		}
	}
	if bad != 0 {
		t.Fatalf("%d window pixels differ from the file", bad)
	}
	if err := os.Remove(path); err != nil {
		t.Fatalf("an open session must not block deleting the file: %v", err)
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
