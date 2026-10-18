package rawdev

import (
	"context"
	"errors"
	"fmt"
)

var (
	ErrCorrupt     = errors.New("rawdev: corrupt or truncated data")
	ErrUnsupported = errors.New("rawdev: unsupported variant")
)

func corrupt(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrCorrupt, fmt.Sprintf(format, args...))
}

func unsupported(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrUnsupported, fmt.Sprintf(format, args...))
}

const (
	fastBits      = 10
	maxHuffTables = 4
	rowsPerCheck  = 8
)

type huffTable struct {
	valid   bool
	fast    [1 << fastBits]uint16
	maxcode [18]int32
	mincode [17]int32
	valptr  [17]int32
	vals    [256]uint8
}

func (h *huffTable) build(counts [16]uint8, vals []uint8) error {
	total := 0
	for _, c := range counts {
		total += int(c)
	}
	if total > 256 || total != len(vals) {
		return corrupt("lj92: huffman table size")
	}
	copy(h.vals[:], vals)
	h.fast = [1 << fastBits]uint16{}
	code := int32(0)
	k := int32(0)
	for l := 1; l <= 16; l++ {
		n := int32(counts[l-1])
		h.valptr[l] = k
		h.mincode[l] = code
		if n == 0 {
			h.maxcode[l] = -1
		} else {
			if code+n > 1<<uint(l) {
				return corrupt("lj92: oversubscribed huffman table")
			}
			if l <= fastBits {
				for i := int32(0); i < n; i++ {
					c := code + i
					start := int(c) << uint(fastBits-l)
					end := start + 1<<uint(fastBits-l)
					entry := uint16(l)<<8 | uint16(vals[k+i])
					for j := start; j < end; j++ {
						h.fast[j] = entry
					}
				}
			}
			code += n
			k += n
			h.maxcode[l] = code - 1
		}
		code <<= 1
	}
	h.maxcode[17] = 1<<30 - 1
	h.valid = true
	return nil
}

type LJ92Info struct {
	Width      int
	Height     int
	Components int
	Precision  int
}

type LJ92 struct {
	Info      LJ92Info
	data      []byte
	tables    [maxHuffTables]huffTable
	tableOf   [4]int
	predictor int
	pointXf   int
	restart   int
	scanStart int
}

func u16(b []byte) int { return int(b[0])<<8 | int(b[1]) }

func ParseLJ92(data []byte) (*LJ92, error) {
	if len(data) < 4 || data[0] != 0xff || data[1] != 0xd8 {
		return nil, corrupt("lj92: missing SOI")
	}
	d := &LJ92{data: data}
	pos := 2
	haveFrame := false
	for {
		for pos < len(data) && data[pos] == 0xff && pos+1 < len(data) && data[pos+1] == 0xff {
			pos++
		}
		if pos+4 > len(data) || data[pos] != 0xff {
			return nil, corrupt("lj92: marker expected at %d", pos)
		}
		marker := data[pos+1]
		length := u16(data[pos+2:])
		if length < 2 || pos+2+length > len(data) {
			return nil, corrupt("lj92: segment of %d bytes at %d", length, pos)
		}
		body := data[pos+4 : pos+2+length]
		switch {
		case marker == 0xc3:
			if haveFrame {
				return nil, corrupt("lj92: second frame header")
			}
			if err := d.frame(body); err != nil {
				return nil, err
			}
			haveFrame = true
		case marker >= 0xc0 && marker <= 0xcf && marker != 0xc4 && marker != 0xc8 && marker != 0xcc:
			return nil, unsupported("lj92: JPEG process %#x", marker)
		case marker == 0xc4:
			if err := d.huffman(body); err != nil {
				return nil, err
			}
		case marker == 0xdd:
			if len(body) != 2 {
				return nil, corrupt("lj92: restart interval")
			}
			d.restart = u16(body)
		case marker == 0xda:
			if !haveFrame {
				return nil, corrupt("lj92: scan before frame")
			}
			if err := d.scan(body); err != nil {
				return nil, err
			}
			d.scanStart = pos + 2 + length
			return d, nil
		case marker == 0xd9:
			return nil, corrupt("lj92: no scan")
		}
		pos += 2 + length
	}
}

func (d *LJ92) frame(b []byte) error {
	if len(b) < 6 {
		return corrupt("lj92: frame header")
	}
	precision := int(b[0])
	height := u16(b[1:])
	width := u16(b[3:])
	nf := int(b[5])
	if precision < 2 || precision > 16 {
		return unsupported("lj92: precision %d", precision)
	}
	if width == 0 || height == 0 {
		return corrupt("lj92: empty frame %dx%d", width, height)
	}
	if nf < 1 || nf > 4 {
		return unsupported("lj92: %d components", nf)
	}
	if len(b) != 6+3*nf {
		return corrupt("lj92: frame header size")
	}
	for i := 0; i < nf; i++ {
		hv := b[6+3*i+1]
		if hv != 0x11 {
			return unsupported("lj92: sampling %#x", hv)
		}
	}
	d.Info = LJ92Info{Width: width, Height: height, Components: nf, Precision: precision}
	return nil
}

func (d *LJ92) huffman(b []byte) error {
	for len(b) > 0 {
		if len(b) < 17 {
			return corrupt("lj92: DHT segment")
		}
		class, id := b[0]>>4, b[0]&15
		var counts [16]uint8
		copy(counts[:], b[1:17])
		total := 0
		for _, c := range counts {
			total += int(c)
		}
		if len(b) < 17+total {
			return corrupt("lj92: DHT symbols")
		}
		if class == 0 && id < maxHuffTables {
			if err := d.tables[id].build(counts, b[17:17+total]); err != nil {
				return err
			}
		}
		b = b[17+total:]
	}
	return nil
}

