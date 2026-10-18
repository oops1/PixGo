package colorm

import (
	"encoding/binary"
	"errors"
	"math"
)

var (
	ErrProfile     = errors.New("colorm: invalid ICC profile")
	ErrUnsupported = errors.New("colorm: unsupported profile")
)

const (
	maxLUTInputs = 4
	maxLUTBytes  = 64 << 20
	maxCurvePts  = 1 << 20
)

type pcsEncoding uint8

const (
	pcsXYZ pcsEncoding = iota
	pcsLab
	pcsLabLegacy
)

const xyzEncodedMax = 1 + 32767.0/32768

func decodePCS(enc pcsEncoding, v [4]float64) Vec3 {
	switch enc {
	case pcsXYZ:
		return Vec3{v[0] * xyzEncodedMax, v[1] * xyzEncodedMax, v[2] * xyzEncodedMax}
	case pcsLabLegacy:
		return labToXYZ(v[0]*65535/65280*100, v[1]*65535/256-128, v[2]*65535/256-128)
	}
	return labToXYZ(v[0]*100, v[1]*255-128, v[2]*255-128)
}

func encodePCS(enc pcsEncoding, x Vec3) [4]float64 {
	switch enc {
	case pcsXYZ:
		return [4]float64{x[0] / xyzEncodedMax, x[1] / xyzEncodedMax, x[2] / xyzEncodedMax}
	}
	l, a, b := xyzToLab(x)
	if enc == pcsLabLegacy {
		return [4]float64{l / 100 * 65280 / 65535, (a + 128) * 256 / 65535, (b + 128) * 256 / 65535}
	}
	return [4]float64{l / 100, (a + 128) / 255, (b + 128) / 255}
}

type lutStage interface {
	apply(v [4]float64) [4]float64
}

type curveStage struct{ curves []*Curve }

func (s curveStage) apply(v [4]float64) [4]float64 {
	for i, c := range s.curves {
		if i < 4 {
			v[i] = c.Eval(v[i])
		}
	}
	return v
}

type matrixStage struct{ m [12]float64 }

func (s matrixStage) apply(v [4]float64) [4]float64 {
	m := s.m
	return [4]float64{
		m[0]*v[0] + m[1]*v[1] + m[2]*v[2] + m[9],
		m[3]*v[0] + m[4]*v[1] + m[5]*v[2] + m[10],
		m[6]*v[0] + m[7]*v[1] + m[8]*v[2] + m[11],
	}
}

type clutStage struct {
	in, out int
	grid    [maxLUTInputs]int
	data    []float32
}

func (s *clutStage) apply(v [4]float64) [4]float64 {
	var idx [maxLUTInputs]int
	var frac [maxLUTInputs]float64
	var stride [maxLUTInputs]int
	step := s.out
	for d := s.in - 1; d >= 0; d-- {
		stride[d] = step
		step *= s.grid[d]
	}
	for d := 0; d < s.in; d++ {
		g := s.grid[d]
		x := clamp01(v[d]) * float64(g-1)
		i := int(x)
		if i >= g-1 {
			i = g - 2
		}
		if i < 0 {
			i = 0
		}
		idx[d] = i
		frac[d] = x - float64(i)
		if g == 1 {
			idx[d], frac[d] = 0, 0
		}
	}
	var out [4]float64
	for corner := 0; corner < 1<<uint(s.in); corner++ {
		w := 1.0
		off := 0
		for d := 0; d < s.in; d++ {
			if corner>>uint(d)&1 == 1 {
				w *= frac[d]
				off += (idx[d] + 1) * stride[d]
			} else {
				w *= 1 - frac[d]
				off += idx[d] * stride[d]
			}
		}
		if w == 0 {
			continue
		}
		for o := 0; o < s.out && o < 4; o++ {
			out[o] += w * float64(s.data[off+o])
		}
	}
	return out
}

type lutPipe struct {
	in, out int
	stages  []lutStage
	enc     pcsEncoding
}

func (p *lutPipe) toPCS(dev [4]float64) Vec3 {
	v := dev
	for _, s := range p.stages {
		v = s.apply(v)
	}
	return decodePCS(p.enc, v)
}

func (p *lutPipe) fromPCS(x Vec3) [4]float64 {
	v := encodePCS(p.enc, x)
	for _, s := range p.stages {
		v = s.apply(v)
	}
	return v
}

func be16(b []byte, off int) uint16 { return binary.BigEndian.Uint16(b[off:]) }
func be32(b []byte, off int) uint32 { return binary.BigEndian.Uint32(b[off:]) }

func s15f16(b []byte, off int) float64 {
	return float64(int32(binary.BigEndian.Uint32(b[off:]))) / 65536
}

