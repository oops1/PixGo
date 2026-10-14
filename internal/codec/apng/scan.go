package apng

import (
	"bytes"
	"context"
	"encoding/binary"
	"hash/crc32"
	"io"
	"time"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/guard"
	"github.com/oops1/PixGo/internal/meta/exif"
)

const (
	signatureSize    = 8
	chunkHeaderSize  = 8
	chunkTrailerSize = 4
	headerChunkSize  = 13
	controlSize      = 8
	frameControlSize = 26
	sequenceSize     = 4
	maxChunkLength   = 1<<31 - 1
	maxChunks        = 1 << 22
	maxSmallChunk    = 768
	windowSize       = 16 << 10
	contextInterval  = 256
	firstChunkOffset = signatureSize + chunkHeaderSize + headerChunkSize + chunkTrailerSize
)

const (
	disposeBackground = 1
	disposePrevious   = 2
	blendOver         = 1
)

const (
	minFrameDelay     = 10 * time.Millisecond
	defaultDelayDenom = 100
)

var signature = []byte("\x89PNG\r\n\x1a\n")

type frame struct {
	x, y          int
	width, height int
	delay         time.Duration
	dispose       byte
	blend         byte
	fromIDAT      bool
	dataStart     int64
}

type layout struct {
	width, height       int
	bitDepth            byte
	colorType           byte
	interlace           byte
	palette             []byte
	transparency        []byte
	numFrames           uint32
	numPlays            uint32
	frameCount          int
	frames              []frame
	exposesTransparency bool
	orientation         codec.Orientation
}

func (l *layout) bitsPerPixel() int {
	depth := int(l.bitDepth)
	switch l.colorType {
	case 2:
		return 3 * depth
	case 4:
		return 2 * depth
	case 6:
		return 4 * depth
	}
	return depth
}

func (l *layout) hasAlpha() bool {
	return l.colorType == 4 || l.colorType == 6 || l.transparency != nil || l.exposesTransparency
}

func validHeader(bitDepth, colorType, compression, filter, interlace uint8) bool {
	if compression != 0 || filter != 0 || interlace > 1 {
		return false
	}
	switch colorType {
	case 0:
		return bitDepth == 1 || bitDepth == 2 || bitDepth == 4 || bitDepth == 8 || bitDepth == 16
	case 2, 4, 6:
		return bitDepth == 8 || bitDepth == 16
	case 3:
		return bitDepth == 1 || bitDepth == 2 || bitDepth == 4 || bitDepth == 8
	}
	return false
}

func frameDelay(numerator, denominator uint16) time.Duration {
	if denominator == 0 {
		denominator = defaultDelayDenom
	}
	delay := time.Duration(numerator) * time.Second / time.Duration(denominator)
	if delay < minFrameDelay {
		return minFrameDelay
	}
	return delay
}

type window struct {
	ctx   context.Context
	r     io.ReaderAt
	size  int64
	start int64
	buf   []byte
}

func newWindow(ctx context.Context, r io.ReaderAt, size int64) *window {
	return &window{ctx: ctx, r: r, size: size}
}

func (w *window) at(position int64, n int) ([]byte, error) {
	if n < 0 || n > windowSize {
		return nil, guard.Corrupt("apng: window request of %d bytes", n)
	}
	if position < 0 || position+int64(n) > w.size {
		return nil, guard.Corrupt("apng: read of %d bytes at %d outside %d byte file", n, position, w.size)
	}
	if position < w.start || position+int64(n) > w.start+int64(len(w.buf)) {
		if err := w.ctx.Err(); err != nil {
			return nil, err
		}
		want := int64(windowSize)
		if want > w.size-position {
			want = w.size - position
		}
		if int64(cap(w.buf)) < want {
			w.buf = make([]byte, want)
		}
		w.buf = w.buf[:want]
		if err := guard.ReadAtInto(w.r, w.buf, position, w.size); err != nil {
			w.buf = w.buf[:0]
			return nil, err
		}
		w.start = position
	}
	offset := position - w.start
	return w.buf[offset : offset+int64(n)], nil
}

func checksumOf(kind, body []byte) uint32 {
	return crc32.Update(crc32.Update(0, crc32.IEEETable, kind), crc32.IEEETable, body)
}

func (w *window) body(position int64, length int) ([]byte, error) {
	if length > maxSmallChunk {
		return nil, guard.Corrupt("apng: chunk of %d bytes where at most %d are expected", length, maxSmallChunk)
	}
	whole, err := w.at(position, chunkHeaderSize+length+chunkTrailerSize)
	if err != nil {
		return nil, err
	}
	body := whole[chunkHeaderSize : chunkHeaderSize+length]
	if binary.BigEndian.Uint32(whole[chunkHeaderSize+length:]) != checksumOf(whole[4:8], body) {
		return nil, guard.Corrupt("apng: %s chunk at %d has a bad checksum", whole[4:8], position)
	}
	return body, nil
}