func (d *LJ92) scan(b []byte) error {
	if len(b) < 1 {
		return corrupt("lj92: scan header")
	}
	ns := int(b[0])
	if ns != d.Info.Components {
		return unsupported("lj92: non-interleaved scan")
	}
	if len(b) != 1+2*ns+3 {
		return corrupt("lj92: scan header size")
	}
	for i := 0; i < ns; i++ {
		td := int(b[2+2*i] >> 4)
		if td >= maxHuffTables || !d.tables[td].valid {
			return corrupt("lj92: scan uses missing huffman table %d", td)
		}
		d.tableOf[i] = td
	}
	tail := b[1+2*ns:]
	d.predictor = int(tail[0])
	d.pointXf = int(tail[2] & 15)
	if d.predictor < 1 || d.predictor > 7 {
		return unsupported("lj92: predictor %d", d.predictor)
	}
	if d.pointXf >= d.Info.Precision {
		return corrupt("lj92: point transform %d", d.pointXf)
	}
	return nil
}

func (d *LJ92) Decode(ctx context.Context, dst []uint16, stride int) error {
	w, h, c := d.Info.Width, d.Info.Height, d.Info.Components
	line := w * c
	if stride < line || (h-1)*stride+line > len(dst) {
		return corrupt("lj92: destination of %d samples is too small for %dx%dx%d", len(dst), w, h, c)
	}
	data := d.data
	pos := d.scanStart
	var acc uint64
	var n uint
	pad := 0
	pt := uint(d.pointXf)
	initial := int32(1) << uint(d.Info.Precision-d.pointXf-1)
	restartLeft := d.restart
	fresh := true
	var tabs [4]*huffTable
	for i := 0; i < c; i++ {
		tabs[i] = &d.tables[d.tableOf[i]]
	}
	for y := 0; y < h; y++ {
		if y%rowsPerCheck == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		row := dst[y*stride : y*stride+line]
		var prev []uint16
		if y > 0 {
			prev = dst[(y-1)*stride : (y-1)*stride+line]
		}
		firstRow := y == 0 || fresh
		atStart := false
		for x := 0; x < w; x++ {
			if d.restart > 0 && restartLeft == 0 {
				if pos+2 > len(data) || data[pos] != 0xff || data[pos+1] < 0xd0 || data[pos+1] > 0xd7 {
					return corrupt("lj92: restart marker expected at %d", pos)
				}
				pos += 2
				acc, n, pad = 0, 0, 0
				restartLeft = d.restart
				firstRow = true
				fresh = true
				atStart = true
			}
			restartLeft--
			for k := 0; k < c; k++ {
				if n < 33 {
					for n <= 56 {
						var b byte
						if pos < len(data) {
							b = data[pos]
							switch {
							case b != 0xff:
								pos++
							case pos+1 < len(data) && data[pos+1] == 0:
								pos += 2
							default:
								b = 0
								pad++
							}
						} else {
							pad++
						}
						acc |= uint64(b) << (56 - n)
						n += 8
					}
				}
				t := tabs[k]
				var sym uint
				if e := t.fast[acc>>(64-fastBits)]; e != 0 {
					l := uint(e >> 8)
					sym = uint(e & 0xff)
					acc <<= l
					n -= l
				} else {
					peek := int32(acc >> 48)
					l := fastBits + 1
					for ; l <= 16; l++ {
						code := peek >> uint(16-l)
						if code <= t.maxcode[l] && t.maxcode[l] >= 0 {
							sym = uint(t.vals[t.valptr[l]+code-t.mincode[l]])
							break
						}
					}
					if l > 16 {
						return corrupt("lj92: invalid huffman code")
					}
					acc <<= uint(l)
					n -= uint(l)
				}
				var diff int32
				switch {
				case sym == 0:
				case sym == 16:
					diff = 32768
				case sym > 16:
					return corrupt("lj92: category %d", sym)
				default:
					v := int32(acc >> (64 - sym))
					acc <<= sym
					n -= sym
					diff = v
					if v < 1<<(sym-1) {
						diff = v - (1 << sym) + 1
					}
				}
				i := x*c + k
				var pred int32
				if firstRow {
					if x == 0 || atStart {
						pred = initial
					} else {
						pred = int32(row[i-c] >> pt)
					}
				} else if x == 0 {
					pred = int32(prev[k] >> pt)
				} else {
					ra := int32(row[i-c] >> pt)
					rb := int32(prev[i] >> pt)
					switch d.predictor {
					case 1:
						pred = ra
					case 2:
						pred = rb
					case 3:
						pred = int32(prev[i-c] >> pt)
					case 4:
						pred = ra + rb - int32(prev[i-c]>>pt)
					case 5:
						pred = ra + (rb-int32(prev[i-c]>>pt))>>1
					case 6:
						pred = rb + (ra-int32(prev[i-c]>>pt))>>1
					default:
						pred = (ra + rb) >> 1
					}
				}
				row[i] = uint16(pred+diff) << pt
			}
			atStart = false
		}
		fresh = false
		if uint(pad*8) > n {
			return corrupt("lj92: data ends inside row %d", y)
		}
	}
	return nil
}
