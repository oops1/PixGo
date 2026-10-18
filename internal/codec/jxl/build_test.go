package jxl

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"

	gjxl "github.com/gen2brain/jxl"

	"github.com/oops1/PixGo/internal/codec/internal/testimg"
)

type bitWriter struct {
	data     []byte
	position int
}

func (w *bitWriter) bits(value uint64, n int) {
	for i := 0; i < n; i++ {
		if w.position%8 == 0 {
			w.data = append(w.data, 0)
		}
		if value>>i&1 != 0 {
			w.data[w.position>>3] |= 1 << (w.position & 7)
		}
		w.position++
	}
}

func (w *bitWriter) flag(v bool) {
	if v {
		w.bits(1, 1)
		return
	}
	w.bits(0, 1)
}

func (w *bitWriter) u32(options [4]distribution, v uint32) {
	for selector, option := range options {
		if option.direct {
			if v == option.offset {
				w.bits(uint64(selector), 2)
				return
			}
			continue
		}
		if v >= option.offset && (option.bits >= 32 || uint64(v-option.offset) < 1<<option.bits) {
			w.bits(uint64(selector), 2)
			w.bits(uint64(v-option.offset), int(option.bits))
			return
		}
	}
	panic("value does not fit any distribution")
}

func (w *bitWriter) u64(v uint64) {
	switch {
	case v == 0:
		w.bits(0, 2)
	case v <= 16:
		w.bits(1, 2)
		w.bits(v-1, 4)
	case v <= 272:
		w.bits(2, 2)
		w.bits(v-17, 8)
	default:
		w.bits(3, 2)
		w.bits(v&0xfff, 12)
		rest := v >> 12
		for shift := 12; ; shift += 8 {
			if rest == 0 {
				w.flag(false)
				break
			}
			w.flag(true)
			if shift == 60 {
				w.bits(rest, 4)
				break
			}
			w.bits(rest&0xff, 8)
			rest >>= 8
		}
	}
}

func (w *bitWriter) enum(v uint32) {
	w.u32([4]distribution{value(0), value(1), shifted(4, 2), shifted(6, 18)}, v)
}

func (w *bitWriter) align() {
	for w.position%8 != 0 {
		w.bits(0, 1)
	}
}

func (w *bitWriter) text(s string) {
	w.u32([4]distribution{value(0), plain(4), shifted(5, 16), shifted(10, 48)}, uint32(len(s)))
	for _, c := range []byte(s) {
		w.bits(uint64(c), 8)
	}
}

type extraSpec struct {
	kind        uint32
	explicit    bool
	floatSample bool
	bits        uint32
	name        string
}

type headerSpec struct {
	width, height   uint32
	smallSize       bool
	ratio           uint32
	orientation     uint32
	intrinsic       bool
	preview         bool
	previewSize     uint32
	animated        bool
	ticksNumerator  uint32
	ticksDenom      uint32
	loops           uint32
	timecodes       bool
	floatSample     bool
	bitsPerSample   uint32
	modular16       bool
	extra           []extraSpec
	xyb             bool
	colorAllDefault bool
	colorSpace      uint32
	embeddedProfile bool
	whiteCustom     bool
	primariesCustom bool
	gamma           bool
	whiteEnum       uint32
	primariesEnum   uint32
	transferEnum    uint32
	gammaBits       uint32
	toneMapping     bool
	intensityBits   uint64
	extensions      bool
	transformData   bool
	allDefault      bool
}

func baseHeader(width, height uint32) headerSpec {
	return headerSpec{width: width, height: height, orientation: 1, bitsPerSample: 8, colorAllDefault: true, ticksNumerator: 100, ticksDenom: 1}
}

func (s headerSpec) writeSize(w *bitWriter, width, height uint32) {
	small := s.smallSize && width%8 == 0 && height%8 == 0 && width <= 256 && height <= 256 && (s.ratio == 0 || s.ratio == 1)
	w.flag(small)
	if small {
		w.bits(uint64(height/8-1), 5)
		ratio := s.ratio
		if ratio == 1 && width != height {
			ratio = 0
		}
		w.bits(uint64(ratio), 3)
		if ratio == 0 {
			w.bits(uint64(width/8-1), 5)
		}
		return
	}
	choices := [4]distribution{plain(9), plain(13), plain(18), plain(30)}
	w.u32(choices, height-1)
	switch {
	case s.ratio == 1 && width == height:
		w.bits(1, 3)
	case s.ratio == 7 && width == height*2:
		w.bits(7, 3)
	case s.ratio == 3 && width == height*4/3:
		w.bits(3, 3)
	default:
		w.bits(0, 3)
		w.u32(choices, width-1)
	}
}

