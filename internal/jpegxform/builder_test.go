package jpegxform

import (
	"bytes"
	"math/rand"
	"testing"
)

type tableStyle uint8

const (
	flatTables tableStyle = iota
	skewedTables
)

type buildComp struct {
	id   byte
	h, v int
	tq   int
}

type buildSpec struct {
	width, height int
	comps         []buildComp
	quant         [4][64]uint16
	wideQuant     bool
	restart       int
	segments      []segment
	style         tableStyle
	sof           byte
	scanOrder     []int
	dcTables      []int
	acTables      []int
}

func samplingSpec(w, h int, factors ...[2]int) buildSpec {
	s := buildSpec{width: w, height: h, sof: markerSOF0}
	for i, f := range factors {
		s.comps = append(s.comps, buildComp{id: byte(i + 1), h: f[0], v: f[1], tq: min(i, 1)})
	}
	if len(factors) == 4 {
		s.segments = append(s.segments, segment{marker: markerAPP0 + 14, payload: []byte{'A', 'd', 'o', 'b', 'e', 0, 100, 0, 0, 0, 0, 0}})
	}
	for t := 0; t < 4; t++ {
		for i := 0; i < 64; i++ {
			s.quant[t][i] = uint16(2 + t*3 + i%7 + i/16)
		}
	}
	return s
}

func (s buildSpec) hmaxVmax() (int, int) {
	if len(s.comps) == 1 {
		return 1, 1
	}
	hm, vm := 1, 1
	for _, c := range s.comps {
		hm, vm = max(hm, c.h), max(vm, c.v)
	}
	return hm, vm
}

func (s buildSpec) mcus() (int, int) {
	hm, vm := s.hmaxVmax()
	return (s.width + 8*hm - 1) / (8 * hm), (s.height + 8*vm - 1) / (8 * vm)
}

func (s buildSpec) factors(i int) (int, int) {
	if len(s.comps) == 1 {
		return 1, 1
	}
	return s.comps[i].h, s.comps[i].v
}

func newWall(s buildSpec) []grid {
	mx, my := s.mcus()
	grids := make([]grid, len(s.comps))
	for i := range s.comps {
		h, v := s.factors(i)
		g := grid{wide: mx * h, high: my * v}
		g.rows = make([][]int16, g.high)
		for y := range g.rows {
			g.rows[y] = make([]int16, g.wide*blockSize)
		}
		grids[i] = g
	}
	return grids
}

func randomWall(rng *rand.Rand, s buildSpec) []grid {
	grids := newWall(s)
	for _, g := range grids {
		for _, row := range g.rows {
			for b := 0; b < g.wide; b++ {
				fillRandomBlock(rng, row[b*blockSize:(b+1)*blockSize])
			}
		}
	}
	return grids
}

func fillRandomBlock(rng *rand.Rand, blk []int16) {
	blk[0] = int16(rng.Intn(2001) - 1000)
	density := rng.Float64() * 0.6
	for i := 1; i < blockSize; i++ {
		if rng.Float64() >= density {
			continue
		}
		bits := 1 + rng.Intn(10)
		v := rng.Intn(1<<bits-1) + 1
		if v > 1023 {
			v = 1023
		}
		if rng.Intn(2) == 0 {
			v = -v
		}
		blk[i] = int16(v)
	}
	if rng.Intn(8) == 0 {
		for i := 1; i < blockSize; i++ {
			blk[i] = 0
		}
	}
	if rng.Intn(8) == 0 {
		for i := 1; i < blockSize-1; i++ {
			blk[zigzag[i]] = 0
		}
		blk[zigzag[blockSize-1]] = int16(rng.Intn(100) + 1)
	}
}

type testTable struct {
	counts  [16]uint8
	symbols []byte
	code    [256]uint32
	size    [256]uint
}

func makeTable(counts [16]uint8, symbols []byte) *testTable {
	t := &testTable{counts: counts, symbols: symbols}
	code := uint32(0)
	k := 0
	for l := 1; l <= 16; l++ {
		for i := 0; i < int(counts[l-1]); i++ {
			t.code[symbols[k]] = code
			t.size[symbols[k]] = uint(l)
			code++
			k++
		}
		code <<= 1
	}
	return t
}

