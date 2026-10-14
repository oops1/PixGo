package apng

import (
	"context"
	"encoding/binary"
	"hash/crc32"
	"io"
	"slices"

	"github.com/oops1/PixGo/internal/codec/internal/guard"
)

func appendChunk(out []byte, kind string, body []byte) []byte {
	out = binary.BigEndian.AppendUint32(out, uint32(len(body)))
	start := len(out)
	out = append(out, kind...)
	out = append(out, body...)
	return binary.BigEndian.AppendUint32(out, crc32.ChecksumIEEE(out[start:]))
}

func (l *layout) framePrefix(width, height int) []byte {
	out := append(make([]byte, 0, 128), signature...)
	header := binary.BigEndian.AppendUint32(nil, uint32(width))
	header = binary.BigEndian.AppendUint32(header, uint32(height))
	header = append(header, l.bitDepth, l.colorType, 0, 0, l.interlace)
	out = appendChunk(out, "IHDR", header)
	if l.palette != nil {
		out = appendChunk(out, "PLTE", l.palette)
	}
	if l.transparency != nil {
		out = appendChunk(out, "tRNS", l.transparency)
	}
	return out
}

func (l *layout) frameStream(ctx context.Context, r io.ReaderAt, size int64, f frame) ([]byte, error) {
	out := l.framePrefix(f.width, f.height)
	lengthAt := len(out)
	out = append(out, 0, 0, 0, 0, 'I', 'D', 'A', 'T')
	dataAt := len(out)
	wanted := "fdAT"
	if f.fromIDAT {
		wanted = "IDAT"
	}
	w := newWindow(ctx, r, size)
	position := f.dataStart
walk:
	for chunks := 0; ; chunks++ {
		if chunks >= maxChunks {
			return nil, guard.TooLarge("apng: more than %d chunks", maxChunks)
		}
		if chunks%contextInterval == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		head, err := w.at(position, chunkHeaderSize)
		if err != nil {
			return nil, guard.Corrupt("apng: frame data at %d: file is truncated", position)
		}
		length := int64(binary.BigEndian.Uint32(head[0:4]))
		kind := string(head[4:8])
		next := position + chunkHeaderSize + length + chunkTrailerSize
		if length > maxChunkLength || next > size {
			return nil, guard.Corrupt("apng: %s chunk at %d runs past the end of the file", kind, position)
		}
		switch {
		case kind == "fcTL" || kind == "IEND":
			break walk
		case kind == wanted:
			if kind == "fdAT" && length < sequenceSize {
				return nil, guard.Corrupt("apng: fdAT of %d bytes", length)
			}
			if int64(len(out)-dataAt)+length > maxChunkLength {
				return nil, guard.TooLarge("apng: frame data exceeds %d bytes", maxChunkLength)
			}
			out = slices.Grow(out, int(length)+chunkTrailerSize)
			raw := out[len(out) : len(out)+int(length)+chunkTrailerSize]
			if err := guard.ReadAtInto(r, raw, position+chunkHeaderSize, size); err != nil {
				return nil, err
			}
			body := raw[:length]
			if binary.BigEndian.Uint32(raw[length:]) != checksumOf(head[4:8], body) {
				return nil, guard.Corrupt("apng: %s chunk at %d has a bad checksum", kind, position)
			}
			if kind == "fdAT" {
				body = body[sequenceSize:]
				copy(raw, body)
			}
			out = out[:len(out)+len(body)]
		}
		position = next
	}
	binary.BigEndian.PutUint32(out[lengthAt:], uint32(len(out)-dataAt))
	out = binary.BigEndian.AppendUint32(out, crc32.ChecksumIEEE(out[lengthAt+4:]))
	return appendChunk(out, "IEND", nil), nil
}
