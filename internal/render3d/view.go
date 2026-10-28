package render3d

import (
	"math"

	"github.com/oops1/PixGo/internal/par"
)

type frame struct {
	r, u, f [3]float64
}

func frameOf(yaw, pitch float64) frame {
	sy, cy := math.Sincos(yaw)
	sp, cp := math.Sincos(pitch)
	return frame{
		r: [3]float64{cy, -sy, 0},
		u: [3]float64{sy * sp, cy * sp, cp},
		f: [3]float64{cp * sy, cp * cy, -sp},
	}
}

type view struct {
	ortho   bool
	S       int
	ws, hs  int
	fr      frame
	center  [3]float64
	tgt     [3]float64
	eye     [3]float64
	R       float64
	H       float64
	k       float64
	cx, cy  float64
	tanHalf float64
	ps      float64
	D       float64
	near    float64
	ax, ay  [4]float64
	aq      [4]float64
	az      [4]float64
}

func (s *Scene) targetOf(fr frame, cam Camera, H float64) [3]float64 {
	return sub(s.center, add(scale3(fr.r, cam.PanX*H), scale3(fr.u, cam.PanY*H)))
}

func (s *Scene) applyFit(cam Camera, w, h int) Camera {
	if cam.Fit > 0 {
		return cam
	}
	H, mx, my := s.autoFit(cam, float64(w)/float64(h))
	cam.Fit = H
	cam.PanX -= mx / H
	cam.PanY -= my / H
	return cam
}

func (s *Scene) newView(cam Camera, w, h, S int) *view {
	cam = s.applyFit(sanitize(cam), w, h)
	v := &view{S: S, ws: w * S, hs: h * S, R: s.radius, center: s.center}
	v.fr = frameOf(cam.Yaw, cam.Pitch)
	H := cam.Fit
	v.H = H
	v.cx, v.cy = float64(v.ws)/2, float64(v.hs)/2
	v.tgt = s.targetOf(v.fr, cam, H)
	r, u, f := v.fr.r, v.fr.u, v.fr.f
	if cam.FOV == 0 {
		v.ortho = true
		v.k = cam.Zoom / H * float64(v.hs) / 2
		v.ax = [4]float64{v.k * r[0], v.k * r[1], v.k * r[2], v.cx - v.k*dot(r, v.tgt)}
		v.ay = [4]float64{-v.k * u[0], -v.k * u[1], -v.k * u[2], v.cy + v.k*dot(u, v.tgt)}
		qs := 1 / (4 * v.R)
		v.aq = [4]float64{qs * f[0], qs * f[1], qs * f[2], 0.5 - qs*dot(f, v.center)}
		return v
	}
	v.tanHalf = math.Tan(cam.FOV / 2)
	v.ps = float64(v.hs) / 2 / v.tanHalf
	v.D = H / (cam.Zoom * v.tanHalf)
	v.eye = sub(v.tgt, scale3(f, v.D))
	v.near = math.Max(v.D-1.02*v.R, 1e-3*v.R)
	v.ax = [4]float64{r[0], r[1], r[2], -dot(r, v.tgt)}
	v.ay = [4]float64{u[0], u[1], u[2], -dot(u, v.tgt)}
	v.az = [4]float64{f[0], f[1], f[2], -dot(f, v.eye)}
	return v
}

type extentKey struct {
	yaw, pitch, fov, aspect float64
}

type extents struct {
	xlo, xhi, ylo, yhi float64
	d, mx, my          float64
	ok                 bool
}

func (s *Scene) extentsFor(yaw, pitch float64) extents {
	return s.extentsCached(extentKey{yaw: yaw, pitch: pitch})
}

func (s *Scene) extentsCached(key extentKey) extents {
	slot := 0
	if key.fov > 0 {
		slot = 1
	}
	s.mu.Lock()
	if s.extOK[slot] && s.extKey[slot] == key {
		e := s.extVal[slot]
		s.mu.Unlock()
		return e
	}
	s.mu.Unlock()
	var e extents
	if key.fov > 0 {
		e = s.perspExtents(key)
	} else {
		fr := frameOf(key.yaw, key.pitch)
		e = s.scan(func(e *extents, d [3]float64) {
			x, y := dot(d, fr.r), dot(d, fr.u)
			e.xlo, e.xhi = min(e.xlo, x), max(e.xhi, x)
			e.ylo, e.yhi = min(e.ylo, y), max(e.yhi, y)
		})
	}
	s.mu.Lock()
	s.extKey[slot], s.extVal[slot], s.extOK[slot] = key, e, true
	s.mu.Unlock()
	return e
}

