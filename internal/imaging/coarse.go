package imaging

import "image"

const (
	fracBits = 8
	fracOne  = 1 << fracBits
)

type axisTaps struct {
	i0   []int
	i1   []int
	frac []uint32
}

func (a *axisTaps) build(from, to int, factor uint, limit int) {
	n := to - from
	a.i0 = make([]int, n)
	a.i1 = make([]int, n)
	a.frac = make([]uint32, n)
	for k := 0; k < n; k++ {
		x := from + k
		v := ((2*x + 1) << (fracBits - 1)) >> factor
		v -= fracOne / 2
		i := v >> fracBits
		f := uint32(v & (fracOne - 1))
		switch {
		case i < 0:
			i, f = 0, 0
		case i >= limit-1:
			i, f = limit-1, 0
		}
		a.i0[k] = i * 4
		a.i1[k] = min(i+1, limit-1) * 4
		a.frac[k] = f
	}
}

func lerpLanes(a, b, w uint32) uint32 {
	const lane = 0x00FF00FF
	const round = 0x00800080
	e := ((a&lane)*(fracOne-w) + (b&lane)*w + round) >> fracBits & lane
	o := (((a>>8)&lane)*(fracOne-w) + ((b>>8)&lane)*w + round) >> fracBits & lane
	return e | o<<8
}

func fillFromCoarse(dst *image.NRGBA, org image.Point, region image.Rectangle, coarse *image.NRGBA, factor uint, opaque bool) {
	if region.Empty() || coarse == nil {
		return
	}
	cb := coarse.Rect
	cw, ch := cb.Dx(), cb.Dy()
	if cw <= 0 || ch <= 0 {
		return
	}
	var xt, yt axisTaps
	xt.build(region.Min.X, region.Max.X, factor, cw)
	yt.build(region.Min.Y, region.Max.Y, factor, ch)
	base := coarse.PixOffset(cb.Min.X, cb.Min.Y)
	n := region.Dx()
	for y := region.Min.Y; y < region.Max.Y; y++ {
		k := y - region.Min.Y
		r0 := coarse.Pix[base+yt.i0[k]/4*coarse.Stride:]
		r1 := coarse.Pix[base+yt.i1[k]/4*coarse.Stride:]
		wy := yt.frac[k]
		o := dst.PixOffset(region.Min.X-org.X+dst.Rect.Min.X, y-org.Y+dst.Rect.Min.Y)
		out := dst.Pix[o : o+n*4]
		if opaque {
			opaqueRow(out, r0, r1, &xt, wy)
		} else {
			alphaRow(out, r0, r1, &xt, wy)
		}
	}
}

func opaqueRow(out, r0, r1 []byte, xt *axisTaps, wy uint32) {
	for i := range xt.i0 {
		a, b, wx := xt.i0[i], xt.i1[i], xt.frac[i]
		var v uint32
		if wy == 0 {
			v = lerpLanes(le.Uint32(r0[a:]), le.Uint32(r0[b:]), wx)
		} else {
			top := lerpLanes(le.Uint32(r0[a:]), le.Uint32(r0[b:]), wx)
			bot := lerpLanes(le.Uint32(r1[a:]), le.Uint32(r1[b:]), wx)
			v = lerpLanes(top, bot, wy)
		}
		le.PutUint32(out[i*4:], v|opaqueAlphaBits)
	}
}

func alphaRow(out, r0, r1 []byte, xt *axisTaps, wy uint32) {
	for i := range xt.i0 {
		a, b, wx := xt.i0[i], xt.i1[i], xt.frac[i]
		taps := [4]uint32{le.Uint32(r0[a:]), le.Uint32(r0[b:]), le.Uint32(r1[a:]), le.Uint32(r1[b:])}
		weights := [4]uint32{(fracOne - wx) * (fracOne - wy), wx * (fracOne - wy), (fracOne - wx) * wy, wx * wy}
		var sumA, sumR, sumG, sumB uint32
		for t := 0; t < 4; t++ {
			p := taps[t]
			wa := weights[t] * (p >> 24)
			sumA += wa
			sumR += wa * (p & 0xFF)
			sumG += wa * ((p >> 8) & 0xFF)
			sumB += wa * ((p >> 16) & 0xFF)
		}
		if sumA == 0 {
			le.PutUint32(out[i*4:], 0)
			continue
		}
		half := sumA / 2
		r := (sumR + half) / sumA
		g := (sumG + half) / sumA
		bl := (sumB + half) / sumA
		alpha := (sumA + fracOne*fracOne/2) / (fracOne * fracOne)
		le.PutUint32(out[i*4:], r|g<<8|bl<<16|alpha<<24)
	}
}
