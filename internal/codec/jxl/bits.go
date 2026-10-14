package jxl

type distribution struct {
	bits   uint8
	offset uint32
	direct bool
}

func value(v uint32) distribution { return distribution{offset: v, direct: true} }

func plain(n uint8) distribution { return distribution{bits: n} }

func shifted(n uint8, offset uint32) distribution { return distribution{bits: n, offset: offset} }

type bitReader struct {
	data      []byte
	position  int
	exhausted bool
	invalid   bool
}

func newBitReader(data []byte) *bitReader { return &bitReader{data: data} }

func (b *bitReader) reject() {
	if !b.exhausted {
		b.invalid = true
	}
}

func (b *bitReader) left() int { return len(b.data)*8 - b.position }

func (b *bitReader) bits(n int) uint64 {
	if n == 0 {
		return 0
	}
	if n > b.left() {
		b.exhausted = true
		b.position = len(b.data) * 8
		return 0
	}
	var out uint64
	for i := 0; i < n; i++ {
		at := b.position + i
		out |= uint64(b.data[at>>3]>>(at&7)&1) << i
	}
	b.position += n
	return out
}

func (b *bitReader) flag() bool { return b.bits(1) != 0 }

func (b *bitReader) skip(n int) {
	if n < 0 || n > b.left() {
		b.exhausted = true
		b.position = len(b.data) * 8
		return
	}
	b.position += n
}

func (b *bitReader) u32(options [4]distribution) uint32 {
	choice := options[b.bits(2)]
	if choice.direct {
		return choice.offset
	}
	return uint32(b.bits(int(choice.bits))) + choice.offset
}

func (b *bitReader) u64() uint64 {
	switch b.bits(2) {
	case 0:
		return 0
	case 1:
		return 1 + b.bits(4)
	case 2:
		return 17 + b.bits(8)
	}
	out := b.bits(12)
	for shift := 12; b.flag(); shift += 8 {
		if shift == 60 {
			out |= b.bits(4) << shift
			break
		}
		out |= b.bits(8) << shift
	}
	return out
}

func (b *bitReader) enum() uint32 {
	return b.u32([4]distribution{value(0), value(1), shifted(4, 2), shifted(6, 18)})
}

func (b *bitReader) alignToByte() {
	if pad := (8 - b.position%8) % 8; b.bits(pad) != 0 {
		b.reject()
	}
}

func (b *bitReader) text() {
	length := int(b.u32([4]distribution{value(0), plain(4), shifted(5, 16), shifted(10, 48)}))
	if length > b.left()/8 {
		b.exhausted = true
		b.position = len(b.data) * 8
		return
	}
	b.skip(length * 8)
}

func (b *bitReader) extensions() {
	selector := b.u64()
	if selector == 0 {
		return
	}
	var total uint64
	for i := 0; i < 64; i++ {
		if selector&(1<<i) == 0 {
			continue
		}
		size := b.u64()
		if total+size < total {
			b.reject()
			return
		}
		total += size
	}
	if total > uint64(b.left()) {
		b.exhausted = true
		b.position = len(b.data) * 8
		return
	}
	b.skip(int(total))
}

func unpackSigned(v uint32) int32 {
	return int32(v>>1) ^ (int32(^v&1) - 1)
}
