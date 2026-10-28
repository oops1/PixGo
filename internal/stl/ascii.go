package stl

import (
	"context"
	"io"
	"math"
	"sync"
	"sync/atomic"

	"github.com/oops1/PixGo/internal/mesh"
	"github.com/oops1/PixGo/internal/par"
)

const (
	cutWindow       = 4096
	cutLookahead    = 9
	maxCutScan      = 64 << 10
	asciiCheckEvery = 4096
	maxPolygonVerts = 1 << 16
	asciiTriBytes   = 128
)

var asciiBlockBytes int64 = 4 << 20

const (
	tokOther = iota
	tokSolid
	tokFacet
	tokVertex
	tokEndLoop
	tokEndFacet
	tokEndSolid
)

func classify(w []byte) int {
	switch len(w) {
	case 5:
		if foldEq(w, "solid") {
			return tokSolid
		}
		if foldEq(w, "facet") {
			return tokFacet
		}
	case 6:
		if foldEq(w, "vertex") {
			return tokVertex
		}
	case 7:
		if foldEq(w, "endloop") {
			return tokEndLoop
		}
	case 8:
		if foldEq(w, "endfacet") {
			return tokEndFacet
		}
		if foldEq(w, "endsolid") {
			return tokEndSolid
		}
	}
	return tokOther
}

type asciiBlock struct {
	v         []float32
	tris      int
	lo, hi    [3]float32
	name      string
	seenSolid bool
	lead      int
	withTris  int
	lastHas   bool
}

type asciiParser struct {
	ctx     context.Context
	collect bool
	limit   int
	blk     asciiBlock
	poly    []float32
	cur     int
	err     error
}

func newASCIIParser(ctx context.Context, collect bool, limit, hint int) *asciiParser {
	p := &asciiParser{ctx: ctx, collect: collect, limit: limit, poly: make([]float32, 0, 12)}
	p.blk.lo = [3]float32{math.MaxFloat32, math.MaxFloat32, math.MaxFloat32}
	p.blk.hi = [3]float32{-math.MaxFloat32, -math.MaxFloat32, -math.MaxFloat32}
	if collect {
		p.blk.v = make([]float32, 0, hint)
	}
	return p
}

func finite3(p []float32) bool {
	return isFinite(p[0]) && isFinite(p[1]) && isFinite(p[2])
}

func (p *asciiParser) bound(c []float32) {
	for a := 0; a < 3; a++ {
		if c[a] < p.blk.lo[a] {
			p.blk.lo[a] = c[a]
		}
		if c[a] > p.blk.hi[a] {
			p.blk.hi[a] = c[a]
		}
	}
}

func (p *asciiParser) emit(a, b, c []float32) {
	if !finite3(a) || !finite3(b) || !finite3(c) {
		return
	}
	if p.blk.tris >= p.limit {
		p.err = tooLarge(int64(p.blk.tris)+1, p.limit)
		return
	}
	p.blk.tris++
	p.cur++
	if p.collect {
		p.blk.v = append(p.blk.v, a[0], a[1], a[2], b[0], b[1], b[2], c[0], c[1], c[2])
	}
	p.bound(a)
	p.bound(b)
	p.bound(c)
	if p.blk.tris&(asciiCheckEvery-1) == 0 {
		if e := p.ctx.Err(); e != nil {
			p.err = e
		}
	}
}

func (p *asciiParser) flush() {
	nv := len(p.poly) / 3
	for t := 1; t+1 < nv && p.err == nil; t++ {
		p.emit(p.poly[0:3], p.poly[t*3:t*3+3], p.poly[t*3+3:t*3+6])
	}
	p.poly = p.poly[:0]
}

func (p *asciiParser) closeSolid() {
	if !p.blk.seenSolid {
		p.blk.lead = p.cur
	} else if p.cur > 0 {
		p.blk.withTris++
	}
	p.cur = 0
}

func lineEnd(data []byte, i int) int {
	for i < len(data) && data[i] != '\n' && data[i] != '\r' {
		i++
	}
	return i
}

