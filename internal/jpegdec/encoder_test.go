package jpegdec

import (
	"bytes"
	"math"
)

type encComp struct {
	id   byte
	h, v int
	tq   int
}

type huffSpec struct {
	counts [16]int
	vals   []byte
}

type encSpec struct {
	width, height int
	comps         []encComp
	restart       int
	mixedCodes    bool
	extended      bool
	wideQuant     bool
	adobe         int
	jfif          bool
	separateScans bool
	seed          uint64
	quantScale    int
	whiteNoise    bool
	unitQuant     bool
	onesBlocks    bool
}

func flatDC() huffSpec {
	spec := huffSpec{}
	spec.counts[3] = 12
	for i := 0; i < 12; i++ {
		spec.vals = append(spec.vals, byte(i))
	}
	return spec
}

func acSymbols() []byte {
	symbols := []byte{0x00, 0xf0}
	for run := 0; run < 16; run++ {
		for size := 1; size <= 10; size++ {
			symbols = append(symbols, byte(run<<4|size))
		}
	}
	return symbols
}

func flatAC() huffSpec {
	symbols := acSymbols()
	spec := huffSpec{vals: symbols}
	spec.counts[7] = len(symbols)
	return spec
}

func mixedAC() huffSpec {
	symbols := acSymbols()
	front := []byte{0x00, 0x01, 0x02, 0x03, 0x11, 0x04, 0x05, 0x21, 0x12, 0x31}
	seen := map[byte]bool{}
	var ordered []byte
	for _, s := range front {
		ordered = append(ordered, s)
		seen[s] = true
	}
	for _, s := range symbols {
		if !seen[s] {
			ordered = append(ordered, s)
		}
	}
	spec := huffSpec{vals: ordered}
	spec.counts[1] = 2
	spec.counts[4] = 8
	spec.counts[15] = len(ordered) - 10
	return spec
}

type huffCode struct {
	code   uint32
	length int
}

func (s huffSpec) codes() [256]huffCode {
	var table [256]huffCode
	code := uint32(0)
	index := 0
	for length := 1; length <= 16; length++ {
		for i := 0; i < s.counts[length-1]; i++ {
			table[s.vals[index]] = huffCode{code: code, length: length}
			code++
			index++
		}
		code <<= 1
	}
	return table
}

type bitWriter struct {
	out  bytes.Buffer
	acc  uint32
	bits int
}

func (w *bitWriter) write(value uint32, count int) {
	for count > 0 {
		take := min(count, 8-w.bits)
		w.acc = w.acc<<uint(take) | (value>>uint(count-take))&(1<<uint(take)-1)
		w.bits += take
		count -= take
		if w.bits == 8 {
			w.out.WriteByte(byte(w.acc))
			if byte(w.acc) == 0xff {
				w.out.WriteByte(0)
			}
			w.acc, w.bits = 0, 0
		}
	}
}

func (w *bitWriter) flush() {
	if w.bits > 0 {
		pad := 8 - w.bits
		w.write(1<<uint(pad)-1, pad)
	}
}

func bitLength(v int) int {
	if v < 0 {
		v = -v
	}
	n := 0
	for v > 0 {
		n++
		v >>= 1
	}
	return n
}

var zigzagOrder = [64]int{
	0, 1, 8, 16, 9, 2, 3, 10,
	17, 24, 32, 25, 18, 11, 4, 5,
	12, 19, 26, 33, 40, 48, 41, 34,
	27, 20, 13, 6, 7, 14, 21, 28,
	35, 42, 49, 56, 57, 50, 43, 36,
	29, 22, 15, 23, 30, 37, 44, 51,
	58, 59, 52, 45, 38, 31, 39, 46,
	53, 60, 61, 54, 47, 55, 62, 63,
}

var dctBasis = func() (basis [8][8]float64) {
	for u := 0; u < 8; u++ {
		alpha := 0.5
		if u == 0 {
			alpha = math.Sqrt(0.125)
		}
		for x := 0; x < 8; x++ {
			basis[u][x] = alpha * math.Cos(float64(2*x+1)*float64(u)*math.Pi/16)
		}
	}
	return basis
}()

