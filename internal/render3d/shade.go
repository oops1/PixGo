package render3d

import (
	"context"
	"image"
	"image/color"
	"math"
	"sync/atomic"

	"github.com/oops1/PixGo/internal/par"
)

const (
	edgeDepthK    = 10.0
	edgeSilDark   = 0.62
	edgeCreaseMin = 0.866
	edgeCreaseMax = 0.707
	edgeCreaseDk  = 0.5
	shoulder      = 0.8
)

var (
	keyDir  = normC(-0.6, 0.6, 0.53)
	fillDir = normC(0.8, -0.1, 0.55)
	rimDir  = normC(0.15, 0.55, -0.82)
)

func normC(x, y, z float64) [3]float64 { return norm3([3]float64{x, y, z}) }

type target struct {
	nrgba *image.NRGBA
	rgba  *image.RGBA
	w, h  int
}

type shading struct {
	base  [3]float64
	bg    [3]float64
	bgA   float64
	bgN   color.NRGBA
	edges bool
}

func newShading(st Style) *shading {
	b := st.Base
	if b == (color.NRGBA{}) {
		b = DefaultStyle().Base
	}
	sh := &shading{
		base:  [3]float64{lin8(b.R), lin8(b.G), lin8(b.B)},
		bg:    [3]float64{lin8(st.Background.R), lin8(st.Background.G), lin8(st.Background.B)},
		bgA:   float64(st.Background.A) / 255,
		bgN:   st.Background,
		edges: st.Edges,
	}
	return sh
}

func fillBackground(out *target, sh *shading) {
	c := sh.bgN
	if out.nrgba != nil {
		p := out.nrgba.Pix
		for y := 0; y < out.h; y++ {
			row := p[y*out.nrgba.Stride : y*out.nrgba.Stride+out.w*4]
			for x := 0; x < len(row); x += 4 {
				row[x], row[x+1], row[x+2], row[x+3] = c.R, c.G, c.B, c.A
			}
		}
		return
	}
	pr, pg, pb, pa := premul(c)
	p := out.rgba.Pix
	for y := 0; y < out.h; y++ {
		row := p[y*out.rgba.Stride : y*out.rgba.Stride+out.w*4]
		for x := 0; x < len(row); x += 4 {
			row[x], row[x+1], row[x+2], row[x+3] = pr, pg, pb, pa
		}
	}
}

func premul(c color.NRGBA) (uint8, uint8, uint8, uint8) {
	a := uint32(c.A)
	return uint8((uint32(c.R)*a + 127) / 255), uint8((uint32(c.G)*a + 127) / 255), uint8((uint32(c.B)*a + 127) / 255), c.A
}

type shader struct {
	s      *Scene
	v      *view
	sh     *shading
	buf    []uint64
	ry0    int
	ry1    int
	last   uint32
	lastOK bool
	lastC  [3]float64
	lastN  [3]float64
	ncID   [normalCache]uint32
	ncN    [normalCache][3]float64
}

const normalCache = 256

func (s *Scene) shadeBand(ctx context.Context, v *view, sh *shading, buf []uint64, ry0, ry1, oy0, oy1 int, out *target) error {
	const rowsPerTask = 4
	rows := oy1 - oy0
	tasks := (rows + rowsPerTask - 1) / rowsPerTask
	var stop atomic.Bool
	par.Parallel(tasks, par.Count(), func(t int) {
		if stop.Load() {
			return
		}
		if ctx.Err() != nil {
			stop.Store(true)
			return
		}
		w := shader{s: s, v: v, sh: sh, buf: buf, ry0: ry0, ry1: ry1}
		for i := range w.ncID {
			w.ncID[i] = math.MaxUint32
		}
		for oy := oy0 + t*rowsPerTask; oy < min(oy0+(t+1)*rowsPerTask, oy1); oy++ {
			w.row(oy, out)
		}
	})
	if stop.Load() {
		return ctx.Err()
	}
	return nil
}

func (w *shader) key(x, y int) uint64 {
	return w.buf[(y-w.ry0)*w.v.ws+x]
}

