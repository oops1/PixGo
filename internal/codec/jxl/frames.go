package jxl

import (
	"context"
	"errors"

	"github.com/oops1/PixGo/internal/codec/internal/guard"
)

const (
	frameRegular         = 0
	frameLowFrequency    = 1
	frameReferenceOnly   = 2
	frameSkipProgressive = 3
	flagUseLowFrequency  = 0x20
	blendReplace         = 0
	maxTableEntries      = 1 << 24
	initialWindow        = 4 << 10
	frameWindow          = 512
	maxWindow            = 16 << 20
	maxScannedFrames     = 1 << 20
	contextCheckEvery    = 64
)

var (
	errPermutedTable = errors.New("jxl: permuted table of contents")
	upsamplingChoice = [4]distribution{value(1), value(2), value(4), value(8)}
	cropChoice       = [4]distribution{plain(8), shifted(11, 256), shifted(14, 2304), shifted(30, 18688)}
	tableChoice      = [4]distribution{plain(10), shifted(14, 1024), shifted(22, 17408), shifted(30, 4211712)}
	horizontalShift  = [4]uint32{0, 1, 1, 0}
	verticalShift    = [4]uint32{0, 1, 0, 1}
)

type frameHeader struct {
	kind           uint32
	modular        bool
	flags          uint64
	jpegUpsampling [3]uint32
	upsampling     uint32
	groupSizeShift uint32
	passes         uint32
	lowFreqLevel   uint32
	cropped        bool
	frameWidth     uint32
	frameHeight    uint32
	duration       uint32
	last           bool
}

func (f frameHeader) visible() bool {
	return (f.last || f.duration > 0) && (f.kind == frameRegular || f.kind == frameSkipProgressive)
}

func readPasses(b *bitReader, f *frameHeader) {
	f.passes = b.u32([4]distribution{value(1), value(2), value(3), shifted(3, 4)})
	if f.passes == 1 {
		return
	}
	downsamples := b.u32([4]distribution{value(0), value(1), value(2), shifted(1, 3)})
	b.skip(int(f.passes-1) * 2)
	for i := uint32(0); i < downsamples; i++ {
		b.u32(upsamplingChoice)
	}
	for i := uint32(0); i < downsamples; i++ {
		b.u32([4]distribution{value(0), value(1), value(2), plain(3)})
	}
}

func readBlending(b *bitReader, extraChannels int, fullFrame bool) (replace bool) {
	mode := b.u32([4]distribution{value(0), value(1), value(2), shifted(2, 3)})
	usesAlpha := mode == 2 || mode == 3
	if extraChannels > 0 && usesAlpha {
		b.u32([4]distribution{value(0), value(1), value(2), shifted(3, 3)})
	}
	if (extraChannels > 0 && usesAlpha) || mode == 4 {
		b.skip(1)
	}
	if !(fullFrame && mode == blendReplace) {
		b.u32([4]distribution{value(0), value(1), value(2), value(3)})
	}
	return mode == blendReplace
}

func readLoopFilter(b *bitReader, modular bool) {
	if b.flag() {
		return
	}
	gaborish := b.flag()
	if gaborish && b.flag() {
		b.skip(6 * 16)
	}
	iterations := b.bits(2)
	if iterations > 0 && !modular && b.flag() {
		b.skip(8 * 16)
	}
	if iterations > 0 && b.flag() {
		b.skip(5 * 16)
	}
	if iterations > 0 && b.flag() {
		if !modular {
			b.skip(16)
		}
		b.skip(3 * 16)
	}
	if iterations > 0 && modular {
		b.skip(16)
	}
	b.extensions()
}

