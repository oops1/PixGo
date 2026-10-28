package step

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"sync/atomic"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/mesh"
	"github.com/oops1/PixGo/internal/par"
)

const (
	defaultMaxEntities = 20_000_000
	defaultDeflection  = 0.0008
	defaultAngle       = 0.35
	maxFileBytes       = math.MaxInt32
)

type Options struct {
	MaxTriangles int
	MaxEntities  int
	Deflection   float64
	Angle        float64
}

type Info struct {
	Schema, System, Name string
	Unit                 string
	Entities             int
	Faces                int
	Solids               int
	Failed               int
}

var facePanics atomic.Int64

func (o Options) norm() Options {
	if o.MaxTriangles <= 0 {
		o.MaxTriangles = mesh.DefaultMaxTriangles
	}
	if o.MaxEntities <= 0 {
		o.MaxEntities = defaultMaxEntities
	}
	if !(o.Deflection > 0) || !finite(o.Deflection) {
		o.Deflection = defaultDeflection
	}
	if !(o.Angle > 0) || !finite(o.Angle) {
		o.Angle = defaultAngle
	}
	o.Angle = math.Min(math.Max(o.Angle, 0.02), 1.2)
	o.Deflection = math.Min(math.Max(o.Deflection, 1e-6), 0.1)
	return o
}