func (s headerSpec) writePreview(w *bitWriter) {
	eights := [4]distribution{value(16), value(32), shifted(5, 1), shifted(9, 33)}
	pixels := [4]distribution{plain(6), shifted(8, 64), shifted(10, 320), shifted(12, 1344)}
	divisible := s.previewSize%8 == 0
	w.flag(divisible)
	if divisible {
		w.u32(eights, s.previewSize/8)
	} else {
		w.u32(pixels, s.previewSize-1)
	}
	w.bits(1, 3)
}

func (s headerSpec) writeBitDepth(w *bitWriter, floatSample bool, bits uint32) {
	w.flag(floatSample)
	if floatSample {
		w.u32([4]distribution{value(32), value(16), value(24), shifted(6, 1)}, bits)
		w.bits(7, 4)
		return
	}
	w.u32([4]distribution{value(8), value(10), value(12), shifted(6, 1)}, bits)
}

func cmpOr(v, fallback uint32) uint32 {
	if v != 0 {
		return v
	}
	return fallback
}

func (s headerSpec) writeColor(w *bitWriter) {
	w.flag(s.colorAllDefault)
	if s.colorAllDefault {
		return
	}
	w.flag(s.embeddedProfile)
	w.enum(s.colorSpace)
	whitePoint := uint32(1)
	if s.whiteEnum != 0 {
		whitePoint = s.whiteEnum
	}
	if s.whiteCustom && !s.embeddedProfile && s.colorSpace != colorSpaceXYB {
		whitePoint = 2
	}
	if !s.embeddedProfile && s.colorSpace != colorSpaceXYB {
		w.enum(whitePoint)
	}
	if whitePoint == 2 {
		chromaticity := [4]distribution{plain(19), shifted(19, 524288), shifted(20, 1048576), shifted(21, 2097152)}
		w.u32(chromaticity, 31270)
		w.u32(chromaticity, 600000)
	}
	primaries := uint32(1)
	if s.primariesEnum != 0 {
		primaries = s.primariesEnum
	}
	if s.primariesCustom {
		primaries = 2
	}
	if !s.embeddedProfile && s.colorSpace != colorSpaceXYB && s.colorSpace != colorSpaceGray {
		w.enum(primaries)
	}
	if primaries == 2 && !s.embeddedProfile && s.colorSpace != colorSpaceXYB && s.colorSpace != colorSpaceGray {
		chromaticity := [4]distribution{plain(19), shifted(19, 524288), shifted(20, 1048576), shifted(21, 2097152)}
		for i := 0; i < 6; i++ {
			w.u32(chromaticity, uint32(30000+i*1000))
		}
	}
	if !s.embeddedProfile {
		if s.colorSpace != colorSpaceXYB {
			w.flag(s.gamma)
			if s.gamma {
				w.bits(uint64(cmpOr(s.gammaBits, 3333333)), 24)
			} else {
				w.enum(cmpOr(s.transferEnum, 13))
			}
		}
		w.enum(1)
	}
}

