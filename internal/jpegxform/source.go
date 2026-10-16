package jpegxform

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/oops1/PixGo/internal/codec"
)

const sourceBuffer = 64 << 10

func corrupt(format string, args ...any) error {
	return fmt.Errorf("%w: jpegxform: %s", codec.ErrCorrupt, fmt.Sprintf(format, args...))
}

func unsupported(format string, args ...any) error {
	return fmt.Errorf("%w: jpegxform: %s", codec.ErrUnsupported, fmt.Sprintf(format, args...))
}

func tooLarge(format string, args ...any) error {
	return fmt.Errorf("%w: jpegxform: %s", codec.ErrTooLarge, fmt.Sprintf(format, args...))
}

type byteSource struct {
	ctx  context.Context
	r    io.ReaderAt
	size int64
	next int64
	buf  []byte
	i, n int
	err  error
}

func newByteSource(r io.ReaderAt, size, offset int64) *byteSource {
	return &byteSource{r: r, size: size, next: offset, buf: make([]byte, sourceBuffer)}
}

func (s *byteSource) fill() bool {
	if s.err != nil {
		return false
	}
	if s.ctx != nil {
		if err := s.ctx.Err(); err != nil {
			s.err = err
			return false
		}
	}
	if s.next >= s.size {
		s.err = io.ErrUnexpectedEOF
		return false
	}
	want := int64(len(s.buf))
	if left := s.size - s.next; left < want {
		want = left
	}
	n, err := s.r.ReadAt(s.buf[:want], s.next)
	if err != nil && !errors.Is(err, io.EOF) {
		s.err = err
		return false
	}
	if n == 0 {
		s.err = io.ErrUnexpectedEOF
		return false
	}
	s.next += int64(n)
	s.i, s.n = 0, n
	return true
}

func (s *byteSource) readByte() (byte, bool) {
	if s.i >= s.n && !s.fill() {
		return 0, false
	}
	c := s.buf[s.i]
	s.i++
	return c, true
}

func (s *byteSource) readFull(p []byte) bool {
	for len(p) > 0 {
		if s.i >= s.n && !s.fill() {
			return false
		}
		c := copy(p, s.buf[s.i:s.n])
		s.i += c
		p = p[c:]
	}
	return true
}

func (s *byteSource) skip(n int) bool {
	for n > 0 {
		if s.i >= s.n && !s.fill() {
			return false
		}
		c := min(n, s.n-s.i)
		s.i += c
		n -= c
	}
	return true
}

func (s *byteSource) failure() error {
	switch {
	case s.err == nil:
		return nil
	case errors.Is(s.err, io.ErrUnexpectedEOF), errors.Is(s.err, io.EOF):
		return corrupt("truncated file")
	}
	return s.err
}

func (s *byteSource) u16() (int, bool) {
	hi, ok := s.readByte()
	if !ok {
		return 0, false
	}
	lo, ok := s.readByte()
	return int(hi)<<8 | int(lo), ok
}

func (s *byteSource) marker() (byte, bool) {
	for {
		c, ok := s.readByte()
		if !ok {
			return 0, false
		}
		if c != 0xFF {
			continue
		}
		d, ok := s.readByte()
		for ok && d == 0xFF {
			d, ok = s.readByte()
		}
		if !ok {
			return 0, false
		}
		if d != 0 {
			return d, true
		}
	}
}
