package step

import (
	"math"
	"sort"
)

const (
	maxGridLines  = 2048
	maxGridPoints = 1 << 18
	maxSegCross   = 4096
	aspectLimit   = 3.0
	edgeFactor    = 0.4
	gridFactor    = 0.2
)

func (fg *faceGeom) tessellate() {
	fg.v, fg.n = nil, nil
	if fg.bad {
		fg.fail = true
		return
	}
	if fg.tri != nil {
		fg.emitTriSet()
		return
	}
	if !fg.tessUV() {
		fg.v, fg.n = nil, nil
		fg.fail = true
	}
}

func (fg *faceGeom) emitTriSet() {
	ts := fg.tri
	fg.v = make([]float32, 0, len(ts.tris)*9)
	fg.n = make([]float32, 0, len(ts.tris)*9)
	for _, t := range ts.tris {
		var p [3]vec3
		for k := 0; k < 3; k++ {
			p[k] = ts.pos[t[k]]
		}
		g, ok := p[1].sub(p[0]).cross(p[2].sub(p[0])).unit()
		if !ok {
			continue
		}
		for k := 0; k < 3; k++ {
			n := g
			if ts.nrm != nil {
				if q, ok := ts.nrm[t[k]].unit(); ok {
					n = q
				}
			}
			fg.v = append(fg.v, float32(p[k][0]), float32(p[k][1]), float32(p[k][2]))
			fg.n = append(fg.n, float32(n[0]), float32(n[1]), float32(n[2]))
		}
	}
}

type tnode struct {
	uv [2]float64
	p  vec3
	n  bool
}

func (fg *faceGeom) tessUV() bool {
	s := fg.surf
	pu, pv := s.periods()
	fc := &faceCtx{fg: fg, s: s, per: [2]float64{pu, pv}, sign: 1, analytic: isAnalytic(s)}
	if !fg.sense {
		fc.sign = -1
	}
	fc.tol3 = 0.01 * fg.defl
	fc.tolPC = 0.5 * fg.defl
	var loops []loopPoly
	for _, l := range fg.loops {
		lp, ok := fc.buildLoop(l)
		if !ok {
			return false
		}
		loops = append(loops, lp)
	}
	rings, ok := fc.rings(loops)
	if !ok || len(rings) == 0 {
		return false
	}
	box := [4]float64{math.Inf(1), math.Inf(-1), math.Inf(1), math.Inf(-1)}
	for _, r := range rings {
		b := bounds2(r)
		box[0], box[1] = math.Min(box[0], b[0]), math.Max(box[1], b[1])
		box[2], box[3] = math.Min(box[2], b[2]), math.Max(box[3], b[3])
	}
	for _, x := range box {
		if !finite(x) {
			return false
		}
	}
	if !(box[1] > box[0]) || !(box[3] > box[2]) {
		return false
	}
	us, vs := isoLines(s, box, fg.defl*gridFactor, fg.ang)
	nodes, ringIdx, ok := expandRings(fc, rings, us, vs)
	if !ok {
		return false
	}
	su, sv := uvScale(s, box)
	scaled := func(ns []tnode) [][2]float64 {
		out := make([][2]float64, len(ns))
		for i, nd := range ns {
			out[i] = [2]float64{nd.uv[0] * su, nd.uv[1] * sv}
		}
		return out
	}
	crossNodes, crossRings := resolveCrossings(nodes, ringIdx, su, sv)
	if len(crossNodes) > len(nodes) {
		if tris, ok := earFallback(scaled(nodes), ringIdx); ok {
			fg.emit(nodes, tris)
			return true
		}
		nodes, ringIdx = crossNodes, crossRings
	}
	nodes = interiorPoints(s, nodes, ringIdx, us, vs, su, sv, box)
	in := &cdtInput{pts: scaled(nodes), rings: ringIdx}
	tris, ok := triangulate(in)
	if !ok {
		tris, ok = earFallback(in.pts, ringIdx)
		if !ok {
			return false
		}
	}
	fg.emit(nodes, tris)
	return true
}

