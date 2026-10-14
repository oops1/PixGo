package exif

import (
	"encoding/binary"
	"io"

	"github.com/oops1/PixGo/internal/codec"
)

const (
	MagicOlympus     uint16 = 0x4f52
	MagicOlympusAlt  uint16 = 0x5352
	MagicPanasonic   uint16 = 0x0055
	magicClassicTIFF uint16 = 42
)

func acceptCameraMagic(magic uint16) bool {
	switch magic {
	case magicClassicTIFF, MagicOlympus, MagicOlympusAlt, MagicPanasonic:
		return true
	}
	return false
}

func NewReaderAtCamera(r io.ReaderAt, base, length int64) (*Reader, error) {
	if base < 0 || length < 0 || r == nil {
		return nil, invalid("bad TIFF window %d+%d", base, length)
	}
	return newReaderAccepting(readerSource{r: r, base: uint64(base), extent: uint64(length)}, acceptCameraMagic)
}

func NewReaderAtOffset(r io.ReaderAt, base, length int64, order binary.ByteOrder, first uint32) (*Reader, error) {
	if base < 0 || length < 0 || r == nil || order == nil {
		return nil, invalid("bad TIFF window %d+%d", base, length)
	}
	return &Reader{src: readerSource{r: r, base: uint64(base), extent: uint64(length)}, order: order, first: first}, nil
}

func (r *Reader) Orientation() (codec.Orientation, bool) {
	return orientationFrom(r)
}

func (e Entry) Location() (offset, length uint64, ok bool) {
	n := e.byteLen()
	if n <= 4 || e.reader == nil {
		return 0, 0, false
	}
	return uint64(e.reader.order.Uint32(e.inline[:])), n, true
}
