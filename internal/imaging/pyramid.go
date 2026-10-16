package imaging

import (
	"encoding/binary"
	"image"
	"sync/atomic"

	"github.com/oops1/PixGo/internal/par"
	"github.com/oops1/PixGo/internal/render"
)

const (
	defaultMinSide   = 256
	pyramidRowsChunk = 16
	pyramidSmallPx   = 1 << 15
	laneMask         = 0x00FF00FF
	laneRound        = 0x00020002
	opaqueAlphaBits  = 0xFF000000
)

var le = binary.LittleEndian

func BuildPyramid(level0 *image.NRGBA, minSide int) []*image.NRGBA {
	if level0 == nil {
		return nil
	}
	if minSide <= 0 {
		minSide = defaultMinSide
	}
	levels := []*image.NRGBA{level0}
	for {
		prev := levels[len(levels)-1]
		pb := prev.Bounds()
		w, h := pb.Dx(), pb.Dy()
		nw, nh := (w+1)/2, (h+1)/2
		if w <= 0 || h <= 0 || (w <= 1 && h <= 1) || max(nw, nh) < minSide {
			break
		}
		next := image.NewNRGBA(image.Rect(0, 0, nw, nh))
		if halve(prev, next) {
			render.MarkOpaque(prev)
			render.MarkOpaque(next)
		}
		levels = append(levels, next)
	}
	return levels
}

func halve(src, dst *image.NRGBA) bool {
	sb := src.Bounds()
	w, h := sb.Dx(), sb.Dy()
	nw, nh := dst.Rect.Dx(), dst.Rect.Dy()
	workers := par.Count()
	if nw*nh < pyramidSmallPx {
		workers = 1
	}
	chunk := max(pyramidRowsChunk, (nh+workers*3-1)/(workers*3))
	tasks := (nh + chunk - 1) / chunk
	var translucent atomic.Bool
	par.Parallel(tasks, workers, func(t int) {
		y0, y1 := t*chunk, min(nh, (t+1)*chunk)
		if !halveRows(src, dst, w, h, nw, y0, y1) {
			translucent.Store(true)
		}
	})
	return !translucent.Load()
}

func halveRows(src, dst *image.NRGBA, w, h, nw, y0, y1 int) bool {
	sb := src.Bounds()
	base := src.PixOffset(sb.Min.X, sb.Min.Y)
	acc := ^uint32(0)
	for y := y0; y < y1; y++ {
		r0 := base + 2*y*src.Stride
		r1 := base + min(2*y+1, h-1)*src.Stride
		row0 := src.Pix[r0 : r0+w*4]
		row1 := src.Pix[r1 : r1+w*4]
		out := dst.Pix[y*dst.Stride : y*dst.Stride+nw*4]
		full := w / 2
		x := 0
		for ; x < full; x++ {
			o := x * 8
			p0 := le.Uint32(row0[o:])
			p1 := le.Uint32(row0[o+4:])
			p2 := le.Uint32(row1[o:])
			p3 := le.Uint32(row1[o+4:])
			le.PutUint32(out[x*4:], average(p0, p1, p2, p3, &acc))
		}
		if x < nw {
			o := x * 8
			p0 := le.Uint32(row0[o:])
			p2 := le.Uint32(row1[o:])
			le.PutUint32(out[x*4:], average(p0, p0, p2, p2, &acc))
		}
	}
	return acc>>24 == 0xFF
}

func average(p0, p1, p2, p3 uint32, acc *uint32) uint32 {
	all := p0 & p1 & p2 & p3
	*acc &= all
	if all>>24 == 0xFF {
		e := (p0 & laneMask) + (p1 & laneMask) + (p2 & laneMask) + (p3 & laneMask) + laneRound
		o := ((p0 >> 8) & laneMask) + ((p1 >> 8) & laneMask) + ((p2 >> 8) & laneMask) + ((p3 >> 8) & laneMask) + laneRound
		return ((e >> 2) & laneMask) | (((o >> 2) & laneMask) << 8)
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