func uvScale(s surface, b [4]float64) (float64, float64) {
	if _, ok := s.(*planeSurf); ok {
		return 1, 1
	}
	su, sv, n := 0.0, 0.0, 0
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			u := b[0] + (b[1]-b[0])*(0.25+0.25*float64(i))
			v := b[2] + (b[3]-b[2])*(0.25+0.25*float64(j))
			_, du, dv := s.d1(u, v)
			if du.finite() && dv.finite() {
				su += du.norm()
				sv += dv.norm()
				n++
			}
		}
	}
	if n == 0 || !(su > 0) || !(sv > 0) || !finite(su) || !finite(sv) {
		return 1, 1
	}
	su, sv = su/float64(n), sv/float64(n)
	if su > 1e4*sv {
		sv = su / 1e4
	}
	if sv > 1e4*su {
		su = sv / 1e4
	}
	return su, sv
}

func crossings(a, b [2]float64, us, vs []float64) []float64 {
	var ts []float64
	add := func(lines []float64, x0, x1 float64) {
		if x0 == x1 {
			return
		}
		lo, hi := math.Min(x0, x1), math.Max(x0, x1)
		i := sort.SearchFloat64s(lines, lo)
		for ; i < len(lines) && lines[i] < hi && len(ts) < maxSegCross; i++ {
			if lines[i] > lo {
				ts = append(ts, (lines[i]-x0)/(x1-x0))
			}
		}
	}
	add(us, a[0], b[0])
	add(vs, a[1], b[1])
	if len(ts) == 0 {
		return nil
	}
	sort.Float64s(ts)
	out := ts[:0]
	for _, t := range ts {
		if t <= 1e-9 || t >= 1-1e-9 {
			continue
		}
		if len(out) > 0 && t-out[len(out)-1] < 1e-9 {
			continue
		}
		out = append(out, t)
	}
	return out
}

func lerp2(a, b [2]float64, t float64) [2]float64 {
	return [2]float64{a[0] + (b[0]-a[0])*t, a[1] + (b[1]-a[1])*t}
}

type cutPt struct {
	t float64
	p vec3
}

func expandRings(fc *faceCtx, rings [][]bpt, us, vs []float64) ([]tnode, [][]int32, bool) {
	cuts := map[int32][]cutPt{}
	for _, r := range rings {
		n := len(r)
		for i := 0; i < n; i++ {
			pt := r[i]
			if pt.seg != segCut || pt.rev {
				continue
			}
			prev := r[(i-1+n)%n]
			var list []cutPt
			for _, t := range crossings(prev.uv, pt.uv, us, vs) {
				q := lerp2(prev.uv, pt.uv, t)
				p := fc.s.at(q[0], q[1])
				if !p.finite() {
					return nil, nil, false
				}
				list = append(list, cutPt{t: t, p: p})
			}
			cuts[pt.cut] = list
		}
	}
	var nodes []tnode
	var idx [][]int32
	for _, r := range rings {
		n := len(r)
		var ri []int32
		for i := 0; i < n; i++ {
			pt := r[i]
			prev := r[(i-1+n)%n]
			switch pt.seg {
			case segPole:
				for _, t := range crossings(prev.uv, pt.uv, us, vs) {
					ri = append(ri, int32(len(nodes)))
					nodes = append(nodes, tnode{uv: lerp2(prev.uv, pt.uv, t), p: prev.p})
				}
			case segFree:
				for _, t := range crossings(prev.uv, pt.uv, us, vs) {
					q := lerp2(prev.uv, pt.uv, t)
					p := fc.s.at(q[0], q[1])
					if !p.finite() {
						return nil, nil, false
					}
					ri = append(ri, int32(len(nodes)))
					nodes = append(nodes, tnode{uv: q, p: p})
				}
			case segCut:
				list, ok := cuts[pt.cut]
				if !ok {
					return nil, nil, false
				}
				if !pt.rev {
					for _, c := range list {
						ri = append(ri, int32(len(nodes)))
						nodes = append(nodes, tnode{uv: lerp2(prev.uv, pt.uv, c.t), p: c.p})
					}
				} else {
					for k := len(list) - 1; k >= 0; k-- {
						c := list[k]
						ri = append(ri, int32(len(nodes)))
						nodes = append(nodes, tnode{uv: lerp2(pt.uv, prev.uv, c.t), p: c.p})
					}
				}
			}
			ri = append(ri, int32(len(nodes)))
			nodes = append(nodes, tnode{uv: pt.uv, p: pt.p})
		}
		if len(ri) >= 2 {
			idx = append(idx, ri)
		}
	}
	return nodes, idx, len(nodes) >= 3
}

