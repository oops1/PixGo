package reduce

import (
	"encoding/binary"
	"image"
)

var be = binary.BigEndian

func HalveDeep(src *image.NRGBA64) *image.NRGBA64 {
	w, h := src.Rect.Dx(), src.Rect.Dy()
	if w <= 0 || h <= 0 {
		return image.NewNRGBA64(image.Rect(0, 0, 0, 0))
	}
	nw, nh := (w+1)/2, (h+1)/2
	dst := image.NewNRGBA64(image.Rect(0, 0, nw, nh))
	halveDeepInto(dst, src, w, h, nw, nh)
	return dst
}

func halveDeepInto(dst, src *image.NRGBA64, w, h, nw, nh int) {
	base := src.PixOffset(src.Rect.Min.X, src.Rect.Min.Y)
	for y := 0; y < nh; y++ {
		first := base + 2*y*src.Stride
		second := base + min(2*y+1, h-1)*src.Stride
		halveDeepRow(dst.Pix[y*dst.Stride:y*dst.Stride+nw*8], src.Pix[first:first+w*8], src.Pix[second:second+w*8], w, nw)
	}
}

func halveDeepRow(out, upper, lower []byte, w, nw int) {
	for x := 0; x < nw; x++ {
		left := x * 16
		right := left + 8
		if 2*x+1 >= w {
			right = left
		}
		averageDeep(out[x*8:x*8+8], upper[left:left+8], upper[right:right+8], lower[left:left+8], lower[right:right+8])
	}
}

func averageDeep(out, p0, p1, p2, p3 []byte) {
	a0, a1, a2, a3 := uint64(be.Uint16(p0[6:])), uint64(be.Uint16(p1[6:])), uint64(be.Uint16(p2[6:])), uint64(be.Uint16(p3[6:]))
	if a0&a1&a2&a3 == 0xffff {
		for c := 0; c < 6; c += 2 {
			sum := uint32(be.Uint16(p0[c:])) + uint32(be.Uint16(p1[c:])) + uint32(be.Uint16(p2[c:])) + uint32(be.Uint16(p3[c:]))
			be.PutUint16(out[c:], uint16((sum+2)>>2))
		}
		out[6], out[7] = 0xff, 0xff
		return
	}
	total := a0 + a1 + a2 + a3
	if total == 0 {
		clear(out[:8])
		return
	}
	for c := 0; c < 6; c += 2 {
		sum := a0*uint64(be.Uint16(p0[c:])) + a1*uint64(be.Uint16(p1[c:])) + a2*uint64(be.Uint16(p2[c:])) + a3*uint64(be.Uint16(p3[c:]))
		be.PutUint16(out[c:], uint16((sum+total/2)/total))
	}
	be.PutUint16(out[6:], uint16((total+2)>>2))
}

func LevelsDeep(src *image.NRGBA64, n int) *image.NRGBA64 {
	for ; n > 0; n-- {
		src = HalveDeep(src)
	}
	return src
}

type ScratchDeep struct {
	stages [2]*image.NRGBA64
}

func (s *ScratchDeep) Levels(src *image.NRGBA64, n int) *image.NRGBA64 {
	for stage := 0; n > 0; n, stage = n-1, stage^1 {
		src = s.halve(src, stage)
	}
	return src
}

func (s *ScratchDeep) halve(src *image.NRGBA64, stage int) *image.NRGBA64 {
	w, h := src.Rect.Dx(), src.Rect.Dy()
	if w <= 0 || h <= 0 {
		return image.NewNRGBA64(image.Rect(0, 0, 0, 0))
	}
	nw, nh := (w+1)/2, (h+1)/2
	dst := s.stages[stage]
	if dst == nil || cap(dst.Pix) < nw*nh*8 {
		dst = image.NewNRGBA64(image.Rect(0, 0, nw, nh))
	} else {
		dst.Pix = dst.Pix[:nw*nh*8]
		dst.Stride = nw * 8
		dst.Rect = image.Rect(0, 0, nw, nh)
	}
	s.stages[stage] = dst
	halveDeepInto(dst, src, w, h, nw, nh)
	return dst
}
