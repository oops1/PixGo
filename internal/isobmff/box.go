package isobmff

import (
	"encoding/binary"
)

const (
	boxHeaderSize      = 8
	largeBoxHeaderSize = 16
	fullBoxHeaderSize  = 4
)

type cursor struct {
	data   []byte
	offset int
	failed bool
}

func newCursor(data []byte) *cursor { return &cursor{data: data} }

func (c *cursor) remaining() int {
	if c.failed {
		return 0
	}
	return len(c.data) - c.offset
}

func (c *cursor) fail() {
	c.failed = true
	c.offset = len(c.data)
}

func (c *cursor) take(n int) []byte {
	if n < 0 || c.remaining() < n {
		c.fail()
		return nil
	}
	out := c.data[c.offset : c.offset+n]
	c.offset += n
	return out
}

func (c *cursor) skip(n int) { c.take(n) }

func (c *cursor) unsigned(n int) uint64 {
	var value uint64
	for _, b := range c.take(n) {
		value = value<<8 | uint64(b)
	}
	return value
}

func (c *cursor) u8() uint8 { return uint8(c.unsigned(1)) }

func (c *cursor) u16() uint16 { return uint16(c.unsigned(2)) }

func (c *cursor) u32() uint32 { return uint32(c.unsigned(4)) }

func (c *cursor) u64() uint64 { return c.unsigned(8) }

func (c *cursor) fourCC() string {
	b := c.take(4)
	if b == nil {
		return ""
	}
	return string(b)
}

func (c *cursor) cString() string {
	for end := c.offset; end < len(c.data); end++ {
		if c.data[end] == 0 {
			text := string(c.data[c.offset:end])
			c.offset = end + 1
			return text
		}
	}
	c.fail()
	return ""
}

func (c *cursor) fullBox() (version uint8, flags uint32) {
	word := c.u32()
	return uint8(word >> 24), word & 0xffffff
}

func (c *cursor) rest() []byte { return c.take(c.remaining()) }

type boxHeader struct {
	kind    string
	headLen int
	total   uint64
}

func parseBoxHeader(b []byte, available uint64) (boxHeader, error) {
	if len(b) < boxHeaderSize {
		return boxHeader{}, corrupt("heif: truncated box header")
	}
	total := uint64(binary.BigEndian.Uint32(b[:4]))
	header := boxHeader{kind: string(b[4:8]), headLen: boxHeaderSize}
	switch total {
	case 1:
		if len(b) < largeBoxHeaderSize {
			return boxHeader{}, corrupt("heif: truncated large box header")
		}
		total = binary.BigEndian.Uint64(b[8:16])
		header.headLen = largeBoxHeaderSize
	case 0:
		total = available
	}
	if total < uint64(header.headLen) || total > available {
		return boxHeader{}, corrupt("heif: box %q of %d bytes does not fit in %d", header.kind, total, available)
	}
	header.total = total
	return header, nil
}

func eachBox(data []byte, visit func(kind string, payload []byte) error) error {
	for len(data) >= boxHeaderSize {
		header, err := parseBoxHeader(data, uint64(len(data)))
		if err != nil {
			return err
		}
		if header.kind != "uuid" {
			if err := visit(header.kind, data[header.headLen:header.total]); err != nil {
				return err
			}
		}
		data = data[header.total:]
	}
	return nil
}

func eachBoxLenient(data []byte, visit func(kind string, payload []byte) bool) {
	for len(data) >= boxHeaderSize {
		total := uint64(binary.BigEndian.Uint32(data[:4]))
		kind := string(data[4:8])
		head := uint64(boxHeaderSize)
		switch total {
		case 1:
			if len(data) < largeBoxHeaderSize {
				return
			}
			total = binary.BigEndian.Uint64(data[8:16])
			head = largeBoxHeaderSize
		case 0:
			total = uint64(len(data))
		}
		if total < head {
			return
		}
		end := min(total, uint64(len(data)))
		if !visit(kind, data[head:end]) {
			return
		}
		if end == uint64(len(data)) {
			return
		}
		data = data[end:]
	}
}
