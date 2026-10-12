package app

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/ui/infopanel"
)

func exifBlock() []byte {
	le := binary.LittleEndian
	var data bytes.Buffer
	entry := func(tag, typ uint16, count, value uint32) []byte {
		b := make([]byte, 12)
		le.PutUint16(b[0:], tag)
		le.PutUint16(b[2:], typ)
		le.PutUint32(b[4:], count)
		le.PutUint32(b[8:], value)
		return b
	}
	const ifd0 = 8
	ifd0Size := 2 + 3*12 + 4
	exifIFD := ifd0 + ifd0Size
	exifSize := 2 + 3*12 + 4
	dataStart := exifIFD + exifSize
	addData := func(b []byte) uint32 {
		off := uint32(dataStart + data.Len())
		data.Write(b)
		return off
	}
	makeOff := addData([]byte("Canon\x00"))
	modelOff := addData([]byte("EOS R5\x00\x00"))
	rat := func(n, d uint32) []byte {
		b := make([]byte, 8)
		le.PutUint32(b, n)
		le.PutUint32(b[4:], d)
		return b
	}
	expOff := addData(rat(1, 250))
	fOff := addData(rat(40, 10))

	var out bytes.Buffer
	out.WriteString("II*\x00")
	binary.Write(&out, le, uint32(ifd0))
	binary.Write(&out, le, uint16(3))
	out.Write(entry(0x010F, 2, 6, makeOff))
	out.Write(entry(0x0110, 2, 7, modelOff))
	out.Write(entry(0x8769, 4, 1, uint32(exifIFD)))
	binary.Write(&out, le, uint32(0))
	binary.Write(&out, le, uint16(3))
	out.Write(entry(0x829A, 5, 1, expOff))
	out.Write(entry(0x829D, 5, 1, fOff))
	out.Write(entry(0x8827, 3, 1, 100))
	binary.Write(&out, le, uint32(0))
	out.Write(data.Bytes())
	return out.Bytes()
}

func writeExifJPEG(t *testing.T, path string) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 96, 64))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = 90, 140, 200, 255
	}
	var plain bytes.Buffer
	if err := jpeg.Encode(&plain, img, nil); err != nil {
		t.Fatal(err)
	}
	tiff := exifBlock()
	var app1 bytes.Buffer
	app1.Write([]byte{0xFF, 0xE1})
	binary.Write(&app1, binary.BigEndian, uint16(2+6+len(tiff)))
	app1.WriteString("Exif\x00\x00")
	app1.Write(tiff)
	b := plain.Bytes()
	out := append(append(append([]byte{}, b[:2]...), app1.Bytes()...), b[2:]...)
	if err := os.WriteFile(path, out, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestInfoPanelShowsCameraDataAfterToggle(t *testing.T) {
	a, _ := goldenApp(t, 900, 600, nil)
	p := filepath.Join(t.TempDir(), "photo.jpg")
	writeExifJPEG(t, p)
	if err := a.Ctl.Open(p); err != nil {
		t.Fatal(err)
	}
	waitReady(t, a)
	if a.Panel() != nil {
		t.Fatal("panel present before it was opened")
	}
	a.Root.HandleKey(widget.KeyEvent{Code: widget.KeyI, Mod: widget.ModCtrl, Pressed: true})
	if a.Panel() == nil || !a.InfoOpen() {
		t.Fatal("Ctrl+I did not open the panel")
	}
	l := a.Root.Layout()
	if l.Panel.Dx() != 326 || l.View.Max.X != l.Panel.Min.X {
		t.Errorf("layout panel=%v view=%v", l.Panel, l.View)
	}
	deadline := time.Now().Add(5 * time.Second)
	var cam *infopanel.Card
	for time.Now().Before(deadline) && cam == nil {
		a.Eng.Flush()
		for _, c := range a.info.Model().Cards {
			if c.Kind == infopanel.CardCamera {
				c := c
				cam = &c
			}
		}
		time.Sleep(3 * time.Millisecond)
	}
	if cam == nil {
		t.Fatalf("no camera card; cards: %+v", a.info.Model().Cards)
	}
	vals := map[string]string{}
	for _, r := range cam.Rows {
		vals[r.Label] = r.Value
	}
	if vals["Производитель"] != "Canon" || vals["Модель"] != "EOS R5" || vals["Выдержка"] != "1/250 с" || vals["Диафрагма"] != "f/4.0" || vals["ISO"] != "100" {
		t.Errorf("camera rows %v", vals)
	}
	m := a.info.Model()
	if m.Name != "photo.jpg" || m.Format != "JPEG" {
		t.Errorf("model header %q / %q", m.Name, m.Format)
	}
	img := a.Eng.RenderOnce()
	panelPx := img.RGBAAt(l.Panel.Min.X+160, l.Panel.Max.Y-6)
	if panelPx.A != 255 {
		t.Errorf("panel pixel %v", panelPx)
	}
	a.Root.HandleKey(widget.KeyEvent{Code: widget.KeyI, Mod: widget.ModCtrl, Pressed: true})
	if a.Panel() != nil || !a.Root.Layout().Panel.Empty() {
		t.Error("panel still open after the second Ctrl+I")
	}
}

func TestInfoPanelFollowsNavigation(t *testing.T) {
	a, _ := goldenApp(t, 900, 600, nil)
	dir := t.TempDir()
	writeExifJPEG(t, filepath.Join(dir, "a.jpg"))
	writePNG(t, filepath.Join(dir, "b.png"), 40, 30, color.NRGBA{1, 2, 3, 255})
	a.Ctl.Open(filepath.Join(dir, "a.jpg"))
	waitReady(t, a)
	a.toggleInfo()
	a.Eng.Flush()
	a.Ctl.Step(1)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && a.info.Model().Name != "b.png" {
		a.Eng.Flush()
		time.Sleep(3 * time.Millisecond)
	}
	if a.info.Model().Name != "b.png" {
		t.Fatalf("panel still shows %q", a.info.Model().Name)
	}
	for _, c := range a.info.Model().Cards {
		if c.Kind == infopanel.CardCamera {
			t.Error("camera card carried over to a PNG")
		}
	}
}

func TestFavoritesAndRecentSurviveRestart(t *testing.T) {
	a, _ := goldenApp(t, 770, 504, nil)
	dir := t.TempDir()
	p := filepath.Join(dir, "x.png")
	writePNG(t, p, 30, 20, color.NRGBA{1, 2, 3, 255})
	a.Ctl.Open(p)
	waitReady(t, a)
	if !a.Ctl.ToggleFavorite() {
		t.Fatal("favorite not added")
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		a.Eng.Flush()
		fav, _ := os.ReadFile(favoritesPath())
		rec, _ := os.ReadFile(recentPath())
		if bytes.Contains(fav, []byte("x.png")) && bytes.Contains(rec, []byte("x.png")) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	b, _ := goldenAppSharingState(t, a)
	if !b.Ctl.Favorites.Contains(p) {
		t.Error("favorite lost after restart")
	}
	if !b.Ctl.Recent.Contains(p) {
		t.Error("recent entry lost after restart")
	}
}