func readAll(ctx context.Context, r io.ReaderAt, size int64) ([]byte, error) {
	if size <= 0 {
		return nil, corrupt("empty file")
	}
	if size > maxFileBytes {
		return nil, tooLarge("file is %d bytes", size)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	buf := make([]byte, size)
	n, err := r.ReadAt(buf, 0)
	if n != len(buf) {
		if err == nil || errors.Is(err, io.EOF) {
			err = io.ErrUnexpectedEOF
		}
		return nil, fmt.Errorf("step: read: %v: %w", err, codec.ErrCorrupt)
	}
	return buf, nil
}

type loaded struct {
	f    *stepFile
	m    *model
	sc   *scene
	info Info
}

func load(ctx context.Context, r io.ReaderAt, size int64, o Options) (*loaded, error) {
	data, err := readAll(ctx, r, size)
	if err != nil {
		return nil, err
	}
	f, err := parseFile(ctx, data, o.MaxEntities)
	if err != nil {
		return nil, err
	}
	m := newModel(f)
	sc, err := m.buildScene(ctx)
	if err != nil {
		return nil, err
	}
	l := &loaded{f: f, m: m, sc: sc}
	l.info = Info{
		Schema:   f.head.schema,
		System:   f.head.system,
		Name:     f.head.name,
		Unit:     sc.unit,
		Entities: len(f.ents),
		Solids:   len(sc.insts),
	}
	for _, in := range sc.insts {
		l.info.Faces += len(sc.items[in.item].faces)
	}
	if len(sc.insts) == 0 {
		return l, unsupported("no solid or surface geometry")
	}
	return l, nil
}

func Probe(ctx context.Context, r io.ReaderAt, size int64, o Options) (Info, [6]float32, error) {
	o = o.norm()
	l, err := load(ctx, r, size, o)
	if err != nil {
		if l != nil {
			return l.info, [6]float32{}, err
		}
		return Info{}, [6]float32{}, err
	}
	lo, hi, ok := l.bounds(ctx)
	if err := ctx.Err(); err != nil {
		return l.info, [6]float32{}, err
	}
	if !ok {
		return l.info, [6]float32{}, unsupported("no points")
	}
	return l.info, [6]float32{float32(lo[0]), float32(lo[1]), float32(lo[2]), float32(hi[0]), float32(hi[1]), float32(hi[2])}, nil
}

func (l *loaded) bounds(ctx context.Context) (vec3, vec3, bool) {
	lo := vec3{math.Inf(1), math.Inf(1), math.Inf(1)}
	hi := vec3{math.Inf(-1), math.Inf(-1), math.Inf(-1)}
	pts := map[int][]vec3{}
	for i, in := range l.sc.insts {
		if i&1023 == 0 && ctx.Err() != nil {
			return lo, hi, false
		}
		ps, ok := pts[in.item]
		if !ok {
			ps = l.m.itemPoints(l.sc.items[in.item])
			pts[in.item] = ps
		}
		for _, p := range ps {
			q := in.x.apply(p)
			for k := 0; k < 3; k++ {
				lo[k] = math.Min(lo[k], q[k])
				hi[k] = math.Max(hi[k], q[k])
			}
		}
	}
	return lo, hi, lo[0] <= hi[0] && lo.finite() && hi.finite()
}

func Read(ctx context.Context, r io.ReaderAt, size int64, o Options) (*mesh.Mesh, Info, error) {
	o = o.norm()
	l, err := load(ctx, r, size, o)
	if err != nil {
		if l != nil {
			return nil, l.info, err
		}
		return nil, Info{}, err
	}
	lo, hi, ok := l.bounds(ctx)
	if err := ctx.Err(); err != nil {
		return nil, l.info, err
	}
	diag := 1.0
	if ok {
		diag = dist(lo, hi)
	}
	if !(diag > 0) || !finite(diag) {
		diag = 1
	}
	deflMM := o.Deflection * diag
	m, sc := l.m, l.sc
	faces := map[int]*faceGeom{}
	var jobs []*faceGeom
	edges := map[int]*edgeGeom{}
	var edgeList []*edgeGeom
	for n, it := range sc.order {
		if n&255 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, l.info, err
			}
		}
		info := sc.items[it]
		s := sc.itemScale[it]
		if !(s > 0) {
			s = 1
		}
		for _, fr := range info.faces {
			if _, ok := faces[fr.face]; ok {
				continue
			}
			fg := m.resolveFace(fr.face, edges)
			fg.defl = deflMM / s
			fg.ang = o.Angle
			faces[fr.face] = fg
			jobs = append(jobs, fg)
			for _, lp := range fg.loops {
				for _, u := range lp.uses {
					if u.e != nil && !u.e.set {
						u.e.set = true
						u.e.defl = fg.defl * edgeFactor
						u.e.ang = fg.ang
						edgeList = append(edgeList, u.e)
					}
				}
			}
		}
	}
	workers := par.Count()
	par.Parallel(len(edgeList), workers, func(i int) {
		if ctx.Err() != nil {
			return
		}
		e := edgeList[i]
		defer func() {
			if r := recover(); r != nil {
				facePanics.Add(1)
				e.ok = false
				e.pts, e.ts = nil, nil
			}
		}()
		e.discretize()
	})
	if err := ctx.Err(); err != nil {
		return nil, l.info, err
	}
	var produced atomic.Int64
	limit := int64(o.MaxTriangles)
	par.Parallel(len(jobs), workers, func(i int) {
		if ctx.Err() != nil || produced.Load() > limit {
			return
		}
		fg := jobs[i]
		defer func() {
			if r := recover(); r != nil {
				facePanics.Add(1)
				fg.v, fg.n = nil, nil
				fg.fail = true
			}
		}()
		fg.tessellate()
		produced.Add(int64(len(fg.v) / 9))
	})
	if err := ctx.Err(); err != nil {
		return nil, l.info, err
	}
	if produced.Load() > limit {
		return nil, l.info, fmt.Errorf("step: more than %d triangles: %w: %w", o.MaxTriangles, codec.ErrTooLarge, mesh.ErrTooManyTriangles)
	}
	return l.assemble(faces, o)
}

