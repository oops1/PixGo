package app

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/gif"
	"os"
	"path/filepath"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"

	"github.com/oops1/PixGo/internal/codec"
	codecall "github.com/oops1/PixGo/internal/codec/all"
	jpegcodec "github.com/oops1/PixGo/internal/codec/jpeg"
	pngcodec "github.com/oops1/PixGo/internal/codec/png"
	"github.com/oops1/PixGo/internal/meta"
	"github.com/oops1/PixGo/internal/platform"
	"github.com/oops1/PixGo/internal/settings"
	uiwin "github.com/oops1/PixGo/internal/ui/window"
)

func exifWithGPS() []byte { return exifOriented(1) }

func exifOriented(orientation byte) []byte {
	le := binary.LittleEndian
	b := make([]byte, 8)
	copy(b, "II")
	le.PutUint16(b[2:], 42)
	le.PutUint32(b[4:], 8)
	entry := func(tag, typ uint16, count uint32, val [4]byte) []byte {
		e := make([]byte, 12)
		le.PutUint16(e[0:], tag)
		le.PutUint16(e[2:], typ)
		le.PutUint32(e[4:], count)
		copy(e[8:], val[:])
		return e
	}
	long := func(v uint32) [4]byte {
		var o [4]byte
		le.PutUint32(o[:], v)
		return o
	}
	ifd0 := []byte{3, 0}
	ifd0 = append(ifd0, entry(0x010F, 2, 4, [4]byte{'C', 'a', 'm', 0})...)
	ifd0 = append(ifd0, entry(0x0112, 3, 1, [4]byte{orientation, 0, 0, 0})...)
	ifd0 = append(ifd0, entry(0x8825, 4, 1, long(50))...)
	ifd0 = append(ifd0, 0, 0, 0, 0)
	b = append(b, ifd0...)
	gps := []byte{2, 0}
	gps = append(gps, entry(0x0001, 2, 2, [4]byte{'N', 0, 0, 0})...)
	gps = append(gps, entry(0x0002, 5, 3, long(80))...)
	gps = append(gps, 0, 0, 0, 0)
	b = append(b, gps...)
	for _, v := range [][2]uint32{{55, 1}, {45, 1}, {30, 1}} {
		d := make([]byte, 8)
		le.PutUint32(d[0:], v[0])
		le.PutUint32(d[4:], v[1])
		b = append(b, d...)
	}
	return b
}

func gradientNRGBA(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{uint8(x * 255 / max(w-1, 1)), uint8(y * 255 / max(h-1, 1)), uint8((x/16 + y/16) % 2 * 200), 255})
		}
	}
	return img
}

func writeEXIFJPEG(t *testing.T, path string, w, h int) { writeOrientedJPEG(t, path, w, h, 1) }

