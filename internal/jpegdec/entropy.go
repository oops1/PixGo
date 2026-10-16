package jpegdec

import (
	"encoding/binary"
	"io"
	"math/bits"
)

const refillTarget = 32

type entropy struct {
	s      *stream
	acc    uint64
	n      int
	pad    int
	ff     uint8
	hit    bool
	err    error
	eobRun uint16
}

func hasFF(w uint64) bool {
	return (^w-0x0101010101010101)&w&0x8080808080808080 != 0
}

func (e *entropy) reset() {
	e.acc = 0
	e.n = 0
	e.pad = 0
	e.ff = 0
	e.hit = false
	e.err = nil
}

func (e *entropy) stop(err error) {
	e.hit = true
	if err != io.EOF {
		e.err = err
	}
}

func (e *entropy) loadByte() {
	s := e.s
	if s.pos >= s.end {
		if err := s.more(); err != nil {
			e.stop(err)
			return
		}
	}
	c := s.buf[s.pos]
	if c != 0xff {
		s.pos++
		e.acc = e.acc<<8 | uint64(c)
		e.n += 8
		e.ff <<= 1
		return
	}
	if s.pos+1 >= s.end {
		if err := s.more(); err != nil {
			e.stop(err)
			return
		}
	}
	if s.buf[s.pos+1] != 0 {
		e.hit = true
		return
	}
	s.pos += 2
	e.acc = e.acc<<8 | 0xff
	e.n += 8
	e.ff = e.ff<<1 | 1
}

func (e *entropy) refill() {
	s := e.s
	for e.n <= 56 && !e.hit {
		if s.end-s.pos >= 8 {
			w := binary.BigEndian.Uint64(s.buf[s.pos : s.pos+8])
			if !hasFF(w) {
				take := (64 - e.n) >> 3
				e.acc = e.acc<<uint(8*take) | w>>uint(64-8*take)
				e.n += 8 * take
				e.ff <<= uint(take)
				s.pos += take
				continue
			}
		}
		e.loadByte()
	}
	if e.n < refillTarget {
		add := refillTarget - e.n
		e.acc <<= uint(add)
		e.n = refillTarget
		e.pad += add
	}
}

func (e *entropy) check() error {
	if e.hit {
		if e.err != nil {
			return e.err
		}
		if e.n < e.pad {
			return errShortData
		}
	}
	return nil
}

func (e *entropy) position() (offset int64, bit uint8, ok bool) {
	if e.pad != 0 || e.err != nil {
		return 0, 0, false
	}
	bytesHeld := (e.n + 7) / 8
	stuffed := bits.OnesCount16(uint16(e.ff) & (1<<uint(bytesHeld) - 1))
	return e.s.offset() - int64(bytesHeld+stuffed), uint8(bytesHeld*8 - e.n), true
}

func (e *entropy) resume(offset int64, bit uint8) {
	e.reset()
	e.s.seek(offset)
	e.refill()
	e.n -= int(bit)
}

func (e *entropy) decodeSymbol(t *huffTable) (uint8, error) {
	if e.n < refillTarget {
		e.refill()
	}
	v := t.lut[(e.acc>>uint(e.n-lutBits))&lutMask]
	if v != 0 {
		e.n -= int(v & 0xff)
		return uint8(v >> 8), nil
	}
	sym, length, ok := t.slow(e.acc, e.n)
	if !ok {
		return 0, errBadHuffmanCode
	}
	e.n -= length
	return sym, nil
}

func (e *entropy) takeBits(count int) int32 {
	e.n -= count
	return int32(e.acc>>uint(e.n)) & (1<<uint(count) - 1)
}

func extend(value int32, count int) int32 {
	return value + (((value >> uint(count-1)) - 1) & ((-1 << uint(count)) + 1))
}

func (e *entropy) receiveExtend(count int) int32 {
	if count == 0 {
		return 0
	}
	return extend(e.takeBits(count), count)
}

func (e *entropy) bit() bool {
	if e.n < 1 {
		e.refill()
	}
	e.n--
	return e.acc>>uint(e.n)&1 != 0
}

func (e *entropy) decodeBlock(b []int16, zz *[blockSize]int8, last int, dc, ac *huffTable, pred *int16) error {
	if e.n < refillTarget {
		e.refill()
	}
	t, err := e.decodeSymbol(dc)
	if err != nil {
		return err
	}
	if t > 16 {
		return UnsupportedError("excessive DC component")
	}
	diff := e.receiveExtend(int(t))
	*pred += int16(diff)
	b[0] = *pred

	if e.eobRun > 0 {
		e.eobRun--
		return nil
	}
	acc, n := e.acc, e.n
	k := 1
	finished := false
	for k <= last {
		if n < refillTarget {
			e.acc, e.n = acc, n
			e.refill()
			acc, n = e.acc, e.n
		}
		v := ac.lut[(acc>>uint(n-lutBits))&lutMask]
		var rs uint8
		if v != 0 {
			n -= int(v & 0xff)
			rs = uint8(v >> 8)
		} else {
			sym, length, ok := ac.slow(acc, n)
			if !ok {
				e.acc, e.n = acc, n
				return errBadHuffmanCode
			}
			n -= length
			rs = sym
		}
		size := int(rs & 15)
		run := int(rs >> 4)
		if size != 0 {
			k += run
			if k >= blockSize {
				finished = true
				break
			}
			n -= size
			value := int32(acc>>uint(n)) & (1<<uint(size) - 1)
			value += ((value >> uint(size-1)) - 1) & ((-1 << uint(size)) + 1)
			if z := zz[k]; z >= 0 {
				b[z] = int16(value)
			}
			k++
			continue
		}
		if run != 15 {
			eob := uint16(1) << uint(run)
			if run != 0 {
				n -= run
				eob |= uint16(acc>>uint(n)) & (1<<uint(run) - 1)
			}
			e.eobRun = eob - 1
			finished = true
			break
		}
		k += 16
	}
	for !finished && k < blockSize {
		if n < refillTarget {
			e.acc, e.n = acc, n
			e.refill()
			acc, n = e.acc, e.n
		}
		v := ac.lut[(acc>>uint(n-lutBits))&lutMask]
		var rs uint8
		if v != 0 {
			n -= int(v & 0xff)
			rs = uint8(v >> 8)
		} else {
			sym, length, ok := ac.slow(acc, n)
			if !ok {
				e.acc, e.n = acc, n
				return errBadHuffmanCode
			}
			n -= length
			rs = sym
		}
		size := int(rs & 15)
		run := int(rs >> 4)
		if size != 0 {
			k += run
			if k >= blockSize {
				break
			}
			n -= size
			k++
			continue
		}
		if run != 15 {
			eob := uint16(1) << uint(run)
			if run != 0 {
				n -= run
				eob |= uint16(acc>>uint(n)) & (1<<uint(run) - 1)
			}
			e.eobRun = eob - 1
			break
		}
		k += 16
	}
	e.acc, e.n = acc, n
	return nil
}