func (l *loaded) assemble(faces map[int]*faceGeom, o Options) (*mesh.Mesh, Info, error) {
	sc := l.sc
	total := 0
	failed := 0
	coloured := false
	for _, in := range sc.insts {
		info := sc.items[in.item]
		if _, ok := sc.faceCol[info.ei]; ok {
			coloured = true
		}
		for _, fr := range info.faces {
			fg := faces[fr.face]
			if fg == nil || fg.fail {
				failed++
				continue
			}
			total += len(fg.v) / 9
			if !coloured {
				if _, ok := sc.faceCol[fr.face]; ok {
					coloured = true
				} else if _, ok := sc.faceCol[fr.shell]; ok && fr.shell >= 0 {
					coloured = true
				}
			}
		}
		if total > o.MaxTriangles {
			return nil, l.info, fmt.Errorf("step: more than %d triangles: %w: %w", o.MaxTriangles, codec.ErrTooLarge, mesh.ErrTooManyTriangles)
		}
	}
	l.info.Failed = failed
	if total == 0 {
		return nil, l.info, unsupported("no triangles")
	}
	ms := &mesh.Mesh{
		V:      make([]float32, 0, total*9),
		N:      make([]float32, 0, total*9),
		Parts:  len(sc.insts),
		Failed: failed,
	}
	if coloured {
		ms.C = make([]uint8, 0, total*3)
	}
	grey := [3]uint8{defaultGreyByte, defaultGreyByte, defaultGreyByte}
	for _, in := range sc.insts {
		info := sc.items[in.item]
		x := in.x
		ident := x.isIdentity()
		mirror := x.det() < 0
		nx := x
		if !ident {
			s := math.Cbrt(math.Abs(x.det()))
			if s > 0 && finite(s) {
				nx = x.scaled(1 / s)
			}
		}
		itemCol, hasItemCol := sc.faceCol[info.ei]
		for _, fr := range info.faces {
			fg := faces[fr.face]
			if fg == nil || fg.fail || len(fg.v) == 0 {
				continue
			}
			col := grey
			if c, ok := sc.faceCol[fr.face]; ok {
				col = c
			} else if c, ok := sc.faceCol[fr.shell]; ok && fr.shell >= 0 {
				col = c
			} else if hasItemCol {
				col = itemCol
			}
			swap := fr.flip != mirror
			neg := fr.flip
			nt := len(fg.v) / 9
			for t := 0; t < nt; t++ {
				order := [3]int{0, 1, 2}
				if swap {
					order = [3]int{0, 2, 1}
				}
				for _, k := range order {
					o := t*9 + k*3
					p := vec3{float64(fg.v[o]), float64(fg.v[o+1]), float64(fg.v[o+2])}
					n := vec3{float64(fg.n[o]), float64(fg.n[o+1]), float64(fg.n[o+2])}
					if !ident {
						p = x.apply(p)
						n = nx.applyDir(n)
						if q, ok := n.unit(); ok {
							n = q
						}
					}
					if neg {
						n = n.mul(-1)
					}
					if ident {
						ms.V = append(ms.V, fg.v[o], fg.v[o+1], fg.v[o+2])
					} else {
						ms.V = append(ms.V, float32(p[0]), float32(p[1]), float32(p[2]))
					}
					ms.N = append(ms.N, float32(n[0]), float32(n[1]), float32(n[2]))
				}
				if coloured {
					ms.C = append(ms.C, col[0], col[1], col[2])
				}
			}
		}
	}
	ms.UpdateBounds()
	return ms, l.info, nil
}

func (m *model) itemPoints(info *itemInfo) []vec3 {
	faces := info.faces
	workers := par.Count()
	chunks := min(workers*4, len(faces)/32+1)
	pts := make([][]vec3, chunks)
	closed := make([][]arcRef, chunks)
	bare := make([][]int, chunks)
	par.Parallel(chunks, workers, func(k int) {
		lo, hi := k*len(faces)/chunks, (k+1)*len(faces)/chunks
		pts[k], closed[k], bare[k] = m.facePoints(faces[lo:hi])
	})
	var out []vec3
	for _, p := range pts {
		out = append(out, p...)
	}
	for _, c := range closed {
		for _, a := range c {
			out = append(out, m.arcPoints(a)...)
		}
	}
	for _, c := range bare {
		for _, s := range c {
			out = append(out, m.surfaceExtent(s)...)
		}
	}
	return out
}

func (m *model) readVertex(ei int) (vec3, bool) {
	if m.f.name(ei) != "VERTEX_POINT" {
		return vec3{}, false
	}
	return m.readPoint(m.ref(m.f.args(ei), 1))
}

type arcRef struct {
	g      int
	p0, p1 vec3
	sense  bool
	closed bool
}

