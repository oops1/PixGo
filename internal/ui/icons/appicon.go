package icons

import (
	"bytes"
	_ "embed"
	"image"
	"image/png"
	"sync"

	xdraw "golang.org/x/image/draw"
)

//go:embed appicon.png
var appIconPNG []byte

var appIconSource = sync.OnceValue(func() *image.NRGBA {
	img, err := png.Decode(bytes.NewReader(appIconPNG))
	if err != nil {
		return image.NewNRGBA(image.Rect(0, 0, 1, 1))
	}
	b := img.Bounds()
	dst := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	xdraw.Copy(dst, image.Point{}, img, b, xdraw.Src, nil)
	return dst
})

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
	img, ok := appCache.m[size]
	if !ok {
		src := appIconSource()
		img = image.NewRGBA(image.Rect(0, 0, size, size))
		if src.Bounds().Dx() == size {
			xdraw.Copy(img, image.Point{}, src, src.Bounds(), xdraw.Src, nil)
		} else {
			xdraw.CatmullRom.Scale(img, img.Bounds(), src, src.Bounds(), xdraw.Src, nil)
		}
		appCache.m[size] = img
	}
	out := image.NewRGBA(img.Rect)
	copy(out.Pix, img.Pix)
	return out
}
