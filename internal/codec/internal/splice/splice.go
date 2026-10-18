package splice

import "io"

type Writer struct {
	w    io.Writer
	at   int
	data []byte
	seen int
	done bool
}

func New(w io.Writer, at int, data []byte) *Writer {
	return &Writer{w: w, at: at, data: data, done: len(data) == 0}
}

func (s *Writer) Write(p []byte) (int, error) {
	if s.done {
		return s.w.Write(p)
	}
	head := min(len(p), s.at-s.seen)
	if head > 0 {
		if n, err := s.w.Write(p[:head]); err != nil {
			return n, err
		}
		s.seen += head
	}
	if s.seen < s.at {
		return len(p), nil
	}
	s.done = true
	if _, err := s.w.Write(s.data); err != nil {
		return head, err
	}
	if head < len(p) {
		n, err := s.w.Write(p[head:])
		return head + n, err
	}
	return len(p), nil
}

func (s *Writer) Flush() error {
	if s.done {
		return nil
	}
	s.done = true
	_, err := s.w.Write(s.data)
	return err
}