func parseCurveElement(b []byte) (*Curve, int, error) {
	if len(b) < 12 {
		return nil, 0, ErrProfile
	}
	switch string(b[:4]) {
	case "curv":
		n := int(be32(b, 8))
		if n < 0 || n > maxCurvePts || 12+2*n > len(b) {
			return nil, 0, ErrProfile
		}
		size := 12 + 2*n
		switch n {
		case 0:
			return identityCurve(), size, nil
		case 1:
			return gammaCurve(float64(be16(b, 12)) / 256), size, nil
		}
		t := make([]float64, n)
		for i := range t {
			t[i] = float64(be16(b, 12+2*i)) / 65535
		}
		return tableCurve(t), size, nil
	case "para":
		kind := int(be16(b, 8))
		counts := [...]int{1, 3, 4, 5, 7}
		if kind >= len(counts) {
			return nil, 0, ErrUnsupported
		}
		n := counts[kind]
		if 12+4*n > len(b) {
			return nil, 0, ErrProfile
		}
		p := make([]float64, 7)
		for i := 0; i < n; i++ {
			p[i] = s15f16(b, 12+4*i)
		}
		return paraCurve(kind, p), 12 + 4*n, nil
	}
	return nil, 0, ErrUnsupported
}

func parseCurveSet(b []byte, n int) ([]*Curve, error) {
	curves := make([]*Curve, 0, n)
	pos := 0
	for i := 0; i < n; i++ {
		if pos > len(b) {
			return nil, ErrProfile
		}
		c, size, err := parseCurveElement(b[pos:])
		if err != nil {
			return nil, err
		}
		curves = append(curves, c)
		pos += (size + 3) &^ 3
	}
	return curves, nil
}

func checkedProduct(vals ...int) (int, bool) {
	p := int64(1)
	for _, v := range vals {
		if v <= 0 {
			return 0, false
		}
		p *= int64(v)
		if p > maxLUTBytes {
			return 0, false
		}
	}
	return int(p), true
}

func parseLUT(b []byte, a2b bool, pcs string) (*lutPipe, error) {
	if len(b) < 32 {
		return nil, ErrProfile
	}
	enc := pcsXYZ
	if pcs == "Lab " {
		enc = pcsLab
	}
	switch string(b[:4]) {
	case "mft1", "mft2":
		return parseMFT(b, a2b, enc)
	case "mAB ":
		if !a2b {
			return nil, ErrProfile
		}
		return parseMAB(b, true, enc)
	case "mBA ":
		if a2b {
			return nil, ErrProfile
		}
		return parseMAB(b, false, enc)
	}
	return nil, ErrUnsupported
}

func parseMFT(b []byte, a2b bool, enc pcsEncoding) (*lutPipe, error) {
	wide := string(b[:4]) == "mft2"
	in, out, grid := int(b[8]), int(b[9]), int(b[10])
	if in < 1 || in > maxLUTInputs || out < 1 || out > maxLUTInputs || grid < 2 {
		return nil, ErrProfile
	}
	var inN, outN int
	pos := 48
	if wide {
		if len(b) < 52 {
			return nil, ErrProfile
		}
		inN, outN = int(be16(b, 48)), int(be16(b, 50))
		pos = 52
		if inN < 2 || inN > 4096 || outN < 2 || outN > 4096 {
			return nil, ErrProfile
		}
		if enc == pcsLab {
			enc = pcsLabLegacy
		}
	} else {
		inN, outN = 256, 256
	}
	width := 1
	if wide {
		width = 2
	}
	nodes := 1
	for i := 0; i < in; i++ {
		nodes *= grid
		if nodes > maxLUTBytes {
			return nil, ErrProfile
		}
	}
	clutN, ok := checkedProduct(nodes, out)
	if !ok {
		return nil, ErrProfile
	}
	need := pos + width*(in*inN+clutN+out*outN)
	if need > len(b) || need < pos {
		return nil, ErrProfile
	}
	read := func(off int) float64 {
		if wide {
			return float64(be16(b, off)) / 65535
		}
		return float64(b[off]) / 255
	}
	pipe := &lutPipe{in: in, out: out, enc: enc}
	if !a2b && enc == pcsXYZ && in == 3 {
		var m [12]float64
		for i := 0; i < 9; i++ {
			m[i] = s15f16(b, 12+4*i)
		}
		if m != [12]float64{1, 0, 0, 0, 1, 0, 0, 0, 1} {
			pipe.stages = append(pipe.stages, matrixStage{m: m})
		}
	}
	inCurves := make([]*Curve, in)
	for c := 0; c < in; c++ {
		t := make([]float64, inN)
		for i := range t {
			t[i] = read(pos + width*(c*inN+i))
		}
		inCurves[c] = tableCurve(t)
	}
	pos += width * in * inN
	data := make([]float32, clutN)
	for i := range data {
		data[i] = float32(read(pos + width*i))
	}
	pos += width * clutN
	outCurves := make([]*Curve, out)
	for c := 0; c < out; c++ {
		t := make([]float64, outN)
		for i := range t {
			t[i] = read(pos + width*(c*outN+i))
		}
		outCurves[c] = tableCurve(t)
	}
	cl := &clutStage{in: in, out: out, data: data}
	for i := 0; i < in; i++ {
		cl.grid[i] = grid
	}
	pipe.stages = append(pipe.stages, curveStage{inCurves}, cl, curveStage{outCurves})
	return pipe, nil
}