func (w *shader) row(oy int, out *target) {
	S := w.v.S
	n := float64(S * S)
	gain := float64(S) / n
	var pix []uint8
	premulOut := out.rgba != nil
	if premulOut {
		pix = out.rgba.Pix[oy*out.rgba.Stride : oy*out.rgba.Stride+out.w*4]
	} else {
		pix = out.nrgba.Pix[oy*out.nrgba.Stride : oy*out.nrgba.Stride+out.w*4]
	}
	bgP := [4]uint8{w.sh.bgN.R, w.sh.bgN.G, w.sh.bgN.B, w.sh.bgN.A}
	if premulOut {
		bgP[0], bgP[1], bgP[2], bgP[3] = premul(w.sh.bgN)
	}
	for ox := 0; ox < out.w; ox++ {
		var sr, sg, sb, edge float64
		cov := 0
		for j := 0; j < S; j++ {
			y := oy*S + j
			for i := 0; i < S; i++ {
				x := ox*S + i
				k := w.key(x, y)
				if k == emptyKey {
					continue
				}
				c, e := w.sample(x, y, k)
				sr += c[0]
				sg += c[1]
				sb += c[2]
				edge += e
				cov++
			}
		}
		o := pix[ox*4 : ox*4+4 : ox*4+4]
		if cov == 0 {
			o[0], o[1], o[2], o[3] = bgP[0], bgP[1], bgP[2], bgP[3]
			continue
		}
		if w.sh.edges && edge > 0 {
			d := 1 - min(1, edge*gain)
			sr *= d
			sg *= d
			sb *= d
		}
		nb := n - float64(cov)
		ba := w.sh.bgA * nb
		alpha := (float64(cov) + ba) / n
		inv := 1 / (float64(cov) + ba)
		r := (sr + w.sh.bg[0]*ba) * inv
		g := (sg + w.sh.bg[1]*ba) * inv
		b := (sb + w.sh.bg[2]*ba) * inv
		a8 := uint8(alpha*255 + 0.5)
		r8, g8, b8 := srgb8(r), srgb8(g), srgb8(b)
		if premulOut {
			a := uint32(a8)
			o[0] = uint8((uint32(r8)*a + 127) / 255)
			o[1] = uint8((uint32(g8)*a + 127) / 255)
			o[2] = uint8((uint32(b8)*a + 127) / 255)
			o[3] = a8
		} else {
			o[0], o[1], o[2], o[3] = r8, g8, b8, a8
		}
	}
}

func (w *shader) triVerts(id uint32) (v0, v1, v2 [3]float64) {
	p := w.s.m.V[int(id)*9 : int(id)*9+9 : int(id)*9+9]
	v0 = [3]float64{float64(p[0]), float64(p[1]), float64(p[2])}
	v1 = [3]float64{float64(p[3]), float64(p[4]), float64(p[5])}
	v2 = [3]float64{float64(p[6]), float64(p[7]), float64(p[8])}
	return
}

func (w *shader) faceNormal(id uint32, toViewer [3]float64) [3]float64 {
	slot := id & (normalCache - 1)
	if w.ncID[slot] != id {
		v0, v1, v2 := w.triVerts(id)
		n := cross(sub(v1, v0), sub(v2, v0))
		l := math.Sqrt(dot(n, n))
		if l > 0 && finite(l) {
			n = scale3(n, 1/l)
		} else {
			n = [3]float64{}
		}
		w.ncID[slot], w.ncN[slot] = id, n
	}
	n := w.ncN[slot]
	if n == ([3]float64{}) {
		return toViewer
	}
	if dot(n, toViewer) < 0 {
		n = scale3(n, -1)
	}
	return n
}

func (w *shader) rayAt(x, y int) (o, d [3]float64, xn, yn float64) {
	v := w.v
	half := float64(v.hs) / 2
	xn = (float64(x) + 0.5 - v.cx) / half
	yn = (v.cy - float64(y) - 0.5) / half
	if v.ortho {
		hw := half / v.k
		o = add(v.tgt, add(scale3(v.fr.r, xn*hw), scale3(v.fr.u, yn*hw)))
		o = sub(o, scale3(v.fr.f, 4*v.R))
		return o, v.fr.f, xn, yn
	}
	d = norm3(add(v.fr.f, add(scale3(v.fr.r, xn*v.tanHalf), scale3(v.fr.u, yn*v.tanHalf))))
	return v.eye, d, xn, yn
}

