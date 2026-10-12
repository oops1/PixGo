package all

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"image"
	"image/color"
	stdgif "image/gif"
	stdjpeg "image/jpeg"
	stdpng "image/png"
	"os"
	"path/filepath"
	"testing"

	xtiff "golang.org/x/image/tiff"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/bmp"
	"github.com/oops1/PixGo/internal/codec/internal/testimg"
)

type sample struct {
	id        codec.ID
	extension string
	data      []byte
	width     int
	height    int
}

func iconData(kind uint16) []byte {
	le := binary.LittleEndian
	header := le.AppendUint32(nil, 40)
	header = le.AppendUint32(header, 2)
	header = le.AppendUint32(header, 4)
	header = le.AppendUint16(header, 1)
	header = le.AppendUint16(header, 32)
	header = append(header, make([]byte, 24)...)
	pixels := bytes.Repeat([]byte{10, 20, 30, 255}, 4)
	pixels = append(pixels, 0, 0, 0, 0, 0, 0, 0, 0)
	body := append(header, pixels...)
	out := le.AppendUint16(nil, 0)
	out = le.AppendUint16(out, kind)
	out = le.AppendUint16(out, 1)
	out = append(out, 2, 2, 0, 0)
	out = le.AppendUint16(out, 1)
	out = le.AppendUint16(out, 32)
	out = le.AppendUint32(out, uint32(len(body)))
	out = le.AppendUint32(out, 22)
	return append(out, body...)
}