func (s headerSpec) write() ([]byte, int) {
	w := &bitWriter{}
	w.bits(0xff, 8)
	w.bits(0x0a, 8)
	s.writeSize(w, s.width, s.height)
	if s.allDefault {
		w.flag(true)
	} else {
		w.flag(false)
		extraFields := s.orientation > 1 || s.intrinsic || s.preview || s.animated || s.toneMapping
		w.flag(extraFields)
		if extraFields {
			w.bits(uint64(max(s.orientation, 1)-1), 3)
			w.flag(s.intrinsic)
			if s.intrinsic {
				s.writeSize(w, s.width/2+1, s.height/2+1)
			}
			w.flag(s.preview)
			if s.preview {
				s.writePreview(w)
			}
			w.flag(s.animated)
			if s.animated {
				w.u32([4]distribution{value(100), value(1000), shifted(10, 1), shifted(30, 1)}, s.ticksNumerator)
				w.u32([4]distribution{value(1), value(1001), shifted(8, 1), shifted(10, 1)}, s.ticksDenom)
				w.u32([4]distribution{value(0), plain(3), plain(16), plain(32)}, s.loops)
				w.flag(s.timecodes)
			}
		}
		s.writeBitDepth(w, s.floatSample, s.bitsPerSample)
		w.flag(s.modular16)
		w.u32([4]distribution{value(0), value(1), shifted(4, 2), shifted(12, 1)}, uint32(len(s.extra)))
		for _, channel := range s.extra {
			w.flag(!channel.explicit)
			if !channel.explicit {
				continue
			}
			w.enum(channel.kind)
			s.writeBitDepth(w, channel.floatSample, max(channel.bits, 8))
			w.u32([4]distribution{value(0), value(3), value(4), shifted(3, 1)}, 0)
			w.text(channel.name)
			switch channel.kind {
			case 0:
				w.flag(false)
			case 2:
				w.bits(0, 64)
			case 5:
				w.u32([4]distribution{value(1), plain(2), shifted(4, 3), shifted(8, 19)}, 1)
			}
		}
		w.flag(s.xyb)
		s.writeColor(w)
		if extraFields {
			w.flag(!s.toneMapping)
			if s.toneMapping {
				w.bits(cmp64(s.intensityBits, 0x3c00), 16)
				w.bits(0, 16)
				w.flag(false)
				w.bits(0, 16)
			}
		}
		if s.extensions {
			w.u64(0b101)
			w.u64(5)
			w.u64(300)
			w.bits(0, 305)
		} else {
			w.u64(0)
		}
	}
	if s.transformData {
		w.flag(false)
		if s.xyb {
			w.flag(false)
			w.bits(0, 16*16)
		}
		w.bits(0b111, 3)
		w.bits(0, (15+55+210)*16)
	} else {
		w.flag(true)
	}
	return w.data, w.position
}

type frameSpec struct {
	allDefault      bool
	kind            uint32
	modular         bool
	flags           uint64
	ycbcr           bool
	jpegUpsampling  [3]uint32
	upsampling      uint32
	groupSizeShift  uint32
	passes          uint32
	lowFreqLevel    uint32
	cropped         bool
	originX         int32
	originY         int32
	frameWidth      uint32
	frameHeight     uint32
	blendMode       uint32
	extraBlending   []uint32
	duration        uint32
	last            bool
	savedReference  uint32
	name            string
	loopFilter      bool
	defaultFilter   bool
	permuted        bool
	entries         []uint32
	data            []byte
	exactEntryCount bool
}

func packSigned(v int32) uint32 {
	if v >= 0 {
		return uint32(v) << 1
	}
	return uint32(-v)<<1 - 1
}