func forwardDCT(samples *[64]float64) [64]float64 {
	var tmp, out [64]float64
	basis := &dctBasis
	for y := 0; y < 8; y++ {
		for u := 0; u < 8; u++ {
			sum := 0.0
			for x := 0; x < 8; x++ {
				sum += samples[y*8+x] * basis[u][x]
			}
			tmp[y*8+u] = sum
		}
	}
	for u := 0; u < 8; u++ {
		for v := 0; v < 8; v++ {
			sum := 0.0
			for y := 0; y < 8; y++ {
				sum += tmp[y*8+u] * basis[v][y]
			}
			out[v*8+u] = sum
		}
	}
	return out
}

type encoder struct {
	spec     encSpec
	maxH     int
	maxV     int
	mxx, myy int
	planes   [][]float64
	pw, ph   []int
	quant    [4][64]int
	dc       [2]huffSpec
	ac       [2]huffSpec
	dcCodes  [2][256]huffCode
	acCodes  [2][256]huffCode
}

func (e *encoder) sample(c, x, y int) float64 {
	fx := float64(x) / 37
	fy := float64(y) / 29
	base := 128 + 70*math.Sin(fx+float64(c))*math.Cos(fy*0.7) + 25*math.Sin(fx*3.1+fy*2.3)
	state := e.spec.seed*0x9e3779b97f4a7c15 + uint64(c)*0x5851f42d4c957f2d + uint64(x)*0x2545f4914f6cdd1d + uint64(y)*0x9e3779b185ebca87 + 1
	state ^= state << 13
	state ^= state >> 7
	state ^= state << 17
	noise := float64(int(state>>58)) - 31.5
	if e.spec.whiteNoise {
		return float64(state >> 56)
	}
	v := base + noise
	if (x/23+y/17)%5 == 0 {
		v = 255 - v
	}
	return math.Max(0, math.Min(255, v))
}

func newEncoder(spec encSpec) *encoder {
	e := &encoder{spec: spec}
	for _, c := range spec.comps {
		e.maxH = max(e.maxH, c.h)
		e.maxV = max(e.maxV, c.v)
	}
	if len(spec.comps) == 1 {
		e.maxH, e.maxV = 1, 1
	}
	e.mxx = (spec.width + 8*e.maxH - 1) / (8 * e.maxH)
	e.myy = (spec.height + 8*e.maxV - 1) / (8 * e.maxV)
	scale := spec.quantScale
	if scale == 0 {
		scale = 1
	}
	for t := 0; t < 4; t++ {
		for i := 0; i < 64; i++ {
			e.quant[t][i] = 1 + ((i*(3+t))%17)*scale + i/8
			if spec.unitQuant {
				e.quant[t][i] = 1
			}
		}
	}
	e.dc = [2]huffSpec{flatDC(), flatDC()}
	if spec.mixedCodes {
		e.ac = [2]huffSpec{mixedAC(), flatAC()}
	} else {
		e.ac = [2]huffSpec{flatAC(), flatAC()}
	}
	for class := 0; class < 2; class++ {
		e.dcCodes[class] = e.dc[class].codes()
		e.acCodes[class] = e.ac[class].codes()
	}
	for ci, c := range spec.comps {
		h, v := c.h, c.v
		if len(spec.comps) == 1 {
			h, v = 1, 1
		}
		cw := (spec.width*h + e.maxH - 1) / e.maxH
		ch := (spec.height*v + e.maxV - 1) / e.maxV
		pw := e.mxx * h * 8
		ph := e.myy * v * 8
		plane := make([]float64, pw*ph)
		for y := 0; y < ph; y++ {
			for x := 0; x < pw; x++ {
				plane[y*pw+x] = e.sample(ci, min(x, cw-1), min(y, ch-1))
			}
		}
		e.planes = append(e.planes, plane)
		e.pw = append(e.pw, pw)
		e.ph = append(e.ph, ph)
	}
	return e
}