func startsAlphaWord(c byte) bool {
	l := c | 0x20
	return l >= 'a' && l <= 'z' && l != 'n' && l != 'i'
}

func (p *asciiParser) vertex(data []byte, i int) int {
	n := len(data)
	nan := float32(math.NaN())
	var xyz [3]float32
	for q := 0; q < 3; q++ {
		for i < n && isSpace(data[i]) {
			i++
		}
		if i >= n || startsAlphaWord(data[i]) {
			xyz[q] = nan
			continue
		}
		j := i + 1
		for j < n && !isSpace(data[j]) {
			j++
		}
		xyz[q] = parseFloat32(data[i:j])
		i = j
	}
	p.poly = append(p.poly, xyz[0], xyz[1], xyz[2])
	if len(p.poly) >= 3*maxPolygonVerts {
		p.flush()
	}
	return i
}

func (p *asciiParser) parse(data []byte, first bool) {
	i, n := 0, len(data)
	if first && n >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
		i = 3
	}
	haveName := false
	for p.err == nil {
		for i < n && isSpace(data[i]) {
			i++
		}
		if i >= n {
			break
		}
		j := i + 1
		for j < n && !isSpace(data[j]) {
			j++
		}
		w := data[i:j]
		i = j
		switch classify(w) {
		case tokVertex:
			i = p.vertex(data, i)
		case tokFacet, tokEndLoop, tokEndFacet:
			p.flush()
		case tokSolid:
			p.flush()
			p.closeSolid()
			p.blk.seenSolid = true
			e := lineEnd(data, i)
			if !haveName {
				if name := cleanName(string(data[i:min(e, i+4*maxNameBytes)])); name != "" {
					p.blk.name = name
					haveName = true
				}
			}
			i = e
		case tokEndSolid:
			p.flush()
			i = lineEnd(data, i)
		}
	}
	if p.err != nil {
		return
	}
	p.flush()
	if p.blk.seenSolid {
		p.blk.lastHas = p.cur > 0
	}
	p.closeSolid()
}

type asciiScan struct {
	r       io.ReaderAt
	size    int64
	collect bool
	limit   int
	total   atomic.Int64
}

func (s *asciiScan) cut(off int64) (int64, error) {
	if off >= s.size {
		return s.size, nil
	}
	win := make([]byte, cutWindow)
	pos := off
	scanLimit := off + maxCutScan
	for pos < s.size && pos < scanLimit {
		n := int(min(int64(len(win)), s.size-pos))
		if err := readFull(s.r, win[:n], pos); err != nil {
			return 0, err
		}
		last := pos+int64(n) == s.size
		w := win[:n]
		for i := 0; i < n; i++ {
			if w[i]|0x20 != 'e' {
				continue
			}
			if !last && i+cutLookahead > n {
				break
			}
			l := 0
			if foldEq(w[i:], "endfacet") {
				l = 8
			} else if foldEq(w[i:], "endloop") {
				l = 7
			}
			if l == 0 || i+l < n && !isSpace(w[i+l]) {
				continue
			}
			return pos + int64(i) + int64(l), nil
		}
		if last {
			return s.size, nil
		}
		pos += int64(n) - cutLookahead
	}
	for pos < s.size {
		n := int(min(int64(len(win)), s.size-pos))
		if err := readFull(s.r, win[:n], pos); err != nil {
			return 0, err
		}
		for i, c := range win[:n] {
			if isSpace(c) {
				return pos + int64(i), nil
			}
		}
		pos += int64(n)
	}
	return s.size, nil
}

