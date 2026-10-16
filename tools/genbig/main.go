package main

import (
	"bufio"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"os"
)

type pattern struct {
	w, h int
}

func (p pattern) ColorModel() color.Model { return color.RGBAModel }

func (p pattern) Bounds() image.Rectangle { return image.Rect(0, 0, p.w, p.h) }

func (p pattern) At(x, y int) color.Color {
	r := uint8(x * 255 / p.w)
	g := uint8(y * 255 / p.h)
	b := uint8((x + y) * 255 / (p.w + p.h))
	dx, dy := x-p.w/2, y-p.h/2
	ring := (dx*dx + dy*dy) / 160000
	if ring%2 == 0 {
		r, g, b = r/2+64, g/2+64, b/2+64
	}
	if x%1024 < 3 || y%1024 < 3 {
		r, g, b = 0, 0, 0
	}
	if x%4096 < 6 || y%4096 < 6 {
		r, g, b = 220, 0, 0
	}
	if (x/512+y/512)%2 == 0 && (x/2+y/2)%2 == 0 && x%64 < 32 && y%64 < 32 {
		r, g, b = 255-r, 255-g, 255-b
	}
	n := int(noise(x, y)&15) - 8
	return color.RGBA{R: clamp(int(r) + n), G: clamp(int(g) + n), B: clamp(int(b) - n), A: 255}
}

func noise(x, y int) uint32 {
	v := uint32(x)*73856093 ^ uint32(y)*19349663
	v ^= v >> 13
	v *= 0x5bd1e995
	v ^= v >> 15
	return v
}

func clamp(v int) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}

func main() {
	w := flag.Int("w", 16384, "width")
	h := flag.Int("h", 12288, "height")
	q := flag.Int("q", 85, "JPEG quality")
	out := flag.String("o", "big.jpg", "output file")
	flag.Parse()
	if *w <= 0 || *h <= 0 || *w > 65500 || *h > 65500 {
		fmt.Fprintln(os.Stderr, "size out of range")
		os.Exit(1)
	}
	f, err := os.Create(*out)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	bw := bufio.NewWriterSize(f, 1<<20)
	if err := jpeg.Encode(bw, pattern{*w, *h}, &jpeg.Options{Quality: *q}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := bw.Flush(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := f.Close(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