func isoLines(s surface, b [4]float64, defl, ang float64) ([]float64, []float64) {
	var us, vs []float64
	switch k := s.(type) {
	case *planeSurf:
		return nil, nil
	case *cylSurf:
		us = uniformInterior(b[0], b[1], arcStep(k.r, defl, ang))
	case *coneSurf:
		r := math.Max(math.Abs(k.r+b[2]*k.tn), math.Abs(k.r+b[3]*k.tn))
		us = uniformInterior(b[0], b[1], arcStep(r, defl, ang))
	case *sphereSurf:
		st := arcStep(k.r, defl, ang)
		us = uniformInterior(b[0], b[1], st)
		vs = uniformInterior(b[2], b[3], st)
	case *torusSurf:
		us = uniformInterior(b[0], b[1], arcStep(k.rb+k.rs, defl, ang))
		vs = uniformInterior(b[2], b[3], arcStep(k.rs, defl, ang))
	default:
		us = densityLines(s, b, 0, defl, ang)
		vs = densityLines(s, b, 1, defl, ang)
	}
	lu := isoLength(s, b, 0)
	lv := isoLength(s, b, 1)
	if len(us) > 0 || len(vs) > 0 {
		hu := lu / float64(len(us)+1)
		hv := lv / float64(len(vs)+1)
		if len(us) > 0 && (len(vs) == 0 || hv > aspectLimit*hu) && hu > 0 {
			n := int(math.Ceil(lv/(aspectLimit*hu))) - 1
			if n > len(vs) {
				vs = uniformInterior(b[2], b[3], (b[3]-b[2])/float64(min(n, maxGridLines)+1))
			}
		} else if len(vs) > 0 && (len(us) == 0 || hu > aspectLimit*hv) && hv > 0 {
			n := int(math.Ceil(lu/(aspectLimit*hv))) - 1
			if n > len(us) {
				us = uniformInterior(b[0], b[1], (b[1]-b[0])/float64(min(n, maxGridLines)+1))
			}
		}
	}
	us = thin(us, maxGridLines)
	vs = thin(vs, maxGridLines)
	for len(us)*len(vs) > maxGridPoints {
		if len(us) > len(vs) {
			us = thin(us, len(us)/2)
		} else {
			vs = thin(vs, len(vs)/2)
		}
	}
	return us, vs
}

func thin(x []float64, n int) []float64 {
	if len(x) <= n {
		return x
	}
	if n <= 0 {
		return nil
	}
	out := make([]float64, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, x[(2*i+1)*len(x)/(2*n)])
	}
	return out
}

func uniformInterior(a, b, step float64) []float64 {
	if !(step > 0) || !(b > a) {
		return nil
	}
	n := int(math.Ceil((b - a) / step))
	if n <= 1 {
		return nil
	}
	n = min(n, maxGridLines+1)
	out := make([]float64, 0, n-1)
	for i := 1; i < n; i++ {
		out = append(out, a+(b-a)*float64(i)/float64(n))
	}
	return out
}

