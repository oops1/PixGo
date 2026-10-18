package rawdev

import (
	"context"
	"image"
	"math"
	"sync"
	"sync/atomic"

	"github.com/oops1/PixGo/internal/par"
)

const (
	bandRows  = 16
	toneSteps = 8192
	toneKnee  = 0.75
	maxEV     = 4
	maxScale  = 8
)

type Mosaic struct {
	Pix     []uint16
	Stride  int
	Samples int
	X, Y    int
	W, H    int
}

type Params struct {
	CFA      [4]uint8
	Black    [4]float64
	White    [4]float64
	DeltaH   []float64
	DeltaV   []float64
	Table    []uint16
	Crop     image.Rectangle
	Render   Rendering
	Exposure float64
}

func OutputSize(crop image.Rectangle, scale int) (int, int) {
	if scale < 1 {
		scale = 1
	}
	return (crop.Dx() + scale - 1) / scale, (crop.Dy() + scale - 1) / scale
}

func ValidScale(scale int) bool {
	return scale == 1 || scale == 2 || scale == 4 || scale == 8
}

func validCFA(cfa [4]uint8) bool {
	var count [3]int
	for _, c := range cfa {
		if c > 2 {
			return false
		}
		count[c]++
	}
	if count[0] != 1 || count[1] != 2 || count[2] != 1 {
		return false
	}
	return (cfa[0] == 1) != (cfa[1] == 1) && (cfa[0] == 1) == (cfa[3] == 1)
}

func ValidateCFA(cfa [4]uint8) bool { return validCFA(cfa) }

func (m Mosaic) validate() error {
	if m.Samples != 1 && m.Samples != 3 {
		return unsupported("%d samples per pixel", m.Samples)
	}
	if m.W < 1 || m.H < 1 || m.X < 0 || m.Y < 0 || m.Stride < (m.X+m.W)*m.Samples {
		return corrupt("mosaic geometry")
	}
	need := int64(m.Y+m.H-1)*int64(m.Stride) + int64(m.X+m.W)*int64(m.Samples)
	if need > int64(len(m.Pix)) {
		return corrupt("mosaic of %d samples is smaller than %d", len(m.Pix), need)
	}
	return nil
}

type workspace struct {
	rows [bandRows + 4][]float32
	rgb  []float32
	acc  []float32
	line []float32
}

type engine struct {
	m        Mosaic
	p        Params
	scale    int
	cx, cy   int
	cw, ch   int
	ow, oh   int
	luts     [4][]float32
	colour   [4]int
	kind     [4]uint8
	mat      [9]float32
	tone     []uint16
	toneMul  float32
	toneMax  float32
	out8     *image.NRGBA
	out16    *image.NRGBA64
	pool     sync.Pool
	xs       []int32
	hasDelta bool
}

const (
	kindRed uint8 = iota
	kindBlue
	kindGreenInRedRow
	kindGreenInBlueRow
)

func mirror(i, n int) int {
	if n <= 1 {
		return 0
	}
	for i < 0 || i >= n {
		if i < 0 {
			i = -i
		}
		if i >= n {
			i = 2*(n-1) - i
		}
	}
	return i
}

func srgbEncode(x float64) float64 {
	if x <= 0 {
		return 0
	}
	if x >= 1 {
		return 1
	}
	if x <= 0.0031308 {
		return 12.92 * x
	}
	return 1.055*math.Pow(x, 1/2.4) - 0.055
}

func toneShoulder(x, white float64) float64 {
	if x <= toneKnee {
		return x
	}
	u := (x - toneKnee) / (1 - toneKnee)
	lw := (white - toneKnee) / (1 - toneKnee)
	g := u * (1 + u/(lw*lw)) / (1 + u)
	return math.Min(1, toneKnee+(1-toneKnee)*g)
}

func ToneCurve(x, ev float64) float64 {
	ev = math.Min(math.Max(ev, -maxEV), maxEV)
	white := math.Max(1, math.Exp2(ev))
	return srgbEncode(toneShoulder(math.Max(x, 0), white))
}