func dcTableFor(style tableStyle) *testTable {
	syms := make([]byte, 16)
	for i := range syms {
		syms[i] = byte(i)
	}
	var counts [16]uint8
	if style == flatTables {
		counts[4] = 16
	} else {
		counts[1] = 2
		counts[4] = 4
		counts[11] = 10
	}
	return makeTable(counts, syms)
}

func acTableFor(style tableStyle) *testTable {
	syms := make([]byte, 256)
	for i := range syms {
		syms[i] = byte(i)
	}
	var counts [16]uint8
	if style == flatTables {
		counts[8] = 255
		syms = syms[:255]
	} else {
		counts[1] = 2
		counts[4] = 4
		counts[11] = 240
		counts[15] = 9
		syms = syms[:255]
	}
	t := makeTable(counts, syms)
	return t
}

type testBits struct {
	out []byte
	acc uint64
	n   uint
}

func (b *testBits) put(v uint32, size uint) {
	for i := int(size) - 1; i >= 0; i-- {
		b.acc = b.acc<<1 | uint64(v>>uint(i)&1)
		b.n++
		if b.n == 8 {
			b.out = append(b.out, byte(b.acc))
			if byte(b.acc) == 0xFF {
				b.out = append(b.out, 0)
			}
			b.acc, b.n = 0, 0
		}
	}
}

func (b *testBits) align() {
	for b.n != 0 {
		b.put(1, 1)
	}
}

func bitSize(v int) uint {
	if v < 0 {
		v = -v
	}
	n := uint(0)
	for v > 0 {
		n++
		v >>= 1
	}
	return n
}

func putValue(b *testBits, v int, size uint) {
	if size == 0 {
		return
	}
	if v < 0 {
		v += 1<<size - 1
	}
	b.put(uint32(v), size)
}

func encodeTestBlock(b *testBits, blk []int16, pred int, dc, ac *testTable) int {
	diff := int(blk[0]) - pred
	s := bitSize(diff)
	b.put(dc.code[s], dc.size[s])
	putValue(b, diff, s)
	run := 0
	for k := 1; k < 64; k++ {
		v := int(blk[zigzag[k]])
		if v == 0 {
			run++
			continue
		}
		for run > 15 {
			b.put(ac.code[0xF0], ac.size[0xF0])
			run -= 16
		}
		s := bitSize(v)
		sym := run<<4 | int(s)
		b.put(ac.code[sym], ac.size[sym])
		putValue(b, v, s)
		run = 0
	}
	if run > 0 {
		b.put(ac.code[0], ac.size[0])
	}
	return int(blk[0])
}

