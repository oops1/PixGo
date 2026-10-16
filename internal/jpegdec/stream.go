package jpegdec

import (
	"context"
	"io"
	"sync"
)

const streamBufferSize = 32 << 10

var streamBuffers = sync.Pool{New: func() any {
	buffer := make([]byte, streamBufferSize)
	return &buffer
}}

type stream struct {
	ctx    context.Context
	reader io.ReaderAt
	size   int64
	holder *[]byte
	buf    []byte
	base   int64
	pos    int
	end    int
}

func newStream(ctx context.Context, reader io.ReaderAt, size int64) *stream {
	holder := streamBuffers.Get().(*[]byte)
	return &stream{ctx: ctx, reader: reader, size: size, holder: holder, buf: *holder}
}

func (s *stream) release() {
	if s.holder != nil {
		streamBuffers.Put(s.holder)
		s.holder = nil
		s.buf = nil
	}
}

func (s *stream) offset() int64 { return s.base + int64(s.pos) }

func (s *stream) seek(offset int64) {
	if offset >= s.base && offset <= s.base+int64(s.end) {
		s.pos = int(offset - s.base)
		return
	}
	s.base = offset
	s.pos = 0
	s.end = 0
}

func (s *stream) more() error {
	if err := s.ctx.Err(); err != nil {
		return err
	}
	if s.pos > 0 {
		copy(s.buf, s.buf[s.pos:s.end])
		s.base += int64(s.pos)
		s.end -= s.pos
		s.pos = 0
	}
	at := s.base + int64(s.end)
	want := int64(len(s.buf) - s.end)
	if remaining := s.size - at; remaining < want {
		want = remaining
	}
	if want <= 0 {
		return io.EOF
	}
	n, err := s.reader.ReadAt(s.buf[s.end:s.end+int(want)], at)
	s.end += n
	if n > 0 {
		return nil
	}
	if err == nil || err == io.EOF {
		return io.ErrUnexpectedEOF
	}
	return err
}

func (s *stream) ensure(n int) error {
	for s.end-s.pos < n {
		if err := s.more(); err != nil {
			if err == io.EOF {
				return io.ErrUnexpectedEOF
			}
			return err
		}
	}
	return nil
}

func (s *stream) readByte() (byte, error) {
	if s.pos >= s.end {
		if err := s.ensure(1); err != nil {
			return 0, err
		}
	}
	c := s.buf[s.pos]
	s.pos++
	return c, nil
}

func (s *stream) readFull(p []byte) error {
	for len(p) > 0 {
		if s.pos >= s.end {
			if err := s.ensure(1); err != nil {
				return err
			}
		}
		n := copy(p, s.buf[s.pos:s.end])
		s.pos += n
		p = p[n:]
	}
	return nil
}

func (s *stream) skip(n int) error {
	if n <= s.end-s.pos {
		s.pos += n
		return nil
	}
	target := s.offset() + int64(n)
	if target > s.size {
		return io.ErrUnexpectedEOF
	}
	if err := s.ctx.Err(); err != nil {
		return err
	}
	s.seek(target)
	return nil
}
