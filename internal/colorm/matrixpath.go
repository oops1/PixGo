package colorm

import "math"

const (
	encShift = 14
	encBase  = 0x33800000
	encTop   = 0x3F800000
	encLen   = (encTop-encBase)>>encShift + 1
	lin16N   = 4096
)

type encoder struct {
	t [3][]uint16
}

func newEncoder(p *Profile) *encoder {
	e := &encoder{}
	build := func(c *Curve) []uint16 {
		t := make([]uint16, encLen)
		for i := range t {
			l := float64(math.Float32frombits(uint32(encBase + i<<encShift + 1<<(encShift-1))))
			if i == 0 {
				l = 0
			}
			t[i] = uint16(math.Round(c.Inverse(l) * 255 * 256))
		}
		return t
	}
	e.t[0] = build(p.trc[0])
	e.t[1], e.t[2] = e.t[0], e.t[0]
	if p.trc[1] != p.trc[0] {
		e.t[1] = build(p.trc[1])
	}
	if p.trc[2] != p.trc[0] {
		if p.trc[2] == p.trc[1] {
			e.t[2] = e.t[1]
		} else {
			e.t[2] = build(p.trc[2])
		}
	}
	return e
}

func (e *encoder) idx(v float32) int32 {
	sb := int32(math.Float32bits(v))
	if sb <= encBase {
		return 0
	}
	i := (sb - encBase) >> encShift
	if i >= encLen {
		return encLen - 1
	}
	return i
}

type lineariser struct {
	t8  [3][256]float32
	t16 [3][lin16N + 1]float32
}

func newLineariser(p *Profile) *lineariser {
	l := &lineariser{}
	for c := 0; c < 3; c++ {
		if c > 0 && p.trc[c] == p.trc[c-1] {
			l.t8[c], l.t16[c] = l.t8[c-1], l.t16[c-1]
			continue
		}
		for i := 0; i < 256; i++ {
			l.t8[c][i] = float32(p.trc[c].Eval(float64(i) / 255))
		}
		for i := 0; i <= lin16N; i++ {
			l.t16[c][i] = float32(p.trc[c].Eval(math.Min(float64(i*16)/65535, 1)))
		}
	}
	return l
}

func (l *lineariser) at16(c int, v uint32) float32 {
	i := v >> 4
	f := float32(v&15) * (1.0 / 16)
	t := &l.t16[c]
	a := t[i]
	return a + (t[i+1]-a)*f
}

type matrixPath struct {
	lin  *lineariser
	enc  *encoder
	m    [9]float32
	soft bool
	luma [3]float32
}

func buildMatrix(t *Transform) (*Transform, error) {
	src, dst := t.src, t.dst
	m := t.dstInv.Mul(src.colorants)
	if !m.finite() {
		return nil, ErrProfile
	}
	t.pixelFn = func(r, g, b float64) (float64, float64, float64) {
		lin := m.Apply(Vec3{src.trc[0].Eval(r), src.trc[1].Eval(g), src.trc[2].Eval(b)})
		if t.opt.SoftClip {
			lin = softClip64(lin, dst.colorants)
		}
		return dst.trc[0].Inverse(lin[0]), dst.trc[1].Inverse(lin[1]), dst.trc[2].Inverse(lin[2])
	}
	if !src.gray && m.near(identity3, 5e-4) && curvesIdentical8(src, dst) {
		t.mode = modeIdentity
		return t, nil
	}
	mp := &matrixPath{lin: newLineariser(src), enc: newEncoder(dst), soft: t.opt.SoftClip, luma: t.dstLuma}
	for i, v := range m {
		mp.m[i] = float32(v)
	}
	t.mp = mp
	t.mode = modeMatrix
	return t, nil
}

func curvesIdentical8(src, dst *Profile) bool {
	for c := 0; c < 3; c++ {
		for i := 0; i < 256; i++ {
			l := src.trc[c].Eval(float64(i) / 255)
			e := dst.trc[c].Inverse(l) * 255
			if math.Abs(e-float64(i)) > 0.3 {
				return false
			}
		}
	}
	return true
}