func writeOrientedJPEG(t *testing.T, path string, w, h int, orientation byte) {
	t.Helper()
	var buf bytes.Buffer
	if err := (jpegcodec.Encoder{}).Encode(&buf, gradientNRGBA(w, h), codec.EncodeOptions{Quality: 92, Metadata: codec.Metadata{EXIF: exifOriented(orientation)}}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writePNGFile(t *testing.T, path string, w, h int) {
	t.Helper()
	var buf bytes.Buffer
	if err := (pngcodec.Encoder{}).Encode(&buf, gradientNRGBA(w, h), codec.EncodeOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeAnimatedGIF(t *testing.T, path string) {
	t.Helper()
	pal := color.Palette{color.Black, color.White}
	g := &gif.GIF{}
	for i := 0; i < 2; i++ {
		frame := image.NewPaletted(image.Rect(0, 0, 32, 32), pal)
		frame.SetColorIndex(i, i, 1)
		g.Image = append(g.Image, frame)
		g.Delay = append(g.Delay, 10)
	}
	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, g); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeSVGFile(t *testing.T, path string) {
	t.Helper()
	svg := `<svg xmlns="http://www.w3.org/2000/svg" width="64" height="48"><rect width="64" height="48" fill="red"/></svg>`
	if err := os.WriteFile(path, []byte(svg), 0o644); err != nil {
		t.Fatal(err)
	}
}

type editRig struct {
	t      *testing.T
	app    *App
	dir    string
	src    string
	dialog *fakeSaveDialog
}

func newEditRig(t *testing.T, name string, write func(*testing.T, string), mutate func(*settings.Settings)) *editRig {
	t.Helper()
	reg := codec.NewRegistry()
	codecall.Register(reg)
	r := &editRig{t: t}
	r.app = formatsApp(t, reg, mutate)
	r.dir = t.TempDir()
	r.src = filepath.Join(r.dir, name)
	write(t, r.src)
	r.app.OpenFiles([]string{r.src})
	waitName(t, r.app, name)
	r.dialog = &fakeSaveDialog{}
	r.app.Platform = &platform.Platform{SaveDialog: r.dialog}
	r.app.Root.Refresh()
	r.settle()
	return r
}

func newPNGEditRig(t *testing.T, w, h int) *editRig {
	return newEditRig(t, "photo.png", func(t *testing.T, p string) { writePNGFile(t, p, w, h) }, nil)
}

func newJPEGEditRig(t *testing.T, w, h int, mutate func(*settings.Settings)) *editRig {
	return newEditRig(t, "photo.jpg", func(t *testing.T, p string) { writeEXIFJPEG(t, p, w, h) }, mutate)
}

func (r *editRig) settle() {
	for i := 0; i < 4; i++ {
		r.app.Eng.Flush()
	}
	r.app.Eng.RenderOnce()
}

func (r *editRig) enter() {
	r.t.Helper()
	r.app.Commands.Run(CmdEditStart)
	r.settle()
	if !r.app.Editing() {
		r.t.Fatal("the edit mode did not start")
	}
}

func (r *editRig) cropTo(rc image.Rectangle) {
	r.t.Helper()
	r.app.Editor.Crop().SetRect(rc)
	if r.app.Editor.Crop().Rect() != rc {
		r.t.Fatalf("crop %v refused", rc)
	}
	r.app.Root.Refresh()
	r.settle()
}

func (r *editRig) key(code widget.KeyCode, mod widget.KeyMod) {
	kr := &uiwin.KeyRouter{Engine: r.app.Eng, Target: r.app.Root}
	kr.SendKeyEvent(widget.KeyEvent{Code: code, Mod: mod, Pressed: true})
	kr.SendKeyEvent(widget.KeyEvent{Code: code, Mod: mod, Pressed: false})
	r.app.Eng.Flush()
}

func (r *editRig) waitFile(name string) []byte {
	r.t.Helper()
	path := filepath.Join(r.dir, name)
	var data []byte
	eventually(r.t, r.app, "the file "+name+" is written", func() bool {
		b, err := os.ReadFile(path)
		data = b
		return err == nil && len(b) > 0
	})
	return data
}

func (r *editRig) waitDialog() {
	r.t.Helper()
	eventually(r.t, r.app, "the save options dialog opens", func() bool { return r.app.saveDlg != nil })
}

func (r *editRig) waitEnded() {
	r.t.Helper()
	eventually(r.t, r.app, "the edit mode ends", func() bool { return !r.app.Editing() })
}

func (r *editRig) missing(name string) bool {
	_, err := os.Stat(filepath.Join(r.dir, name))
	return os.IsNotExist(err)
}

func decodeFile(t *testing.T, data []byte) image.Image {
	t.Helper()
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func blocksOf(t *testing.T, data []byte, id codec.ID) codec.Metadata {
	t.Helper()
	m, err := meta.ReadBlocks(bytes.NewReader(data), int64(len(data)), id)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func hasGPS(t *testing.T, exif []byte) bool {
	t.Helper()
	return gpsPointer(exif)
}

func gpsPointer(exif []byte) bool {
	if len(exif) < 12 {
		return false
	}
	var order binary.ByteOrder = binary.LittleEndian
	if exif[0] == 'M' {
		order = binary.BigEndian
	}
	off := int(order.Uint32(exif[4:]))
	if off+2 > len(exif) {
		return false
	}
	n := int(order.Uint16(exif[off:]))
	for i := 0; i < n; i++ {
		e := off + 2 + 12*i
		if e+12 > len(exif) {
			return false
		}
		if order.Uint16(exif[e:]) == 0x8825 {
			return order.Uint32(exif[e+8:]) != 0
		}
	}
	return false
}

func writeStillGIF(t *testing.T, path string) {
	t.Helper()
	pal := color.Palette{color.Black, color.White}
	frame := image.NewPaletted(image.Rect(0, 0, 64, 48), pal)
	frame.SetColorIndex(3, 3, 1)
	var buf bytes.Buffer
	if err := gif.Encode(&buf, frame, nil); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

type namedSaveDialog struct {
	on func(name string) string
}

func (n namedSaveDialog) Save(o platform.SaveOptions) (string, bool, error) {
	p := n.on(o.Name)
	return p, p != "", nil
}

func platformSaving(on func(name string) string) *platform.Platform {
	return &platform.Platform{SaveDialog: namedSaveDialog{on: on}}
}

func orientationOf(exif []byte) int {
	if len(exif) < 12 {
		return 0
	}
	var order binary.ByteOrder = binary.LittleEndian
	if exif[0] == 'M' {
		order = binary.BigEndian
	}
	off := int(order.Uint32(exif[4:]))
	n := int(order.Uint16(exif[off:]))
	for i := 0; i < n; i++ {
		e := off + 2 + 12*i
		if order.Uint16(exif[e:]) == 0x0112 {
			return int(order.Uint16(exif[e+8:]))
		}
	}
	return 0
}