func newEngine(m Mosaic, p Params, scale int, deep bool) (*engine, error) {
	if err := m.validate(); err != nil {
		return nil, err
	}
	if !ValidScale(scale) {
		return nil, unsupported("scale %d", scale)
	}
	if m.Samples == 1 && !validCFA(p.CFA) {
		return nil, unsupported("color filter array %v", p.CFA)
	}
	crop := p.Crop
	if crop.Min.X < 0 || crop.Min.Y < 0 || crop.Max.X > m.W || crop.Max.Y > m.H || crop.Empty() {
		return nil, corrupt("crop %v outside the %dx%d active area", crop, m.W, m.H)
	}
	if len(p.Table) > 1<<16 {
		return nil, corrupt("linearization table of %d entries", len(p.Table))
	}
	for i := range p.Render.Matrix {
		if math.IsNaN(p.Render.Matrix[i]) || math.IsInf(p.Render.Matrix[i], 0) {
			return nil, corrupt("rendering matrix")
		}
	}
	for i := range p.Render.Gain {
		if !(p.Render.Gain[i] > 0) || math.IsInf(p.Render.Gain[i], 0) {
			return nil, corrupt("white balance gain")
		}
	}
	e := &engine{m: m, p: p, scale: scale, cx: crop.Min.X, cy: crop.Min.Y, cw: crop.Dx(), ch: crop.Dy()}
	e.ow, e.oh = OutputSize(crop, scale)
	positions := 4
	if m.Samples == 3 {
		positions = 3
	}
	for pos := 0; pos < positions; pos++ {
		colour := pos
		if m.Samples == 1 {
			colour = int(p.CFA[pos])
		}
		e.colour[pos] = colour
		span := p.White[pos] - p.Black[pos]
		if !(span > 0) || math.IsInf(span, 0) {
			return nil, corrupt("white level %v is not above black level %v", p.White[pos], p.Black[pos])
		}
		lut := make([]float32, 1<<16)
		gain := p.Render.Gain[colour]
		for v := range lut {
			lin := float64(v)
			if len(p.Table) > 0 {
				lin = float64(p.Table[min(v, len(p.Table)-1)])
			}
			n := (lin - p.Black[pos]) / span * gain
			lut[v] = float32(math.Min(1, math.Max(0, n)))
		}
		e.luts[pos] = lut
	}
	if m.Samples == 1 {
		for pos := 0; pos < 4; pos++ {
			switch p.CFA[pos] {
			case 0:
				e.kind[pos] = kindRed
			case 2:
				e.kind[pos] = kindBlue
			default:
				if p.CFA[pos^1] == 0 {
					e.kind[pos] = kindGreenInRedRow
				} else {
					e.kind[pos] = kindGreenInBlueRow
				}
			}
		}
		e.hasDelta = len(p.DeltaH) > 0 || len(p.DeltaV) > 0
	}
	ev := math.Min(math.Max(p.Exposure, -maxEV), maxEV)
	if math.IsNaN(ev) {
		ev = 0
	}
	gain := math.Exp2(ev)
	for i := range e.mat {
		e.mat[i] = float32(p.Render.Matrix[i] * gain)
	}
	white := math.Max(1, gain)
	e.toneMax = float32(white)
	e.toneMul = float32(toneSteps / white)
	e.tone = make([]uint16, toneSteps+2)
	for i := 0; i <= toneSteps; i++ {
		x := float64(i) / toneSteps * white
		e.tone[i] = uint16(math.Round(srgbEncode(toneShoulder(x, white)) * 65535))
	}
	e.tone[toneSteps+1] = e.tone[toneSteps]
	if scale > 1 {
		e.xs = make([]int32, e.ow*scale)
		for k := range e.xs {
			e.xs[k] = int32(mirror(e.cx+k, m.W))
		}
	}
	if deep {
		e.out16 = image.NewNRGBA64(image.Rect(0, 0, e.ow, e.oh))
	} else {
		e.out8 = image.NewNRGBA(image.Rect(0, 0, e.ow, e.oh))
	}
	e.pool.New = func() any {
		ws := &workspace{rgb: make([]float32, 3*e.ow)}
		if scale == 1 && m.Samples == 1 {
			for i := range ws.rows {
				ws.rows[i] = make([]float32, e.cw+4)
			}
		}
		if scale > 1 {
			ws.acc = make([]float32, 3*e.ow)
			ws.line = make([]float32, 3*e.ow*scale)
		}
		return ws
	}
	return e, nil
}

