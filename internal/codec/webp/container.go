package webp

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"time"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/guard"
	"github.com/oops1/PixGo/internal/meta/exif"
)

const (
	riffHeaderSize       = 12
	chunkHeaderSize      = 8
	vp8xBodySize         = 10
	vp8HeaderSize        = 10
	vp8lHeaderSize       = 5
	animBodySize         = 6
	frameHeaderSize      = 16
	exifPrefixSize       = 6
	maxChunks            = 1 << 20
	contextCheckInterval = 256
	flagAnimation        = 0x02
	flagAlpha            = 0x10
	vp8lSignature        = 0x2f
	vp8StartCode0        = 0x9d
	vp8StartCode1        = 0x01
	vp8StartCode2        = 0x2a
	dimensionMask        = 0x3fff
)

type layout struct {
	width       int
	height      int
	hasAlpha    bool
	animated    bool
	lossless    bool
	frames      int
	loop        int
	firstDelay  time.Duration
	orientation codec.Orientation
}

func (l layout) info() codec.Info {
	depth := 24
	if l.hasAlpha {
		depth = 32
	}
	return codec.Info{
		Format:      codec.WebP,
		FormatName:  "WebP",
		Width:       l.width,
		Height:      l.height,
		Frames:      l.frames,
		Animated:    l.animated && l.frames > 1,
		HasAlpha:    l.hasAlpha,
		Orientation: l.orientation,
		BitDepth:    depth,
	}
}

func readUint24(b []byte) int {
	return int(b[0]) | int(b[1])<<8 | int(b[2])<<16
}

type scanner struct {
	r             io.ReaderAt
	size          int64
	end           int64
	found         layout
	extended      bool
	haveBitstream bool
	canvasWidth   int
	canvasHeight  int
}

func riffEnd(r io.ReaderAt, size int64) (int64, error) {
	if size < riffHeaderSize {
		return 0, guard.Corrupt("webp: %d bytes is too short", size)
	}
	var riff [riffHeaderSize]byte
	if err := guard.ReadAtInto(r, riff[:], 0, size); err != nil {
		return 0, err
	}
	if string(riff[:4]) != "RIFF" || string(riff[8:12]) != "WEBP" {
		return 0, guard.Corrupt("webp: not a RIFF WEBP container")
	}
	declared := int64(binary.LittleEndian.Uint32(riff[4:8]))
	if declared < 4 {
		return 0, guard.Corrupt("webp: RIFF size %d", declared)
	}
	return min(size, declared+8), nil
}

func walkChunks(ctx context.Context, r io.ReaderAt, size int64) (layout, error) {
	s := &scanner{r: r, size: size, found: layout{orientation: codec.OrientNormal}}
	end, err := riffEnd(r, size)
	if err != nil {
		return s.found, err
	}
	s.end = end
	if err := s.scan(ctx); err != nil {
		return s.found, err
	}
	return s.finish()
}

func (s *scanner) scan(ctx context.Context) error {
	position := int64(riffHeaderSize)
	for chunks := 0; ; chunks++ {
		if chunks >= maxChunks {
			return guard.Corrupt("webp: more than %d chunks", maxChunks)
		}
		if chunks%contextCheckInterval == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		if position+chunkHeaderSize > s.end {
			return nil
		}
		var head [chunkHeaderSize]byte
		if err := guard.ReadAtInto(s.r, head[:], position, s.size); err != nil {
			return err
		}
		length := int64(binary.LittleEndian.Uint32(head[4:8]))
		payload := position + chunkHeaderSize
		if payload+length > s.end {
			return nil
		}
		stop, err := s.visit(string(head[:4]), chunks == 0, payload, length)
		if err != nil || stop {
			return err
		}
		position = payload + length + length&1
	}
}

func (s *scanner) visit(id string, first bool, payload, length int64) (bool, error) {
	if first && id != "VP8X" && id != "VP8 " && id != "VP8L" {
		return true, guard.Corrupt("webp: first chunk is %q", id)
	}
	switch id {
	case "VP8X":
		if first {
			return false, s.visitExtended(payload, length)
		}
	case "VP8 ":
		return s.visitLossy(payload, length)
	case "VP8L":
		return s.visitLossless(payload, length)
	case "ALPH":
		s.found.hasAlpha = true
	case "ANIM":
		return false, s.visitAnimation(payload, length)
	case "ANMF":
		return false, s.visitFrame(payload, length)
	case "EXIF":
		s.visitExif(payload, length)
	}
	return false, nil
}