func (m *model) facePoints(faces []faceRef) ([]vec3, []arcRef, []int) {
	var out []vec3
	var closed []arcRef
	var bare []int
	f := m.f
	seenEdge := map[int]bool{}
	for _, fr := range faces {
		ei := fr.face
		switch f.name(ei) {
		case "TRIANGULATED_FACE", "COMPLEX_TRIANGULATED_FACE", "TRIANGULATED_SURFACE_SET", "COMPLEX_TRIANGULATED_SURFACE_SET":
			cl := m.ref(f.args(ei), 1)
			if seenEdge[-cl-2] {
				continue
			}
			seenEdge[-cl-2] = true
			if l, ok := argList(f.args(cl), 2); ok {
				for _, v := range l {
					if v.kind == vList && len(v.list) == 3 {
						if xs, ok := numList(v.list); ok {
							out = append(out, vec3{xs[0], xs[1], xs[2]})
						}
					}
				}
			}
			continue
		}
		a := f.args(ei)
		got := false
		for _, b := range m.refs(a, 1) {
			lp := m.ref(f.args(b), 1)
			la := f.args(lp)
			switch f.name(lp) {
			case "EDGE_LOOP":
				inLoop := map[int]bool{}
				for _, oe := range m.refs(la, 1) {
					e, _, ok := m.orientedEdge(oe, 0)
					if !ok {
						continue
					}
					got = true
					if inLoop[e] && len(a) > 2 {
						bare = append(bare, m.ref(a, 2))
					}
					inLoop[e] = true
					if seenEdge[e] {
						continue
					}
					seenEdge[e] = true
					ea := f.args(e)
					va, vb := m.ref(ea, 1), m.ref(ea, 2)
					p0, ok0 := m.readVertex(va)
					p1, ok1 := m.readVertex(vb)
					if ok0 {
						out = append(out, p0)
					}
					if ok1 {
						out = append(out, p1)
					}
					if ok0 && ok1 {
						if g := m.ref(ea, 3); m.isConic(g) {
							closed = append(closed, arcRef{g: g, p0: p0, p1: p1, sense: argBool(ea, 4, true), closed: va == vb || p0 == p1})
						}
					}
				}
			case "POLY_LOOP":
				for _, pi := range m.refs(la, 1) {
					if p, ok := m.readPoint(pi); ok {
						out = append(out, p)
						got = true
					}
				}
			case "VERTEX_LOOP":
				if p, ok := m.readVertex(m.ref(la, 1)); ok {
					out = append(out, p)
				}
			}
		}
		if !got && len(a) > 2 {
			bare = append(bare, m.ref(a, 2))
		}
	}
	return out, closed, bare
}

func (m *model) isConic(g int) bool {
	f := m.f
	switch f.name(g) {
	case "SURFACE_CURVE", "SEAM_CURVE", "INTERSECTION_CURVE", "BOUNDED_SURFACE_CURVE":
		g = m.ref(f.args(g), 1)
	}
	switch f.name(g) {
	case "CIRCLE", "ELLIPSE":
		return true
	}
	return false
}

func (m *model) arcPoints(a arcRef) []vec3 {
	f := m.f
	g := a.g
	switch f.name(g) {
	case "SURFACE_CURVE", "SEAM_CURVE", "INTERSECTION_CURVE", "BOUNDED_SURFACE_CURVE":
		g = m.ref(f.args(g), 1)
	}
	c := m.curve(g, 0)
	var fr frame
	var ra, rb float64
	switch k := c.(type) {
	case *circleCurve:
		fr, ra, rb = k.f, k.r, k.r
	case *ellipseCurve:
		fr, ra, rb = k.f, k.a, k.b
	default:
		return nil
	}
	lo, hi := 0.0, 2*math.Pi
	if !a.closed {
		t0, ok0 := invertCurve(c, a.p0)
		t1, ok1 := invertCurve(c, a.p1)
		if !ok0 || !ok1 {
			return nil
		}
		if a.sense {
			lo, hi = t0, t0+math.Mod(t1-t0+4*math.Pi, 2*math.Pi)
		} else {
			lo, hi = t1, t1+math.Mod(t0-t1+4*math.Pi, 2*math.Pi)
		}
	}
	var out []vec3
	for k := 0; k < 3; k++ {
		base := math.Atan2(rb*fr.y[k], ra*fr.x[k])
		for j := -4; j <= 6; j++ {
			t := base + float64(j)*math.Pi
			if t >= lo && t <= hi {
				out = append(out, fr.at(ra*math.Cos(t), rb*math.Sin(t), 0))
			}
		}
	}
	return out
}

func (m *model) surfaceExtent(sid int) []vec3 {
	s := m.surface(sid, 0)
	var c vec3
	var r float64
	switch k := s.(type) {
	case *sphereSurf:
		c, r = k.f.o, k.r
	case *torusSurf:
		c, r = k.f.o, k.rb+k.rs
	default:
		return nil
	}
	return []vec3{c.add(vec3{r, 0, 0}), c.add(vec3{-r, 0, 0}), c.add(vec3{0, r, 0}), c.add(vec3{0, -r, 0}), c.add(vec3{0, 0, r}), c.add(vec3{0, 0, -r})}
}