func (s *Scene) perspExtents(key extentKey) extents {
	fr := frameOf(key.yaw, key.pitch)
	th := math.Tan(key.fov / 2)
	tx, ty := th*key.aspect*fitMargin, th*fitMargin
	e := s.scan(func(e *extents, d [3]float64) {
		x, y, zc := dot(d, fr.r), dot(d, fr.u), -dot(d, fr.f)
		e.xhi, e.xlo = max(e.xhi, x+tx*zc), min(e.xlo, x-tx*zc)
		e.yhi, e.ylo = max(e.yhi, y+ty*zc), min(e.ylo, y-ty*zc)
		e.d = max(e.d, zc)
	})
	if !e.ok {
		return e
	}
	D := max((e.xhi-e.xlo)/(2*tx), (e.yhi-e.ylo)/(2*ty), e.d+1e-3*s.radius)
	mx, my := (e.xhi+e.xlo)/2, (e.yhi+e.ylo)/2
	lx, ux := e.xhi-tx*D, e.xlo+tx*D
	ly, uy := e.yhi-ty*D, e.ylo+ty*D
	for range 3 {
		x, y := s.ndcExtremes(fr, D, mx, my)
		if dx := x.shift(); finite(dx) {
			mx = max(lx, min(ux, mx+dx))
		}
		if dy := y.shift(); finite(dy) {
			my = max(ly, min(uy, my+dy))
		}
	}
	e.d, e.mx, e.my = D, mx, my
	return e
}

type extreme struct {
	lo, hi   float64
	zlo, zhi float64
}

func (e extreme) shift() float64 {
	if !(e.zlo > 0) || !(e.zhi > 0) {
		return 0
	}
	return (e.hi + e.lo) / (1/e.zhi + 1/e.zlo)
}

func (e *extreme) add(v, z float64) {
	if v < e.lo {
		e.lo, e.zlo = v, z
	}
	if v > e.hi {
		e.hi, e.zhi = v, z
	}
}

func (s *Scene) ndcExtremes(fr frame, D, mx, my float64) (extreme, extreme) {
	vtx := s.m.V
	const chunk = 1 << 15
	tasks := (s.tris + chunk - 1) / chunk
	xs := make([]extreme, tasks)
	ys := make([]extreme, tasks)
	c := s.center
	empty := extreme{lo: math.Inf(1), hi: math.Inf(-1)}
	par.Parallel(tasks, par.Count(), func(t int) {
		ex, ey := empty, empty
		end := min((t+1)*chunk, s.tris)
		for i := t * chunk; i < end; i++ {
			p := vtx[i*9 : i*9+9 : i*9+9]
			if !finiteTri(p) {
				continue
			}
			for j := 0; j < 9; j += 3 {
				d := [3]float64{float64(p[j]) - c[0], float64(p[j+1]) - c[1], float64(p[j+2]) - c[2]}
				z := D + dot(d, fr.f)
				if !(z > 0) {
					continue
				}
				ex.add((dot(d, fr.r)-mx)/z, z)
				ey.add((dot(d, fr.u)-my)/z, z)
			}
		}
		xs[t], ys[t] = ex, ey
	})
	ex, ey := empty, empty
	for t := range tasks {
		ex.add(xs[t].lo, xs[t].zlo)
		ex.add(xs[t].hi, xs[t].zhi)
		ey.add(ys[t].lo, ys[t].zlo)
		ey.add(ys[t].hi, ys[t].zhi)
	}
	return ex, ey
}

func (s *Scene) scan(fn func(e *extents, d [3]float64)) extents {
	vtx := s.m.V
	const chunk = 1 << 15
	tasks := (s.tris + chunk - 1) / chunk
	res := make([]extents, tasks)
	c := s.center
	empty := extents{xlo: math.Inf(1), xhi: math.Inf(-1), ylo: math.Inf(1), yhi: math.Inf(-1), d: math.Inf(-1)}
	par.Parallel(tasks, par.Count(), func(t int) {
		e := empty
		end := min((t+1)*chunk, s.tris)
		for i := t * chunk; i < end; i++ {
			p := vtx[i*9 : i*9+9 : i*9+9]
			if !finiteTri(p) {
				continue
			}
			e.ok = true
			for j := 0; j < 9; j += 3 {
				fn(&e, [3]float64{float64(p[j]) - c[0], float64(p[j+1]) - c[1], float64(p[j+2]) - c[2]})
			}
		}
		res[t] = e
	})
	out := empty
	for _, e := range res {
		if !e.ok {
			continue
		}
		out.ok = true
		out.xlo, out.xhi = min(out.xlo, e.xlo), max(out.xhi, e.xhi)
		out.ylo, out.yhi = min(out.ylo, e.ylo), max(out.yhi, e.yhi)
		out.d = max(out.d, e.d)
	}
	return out
}

