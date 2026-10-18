package convert

import (
	"image"
	"image/color"
)

func Deep(src image.Image) *image.NRGBA64 {
	if src == nil {
		return image.NewNRGBA64(image.Rectangle{})
	}
	bounds := src.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	switch s := src.(type) {
	case *image.NRGBA64:
		if s.Rect.Min == (image.Point{}) {
			return s
		}
	case *image.RGBA64:
		if s.Rect.Min == (image.Point{}) && s.Stride == width*8 {
			unpremultiply64(s.Pix[:width*height*8])
			return &image.NRGBA64{Pix: s.Pix, Stride: s.Stride, Rect: s.Rect}
		}
	}
	dst := image.NewNRGBA64(image.Rect(0, 0, width, height))
	if width == 0 || height == 0 {
		return dst
	}
	var rows func(y0, y1 int)
	switch s := src.(type) {
	case *image.Gray16:
		rows = func(y0, y1 int) { gray16Deep(dst, s, y0, y1) }
	case *image.Gray:
		rows = func(y0, y1 int) { grayDeep(dst, s, y0, y1) }
	case *image.NRGBA:
		rows = func(y0, y1 int) { nrgbaDeep(dst, s, y0, y1) }
	case *image.NRGBA64:
		rows = func(y0, y1 int) { nrgba64Copy(dst, s, y0, y1) }
	case *image.RGBA64:
		rows = func(y0, y1 int) { rgba64Deep(dst, s, y0, y1) }
	default:
		rows = func(y0, y1 int) { genericDeep(dst, src, y0, y1) }
	}
	Bands(width, height, rows)
	return dst
}

func Widen(src *image.NRGBA) *image.NRGBA64 {
	return Deep(src)
}

func unpremultiply64(pix []byte) {
	for o := 0; o+8 <= len(pix); o += 8 {
		a := uint32(pix[o+6])<<8 | uint32(pix[o+7])
		switch a {
		case 0xffff:
		case 0:
			clear(pix[o : o+6])
		default:
			for c := 0; c < 6; c += 2 {
				v := uint32(pix[o+c])<<8 | uint32(pix[o+c+1])
				v = min(v*0xffff/a, 0xffff)
				pix[o+c], pix[o+c+1] = byte(v>>8), byte(v)
			}
		}
	}
}

func gray16Deep(dst *image.NRGBA64, src *image.Gray16, y0, y1 int) {
	width := src.Rect.Dx()
	for row := y0; row < y1; row++ {
		in := src.Pix[src.PixOffset(src.Rect.Min.X, src.Rect.Min.Y+row):]
		out := dst.Pix[row*dst.Stride : row*dst.Stride+width*8]
		for i := 0; i < width; i++ {
			hi, lo := in[2*i], in[2*i+1]
			o := i * 8
			out[o], out[o+1], out[o+2], out[o+3], out[o+4], out[o+5] = hi, lo, hi, lo, hi, lo
			out[o+6], out[o+7] = 0xff, 0xff
		}
	}
}

func grayDeep(dst *image.NRGBA64, src *image.Gray, y0, y1 int) {
	width := src.Rect.Dx()
	for row := y0; row < y1; row++ {
		in := src.Pix[src.PixOffset(src.Rect.Min.X, src.Rect.Min.Y+row):]
		out := dst.Pix[row*dst.Stride : row*dst.Stride+width*8]
		for i := 0; i < width; i++ {
			v := in[i]
			o := i * 8
			out[o], out[o+1], out[o+2], out[o+3], out[o+4], out[o+5] = v, v, v, v, v, v
			out[o+6], out[o+7] = 0xff, 0xff
		}
	}
}

func nrgbaDeep(dst *image.NRGBA64, src *image.NRGBA, y0, y1 int) {
	width := src.Rect.Dx()
	for row := y0; row < y1; row++ {
		in := src.Pix[src.PixOffset(src.Rect.Min.X, src.Rect.Min.Y+row):]
		out := dst.Pix[row*dst.Stride : row*dst.Stride+width*8]
		for i := 0; i < width; i++ {
			o := i * 8
			for c := 0; c < 4; c++ {
				v := in[4*i+c]
				out[o+2*c], out[o+2*c+1] = v, v
			}
		}
	}
}

func nrgba64Copy(dst *image.NRGBA64, src *image.NRGBA64, y0, y1 int) {
	width := src.Rect.Dx()
	for row := y0; row < y1; row++ {
		start := src.PixOffset(src.Rect.Min.X, src.Rect.Min.Y+row)
		copy(dst.Pix[row*dst.Stride:row*dst.Stride+width*8], src.Pix[start:start+width*8])
	}
}

func rgba64Deep(dst *image.NRGBA64, src *image.RGBA64, y0, y1 int) {
	width := src.Rect.Dx()
	for row := y0; row < y1; row++ {
		start := src.PixOffset(src.Rect.Min.X, src.Rect.Min.Y+row)
		out := dst.Pix[row*dst.Stride : row*dst.Stride+width*8]
		copy(out, src.Pix[start:start+width*8])
		unpremultiply64(out)
	}
}

func genericDeep(dst *image.NRGBA64, src image.Image, y0, y1 int) {
	bounds := src.Bounds()
	width := bounds.Dx()
	for row := y0; row < y1; row++ {
		out := dst.Pix[row*dst.Stride : row*dst.Stride+width*8]
		for i := 0; i < width; i++ {
			c := color.NRGBA64Model.Convert(src.At(bounds.Min.X+i, bounds.Min.Y+row)).(color.NRGBA64)
			o := i * 8
			out[o], out[o+1] = byte(c.R>>8), byte(c.R)
			out[o+2], out[o+3] = byte(c.G>>8), byte(c.G)
			out[o+4], out[o+5] = byte(c.B>>8), byte(c.B)
			out[o+6], out[o+7] = byte(c.A>>8), byte(c.A)
		}
	}
}
