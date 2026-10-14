package app

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/oops1/PixGo/internal/isobmff/isotest"
	"github.com/oops1/PixGo/internal/ui/infopanel"
)

func readTestdata(t *testing.T, rel string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "codec", rel))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func withExifItem(t *testing.T, sample []byte) []byte {
	t.Helper()
	parsed, err := isotest.Parse(sample)
	if err != nil {
		t.Fatal(err)
	}
	id := uint32(len(parsed.Items) + 1)
	payload := append(isotest.U32(6), "Exif\x00\x00"...)
	parsed.Items = append(parsed.Items, isotest.Item{ID: id, Type: "Exif", Hidden: true, Data: append(payload, exifBlock()...)})
	parsed.Refs = append(parsed.Refs, isotest.Ref{Type: "cdsc", From: id, To: []uint32{parsed.Primary}})
	return parsed.Bytes()
}

func jxlWithExif(t *testing.T, codestream []byte) []byte {
	t.Helper()
	out := []byte("\x00\x00\x00\x0cJXL \x0d\x0a\x87\x0a")
	out = append(out, isotest.Box("ftyp", []byte("jxl "), isotest.U32(0), []byte("jxl "))...)
	out = append(out, isotest.Box("Exif", isotest.U32(0), exifBlock())...)
	return append(out, isotest.Box("jxlc", codestream)...)
}

func rafWithExifPreview(t *testing.T) []byte {
	t.Helper()
	path := filepath.Join(t.TempDir(), "preview.jpg")
	writeExifJPEG(t, path)
	jpeg, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	head := make([]byte, 160)
	copy(head, "FUJIFILMCCD-RAW 0201")
	binary.BigEndian.PutUint32(head[84:], uint32(len(head)))
	binary.BigEndian.PutUint32(head[88:], uint32(len(jpeg)))
	return append(head, jpeg...)
}

func TestInfoPanelShowsCameraDataOfEveryNewFormat(t *testing.T) {
	cases := []struct {
		file   string
		format string
		build  func(*testing.T) []byte
	}{
		{"photo.heic", "HEIC", func(t *testing.T) []byte { return withExifItem(t, readTestdata(t, "heic/testdata/basic.heic")) }},
		{"photo.avif", "AVIF", func(t *testing.T) []byte { return withExifItem(t, readTestdata(t, "avif/testdata/yuv420-8.avif")) }},
		{"photo.jxl", "JPEG XL", func(t *testing.T) []byte { return jxlWithExif(t, readTestdata(t, "jxl/testdata/dct_small.jxl")) }},
		{"photo.raf", "RAW", rafWithExifPreview},
	}
	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			a, _ := goldenApp(t, 900, 600, nil)
			p := filepath.Join(t.TempDir(), c.file)
			if err := os.WriteFile(p, c.build(t), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := a.Ctl.Open(p); err != nil {
				t.Fatal(err)
			}
			waitReady(t, a)
			a.toggleInfo()
			deadline := time.Now().Add(5 * time.Second)
			var cam *infopanel.Card
			for time.Now().Before(deadline) && cam == nil {
				a.Eng.Flush()
				for _, card := range a.info.Model().Cards {
					if card.Kind == infopanel.CardCamera {
						card := card
						cam = &card
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
			if got := a.info.Model().Format; !strings.HasPrefix(got, c.format) {
				t.Errorf("format line %q, want %q", got, c.format)
			}
		})
	}
}