func readFrameHeader(b *bitReader, image imageHeader, imageWidth, imageHeight uint32) frameHeader {
	f := frameHeader{upsampling: 1, groupSizeShift: 1, passes: 1}
	if b.flag() {
		f.last = true
		return f
	}
	extraChannels := len(image.extraChannels)
	f.kind = uint32(b.bits(2))
	f.modular = b.bits(1) == 1
	f.flags = b.u64()
	ycbcr := false
	if !image.xybEncoded {
		ycbcr = b.flag()
	}
	usesLowFrequency := f.flags&flagUseLowFrequency != 0
	if ycbcr && !usesLowFrequency {
		for i := range f.jpegUpsampling {
			f.jpegUpsampling[i] = uint32(b.bits(2))
		}
	}
	if !usesLowFrequency {
		f.upsampling = b.u32(upsamplingChoice)
		for i := 0; i < extraChannels; i++ {
			b.u32(upsamplingChoice)
		}
	}
	if f.modular {
		f.groupSizeShift = uint32(b.bits(2))
	}
	if !f.modular && image.xybEncoded {
		b.skip(6)
	}
	if f.kind != frameReferenceOnly {
		readPasses(b, &f)
	}
	if f.kind == frameLowFrequency {
		f.lowFreqLevel = b.u32([4]distribution{value(1), value(2), value(3), value(4)})
	} else {
		f.cropped = b.flag()
	}
	var originX, originY int32
	if f.cropped && f.kind != frameReferenceOnly {
		originX = unpackSigned(b.u32(cropChoice))
		originY = unpackSigned(b.u32(cropChoice))
	}
	if f.cropped {
		f.frameWidth = b.u32(cropChoice)
		f.frameHeight = b.u32(cropChoice)
	}
	covers := originX <= 0 && originY <= 0 &&
		int64(f.frameWidth)+int64(originX) >= int64(imageWidth) &&
		int64(f.frameHeight)+int64(originY) >= int64(imageHeight)
	fullFrame := !f.cropped || covers
	normal := f.kind == frameRegular || f.kind == frameSkipProgressive
	replace := false
	if normal {
		replace = readBlending(b, extraChannels, fullFrame)
		for i := 0; i < extraChannels; i++ {
			readBlending(b, extraChannels, fullFrame)
		}
	}
	if normal && image.animated {
		f.duration = b.u32([4]distribution{value(0), value(1), plain(8), plain(32)})
		if image.timecodes {
			b.skip(32)
		}
	}
	f.last = f.kind == frameRegular
	if normal {
		f.last = b.flag()
	}
	var savedReference uint64
	if f.kind != frameLowFrequency && !f.last {
		savedReference = b.bits(2)
	}
	referenced := !f.last && f.kind != frameLowFrequency && (f.duration == 0 || savedReference != 0)
	if f.kind == frameReferenceOnly || (referenced && replace && fullFrame && normal) {
		b.skip(1)
	}
	b.text()
	readLoopFilter(b, f.modular)
	b.extensions()
	return f
}

func ceilDiv(value, divisor uint64) uint64 { return (value + divisor - 1) / divisor }

func (f frameHeader) tableEntries(imageWidth, imageHeight uint32) uint64 {
	width, height := uint64(f.frameWidth), uint64(f.frameHeight)
	if width == 0 {
		width = uint64(imageWidth)
	}
	if height == 0 {
		height = uint64(imageHeight)
	}
	scale := uint64(1) << (3 * f.lowFreqLevel)
	width, height = ceilDiv(width, scale), ceilDiv(height, scale)
	width, height = ceilDiv(width, uint64(f.upsampling)), ceilDiv(height, uint64(f.upsampling))
	var maxHorizontal, maxVertical uint32
	for _, choice := range f.jpegUpsampling {
		maxHorizontal = max(maxHorizontal, horizontalShift[choice])
		maxVertical = max(maxVertical, verticalShift[choice])
	}
	blockWidth := ceilDiv(width, 8<<maxHorizontal) << maxHorizontal
	blockHeight := ceilDiv(height, 8<<maxVertical) << maxVertical
	group := uint64(1) << (7 + f.groupSizeShift)
	groups := ceilDiv(width, group) * ceilDiv(height, group)
	if groups == 1 && f.passes == 1 {
		return 1
	}
	lowFreqGroups := ceilDiv(blockWidth, group) * ceilDiv(blockHeight, group)
	return 2 + lowFreqGroups + groups*uint64(f.passes)
}

