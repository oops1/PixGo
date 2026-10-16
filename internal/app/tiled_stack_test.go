package app

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/oops1/PixGo/internal/codec/tiff"
	"github.com/oops1/PixGo/internal/imaging"
	"github.com/oops1/PixGo/internal/settings"
)

const stackRows = 128

func stackGray(x, y int) uint8 {
	return uint8(x*7 + y*13 + (x>>6)*29 + (y>>6)*17)
}

func writeStackTIFF(t *testing.T, path string, w, h int) {
	t.Helper()
	strips := (h + stackRows - 1) / stackRows
	type entry struct {
		tag, typ uint16
		count    uint32
		value    uint32
	}
	entries := []entry{
		{256, 4, 1, uint32(w)},
		{257, 4, 1, uint32(h)},
		{258, 3, 1, 8},
		{259, 3, 1, 1},
		{262, 3, 1, 1},
		{273, 4, uint32(strips), 0},
		{277, 3, 1, 1},
		{278, 4, 1, stackRows},
		{279, 4, uint32(strips), 0},
		{284, 3, 1, 1},
	}
	arrays := 8 + 2 + len(entries)*12 + 4
	dataStart := arrays + 2*strips*4
	var buf bytes.Buffer
	le := binary.LittleEndian
	buf.Write([]byte{'I', 'I', 42, 0})
	binary.Write(&buf, le, uint32(8))
	binary.Write(&buf, le, uint16(len(entries)))
	for _, e := range entries {
		switch e.tag {
		case 273:
			e.value = uint32(arrays)
		case 279:
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
		offset += min(stackRows, h-s*stackRows) * w
	}
	for s := 0; s < strips; s++ {
		binary.Write(&buf, le, uint32(min(stackRows, h-s*stackRows)*w))
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
			row[x] = stackGray(x, y)
		}
		if _, err := f.Write(row); err != nil {
			t.Fatal(err)
		}
	}
}

func TestFullStackShowsRealTilesAtActualSize(t *testing.T) {
	const w, h = 5000, 4000
	dir := t.TempDir()
	path := filepath.Join(dir, "big.tif")
	writeStackTIFF(t, path, w, h)
	hn := newHarnessWith(t, func(s *settings.Settings) { s.MaxPixels = 4; s.Preload = 0 }, tiff.Decoder{})
	if err := hn.c.Open(path); err != nil {
		t.Fatal(err)
	}
	hn.pump(func() bool { return hn.c.Status() == StatusReady })
	d := hn.c.Shown()
	if d == nil || d.Shift != 1 || d.Detail == nil {
		t.Fatalf("shown %+v", d)
	}
	hn.canvas.Actual()
	mgr := d.Detail.(*imaging.TileManager)
	frame := hn.eng.RenderOnce()
	deadline := time.Now().Add(20 * time.Second)
	for {
		hn.pump(func() bool { return true })
		frame = hn.eng.RenderOnce()
		st := mgr.Stats()
		if st.Pending == 0 && st.Inflight == 0 && st.Decoded > 0 {
			hn.pump(func() bool { return true })
			frame = hn.eng.RenderOnce()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("tiles did not arrive: %+v", st)
		}
		time.Sleep(time.Millisecond)
	}
	if st := mgr.Stats(); st.Failed != 0 {
		t.Fatalf("stats %+v", st)
	}
	f := hn.canvas.Frame()
	bad := 0
	for _, p := range [][2]int{{5, 5}, {200, 150}, {399, 299}, {123, 77}, {350, 20}} {
		u, v := hn.canvas.View().AreaToImage(f, float64(p[0])+0.5, float64(p[1])+0.5)
		want := stackGray(int(u), int(v))
		got := frame.RGBAAt(p[0], p[1])
		if got.R != want || got.G != want || got.B != want {
			bad++
			t.Logf("screen %v -> image (%.1f, %.1f): got %v want gray %d", p, u, v, got, want)
		}
	}
	if bad != 0 {
		t.Fatalf("%d sampled pixels differ from the file content", bad)
	}
	hn.canvas.RotateRight()
	hn.pump(func() bool { return true })
	frame = hn.eng.RenderOnce()
	deadline = time.Now().Add(20 * time.Second)
	for {
		hn.pump(func() bool { return true })
		frame = hn.eng.RenderOnce()
		st := mgr.Stats()
		if st.Pending == 0 && st.Inflight == 0 {
			hn.pump(func() bool { return true })
			frame = hn.eng.RenderOnce()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("tiles did not arrive after the rotation: %+v", st)
		}
		time.Sleep(time.Millisecond)
	}
	f = hn.canvas.Frame()
	o := hn.canvas.Orientation()
	for _, p := range [][2]int{{5, 5}, {200, 150}, {399, 299}} {
		u, v := hn.canvas.View().AreaToImage(f, float64(p[0])+0.5, float64(p[1])+0.5)
		sx, sy := o.ToSource(u, v, w, h)
		want := stackGray(int(sx), int(sy))
		got := frame.RGBAAt(p[0], p[1])
		if got.R != want {
			t.Fatalf("rotated: screen %v -> source (%.1f, %.1f): got %v want gray %d", p, sx, sy, got, want)
		}
	}
}