func (e *encoder) block(c, bx, by int) [64]int {
	if e.spec.onesBlocks {
		var q [64]int
		q[0] = (bx + 2*by + c) % 5
		for z := 1; z < 64; z++ {
			switch (z + bx + by) % 4 {
			case 0:
				q[z] = 255
			case 1:
				q[z] = 127
			case 2:
				q[z] = 1023
			}
		}
		return q
	}
	var samples [64]float64
	pw := e.pw[c]
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			samples[y*8+x] = e.planes[c][(by*8+y)*pw+bx*8+x] - 128
		}
	}
	coeffs := forwardDCT(&samples)
	var q [64]int
	table := &e.quant[e.spec.comps[c].tq]
	for z := 0; z < 64; z++ {
		n := zigzagOrder[z]
		q[z] = int(math.Round(coeffs[n] / float64(table[z])))
	}
	return q
}

func (e *encoder) encodeBlock(w *bitWriter, c int, q [64]int, pred *int) {
	class := 0
	if c > 0 {
		class = 1
	}
	if len(e.spec.comps) == 4 {
		class = 0
		if c == 1 || c == 2 {
			class = 1
		}
	}
	dcCodes := &e.dcCodes[class]
	acCodes := &e.acCodes[class]
	diff := q[0] - *pred
	*pred = q[0]
	size := bitLength(diff)
	size = min(size, 11)
	code := dcCodes[size]
	w.write(code.code, code.length)
	if size > 0 {
		bits := diff
		if diff < 0 {
			bits = diff - 1
		}
		w.write(uint32(bits)&(1<<uint(size)-1), size)
	}
	run := 0
	for z := 1; z < 64; z++ {
		v := q[z]
		if v == 0 {
			run++
			continue
		}
		for run > 15 {
			zrl := acCodes[0xf0]
			w.write(zrl.code, zrl.length)
			run -= 16
		}
		size := min(bitLength(v), 10)
		if v > 1<<uint(size)-1 {
			v = 1<<uint(size) - 1
		} else if v < -(1<<uint(size) - 1) {
			v = -(1<<uint(size) - 1)
		}
		sym := acCodes[byte(run<<4|size)]
		w.write(sym.code, sym.length)
		bits := v
		if v < 0 {
			bits = v - 1
		}
		w.write(uint32(bits)&(1<<uint(size)-1), size)
		run = 0
	}
	if run > 0 {
		eob := acCodes[0x00]
		w.write(eob.code, eob.length)
	}
}

func (e *encoder) restartMarker(w *bitWriter, count *int) {
	w.flush()
	w.out.WriteByte(0xff)
	w.out.WriteByte(byte(0xd0 + *count%8))
	*count++
}

func (e *encoder) scanInterleaved() []byte {
	w := &bitWriter{}
	pred := make([]int, len(e.spec.comps))
	restarts := 0
	mcu := 0
	total := e.mxx * e.myy
	for my := 0; my < e.myy; my++ {
		for mx := 0; mx < e.mxx; mx++ {
			for c, comp := range e.spec.comps {
				h, v := comp.h, comp.v
				if len(e.spec.comps) == 1 {
					h, v = 1, 1
				}
				for j := 0; j < v; j++ {
					for l := 0; l < h; l++ {
						e.encodeBlock(w, c, e.block(c, h*mx+l, v*my+j), &pred[c])
					}
				}
			}
			mcu++
			if e.spec.restart > 0 && mcu%e.spec.restart == 0 && mcu < total {
				e.restartMarker(w, &restarts)
				for i := range pred {
					pred[i] = 0
				}
			}
		}
	}
	w.flush()
	return w.out.Bytes()
}

