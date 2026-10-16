package imaging

import (
	"os"
	"sync/atomic"

	"github.com/oops1/PixGo/internal/codec"
)

var (
	openedHook atomic.Pointer[func(*sourceFile)]
	closedHook atomic.Pointer[func(*sourceFile)]
)

type sourceFile struct{ f *os.File }

func (s *sourceFile) Close() error {
	if h := closedHook.Load(); h != nil {
		(*h)(s)
	}
	return s.f.Close()
}

func openSource(path string) (*codec.Source, *sourceFile, error) {
	f, err := openShared(path)
	if err != nil {
		return nil, nil, err
	}
	file := &sourceFile{f: f}
	if h := openedHook.Load(); h != nil {
		(*h)(file)
	}
	st, err := f.Stat()
	if err != nil {
		file.Close()
		return nil, nil, err
	}
	head := make([]byte, codec.SniffLen)
	n, _ := f.ReadAt(head, 0)
	return &codec.Source{Path: path, R: f, Size: st.Size(), Head: head[:n]}, file, nil
}