func (s *asciiScan) block(ctx context.Context, t, tasks int) (asciiBlock, error) {
	start, end := int64(0), s.size
	var err error
	if t > 0 {
		if start, err = s.cut(int64(t) * asciiBlockBytes); err != nil {
			return asciiBlock{}, err
		}
	}
	if t < tasks-1 {
		if end, err = s.cut(int64(t+1) * asciiBlockBytes); err != nil {
			return asciiBlock{}, err
		}
	}
	if end <= start {
		return asciiBlock{}, nil
	}
	buf := getBuffer(int(end - start))
	defer putBuffer(buf)
	if err := readFull(s.r, buf, start); err != nil {
		return asciiBlock{}, err
	}
	p := newASCIIParser(ctx, s.collect, s.limit, len(buf)/asciiTriBytes*9+9)
	p.parse(buf, start == 0)
	if p.err != nil {
		return asciiBlock{}, p.err
	}
	if s.total.Add(int64(p.blk.tris)) > int64(s.limit) {
		return asciiBlock{}, tooLarge(s.total.Load(), s.limit)
	}
	return p.blk, nil
}

type asciiResult struct {
	blocks []asciiBlock
	tris   int
	parts  int
	name   string
	lo, hi [3]float32
}

func (s *asciiScan) run(ctx context.Context) (asciiResult, error) {
	tasks := int((s.size + asciiBlockBytes - 1) / asciiBlockBytes)
	blocks := make([]asciiBlock, tasks)
	var failed atomic.Bool
	var mu sync.Mutex
	var firstErr error
	par.Parallel(tasks, par.Count(), func(t int) {
		if failed.Load() {
			return
		}
		blk, err := func() (asciiBlock, error) {
			if err := ctx.Err(); err != nil {
				return asciiBlock{}, err
			}
			return s.block(ctx, t, tasks)
		}()
		if err != nil {
			mu.Lock()
			if firstErr == nil {
				firstErr = err
			}
			mu.Unlock()
			failed.Store(true)
			return
		}
		blocks[t] = blk
	})
	if firstErr != nil {
		return asciiResult{}, firstErr
	}
	res := asciiResult{
		blocks: blocks,
		lo:     [3]float32{math.MaxFloat32, math.MaxFloat32, math.MaxFloat32},
		hi:     [3]float32{-math.MaxFloat32, -math.MaxFloat32, -math.MaxFloat32},
	}
	carry := false
	for i := range blocks {
		b := &blocks[i]
		res.tris += b.tris
		if res.name == "" {
			res.name = b.name
		}
		if b.lead > 0 && !carry {
			res.parts++
		}
		if b.seenSolid {
			res.parts += b.withTris
			carry = b.lastHas
		} else {
			carry = carry || b.lead > 0
		}
		if b.tris == 0 {
			continue
		}
		for a := 0; a < 3; a++ {
			res.lo[a] = min(res.lo[a], b.lo[a])
			res.hi[a] = max(res.hi[a], b.hi[a])
		}
	}
	return res, nil
}

func readASCII(ctx context.Context, r io.ReaderAt, size int64, limit int) (*mesh.Mesh, Info, error) {
	s := &asciiScan{r: r, size: size, collect: true, limit: limit}
	res, err := s.run(ctx)
	if err != nil {
		return nil, Info{}, err
	}
	if res.tris == 0 {
		return nil, Info{}, corrupt("no triangle with finite coordinates")
	}
	var v []float32
	if len(res.blocks) == 1 {
		v = res.blocks[0].v
	} else {
		v = make([]float32, 0, res.tris*9)
		for i := range res.blocks {
			v = append(v, res.blocks[i].v...)
			res.blocks[i].v = nil
		}
	}
	m := &mesh.Mesh{V: v, Parts: max(res.parts, 1), Min: res.lo, Max: res.hi}
	return m, Info{ASCII: true, Triangles: res.tris, Name: res.name}, nil
}

func probeASCII(ctx context.Context, r io.ReaderAt, size int64) (Info, [6]float32, error) {
	s := &asciiScan{r: r, size: size, limit: math.MaxInt}
	res, err := s.run(ctx)
	if err != nil {
		return Info{}, [6]float32{}, err
	}
	if res.tris == 0 {
		return Info{}, [6]float32{}, corrupt("no triangle with finite coordinates")
	}
	b := [6]float32{res.lo[0], res.lo[1], res.lo[2], res.hi[0], res.hi[1], res.hi[2]}
	return Info{ASCII: true, Triangles: res.tris, Name: res.name}, b, nil
}
