package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"

	"github.com/oops1/PixGo/internal/ui/icons"
)

var icoSizes = []int{16, 24, 32, 48, 64, 128, 256}

var hicolorSizes = []int{16, 32, 48, 64, 128, 256}

func main() {
	out := flag.String("out", "packaging/icons", "output directory")
	flag.Parse()
	if err := run(*out); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(out string) error {
	if err := writeICO(filepath.Join(out, "pixgo.ico")); err != nil {
		return err
	}
	for _, s := range hicolorSizes {
		dir := filepath.Join(out, "hicolor", fmt.Sprintf("%dx%d", s, s), "apps")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		data, err := encodePNG(s)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, "pixgo.png"), data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func straight(src *image.RGBA) *image.NRGBA {
	dst := image.NewNRGBA(src.Bounds())
	draw.Draw(dst, dst.Bounds(), src, src.Bounds().Min, draw.Src)
	for i := 0; i < len(dst.Pix); i += 4 {
		if dst.Pix[i+3] == 0 {
			dst.Pix[i], dst.Pix[i+1], dst.Pix[i+2] = 0, 0, 0
		}
	}
	return dst
}

func encodePNG(size int) ([]byte, error) {
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(&buf, straight(icons.AppIcon(size))); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeICO(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var images [][]byte
	for _, s := range icoSizes {
		data, err := encodePNG(s)
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
