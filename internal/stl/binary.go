package stl

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"math"
	"sync/atomic"

	"github.com/oops1/PixGo/internal/mesh"
	"github.com/oops1/PixGo/internal/par"
)

const (
	binaryChunkTriangles = 8192
	colorValidBit        = 0x8000
	materialiseTag       = "COLOR="
	materialiseCut       = "MATERIAL="
)

var defaultColor = [3]uint8{0xB4, 0xB4, 0xB4}

const (
	modeViscam = iota
	modeMaterialise
)

type binaryHeader struct {
	name  string
	mode  int
	color [3]uint8
}

func parseBinaryHeader(h []byte) binaryHeader {
	out := binaryHeader{mode: modeViscam, color: defaultColor}
	text := h
	if i := bytes.Index(h, []byte(materialiseTag)); i >= 0 {
		out.mode = modeMaterialise
		if i+len(materialiseTag)+3 <= len(h) {
			c := h[i+len(materialiseTag):]
			out.color = [3]uint8{c[0], c[1], c[2]}
		}
		text = h[:i]
	}
	if i := bytes.Index(text, []byte(materialiseCut)); i >= 0 {
		text = text[:i]
	}
	if i := bytes.IndexByte(text, 0); i >= 0 {
		text = text[:i]
	}
	name := cleanName(string(text))
	if len(name) >= 5 && foldEq([]byte(name[:5]), "solid") && (len(name) == 5 || name[5] == ' ') {
		name = cleanName(name[5:])
	}
	out.name = name
	return out
}

func expand5(v uint16) uint8 { return uint8(v<<3 | v>>2) }

type chunkStat struct {
	kept    int
	colored bool
	lo, hi  [3]float32
	err     error
}

type binaryScan struct {
	r     io.ReaderAt
	n     int
	mode  int
	color [3]uint8
	v     []float32
	c     []uint8
}

type binaryResult struct {
	kept    int
	colored bool
	lo, hi  [3]float32
}

func (s *binaryScan) chunk(buf []byte, first int, tris int) chunkStat {
	st := chunkStat{
		lo: [3]float32{math.MaxFloat32, math.MaxFloat32, math.MaxFloat32},
		hi: [3]float32{-math.MaxFloat32, -math.MaxFloat32, -math.MaxFloat32},
	}
	write := s.v != nil
	le := binary.LittleEndian
	k := 0
	for j := 0; j < tris; j++ {
		rec := buf[j*binaryRecordSize+12 : (j+1)*binaryRecordSize]
		var f [9]float32
		ok := true
		for q := 0; q < 9; q++ {
			bits := le.Uint32(rec[q*4:])
			if bits&0x7F800000 == 0x7F800000 {
				ok = false
				break
			}
			f[q] = math.Float32frombits(bits)
		}
		if !ok {
			continue
		}
		for q := 0; q < 9; q++ {
			a := q % 3
			if f[q] < st.lo[a] {
				st.lo[a] = f[q]
			}
			if f[q] > st.hi[a] {
				st.hi[a] = f[q]
			}
		}
		attr := le.Uint16(rec[36:])
		rgb := s.color
		var r5, g5, b5 uint16
		has := false
		if s.mode == modeViscam {
			if attr&colorValidBit != 0 {
				has = true
				b5, g5, r5 = attr&31, attr>>5&31, attr>>10&31
			}
		} else if attr&colorValidBit == 0 && attr != 0 {
			has = true
			r5, g5, b5 = attr&31, attr>>5&31, attr>>10&31
		}
		if has {
			rgb = [3]uint8{expand5(r5), expand5(g5), expand5(b5)}
			st.colored = true
		}
		if write {
			at := (first + k) * 9
			copy(s.v[at:at+9], f[:])
			cat := (first + k) * 3
			s.c[cat], s.c[cat+1], s.c[cat+2] = rgb[0], rgb[1], rgb[2]
		}
		k++
	}
	st.kept = k
	return st
}

