package thumbs

import (
	"image"
	"image/draw"
)

func fitSize(w, h, side int) (int, int) {
	if w <= 0 || h <= 0 || side <= 0 {
		return 0, 0
	}
	if w <= side && h <= side {
		return w, h
	}
	if w >= h {
		nh := int((int64(h)*int64(side) + int64(w)/2) / int64(w))
		return side, max(nh, 1)
	}
	nw := int((int64(w)*int64(side) + int64(h)/2) / int64(h))
	return max(nw, 1), side
}

func scaleHint(longest, side int) int {
	if side <= 0 {
		return 1
	}
	for _, n := range [...]int{8, 4, 2} {
		if (longest+n-1)/n >= side {
			return n
		}
	}
	return 1
}

type span struct {
	first  int
	last   int
	wFirst uint64
	wLast  uint64
}

func buildSpans(sn, dn int) []span {
	spans := make([]span, dn)
	s64, d64 := int64(sn), int64(dn)
	for x := range spans {
		lo := int64(x) * s64
		hi := lo + s64
		first := lo / d64
		last := (hi - 1) / d64
		sp := span{first: int(first), last: int(last)}
		if first == last {
			sp.wFirst = uint64(hi - lo)
		} else {
			sp.wFirst = uint64((first+1)*d64 - lo)
			sp.wLast = uint64(hi - last*d64)
		}
		spans[x] = sp
	}
	return spans
}

func (sp span) weightOf(i int, full uint64) uint64 {
	switch {
	case sp.first == sp.last:
		return sp.wFirst
	case i == sp.first:
		return sp.wFirst
	case i == sp.last:
		return sp.wLast
	}
	return full
}

func horizontal(row []byte, spans []span, dw uint64, out []uint64) {
	for x := range spans {
		sp := &spans[x]
		var r, g, b, a uint64
		if sp.first == sp.last {
			q := row[sp.first*4 : sp.first*4+4 : sp.first*4+4]
			pa := uint64(q[3])
			w := sp.wFirst
			r = w * uint64(q[0]) * pa
			g = w * uint64(q[1]) * pa
			b = w * uint64(q[2]) * pa
			a = w * pa
		} else {
			q := row[sp.first*4 : sp.first*4+4 : sp.first*4+4]
			pa := uint64(q[3])
			w := sp.wFirst
			r = w * uint64(q[0]) * pa
			g = w * uint64(q[1]) * pa
			b = w * uint64(q[2]) * pa
			a = w * pa
			var mr, mg, mb, ma uint64
			for i := sp.first + 1; i < sp.last; i++ {
				q := row[i*4 : i*4+4 : i*4+4]
				pa := uint64(q[3])
				mr += uint64(q[0]) * pa
				mg += uint64(q[1]) * pa
				mb += uint64(q[2]) * pa
				ma += pa
			}
			r += mr * dw
			g += mg * dw
			b += mb * dw
			a += ma * dw
			q = row[sp.last*4 : sp.last*4+4 : sp.last*4+4]
			pa = uint64(q[3])
			w = sp.wLast
			r += w * uint64(q[0]) * pa
			g += w * uint64(q[1]) * pa
			b += w * uint64(q[2]) * pa
			a += w * pa
		}
		o := out[x*4 : x*4+4 : x*4+4]
		o[0], o[1], o[2], o[3] = r, g, b, a*255
	}
}

func premultiplyCopy(src *image.NRGBA, dst *image.RGBA) {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	base := src.PixOffset(b.Min.X, b.Min.Y)
	for y := 0; y < h; y++ {
		in := src.Pix[base+y*src.Stride : base+y*src.Stride+w*4]
		out := dst.Pix[y*dst.Stride : y*dst.Stride+w*4]
		for i := 0; i < len(in); i += 4 {
			a := uint32(in[i+3])
			if a == 255 {
				out[i], out[i+1], out[i+2], out[i+3] = in[i], in[i+1], in[i+2], 255
				continue
			}
			out[i] = uint8((uint32(in[i])*a + 127) / 255)
			out[i+1] = uint8((uint32(in[i+1])*a + 127) / 255)
			out[i+2] = uint8((uint32(in[i+2])*a + 127) / 255)
			out[i+3] = uint8(a)
		}
	}
}

func resizeArea(src *image.NRGBA, dw, dh int) *image.RGBA {
	if src == nil {
		return image.NewRGBA(image.Rectangle{})
	}
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	if dw <= 0 || dh <= 0 || sw <= 0 || sh <= 0 {
		return image.NewRGBA(image.Rectangle{})
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	if dw == sw && dh == sh {
		premultiplyCopy(src, dst)
		return dst
	}
	xs := buildSpans(sw, dw)
	ys := buildSpans(sh, dh)
	hrow := make([]uint64, dw*4)
	acc := make([]uint64, dw*4)
	div := uint64(sw) * uint64(sh) * 255
	half := div / 2
	base := src.PixOffset(b.Min.X, b.Min.Y)
	cached := -1
	for y := 0; y < dh; y++ {
		sp := ys[y]
		clear(acc)
		for j := sp.first; j <= sp.last; j++ {
			if j != cached {
				o := base + j*src.Stride
				horizontal(src.Pix[o:o+sw*4], xs, uint64(dw), hrow)
				cached = j
			}
			wt := sp.weightOf(j, uint64(dh))
			for k, v := range hrow {
				acc[k] += wt * v
			}
		}
		out := dst.Pix[y*dst.Stride : y*dst.Stride+dw*4]
		for k, v := range acc {
			out[k] = uint8((v + half) / div)
		}
	}
	return dst
}

func toRGBA(src image.Image) *image.RGBA {
	b := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), src, b.Min, draw.Src)
	return dst
}

func toNRGBA(src image.Image) *image.NRGBA {
	if n, ok := src.(*image.NRGBA); ok {
		return n
	}
	b := src.Bounds()
	dst := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), src, b.Min, draw.Src)
	return dst
}

func isOpaque(img *image.RGBA) bool {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	for y := 0; y < h; y++ {
		row := img.Pix[y*img.Stride : y*img.Stride+w*4]
		for i := 3; i < len(row); i += 4 {
			if row[i] != 255 {
				return false
			}
		}
	}
	return true
}
