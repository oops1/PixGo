package raw

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"

	"github.com/oops1/PixGo/internal/codec/internal/guard"
)

const (
	maxJPEGSegments = 256
	carveBlock      = 256 << 10
	maxCarveBytes   = 16 << 20
	maxCarveHits    = 512
)

type jpegHead struct {
	width, height int
	components    int
}

type jpegStatus uint8

const (
	jpegOK jpegStatus = iota
	jpegNotJPEG
	jpegUnsupported
	jpegOutOfRange
)

var jpegStart = []byte{0xff, 0xd8, 0xff}

func isSupportedFrame(marker byte) bool {
	return marker == 0xc0 || marker == 0xc1 || marker == 0xc2
}

func isOtherFrame(marker byte) bool {
	switch marker {
	case 0xc3, 0xc5, 0xc6, 0xc7, 0xc9, 0xca, 0xcb, 0xcd, 0xce, 0xcf:
		return true
	}
	return false
}

func probeJPEG(r io.ReaderAt, size, offset, length int64) (jpegHead, jpegStatus) {
	end := size
	if length > 0 {
		if length > size-offset || offset > size {
			return jpegHead{}, jpegOutOfRange
		}
		end = offset + length
	}
	if offset < 0 || offset+int64(len(jpegStart)) > end {
		return jpegHead{}, jpegOutOfRange
	}
	var lead [3]byte
	if err := guard.ReadAtInto(r, lead[:], offset, size); err != nil {
		return jpegHead{}, jpegOutOfRange
	}
	if !bytes.Equal(lead[:], jpegStart) {
		return jpegHead{}, jpegNotJPEG
	}
	position := offset + 2
	for segments := 0; segments < maxJPEGSegments; segments++ {
		var marker [4]byte
		available := end - position
		if available < 2 {
			return jpegHead{}, jpegOutOfRange
		}
		take := int64(len(marker))
		if available < take {
			take = available
		}
		if err := guard.ReadAtInto(r, marker[:take], position, size); err != nil {
			return jpegHead{}, jpegOutOfRange
		}
		if marker[0] != 0xff {
			return jpegHead{}, jpegNotJPEG
		}
		code := marker[1]
		switch {
		case code == 0xff:
			position++
			continue
		case code == 0x01 || code == 0xd8 || (code >= 0xd0 && code <= 0xd7):
			position += 2
			continue
		case code == 0xd9 || code == 0xda:
			return jpegHead{}, jpegNotJPEG
		case take < 4:
			return jpegHead{}, jpegOutOfRange
		}
		segment := int64(binary.BigEndian.Uint16(marker[2:4]))
		if segment < 2 {
			return jpegHead{}, jpegNotJPEG
		}
		if isOtherFrame(code) {
			return jpegHead{}, jpegUnsupported
		}
		if isSupportedFrame(code) {
			return readFrameHeader(r, size, position, segment)
		}
		position += 2 + segment
	}
	return jpegHead{}, jpegNotJPEG
}

func readFrameHeader(r io.ReaderAt, size, position, segment int64) (jpegHead, jpegStatus) {
	if segment < 8 {
		return jpegHead{}, jpegNotJPEG
	}
	var body [6]byte
	if err := guard.ReadAtInto(r, body[:], position+4, size); err != nil {
		return jpegHead{}, jpegOutOfRange
	}
	precision := body[0]
	height := int(binary.BigEndian.Uint16(body[1:3]))
	width := int(binary.BigEndian.Uint16(body[3:5]))
	components := int(body[5])
	if height == 0 || width == 0 {
		return jpegHead{}, jpegNotJPEG
	}
	if precision != 8 || (components != 1 && components != 3 && components != 4) {
		return jpegHead{}, jpegUnsupported
	}
	return jpegHead{width: width, height: height, components: components}, jpegOK
}

func carve(ctx context.Context, r io.ReaderAt, size, from, to int64, found func(offset int64) bool) error {
	if to > size {
		to = size
	}
	if to-from > maxCarveBytes {
		to = from + maxCarveBytes
	}
	buf := make([]byte, carveBlock+len(jpegStart)-1)
	hits := 0
	for position := from; position < to; position += carveBlock {
		if err := ctx.Err(); err != nil {
			return err
		}
		want := int64(len(buf))
		if want > to-position {
			want = to - position
		}
		block := buf[:want]
		if err := guard.ReadAtInto(r, block, position, size); err != nil {
			return nil
		}
		usable := int64(carveBlock)
		if position+usable >= to {
			usable = want
		}
		for start := 0; start < len(block); {
			i := bytes.Index(block[start:], jpegStart)
			if i < 0 || int64(start+i) >= usable {
				break
			}
			hits++
			if hits > maxCarveHits || !found(position+int64(start+i)) {
				return nil
			}
			start += i + 1
		}
	}
	return nil
}