func buildJPEG(t testing.TB, s buildSpec, grids []grid) []byte {
	t.Helper()
	var out bytes.Buffer
	out.Write([]byte{0xFF, markerSOI})
	for _, seg := range s.segments {
		out.Write(appendSegment(nil, seg.marker, seg.payload))
	}
	var dqt []byte
	for id := 0; id < 4; id++ {
		used := false
		for _, c := range s.comps {
			used = used || c.tq == id
		}
		if !used {
			continue
		}
		if s.wideQuant {
			dqt = append(dqt, 0x10|byte(id))
			for _, v := range s.quant[id] {
				dqt = append(dqt, byte(v>>8), byte(v))
			}
			continue
		}
		dqt = append(dqt, byte(id))
		for _, v := range s.quant[id] {
			dqt = append(dqt, byte(v))
		}
	}
	out.Write(appendSegment(nil, markerDQT, dqt))
	sof := s.sof
	if sof == 0 {
		sof = markerSOF0
	}
	frameHeader := []byte{8, byte(s.height >> 8), byte(s.height), byte(s.width >> 8), byte(s.width), byte(len(s.comps))}
	for _, c := range s.comps {
		frameHeader = append(frameHeader, c.id, byte(c.h<<4|c.v), byte(c.tq))
	}
	out.Write(appendSegment(nil, sof, frameHeader))
	dcIDs := s.dcTables
	if dcIDs == nil {
		dcIDs = make([]int, len(s.comps))
	}
	acIDs := s.acTables
	if acIDs == nil {
		acIDs = make([]int, len(s.comps))
	}
	dc, ac := dcTableFor(s.style), acTableFor(s.style)
	usedDC, usedAC := map[int]bool{}, map[int]bool{}
	for i := range s.comps {
		usedDC[dcIDs[i]], usedAC[acIDs[i]] = true, true
	}
	for id := 0; id < 4; id++ {
		if usedDC[id] {
			p := append([]byte{byte(id)}, dc.counts[:]...)
			out.Write(appendSegment(nil, markerDHT, append(p, dc.symbols...)))
		}
		if usedAC[id] {
			p := append([]byte{0x10 | byte(id)}, ac.counts[:]...)
			out.Write(appendSegment(nil, markerDHT, append(p, ac.symbols...)))
		}
	}
	if s.restart > 0 {
		out.Write(appendSegment(nil, markerDRI, []byte{byte(s.restart >> 8), byte(s.restart)}))
	}
	order := s.scanOrder
	if order == nil {
		for i := range s.comps {
			order = append(order, i)
		}
	}
	scanHeader := []byte{byte(len(order))}
	for _, ci := range order {
		scanHeader = append(scanHeader, s.comps[ci].id, byte(dcIDs[ci]<<4|acIDs[ci]))
	}
	scanHeader = append(scanHeader, 0, 63, 0)
	out.Write(appendSegment(nil, markerSOS, scanHeader))
	mx, my := s.mcus()
	b := &testBits{}
	preds := make([]int, len(s.comps))
	count, rst := 0, 0
	for y := 0; y < my; y++ {
		for x := 0; x < mx; x++ {
			if s.restart > 0 && count > 0 && count%s.restart == 0 {
				b.align()
				b.out = append(b.out, 0xFF, byte(markerRST0+rst%8))
				rst++
				for i := range preds {
					preds[i] = 0
				}
			}
			for _, ci := range order {
				h, v := s.factors(ci)
				for vv := 0; vv < v; vv++ {
					for hh := 0; hh < h; hh++ {
						blk := grids[ci].block(x*h+hh, y*v+vv)
						preds[ci] = encodeTestBlock(b, blk, preds[ci], dc, ac)
					}
				}
			}
			count++
		}
	}
	b.align()
	out.Write(b.out)
	out.Write([]byte{0xFF, markerEOI})
	return out.Bytes()
}

func gridsEqual(a, b []grid) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].wide != b[i].wide || a[i].high != b[i].high {
			return false
		}
		for y := range a[i].rows {
			if !equalBlocks(a[i].rows[y], b[i].rows[y]) {
				return false
			}
		}
	}
	return true
}

func equalBlocks(a, b []int16) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

var samplingCases = []struct {
	name    string
	factors [][2]int
}{
	{"gray", [][2]int{{1, 1}}},
	{"gray-declared-2x2", [][2]int{{2, 2}}},
	{"444", [][2]int{{1, 1}, {1, 1}, {1, 1}}},
	{"422", [][2]int{{2, 1}, {1, 1}, {1, 1}}},
	{"440", [][2]int{{1, 2}, {1, 1}, {1, 1}}},
	{"420", [][2]int{{2, 2}, {1, 1}, {1, 1}}},
	{"411", [][2]int{{4, 1}, {1, 1}, {1, 1}}},
	{"410", [][2]int{{4, 2}, {1, 1}, {1, 1}}},
	{"140", [][2]int{{1, 4}, {1, 1}, {1, 1}}},
	{"mixed-221122", [][2]int{{2, 2}, {1, 1}, {2, 2}}},
	{"mixed-222112", [][2]int{{2, 2}, {2, 1}, {1, 2}}},
	{"mixed-121121", [][2]int{{1, 2}, {1, 1}, {2, 1}}},
	{"cmyk", [][2]int{{1, 1}, {1, 1}, {1, 1}, {1, 1}}},
	{"cmyk-subsampled", [][2]int{{2, 2}, {1, 1}, {1, 1}, {2, 2}}},
}

func mildWall(rng *rand.Rand, s buildSpec) []grid {
	grids := newWall(s)
	for _, g := range grids {
		for _, row := range g.rows {
			for b := 0; b < g.wide; b++ {
				blk := row[b*blockSize : (b+1)*blockSize]
				blk[0] = int16(rng.Intn(61) - 30)
				for i := 1; i < 12; i++ {
					if rng.Intn(3) == 0 {
						blk[zigzag[i]] = int16(rng.Intn(5) - 2)
					}
				}
			}
		}
	}
	return grids
}
