package stl

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/mesh"
)

const (
	binaryHeaderSize = 84
	binaryRecordSize = 50
	binarySlack      = 512
	headSize         = 4096
	maxNameBytes     = 256
	pooledBufferMin  = 64 << 10
)

type Options struct {
	MaxTriangles int
}

type Info struct {
	ASCII     bool
	Triangles int
	Name      string
	Colors    bool
}

type format int

const (
	formatBinary format = iota + 1
	formatASCII
)

var bufferPool sync.Pool

func getBuffer(n int) []byte {
	if n < pooledBufferMin {
		return make([]byte, n)
	}
	if v, ok := bufferPool.Get().(*[]byte); ok && cap(*v) >= n {
		return (*v)[:n]
	}
	return make([]byte, n, n+n/4)
}

func putBuffer(b []byte) {
	if cap(b) >= pooledBufferMin {
		bufferPool.Put(&b)
	}
}

func corrupt(format string, args ...any) error {
	return fmt.Errorf("stl: %s: %w", fmt.Sprintf(format, args...), codec.ErrCorrupt)
}

func unsupported(format string, args ...any) error {
	return fmt.Errorf("stl: %s: %w", fmt.Sprintf(format, args...), codec.ErrUnsupported)
}

func tooLarge(count int64, limit int) error {
	return fmt.Errorf("stl: %d triangles exceed the limit of %d: %w: %w", count, limit, codec.ErrTooLarge, mesh.ErrTooManyTriangles)
}

func readFull(r io.ReaderAt, buf []byte, off int64) error {
	if len(buf) == 0 {
		return nil
	}
	got, err := r.ReadAt(buf, off)
	if got == len(buf) {
		return nil
	}
	if err == nil || err == io.EOF {
		return corrupt("file ends at %d, expected %d more bytes", off+int64(got), len(buf)-got)
	}
	return fmt.Errorf("stl: read at %d: %w", off, err)
}

func isSpace(c byte) bool { return c <= ' ' }

func trimLead(b []byte) []byte {
	if len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		b = b[3:]
	}
	for len(b) > 0 && isSpace(b[0]) {
		b = b[1:]
	}
	return b
}

func foldEq(w []byte, lit string) bool {
	if len(w) < len(lit) {
		return false
	}
	for k := 0; k < len(lit); k++ {
		if w[k]|0x20 != lit[k] {
			return false
		}
	}
	return true
}

func firstFold(b []byte, lits ...string) int {
	best := -1
	for _, lit := range lits {
		for i := 0; i+len(lit) <= len(b); i++ {
			if best >= 0 && i+len(lit) >= best {
				break
			}
			if b[i]|0x20 == lit[0] && foldEq(b[i:i+len(lit)], lit) {
				best = i + len(lit)
				break
			}
		}
	}
	return best
}

func Sniff(head []byte) bool {
	b := trimLead(head)
	if len(b) < 5 || !foldEq(b, "solid") {
		return false
	}
	b = b[5:]
	if len(b) > 0 && !isSpace(b[0]) {
		return false
	}
	end := firstFold(b, "facet", "endsolid")
	return end >= 0 && bytes.IndexByte(head[:len(head)-len(b)+end], 0) < 0
}

func IsBinary(head []byte, size int64) bool {
	if len(head) < binaryHeaderSize || size < binaryHeaderSize {
		return false
	}
	want := binaryHeaderSize + binaryRecordSize*int64(binary.LittleEndian.Uint32(head[80:84]))
	return size >= want && size-want <= binarySlack
}

func detect(r io.ReaderAt, size int64) (format, []byte, error) {
	if size <= 0 {
		return 0, nil, corrupt("empty file")
	}
	head := make([]byte, int(min(size, headSize)))
	if err := readFull(r, head, 0); err != nil {
		return 0, nil, err
	}
	if IsBinary(head, size) {
		return formatBinary, head, nil
	}
	if len(head) >= 2 && (head[0] == 0xFF && head[1] == 0xFE || head[0] == 0xFE && head[1] == 0xFF) {
		return 0, nil, unsupported("UTF-16 text")
	}
	if Sniff(head) {
		return formatASCII, head, nil
	}
	if looseBinary(head, size) {
		return formatBinary, head, nil
	}
	return 0, nil, corrupt("neither binary nor ASCII STL")
}

func looseBinary(head []byte, size int64) bool {
	if len(head) < binaryHeaderSize {
		return false
	}
	n := int64(binary.LittleEndian.Uint32(head[80:84]))
	return n > 0 && size >= binaryHeaderSize+binaryRecordSize*n
}

func cleanName(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > maxNameBytes {
		s = s[:maxNameBytes]
	}
	s = strings.ToValidUTF8(s, "")
	s = strings.Map(func(r rune) rune {
		if r < ' ' || r == 0x7f || r == utf8.RuneError {
			return -1
		}
		return r
	}, s)
	return strings.TrimSpace(s)
}

func Read(ctx context.Context, r io.ReaderAt, size int64, o Options) (*mesh.Mesh, Info, error) {
	if err := ctx.Err(); err != nil {
		return nil, Info{}, err
	}
	limit := o.MaxTriangles
	if limit <= 0 {
		limit = mesh.DefaultMaxTriangles
	}
	f, head, err := detect(r, size)
	if err != nil {
		return nil, Info{}, err
	}
	var m *mesh.Mesh
	var info Info
	if f == formatBinary {
		m, info, err = readBinary(ctx, r, head, limit)
	} else {
		m, info, err = readASCII(ctx, r, size, limit)
	}
	if err != nil {
		if cerr := ctx.Err(); cerr != nil && !errors.Is(err, codec.ErrTooLarge) {
			return nil, Info{}, cerr
		}
		return nil, Info{}, err
	}
	return m, info, nil
}

func Probe(ctx context.Context, r io.ReaderAt, size int64) (Info, [6]float32, error) {
	if err := ctx.Err(); err != nil {
		return Info{}, [6]float32{}, err
	}
	f, head, err := detect(r, size)
	if err != nil {
		return Info{}, [6]float32{}, err
	}
	var info Info
	var bounds [6]float32
	if f == formatBinary {
		info, bounds, err = probeBinary(ctx, r, head)
	} else {
		info, bounds, err = probeASCII(ctx, r, size)
	}
	if err != nil {
		if cerr := ctx.Err(); cerr != nil {
			return Info{}, [6]float32{}, cerr
		}
		return Info{}, [6]float32{}, err
	}
	return info, bounds, nil
}

func isFinite(f float32) bool { return math.Float32bits(f)&0x7F800000 != 0x7F800000 }