func (s *Scene) autoFit(cam Camera, aspect float64) (float64, float64, float64) {
	if s.tris == 0 || !(aspect > 0) || !finite(aspect) {
		return s.radius, 0, 0
	}
	if cam.FOV == 0 {
		e := s.extentsFor(cam.Yaw, cam.Pitch)
		if !e.ok {
			return s.radius, 0, 0
		}
		h := max(e.yhi-e.ylo, (e.xhi-e.xlo)/aspect) / 2 / fitMargin
		if !(h > 1e-9*s.radius) || !finite(h) {
			return s.radius, 0, 0
		}
		return h, (e.xhi + e.xlo) / 2, (e.yhi + e.ylo) / 2
	}
	e := s.extentsCached(extentKey{yaw: cam.Yaw, pitch: cam.Pitch, fov: cam.FOV, aspect: aspect})
	if !e.ok || !finite(e.d) || e.d <= 0 {
		return s.radius, 0, 0
	}
	h := e.d * math.Tan(cam.FOV/2)
	if !(h > 1e-9*s.radius) || !finite(h) || !finite(e.mx) || !finite(e.my) {
		return s.radius, 0, 0
	}
	return h, e.mx, e.my
}

func (s *Scene) rayCast(o, d [3]float64) (float64, bool) {
	if s.tris == 0 {
		return 0, false
	}
	vtx := s.m.V
	const chunk = 1 << 15
	tasks := (s.tris + chunk - 1) / chunk
	best := make([]float64, tasks)
	par.Parallel(tasks, par.Count(), func(t int) {
		b := math.Inf(1)
		end := min((t+1)*chunk, s.tris)
		for i := t * chunk; i < end; i++ {
			p := vtx[i*9 : i*9+9 : i*9+9]
			if !finiteTri(p) {
				continue
			}
			v0 := [3]float64{float64(p[0]), float64(p[1]), float64(p[2])}
			e1 := [3]float64{float64(p[3]) - v0[0], float64(p[4]) - v0[1], float64(p[5]) - v0[2]}
			e2 := [3]float64{float64(p[6]) - v0[0], float64(p[7]) - v0[1], float64(p[8]) - v0[2]}
			pv := cross(d, e2)
			det := dot(e1, pv)
			if det == 0 || !finite(det) {
				continue
			}
			inv := 1 / det
			tv := sub(o, v0)
			bu := dot(tv, pv) * inv
			if bu < 0 || bu > 1 {
				continue
			}
			qv := cross(tv, e1)
			bv := dot(d, qv) * inv
			if bv < 0 || bu+bv > 1 {
				continue
			}
			tt := dot(e2, qv) * inv
			if tt > 0 && tt < b {
				b = tt
			}
		}
		best[t] = b
	})
	b := math.Inf(1)
	for _, x := range best {
		b = min(b, x)
	}
	return b, !math.IsInf(b, 1)
}

func dot(a, b [3]float64) float64 { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }

func cross(a, b [3]float64) [3]float64 {
	return [3]float64{a[1]*b[2] - a[2]*b[1], a[2]*b[0] - a[0]*b[2], a[0]*b[1] - a[1]*b[0]}
}

func add(a, b [3]float64) [3]float64 { return [3]float64{a[0] + b[0], a[1] + b[1], a[2] + b[2]} }

func sub(a, b [3]float64) [3]float64 { return [3]float64{a[0] - b[0], a[1] - b[1], a[2] - b[2]} }

func scale3(a [3]float64, k float64) [3]float64 { return [3]float64{a[0] * k, a[1] * k, a[2] * k} }

func norm3(a [3]float64) [3]float64 {
	l := math.Sqrt(dot(a, a))
	if l == 0 || !finite(l) {
		return a
	}
	return scale3(a, 1/l)
}