func (s *scanner) visitExtended(payload, length int64) error {
	if length < vp8xBodySize {
		return guard.Corrupt("webp: VP8X chunk of %d bytes", length)
	}
	var body [vp8xBodySize]byte
	if err := guard.ReadAtInto(s.r, body[:], payload, s.size); err != nil {
		return err
	}
	s.extended = true
	s.found.animated = body[0]&flagAnimation != 0
	s.found.hasAlpha = s.found.hasAlpha || body[0]&flagAlpha != 0
	s.canvasWidth = readUint24(body[4:7]) + 1
	s.canvasHeight = readUint24(body[7:10]) + 1
	return nil
}

func (s *scanner) visitLossy(payload, length int64) (bool, error) {
	if s.haveBitstream {
		return false, nil
	}
	if length < vp8HeaderSize {
		return true, guard.Corrupt("webp: VP8 chunk of %d bytes", length)
	}
	var body [vp8HeaderSize]byte
	if err := guard.ReadAtInto(s.r, body[:], payload, s.size); err != nil {
		return true, err
	}
	if body[0]&1 != 0 || body[3] != vp8StartCode0 || body[4] != vp8StartCode1 || body[5] != vp8StartCode2 {
		return true, guard.Corrupt("webp: bad VP8 key frame header")
	}
	width := int(binary.LittleEndian.Uint16(body[6:8]) & dimensionMask)
	height := int(binary.LittleEndian.Uint16(body[8:10]) & dimensionMask)
	return s.adoptBitstream(width, height)
}

func (s *scanner) visitLossless(payload, length int64) (bool, error) {
	if s.haveBitstream {
		return false, nil
	}
	if length < vp8lHeaderSize {
		return true, guard.Corrupt("webp: VP8L chunk of %d bytes", length)
	}
	var body [vp8lHeaderSize]byte
	if err := guard.ReadAtInto(s.r, body[:], payload, s.size); err != nil {
		return true, err
	}
	if body[0] != vp8lSignature {
		return true, guard.Corrupt("webp: bad VP8L signature")
	}
	bits := binary.LittleEndian.Uint32(body[1:5])
	if bits>>29 != 0 {
		return true, guard.Unsupported("webp: VP8L version %d", bits>>29)
	}
	s.found.lossless = true
	s.found.hasAlpha = s.found.hasAlpha || bits>>28&1 != 0
	return s.adoptBitstream(int(bits&dimensionMask)+1, int(bits>>14&dimensionMask)+1)
}

func (s *scanner) adoptBitstream(width, height int) (bool, error) {
	if width <= 0 || height <= 0 {
		return true, guard.Corrupt("webp: bitstream dimensions %dx%d", width, height)
	}
	if s.extended && !s.found.animated && (width != s.canvasWidth || height != s.canvasHeight) {
		return true, guard.Corrupt("webp: bitstream %dx%d does not match canvas %dx%d", width, height, s.canvasWidth, s.canvasHeight)
	}
	s.found.width, s.found.height = width, height
	s.haveBitstream = true
	return !s.extended, nil
}

func (s *scanner) visitAnimation(payload, length int64) error {
	if length < animBodySize {
		return guard.Corrupt("webp: ANIM chunk of %d bytes", length)
	}
	var body [animBodySize]byte
	if err := guard.ReadAtInto(s.r, body[:], payload, s.size); err != nil {
		return err
	}
	s.found.loop = int(binary.LittleEndian.Uint16(body[4:6]))
	return nil
}

func (s *scanner) visitFrame(payload, length int64) error {
	if length < frameHeaderSize {
		return guard.Corrupt("webp: ANMF chunk of %d bytes", length)
	}
	if s.found.frames == 0 {
		var body [frameHeaderSize]byte
		if err := guard.ReadAtInto(s.r, body[:], payload, s.size); err != nil {
			return err
		}
		s.found.firstDelay = frameDelay(readUint24(body[12:15]))
	}
	s.found.frames++
	return nil
}

func (s *scanner) visitExif(payload, length int64) {
	if s.found.orientation != codec.OrientNormal || length < 8 {
		return
	}
	base, window := payload, length
	var prefix [exifPrefixSize]byte
	if err := guard.ReadAtInto(s.r, prefix[:], payload, s.size); err == nil && bytes.Equal(prefix[:], []byte("Exif\x00\x00")) {
		base += exifPrefixSize
		window -= exifPrefixSize
	}
	if orientation, ok := exif.OrientationAt(s.r, base, window); ok {
		s.found.orientation = orientation
	}
}

func (s *scanner) finish() (layout, error) {
	found := s.found
	if s.extended {
		found.width, found.height = s.canvasWidth, s.canvasHeight
	}
	if found.animated {
		if found.frames < 1 {
			return found, guard.Corrupt("webp: animation without frames")
		}
		return found, nil
	}
	if !s.haveBitstream {
		return found, guard.Corrupt("webp: no image data")
	}
	found.frames = 1
	return found, nil
}