func (f frameSpec) write(image headerSpec, imageWidth, imageHeight uint32) []byte {
	w := &bitWriter{}
	var header frameHeader
	if f.allDefault {
		w.flag(true)
		header = frameHeader{upsampling: 1, groupSizeShift: 1, passes: 1, last: true}
	} else {
		w.flag(false)
		w.bits(uint64(f.kind), 2)
		w.flag(f.modular)
		w.u64(f.flags)
		if !image.xyb {
			w.flag(f.ycbcr)
		}
		usesLow := f.flags&flagUseLowFrequency != 0
		if f.ycbcr && !usesLow {
			for _, v := range f.jpegUpsampling {
				w.bits(uint64(v), 2)
			}
		}
		upsampling := max(f.upsampling, 1)
		if !usesLow {
			w.u32(upsamplingChoice, upsampling)
			for range image.extra {
				w.u32(upsamplingChoice, 1)
			}
		}
		if f.modular {
			w.bits(uint64(f.groupSizeShift), 2)
		}
		if !f.modular && image.xyb {
			w.bits(0, 6)
		}
		passes := max(f.passes, 1)
		if f.kind != frameReferenceOnly {
			w.u32([4]distribution{value(1), value(2), value(3), shifted(3, 4)}, passes)
			if passes != 1 {
				w.u32([4]distribution{value(0), value(1), value(2), shifted(1, 3)}, 0)
				w.bits(0, int(passes-1)*2)
			}
		}
		if f.kind == frameLowFrequency {
			w.u32([4]distribution{value(1), value(2), value(3), value(4)}, max(f.lowFreqLevel, 1))
		} else {
			w.flag(f.cropped)
		}
		if f.cropped && f.kind != frameReferenceOnly {
			w.u32(cropChoice, packSigned(f.originX))
			w.u32(cropChoice, packSigned(f.originY))
		}
		if f.cropped {
			w.u32(cropChoice, f.frameWidth)
			w.u32(cropChoice, f.frameHeight)
		}
		covers := f.originX <= 0 && f.originY <= 0 && int64(f.frameWidth)+int64(f.originX) >= int64(imageWidth) && int64(f.frameHeight)+int64(f.originY) >= int64(imageHeight)
		fullFrame := !f.cropped || covers
		normal := f.kind == frameRegular || f.kind == frameSkipProgressive
		writeBlend := func(mode uint32) {
			w.u32([4]distribution{value(0), value(1), value(2), shifted(2, 3)}, mode)
			usesAlpha := mode == 2 || mode == 3
			if len(image.extra) > 0 && usesAlpha {
				w.u32([4]distribution{value(0), value(1), value(2), shifted(3, 3)}, 0)
			}
			if (len(image.extra) > 0 && usesAlpha) || mode == 4 {
				w.flag(false)
			}
			if !(fullFrame && mode == 0) {
				w.u32([4]distribution{value(0), value(1), value(2), value(3)}, 0)
			}
		}
		if normal {
			writeBlend(f.blendMode)
			for i := range image.extra {
				mode := uint32(0)
				if i < len(f.extraBlending) {
					mode = f.extraBlending[i]
				}
				writeBlend(mode)
			}
		}
		if normal && image.animated {
			w.u32([4]distribution{value(0), value(1), plain(8), plain(32)}, f.duration)
			if image.timecodes {
				w.bits(0xdeadbeef, 32)
			}
		}
		last := f.last && normal
		if normal {
			w.flag(last)
		}
		if f.kind != frameLowFrequency && !last {
			w.bits(uint64(f.savedReference), 2)
		}
		referenced := !last && f.kind != frameLowFrequency && (f.duration == 0 || f.savedReference != 0)
		if f.kind == frameReferenceOnly || (referenced && f.blendMode == 0 && fullFrame && normal) {
			w.flag(false)
		}
		w.text(f.name)
		switch {
		case f.defaultFilter:
			w.flag(true)
		case !f.loopFilter:
			w.flag(false)
			w.flag(false)
			w.bits(0, 2)
			w.u64(0)
		default:
			w.flag(false)
		}
		if f.loopFilter && !f.defaultFilter {
			w.flag(true)
			w.flag(false)
			w.bits(2, 2)
			if !f.modular {
				w.flag(false)
			}
			w.flag(false)
			w.flag(true)
			if !f.modular {
				w.bits(0, 16)
			}
			w.bits(0, 48)
			if f.modular {
				w.bits(0, 16)
			}
			w.u64(0)
		}
		w.u64(0)
		header = frameHeader{
			kind: f.kind, modular: f.modular, jpegUpsampling: f.jpegUpsampling, upsampling: upsampling, groupSizeShift: f.groupSizeShift,
			passes: passes, lowFreqLevel: f.lowFreqLevel, frameWidth: f.frameWidth, frameHeight: f.frameHeight,
		}
		if !f.modular {
			header.groupSizeShift = 1
		}
	}
	entries := f.entries
	if entries == nil {
		entries = make([]uint32, header.tableEntries(imageWidth, imageHeight))
		for i := range entries {
			entries[i] = 1
		}
	}
	w.flag(f.permuted)
	w.align()
	for _, entry := range entries {
		w.u32(tableChoice, entry)
	}
	w.align()
	out := append([]byte(nil), w.data...)
	var total uint32
	for _, entry := range entries {
		total += entry
	}
	data := f.data
	if data == nil {
		data = make([]byte, total)
	}
	return append(out, data...)
}

func stillFrame(t testing.TB, data []byte) (header imageHeader, tocEntries []uint32, payload []byte) {
	t.Helper()
	header, err := parseImageHeader(data)
	if err != nil {
		t.Fatal(err)
	}
	start := (header.endBit + 7) / 8
	r := newBitReader(data[start:])
	readFrameHeader(r, header, header.width, header.height)
	r.flag()
	r.alignToByte()
	count := baselineEntries(t, data[start:], header)
	for i := 0; i < count; i++ {
		tocEntries = append(tocEntries, r.u32(tableChoice))
	}
	r.alignToByte()
	return header, tocEntries, data[start+r.position/8:]
}