func (e *encoder) scanComponent(c int) []byte {
	w := &bitWriter{}
	comp := e.spec.comps[c]
	h, v := comp.h, comp.v
	if len(e.spec.comps) == 1 {
		h, v = 1, 1
	}
	cw := (e.spec.width*h + e.maxH - 1) / e.maxH
	ch := (e.spec.height*v + e.maxV - 1) / e.maxV
	bw := (cw + 7) / 8
	bh := (ch + 7) / 8
	pred := 0
	restarts := 0
	count := 0
	for by := 0; by < bh; by++ {
		for bx := 0; bx < bw; bx++ {
			e.encodeBlock(w, c, e.block(c, bx, by), &pred)
			count++
			if e.spec.restart > 0 && count%e.spec.restart == 0 && count < bw*bh {
				e.restartMarker(w, &restarts)
				pred = 0
			}
		}
	}
	w.flush()
	return w.out.Bytes()
}

func segment(marker byte, payload []byte) []byte {
	length := len(payload) + 2
	out := []byte{0xff, marker, byte(length >> 8), byte(length)}
	return append(out, payload...)
}

func (e *encoder) dht(class, id int, spec huffSpec) []byte {
	payload := []byte{byte(class<<4 | id)}
	for _, c := range spec.counts {
		payload = append(payload, byte(c))
	}
	payload = append(payload, spec.vals...)
	return segment(0xc4, payload)
}

func (e *encoder) sosHeader(indices []int) []byte {
	payload := []byte{byte(len(indices))}
	for _, c := range indices {
		class := 0
		if c > 0 {
			class = 1
		}
		if len(e.spec.comps) == 4 {
			class = 0
			if c == 1 || c == 2 {
				class = 1
			}
		}
		payload = append(payload, e.spec.comps[c].id, byte(class<<4|class))
	}
	payload = append(payload, 0, 63, 0)
	return segment(0xda, payload)
}

func (e *encoder) bytes() []byte {
	var out bytes.Buffer
	out.Write([]byte{0xff, 0xd8})
	if e.spec.jfif {
		out.Write(segment(0xe0, []byte{'J', 'F', 'I', 'F', 0, 1, 1, 0, 0, 1, 0, 1, 0, 0}))
	}
	if e.spec.adobe > 0 {
		out.Write(segment(0xee, []byte{'A', 'd', 'o', 'b', 'e', 0, 100, 0, 0, 0, 0, byte(e.spec.adobe - 1)}))
	}
	for t := 0; t < 4; t++ {
		used := false
		for _, c := range e.spec.comps {
			if c.tq == t {
				used = true
			}
		}
		if !used {
			continue
		}
		if e.spec.wideQuant {
			payload := []byte{0x10 | byte(t)}
			for z := 0; z < 64; z++ {
				v := e.quant[t][z]
				payload = append(payload, byte(v>>8), byte(v))
			}
			out.Write(segment(0xdb, payload))
		} else {
			payload := []byte{byte(t)}
			for z := 0; z < 64; z++ {
				payload = append(payload, byte(e.quant[t][z]))
			}
			out.Write(segment(0xdb, payload))
		}
	}
	sof := byte(0xc0)
	if e.spec.extended {
		sof = 0xc1
	}
	payload := []byte{8, byte(e.spec.height >> 8), byte(e.spec.height), byte(e.spec.width >> 8), byte(e.spec.width), byte(len(e.spec.comps))}
	for _, c := range e.spec.comps {
		payload = append(payload, c.id, byte(c.h<<4|c.v), byte(c.tq))
	}
	out.Write(segment(sof, payload))
	for class := 0; class < 2; class++ {
		out.Write(e.dht(0, class, e.dc[class]))
		out.Write(e.dht(1, class, e.ac[class]))
	}
	if e.spec.restart > 0 {
		out.Write(segment(0xdd, []byte{byte(e.spec.restart >> 8), byte(e.spec.restart)}))
	}
	if e.spec.separateScans {
		for c := range e.spec.comps {
			out.Write(e.sosHeader([]int{c}))
			out.Write(e.scanComponent(c))
		}
	} else {
		indices := make([]int, len(e.spec.comps))
		for i := range indices {
			indices[i] = i
		}
		out.Write(e.sosHeader(indices))
		out.Write(e.scanInterleaved())
	}
	out.Write([]byte{0xff, 0xd9})
	return out.Bytes()
}