func isoLength(s surface, b [4]float64, dir int) float64 {
	total := 0.0
	for k := 0; k < 3; k++ {
		o := b[2*(1-dir)] + (b[2*(1-dir)+1]-b[2*(1-dir)])*(0.25+0.25*float64(k))
		var prev vec3
		for i := 0; i <= 16; i++ {
			t := b[2*dir] + (b[2*dir+1]-b[2*dir])*float64(i)/16
			var p vec3
			if dir == 0 {
				p = s.at(t, o)
			} else {
				p = s.at(o, t)
			}
			if i > 0 && p.finite() && prev.finite() {
				total += dist(p, prev)
			}
			prev = p
		}
	}
	return total / 3
}

func densityLines(s surface, b [4]float64, dir int, defl, ang float64) []float64 {
	a0, a1 := b[2*dir], b[2*dir+1]
	o0, o1 := b[2*(1-dir)], b[2*(1-dir)+1]
	ns := 48
	if bs, ok := s.(*bspSurf); ok {
		kn, p, ncp := bs.ku, bs.pu, bs.nu
		if dir == 1 {
			kn, p, ncp = bs.kv, bs.pv, bs.nv
		}
		spans := 0
		for i := p; i < ncp; i++ {
			if kn[i+1] > kn[i] && kn[i+1] > a0 && kn[i] < a1 {
				spans++
			}
		}
		ns = min(max(24, spans*8), 400)
	}
	h := (a1 - a0) / float64(ns)
	if !(h > 0) {
		return nil
	}
	dens := make([]float64, ns)
	pts := make([]vec3, ns+1)
	for k := 0; k < 5; k++ {
		o := o0 + (o1-o0)*float64(k)/4
		for i := 0; i <= ns; i++ {
			t := a0 + h*float64(i)
			if dir == 0 {
				pts[i] = s.at(t, o)
			} else {
				pts[i] = s.at(o, t)
			}
		}
		for i := 1; i < ns; i++ {
			d1 := pts[i+1].sub(pts[i-1]).mul(1 / (2 * h))
			d2 := pts[i+1].sub(pts[i].mul(2)).add(pts[i-1]).mul(1 / (h * h))
			sp := d1.norm()
			if !(sp > 1e-300) || !d2.finite() {
				continue
			}
			tu := d1.mul(1 / sp)
			perp := d2.sub(tu.mul(d2.dot(tu))).norm()
			if !(perp > 1e-300) {
				continue
			}
			step := math.Min(math.Sqrt(8*defl/perp), ang*sp/perp)
			r := 1 / step
			if !finite(r) {
				continue
			}
			dens[i-1] = math.Max(dens[i-1], r)
			dens[i] = math.Max(dens[i], r)
		}
	}
	total := 0.0
	for _, d := range dens {
		total += d * h
	}
	if !finite(total) {
		return nil
	}
	n := int(math.Ceil(total))
	if n <= 1 {
		return nil
	}
	n = min(n, maxGridLines+1)
	out := make([]float64, 0, n-1)
	acc := 0.0
	j := 1
	for i := 0; i < ns && j < n; i++ {
		seg := dens[i] * h
		for j < n && acc+seg >= float64(j)*total/float64(n) {
			f := 0.0
			if seg > 0 {
				f = (float64(j)*total/float64(n) - acc) / seg
			}
			out = append(out, a0+h*(float64(i)+f))
			j++
		}
		acc += seg
	}
	return out
}

