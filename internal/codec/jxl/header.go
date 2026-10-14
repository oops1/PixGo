package jxl

import (
	"errors"

	"github.com/oops1/PixGo/internal/codec/internal/guard"
)

const (
	extraChannelAlpha = 0
	colorSpaceRGB     = 0
	colorSpaceGray    = 1
	colorSpaceXYB     = 2
	colorSpaceUnknown = 3
	whitePointCustom  = 2
	primariesCustom   = 2
	maxExtraChannels  = 256
)

var errNeedMoreData = errors.New("jxl: header continues past the data read")

type extraChannel struct {
	kind          uint32
	floatSample   bool
	bitsPerSample uint32
}

type imageHeader struct {
	width           uint32
	height          uint32
	orientation     uint32
	hasPreview      bool
	previewWidth    uint32
	previewHeight   uint32
	animated        bool
	ticksNumerator  uint32
	ticksDenom      uint32
	loops           uint32
	timecodes       bool
	floatSample     bool
	bitsPerSample   uint32
	extraChannels   []extraChannel
	xybEncoded      bool
	colorSpace      uint32
	embeddedProfile bool
	endBit          int
}

func (h imageHeader) transposes() bool { return h.orientation >= 5 && h.orientation <= 8 }

func (h imageHeader) displaySize() (int, int) {
	if h.transposes() {
		return int(h.height), int(h.width)
	}
	return int(h.width), int(h.height)
}

func (h imageHeader) hasAlpha() bool {
	for _, channel := range h.extraChannels {
		if channel.kind == extraChannelAlpha {
			return true
		}
	}
	return false
}

func (h imageHeader) colorChannels() int {
	if h.colorSpace == colorSpaceGray {
		return 1
	}
	return 3
}

func widthFromRatio(ratio, height uint64) uint64 {
	switch ratio {
	case 1:
		return height
	case 2:
		return height * 12 / 10
	case 3:
		return height * 4 / 3
	case 4:
		return height * 3 / 2
	case 5:
		return height * 16 / 9
	case 6:
		return height * 5 / 4
	case 7:
		return height * 2
	}
	return 0
}

func finishSize(b *bitReader, width, height uint64) (uint32, uint32) {
	if width == 0 || height == 0 || width > 1<<32-1 || height > 1<<32-1 {
		b.reject()
		return 0, 0
	}
	return uint32(width), uint32(height)
}

func readSizeHeader(b *bitReader) (width, height uint32) {
	small := b.flag()
	read := func() uint64 {
		if small {
			return (b.bits(5) + 1) * 8
		}
		return uint64(b.u32([4]distribution{plain(9), plain(13), plain(18), plain(30)})) + 1
	}
	y := read()
	ratio := b.bits(3)
	x := widthFromRatio(ratio, y)
	if ratio == 0 {
		x = read()
	}
	return finishSize(b, x, y)
}

func readPreviewSize(b *bitReader) (width, height uint32) {
	divisibleByEight := b.flag()
	read := func() uint64 {
		if divisibleByEight {
			return uint64(b.u32([4]distribution{value(16), value(32), shifted(5, 1), shifted(9, 33)})) * 8
		}
		return uint64(b.u32([4]distribution{plain(6), shifted(8, 64), shifted(10, 320), shifted(12, 1344)})) + 1
	}
	y := read()
	ratio := b.bits(3)
	x := widthFromRatio(ratio, y)
	if ratio == 0 {
		x = read()
	}
	return finishSize(b, x, y)
}

func readBitDepth(b *bitReader) (floatSample bool, bitsPerSample uint32) {
	floatSample = b.flag()
	if floatSample {
		bitsPerSample = b.u32([4]distribution{value(32), value(16), value(24), shifted(6, 1)})
		b.skip(4)
		return floatSample, bitsPerSample
	}
	bitsPerSample = b.u32([4]distribution{value(8), value(10), value(12), shifted(6, 1)})
	if bitsPerSample > 31 {
		b.reject()
	}
	return floatSample, bitsPerSample
}

func readExtraChannel(b *bitReader) extraChannel {
	channel := extraChannel{kind: extraChannelAlpha, bitsPerSample: 8}
	if b.flag() {
		return channel
	}
	channel.kind = b.enum()
	channel.floatSample, channel.bitsPerSample = readBitDepth(b)
	b.u32([4]distribution{value(0), value(3), value(4), shifted(3, 1)})
	b.text()
	switch channel.kind {
	case 0:
		b.skip(1)
	case 2:
		b.skip(4 * 16)
	case 5:
		b.u32([4]distribution{value(1), plain(2), shifted(4, 3), shifted(8, 19)})
	}
	return channel
}

