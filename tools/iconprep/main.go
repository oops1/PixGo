package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	"image/png"
	"math"
	"os"
	"sort"

	xdraw "golang.org/x/image/draw"
)

const (
	flatTolerance = 12
	edgeBand      = 4
	sampleRadius  = 9
)

func main() {
	in := flag.String("in", "", "picture on a plain light background")
	out := flag.String("out", "pixgo-icon.png", "transparent square picture")
	side := flag.Int("size", 1024, "side of the result in pixels")
	margin := flag.Float64("margin", 0.03, "empty border around the shape, as a share of the side")
	flag.Parse()
	if *in == "" {
		fmt.Fprintln(os.Stderr, "usage: iconprep -in picture.png [-out icon.png] [-size 1024] [-margin 0.03]")
		os.Exit(2)
	}
	if err := run(*in, *out, *side, *margin); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(in, out string, side int, margin float64) error {
	f, err := os.Open(in)
	if err != nil {
		return err
	}
	defer f.Close()
	src, _, err := image.Decode(f)
	if err != nil {
		return err
	}
	cut := Cut(src)
	square := fit(cut, side, margin)
	o, err := os.Create(out)
	if err != nil {
		return err
	}
	defer o.Close()
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	return enc.Encode(o, square)
}

type rgb [3]float64

func Cut(src image.Image) *image.NRGBA {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	pix := make([]rgb, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, bl, _ := src.At(b.Min.X+x, b.Min.Y+y).RGBA()
			pix[y*w+x] = rgb{float64(r >> 8), float64(g >> 8), float64(bl >> 8)}
		}
	}
	bg := backgroundColour(pix, w, h)
	outside := floodOutside(pix, w, h, bg)
	dist := distanceFromOutside(outside, w, h)
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := y*w + x
			switch {
			case outside[i]:
			case dist[i] > edgeBand:
				dst.SetNRGBA(x, y, color.NRGBA{R: u8(pix[i][0]), G: u8(pix[i][1]), B: u8(pix[i][2]), A: 255})
			default:
				c, ok := solidColour(pix, dist, w, h, x, y)
				if !ok {
					dst.SetNRGBA(x, y, color.NRGBA{R: u8(pix[i][0]), G: u8(pix[i][1]), B: u8(pix[i][2]), A: 255})
					continue
				}
				a := coverage(pix[i], bg, c)
				if a <= 0.004 {
					continue
				}
				var o [3]float64
				for k := 0; k < 3; k++ {
					o[k] = (pix[i][k] - (1-a)*bg[k]) / a
				}
				dst.SetNRGBA(x, y, color.NRGBA{R: u8(o[0]), G: u8(o[1]), B: u8(o[2]), A: u8(a * 255)})
			}
		}
	}
	return dst
}

func u8(v float64) uint8 {
	return uint8(math.Max(0, math.Min(255, math.Round(v))))
}

func backgroundColour(pix []rgb, w, h int) rgb {
	var rs, gs, bs []float64
	add := func(x, y int) {
		p := pix[y*w+x]
		rs, gs, bs = append(rs, p[0]), append(gs, p[1]), append(bs, p[2])
	}
	for x := 0; x < w; x++ {
		add(x, 0)
		add(x, h-1)
	}
	for y := 1; y < h-1; y++ {
		add(0, y)
		add(w-1, y)
	}
	med := func(v []float64) float64 {
		sort.Float64s(v)
		return v[len(v)/2]
	}
	return rgb{med(rs), med(gs), med(bs)}
}

func near(p, bg rgb, tolerance float64) bool {
	return math.Abs(p[0]-bg[0]) <= tolerance && math.Abs(p[1]-bg[1]) <= tolerance && math.Abs(p[2]-bg[2]) <= tolerance
}

func floodOutside(pix []rgb, w, h int, bg rgb) []bool {
	outside := make([]bool, w*h)
	var stack []int
	push := func(x, y int) {
		i := y*w + x
		if !outside[i] && near(pix[i], bg, flatTolerance) {
			outside[i] = true
			stack = append(stack, i)
		}
	}
	for x := 0; x < w; x++ {
		push(x, 0)
		push(x, h-1)
	}
	for y := 0; y < h; y++ {
		push(0, y)
		push(w-1, y)
	}
	for len(stack) > 0 {
		i := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		x, y := i%w, i/w
		if x > 0 {
			push(x-1, y)
		}
		if x < w-1 {
			push(x+1, y)
		}
		if y > 0 {
			push(x, y-1)
		}
		if y < h-1 {
			push(x, y+1)
		}
	}
	return outside
}

func distanceFromOutside(outside []bool, w, h int) []int {
	const far = 1 << 20
	dist := make([]int, w*h)
	var queue []int
	for i := range dist {
		if outside[i] {
			queue = append(queue, i)
		} else {
			dist[i] = far
		}
	}
	for head := 0; head < len(queue); head++ {
		i := queue[head]
		if dist[i] >= edgeBand+2 {
			continue
		}
		x, y := i%w, i/w
		for dy := -1; dy <= 1; dy++ {
			for dx := -1; dx <= 1; dx++ {
				nx, ny := x+dx, y+dy
				if (dx == 0 && dy == 0) || nx < 0 || ny < 0 || nx >= w || ny >= h {
					continue
				}
				j := ny*w + nx
				if dist[j] == far {
					dist[j] = dist[i] + 1
					queue = append(queue, j)
				}
			}
		}
	}
	return dist
}

func solidColour(pix []rgb, dist []int, w, h, x, y int) (rgb, bool) {
	var sum rgb
	n := 0
	for dy := -sampleRadius; dy <= sampleRadius; dy++ {
		for dx := -sampleRadius; dx <= sampleRadius; dx++ {
			nx, ny := x+dx, y+dy
			if nx < 0 || ny < 0 || nx >= w || ny >= h {
				continue
			}
			j := ny*w + nx
			if dist[j] > edgeBand {
				for k := 0; k < 3; k++ {
					sum[k] += pix[j][k]
				}
				n++
			}
		}
	}
	if n == 0 {
		return rgb{}, false
	}
	for k := range sum {
		sum[k] /= float64(n)
	}
	return sum, true
}

func coverage(p, bg, c rgb) float64 {
	var num, den float64
	for k := 0; k < 3; k++ {
		d := bg[k] - c[k]
		num += (bg[k] - p[k]) * d
		den += d * d
	}
	if den < 1 {
		return 1
	}
	return math.Max(0, math.Min(1, num/den))
}

func bounds(img *image.NRGBA) image.Rectangle {
	b := img.Bounds()
	r := image.Rectangle{Min: b.Max, Max: b.Min}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if img.NRGBAAt(x, y).A > 8 {
				r.Min.X, r.Min.Y = min(r.Min.X, x), min(r.Min.Y, y)
				r.Max.X, r.Max.Y = max(r.Max.X, x+1), max(r.Max.Y, y+1)
			}
		}
	}
	return r
}

func fit(src *image.NRGBA, side int, margin float64) *image.NRGBA {
	r := bounds(src)
	extent := max(r.Dx(), r.Dy())
	inner := float64(side) * (1 - 2*margin)
	scale := inner / float64(extent)
	w, h := int(math.Round(float64(r.Dx())*scale)), int(math.Round(float64(r.Dy())*scale))
	dst := image.NewNRGBA(image.Rect(0, 0, side, side))
	at := image.Rect((side-w)/2, (side-h)/2, (side-w)/2+w, (side-h)/2+h)
	xdraw.CatmullRom.Scale(dst, at, src, r, xdraw.Over, nil)
	return dst
}
