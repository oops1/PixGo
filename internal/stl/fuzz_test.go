package stl

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"math"
	"testing"

	"github.com/oops1/PixGo/internal/codec"
)

func classified(err error) bool {
	return errors.Is(err, codec.ErrCorrupt) || errors.Is(err, codec.ErrTooLarge) || errors.Is(err, codec.ErrUnsupported)
}

func seedCorpus() [][]byte {
	cube := cubeTris(0, 0, 0, 1)
	attrs := make([]uint16, 12)
	attrs[0] = 0x8000 | 31<<10
	attrs[5] = 7
	seeds := [][]byte{
		binarySTL("cube", cube, nil, 0),
		binarySTL("solid cube", cube, nil, 17),
		binarySTL("viscam", cube, attrs, 0),
		binarySTL("solid COLOR=\x0a\x14\x1e\xff MATERIAL=\x01\x02\x03\xff", cube, attrs, 0),
		binarySTL("", nil, nil, 0),
		binarySTL("", []tri{{float32(math.NaN()), 0, 0, 1, 1, 1, 2, 2, 2}}, nil, 0),
		asciiSTL("cube", [][]tri{cube}, asciiStyle{}),
		asciiSTL("cube", [][]tri{cube, cube}, asciiStyle{crlf: true, upper: true}),
		asciiSTL("cube", [][]tri{cube}, asciiStyle{exp: true, tabs: true, noEnd: true, bom: true}),
		asciiSTL("cube", [][]tri{cube}, asciiStyle{cr: true, garbage: "\x00\x00junk"}),
		[]byte("solid q\nfacet normal 0 0 1\nouter loop\nvertex 0 0 0\nvertex 1 0 0\nvertex 1 1 0\nvertex 0 1 0\nendloop\nendfacet\nendsolid q\n"),
		[]byte("solid x\nfacet normal 0 0 1\nouter loop\nvertex nan 1.#QNAN 1e999\nvertex 1e 2e+ -\nvertex . .e1 0x1p3\nendloop\nendfacet\n"),
		[]byte("solid\nendsolid"),
		[]byte("\xFF\xFEs\x00o\x00l\x00i\x00d\x00"),
		nil,
	}
	return seeds
}

func fixCount(data []byte) []byte {
	if len(data) < binaryHeaderSize {
		return data
	}
	out := append([]byte(nil), data...)
	binary.LittleEndian.PutUint32(out[80:], uint32((len(out)-binaryHeaderSize)/binaryRecordSize))
	return out
}

func FuzzRead(f *testing.F) {
	for _, s := range seedCorpus() {
		f.Add(s, false)
		f.Add(s, true)
	}
	f.Fuzz(func(t *testing.T, data []byte, fix bool) {
		if fix {
			data = fixCount(data)
		}
		ctx := context.Background()
		m, info, err := Read(ctx, bytes.NewReader(data), int64(len(data)), Options{MaxTriangles: 1 << 16})
		if err != nil {
			if !classified(err) {
				t.Fatalf("unclassified error: %v", err)
			}
			return
		}
		tris := m.Triangles()
		if tris == 0 || len(m.V) != tris*9 || info.Triangles != tris || tris > 1<<16 {
			t.Fatalf("triangles %d, len(V) %d, info %+v", tris, len(m.V), info)
		}
		for _, v := range m.V {
			if !isFinite(v) {
				t.Fatalf("non-finite coordinate")
			}
		}
		if info.Colors != m.HasColors() || (m.C != nil && len(m.C) != tris*3) || m.N != nil {
			t.Fatalf("colour state inconsistent: %+v %d", info, len(m.C))
		}
		if m.Parts < 1 || m.Parts > tris {
			t.Fatalf("Parts = %d for %d triangles", m.Parts, tris)
		}
		lo, hi := m.Min, m.Max
		m.UpdateBounds()
		if lo != m.Min || hi != m.Max {
			t.Fatalf("bounds %v %v, recomputed %v %v", lo, hi, m.Min, m.Max)
		}
		pi, b, perr := Probe(ctx, bytes.NewReader(data), int64(len(data)))
		if perr != nil {
			t.Fatalf("Probe failed where Read succeeded: %v", perr)
		}
		if pi != info || b != [6]float32{lo[0], lo[1], lo[2], hi[0], hi[1], hi[2]} {
			t.Fatalf("Probe %+v %v, Read %+v %v %v", pi, b, info, lo, hi)
		}
		if IsBinary(data, int64(len(data))) && info.ASCII {
			t.Fatalf("a file with the binary size relation was read as ASCII")
		}
	})
}

func FuzzProbe(f *testing.F) {
	for _, s := range seedCorpus() {
		f.Add(s, false)
		f.Add(s, true)
	}
	f.Fuzz(func(t *testing.T, data []byte, fix bool) {
		if fix {
			data = fixCount(data)
		}
		info, b, err := Probe(context.Background(), bytes.NewReader(data), int64(len(data)))
		if err != nil {
			if !classified(err) {
				t.Fatalf("unclassified error: %v", err)
			}
			return
		}
		if info.Triangles <= 0 {
			t.Fatalf("Probe returned %d triangles", info.Triangles)
		}
		for a := 0; a < 3; a++ {
			if !isFinite(b[a]) || !isFinite(b[a+3]) || b[a] > b[a+3] {
				t.Fatalf("bad bounds %v", b)
			}
		}
	})
}

func FuzzSniff(f *testing.F) {
	for _, s := range seedCorpus() {
		f.Add(s)
		if len(s) > 100 {
			f.Add(s[:100])
		}
	}
	f.Fuzz(func(t *testing.T, head []byte) {
		sniffed := Sniff(head)
		binary := IsBinary(head, int64(len(head)))
		if sniffed {
			b := trimLead(head)
			if len(b) < 5 || !foldEq(b, "solid") {
				t.Fatalf("Sniff accepted %q", head)
			}
		}
		if binary && len(head) < binaryHeaderSize {
			t.Fatalf("IsBinary accepted a %d byte file", len(head))
		}
		Sniff(head[:len(head)/2])
		IsBinary(head, int64(len(head))+binarySlack+1)
	})
}