func encodeSpec(spec encSpec) []byte {
	return newEncoder(spec).bytes()
}

func ycbcrComps(h0, v0, h1, v1, h2, v2 int) []encComp {
	return []encComp{{1, h0, v0, 0}, {2, h1, v1, 1}, {3, h2, v2, 1}}
}

func standardSpecs() map[string]encSpec {
	specs := map[string]encSpec{}
	add := func(name string, spec encSpec) {
		if spec.seed == 0 {
			spec.seed = uint64(len(specs) + 1)
		}
		if len(spec.comps) == 3 && spec.adobe == 0 && spec.comps[0].id != 'R' {
			spec.jfif = true
		}
		specs[name] = spec
	}
	for _, size := range [][2]int{{1, 1}, {7, 9}, {8, 8}, {16, 16}, {17, 33}, {65, 31}, {150, 103}} {
		sz := itoa(size[0]) + "x" + itoa(size[1])
		w, h := size[0], size[1]
		add("gray-"+sz, encSpec{width: w, height: h, comps: []encComp{{1, 1, 1, 0}}})
		add("444-"+sz, encSpec{width: w, height: h, comps: ycbcrComps(1, 1, 1, 1, 1, 1)})
		add("420-"+sz, encSpec{width: w, height: h, comps: ycbcrComps(2, 2, 1, 1, 1, 1), mixedCodes: true})
		add("422-"+sz, encSpec{width: w, height: h, comps: ycbcrComps(2, 1, 1, 1, 1, 1)})
		add("440-"+sz, encSpec{width: w, height: h, comps: ycbcrComps(1, 2, 1, 1, 1, 1), mixedCodes: true})
		add("411-"+sz, encSpec{width: w, height: h, comps: ycbcrComps(4, 1, 1, 1, 1, 1)})
		add("410-"+sz, encSpec{width: w, height: h, comps: ycbcrComps(4, 2, 1, 1, 1, 1), mixedCodes: true})
		add("420-restart-"+sz, encSpec{width: w, height: h, comps: ycbcrComps(2, 2, 1, 1, 1, 1), restart: 3})
		add("444-restart1-"+sz, encSpec{width: w, height: h, comps: ycbcrComps(1, 1, 1, 1, 1, 1), restart: 1, mixedCodes: true})
	}
	add("noise-420", encSpec{width: 96, height: 80, comps: ycbcrComps(2, 2, 1, 1, 1, 1), whiteNoise: true, unitQuant: true})
	add("noise-444-restart", encSpec{width: 96, height: 80, comps: ycbcrComps(1, 1, 1, 1, 1, 1), whiteNoise: true, restart: 7})
	add("noise-gray", encSpec{width: 96, height: 80, comps: []encComp{{1, 1, 1, 0}}, whiteNoise: true, unitQuant: true})
	add("ones-420", encSpec{width: 96, height: 80, comps: ycbcrComps(2, 2, 1, 1, 1, 1), onesBlocks: true})
	add("ones-444-restart", encSpec{width: 96, height: 80, comps: ycbcrComps(1, 1, 1, 1, 1, 1), onesBlocks: true, restart: 5, mixedCodes: true})
	add("ones-gray", encSpec{width: 96, height: 80, comps: []encComp{{1, 1, 1, 0}}, onesBlocks: true})
	add("noise-422-mixed", encSpec{width: 96, height: 80, comps: ycbcrComps(2, 1, 1, 1, 1, 1), whiteNoise: true, unitQuant: true, mixedCodes: true})
	add("flex-cb-differs", encSpec{width: 70, height: 50, comps: ycbcrComps(2, 2, 1, 1, 2, 2)})
	add("flex-y-small", encSpec{width: 70, height: 50, comps: ycbcrComps(1, 2, 2, 1, 1, 1)})
	add("flex-1x4", encSpec{width: 70, height: 50, comps: ycbcrComps(1, 4, 1, 1, 1, 1)})
	add("flex-mixed", encSpec{width: 70, height: 50, comps: ycbcrComps(2, 1, 1, 2, 1, 1), mixedCodes: true})
	add("extended-wide-quant", encSpec{width: 61, height: 47, comps: ycbcrComps(2, 2, 1, 1, 1, 1), extended: true, wideQuant: true, quantScale: 4})
	add("rgb-adobe0", encSpec{width: 45, height: 38, comps: []encComp{{'R', 1, 1, 0}, {'G', 1, 1, 0}, {'B', 1, 1, 0}}, adobe: 1})
	add("rgb-ids", encSpec{width: 45, height: 38, comps: []encComp{{'R', 2, 2, 0}, {'G', 1, 1, 0}, {'B', 1, 1, 0}}})
	add("cmyk-adobe0", encSpec{width: 45, height: 38, comps: []encComp{{1, 1, 1, 0}, {2, 1, 1, 1}, {3, 1, 1, 1}, {4, 1, 1, 0}}, adobe: 1})
	add("cmyk-adobe0-2x2", encSpec{width: 45, height: 38, comps: []encComp{{1, 2, 2, 0}, {2, 1, 1, 1}, {3, 1, 1, 1}, {4, 2, 2, 0}}, adobe: 1})
	add("ycck", encSpec{width: 45, height: 38, comps: []encComp{{1, 1, 1, 0}, {2, 1, 1, 1}, {3, 1, 1, 1}, {4, 1, 1, 0}}, adobe: 3})
	add("ycck-2x2", encSpec{width: 45, height: 38, comps: []encComp{{1, 2, 2, 0}, {2, 1, 1, 1}, {3, 1, 1, 1}, {4, 2, 2, 0}}, adobe: 3, restart: 5})
	add("separate-444", encSpec{width: 45, height: 38, comps: ycbcrComps(1, 1, 1, 1, 1, 1), separateScans: true})
	add("separate-444-restart", encSpec{width: 45, height: 38, comps: ycbcrComps(1, 1, 1, 1, 1, 1), separateScans: true, restart: 4})
	add("separate-420", encSpec{width: 45, height: 38, comps: ycbcrComps(2, 2, 1, 1, 1, 1), separateScans: true})
	add("separate-gray", encSpec{width: 45, height: 38, comps: []encComp{{1, 1, 1, 0}}, separateScans: true})
	return specs
}