func readChromaticity(b *bitReader) {
	options := [4]distribution{plain(19), shifted(19, 524288), shifted(20, 1048576), shifted(21, 2097152)}
	b.u32(options)
	b.u32(options)
}

func readColorEncoding(b *bitReader) (colorSpace uint32, embeddedProfile bool) {
	if b.flag() {
		return colorSpaceRGB, false
	}
	embeddedProfile = b.flag()
	colorSpace = b.enum()
	if colorSpace > colorSpaceUnknown {
		b.reject()
		return colorSpace, embeddedProfile
	}
	whitePoint := uint32(1)
	if !embeddedProfile && colorSpace != colorSpaceXYB {
		whitePoint = b.enum()
	}
	if whitePoint == whitePointCustom {
		readChromaticity(b)
	}
	primaries := uint32(1)
	if !embeddedProfile && colorSpace != colorSpaceXYB && colorSpace != colorSpaceGray {
		primaries = b.enum()
	}
	if primaries == primariesCustom {
		readChromaticity(b)
		readChromaticity(b)
		readChromaticity(b)
	}
	if !embeddedProfile {
		if colorSpace != colorSpaceXYB {
			if b.flag() {
				b.skip(24)
			} else {
				b.enum()
			}
		}
		b.enum()
	}
	return colorSpace, embeddedProfile
}

func readTransformData(b *bitReader, xybEncoded bool) {
	if b.flag() {
		return
	}
	if xybEncoded && !b.flag() {
		b.skip(16 * 16)
	}
	mask := b.bits(3)
	if mask&1 != 0 {
		b.skip(15 * 16)
	}
	if mask&2 != 0 {
		b.skip(55 * 16)
	}
	if mask&4 != 0 {
		b.skip(210 * 16)
	}
}

func parseImageHeader(data []byte) (imageHeader, error) {
	b := newBitReader(data)
	header := imageHeader{orientation: 1, bitsPerSample: 8, xybEncoded: true}
	if b.bits(8) != 0xff || b.bits(8) != 0x0a {
		return header, guard.Corrupt("jxl: bad codestream signature")
	}
	header.width, header.height = readSizeHeader(b)
	if !b.flag() {
		extraFields := b.flag()
		if extraFields {
			header.orientation = uint32(b.bits(3)) + 1
			if b.flag() {
				readSizeHeader(b)
			}
			if header.hasPreview = b.flag(); header.hasPreview {
				header.previewWidth, header.previewHeight = readPreviewSize(b)
			}
			if header.animated = b.flag(); header.animated {
				header.ticksNumerator = b.u32([4]distribution{value(100), value(1000), shifted(10, 1), shifted(30, 1)})
				header.ticksDenom = b.u32([4]distribution{value(1), value(1001), shifted(8, 1), shifted(10, 1)})
				header.loops = b.u32([4]distribution{value(0), plain(3), plain(16), plain(32)})
				header.timecodes = b.flag()
			}
		}
		header.floatSample, header.bitsPerSample = readBitDepth(b)
		b.skip(1)
		count := b.u32([4]distribution{value(0), value(1), shifted(4, 2), shifted(12, 1)})
		if count > maxExtraChannels {
			return header, guard.Corrupt("jxl: %d extra channels", count)
		}
		if int(count) > b.left() {
			return header, errNeedMoreData
		}
		for i := uint32(0); i < count; i++ {
			header.extraChannels = append(header.extraChannels, readExtraChannel(b))
		}
		header.xybEncoded = b.flag()
		header.colorSpace, header.embeddedProfile = readColorEncoding(b)
		if extraFields && !b.flag() {
			b.skip(16 + 16 + 1 + 16)
		}
		b.extensions()
	}
	readTransformData(b, header.xybEncoded)
	if b.invalid {
		return header, guard.Corrupt("jxl: invalid image header")
	}
	if b.exhausted {
		return header, errNeedMoreData
	}
	if header.width == 0 || header.height == 0 {
		return header, guard.Corrupt("jxl: empty image")
	}
	header.endBit = b.position
	return header, nil
}