func interiorPoints(s surface, nodes []tnode, rings [][]int32, us, vs []float64, su, sv float64, box [4]float64) []tnode {
	if len(us) == 0 || len(vs) == 0 {
		return nodes
	}
	type seg struct{ ax, ay, bx, by float64 }
	var segs []seg
	for _, r := range rings {
		for i := range r {
			a, b := nodes[r[i]].uv, nodes[r[(i+1)%len(r)]].uv
			segs = append(segs, seg{a[0] * su, a[1] * sv, b[0] * su, b[1] * sv})
		}
	}
	ys := make([]float64, len(vs))
	for j, v := range vs {
		ys[j] = v * sv
	}
	xs := make([]float64, len(us))
	for i, u := range us {
		xs[i] = u * su
	}
	rowX := make([][]float64, len(vs))
	for _, sg := range segs {
		lo, hi := math.Min(sg.ay, sg.by), math.Max(sg.ay, sg.by)
		if lo == hi {
			continue
		}
		j := sort.SearchFloat64s(ys, lo)
		for ; j < len(ys) && ys[j] < hi; j++ {
			y := ys[j]
			if (sg.ay <= y) == (sg.by <= y) {
				continue
			}
			x := sg.ax + (y-sg.ay)*(sg.bx-sg.ax)/(sg.by-sg.ay)
			rowX[j] = append(rowX[j], x)
		}
	}
	spacing := func(lines []float64, i int, lo, hi float64) float64 {
		a, b := lo, hi
		if i > 0 {
			a = lines[i-1]
		}
		if i+1 < len(lines) {
			b = lines[i+1]
		}
		return math.Min(lines[i]-a, b-lines[i])
	}
	cell := 0.0
	for i := range xs {
		cell = math.Max(cell, spacing(us, i, box[0], box[1])*su)
	}
	for j := range ys {
		cell = math.Max(cell, spacing(vs, j, box[2], box[3])*sv)
	}
	if len(us) == 1 || len(vs) == 1 {
		cell = math.Max(cell, 1e-300)
	}
	minX, maxX, minY, maxY := math.Inf(1), math.Inf(-1), math.Inf(1), math.Inf(-1)
	for _, sg := range segs {
		minX = math.Min(minX, math.Min(sg.ax, sg.bx))
		maxX = math.Max(maxX, math.Max(sg.ax, sg.bx))
		minY = math.Min(minY, math.Min(sg.ay, sg.by))
		maxY = math.Max(maxY, math.Max(sg.ay, sg.by))
	}
	if !(cell > 0) || !finite(cell) {
		cell = math.Max(maxX-minX, maxY-minY) / 64
	}
	gw := int((maxX-minX)/cell) + 1
	gh := int((maxY-minY)/cell) + 1
	for gw*gh > 1<<20 {
		cell *= 2
		gw = int((maxX-minX)/cell) + 1
		gh = int((maxY-minY)/cell) + 1
	}
	if gw < 1 || gh < 1 {
		return nodes
	}
	buckets := make([][]int32, gw*gh)
	cellOf := func(x, y float64) (int, int) {
		cx := int((x - minX) / cell)
		cy := int((y - minY) / cell)
		return min(max(cx, 0), gw-1), min(max(cy, 0), gh-1)
	}
	for k, sg := range segs {
		x0, y0 := cellOf(math.Min(sg.ax, sg.bx), math.Min(sg.ay, sg.by))
		x1, y1 := cellOf(math.Max(sg.ax, sg.bx), math.Max(sg.ay, sg.by))
		if (x1-x0+1)*(y1-y0+1) > 1<<16 {
			continue
		}
		for cy := y0; cy <= y1; cy++ {
			for cx := x0; cx <= x1; cx++ {
				buckets[cy*gw+cx] = append(buckets[cy*gw+cx], int32(k))
			}
		}
	}
	near := func(x, y, r float64) bool {
		cx, cy := cellOf(x, y)
		rr := r * r
		for dy := -1; dy <= 1; dy++ {
			for dx := -1; dx <= 1; dx++ {
				gx, gy := cx+dx, cy+dy
				if gx < 0 || gy < 0 || gx >= gw || gy >= gh {
					continue
				}
				for _, k := range buckets[gy*gw+gx] {
					sg := segs[k]
					ex, ey := sg.bx-sg.ax, sg.by-sg.ay
					l2 := ex*ex + ey*ey
					t := 0.0
					if l2 > 0 {
						t = ((x-sg.ax)*ex + (y-sg.ay)*ey) / l2
						t = math.Max(0, math.Min(1, t))
					}
					px, py := sg.ax+t*ex-x, sg.ay+t*ey-y
					if px*px+py*py < rr {
						return true
					}
				}
			}
		}
		return false
	}
	for jj := range vs {
		j := jj
		row := rowX[j]
		if len(row) < 2 {
			continue
		}
		sort.Float64s(row)
		hv := spacing(vs, j, box[2], box[3]) * sv
		start := len(nodes)
		for k := 0; k+1 < len(row); k += 2 {
			x0, x1 := row[k], row[k+1]
			i := sort.SearchFloat64s(xs, x0)
			for ; i < len(xs) && xs[i] < x1; i++ {
				if xs[i] <= x0 {
					continue
				}
				hu := spacing(us, i, box[0], box[1]) * su
				r := 0.45 * math.Min(hu, hv)
				if near(xs[i], ys[j], r) {
					continue
				}
				uv := [2]float64{us[i], vs[j]}
				p := s.at(uv[0], uv[1])
				if !p.finite() {
					continue
				}
				nodes = append(nodes, tnode{uv: uv, p: p, n: true})
			}
		}
		if jj%2 == 1 {
			seg := nodes[start:]
			for a, b := 0, len(seg)-1; a < b; a, b = a+1, b-1 {
				seg[a], seg[b] = seg[b], seg[a]
			}
		}
	}
	return nodes
}