func softClip64(v Vec3, colorants Mat3) Vec3 {
	if v[0] >= 0 && v[0] <= 1 && v[1] >= 0 && v[1] <= 1 && v[2] >= 0 && v[2] <= 1 {
		return v
	}
	y := colorants[3]*v[0] + colorants[4]*v[1] + colorants[5]*v[2]
	if y >= 1 {
		return Vec3{1, 1, 1}
	}
	if y <= 0 {
		return Vec3{}
	}
	t := 1.0
	for _, c := range v {
		switch {
		case c > 1:
			t = math.Min(t, (1-y)/(c-y))
		case c < 0:
			t = math.Min(t, y/(y-c))
		}
	}
	return Vec3{y + t*(v[0]-y), y + t*(v[1]-y), y + t*(v[2]-y)}
}

func softClip32(r, g, b float32, l [3]float32) (float32, float32, float32) {
	y := l[0]*r + l[1]*g + l[2]*b
	if y >= 1 {
		return 1, 1, 1
	}
	if y <= 0 {
		return 0, 0, 0
	}
	t := float32(1)
	for _, c := range [3]float32{r, g, b} {
		switch {
		case c > 1:
			t = min(t, (1-y)/(c-y))
		case c < 0:
			t = min(t, y/(y-c))
		}
	}
	return y + t*(r-y), y + t*(g-y), y + t*(b-y)
}

func (p *matrixPath) row8(px []byte) {
	t0, t1, t2 := &p.lin.t8[0], &p.lin.t8[1], &p.lin.t8[2]
	e0, e1, e2 := p.enc.t[0], p.enc.t[1], p.enc.t[2]
	m := p.m
	for i := 0; i+3 < len(px); i += 4 {
		r, g, b := t0[px[i]], t1[px[i+1]], t2[px[i+2]]
		R := m[0]*r + m[1]*g + m[2]*b
		G := m[3]*r + m[4]*g + m[5]*b
		B := m[6]*r + m[7]*g + m[8]*b
		if p.soft && (R < 0 || R > 1 || G < 0 || G > 1 || B < 0 || B > 1) {
			R, G, B = softClip32(R, G, B, p.luma)
		}
		px[i] = uint8((e0[p.enc.idx(R)] + 128) >> 8)
		px[i+1] = uint8((e1[p.enc.idx(G)] + 128) >> 8)
		px[i+2] = uint8((e2[p.enc.idx(B)] + 128) >> 8)
	}
}

func (p *matrixPath) row16(d, s []byte, y int, dither bool) {
	e0, e1, e2 := p.enc.t[0], p.enc.t[1], p.enc.t[2]
	m := p.m
	n := len(d) / 4
	for x := 0; x < n; x++ {
		so := x * 8
		r := p.lin.at16(0, uint32(s[so])<<8|uint32(s[so+1]))
		g := p.lin.at16(1, uint32(s[so+2])<<8|uint32(s[so+3]))
		b := p.lin.at16(2, uint32(s[so+4])<<8|uint32(s[so+5]))
		R := m[0]*r + m[1]*g + m[2]*b
		G := m[3]*r + m[4]*g + m[5]*b
		B := m[6]*r + m[7]*g + m[8]*b
		if p.soft && (R < 0 || R > 1 || G < 0 || G > 1 || B < 0 || B > 1) {
			R, G, B = softClip32(R, G, B, p.luma)
		}
		th := ditherAt(x, y, dither)
		do := x * 4
		d[do] = uint8((uint32(e0[p.enc.idx(R)]) + th) >> 8)
		d[do+1] = uint8((uint32(e1[p.enc.idx(G)]) + th) >> 8)
		d[do+2] = uint8((uint32(e2[p.enc.idx(B)]) + th) >> 8)
		a := uint32(s[so+6])<<8 | uint32(s[so+7])
		d[do+3] = uint8((a*255 + 32767) / 65535)
	}
}
