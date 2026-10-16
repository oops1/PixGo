package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"

	xdraw "golang.org/x/image/draw"
)

var icoSizes = []int{16, 24, 32, 48, 64, 128, 256}

var hicolorSizes = []int{16, 32, 48, 64, 128, 256}

const runtimeSize = 256

func main() {
	out := flag.String("out", "packaging/icons", "output directory")
	source := flag.String("source", "packaging/icons/source/pixgo-icon.png", "transparent square picture of the icon, at least 256 pixels")
	runtime := flag.String("runtime", "internal/ui/icons/appicon.png", "the picture the program embeds for its window icon")
	flag.Parse()
	if err := run(*out, *source, *runtime); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(out, source, runtime string) error {
	src, err := load(source)
	if err != nil {
		return err
	}
	if err := writeICO(filepath.Join(out, "pixgo.ico"), src); err != nil {
		return err
	}
	for _, s := range hicolorSizes {
		dir := filepath.Join(out, "hicolor", fmt.Sprintf("%dx%d", s, s), "apps")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		data, err := encodePNG(src, s)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, "pixgo.png"), data, 0o644); err != nil {
			return err
		}
	}
	data, err := encodePNG(src, runtimeSize)
	if err != nil {
		return err
	}
	return os.WriteFile(runtime, data, 0o644)
}

func load(path string) (*image.NRGBA, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return nil, err
	}
	b := img.Bounds()
	if b.Dx() != b.Dy() || b.Dx() < runtimeSize {
		return nil, fmt.Errorf("%s: the picture must be square and at least %d pixels, it is %dx%d", path, runtimeSize, b.Dx(), b.Dy())
	}
	dst := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	xdraw.Copy(dst, image.Point{}, img, b, xdraw.Src, nil)
	return dst, nil
}

func resize(src *image.NRGBA, size int) *image.NRGBA {
	if src.Bounds().Dx() == size {
		return src
	}
	dst := image.NewNRGBA(image.Rect(0, 0, size, size))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), xdraw.Src, nil)
	for i := 0; i < len(dst.Pix); i += 4 {
		if dst.Pix[i+3] == 0 {
			dst.Pix[i], dst.Pix[i+1], dst.Pix[i+2] = 0, 0, 0
		}
	}
	return dst
}

func encodePNG(src *image.NRGBA, size int) ([]byte, error) {
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(&buf, resize(src, size)); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeICO(path string, src *image.NRGBA) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var images [][]byte
	for _, s := range icoSizes {
		data, err := encodePNG(src, s)
		if err != nil {
			return err
		}
		images = append(images, data)
	}
	var buf bytes.Buffer
	le := binary.LittleEndian
	binary.Write(&buf, le, uint16(0))
	binary.Write(&buf, le, uint16(1))
	binary.Write(&buf, le, uint16(len(images)))
	offset := 6 + 16*len(images)
	for i, s := range icoSizes {
		dim := byte(s)
		if s >= 256 {
			dim = 0
		}
		buf.Write([]byte{dim, dim, 0, 0})
		binary.Write(&buf, le, uint16(1))
		binary.Write(&buf, le, uint16(32))
		binary.Write(&buf, le, uint32(len(images[i])))
		binary.Write(&buf, le, uint32(offset))
		offset += len(images[i])
	}
	for _, d := range images {
		buf.Write(d)
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}