func (fg *faceGeom) emit(nodes []tnode, tris [][3]int32) {
	s := fg.surf
	sign := 1.0
	if !fg.sense {
		sign = -1
	}
	nrm := make([]vec3, len(nodes))
	nok := make([]bool, len(nodes))
	for i, nd := range nodes {
		if n, ok := surfNormal(s, nd.uv[0], nd.uv[1]); ok {
			nrm[i], nok[i] = n.mul(sign), true
		}
	}
	fg.v = make([]float32, 0, len(tris)*9)
	fg.n = make([]float32, 0, len(tris)*9)
	for _, t := range tris {
		a, b, c := t[0], t[1], t[2]
		if a < 0 || b < 0 || c < 0 {
			continue
		}
		if sign < 0 {
			b, c = c, b
		}
		pa, pb, pc := nodes[a].p, nodes[b].p, nodes[c].p
		if pa == pb || pb == pc || pa == pc {
			continue
		}
		g, gok := pb.sub(pa).cross(pc.sub(pa)).unit()
		var cn vec3
		cok := false
		for _, id := range [3]int32{a, b, c} {
			n := nrm[id]
			if !nok[id] {
				if !cok {
					uv := [2]float64{
						(nodes[a].uv[0] + nodes[b].uv[0] + nodes[c].uv[0]) / 3,
						(nodes[a].uv[1] + nodes[b].uv[1] + nodes[c].uv[1]) / 3,
					}
					if q, ok := surfNormal(s, uv[0], uv[1]); ok {
						cn, cok = q.mul(sign), true
					} else if gok {
						cn, cok = g, true
					} else {
						cn, cok = vec3{0, 0, 1}, true
					}
				}
				n = cn
			}
			p := nodes[id].p
			fg.v = append(fg.v, float32(p[0]), float32(p[1]), float32(p[2]))
			fg.n = append(fg.n, float32(n[0]), float32(n[1]), float32(n[2]))
		}
	}
}

func orient2(ax, ay, bx, by, cx, cy float64) float64 {
	return (bx-ax)*(cy-ay) - (by-ay)*(cx-ax)
}

