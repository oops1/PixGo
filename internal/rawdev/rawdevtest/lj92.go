package rawdevtest

import (
	"math/bits"
	"sort"
)

type LJOptions struct {
	Predictor int
	Pt        int
	Restart   int
	Lengths   [17]int
}

var (
	LengthsShort = [17]int{5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5}
	LengthsLong  = [17]int{2, 2, 3, 3, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}
)

type bitWriter struct {
	out []byte
	acc uint32
	n   uint
}

func (w *bitWriter) put(v uint32, n uint) {
	for n > 0 {
		n--
		w.acc = w.acc<<1 | (v>>n)&1
		w.n++
		if w.n == 8 {
			w.flushByte()
		}
	}
}

func (w *bitWriter) flushByte() {
	b := byte(w.acc)
	w.out = append(w.out, b)
	if b == 0xff {
		w.out = append(w.out, 0)
	}
	w.acc, w.n = 0, 0
}

func (w *bitWriter) align() {
	for w.n != 0 {
		w.put(1, 1)
	}
}

func canonical(lengths [17]int) (codes [17]uint32, counts [16]uint8, order []uint8) {
	type sl struct{ sym, l int }
	var list []sl
	for s, l := range lengths {
		if l > 0 {
			list = append(list, sl{s, l})
			counts[l-1]++
		}
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].l != list[j].l {
			return list[i].l < list[j].l
		}
		return list[i].sym < list[j].sym
	})
	code := uint32(0)
	prev := 0
	for _, e := range list {
		code <<= uint(e.l - prev)
		prev = e.l
		codes[e.sym] = code
		code++
		order = append(order, uint8(e.sym))
	}
	return
}

func EncodeLJ92(w, h, comps, precision int, samples []uint16, o LJOptions) []byte {
	if o.Predictor == 0 {
		o.Predictor = 1
	}
	if o.Lengths[0] == 0 {
		o.Lengths = LengthsShort
	}
	codes, counts, order := canonical(o.Lengths)
	out := []byte{0xff, 0xd8}
	out = append(out, 0xff, 0xc4, byte((19+len(order))>>8), byte(19+len(order)), 0x00)
	for _, c := range counts {
		out = append(out, c)
	}
	out = append(out, order...)
	if o.Restart > 0 {
		out = append(out, 0xff, 0xdd, 0, 4, byte(o.Restart>>8), byte(o.Restart))
	}
	out = append(out, 0xff, 0xc3, byte((8+3*comps)>>8), byte(8+3*comps), byte(precision), byte(h>>8), byte(h), byte(w>>8), byte(w), byte(comps))
	for i := 0; i < comps; i++ {
		out = append(out, byte(i+1), 0x11, 0)
	}
	out = append(out, 0xff, 0xda, byte((6+2*comps)>>8), byte(6+2*comps), byte(comps))
	for i := 0; i < comps; i++ {
		out = append(out, byte(i+1), 0x00)
	}
	out = append(out, byte(o.Predictor), 0, byte(o.Pt))
	bw := &bitWriter{out: out}
	initial := int32(1) << uint(precision-o.Pt-1)
	left := o.Restart
	rst := 0
	fresh := true
	at := func(x, y, k int) int32 { return int32(samples[(y*w+x)*comps+k] >> uint(o.Pt)) }
	for y := 0; y < h; y++ {
		firstRow := y == 0 || fresh
		atStart := false
		for x := 0; x < w; x++ {
			if o.Restart > 0 && left == 0 {
				bw.align()
				bw.out = append(bw.out, 0xff, byte(0xd0+rst&7))
				rst++
				left = o.Restart
				firstRow, fresh, atStart = true, true, true
			}
			left--
			for k := 0; k < comps; k++ {
				var pred int32
				switch {
				case firstRow:
					if x == 0 || atStart {
						pred = initial
					} else {
						pred = at(x-1, y, k)
					}
				case x == 0:
					pred = at(0, y-1, k)
				default:
					ra, rb, rc := at(x-1, y, k), at(x, y-1, k), at(x-1, y-1, k)
					switch o.Predictor {
					case 1:
						pred = ra
					case 2:
						pred = rb
					case 3:
						pred = rc
					case 4:
						pred = ra + rb - rc
					case 5:
						pred = ra + (rb-rc)>>1
					case 6:
						pred = rb + (ra-rc)>>1
					default:
						pred = (ra + rb) >> 1
					}
				}
				diff := int16(uint16(at(x, y, k)) - uint16(pred))
				mag := int32(diff)
				if mag < 0 {
					mag = -mag
				}
				ssss := bits.Len32(uint32(mag))
				bw.put(codes[ssss], uint(o.Lengths[ssss]))
				if ssss > 0 && ssss < 16 {
					v := int32(diff)
					if v < 0 {
						v += 1<<uint(ssss) - 1
					}
					bw.put(uint32(v), uint(ssss))
				}
			}
			atStart = false
		}
		fresh = false
	}
	bw.align()
	return append(bw.out, 0xff, 0xd9)
}
