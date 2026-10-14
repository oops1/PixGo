//go:build !noraw

package all

import (
	"bytes"
	"context"
	"encoding/binary"
	"image"
	stdjpeg "image/jpeg"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
)

func previewJPEG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 40, 30))
	for i := 0; i < len(img.Pix); i += 4 {
		copy(img.Pix[i:], []byte{200, 80, 40, 255})
	}
	var buf bytes.Buffer
	if err := stdjpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func cameraTIFF(t *testing.T, header []byte, makeName string, crSignature bool) []byte {
	t.Helper()
	jpegData := previewJPEG(t)
	var order interface {
		binary.ByteOrder
		binary.AppendByteOrder
	} = binary.BigEndian
	if header[0] == 'I' {
		order = binary.LittleEndian
	}
	out := append([]byte(nil), header...)
	if crSignature {
		out = append(out, 'C', 'R', 2, 0, 0, 0, 0, 0)
	}
	makeAt := len(out)
	out = append(out, makeName...)
	out = append(out, 0)
	if len(out)%2 == 1 {
		out = append(out, 0)
	}
	ifdAt := len(out)
	jpegAt := ifdAt + 2 + 3*12 + 4
	out = order.AppendUint16(out, 3)
	for _, e := range [][3]uint32{{0x010f, 2, uint32(len(makeName) + 1)}, {0x0201, 4, 1}, {0x0202, 4, 1}} {
		out = order.AppendUint16(out, uint16(e[0]))
		out = order.AppendUint16(out, uint16(e[1]))
		out = order.AppendUint32(out, e[2])
		switch e[0] {
		case 0x010f:
			out = order.AppendUint32(out, uint32(makeAt))
		case 0x0201:
			out = order.AppendUint32(out, uint32(jpegAt))
		default:
			out = order.AppendUint32(out, uint32(len(jpegData)))
		}
	}
	out = order.AppendUint32(out, 0)
	order.PutUint32(out[4:], uint32(ifdAt))
	return append(out, jpegData...)
}

func rawSamples(t *testing.T) map[string][]byte {
	t.Helper()
	jpegData := previewJPEG(t)
	raf := make([]byte, 160)
	copy(raf, "FUJIFILMCCD-RAW 0201FF129502X-T20")
	binary.BigEndian.PutUint32(raf[84:], 160)
	binary.BigEndian.PutUint32(raf[88:], uint32(len(jpegData)))
	box := func(kind string, payload []byte) []byte {
		return append(append(binary.BigEndian.AppendUint32(nil, uint32(8+len(payload))), kind...), payload...)
	}
	cr3 := bytes.Join([][]byte{box("ftyp", []byte("crx \x00\x01\x00\x00crx isom")), box("mdat", jpegData)}, nil)
	orf := cameraTIFF(t, []byte{'I', 'I', 'R', 'O', 0, 0, 0, 0}, "OLYMPUS", false)
	return map[string][]byte{
		".nef": cameraTIFF(t, []byte{'M', 'M', 0, 42, 0, 0, 0, 0}, "NIKON CORPORATION", false),
		".arw": cameraTIFF(t, []byte{'I', 'I', 42, 0, 0, 0, 0, 0}, "SONY", false),
		".cr2": cameraTIFF(t, []byte{'I', 'I', 42, 0, 0, 0, 0, 0}, "Canon", true),
		".orf": orf,
		".raf": append(raf, jpegData...),
		".cr3": cr3,
	}
}

func TestRawFilesAreDetectedAndOpenedAsPreviews(t *testing.T) {
	r := newRegistry()
	for ext, data := range rawSamples(t) {
		d, ok := r.Detect(head(data), "photo"+ext)
		if !ok || d.ID() != codec.RAW {
			t.Fatalf("%s: detected as %v", ext, d)
		}
		src := &codec.Source{Path: "photo" + ext, R: bytes.NewReader(data), Size: int64(len(data)), Head: head(data)}
		dec, info, err := r.Probe(context.Background(), src, codec.DefaultLimits())
		if err != nil || dec.ID() != codec.RAW || !info.Preview || info.Format != codec.RAW || info.Width != 40 || info.Height != 30 {
			t.Fatalf("%s: %v %+v %v", ext, dec, info, err)
		}
		img, err := r.Decode(context.Background(), dec, src, codec.Options{Limits: codec.DefaultLimits()})
		if err != nil || img.First().Bounds().Dx() != 40 || img.Info.Format != codec.RAW {
			t.Fatalf("%s: %v", ext, err)
		}
		if c := img.First().NRGBAAt(20, 15); c.R < 180 || c.G < 60 || c.B > 70 {
			t.Fatalf("%s: unexpected colour %v", ext, c)
		}
	}
}

func TestContainersThatAreNotTIFFNeedNoExtension(t *testing.T) {
	r := newRegistry()
	for _, ext := range []string{".raf", ".cr3", ".orf"} {
		data := rawSamples(t)[ext]
		for _, name := range []string{"x.dat", "x", "x.jpg", "x.tif", "x.png"} {
			if d, ok := r.Detect(head(data), name); !ok || d.ID() != codec.RAW {
				t.Fatalf("%s as %s: %v", ext, name, d)
			}
		}
	}
}

func TestTIFFContainersFollowTheExtensionAndPlainTIFFStaysTIFF(t *testing.T) {
	r := newRegistry()
	nef := rawSamples(t)[".nef"]
	for name, want := range map[string]codec.ID{
		"x.nef": codec.RAW, "x.NEF": codec.RAW, "x.tif": codec.TIFF, "x.tiff": codec.TIFF, "x.dat": codec.TIFF,
	} {
		if d, ok := r.Detect(head(nef), name); !ok || d.ID() != want {
			t.Fatalf("NEF as %s: %v, want %s", name, d, want)
		}
	}
	for _, s := range samples(t) {
		if s.id != codec.TIFF {
			continue
		}
		for _, name := range []string{"x.tif", "x.nef", "x.dng", "x.arw", "x.cr2", "x"} {
			if d, ok := r.Detect(head(s.data), name); !ok || d.ID() != codec.TIFF {
				t.Fatalf("plain TIFF as %s: %v", name, d)
			}
		}
	}
}

func TestRawCanBeSwitchedOff(t *testing.T) {
	r := newRegistry()
	r.SetEnabled(codec.RAW, false)
	data := rawSamples(t)[".nef"]
	if d, ok := r.Detect(head(data), "x.nef"); !ok || d.ID() != codec.TIFF {
		t.Fatalf("%v", d)
	}
	if r.Supports("x.nef") {
		t.Fatal(".nef must not be listed while RAW is off")
	}
	if d, ok := r.Detect(head(rawSamples(t)[".raf"]), "x.raf"); ok {
		t.Fatalf("RAF detected as %v with RAW off", d)
	}
}

func TestRawExtensionsAreSupported(t *testing.T) {
	r := newRegistry()
	for _, ext := range []string{".cr2", ".cr3", ".nef", ".arw", ".dng", ".orf", ".rw2", ".pef", ".raf"} {
		if !r.Supports("IMG_0001" + ext) {
			t.Fatalf("%s is not supported", ext)
		}
	}
}