func resolveCrossings(nodes []tnode, rings [][]int32, su, sv float64) ([]tnode, [][]int32) {
	type sref struct {
		r, i int
	}
	var segs []sref
	minX, minY, maxX, maxY := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	total := 0.0
	for r, ring := range rings {
		for i := range ring {
			segs = append(segs, sref{r, i})
			a, b := nodes[ring[i]].uv, nodes[ring[(i+1)%len(ring)]].uv
			minX, maxX = math.Min(minX, a[0]*su), math.Max(maxX, a[0]*su)
			minY, maxY = math.Min(minY, a[1]*sv), math.Max(maxY, a[1]*sv)
			total += math.Hypot((b[0]-a[0])*su, (b[1]-a[1])*sv)
		}
	}
	if len(segs) < 4 || len(segs) > 1<<20 {
		return nodes, rings
	}
	ext := math.Max(maxX-minX, maxY-minY)
	if !(ext > 0) {
		return nodes, rings
	}
	cell := math.Max(total/float64(len(segs)), ext/512)
	gw := int((maxX-minX)/cell) + 1
	gh := int((maxY-minY)/cell) + 1
	grid := map[int][]int32{}
	end := func(s sref) ([2]float64, [2]float64, int32, int32) {
		ring := rings[s.r]
		ia, ib := ring[s.i], ring[(s.i+1)%len(ring)]
		a, b := nodes[ia].uv, nodes[ib].uv
		return [2]float64{a[0] * su, a[1] * sv}, [2]float64{b[0] * su, b[1] * sv}, ia, ib
	}
	for k, s := range segs {
		a, b, _, _ := end(s)
		x0 := int((math.Min(a[0], b[0]) - minX) / cell)
		x1 := int((math.Max(a[0], b[0]) - minX) / cell)
		y0 := int((math.Min(a[1], b[1]) - minY) / cell)
		y1 := int((math.Max(a[1], b[1]) - minY) / cell)
		if (x1-x0+1)*(y1-y0+1) > 4096 {
			continue
		}
		for y := y0; y <= y1; y++ {
			for x := x0; x <= x1; x++ {
				grid[y*gw+x] = append(grid[y*gw+x], int32(k))
			}
		}
	}
	_ = gh
	type hit struct {
		t    float64
		node int32
	}
	hits := map[int][]hit{}
	seen := map[[2]int32]bool{}
	eps := 1e-14 * ext * ext
	for _, list := range grid {
		if len(list) > 256 {
			continue
		}
		for x := 0; x < len(list); x++ {
			for y := x + 1; y < len(list); y++ {
				i, j := list[x], list[y]
				key := [2]int32{min(i, j), max(i, j)}
				if seen[key] {
					continue
				}
				seen[key] = true
				a, b, ia, ib := end(segs[i])
				c, d, ic, id := end(segs[j])
				if ia == ic || ia == id || ib == ic || ib == id {
					continue
				}
				o1 := orient2(a[0], a[1], b[0], b[1], c[0], c[1])
				o2 := orient2(a[0], a[1], b[0], b[1], d[0], d[1])
				o3 := orient2(c[0], c[1], d[0], d[1], a[0], a[1])
				o4 := orient2(c[0], c[1], d[0], d[1], b[0], b[1])
				if !((o1 > eps && o2 < -eps) || (o1 < -eps && o2 > eps)) {
					continue
				}
				if !((o3 > eps && o4 < -eps) || (o3 < -eps && o4 > eps)) {
					continue
				}
				ta := o3 / (o3 - o4)
				tb := o1 / (o1 - o2)
				na, nb := nodes[ia], nodes[ib]
				nn := tnode{uv: lerp2(na.uv, nb.uv, ta), p: na.p.add(nb.p.sub(na.p).mul(ta))}
				idx := int32(len(nodes))
				nodes = append(nodes, nn)
				hits[int(i)] = append(hits[int(i)], hit{ta, idx})
				hits[int(j)] = append(hits[int(j)], hit{tb, idx})
			}
		}
	}
	if len(hits) == 0 {
		return nodes, rings
	}
	out := make([][]int32, len(rings))
	k := 0
	for r, ring := range rings {
		var nr []int32
		for i := range ring {
			nr = append(nr, ring[i])
			hs := hits[k]
			k++
			sort.Slice(hs, func(x, y int) bool { return hs[x].t < hs[y].t })
			for _, h := range hs {
				nr = append(nr, h.node)
			}
		}
		out[r] = nr
	}
	return nodes, out
}
