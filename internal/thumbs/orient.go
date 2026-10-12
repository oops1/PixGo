package thumbs

import (
	"encoding/binary"
	"image"
	"math"

	"github.com/oops1/PixGo/internal/codec"
)

var le = binary.LittleEndian

func validOrientation(o codec.Orientation) codec.Orientation {
	if o.Valid() {
		return o
	}
	return codec.OrientNormal
}

func orientedSize(o codec.Orientation, w, h int) (int, int) {
	return validOrientation(o).Size(w, h)
}

func storedSize(o codec.Orientation, w, h int) (int, int) {
	if validOrientation(o).Transposes() {
		return h, w
	}
	return w, h
}

func orient(src *image.RGBA, o codec.Orientation) *image.RGBA {
	o = validOrientation(o)
	if o == codec.OrientNormal || src == nil {
		return src
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return src
	}
	dw, dh := o.Size(w, h)
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	a, bb, c, d, e, f := o.Matrix(float64(w), float64(h))
	sx0 := int(math.Floor(0.5*a + 0.5*bb + c))
	sy0 := int(math.Floor(0.5*d + 0.5*e + f))
	stepX := int(a) + int(d)*w
	stepY := int(bb) + int(e)*w
	base := sy0*w + sx0
	pix := src.Pix
	if src.Stride != w*4 || b.Min != (image.Point{}) {
		pix = make([]byte, w*h*4)
		for y := 0; y < h; y++ {
			off := src.PixOffset(b.Min.X, b.Min.Y+y)
			copy(pix[y*w*4:(y+1)*w*4], src.Pix[off:off+w*4])
		}
	}
	for y := 0; y < dh; y++ {
		p := base + y*stepY
		row := dst.Pix[y*dst.Stride : y*dst.Stride+dw*4]
		for x := 0; x < dw; x++ {
			le.PutUint32(row[x*4:], le.Uint32(pix[p*4:]))
			p += stepX
		}
	}
	return dst
}