func Develop(ctx context.Context, m Mosaic, p Params, scale int, deep bool) (*image.NRGBA, *image.NRGBA64, error) {
	e, err := newEngine(m, p, scale, deep)
	if err != nil {
		return nil, nil, err
	}
	tasks := (e.oh + bandRows - 1) / bandRows
	var failed atomic.Pointer[error]
	par.Parallel(tasks, par.Count(), func(t int) {
		if failed.Load() != nil {
			return
		}
		if err := ctx.Err(); err != nil {
			failed.CompareAndSwap(nil, &err)
			return
		}
		ws := e.pool.Get().(*workspace)
		e.band(ws, t*bandRows, min((t+1)*bandRows, e.oh))
		e.pool.Put(ws)
	})
	if err := failed.Load(); err != nil {
		return nil, nil, *err
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	return e.out8, e.out16, nil
}

func (e *engine) band(ws *workspace, oy0, oy1 int) {
	switch {
	case e.m.Samples == 3 && e.scale == 1:
		e.linearBand(ws, oy0, oy1)
	case e.m.Samples == 3:
		e.binnedLinearBand(ws, oy0, oy1)
	case e.scale == 1:
		e.bayerBand(ws, oy0, oy1)
	default:
		e.binnedBayerBand(ws, oy0, oy1)
	}
}

func (e *engine) sample(my, ax int) float32 {
	v := int(e.m.Pix[(e.m.Y+my)*e.m.Stride+e.m.X+ax])
	if e.hasDelta {
		var d float64
		if ax < len(e.p.DeltaH) {
			d += e.p.DeltaH[ax]
		}
		if my < len(e.p.DeltaV) {
			d += e.p.DeltaV[my]
		}
		v = min(max(v-int(math.Round(d)), 0), 65535)
	}
	return e.luts[(my&1)<<1|ax&1][v]
}

func (e *engine) convertRow(dst []float32, my, ax0 int) {
	n := len(dst)
	base := (e.m.Y+my)*e.m.Stride + e.m.X
	src := e.m.Pix[base : base+e.m.W]
	lo := min(max(-ax0, 0), n)
	hi := max(min(e.m.W-ax0, n), lo)
	if e.hasDelta {
		for x := 0; x < n; x++ {
			dst[x] = e.sample(my, mirror(ax0+x, e.m.W))
		}
		return
	}
	for x := 0; x < lo; x++ {
		dst[x] = e.sample(my, mirror(ax0+x, e.m.W))
	}
	py := (my & 1) << 1
	luts := [2][]float32{e.luts[py], e.luts[py|1]}
	for x := lo; x < hi; x++ {
		ax := ax0 + x
		dst[x] = luts[ax&1][src[ax]]
	}
	for x := hi; x < n; x++ {
		dst[x] = e.sample(my, mirror(ax0+x, e.m.W))
	}
}

func (e *engine) bayerBand(ws *workspace, oy0, oy1 int) {
	for r := 0; r < oy1-oy0+4; r++ {
		sy := e.cy + oy0 - 2 + r
		e.convertRow(ws.rows[r], mirror(sy, e.m.H), e.cx-2)
	}
	for oy := oy0; oy < oy1; oy++ {
		b := oy - oy0
		e.demosaicRow(ws.rgb, ws.rows[b], ws.rows[b+1], ws.rows[b+2], ws.rows[b+3], ws.rows[b+4], (e.cy+oy)&1, e.cx&1)
		e.finish(ws.rgb, oy)
	}
}

func (e *engine) demosaicRow(out []float32, r0, r1, r2, r3, r4 []float32, py, px0 int) {
	k0 := e.kind[py<<1|px0]
	k1 := e.kind[py<<1|(px0^1)]
	kinds := [2]uint8{k0, k1}
	n := e.cw
	r0, r1, r2, r3, r4 = r0[:n+4], r1[:n+4], r2[:n+4], r3[:n+4], r4[:n+4]
	out = out[:3*n]
	for j := 0; j < n; j++ {
		i := j + 2
		c := r2[i]
		var r, g, b float32
		switch kinds[j&1] {
		case kindRed:
			ring := r2[i-2] + r2[i+2] + r0[i] + r4[i]
			g = (4*c + 2*(r2[i-1]+r2[i+1]+r1[i]+r3[i]) - ring) * 0.125
			b = (6*c + 2*(r1[i-1]+r1[i+1]+r3[i-1]+r3[i+1]) - 1.5*ring) * 0.125
			r = c
		case kindBlue:
			ring := r2[i-2] + r2[i+2] + r0[i] + r4[i]
			g = (4*c + 2*(r2[i-1]+r2[i+1]+r1[i]+r3[i]) - ring) * 0.125
			r = (6*c + 2*(r1[i-1]+r1[i+1]+r3[i-1]+r3[i+1]) - 1.5*ring) * 0.125
			b = c
		case kindGreenInRedRow:
			diag := r1[i-1] + r1[i+1] + r3[i-1] + r3[i+1]
			r = (5*c + 4*(r2[i-1]+r2[i+1]) - (r2[i-2] + r2[i+2]) - diag + 0.5*(r0[i]+r4[i])) * 0.125
			b = (5*c + 4*(r1[i]+r3[i]) - (r0[i] + r4[i]) - diag + 0.5*(r2[i-2]+r2[i+2])) * 0.125
			g = c
		default:
			diag := r1[i-1] + r1[i+1] + r3[i-1] + r3[i+1]
			b = (5*c + 4*(r2[i-1]+r2[i+1]) - (r2[i-2] + r2[i+2]) - diag + 0.5*(r0[i]+r4[i])) * 0.125
			r = (5*c + 4*(r1[i]+r3[i]) - (r0[i] + r4[i]) - diag + 0.5*(r2[i-2]+r2[i+2])) * 0.125
			g = c
		}
		out[3*j], out[3*j+1], out[3*j+2] = r, g, b
	}
}

func (e *engine) binnedBayerBand(ws *workspace, oy0, oy1 int) {
	s := e.scale
	half := float32(s / 2)
	inv := [3]float32{1 / (half * half), 1 / (2 * half * half), 1 / (half * half)}
	for oy := oy0; oy < oy1; oy++ {
		acc := ws.acc
		for i := range acc {
			acc[i] = 0
		}
		for j := 0; j < s; j++ {
			my := mirror(e.cy+oy*s+j, e.m.H)
			line := ws.line[:e.ow*s]
			py := (my & 1) << 1
			base := (e.m.Y+my)*e.m.Stride + e.m.X
			src := e.m.Pix[base : base+e.m.W]
			if e.hasDelta {
				for k, sx := range e.xs {
					line[k] = e.sample(my, int(sx))
				}
			} else {
				for k, sx := range e.xs {
					line[k] = e.luts[py|int(sx&1)][src[sx]]
				}
			}
			for ox := 0; ox < e.ow; ox++ {
				var sum [3]float32
				for i := 0; i < s; i++ {
					k := ox*s + i
					sum[e.colour[py|int(e.xs[k]&1)]] += line[k]
				}
				acc[3*ox] += sum[0]
				acc[3*ox+1] += sum[1]
				acc[3*ox+2] += sum[2]
			}
		}
		for ox := 0; ox < e.ow; ox++ {
			ws.rgb[3*ox] = acc[3*ox] * inv[0]
			ws.rgb[3*ox+1] = acc[3*ox+1] * inv[1]
			ws.rgb[3*ox+2] = acc[3*ox+2] * inv[2]
		}
		e.finish(ws.rgb, oy)
	}
}

func (e *engine) linearBand(ws *workspace, oy0, oy1 int) {
	for oy := oy0; oy < oy1; oy++ {
		base := (e.m.Y+e.cy+oy)*e.m.Stride + (e.m.X+e.cx)*3
		src := e.m.Pix[base : base+3*e.cw]
		l0, l1, l2 := e.luts[0], e.luts[1], e.luts[2]
		for x := 0; x < e.cw; x++ {
			ws.rgb[3*x] = l0[src[3*x]]
			ws.rgb[3*x+1] = l1[src[3*x+1]]
			ws.rgb[3*x+2] = l2[src[3*x+2]]
		}
		e.finish(ws.rgb, oy)
	}
}

func (e *engine) binnedLinearBand(ws *workspace, oy0, oy1 int) {
	s := e.scale
	inv := 1 / float32(s*s)
	for oy := oy0; oy < oy1; oy++ {
		acc := ws.acc
		for i := range acc {
			acc[i] = 0
		}
		for j := 0; j < s; j++ {
			my := mirror(e.cy+oy*s+j, e.m.H)
			base := (e.m.Y+my)*e.m.Stride + e.m.X*3
			src := e.m.Pix[base : base+3*e.m.W]
			for ox := 0; ox < e.ow; ox++ {
				var a0, a1, a2 float32
				for i := 0; i < s; i++ {
					sx := int(e.xs[ox*s+i])
					a0 += e.luts[0][src[3*sx]]
					a1 += e.luts[1][src[3*sx+1]]
					a2 += e.luts[2][src[3*sx+2]]
				}
				acc[3*ox] += a0
				acc[3*ox+1] += a1
				acc[3*ox+2] += a2
			}
		}
		for i := range acc {
			ws.rgb[i] = acc[i] * inv
		}
		e.finish(ws.rgb, oy)
	}
}

func (e *engine) finish(rgb []float32, oy int) {
	m := e.mat
	top := e.toneMax
	mul := e.toneMul
	tone := e.tone
	var px [3]uint16
	var o8 []uint8
	var o16 []uint8
	if e.out8 != nil {
		o8 = e.out8.Pix[oy*e.out8.Stride : oy*e.out8.Stride+4*e.ow]
	} else {
		o16 = e.out16.Pix[oy*e.out16.Stride : oy*e.out16.Stride+8*e.ow]
	}
	for x := 0; x < e.ow; x++ {
		r, g, b := rgb[3*x], rgb[3*x+1], rgb[3*x+2]
		if !(r > 0) {
			r = 0
		} else if r > 1 {
			r = 1
		}
		if !(g > 0) {
			g = 0
		} else if g > 1 {
			g = 1
		}
		if !(b > 0) {
			b = 0
		} else if b > 1 {
			b = 1
		}
		v := [3]float32{
			m[0]*r + m[1]*g + m[2]*b,
			m[3]*r + m[4]*g + m[5]*b,
			m[6]*r + m[7]*g + m[8]*b,
		}
		for c := 0; c < 3; c++ {
			t := v[c]
			if !(t > 0) {
				t = 0
			} else if t > top {
				t = top
			}
			f := t * mul
			i := int(f)
			lo := float32(tone[i])
			px[c] = uint16(lo + (float32(tone[i+1])-lo)*(f-float32(i)) + 0.5)
		}
		if o8 != nil {
			o8[4*x] = uint8((uint32(px[0]) + 128) / 257)
			o8[4*x+1] = uint8((uint32(px[1]) + 128) / 257)
			o8[4*x+2] = uint8((uint32(px[2]) + 128) / 257)
			o8[4*x+3] = 255
		} else {
			o16[8*x], o16[8*x+1] = uint8(px[0]>>8), uint8(px[0])
			o16[8*x+2], o16[8*x+3] = uint8(px[1]>>8), uint8(px[1])
			o16[8*x+4], o16[8*x+5] = uint8(px[2]>>8), uint8(px[2])
			o16[8*x+6], o16[8*x+7] = 255, 255
		}
	}
}
