package colorm

import (
	"github.com/oops1/PixGo/internal/par"
)

const (
	grid3 = 33
	grid4 = 17
)

type lutPath struct {
	n    int
	data []float32
	lo   [256]int32
	fr   [256]float32
	scl  float32
}

func (t *Transform) dstEncode(xyz Vec3) [3]float64 {
	if t.dst.kind == kindLUT || t.opt.Intent == Perceptual && t.dst.b2aFor(t.opt.Intent) != nil {
		v := t.dst.b2aFor(t.opt.Intent).fromPCS(xyz)
		return [3]float64{clamp01(v[0]), clamp01(v[1]), clamp01(v[2])}
	}
	lin := t.dstInv.Apply(xyz)
	if t.opt.SoftClip {
		lin = softClip64(lin, t.dst.colorants)
	}
	return [3]float64{t.dst.trc[0].Inverse(lin[0]), t.dst.trc[1].Inverse(lin[1]), t.dst.trc[2].Inverse(lin[2])}
}

func (t *Transform) devToDst(dev [4]float64) [3]float64 {
	toPCS := t.src.toPCS(t.opt.Intent)
	return t.dstEncode(t.bpc.apply(toPCS(dev)))
}

func newLUTPath(n int) *lutPath {
	lp := &lutPath{n: n}
	for v := 0; v < 256; v++ {
		p := float32(v) * float32(n-1) / 255
		i := int32(p)
		if int(i) > n-2 {
			i = int32(n - 2)
		}
		lp.lo[v] = i
		lp.fr[v] = p - float32(i)
	}
	lp.scl = float32(n-1) / 65535
	return lp
}

func buildLUT3(t *Transform, srcLUT, dstLUT bool) (*Transform, error) {
	t.setupBPC(srcLUT)
	lp := newLUTPath(grid3)
	n := grid3
	lp.data = make([]float32, n*n*n*3)
	par.Parallel(n, par.Count(), func(ri int) {
		for gi := 0; gi < n; gi++ {
			for bi := 0; bi < n; bi++ {
				dev := [4]float64{float64(ri) / float64(n-1), float64(gi) / float64(n-1), float64(bi) / float64(n-1)}
				out := t.devToDst(dev)
				o := ((ri*n+gi)*n + bi) * 3
				lp.data[o], lp.data[o+1], lp.data[o+2] = float32(out[0]), float32(out[1]), float32(out[2])
			}
		}
	})
	t.lp = lp
	t.mode = modeLUT3
	t.pixelFn = func(r, g, b float64) (float64, float64, float64) {
		o := t.devToDst([4]float64{r, g, b})
		return o[0], o[1], o[2]
	}
	return t, nil
}

func buildCMYK(t *Transform) (*Transform, error) {
	if t.src.a2bFor(t.opt.Intent) == nil {
		return nil, ErrUnsupported
	}
	t.setupBPC(true)
	n := grid4
	lp := newLUTPath(n)
	lp.data = make([]float32, n*n*n*n*3)
	par.Parallel(n, par.Count(), func(ci int) {
		for mi := 0; mi < n; mi++ {
			for yi := 0; yi < n; yi++ {
				for ki := 0; ki < n; ki++ {
					dev := [4]float64{float64(ci) / float64(n-1), float64(mi) / float64(n-1), float64(yi) / float64(n-1), float64(ki) / float64(n-1)}
					out := t.devToDst(dev)
					o := (((ci*n+mi)*n+yi)*n + ki) * 3
					lp.data[o], lp.data[o+1], lp.data[o+2] = float32(out[0]), float32(out[1]), float32(out[2])
				}
			}
		}
	})
	t.lp = lp
	t.mode = modeCMYK
	t.pixelFn = func(c, m, y float64) (float64, float64, float64) {
		o := t.devToDst([4]float64{c, m, y, 0})
		return o[0], o[1], o[2]
	}
	return t, nil
}

func (t *Transform) PixelCMYK(c, m, y, k float64) (float64, float64, float64) {
	o := t.devToDst([4]float64{c, m, y, k})
	return o[0], o[1], o[2]
}