func parseMAB(b []byte, a2b bool, enc pcsEncoding) (*lutPipe, error) {
	in, out := int(b[8]), int(b[9])
	if in < 1 || in > maxLUTInputs || out < 1 || out > maxLUTInputs {
		return nil, ErrProfile
	}
	offB, offMat, offM, offClut, offA := int(be32(b, 12)), int(be32(b, 16)), int(be32(b, 20)), int(be32(b, 24)), int(be32(b, 28))
	for _, o := range [...]int{offB, offMat, offM, offClut, offA} {
		if o < 0 || o > len(b) || o != 0 && o < 32 {
			return nil, ErrProfile
		}
	}
	var bCurves, mCurves, aCurves []*Curve
	var err error
	parse := func(off, n int) ([]*Curve, error) {
		if off == 0 {
			return nil, nil
		}
		return parseCurveSet(b[off:], n)
	}
	pcsCh := 3
	if a2b && out != 3 || !a2b && in != 3 {
		return nil, ErrProfile
	}
	if bCurves, err = parse(offB, pcsCh); err != nil {
		return nil, err
	}
	if offM != 0 {
		if mCurves, err = parse(offM, pcsCh); err != nil {
			return nil, err
		}
	}
	var mat *matrixStage
	if offMat != 0 {
		if offMat+48 > len(b) {
			return nil, ErrProfile
		}
		var m [12]float64
		for i := range m {
			m[i] = s15f16(b, offMat+4*i)
		}
		mat = &matrixStage{m: m}
	}
	var cl *clutStage
	if offClut != 0 {
		if offClut+20 > len(b) {
			return nil, ErrProfile
		}
		cl = &clutStage{in: in, out: out}
		nodes := 1
		for d := 0; d < cl.in; d++ {
			g := int(b[offClut+d])
			if g < 1 {
				return nil, ErrProfile
			}
			cl.grid[d] = g
			nodes *= g
			if nodes > maxLUTBytes {
				return nil, ErrProfile
			}
		}
		prec := int(b[offClut+16])
		if prec != 1 && prec != 2 {
			return nil, ErrProfile
		}
		n, ok := checkedProduct(nodes, cl.out)
		if !ok || offClut+20+n*prec > len(b) {
			return nil, ErrProfile
		}
		cl.data = make([]float32, n)
		for i := range cl.data {
			if prec == 2 {
				cl.data[i] = float32(be16(b, offClut+20+2*i)) / 65535
			} else {
				cl.data[i] = float32(b[offClut+20+i]) / 255
			}
		}
		if offA != 0 {
			n := cl.in
			if !a2b {
				n = cl.out
			}
			aCurves, err = parse(offA, n)
			if err != nil {
				return nil, err
			}
		}
	} else if in != out {
		return nil, ErrProfile
	}
	pipe := &lutPipe{in: in, out: out, enc: enc}
	add := func(cs []*Curve) {
		if len(cs) > 0 {
			pipe.stages = append(pipe.stages, curveStage{cs})
		}
	}
	if a2b {
		add(aCurves)
		if cl != nil {
			pipe.stages = append(pipe.stages, cl)
		}
		add(mCurves)
		if mat != nil {
			pipe.stages = append(pipe.stages, *mat)
		}
		add(bCurves)
	} else {
		add(bCurves)
		if mat != nil {
			pipe.stages = append(pipe.stages, *mat)
		}
		add(mCurves)
		if cl != nil {
			pipe.stages = append(pipe.stages, cl)
		}
		add(aCurves)
	}
	for _, s := range pipe.stages {
		if m, ok := s.(matrixStage); ok {
			for _, v := range m.m {
				if math.IsNaN(v) || math.IsInf(v, 0) {
					return nil, ErrProfile
				}
			}
		}
	}
	return pipe, nil
}