type scanner struct {
	r             io.ReaderAt
	w             *window
	lim           codec.Limits
	keep          bool
	l             *layout
	seenControl   bool
	seenData      bool
	dataEnded     bool
	seenEnd       bool
	nextSequence  uint32
	frameOpen     bool
	frameHasData  bool
	frameFromIDAT bool
	lastDispose   byte
}

func scan(ctx context.Context, r io.ReaderAt, size int64, lim codec.Limits, keep bool) (*layout, error) {
	s := &scanner{
		r:    r,
		w:    newWindow(ctx, r, size),
		lim:  lim,
		keep: keep,
		l:    &layout{orientation: codec.OrientNormal},
	}
	if err := s.header(size); err != nil {
		return nil, err
	}
	position := int64(firstChunkOffset)
	for chunks := 0; !s.seenEnd; chunks++ {
		if chunks >= maxChunks {
			return nil, guard.TooLarge("apng: more than %d chunks", maxChunks)
		}
		if chunks%contextInterval == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		next, err := s.chunk(position, size)
		if err != nil {
			return nil, err
		}
		position = next
	}
	if err := s.finish(); err != nil {
		return nil, err
	}
	return s.l, nil
}

func (s *scanner) header(size int64) error {
	if size < firstChunkOffset {
		return guard.Corrupt("apng: %d bytes is too short", size)
	}
	lead, err := s.w.at(0, firstChunkOffset)
	if err != nil {
		return err
	}
	if !bytes.Equal(lead[:signatureSize], signature) {
		return guard.Corrupt("apng: bad signature")
	}
	if binary.BigEndian.Uint32(lead[8:12]) != headerChunkSize || string(lead[12:16]) != "IHDR" {
		return guard.Corrupt("apng: IHDR must come first")
	}
	if binary.BigEndian.Uint32(lead[29:33]) != checksumOf(lead[12:16], lead[16:29]) {
		return guard.Corrupt("apng: IHDR has a bad checksum")
	}
	fields := lead[16:29]
	width := binary.BigEndian.Uint32(fields[0:4])
	height := binary.BigEndian.Uint32(fields[4:8])
	if width == 0 || height == 0 || width > maxChunkLength || height > maxChunkLength {
		return guard.Corrupt("apng: dimensions %dx%d", width, height)
	}
	if !validHeader(fields[8], fields[9], fields[10], fields[11], fields[12]) {
		return guard.Corrupt("apng: invalid IHDR (depth %d, colour type %d)", fields[8], fields[9])
	}
	if err := s.lim.Check(int(width), int(height)); err != nil {
		return err
	}
	s.l.width, s.l.height = int(width), int(height)
	s.l.bitDepth, s.l.colorType, s.l.interlace = fields[8], fields[9], fields[12]
	return nil
}

func (s *scanner) chunk(position, size int64) (int64, error) {
	head, err := s.w.at(position, chunkHeaderSize)
	if err != nil {
		return 0, guard.Corrupt("apng: chunk header at %d: file is truncated", position)
	}
	length := int64(binary.BigEndian.Uint32(head[0:4]))
	kind := string(head[4:8])
	next := position + chunkHeaderSize + length + chunkTrailerSize
	if length > maxChunkLength || next > size {
		return 0, guard.Corrupt("apng: %s chunk at %d runs past the end of the file", kind, position)
	}
	if kind != "IDAT" && s.seenData {
		s.dataEnded = true
	}
	switch kind {
	case "IEND":
		s.seenEnd = true
	case "acTL":
		err = s.control(position, length)
	case "fcTL":
		err = s.frameControl(position, length, next)
	case "IDAT":
		err = s.imageData()
	case "fdAT":
		err = s.frameData(position, length)
	case "PLTE":
		err = s.paletteChunk(position, length)
	case "tRNS":
		err = s.transparencyChunk(position, length)
	case "eXIf":
		if s.l.orientation == codec.OrientNormal {
			if orientation, ok := exif.OrientationAt(s.r, position+chunkHeaderSize, length); ok {
				s.l.orientation = orientation
			}
		}
	}
	return next, err
}

func (s *scanner) control(position, length int64) error {
	if s.seenData || s.seenControl {
		return nil
	}
	if length != controlSize {
		return guard.Corrupt("apng: acTL of %d bytes", length)
	}
	body, err := s.w.body(position, int(length))
	if err != nil {
		return err
	}
	frames := binary.BigEndian.Uint32(body[0:4])
	if frames == 0 {
		return guard.Corrupt("apng: acTL declares no frames")
	}
	if int64(frames) > int64(s.lim.MaxFrames) {
		return guard.TooLarge("apng: %d frames exceed limit %d", frames, s.lim.MaxFrames)
	}
	s.l.numFrames = frames
	s.l.numPlays = binary.BigEndian.Uint32(body[4:8])
	s.seenControl = true
	return nil
}