func baselineEntries(t testing.TB, data []byte, header imageHeader) int {
	t.Helper()
	r := newBitReader(data)
	frame := readFrameHeader(r, header, header.width, header.height)
	return int(frame.tableEntries(header.width, header.height))
}

type clipSpec struct {
	duration uint32
	delayed  bool
	hidden   bool
}

func encodeLossless(t testing.TB, width, height int, seed uint64) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := gjxl.Encode(&buf, testimg.Noise(width, height, seed, false), gjxl.EncodeOptions{Lossless: true, Effort: 1}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

type animationSpec struct {
	width, height int
	orientation   uint32
	loops         uint32
	ticks         [2]uint32
	clips         []clipSpec
	preview       bool
}

func buildAnimation(t testing.TB, spec animationSpec) []byte {
	t.Helper()
	header := baseHeader(uint32(spec.width), uint32(spec.height))
	header.animated = true
	header.loops = spec.loops
	if spec.ticks[0] != 0 {
		header.ticksNumerator, header.ticksDenom = spec.ticks[0], spec.ticks[1]
	}
	if spec.orientation != 0 {
		header.orientation = spec.orientation
	}
	header.preview = spec.preview
	header.previewSize = 16
	header.modular16 = true
	headerBits, endBit := header.write()
	out := &bitWriter{data: append([]byte(nil), headerBits...), position: endBit}
	out.align()
	stream := out.data
	if spec.preview {
		still := encodeLossless(t, 16, 16, 99)
		_, entries, payload := stillFrame(t, still)
		frame := frameSpec{modular: true, groupSizeShift: 1, last: true, entries: entries, data: payload[:sumOf(entries)]}
		stream = append(stream, frame.write(header, 16, 16)...)
	}
	for i, clip := range spec.clips {
		still := encodeLossless(t, spec.width, spec.height, uint64(i+1))
		_, entries, payload := stillFrame(t, still)
		frame := frameSpec{modular: true, groupSizeShift: 1, duration: clip.duration, last: i == len(spec.clips)-1, entries: entries, data: payload[:sumOf(entries)]}
		if clip.hidden {
			frame.duration = 0
			frame.savedReference = 1
		}
		stream = append(stream, frame.write(header, uint32(spec.width), uint32(spec.height))...)
	}
	return stream
}

func sumOf(entries []uint32) int {
	var total int
	for _, entry := range entries {
		total += int(entry)
	}
	return total
}

func boxed(parts ...[]byte) []byte {
	var out []byte
	for _, part := range parts {
		out = append(out, part...)
	}
	return out
}

func box(kind string, body ...[]byte) []byte {
	payload := boxed(body...)
	out := binary.BigEndian.AppendUint32(nil, uint32(8+len(payload)))
	out = append(out, kind...)
	return append(out, payload...)
}

func largeBox(kind string, body ...[]byte) []byte {
	payload := boxed(body...)
	out := binary.BigEndian.AppendUint32(nil, 1)
	out = append(out, kind...)
	out = binary.BigEndian.AppendUint64(out, uint64(16+len(payload)))
	return append(out, payload...)
}

func openEndedBox(kind string, body ...[]byte) []byte {
	out := binary.BigEndian.AppendUint32(nil, 0)
	out = append(out, kind...)
	return append(out, boxed(body...)...)
}

func ftypBox() []byte {
	return box("ftyp", []byte("jxl "), binary.BigEndian.AppendUint32(nil, 0), []byte("jxl "))
}

func contained(codestream []byte, extra ...[]byte) []byte {
	return boxed(containerSignature, ftypBox(), boxed(extra...), box("jxlc", codestream))
}

func partial(codestream []byte, parts int) [][]byte {
	size := (len(codestream) + parts - 1) / parts
	var boxes [][]byte
	for i := 0; i*size < len(codestream); i++ {
		end := min(len(codestream), (i+1)*size)
		index := uint32(i)
		if end == len(codestream) {
			index |= 1 << 31
		}
		boxes = append(boxes, box("jxlp", binary.BigEndian.AppendUint32(nil, index), codestream[i*size:end]))
	}
	return boxes
}

func cmp64(v, fallback uint64) uint64 {
	if v != 0 {
		return v
	}
	return fallback
}

func halfBits(v float64) uint64 {
	exponent := math.Floor(math.Log2(v))
	mantissa := math.Round((v/math.Ldexp(1, int(exponent)) - 1) * 1024)
	return uint64(int(exponent)+15)<<10 | uint64(mantissa)
}
