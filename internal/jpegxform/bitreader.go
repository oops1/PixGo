package jpegxform

const lookupBits = 9

type huffDecoder struct {
	lookup  [1 << lookupBits]uint16
	maxCode [18]int32
	valPtr  [17]int32
	minCode [17]int32
	values  []byte
}

func newHuffDecoder(s huffSpec) (*huffDecoder, error) {
	d := &huffDecoder{values: s.symbols}
	code := int32(0)
	k := int32(0)
	for l := 1; l <= 16; l++ {
		n := int32(s.counts[l-1])
		d.valPtr[l] = k
		d.minCode[l] = code
		for i := int32(0); i < n; i++ {
			if code >= 1<<l {
				return nil, corrupt("oversubscribed Huffman table")
			}
			if l <= lookupBits {
				first := int(code) << (lookupBits - l)
				entry := uint16(l)<<8 | uint16(s.symbols[k])
				for j := 0; j < 1<<(lookupBits-l); j++ {
					d.lookup[first+j] = entry
				}
			}
			code++
			k++
		}
		if n > 0 {
			d.maxCode[l] = code - 1
		} else {
			d.maxCode[l] = -1
		}
		code <<= 1
	}
	return d, nil
}

type bitReader struct {
	src     *byteSource
	acc     uint64
	n       int
	fake    int
	marker  byte
	stopped bool
}

func newBitReader(src *byteSource) *bitReader { return &bitReader{src: src} }

func (b *bitReader) fill() {
	for b.n <= 56 {
		if b.stopped {
			b.acc <<= 8
			b.n += 8
			b.fake += 8
			continue
		}
		c, ok := b.src.readByte()
		if !ok {
			b.stopped = true
			continue
		}
		if c == 0xFF {
			d, ok := b.src.readByte()
			for ok && d == 0xFF {
				d, ok = b.src.readByte()
			}
			if !ok {
				b.stopped = true
				continue
			}
			if d != 0 {
				b.marker = d
				b.stopped = true
				continue
			}
		}
		b.acc = b.acc<<8 | uint64(c)
		b.n += 8
	}
}

func (b *bitReader) overrun() bool { return b.fake > b.n }

func (b *bitReader) decode(t *huffDecoder) (int, bool) {
	if b.n < 16 {
		b.fill()
	}
	idx := (b.acc >> uint(b.n-lookupBits)) & (1<<lookupBits - 1)
	if e := t.lookup[idx]; e != 0 {
		b.n -= int(e >> 8)
		return int(e & 0xFF), true
	}
	for l := lookupBits + 1; l <= 16; l++ {
		code := int32((b.acc >> uint(b.n-l)) & (1<<uint(l) - 1))
		if code <= t.maxCode[l] {
			b.n -= l
			return int(t.values[t.valPtr[l]+code-t.minCode[l]]), true
		}
	}
	return 0, false
}

func (b *bitReader) receive(s int) int {
	if b.n < s {
		b.fill()
	}
	b.n -= s
	v := int(b.acc>>uint(b.n)) & (1<<uint(s) - 1)
	if v < 1<<uint(s-1) {
		v -= 1<<uint(s) - 1
	}
	return v
}

func (b *bitReader) restart() error {
	if b.overrun() {
		return corrupt("scan data ends inside a restart interval")
	}
	b.acc, b.n, b.fake = 0, 0, 0
	for b.marker == 0 {
		c, ok := b.src.readByte()
		if !ok {
			return corrupt("missing restart marker")
		}
		if c != 0xFF {
			continue
		}
		d, ok := b.src.readByte()
		for ok && d == 0xFF {
			d, ok = b.src.readByte()
		}
		if !ok {
			return corrupt("missing restart marker")
		}
		if d != 0 {
			b.marker = d
		}
	}
	if b.marker < markerRST0 || b.marker > markerRST7 {
		return corrupt("marker %#x where a restart marker is due", b.marker)
	}
	b.marker = 0
	b.stopped = false
	return nil
}
