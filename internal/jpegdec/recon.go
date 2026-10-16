package jpegdec

import (
	"image"
	"sync"
)

type geometry struct {
	shift      int
	k          int
	mxx, myy   int
	mcuW, mcuH int
	outW, outH int
	kcx, kcy   [maxComponents]int
	ks, kk     [maxComponents]int
}

func (f *frame) geometry(shift, storedShift int) *geometry {
	scale := 1 << uint(shift)
	mxx, myy := f.mcuCounts()
	k := 8 >> uint(shift)
	g := &geometry{
		shift: shift,
		k:     k,
		mxx:   mxx,
		myy:   myy,
		mcuW:  f.maxH * k,
		mcuH:  f.maxV * k,
		outW:  (f.width + scale - 1) / scale,
		outH:  (f.height + scale - 1) / scale,
	}
	stored := 8 >> uint(storedShift)
	for c := 0; c < f.nComp; c++ {
		expandH, expandV := 1<<f.comp[c].shiftH, 1<<f.comp[c].shiftV
		g.kcx[c] = min(8, k*expandH)
		g.kcy[c] = min(8, k*expandV)
		g.ks[c] = max(min(8, stored*expandH), min(8, stored*expandV))
		g.kk[c] = g.ks[c] * g.ks[c]
	}
	return g
}

func (g *geometry) replication(comp *component, c int) (uint8, uint8) {
	h, v := comp.shiftH, comp.shiftV
	for n := g.kcx[c] / g.k; n > 1; n >>= 1 {
		h--
	}
	for n := g.kcy[c] / g.k; n > 1; n >>= 1 {
		v--
	}
	return h, v
}

type blockFunc func(sc *scratch, coef []int16, stride int, q *[blockSize]int32, dst []uint8, outStride int)

func blockFor(kx, ky int) blockFunc {
	if kx != ky {
		return rectangularBlock(kx, ky)
	}
	switch kx {
	case 8:
		return (*scratch).block8
	case 4:
		return (*scratch).block4
	case 2:
		return (*scratch).block2
	}
	return (*scratch).block1
}

type recon struct {
	f     *frame
	mode  colorMode
	g     *geometry
	q     [maxComponents][blockSize]int32
	block [maxComponents]blockFunc
	hs    [maxComponents]uint8
	vs    [maxComponents]uint8
}

func newRecon(f *frame, mode colorMode, g *geometry, quant *[maxComponents][blockSize]int32) *recon {
	r := &recon{f: f, mode: mode, g: g, q: *quant}
	for c := 0; c < f.nComp; c++ {
		r.block[c] = blockFor(g.kcx[c], g.kcy[c])
		r.hs[c], r.vs[c] = g.replication(&f.comp[c], c)
	}
	return r
}

func dequantTables(f *frame) [maxComponents][blockSize]int32 {
	var q [maxComponents][blockSize]int32
	for c := 0; c < f.nComp; c++ {
		table := &f.quant[f.comp[c].tq]
		for z := 0; z < blockSize; z++ {
			q[c][unzig[z]] = table[z]
		}
	}
	return q
}

type scratch struct {
	blk    block
	planes [maxComponents][]uint8
	rows   rowSet
}

var scratchPool = sync.Pool{New: func() any { return new(scratch) }}

func (r *recon) row(sc *scratch, coef [maxComponents][]int16, cstride [maxComponents]int, nCols, mxFrom, my int, out *image.NRGBA, rect image.Rectangle) {
	g := r.g
	f := r.f
	mcuY0 := my * g.mcuH
	ys, ye := max(rect.Min.Y, mcuY0), min(rect.Max.Y, mcuY0+g.mcuH)
	mcuX0 := mxFrom * g.mcuW
	xs, xe := max(rect.Min.X, mcuX0), min(rect.Max.X, mcuX0+nCols*g.mcuW)
	if ys >= ye || xs >= xe {
		return
	}
	var planeW [maxComponents]int
	for c := 0; c < f.nComp; c++ {
		comp := &f.comp[c]
		kx, ky := g.kcx[c], g.kcy[c]
		planeW[c] = nCols * comp.h * kx
		need := planeW[c] * comp.v * ky
		if cap(sc.planes[c]) < need {
			sc.planes[c] = make([]uint8, need)
		}
		plane := sc.planes[c][:need]
		byLo := ((ys - mcuY0) >> r.vs[c]) / ky
		byHi := ((ye - 1 - mcuY0) >> r.vs[c]) / ky
		bxLo := ((xs - mcuX0) >> r.hs[c]) / kx
		bxHi := ((xe - 1 - mcuX0) >> r.hs[c]) / kx
		for by := byLo; by <= byHi; by++ {
			for bx := bxLo; bx <= bxHi; bx++ {
				blk := coef[c][(by*cstride[c]+bx)*g.kk[c]:]
				r.block[c](sc, blk, g.ks[c], &r.q[c], plane[by*ky*planeW[c]+bx*kx:], planeW[c])
			}
		}
		sc.rows.shift[c] = r.hs[c]
	}
	first := xs - mcuX0
	n := xe - xs
	for y := ys; y < ye; y++ {
		ry := y - mcuY0
		for c := 0; c < f.nComp; c++ {
			pr := ry >> r.vs[c]
			sc.rows.rows[c] = sc.planes[c][pr*planeW[c] : (pr+1)*planeW[c]]
		}
		o := (y-rect.Min.Y)*out.Stride + (xs-rect.Min.X)*4
		convertRow(r.mode, out.Pix[o:o+n*4], &sc.rows, first, n)
	}
}