func samples(t *testing.T) []sample {
	t.Helper()
	photo := testimg.Photo(40, 30)
	rgba := image.NewRGBA(photo.Bounds())
	for i := 0; i < len(photo.Pix); i += 4 {
		rgba.Pix[i], rgba.Pix[i+1], rgba.Pix[i+2], rgba.Pix[i+3] = photo.Pix[i], photo.Pix[i+1], photo.Pix[i+2], 255
	}
	var jpegBuf, pngBuf, gifBuf, bmpBuf, tiffBuf bytes.Buffer
	if err := stdjpeg.Encode(&jpegBuf, rgba, nil); err != nil {
		t.Fatal(err)
	}
	if err := stdpng.Encode(&pngBuf, photo); err != nil {
		t.Fatal(err)
	}
	paletted := image.NewPaletted(image.Rect(0, 0, 40, 30), color.Palette{color.RGBA{R: 1, G: 2, B: 3, A: 255}, color.RGBA{R: 200, G: 100, B: 50, A: 255}})
	if err := stdgif.Encode(&gifBuf, paletted, nil); err != nil {
		t.Fatal(err)
	}
	if err := (bmp.Encoder{}).Encode(&bmpBuf, photo, codec.EncodeOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := xtiff.Encode(&tiffBuf, photo, nil); err != nil {
		t.Fatal(err)
	}
	webpData, err := os.ReadFile(filepath.Join("..", "webp", "testdata", "gopher-doc.1bpp.lossless.webp"))
	if err != nil {
		t.Fatal(err)
	}
	svgText := []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 40 30"><rect width="40" height="30" fill="red"/></svg>`)
	var svgz bytes.Buffer
	zw := gzip.NewWriter(&svgz)
	zw.Write(svgText)
	zw.Close()
	return []sample{
		{codec.JPEG, ".jpg", jpegBuf.Bytes(), 40, 30},
		{codec.PNG, ".png", pngBuf.Bytes(), 40, 30},
		{codec.GIF, ".gif", gifBuf.Bytes(), 40, 30},
		{codec.BMP, ".bmp", bmpBuf.Bytes(), 40, 30},
		{codec.TIFF, ".tif", tiffBuf.Bytes(), 40, 30},
		{codec.WebP, ".webp", webpData, 0, 0},
		{codec.ICO, ".ico", iconData(1), 2, 2},
		{codec.CUR, ".cur", iconData(2), 2, 2},
		{codec.SVG, ".svg", svgText, 40, 30},
		{codec.SVG, ".svgz", svgz.Bytes(), 40, 30},
	}
}

func newRegistry() *codec.Registry {
	r := codec.NewRegistry()
	Register(r)
	return r
}

func head(data []byte) []byte {
	if len(data) > codec.SniffLen {
		return data[:codec.SniffLen]
	}
	return data
}

func TestEverySampleIsDetectedWhateverItsExtension(t *testing.T) {
	r := newRegistry()
	for _, s := range samples(t) {
		for _, name := range []string{"noextension", "wrong.dat", "photo" + s.extension, "photo.JPG", "photo.png", "photo.txt", "photo.svg", "photo.tif", "photo.webp", "photo.ico"} {
			d, ok := r.Detect(head(s.data), name)
			if !ok {
				t.Fatalf("%s as %q: not detected", s.id, name)
			}
			if d.ID() != s.id {
				t.Fatalf("%s as %q: detected as %s", s.id, name, d.ID())
			}
		}
	}
}

func TestRegistryProbeAndDecodeEverySample(t *testing.T) {
	r := newRegistry()
	for _, s := range samples(t) {
		src := &codec.Source{Path: "sample" + s.extension, R: bytes.NewReader(s.data), Size: int64(len(s.data)), Head: head(s.data)}
		d, info, err := r.Probe(context.Background(), src, codec.DefaultLimits())
		if err != nil {
			t.Fatalf("%s probe: %v", s.extension, err)
		}
		if d.ID() != s.id || info.Format != s.id {
			t.Fatalf("%s: decoder %s info %+v", s.extension, d.ID(), info)
		}
		if s.width > 0 && (info.Width != s.width || info.Height != s.height) {
			t.Fatalf("%s: %dx%d", s.extension, info.Width, info.Height)
		}
		img, err := r.Decode(context.Background(), d, src, codec.Options{Limits: codec.DefaultLimits(), Frames: codec.AllFrames})
		if err != nil {
			t.Fatalf("%s decode: %v", s.extension, err)
		}
		if img.First() == nil || img.First().Bounds().Empty() || img.Info.Format != s.id {
			t.Fatalf("%s: %+v", s.extension, img.Info)
		}
		if s.id != codec.SVG && (img.First().Bounds().Dx() != info.Width || img.First().Bounds().Dy() != info.Height) {
			t.Fatalf("%s: decoded %v, probed %dx%d", s.extension, img.First().Bounds(), info.Width, info.Height)
		}
	}
}

func TestRegistryCatalogue(t *testing.T) {
	r := newRegistry()
	want := []string{".bmp", ".cur", ".dib", ".gif", ".ico", ".jfif", ".jpe", ".jpeg", ".jpg", ".png", ".svg", ".svgz", ".tif", ".tiff", ".webp"}
	got := r.Extensions()
	if len(got) != len(want) {
		t.Fatalf("extensions %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("extensions %v, want %v", got, want)
		}
	}
	for _, name := range []string{"a.JPG", "b.Png", "c.TIFF", "d.webp", "e.svgz", "f.CUR"} {
		if !r.Supports(name) {
			t.Fatalf("%s is not supported", name)
		}
	}
	if r.Supports("movie.mp4") || r.Supports("noextension") {
		t.Fatal("unsupported file accepted")
	}
	if len(r.Decoders()) != 9 {
		t.Fatalf("%d decoders", len(r.Decoders()))
	}
	for _, id := range []codec.ID{codec.JPEG, codec.PNG, codec.BMP} {
		e, ok := r.Encoder(id)
		if !ok || e.ID() != id || e.Extension() == "" || e.Name() == "" {
			t.Fatalf("encoder %s: %v", id, ok)
		}
	}
	if _, ok := r.Encoder(codec.GIF); ok {
		t.Fatal("unexpected GIF encoder")
	}
	if len(r.Encoders()) != 3 {
		t.Fatalf("%d encoders", len(r.Encoders()))
	}
}

func TestEncodersProduceFilesTheRegistryReads(t *testing.T) {
	r := newRegistry()
	src := testimg.Photo(33, 21)
	for _, e := range r.Encoders() {
		var buf bytes.Buffer
		if err := e.Encode(&buf, src, codec.EncodeOptions{Quality: 80}); err != nil {
			t.Fatalf("%s: %v", e.ID(), err)
		}
		d, ok := r.Detect(head(buf.Bytes()), "out"+e.Extension())
		if !ok || d.ID() != e.ID() {
			t.Fatalf("%s: detected %v", e.ID(), ok)
		}
		img, err := d.Decode(context.Background(), bytes.NewReader(buf.Bytes()), int64(buf.Len()), codec.Options{})
		if err != nil || img.First().Bounds().Dx() != 33 || img.First().Bounds().Dy() != 21 {
			t.Fatalf("%s: %v", e.ID(), err)
		}
	}
}

func TestDisabledFormatsAreNotDetected(t *testing.T) {
	r := newRegistry()
	r.SetEnabled(codec.PNG, false)
	for _, s := range samples(t) {
		_, ok := r.Detect(head(s.data), "x.dat")
		if s.id == codec.PNG && ok {
			t.Fatal("disabled PNG detected")
		}
		if s.id != codec.PNG && !ok {
			t.Fatalf("%s lost when PNG was disabled", s.id)
		}
	}
}

func TestRegistryRejectsGarbage(t *testing.T) {
	r := newRegistry()
	for _, data := range [][]byte{nil, []byte("hello world, this is plain text"), bytes.Repeat([]byte{0}, 600), bytes.Repeat([]byte{0xff}, 600)} {
		if d, ok := r.Detect(head(data), "x.png"); ok {
			t.Fatalf("garbage detected as %s", d.ID())
		}
	}
	src := &codec.Source{Path: "x.png", R: bytes.NewReader([]byte("garbage")), Size: 7, Head: []byte("garbage")}
	if _, _, err := r.Probe(context.Background(), src, codec.DefaultLimits()); err == nil {
		t.Fatal("garbage probed")
	}
}

func TestDecodersNeverPanicOnEachOthersFiles(t *testing.T) {
	r := newRegistry()
	all := samples(t)
	for _, d := range r.Decoders() {
		for _, s := range all {
			if _, err := d.Decode(context.Background(), bytes.NewReader(s.data), int64(len(s.data)), codec.Options{Limits: testimg.SmallLimits(), Frames: codec.AllFrames}); err != nil {
				testimg.RequireCodecError(t, err)
			}
			if _, err := d.Probe(context.Background(), bytes.NewReader(s.data), int64(len(s.data)), testimg.SmallLimits()); err != nil {
				testimg.RequireCodecError(t, err)
			}
		}
	}
}
