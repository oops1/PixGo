package jxl

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"sort"

	"github.com/oops1/PixGo/internal/codec/internal/guard"
)

const (
	boxHeaderSize      = 8
	largeBoxHeaderSize = 16
	maxContainerBoxes  = 1 << 16
	supportedLevel     = 5
	partIndexMask      = 0x7fffffff
)

var (
	codestreamSignature = []byte{0xff, 0x0a}
	containerSignature  = []byte("\x00\x00\x00\x0cJXL \x0d\x0a\x87\x0a")
)

type segment struct {
	file   int64
	length int64
	start  int64
}

type codestream struct {
	source   io.ReaderAt
	segments []segment
	size     int64
	level    int
	scratch  []byte
}

func (s *codestream) readInto(buf []byte, offset int64) error {
	length := int64(len(buf))
	if offset < 0 || offset > s.size || length > s.size-offset {
		return guard.Corrupt("jxl: codestream range %d+%d outside %d bytes", offset, length, s.size)
	}
	index := sort.Search(len(s.segments), func(i int) bool { return s.segments[i].start+s.segments[i].length > offset })
	for done := int64(0); done < length; index++ {
		if index >= len(s.segments) {
			return guard.Corrupt("jxl: codestream range %d+%d outside the parts found", offset, length)
		}
		part := s.segments[index]
		inner := offset + done - part.start
		take := min(length-done, part.length-inner)
		if err := guard.ReadAtInto(s.source, buf[done:done+take], part.file+inner, part.file+part.length); err != nil {
			return err
		}
		done += take
	}
	return nil
}

func (s *codestream) read(offset, length int64) ([]byte, error) {
	if length < 0 {
		return nil, guard.Corrupt("jxl: codestream range %d+%d outside %d bytes", offset, length, s.size)
	}
	out := make([]byte, length)
	if err := s.readInto(out, offset); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *codestream) window(offset, length int64) ([]byte, error) {
	if length < 0 {
		return nil, guard.Corrupt("jxl: codestream range %d+%d outside %d bytes", offset, length, s.size)
	}
	if int64(cap(s.scratch)) < length {
		s.scratch = make([]byte, length)
	}
	buf := s.scratch[:length]
	if err := s.readInto(buf, offset); err != nil {
		return nil, err
	}
	return buf, nil
}

func readBoxHeader(r io.ReaderAt, position, size int64) (kind string, payload, total int64, err error) {
	var head [largeBoxHeaderSize]byte
	window := min(int64(len(head)), size-position)
	if window < boxHeaderSize {
		return "", 0, 0, guard.Corrupt("jxl: truncated box header at %d", position)
	}
	if err := guard.ReadAtInto(r, head[:window], position, size); err != nil {
		return "", 0, 0, err
	}
	total = int64(binary.BigEndian.Uint32(head[:4]))
	kind = string(head[4:8])
	headLen := int64(boxHeaderSize)
	switch total {
	case 1:
		if window < largeBoxHeaderSize {
			return "", 0, 0, guard.Corrupt("jxl: truncated large box header at %d", position)
		}
		large := binary.BigEndian.Uint64(head[8:16])
		if large > uint64(size) {
			return "", 0, 0, guard.Corrupt("jxl: box %q of %d bytes", kind, large)
		}
		total = int64(large)
		headLen = largeBoxHeaderSize
	case 0:
		total = size - position
	}
	if total < headLen || total > size-position {
		return "", 0, 0, guard.Corrupt("jxl: box %q of %d bytes does not fit the file", kind, total)
	}
	return kind, position + headLen, total - headLen, nil
}

type codestreamPart struct {
	index uint32
	last  bool
	seg   segment
}

func openContainer(ctx context.Context, r io.ReaderAt, size int64) (*codestream, error) {
	stream := &codestream{source: r, level: supportedLevel}
	var parts []codestreamPart
	seenFtyp, seenWhole := false, false
	position := int64(len(containerSignature))
	for boxes := 0; position < size; boxes++ {
		if boxes >= maxContainerBoxes {
			return nil, guard.Corrupt("jxl: more than %d container boxes", maxContainerBoxes)
		}
		if boxes%contextCheckEvery == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		kind, payload, length, err := readBoxHeader(r, position, size)
		if err != nil {
			return nil, err
		}
		if !seenFtyp && kind != "ftyp" {
			return nil, guard.Corrupt("jxl: first container box is %q, not ftyp", kind)
		}
		switch kind {
		case "ftyp":
			body, err := guard.ReadAt(r, payload, min(length, 16), size)
			if err != nil {
				return nil, err
			}
			if seenFtyp || len(body) < 8 || string(body[:4]) != "jxl " || binary.BigEndian.Uint32(body[4:8]) > 1 {
				return nil, guard.Corrupt("jxl: bad ftyp box")
			}
			seenFtyp = true
		case "jxll":
			body, err := guard.ReadAt(r, payload, min(length, 1), size)
			if err != nil || len(body) < 1 {
				return nil, guard.Corrupt("jxl: bad jxll box")
			}
			stream.level = int(body[0])
		case "jxlc":
			if seenWhole || len(parts) > 0 {
				return nil, guard.Corrupt("jxl: more than one codestream box")
			}
			seenWhole = true
			parts = append(parts, codestreamPart{seg: segment{file: payload, length: length}})
		case "jxlp":
			if seenWhole || length < 4 {
				return nil, guard.Corrupt("jxl: bad partial codestream box")
			}
			var index [4]byte
			if err := guard.ReadAtInto(r, index[:], payload, size); err != nil {
				return nil, err
			}
			word := binary.BigEndian.Uint32(index[:])
			parts = append(parts, codestreamPart{
				index: word & partIndexMask,
				last:  word&^partIndexMask != 0,
				seg:   segment{file: payload + 4, length: length - 4},
			})
		}
		position = payload + length
	}
	if !seenFtyp || len(parts) == 0 {
		return nil, guard.Corrupt("jxl: container holds no codestream")
	}
	sort.SliceStable(parts, func(i, j int) bool { return parts[i].index < parts[j].index })
	for i, part := range parts {
		if !seenWhole && part.index != uint32(i) {
			return nil, guard.Corrupt("jxl: codestream part %d is missing", i)
		}
		if !seenWhole && part.last != (i == len(parts)-1) {
			return nil, guard.Corrupt("jxl: codestream part %d is misplaced as the last one", i)
		}
		part.seg.start = stream.size
		stream.segments = append(stream.segments, part.seg)
		stream.size += part.seg.length
	}
	return stream, nil
}

func openCodestream(ctx context.Context, r io.ReaderAt, size int64) (*codestream, error) {
	var head [12]byte
	window := min(int64(len(head)), size)
	if err := guard.ReadAtInto(r, head[:window], 0, size); err != nil {
		return nil, err
	}
	switch {
	case bytes.HasPrefix(head[:window], containerSignature):
		return openContainer(ctx, r, size)
	case bytes.HasPrefix(head[:window], codestreamSignature):
		return &codestream{source: r, size: size, level: supportedLevel, segments: []segment{{file: 0, length: size}}}, nil
	}
	return nil, guard.Corrupt("jxl: no JPEG XL signature")
}
