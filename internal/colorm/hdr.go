package colorm

import (
	"math"
)

const (
	hlgPeakNits = 1000.0
	hlgGamma    = 1.2
	hlgSteps    = 4096
)

func (c HDRConfig) ToneNits(l float64) float64 {
	c = c.withDefaults()
	if l <= 0 {
		return 0
	}
	pqS := pqInverse(c.SourcePeak / pqPeakNits)
	e := pqInverse(l / pqPeakNits)
	pqP := pqInverse(c.Peak / pqPeakNits)
	pqW := pqInverse(c.White / pqPeakNits)
	e1 := math.Min(e/pqS, 1)
	ks := math.Min(pqW/pqS, 1)
	maxL := math.Min(pqP/pqS, 1)
	if e1 <= ks {
		return l
	}
	if maxL <= ks || ks >= 1 {
		return pqEOTF(ks*pqS) * pqPeakNits
	}
	s := (maxL - ks) / (1 - ks)
	m0 := math.Min(1, 3*s)
	t := (e1 - ks) / (1 - ks)
	t2, t3 := t*t, t*t*t
	h00 := 2*t3 - 3*t2 + 1
	h10 := t3 - 2*t2 + t
	h01 := -2*t3 + 3*t2
	e2 := h00*ks + h10*(1-ks)*m0 + h01*maxL
	return pqEOTF(e2*pqS) * pqPeakNits
}

func (c HDRConfig) toneGain(l float64) float64 {
	if l <= 1e-9 {
		return 1
	}
	return c.ToneNits(l) / l
}

type hdrPath struct {
	kind   hdrKind
	cfg    HDRConfig
	lin    [3][lin16N + 1]float32
	gain   [lin16N + 1]float32
	ysPow  [hlgSteps + 1]float32
	hlgG   [hlgSteps + 1]float32
	m      [9]float32
	enc    *encoder
	luma   [3]float32
	ysCoef [3]float32
	white  float32
	exact  func(r, g, b float64) (float64, float64, float64)
}

func interp(t *[lin16N + 1]float32, v uint32) float32 {
	i := v >> 4
	f := float32(v&15) * (1.0 / 16)
	a := t[i]
	return a + (t[i+1]-a)*f
}

func interpSteps(t *[hlgSteps + 1]float32, u float32) float32 {
	p := u * hlgSteps
	if p < 0 {
		p = 0
	}
	i := int(p)
	if i >= hlgSteps {
		return t[hlgSteps]
	}
	f := p - float32(i)
	return t[i] + (t[i+1]-t[i])*f
}

func buildHDR(t *Transform) (*Transform, error) {
	src := t.src
	cfg := t.opt.HDR
	if src.cicp == nil {
		return nil, ErrUnsupported
	}
	ps, ok := cicpPrimaries[src.cicp.Primaries]
	if !ok {
		ps = cicpPrimaries[1]
	}
	raw, ok := PrimariesMatrix(ps.rx, ps.ry, ps.gx, ps.gy, ps.bx, ps.by, ps.wx, ps.wy)
	if !ok {
		return nil, ErrProfile
	}
	m := t.dstInv.Mul(src.colorants)
	if !m.finite() {
		return nil, ErrProfile
	}
	h := &hdrPath{kind: src.hdr, cfg: cfg, enc: newEncoder(t.dst), luma: t.dstLuma, white: float32(cfg.White)}
	for i, v := range m {
		h.m[i] = float32(v)
	}
	h.ysCoef = [3]float32{float32(raw[3]), float32(raw[4]), float32(raw[5])}
	for i := 0; i <= lin16N; i++ {
		code := math.Min(float64(i*16)/65535, 1)
		switch h.kind {
		case hdrPQ:
			nits := pqEOTF(code) * pqPeakNits
			for c := 0; c < 3; c++ {
				h.lin[c][i] = float32(nits / cfg.White)
			}
			h.gain[i] = float32(cfg.toneGain(nits))
		case hdrHLG:
			e := hlgOETFInverse(code)
			for c := 0; c < 3; c++ {
				h.lin[c][i] = float32(e)
			}
		}
	}
	if h.kind == hdrHLG {
		for i := 0; i <= hlgSteps; i++ {
			s := float64(i) / hlgSteps
			h.ysPow[i] = float32(math.Pow(s, 2*(hlgGamma-1)))
			h.hlgG[i] = float32(cfg.toneGain(s * s * hlgPeakNits))
		}
	}
	dstMat := t.dstMat
	t.pixelFn = func(r, g, b float64) (float64, float64, float64) {
		var lin Vec3
		switch src.hdr {
		case hdrPQ:
			nits := Vec3{pqEOTF(r) * pqPeakNits, pqEOTF(g) * pqPeakNits, pqEOTF(b) * pqPeakNits}
			gain := cfg.toneGain(math.Max(nits[0], math.Max(nits[1], nits[2])))
			lin = Vec3{nits[0] * gain / cfg.White, nits[1] * gain / cfg.White, nits[2] * gain / cfg.White}
		default:
			e := Vec3{hlgOETFInverse(r), hlgOETFInverse(g), hlgOETFInverse(b)}
			ys := raw[3]*e[0] + raw[4]*e[1] + raw[5]*e[2]
			k := hlgPeakNits * math.Pow(ys, hlgGamma-1)
			nits := Vec3{k * e[0], k * e[1], k * e[2]}
			gain := cfg.toneGain(math.Max(nits[0], math.Max(nits[1], nits[2])))
			lin = Vec3{nits[0] * gain / cfg.White, nits[1] * gain / cfg.White, nits[2] * gain / cfg.White}
		}
		out := softClip64(m.Apply(lin), dstMat)
		return t.dst.trc[0].Inverse(out[0]), t.dst.trc[1].Inverse(out[1]), t.dst.trc[2].Inverse(out[2])
	}
	t.hp = h
	t.mode = modeHDR
	return t, nil
}

