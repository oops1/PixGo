package app

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/oops1/PixGo/internal/settings"
)

func pngWithCICP(t *testing.T, path string, c color.NRGBA, primaries, transfer byte) {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 32, 24))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, 255
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	raw := buf.Bytes()
	data := []byte{primaries, transfer, 0, 1}
	chunk := binary.BigEndian.AppendUint32(nil, uint32(len(data)))
	body := append([]byte("cICP"), data...)
	chunk = append(chunk, body...)
	chunk = binary.BigEndian.AppendUint32(chunk, crc32.ChecksumIEEE(body))
	at := 8 + 25
	out := append(append(append([]byte(nil), raw[:at]...), chunk...), raw[at:]...)
	if err := os.WriteFile(path, out, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestColorManagementConvertsAndFollowsTheSetting(t *testing.T) {
	h := newHarness(t, func(s *settings.Settings) { s.DisplayProfile = settings.DisplayProfileSRGB })
	p := filepath.Join(t.TempDir(), "p3.png")
	src := color.NRGBA{R: 200, G: 100, B: 50, A: 255}
	pngWithCICP(t, p, src, 12, 13)
	if err := h.open(p); err != nil {
		t.Fatal(err)
	}
	h.pump(func() bool { return h.c.Status() == StatusReady })
	shown := h.c.Shown()
	px := shown.Levels[0].Pix
	if px[0] == 200 && px[1] == 100 && px[2] == 50 {
		t.Fatal("a Display P3 picture must be converted to sRGB")
	}
	if rep := shown.Color; !rep.Managed || rep.Profile != "Display P3 / sRGB" || !rep.Converted {
		t.Fatalf("report %+v", rep)
	}

	h.c.env.Settings.Update(func(s *settings.Settings) { s.ColorManagement = false })
	h.c.ApplyColor()
	h.pump(func() bool {
		d := h.c.Shown()
		return h.c.Status() == StatusReady && d != nil && !d.Color.Managed
	})
	px = h.c.Shown().Levels[0].Pix
	if px[0] != 200 || px[1] != 100 || px[2] != 50 {
		t.Fatalf("with colour management off the numbers must be read as sRGB, got %v", px[:3])
	}

	h.c.env.Settings.Update(func(s *settings.Settings) { s.ColorManagement = true })
	h.c.ApplyColor()
	h.pump(func() bool {
		d := h.c.Shown()
		return h.c.Status() == StatusReady && d != nil && d.Color.Managed
	})
	if h.c.Shown().Levels[0].Pix[0] == 200 {
		t.Fatal("turning the option back on must convert again")
	}
}

func TestPlainSRGBCICPIsLeftAlone(t *testing.T) {
	h := newHarness(t, nil)
	p := filepath.Join(t.TempDir(), "s.png")
	pngWithCICP(t, p, color.NRGBA{R: 12, G: 200, B: 99, A: 255}, 1, 13)
	if err := h.open(p); err != nil {
		t.Fatal(err)
	}
	h.pump(func() bool { return h.c.Status() == StatusReady })
	px := h.c.Shown().Levels[0].Pix
	if px[0] != 12 || px[1] != 200 || px[2] != 99 || h.c.Shown().Color.Converted {
		t.Fatalf("%v %+v", px[:3], h.c.Shown().Color)
	}
}
