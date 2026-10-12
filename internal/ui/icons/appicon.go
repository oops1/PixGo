package icons

import (
	"image"
	"image/color"
	"sync"

	"github.com/oops1/PixGo/internal/ui/gfx"
)

type paintLayer struct {
	shape gfx.Shape
	fill  func(x, y float64) color.RGBA
}

func solid(col uint32, a float64) func(x, y float64) color.RGBA {
	c := gfx.RGB(col)
	c.A = uint8(a*255 + .5)
	return func(x, y float64) color.RGBA { return c }
}

func fade(col uint32, a, end float64) func(x, y float64) color.RGBA {
	c := gfx.RGB(col)
	return func(x, y float64) color.RGBA {
		f := max(0, 1-y/end)
		c.A = uint8(a*f*f*255 + .5)
		return c
	}
}

func vertical(top, bottom uint32, y0, y1, tilt float64) func(x, y float64) color.RGBA {
	t, b := gfx.RGB(top), gfx.RGB(bottom)
	return func(x, y float64) color.RGBA {
		f := (y-y0)/(y1-y0) + (x-.5)*tilt
		return gfx.Blend(t, b, max(0, min(1, f)))
	}
}

func appLayers() []paintLayer {
	base := gfx.RRect(.035, .035, .965, .965, .22)
	card := gfx.RRect(.17, .23, .83, .77, .085)
	inset := gfx.RRect(.24, .30, .76, .70, .04)
	sun := gfx.Circle(.625, .43, .062)
	far := gfx.Poly(.24, .70, .24, .60, .37, .45, .47, .58, .56, .49, .76, .66, .76, .70)
	near := gfx.Poly(.24, .70, .24, .66, .33, .57, .44, .70)
	snow := gfx.Poly(.37, .45, .325, .515, .365, .50, .40, .525, .415, .50)
	return []paintLayer{
		{base, vertical(0x58c6ff, 0x1c5bd4, .035, .965, .35)},
		{gfx.Inter(base, gfx.Box(0, 0, 1, .6)), fade(0xffffff, .26, .6)},
		{shifted(card, 0, .028), solid(0x0b2f80, .30)},
		{card, solid(0xfdfeff, 1)},
		{inset, vertical(0x66c3ff, 0xdff3ff, .30, .70, 0)},
		{gfx.Inter(far, inset), solid(0x4a8be6, 1)},
		{gfx.Inter(near, inset), solid(0x1f5cc2, 1)},
		{gfx.Inter(snow, inset), solid(0xf2f8ff, .95)},
		{sun, solid(0xffc94d, 1)},
	}
}

func shifted(s gfx.Shape, dx, dy float64) gfx.Shape {
	return func(x, y float64) bool { return s(x-dx, y-dy) }
}

var appCache = struct {
	sync.Mutex
	m map[int]*image.RGBA
}{m: map[int]*image.RGBA{}}

func AppIcon(size int) *image.RGBA {
	if size < 1 {
		size = 1
	}
	appCache.Lock()
	defer appCache.Unlock()
	if img, ok := appCache.m[size]; ok {
		out := image.NewRGBA(img.Rect)
		copy(out.Pix, img.Pix)
		return out
	}
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	for _, l := range appLayers() {
		mask := gfx.Mask(gfx.Scaled(l.shape, float64(size)), size, size)
		for y := 0; y < size; y++ {
			for x := 0; x < size; x++ {
				a := mask[y*size+x]
				if a == 0 {
					continue
				}
				c := l.fill((float64(x)+.5)/float64(size), (float64(y)+.5)/float64(size))
				blend(img, x, y, c, float64(a)/255*float64(c.A)/255)
			}
		}
	}
	appCache.m[size] = img
	out := image.NewRGBA(img.Rect)
	copy(out.Pix, img.Pix)
	return out
}

func blend(img *image.RGBA, x, y int, c color.RGBA, sa float64) {
	i := img.PixOffset(x, y)
	p := img.Pix[i : i+4 : i+4]
	k := 1 - sa
	p[0] = uint8(float64(c.R)*sa + float64(p[0])*k + .5)
	p[1] = uint8(float64(c.G)*sa + float64(p[1])*k + .5)
	p[2] = uint8(float64(c.B)*sa + float64(p[2])*k + .5)
	p[3] = uint8(255*sa + float64(p[3])*k + .5)
}