func (w *shader) sample(x, y int, k uint64) ([3]float64, float64) {
	v := w.v
	id := uint32(k)
	var toViewer [3]float64
	var o, d [3]float64
	if v.ortho {
		toViewer = scale3(v.fr.f, -1)
	} else {
		o, d, _, _ = w.rayAt(x, y)
		toViewer = scale3(d, -1)
	}
	var col [3]float64
	var ng [3]float64
	if v.ortho && !w.s.smooth && w.lastOK && w.last == id {
		col, ng = w.lastC, w.lastN
	} else {
		ng = w.faceNormal(id, toViewer)
		n := ng
		if w.s.smooth {
			if v.ortho {
				o, d, _, _ = w.rayAt(x, y)
			}
			n = w.smoothNormal(id, o, d, ng, toViewer)
		}
		col = w.light(id, n, toViewer)
		w.last, w.lastOK, w.lastC, w.lastN = id, true, col, ng
	}
	e := 0.0
	if w.sh.edges {
		e = w.edge(x, y, k, ng, toViewer)
	}
	return col, e
}

func (w *shader) smoothNormal(id uint32, o, d, ng, toViewer [3]float64) [3]float64 {
	v0, v1, v2 := w.triVerts(id)
	e1, e2 := sub(v1, v0), sub(v2, v0)
	pv := cross(d, e2)
	det := dot(e1, pv)
	if det == 0 || !finite(det) {
		return ng
	}
	inv := 1 / det
	tv := sub(o, v0)
	bu := dot(tv, pv) * inv
	qv := cross(tv, e1)
	bv := dot(d, qv) * inv
	bu = max(0, min(1, bu))
	bv = max(0, min(1-bu, bv))
	bw := 1 - bu - bv
	N := w.s.m.N[int(id)*9 : int(id)*9+9 : int(id)*9+9]
	n := [3]float64{
		bw*float64(N[0]) + bu*float64(N[3]) + bv*float64(N[6]),
		bw*float64(N[1]) + bu*float64(N[4]) + bv*float64(N[7]),
		bw*float64(N[2]) + bu*float64(N[5]) + bv*float64(N[8]),
	}
	l := math.Sqrt(dot(n, n))
	if !(l > 1e-12) || !finite(l) {
		return ng
	}
	n = scale3(n, 1/l)
	if dot(n, ng) < 0 {
		n = scale3(n, -1)
	}
	if c := dot(n, toViewer); c < 0.02 {
		n = norm3(add(n, scale3(toViewer, 0.02-c)))
	}
	return n
}

func (w *shader) light(id uint32, n, toViewer [3]float64) [3]float64 {
	v := w.v
	fr := v.fr
	nc := [3]float64{dot(n, fr.r), dot(n, fr.u), -dot(n, fr.f)}
	vc := [3]float64{dot(toViewer, fr.r), dot(toViewer, fr.u), -dot(toViewer, fr.f)}
	alb := w.sh.base
	if w.s.colors {
		c := w.s.m.C[int(id)*3 : int(id)*3+3 : int(id)*3+3]
		alb = [3]float64{lin8(c[0]), lin8(c[1]), lin8(c[2])}
	}
	amb := 0.14 + 0.12*(0.5+0.5*nc[1])
	kd := max(0, (dot(nc, keyDir)+0.1)/1.1) * 0.85
	fd := max(0, dot(nc, fillDir)) * 0.3
	rd := max(0, dot(nc, rimDir)) * 0.10
	diff := amb + kd + fd + rd
	h := norm3(add(keyDir, vc))
	sp := max(0, dot(nc, h))
	sp *= sp
	sp *= sp
	sp *= sp
	sp *= sp
	sp *= sp
	spec := 0.16 * sp
	fres := 1 - max(0, dot(nc, vc))
	fres *= fres
	fres *= fres
	spec += 0.05 * fres
	return [3]float64{tone(alb[0]*diff + spec), tone(alb[1]*diff + spec), tone(alb[2]*diff + spec)}
}

func tone(c float64) float64 {
	if c <= shoulder {
		return max(0, c)
	}
	return shoulder + (1-shoulder)*(1-math.Exp(-(c-shoulder)/(1-shoulder)))
}

func (w *shader) depthOf(k uint64) float64 {
	q := float64(k>>32) / qScale
	if w.v.ortho {
		return (q - 0.5) * 4 * w.v.R
	}
	return (1 - q) / w.v.near
}

