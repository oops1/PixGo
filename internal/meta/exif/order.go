package exif

import "encoding/binary"

func (e Entry) ByteOrder() binary.ByteOrder {
	if e.reader == nil {
		return binary.LittleEndian
	}
	return e.reader.order
}

func (e Entry) InlineOffset() (uint32, bool) {
	if e.byteLen() <= 4 || e.reader == nil {
		return 0, false
	}
	return e.reader.order.Uint32(e.inline[:]), true
}
