package render

import (
	"encoding/binary"
	"image"
	"math"
	"sync"

	"github.com/oops1/PixGo/internal/par"
)

const (
	tileSide    = 64
	tileBytes   = tileSide * tileSide * 4
	minRowsTask = 16
	maxRowsTask = 128
	smallAreaPx = 1 << 15
	defaultCell = 8

	unitScaleTolerance = 1e-9
	opaqueAlpha        = 0xFF000000
	fixedOne           = 65536
	alphaDivisor       = 255 * fixedOne
)

var le = binary.LittleEndian

type palette struct {
	px      uint32
	r, g, b uint32
}

type plan struct {
	dst        *image.RGBA
	rect       image.Rectangle
	nx, ny     int
	lvl        []byte
	opaque     bool
	nearest    bool
	transposed bool
	xa, ya     axis
	h, v       *axis
	canvasRow  []byte
	pal        [2]palette
}

var planPool = sync.Pool{New: func() any { return new(plan) }}

var tilePool = sync.Pool{New: func() any { return new([tileBytes]byte) }}

func pack(r, g, b uint8) uint32 {
	return uint32(r) | uint32(g)<<8 | uint32(b)<<16 | opaqueAlpha
}

func Render(dst *image.RGBA, src Source, m Mapping, bg Background) {
	if dst == nil {
		return
	}
	RenderRect(dst, dst.Bounds(), src, m, bg)
}

func RenderRect(dst *image.RGBA, r image.Rectangle, src Source, m Mapping, bg Background) {
	if dst == nil {
		return
	}
	r = r.Intersect(dst.Bounds())
	if r.Empty() {
		return
	}
	p := planPool.Get().(*plan)
	p.setup(dst, r, src, m, bg)
	p.run()
	p.dst = nil
	p.lvl = nil
	planPool.Put(p)
}

func (p *plan) setup(dst *image.RGBA, r image.Rectangle, src Source, m Mapping, bg Background) {
	p.dst = dst
	p.rect = r
	p.nx, p.ny = r.Dx(), r.Dy()
	p.transposed = false
	p.h, p.v = &p.xa, &p.ya
	p.xa.clear()
	p.ya.clear()

	canvas := pack(bg.Canvas.R, bg.Canvas.G, bg.Canvas.B)
	rowLen := max(p.nx, tileSide) * 4
	if cap(p.canvasRow) < rowLen {
		p.canvasRow = make([]byte, rowLen)
	}
	p.canvasRow = p.canvasRow[:rowLen]
	for i := 0; i < rowLen; i += 4 {
		le.PutUint32(p.canvasRow[i:], canvas)
	}

	if bg.Kind == BgChecker {
		p.pal[0] = makePalette(bg.Checker[0].R, bg.Checker[0].G, bg.Checker[0].B)
		p.pal[1] = makePalette(bg.Checker[1].R, bg.Checker[1].G, bg.Checker[1].B)
	} else {
		p.pal[0] = makePalette(bg.Fill.R, bg.Fill.G, bg.Fill.B)
		p.pal[1] = p.pal[0]
	}

	w, h := src.Width, src.Height
	if w <= 0 || h <= 0 || len(src.Levels) == 0 || !(m.Scale > 0) || math.IsInf(m.Scale, 0) {
		return
	}
	m = normalizeMapping(m)
	lvl, levelShift, ox, oy := pickLevel(src, m, image.Rect(r.Min.X-dst.Rect.Min.X, r.Min.Y-dst.Rect.Min.Y, r.Max.X-dst.Rect.Min.X, r.Max.Y-dst.Rect.Min.Y))
	if lvl == nil || lvl.Rect.Empty() {
		return
	}
	lb := lvl.Rect
	lw, lh := lb.Dx(), lb.Dy()
	p.lvl = lvl.Pix[lvl.PixOffset(lb.Min.X, lb.Min.Y):]
	p.opaque = IsOpaque(lvl)
	p.nearest = m.Filter == FilterNearest || (m.Filter == FilterAuto && m.Scale >= 4)

	ow, oh := m.Orient.Size(w, h)
	tr := m.Orient.Transposes()
	shift := 1 / float64(uint(1)<<uint(levelShift))
	checker := bg.Kind == BgChecker
	cell := float64(bg.Cell)
	if bg.Cell <= 0 {
		cell = defaultCell
	}
	fw, fh := float64(w), float64(h)

	xs := axisSpec{
		n:       p.nx,
		base:    r.Min.X - dst.Rect.Min.X,
		origin:  m.OriginX,
		scale:   m.Scale,
		extent:  float64(ow),
		mapper:  coordMapper{orient: m.Orient, w: fw, h: fh, fromU: true, wantX: !tr},
		shift:   shift,
		nearest: p.nearest,
		checker: checker,
		cell:    cell,
	}
	ys := axisSpec{
		n:       p.ny,
		base:    r.Min.Y - dst.Rect.Min.Y,
		origin:  m.OriginY,
		scale:   m.Scale,
		extent:  float64(oh),
		mapper:  coordMapper{orient: m.Orient, w: fw, h: fh, fromU: false, wantX: tr},
		shift:   shift,
		nearest: p.nearest,
		checker: checker,
		cell:    cell,
	}
	if tr {
		xs.limit, xs.mul, xs.offset = lh, lvl.Stride, float64(oy)
		ys.limit, ys.mul, ys.offset = lw, 4, float64(ox)
	} else {
		xs.limit, xs.mul, xs.offset = lw, 4, float64(ox)
		ys.limit, ys.mul, ys.offset = lh, lvl.Stride, float64(oy)
	}
	p.xa.build(&xs)
	p.ya.build(&ys)
	if p.xa.lo >= p.xa.hi || p.ya.lo >= p.ya.hi {
		p.xa.clear()
		p.ya.clear()
		return
	}
	p.transposed = tr
	if tr {
		p.h, p.v = &p.ya, &p.xa
	}
}