func (s *binaryScan) run(ctx context.Context) (binaryResult, error) {
	chunks := (s.n + binaryChunkTriangles - 1) / binaryChunkTriangles
	stats := make([]chunkStat, chunks)
	var failed atomic.Bool
	par.Parallel(chunks, par.Count(), func(t int) {
		if failed.Load() {
			return
		}
		if err := ctx.Err(); err != nil {
			stats[t].err = err
			failed.Store(true)
			return
		}
		first := t * binaryChunkTriangles
		tris := min(binaryChunkTriangles, s.n-first)
		buf := getBuffer(tris * binaryRecordSize)
		off := binaryHeaderSize + int64(first)*binaryRecordSize
		if err := readFull(s.r, buf, off); err != nil {
			stats[t].err = err
			failed.Store(true)
			putBuffer(buf)
			return
		}
		stats[t] = s.chunk(buf, first, tris)
		putBuffer(buf)
	})
	res := binaryResult{
		lo: [3]float32{math.MaxFloat32, math.MaxFloat32, math.MaxFloat32},
		hi: [3]float32{-math.MaxFloat32, -math.MaxFloat32, -math.MaxFloat32},
	}
	for t := range stats {
		if stats[t].err != nil {
			return binaryResult{}, stats[t].err
		}
	}
	dst := 0
	for t := range stats {
		st := &stats[t]
		first := t * binaryChunkTriangles
		if s.v != nil && first != dst && st.kept > 0 {
			copy(s.v[dst*9:], s.v[first*9:(first+st.kept)*9])
			copy(s.c[dst*3:], s.c[first*3:(first+st.kept)*3])
		}
		dst += st.kept
		res.colored = res.colored || st.colored
		if st.kept == 0 {
			continue
		}
		for a := 0; a < 3; a++ {
			res.lo[a] = min(res.lo[a], st.lo[a])
			res.hi[a] = max(res.hi[a], st.hi[a])
		}
	}
	res.kept = dst
	return res, nil
}

func binaryCount(head []byte, limit int) (int, error) {
	n := int64(binary.LittleEndian.Uint32(head[80:84]))
	if n == 0 {
		return 0, corrupt("binary file without triangles")
	}
	if n > int64(limit) || n > math.MaxInt/36 {
		return 0, tooLarge(n, limit)
	}
	return int(n), nil
}

func readBinary(ctx context.Context, r io.ReaderAt, head []byte, limit int) (*mesh.Mesh, Info, error) {
	n, err := binaryCount(head, limit)
	if err != nil {
		return nil, Info{}, err
	}
	hdr := parseBinaryHeader(head[:80])
	s := &binaryScan{r: r, n: n, mode: hdr.mode, color: hdr.color, v: make([]float32, n*9), c: make([]uint8, n*3)}
	res, err := s.run(ctx)
	if err != nil {
		return nil, Info{}, err
	}
	if res.kept == 0 {
		return nil, Info{}, corrupt("no triangle with finite coordinates")
	}
	m := &mesh.Mesh{V: s.v[:res.kept*9], Parts: 1, Min: res.lo, Max: res.hi}
	if res.colored {
		m.C = s.c[:res.kept*3]
	}
	return m, Info{Triangles: res.kept, Name: hdr.name, Colors: res.colored}, nil
}

func probeBinary(ctx context.Context, r io.ReaderAt, head []byte) (Info, [6]float32, error) {
	n, err := binaryCount(head, math.MaxInt)
	if err != nil {
		return Info{}, [6]float32{}, err
	}
	hdr := parseBinaryHeader(head[:80])
	s := &binaryScan{r: r, n: n, mode: hdr.mode, color: hdr.color}
	res, err := s.run(ctx)
	if err != nil {
		return Info{}, [6]float32{}, err
	}
	if res.kept == 0 {
		return Info{}, [6]float32{}, corrupt("no triangle with finite coordinates")
	}
	b := [6]float32{res.lo[0], res.lo[1], res.lo[2], res.hi[0], res.hi[1], res.hi[2]}
	return Info{Triangles: res.kept, Name: hdr.name, Colors: res.colored}, b, nil
}
