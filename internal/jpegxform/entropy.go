package jpegxform

import (
	"io"
	"math/bits"
)

const writerBuffer = 64 << 10

type bitWriter struct {
	w   io.Writer
	buf []byte
	acc uint32
	n   uint
	err error
}

func newBitWriter(w io.Writer) *bitWriter {
	return &bitWriter{w: w, buf: make([]byte, 0, writerBuffer)}
}

func (b *bitWriter) flushBuffer() {
	if b.err == nil && len(b.buf) > 0 {
		_, b.err = b.w.Write(b.buf)
	}
	b.buf = b.buf[:0]
}

func (b *bitWriter) put(code uint32, size uint) {
	b.acc = b.acc<<size | code&(1<<size-1)
	b.n += size
	for b.n >= 8 {
		b.n -= 8
		c := byte(b.acc >> b.n)
		b.buf = append(b.buf, c)
		if c == 0xFF {
			b.buf = append(b.buf, 0)
		}
	}
	b.acc &= 1<<b.n - 1
	if len(b.buf) >= writerBuffer-8 {
		b.flushBuffer()
	}
}

func (b *bitWriter) finish() error {
	if b.n > 0 {
		b.put(1<<(8-b.n)-1, 8-b.n)
	}
	b.flushBuffer()
	return b.err
}

type tableRef struct {
	enc  *encoderTable
	freq *[symbolSentinel + 1]int
}

type blockCoder struct {
	out *bitWriter
}

func (c *blockCoder) symbol(t *tableRef, sym int) {
	if c.out == nil {
		t.freq[sym]++
		return
	}
	c.out.put(uint32(t.enc.code[sym]), uint(t.enc.size[sym]))
}

func (c *blockCoder) extra(v int, size int) {
	if c.out == nil || size == 0 {
		return
	}
	if v < 0 {
		v--
	}
	c.out.put(uint32(v), uint(size))
}

func (c *blockCoder) block(blk []int16, pred int, dc, ac *tableRef) int {
	cur := int(blk[0])
	diff := cur - pred
	mag := diff
	if mag < 0 {
		mag = -mag
	}
	size := bits.Len(uint(mag))
	c.symbol(dc, size)
	c.extra(diff, size)
	run := 0
	for k := 1; k < blockSize; k++ {
		v := int(blk[zigzag[k]])
		if v == 0 {
			run++
			continue
		}
		for run > 15 {
			c.symbol(ac, 0xF0)
			run -= 16
		}
		m := v
		if m < 0 {
			m = -m
		}
		s := bits.Len(uint(m))
		c.symbol(ac, run<<4|s)
		c.extra(v, s)
		run = 0
	}
	if run > 0 {
		c.symbol(ac, 0)
	}
	return cur
}