func (s *scanner) frameControl(position, length, next int64) error {
	if !s.seenControl {
		return guard.Corrupt("apng: fcTL before acTL")
	}
	if length != frameControlSize {
		return guard.Corrupt("apng: fcTL of %d bytes", length)
	}
	if s.frameOpen && !s.frameHasData {
		return guard.Corrupt("apng: frame %d has no image data", s.l.frameCount-1)
	}
	body, err := s.w.body(position, int(length))
	if err != nil {
		return err
	}
	if sequence := binary.BigEndian.Uint32(body[0:4]); sequence != s.nextSequence {
		return guard.Corrupt("apng: fcTL sequence number %d, want %d", sequence, s.nextSequence)
	}
	s.nextSequence++
	f := frame{
		width:     clampInt(binary.BigEndian.Uint32(body[4:8])),
		height:    clampInt(binary.BigEndian.Uint32(body[8:12])),
		x:         clampInt(binary.BigEndian.Uint32(body[12:16])),
		y:         clampInt(binary.BigEndian.Uint32(body[16:20])),
		delay:     frameDelay(binary.BigEndian.Uint16(body[20:22]), binary.BigEndian.Uint16(body[22:24])),
		dispose:   body[24],
		blend:     body[25],
		fromIDAT:  !s.seenData,
		dataStart: next,
	}
	if f.width <= 0 || f.height <= 0 {
		return guard.Corrupt("apng: frame of %dx%d", f.width, f.height)
	}
	if int64(f.x)+int64(f.width) > int64(s.l.width) || int64(f.y)+int64(f.height) > int64(s.l.height) {
		return guard.Corrupt("apng: frame %dx%d at (%d,%d) leaves the %dx%d canvas", f.width, f.height, f.x, f.y, s.l.width, s.l.height)
	}
	if f.dispose > disposePrevious || f.blend > blendOver {
		return guard.Corrupt("apng: dispose %d, blend %d", f.dispose, f.blend)
	}
	coversCanvas := f.x == 0 && f.y == 0 && f.width == s.l.width && f.height == s.l.height
	if f.fromIDAT && !coversCanvas {
		return guard.Corrupt("apng: the default image frame must cover the canvas")
	}
	if s.l.frameCount >= int(s.l.numFrames) {
		return guard.Corrupt("apng: more fcTL chunks than the %d frames in acTL", s.l.numFrames)
	}
	if s.l.frameCount == 0 && !coversCanvas {
		s.l.exposesTransparency = true
	}
	if s.l.frameCount > 0 && s.lastDispose == disposeBackground {
		s.l.exposesTransparency = true
	}
	s.lastDispose = f.dispose
	s.l.frameCount++
	s.frameOpen, s.frameHasData, s.frameFromIDAT = true, false, f.fromIDAT
	if s.keep {
		s.l.frames = append(s.l.frames, f)
	}
	return nil
}

func clampInt(v uint32) int {
	return int(min(v, maxChunkLength))
}

func (s *scanner) imageData() error {
	if !s.seenControl {
		return guard.Corrupt("apng: image data before acTL")
	}
	if s.dataEnded {
		return guard.Corrupt("apng: IDAT chunks are not consecutive")
	}
	s.seenData = true
	if s.frameOpen && s.frameFromIDAT {
		s.frameHasData = true
	}
	return nil
}

func (s *scanner) frameData(position, length int64) error {
	if !s.frameOpen {
		return guard.Corrupt("apng: fdAT outside a frame")
	}
	if s.frameFromIDAT {
		return guard.Corrupt("apng: fdAT for the frame stored in IDAT")
	}
	if length < sequenceSize {
		return guard.Corrupt("apng: fdAT of %d bytes", length)
	}
	raw, err := s.w.at(position+chunkHeaderSize, sequenceSize)
	if err != nil {
		return err
	}
	if sequence := binary.BigEndian.Uint32(raw); sequence != s.nextSequence {
		return guard.Corrupt("apng: fdAT sequence number %d, want %d", sequence, s.nextSequence)
	}
	s.nextSequence++
	if length > sequenceSize {
		s.frameHasData = true
	}
	return nil
}

func (s *scanner) paletteChunk(position, length int64) error {
	if s.seenData || s.l.palette != nil {
		return nil
	}
	if length < 3 || length%3 != 0 || length > maxSmallChunk {
		return guard.Corrupt("apng: PLTE of %d bytes", length)
	}
	body, err := s.w.body(position, int(length))
	if err != nil {
		return err
	}
	s.l.palette = append([]byte(nil), body...)
	return nil
}

func (s *scanner) transparencyChunk(position, length int64) error {
	if s.seenData || s.l.transparency != nil {
		return nil
	}
	body, err := s.w.body(position, int(length))
	if err != nil {
		return err
	}
	s.l.transparency = append(make([]byte, 0, len(body)), body...)
	return nil
}

func (s *scanner) finish() error {
	switch {
	case !s.seenControl:
		return guard.Corrupt("apng: no acTL chunk")
	case !s.seenData:
		return guard.Corrupt("apng: no image data")
	case s.frameOpen && !s.frameHasData:
		return guard.Corrupt("apng: frame %d has no image data", s.l.frameCount-1)
	case s.l.frameCount != int(s.l.numFrames):
		return guard.Corrupt("apng: acTL declares %d frames, the file has %d", s.l.numFrames, s.l.frameCount)
	}
	return nil
}