func (w *shader) inside(x, y int) bool {
	return x >= 0 && x < w.v.ws && y >= w.ry0 && y < w.ry1 && y >= 0 && y < w.v.hs
}

func (w *shader) edge(x, y int, k uint64, ng, toViewer [3]float64) float64 {
	s := w.edgeDir(x, y, k, ng, toViewer, 1, 0)
	if s >= 1 {
		return s
	}
	return max(s, w.edgeDir(x, y, k, ng, toViewer, 0, 1))
}

func (w *shader) edgeDir(x, y int, k uint64, ng, toViewer [3]float64, dx, dy int) float64 {
	ax, ay, bx, by := x-dx, y-dy, x+dx, y+dy
	ina, inb := w.inside(ax, ay), w.inside(bx, by)
	ka, kb := k, k
	if ina {
		ka = w.key(ax, ay)
	}
	if inb {
		kb = w.key(bx, by)
	}
	id := uint32(k)
	if ka == emptyKey || kb == emptyKey {
		if ka == emptyKey && !w.bridged(x-2*dx, y-2*dy, k, ng, toViewer) {
			return edgeSilDark
		}
		if kb == emptyKey && !w.bridged(x+2*dx, y+2*dy, k, ng, toViewer) {
			return edgeSilDark
		}
		return 0
	}
	ia, ib := uint32(ka), uint32(kb)
	if ia == id && ib == id {
		return 0
	}
	dc := w.depthOf(k)
	da, db := w.depthOf(ka), w.depthOf(kb)
	var lap, pix float64
	if w.v.ortho {
		lap = da + db - 2*dc
		pix = 1 / w.v.k
	} else {
		if !(dc > 0) {
			return 0
		}
		lap = (2*dc - da - db) / (dc * dc)
		pix = 1 / (dc * w.v.ps)
	}
	if lap > edgeDepthK*pix {
		side := 1
		if math.Abs(da-dc) > math.Abs(db-dc) {
			side = -1
		}
		if !w.bridged(x+2*side*dx, y+2*side*dy, k, ng, toViewer) {
			return edgeSilDark
		}
		return 0
	}
	if ib == id {
		return 0
	}
	nn := w.faceNormal(ib, toViewer)
	c := dot(ng, nn)
	if c >= edgeCreaseMin {
		return 0
	}
	if w.bridged(x+2*dx, y+2*dy, k, ng, toViewer) {
		return 0
	}
	t := min(1, (edgeCreaseMin-c)/(edgeCreaseMin-edgeCreaseMax))
	return edgeCreaseDk * t
}

func (w *shader) bridged(x, y int, k uint64, ng, toViewer [3]float64) bool {
	if !w.inside(x, y) {
		return false
	}
	k2 := w.key(x, y)
	if k2 == emptyKey {
		return false
	}
	if uint32(k2) == uint32(k) {
		return true
	}
	zc, z2 := w.linearDepth(k), w.linearDepth(k2)
	pix := 1 / w.v.k
	if !w.v.ortho {
		pix = zc / w.v.ps
	}
	if math.Abs(z2-zc) > 2*edgeDepthK*pix {
		return false
	}
	return dot(ng, w.faceNormal(uint32(k2), toViewer)) >= edgeCreaseMin
}

func (w *shader) linearDepth(k uint64) float64 {
	q := float64(k>>32) / qScale
	if w.v.ortho {
		return (q - 0.5) * 4 * w.v.R
	}
	return w.v.near / max(1-q, 1e-12)
}

var (
	lin8Table [256]float64
	srgbTable [4097]uint8
)

func init() {
	for i := range lin8Table {
		c := float64(i) / 255
		if c <= 0.04045 {
			lin8Table[i] = c / 12.92
		} else {
			lin8Table[i] = math.Pow((c+0.055)/1.055, 2.4)
		}
	}
	for i := range srgbTable {
		l := float64(i) / 4096
		var c float64
		if l <= 0.0031308 {
			c = l * 12.92
		} else {
			c = 1.055*math.Pow(l, 1/2.4) - 0.055
		}
		srgbTable[i] = uint8(max(0, min(255, c*255+0.5)))
	}
}

func lin8(c uint8) float64 { return lin8Table[c] }

func srgb8(l float64) uint8 {
	if !(l > 0) {
		return 0
	}
	if l >= 1 {
		return 255
	}
	return srgbTable[int(l*4096+0.5)]
}