func (h *hdrPath) rel16(r, g, b uint32) (float32, float32, float32) {
	if h.kind == hdrPQ {
		gain := interp(&h.gain, max(r, g, b))
		return interp(&h.lin[0], r) * gain, interp(&h.lin[1], g) * gain, interp(&h.lin[2], b) * gain
	}
	er, eg, eb := interp(&h.lin[0], r), interp(&h.lin[1], g), interp(&h.lin[2], b)
	ys := h.ysCoef[0]*er + h.ysCoef[1]*eg + h.ysCoef[2]*eb
	if ys < 0 {
		ys = 0
	}
	pw := interpSteps(&h.ysPow, float32(math.Sqrt(float64(ys))))
	k := hlgPeakNits * pw
	mn := k * max(er, eg, eb)
	u := float32(math.Sqrt(float64(mn / hlgPeakNits)))
	gain := interpSteps(&h.hlgG, u) * k / h.white
	return er * gain, eg * gain, eb * gain
}

func (h *hdrPath) row16(d, s []byte, y int, dither bool) {
	e0, e1, e2 := h.enc.t[0], h.enc.t[1], h.enc.t[2]
	m := h.m
	n := len(d) / 4
	for x := 0; x < n; x++ {
		so := x * 8
		r, g, b := h.rel16(uint32(s[so])<<8|uint32(s[so+1]), uint32(s[so+2])<<8|uint32(s[so+3]), uint32(s[so+4])<<8|uint32(s[so+5]))
		R := m[0]*r + m[1]*g + m[2]*b
		G := m[3]*r + m[4]*g + m[5]*b
		B := m[6]*r + m[7]*g + m[8]*b
		if R < 0 || R > 1 || G < 0 || G > 1 || B < 0 || B > 1 {
			R, G, B = softClip32(R, G, B, h.luma)
		}
		th := ditherAt(x, y, dither)
		do := x * 4
		d[do] = uint8((uint32(e0[h.enc.idx(R)]) + th) >> 8)
		d[do+1] = uint8((uint32(e1[h.enc.idx(G)]) + th) >> 8)
		d[do+2] = uint8((uint32(e2[h.enc.idx(B)]) + th) >> 8)
		a := uint32(s[so+6])<<8 | uint32(s[so+7])
		d[do+3] = uint8((a*255 + 32767) / 65535)
	}
}

func (h *hdrPath) row8(px []byte, tmp []byte) []byte {
	n := len(px) / 4
	if cap(tmp) < n*8 {
		tmp = make([]byte, n*8)
	}
	tmp = tmp[:n*8]
	for x := 0; x < n; x++ {
		for c := 0; c < 4; c++ {
			v := px[x*4+c]
			tmp[x*8+2*c], tmp[x*8+2*c+1] = v, v
		}
	}
	h.row16(px[:n*4], tmp, 0, false)
	return tmp
}