func pickLevel(src Source, m Mapping, px image.Rectangle) (lvl *image.NRGBA, levelShift, ox, oy int) {
	want := LevelShift(m.Scale)
	base := max(src.Shift, 0)
	if src.Detail != nil {
		for n := want; n < base; n++ {
			if win, ok := fetchWindow(src, m, px, n); ok {
				return win.Img, win.Shift, win.X, win.Y
			}
		}
	}
	idx := clampInt(want-base, 0, len(src.Levels)-1)
	return src.Levels[idx], base + idx, 0, 0
}

func fetchWindow(src Source, m Mapping, px image.Rectangle, shift int) (Window, bool) {
	need := levelNeed(visibleRect(m, src.Width, src.Height, px), shift, src.Width, src.Height)
	if need.Empty() {
		return Window{}, false
	}
	win := src.Detail.Window(shift, need)
	if win.Img == nil || win.Shift != shift || win.Img.Rect.Empty() || !need.In(win.Rect()) {
		return Window{}, false
	}
	return win, true
}

func makePalette(r, g, b uint8) palette {
	return palette{px: pack(r, g, b), r: uint32(r), g: uint32(g), b: uint32(b)}
}

func (p *plan) run() {
	workers := par.Count()
	if p.nx*p.ny < smallAreaPx {
		workers = 1
	}
	if p.transposed {
		tx := (p.nx + tileSide - 1) / tileSide
		ty := (p.ny + tileSide - 1) / tileSide
		par.Parallel(tx*ty, workers, func(t int) {
			p.renderTile((t%tx)*tileSide, (t/tx)*tileSide)
		})
		return
	}
	chunk := clampInt(p.ny/(workers*3), minRowsTask, maxRowsTask)
	tasks := (p.ny + chunk - 1) / chunk
	par.Parallel(tasks, workers, func(t int) {
		for i := t * chunk; i < min(p.ny, (t+1)*chunk); i++ {
			o := p.dst.PixOffset(p.rect.Min.X, p.rect.Min.Y+i)
			p.span(p.dst.Pix[o:o+p.nx*4], i, 0, p.nx)
		}
	})
}

func (p *plan) renderTile(ia, ja int) {
	ib := min(p.nx, ia+tileSide)
	jb := min(p.ny, ja+tileSide)
	rows, tw := ib-ia, jb-ja
	buf := tilePool.Get().(*[tileBytes]byte)
	for i := 0; i < rows; i++ {
		p.span(buf[i*tw*4:(i+1)*tw*4], ia+i, ja, jb)
	}
	for j := 0; j < tw; j++ {
		o := p.dst.PixOffset(p.rect.Min.X+ia, p.rect.Min.Y+ja+j)
		row := p.dst.Pix[o : o+rows*4]
		for i := 0; i < rows; i++ {
			le.PutUint32(row[i*4:], le.Uint32(buf[(i*tw+j)*4:]))
		}
	}
	tilePool.Put(buf)
}

func (p *plan) span(out []byte, i, ja, jb int) {
	v, h := p.v, p.h
	if i < v.lo || i >= v.hi || ja >= h.hi || jb <= h.lo {
		copy(out[:(jb-ja)*4], p.canvasRow)
		return
	}
	a, b := max(ja, h.lo), min(jb, h.hi)
	if ja < a {
		copy(out[:(a-ja)*4], p.canvasRow)
	}
	if b < jb {
		copy(out[(b-ja)*4:(jb-ja)*4], p.canvasRow)
	}
	mid := out[(a-ja)*4 : (b-ja)*4]
	r0 := v.off0[i]
	wy := v.frac[i]
	row0 := p.lvl[r0:]
	if p.opaque {
		switch {
		case h.copyable && wy == 0:
			o := r0 + h.off0[a]
			copy(mid, p.lvl[o:o+len(mid)])
		case p.nearest:
			spanOpaqueNearest(mid, row0, h.off0[a:b])
		case wy == 0:
			spanOpaqueBilinearH(mid, row0, h.off0[a:b], h.off1[a:b], h.frac[a:b])
		default:
			spanOpaqueBilinear(mid, row0, p.lvl[v.off1[i]:], wy, h.off0[a:b], h.off1[a:b], h.frac[a:b])
		}
		return
	}
	rp := v.par[i]
	if p.nearest || (h.copyable && wy == 0) {
		p.spanAlphaNearest(mid, row0, h.off0[a:b], h.par[a:b], rp)
		return
	}
	p.spanAlphaBilinear(mid, row0, p.lvl[v.off1[i]:], wy, h.off0[a:b], h.off1[a:b], h.frac[a:b], h.par[a:b], rp)
}