func syntheticFiles() map[string][]byte {
	files := map[string][]byte{}
	for name, spec := range standardSpecs() {
		files[name] = encodeSpec(spec)
	}
	return files
}

func tallJPEG(spec encSpec, rows int) []byte {
	e := newEncoder(spec)
	single := e.bytes()
	sos := bytes.Index(single, []byte{0xff, 0xda})
	sosLength := int(single[sos+2])<<8 | int(single[sos+3])
	scanStart := sos + 2 + sosLength
	scanEnd := len(single) - 2
	header := append([]byte(nil), single[:sos]...)
	sof := bytes.Index(header, []byte{0xff, 0xc0})
	if sof < 0 {
		sof = bytes.Index(header, []byte{0xff, 0xc1})
	}
	height := rows * 8 * e.maxV
	header[sof+5], header[sof+6] = byte(height>>8), byte(height)
	header = append(header, 0xff, 0xdd, 0, 4, byte(e.mxx>>8), byte(e.mxx))
	out := append(header, single[sos:scanStart]...)
	scan := single[scanStart:scanEnd]
	for row := 0; row < rows; row++ {
		out = append(out, scan...)
		if row < rows-1 {
			out = append(out, 0xff, byte(0xd0+row%8))
		}
	}
	return append(out, 0xff, 0xd9)
}