type frameLayout struct {
	header      frameHeader
	headerBytes int
	dataBytes   uint64
}

func parseFrame(data []byte, image imageHeader, imageWidth, imageHeight uint32) (frameLayout, error) {
	b := newBitReader(data)
	header := readFrameHeader(b, image, imageWidth, imageHeight)
	if b.invalid {
		return frameLayout{}, guard.Corrupt("jxl: invalid frame header")
	}
	if b.exhausted {
		return frameLayout{}, errNeedMoreData
	}
	entries := header.tableEntries(imageWidth, imageHeight)
	if entries > maxTableEntries {
		return frameLayout{}, guard.Corrupt("jxl: %d table of contents entries", entries)
	}
	if b.flag() {
		return frameLayout{}, errPermutedTable
	}
	b.alignToByte()
	var total uint64
	for i := uint64(0); i < entries; i++ {
		total += uint64(b.u32(tableChoice))
		if b.exhausted {
			return frameLayout{}, errNeedMoreData
		}
	}
	b.alignToByte()
	if b.exhausted {
		return frameLayout{}, errNeedMoreData
	}
	if b.invalid || total > 1<<32-1 {
		return frameLayout{}, guard.Corrupt("jxl: invalid table of contents")
	}
	return frameLayout{header: header, headerBytes: b.position / 8, dataBytes: total}, nil
}

func (s *codestream) frameAt(offset int64, image imageHeader, imageWidth, imageHeight uint32) (frameLayout, error) {
	window := int64(frameWindow)
	for {
		available := s.size - offset
		if available <= 0 {
			return frameLayout{}, guard.Corrupt("jxl: codestream ends before the last frame")
		}
		data, err := s.window(offset, min(window, available))
		if err != nil {
			return frameLayout{}, err
		}
		layout, err := parseFrame(data, image, imageWidth, imageHeight)
		if !errors.Is(err, errNeedMoreData) {
			return layout, err
		}
		if int64(len(data)) >= available {
			return frameLayout{}, guard.Corrupt("jxl: codestream ends inside a frame header")
		}
		if window >= maxWindow {
			return frameLayout{}, guard.Corrupt("jxl: frame header larger than %d bytes", maxWindow)
		}
		window *= 4
	}
}

type frameCount struct {
	visible int
	total   int
}

func (s *codestream) countFrames(ctx context.Context, image imageHeader, maxVisible int) (frameCount, error) {
	var counted frameCount
	offset := int64((image.endBit + 7) / 8)
	if image.hasPreview {
		layout, err := s.frameAt(offset, image, image.previewWidth, image.previewHeight)
		if err != nil {
			return counted, err
		}
		offset += int64(layout.headerBytes) + int64(layout.dataBytes)
	}
	for counted.total < maxScannedFrames {
		if counted.total%contextCheckEvery == 0 {
			if err := ctx.Err(); err != nil {
				return counted, err
			}
		}
		layout, err := s.frameAt(offset, image, image.width, image.height)
		if err != nil {
			return counted, err
		}
		end := offset + int64(layout.headerBytes) + int64(layout.dataBytes)
		if end > s.size {
			return counted, guard.Corrupt("jxl: frame %d runs %d bytes past the end of the codestream", counted.total, end-s.size)
		}
		counted.total++
		if layout.header.visible() {
			counted.visible++
			if counted.visible > maxVisible {
				return counted, guard.TooLarge("jxl: more than %d frames", maxVisible)
			}
		}
		if layout.header.last {
			return counted, nil
		}
		offset = end
	}
	return counted, guard.TooLarge("jxl: more than %d frames in the stream", maxScannedFrames)
}
