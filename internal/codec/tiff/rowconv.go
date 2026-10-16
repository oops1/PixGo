package tiff

import (
	"sync"

	"github.com/oops1/PixGo/internal/codec/internal/guard"
)

var (
	unpremultiplyTable     [256 * 256]uint8
	unpremultiplyTableOnce sync.Once
)

func buildUnpremultiplyTable() {
	for a := 1; a < 256; a++ {
		for v := 0; v < 256; v++ {
			unpremultiplyTable[a<<8|v] = uint8((uint32(v) * 0xffff / uint32(a)) >> 8)
		}
	}
}

func (lay *layout) undoPredictor(row []byte, pixels int) {
	step := lay.samples
	count := pixels * step
	if lay.bits == 8 {
		for i := step; i < count; i++ {
			row[i] += row[i-step]
		}
		return
	}
	if lay.littleEndian {
		for i := step; i < count; i++ {
			sum := (uint16(row[2*i]) | uint16(row[2*i+1])<<8) + (uint16(row[2*(i-step)]) | uint16(row[2*(i-step)+1])<<8)
			row[2*i], row[2*i+1] = byte(sum), byte(sum>>8)
		}
		return
	}
	for i := step; i < count; i++ {
		sum := (uint16(row[2*i])<<8 | uint16(row[2*i+1])) + (uint16(row[2*(i-step)])<<8 | uint16(row[2*(i-step)+1]))
		row[2*i], row[2*i+1] = byte(sum>>8), byte(sum)
	}
}

func (lay *layout) convertRow(dst, src []byte, pixels int) error {
	switch lay.mode {
	case modeIndexed:
		return lay.convertIndexed(dst, src, pixels)
	case modeGray16:
		lay.convertGray16(dst, src, pixels)
	case modeRGB8:
		for i := 0; i < pixels; i++ {
			dst[4*i], dst[4*i+1], dst[4*i+2], dst[4*i+3] = src[3*i], src[3*i+1], src[3*i+2], 0xff
		}
	case modeRGB16:
		high := lay.highByte()
		for i := 0; i < pixels; i++ {
			dst[4*i], dst[4*i+1], dst[4*i+2], dst[4*i+3] = src[6*i+high], src[6*i+2+high], src[6*i+4+high], 0xff
		}
	case modeRGBA8:
		if lay.associated {
			unpremultiplyTableOnce.Do(buildUnpremultiplyTable)
			unpremultiply8(dst, src, pixels)
		} else {
			copy(dst[:4*pixels], src[:4*pixels])
		}
	case modeRGBA16:
		lay.convertRGBA16(dst, src, pixels)
	}
	return nil
}

func (lay *layout) highByte() int {
	if lay.littleEndian {
		return 1
	}
	return 0
}

func (lay *layout) convertIndexed(dst, src []byte, pixels int) error {
	lutLen := lay.lutLen
	if lay.bits == 8 {
		for i := 0; i < pixels; i++ {
			index := int(src[i])
			if index >= lutLen {
				return guard.Corrupt("tiff: palette index %d beyond %d colours", index, lutLen)
			}
			c := lay.lut[index]
			dst[4*i], dst[4*i+1], dst[4*i+2], dst[4*i+3] = c[0], c[1], c[2], c[3]
		}
		return nil
	}
	bits := uint(lay.bits)
	mask := byte(1<<bits - 1)
	for i := 0; i < pixels; i++ {
		position := uint(i) * bits
		shift := 8 - bits - (position & 7)
		index := int((src[position>>3] >> shift) & mask)
		if index >= lutLen {
			return guard.Corrupt("tiff: palette index %d beyond %d colours", index, lutLen)
		}
		c := lay.lut[index]
		dst[4*i], dst[4*i+1], dst[4*i+2], dst[4*i+3] = c[0], c[1], c[2], c[3]
	}
	return nil
}

func (lay *layout) convertGray16(dst, src []byte, pixels int) {
	high := lay.highByte()
	for i := 0; i < pixels; i++ {
		v := src[2*i+high]
		if lay.invert {
			v = 255 - v
		}
		dst[4*i], dst[4*i+1], dst[4*i+2], dst[4*i+3] = v, v, v, 0xff
	}
}

func unpremultiply8(dst, src []byte, pixels int) {
	for o := 0; o < pixels*4; o += 4 {
		a := src[o+3]
		switch a {
		case 0xff:
			dst[o], dst[o+1], dst[o+2], dst[o+3] = src[o], src[o+1], src[o+2], 0xff
		case 0:
			dst[o], dst[o+1], dst[o+2], dst[o+3] = 0, 0, 0, 0
		default:
			base := int(a) << 8
			dst[o] = unpremultiplyTable[base|int(src[o])]
			dst[o+1] = unpremultiplyTable[base|int(src[o+1])]
			dst[o+2] = unpremultiplyTable[base|int(src[o+2])]
			dst[o+3] = a
		}
	}
}

func (lay *layout) word(src []byte, index int) uint32 {
	if lay.littleEndian {
		return uint32(src[2*index]) | uint32(src[2*index+1])<<8
	}
	return uint32(src[2*index])<<8 | uint32(src[2*index+1])
}

func (lay *layout) convertRGBA16(dst, src []byte, pixels int) {
	high := lay.highByte()
	for i := 0; i < pixels; i++ {
		o := 4 * i
		if !lay.associated {
			dst[o], dst[o+1], dst[o+2], dst[o+3] = src[8*i+high], src[8*i+2+high], src[8*i+4+high], src[8*i+6+high]
			continue
		}
		r, g, b, a := lay.word(src, 4*i), lay.word(src, 4*i+1), lay.word(src, 4*i+2), lay.word(src, 4*i+3)
		switch a {
		case 0xffff:
			dst[o], dst[o+1], dst[o+2], dst[o+3] = uint8(r>>8), uint8(g>>8), uint8(b>>8), 0xff
		case 0:
			dst[o], dst[o+1], dst[o+2], dst[o+3] = 0, 0, 0, 0
		default:
			dst[o] = uint8((r * 0xffff / a) >> 8)
			dst[o+1] = uint8((g * 0xffff / a) >> 8)
			dst[o+2] = uint8((b * 0xffff / a) >> 8)
			dst[o+3] = uint8(a >> 8)
		}
	}
}
