package webp

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/meta"
	"github.com/oops1/PixGo/internal/meta/exif"
)

const (
	flagICC   = 0x20
	flagEXIF  = 0x08
	flagXMP   = 0x04
	maxRIFF   = 1<<32 - 1
	alphaBit  = 0x10
	vp8lAlpha = 1 << 28
)

type riffChunk struct {
	kind string
	body []byte
}

func readChunks(data []byte) ([]riffChunk, error) {
	if len(data) < riffHeaderSize || string(data[:4]) != "RIFF" || string(data[8:12]) != "WEBP" {
		return nil, errors.New("webp: encoder produced no RIFF container")
	}
	var chunks []riffChunk
	pos := riffHeaderSize
	for pos+chunkHeaderSize <= len(data) {
		n := int(binary.LittleEndian.Uint32(data[pos+4:]))
		start := pos + chunkHeaderSize
		if n > len(data)-start {
			return nil, errors.New("webp: encoder produced a truncated chunk")
		}
		chunks = append(chunks, riffChunk{kind: string(data[pos : pos+4]), body: data[start : start+n]})
		pos = start + n + n&1
	}
	return chunks, nil
}

func appendChunk(dst []byte, kind string, body []byte) []byte {
	dst = append(dst, kind...)
	dst = binary.LittleEndian.AppendUint32(dst, uint32(len(body)))
	dst = append(dst, body...)
	if len(body)&1 == 1 {
		dst = append(dst, 0)
	}
	return dst
}

func putUint24(dst []byte, v int) []byte {
	return append(dst, byte(v), byte(v>>8), byte(v>>16))
}

func hasAlpha(chunks []riffChunk) bool {
	for _, c := range chunks {
		switch c.kind {
		case "VP8X":
			if len(c.body) >= 1 && c.body[0]&alphaBit != 0 {
				return true
			}
		case "ALPH":
			return true
		case "VP8L":
			if len(c.body) >= 5 && c.body[0] == vp8lSignature && binary.LittleEndian.Uint32(c.body[1:5])&vp8lAlpha != 0 {
				return true
			}
		}
	}
	return false
}

func addMetadata(data []byte, m codec.Metadata, width, height int) ([]byte, error) {
	m, _ = meta.Fit(m, codec.WebP)
	m.EXIF = exif.StripExifHeader(m.EXIF)
	if m.Empty() {
		return data, nil
	}
	chunks, err := readChunks(data)
	if err != nil {
		return nil, err
	}
	var flags byte
	if hasAlpha(chunks) {
		flags |= alphaBit
	}
	if len(m.ICC) > 0 {
		flags |= flagICC
	}
	if len(m.EXIF) > 0 {
		flags |= flagEXIF
	}
	if len(m.XMP) > 0 {
		flags |= flagXMP
	}
	canvas := []byte{flags, 0, 0, 0}
	canvas = putUint24(canvas, width-1)
	canvas = putUint24(canvas, height-1)
	out := []byte("RIFF\x00\x00\x00\x00WEBP")
	out = appendChunk(out, "VP8X", canvas)
	if len(m.ICC) > 0 {
		out = appendChunk(out, "ICCP", m.ICC)
	}
	for _, c := range chunks {
		switch c.kind {
		case "VP8X", "ICCP", "EXIF", "XMP ":
			continue
		}
		out = appendChunk(out, c.kind, c.body)
	}
	if len(m.EXIF) > 0 {
		out = appendChunk(out, "EXIF", m.EXIF)
	}
	if len(m.XMP) > 0 {
		out = appendChunk(out, "XMP ", m.XMP)
	}
	if len(out)-8 > maxRIFF {
		return nil, fmt.Errorf("webp: %d bytes exceed the RIFF limit", len(out))
	}
	binary.LittleEndian.PutUint32(out[4:], uint32(len(out)-8))
	return out, nil
}
