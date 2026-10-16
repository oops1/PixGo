package reduce

import (
	"encoding/binary"
	"image"
)

const (
	laneMask  = 0x00FF00FF
	laneRound = 0x00020002
)

var le = binary.LittleEndian

func Halve(src *image.NRGBA) *image.NRGBA {
	w, h := src.Rect.Dx(), src.Rect.Dy()
	if w <= 0 || h <= 0 {
		return image.NewNRGBA(image.Rect(0, 0, 0, 0))
	}
	nw, nh := (w+1)/2, (h+1)/2
	dst := image.NewNRGBA(image.Rect(0, 0, nw, nh))
	halveInto(dst, src, w, h, nw, nh)
	return dst
}

func halveInto(dst, src *image.NRGBA, w, h, nw, nh int) {
	base := src.PixOffset(src.Rect.Min.X, src.Rect.Min.Y)
	for y := 0; y < nh; y++ {
		first := base + 2*y*src.Stride
		second := base + min(2*y+1, h-1)*src.Stride
		halveRow(dst.Pix[y*dst.Stride:y*dst.Stride+nw*4], src.Pix[first:first+w*4], src.Pix[second:second+w*4], w, nw)
	}
}

func halveRow(out, upper, lower []byte, w, nw int) {
	full := w / 2
	x := 0
	for ; x < full; x++ {
		o := x * 8
		le.PutUint32(out[x*4:], average(le.Uint32(upper[o:]), le.Uint32(upper[o+4:]), le.Uint32(lower[o:]), le.Uint32(lower[o+4:])))
	}
	if x < nw {
		o := x * 8
		p0 := le.Uint32(upper[o:])
		p2 := le.Uint32(lower[o:])
		le.PutUint32(out[x*4:], average(p0, p0, p2, p2))
	}
}

func average(p0, p1, p2, p3 uint32) uint32 {
	if (p0&p1&p2&p3)>>24 == 0xFF {
		even := (p0 & laneMask) + (p1 & laneMask) + (p2 & laneMask) + (p3 & laneMask) + laneRound
		odd := ((p0 >> 8) & laneMask) + ((p1 >> 8) & laneMask) + ((p2 >> 8) & laneMask) + ((p3 >> 8) & laneMask) + laneRound
		return ((even >> 2) & laneMask) | (((odd >> 2) & laneMask) << 8)
	}
	a0, a1, a2, a3 := p0>>24, p1>>24, p2>>24, p3>>24
	total := a0 + a1 + a2 + a3
	if total == 0 {
		return 0
	}
	r := (a0*(p0&0xFF) + a1*(p1&0xFF) + a2*(p2&0xFF) + a3*(p3&0xFF) + total/2) / total
	g := (a0*((p0>>8)&0xFF) + a1*((p1>>8)&0xFF) + a2*((p2>>8)&0xFF) + a3*((p3>>8)&0xFF) + total/2) / total
	b := (a0*((p0>>16)&0xFF) + a1*((p1>>16)&0xFF) + a2*((p2>>16)&0xFF) + a3*((p3>>16)&0xFF) + total/2) / total
	return r | g<<8 | b<<16 | ((total+2)>>2)<<24
}

func Levels(src *image.NRGBA, n int) *image.NRGBA {
	for ; n > 0; n-- {
		src = Halve(src)
	}
	return src
}

type Scratch struct {
	stages [2]*image.NRGBA
}

func (s *Scratch) Levels(src *image.NRGBA, n int) *image.NRGBA {
	for stage := 0; n > 0; n, stage = n-1, stage^1 {
		src = s.halve(src, stage)
	}
	return src
}

func (s *Scratch) halve(src *image.NRGBA, stage int) *image.NRGBA {
	w, h := src.Rect.Dx(), src.Rect.Dy()
	if w <= 0 || h <= 0 {
		return image.NewNRGBA(image.Rect(0, 0, 0, 0))
	}
	nw, nh := (w+1)/2, (h+1)/2
	dst := s.stages[stage]
	if dst == nil || cap(dst.Pix) < nw*nh*4 {
		dst = image.NewNRGBA(image.Rect(0, 0, nw, nh))
	} else {
		dst.Pix = dst.Pix[:nw*nh*4]
		dst.Stride = nw * 4
		dst.Rect = image.Rect(0, 0, nw, nh)
	}
	s.stages[stage] = dst
	halveInto(dst, src, w, h, nw, nh)
	return dst
}