func tetra(g []float32, base, sr, sg, sb int, x, y, z float32, out *[3]float32) {
	var o1, o2, o3 int
	var w1, w2, w3 float32
	if x >= y {
		switch {
		case y >= z:
			o1, o2, o3, w1, w2, w3 = sr, sr+sg, sr+sg+sb, x, y, z
		case x >= z:
			o1, o2, o3, w1, w2, w3 = sr, sr+sb, sr+sg+sb, x, z, y
		default:
			o1, o2, o3, w1, w2, w3 = sb, sr+sb, sr+sg+sb, z, x, y
		}
	} else {
		switch {
		case x >= z:
			o1, o2, o3, w1, w2, w3 = sg, sr+sg, sr+sg+sb, y, x, z
		case y >= z:
			o1, o2, o3, w1, w2, w3 = sg, sg+sb, sr+sg+sb, y, z, x
		default:
			o1, o2, o3, w1, w2, w3 = sb, sg+sb, sr+sg+sb, z, y, x
		}
	}
	c0 := g[base : base+3 : base+3]
	c1 := g[base+o1 : base+o1+3 : base+o1+3]
	c2 := g[base+o2 : base+o2+3 : base+o2+3]
	c3 := g[base+o3 : base+o3+3 : base+o3+3]
	out[0] = c0[0] + w1*(c1[0]-c0[0]) + w2*(c2[0]-c1[0]) + w3*(c3[0]-c2[0])
	out[1] = c0[1] + w1*(c1[1]-c0[1]) + w2*(c2[1]-c1[1]) + w3*(c3[1]-c2[1])
	out[2] = c0[2] + w1*(c1[2]-c0[2]) + w2*(c2[2]-c1[2]) + w3*(c3[2]-c2[2])
}

func quant(v, bias float32) uint8 {
	f := int32(v*255 + bias)
	if uint32(f) > 255 {
		if f < 0 {
			return 0
		}
		return 255
	}
	return uint8(f)
}

func (lp *lutPath) row8(px []byte) {
	n := lp.n
	sr, sg, sb := n*n*3, n*3, 3
	var out [3]float32
	for i := 0; i+3 < len(px); i += 4 {
		r, g, b := px[i], px[i+1], px[i+2]
		base := int(lp.lo[r])*sr + int(lp.lo[g])*sg + int(lp.lo[b])*sb
		tetra(lp.data, base, sr, sg, sb, lp.fr[r], lp.fr[g], lp.fr[b], &out)
		px[i], px[i+1], px[i+2] = quant(out[0], 0.5), quant(out[1], 0.5), quant(out[2], 0.5)
	}
}

func (lp *lutPath) pos16(v uint32) (int, float32) {
	p := float32(v) * lp.scl
	i := int(p)
	if i > lp.n-2 {
		i = lp.n - 2
	}
	return i, p - float32(i)
}

func (lp *lutPath) row16(d, s []byte, y int, dither bool) {
	n := lp.n
	sr, sg, sb := n*n*3, n*3, 3
	var out [3]float32
	cnt := len(d) / 4
	for x := 0; x < cnt; x++ {
		so := x * 8
		ri, rf := lp.pos16(uint32(s[so])<<8 | uint32(s[so+1]))
		gi, gf := lp.pos16(uint32(s[so+2])<<8 | uint32(s[so+3]))
		bi, bf := lp.pos16(uint32(s[so+4])<<8 | uint32(s[so+5]))
		tetra(lp.data, ri*sr+gi*sg+bi*sb, sr, sg, sb, rf, gf, bf, &out)
		bias := float32(ditherAt(x, y, dither)) * (1.0 / 256)
		do := x * 4
		d[do], d[do+1], d[do+2] = quant(out[0], bias), quant(out[1], bias), quant(out[2], bias)
		a := uint32(s[so+6])<<8 | uint32(s[so+7])
		d[do+3] = uint8((a*255 + 32767) / 65535)
	}
}

func (lp *lutPath) rowCMYK(d, s []byte) {
	n := lp.n
	sk := 3
	sy := 3 * n
	sm := 3 * n * n
	sc := 3 * n * n * n
	var o0, o1 [3]float32
	for i := 0; i+3 < len(d); i += 4 {
		c, m, y, k := s[i], s[i+1], s[i+2], s[i+3]
		base := int(lp.lo[c])*sc + int(lp.lo[m])*sm + int(lp.lo[y])*sy + int(lp.lo[k])*sk
		tetra(lp.data, base, sc, sm, sy, lp.fr[c], lp.fr[m], lp.fr[y], &o0)
		fk := lp.fr[k]
		if fk != 0 {
			tetra(lp.data, base+sk, sc, sm, sy, lp.fr[c], lp.fr[m], lp.fr[y], &o1)
			o0[0] += (o1[0] - o0[0]) * fk
			o0[1] += (o1[1] - o0[1]) * fk
			o0[2] += (o1[2] - o0[2]) * fk
		}
		d[i], d[i+1], d[i+2], d[i+3] = quant(o0[0], 0.5), quant(o0[1], 0.5), quant(o0[2], 0.5), 255
	}
}
