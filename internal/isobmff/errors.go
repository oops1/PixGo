package isobmff

import (
	"fmt"
	"io"
	"math"

	"github.com/oops1/PixGo/internal/codec"
)

func corrupt(format string, args ...any) error {
	return fmt.Errorf("%w: %s", codec.ErrCorrupt, fmt.Sprintf(format, args...))
}

func unsupported(format string, args ...any) error {
	return fmt.Errorf("%w: %s", codec.ErrUnsupported, fmt.Sprintf(format, args...))
}

func tooLarge(format string, args ...any) error {
	return fmt.Errorf("%w: %s", codec.ErrTooLarge, fmt.Sprintf(format, args...))
}

func readAt(r io.ReaderAt, off, n, size int64) ([]byte, error) {
	if off < 0 || n < 0 || off > size || n > size-off {
		return nil, corrupt("range %d+%d outside %d byte file", off, n, size)
	}
	if n > math.MaxInt32 {
		return nil, tooLarge("read of %d bytes", n)
	}
	buf := make([]byte, n)
	if n == 0 {
		return buf, nil
	}
	got, err := r.ReadAt(buf, off)
	if got == len(buf) {
		return buf, nil
	}
	if err == nil || err == io.EOF {
		err = io.ErrUnexpectedEOF
	}
	return nil, fmt.Errorf("%w: %v", codec.ErrCorrupt, err)
}

func readAtInto(r io.ReaderAt, buf []byte, off, size int64) error {
	n := int64(len(buf))
	if off < 0 || off > size || n > size-off {
		return corrupt("range %d+%d outside %d byte file", off, n, size)
	}
	if n == 0 {
		return nil
	}
	got, err := r.ReadAt(buf, off)
	if got == len(buf) {
		return nil
	}
	if err == nil || err == io.EOF {
		err = io.ErrUnexpectedEOF
	}
	return fmt.Errorf("%w: %v", codec.ErrCorrupt, err)
}
