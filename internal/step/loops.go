package step

import (
	"math"
)

const (
	segEdge uint8 = iota
	segPole
	segFree
	segCut
)

type bpt struct {
	uv  [2]float64
	p   vec3
	seg uint8
	cut int32
	rev bool
}

type loopPoly struct {
	pts   []bpt
	wind  [2]int
	area  float64
	valid bool
}

type faceCtx struct {
	fg       *faceGeom
	s        surface
	per      [2]float64
	tol3     float64
	tolPC    float64
	sign     float64
	cuts     int32
	analytic bool
}

func shoelace(pts []bpt) float64 {
	a := 0.0
	n := len(pts)
	for i := 0; i < n; i++ {
		p, q := pts[i].uv, pts[(i+1)%n].uv
		a += p[0]*q[1] - q[0]*p[1]
	}
	return a / 2
}

func (fc *faceCtx) degenerate(a, b [2]float64, p vec3) bool {
	for k := 1; k <= 5; k++ {
		t := float64(k) / 6
		q := fc.s.at(a[0]+(b[0]-a[0])*t, a[1]+(b[1]-a[1])*t)
		if !q.finite() || dist(q, p) > fc.tol3 {
			return false
		}
	}
	return true
}

func (fc *faceCtx) useUV(u edgeUse, hint [2]float64, hasHint bool) ([]bpt, bool) {
	var pts []vec3
	var ts []float64
	var pc curve
	if u.poly != nil {
		pts = u.poly
	} else {
		e := u.e
		if e == nil || !e.ok || len(e.pts) == 0 {
			return nil, false
		}
		pts, ts = e.pts, e.ts
		for _, p := range e.pcs {
			if p.surf == fc.fg.sid {
				pc = p.c
				break
			}
		}
	}
	n := len(pts)
	out := make([]bpt, n)
	inverted := make([]bool, n)
	for k := 0; k < n; k++ {
		idx := k
		if !u.fwd {
			idx = n - 1 - k
		}
		p := pts[idx]
		var uv [2]float64
		ok := false
		havePC := false
		if pc != nil {
			q := pc.at(ts[idx])
			uv = [2]float64{q[0], q[1]}
			havePC = finite(uv[0]) && finite(uv[1])
		}
		if havePC {
			if fc.analytic {
				r, good := invertSurf(fc.s, p, uv, true)
				if good && finite(r[0]) && finite(r[1]) {
					_, su, sv := fc.s.d1(r[0], r[1])
					for d, der := range [2]vec3{su, sv} {
						if fc.per[d] > 0 {
							r[d] += math.Round((uv[d]-r[d])/fc.per[d]) * fc.per[d]
						}
						if der.norm() < 1e-9*(1+p.norm()) {
							r[d] = uv[d]
						}
					}
					uv, ok = r, true
				}
			} else {
				e0 := dist(fc.s.at(uv[0], uv[1]), p)
				r := newtonSurf(fc.s, p, uv)
				if e1 := dist(fc.s.at(r[0], r[1]), p); e1 <= e0 {
					uv, e0 = r, e1
				}
				ok = e0 <= fc.tolPC
			}
		}
		if !ok {
			h, hh := hint, hasHint
			if pc != nil && finite(uv[0]) && finite(uv[1]) {
				h, hh = uv, true
			}
			r, good := invertSurf(fc.s, p, h, hh)
			if !good || !finite(r[0]) || !finite(r[1]) {
				return nil, false
			}
			uv = r
			inverted[k] = true
		}
		if k > 0 {
			prev := out[k-1].uv
			for d := 0; d < 2; d++ {
				if fc.per[d] > 0 {
					uv[d] += math.Round((prev[d]-uv[d])/fc.per[d]) * fc.per[d]
				}
			}
		}
		out[k] = bpt{uv: uv, p: p}
		hint, hasHint = uv, true
	}
	for k := 0; k < n; k++ {
		if !inverted[k] || n < 2 {
			continue
		}
		_, su, sv := fc.s.d1(out[k].uv[0], out[k].uv[1])
		nb := k - 1
		if nb < 0 {
			nb = k + 1
		}
		for d, der := range [2]vec3{su, sv} {
			if der.norm() < 1e-9*(1+out[k].p.norm()) {
				out[k].uv[d] = out[nb].uv[d]
			}
		}
	}
	return out, true
}

func (fc *faceCtx) buildLoop(l loopGeom) (loopPoly, bool) {
	n := len(l.uses)
	uvs := make([][]bpt, n)
	var hint [2]float64
	hasHint := false
	for i, u := range l.uses {
		pts, ok := fc.useUV(u, hint, hasHint)
		if !ok || len(pts) == 0 {
			return loopPoly{}, false
		}
		uvs[i] = pts
		hint, hasHint = pts[len(pts)-1].uv, true
	}
	second := make([]int, n)
	for i := range second {
		second[i] = -1
	}
	type pair struct{ a, b int }
	var pairs []pair
	sameDir := make([]bool, n)
	firstOf := map[*edgeGeom]int{}
	for i, u := range l.uses {
		if u.e == nil {
			continue
		}
		if j, ok := firstOf[u.e]; ok {
			if second[i] < 0 && len(pairs) < 3 {
				second[i] = len(pairs)
				sameDir[i] = l.uses[j].fwd == u.fwd
				pairs = append(pairs, pair{j, i})
			}
			continue
		}
		firstOf[u.e] = i
	}
	var offs [][2]float64
	for d := 0; d < 2; d++ {
		if fc.per[d] > 0 {
			var o1, o2 [2]float64
			o1[d], o2[d] = fc.per[d], -fc.per[d]
			offs = append(offs, o1, o2)
		}
	}
	if len(offs) == 0 {
		for i := range second {
			second[i] = -1
		}
		pairs = nil
	}
	combos := 1
	for range pairs {
		combos *= len(offs)
	}
	var best loopPoly
	found := false
	bestRank := -1
	for cmb := 0; cmb < combos; cmb++ {
		choice := make([][2]float64, len(pairs))
		x := cmb
		for k := range pairs {
			choice[k] = offs[x%len(offs)]
			x /= len(offs)
		}
		lp, ok := fc.assemble(uvs, second, sameDir, func(k int) int { return pairs[k].a }, choice)
		if !ok {
			continue
		}
		rank := 0
		if lp.wind == [2]int{} {
			rank = 1
			if lp.area*fc.sign > 0 {
				rank = 2
			}
		}
		if !found || rank > bestRank || (rank == bestRank && math.Abs(lp.area) > math.Abs(best.area)) {
			best, bestRank, found = lp, rank, true
		}
	}
	return best, found
}

func (fc *faceCtx) assemble(uvs [][]bpt, second []int, sameDir []bool, firstIdx func(int) int, choice [][2]float64) (loopPoly, bool) {
	n := len(uvs)
	shifts := make([][2]float64, n)
	total := 0
	for _, u := range uvs {
		total += len(u)
	}
	pts := make([]bpt, 0, total+4)
	for i := 0; i < n; i++ {
		var sh [2]float64
		if k := second[i]; k >= 0 {
			fi := firstIdx(k)
			f := shifts[fi]
			fu := uvs[fi]
			end := fu[len(fu)-1].uv
			if sameDir != nil && sameDir[i] {
				end = fu[0].uv
			}
			st := uvs[i][0].uv
			for d := 0; d < 2; d++ {
				target := end[d] + f[d] + choice[k][d]
				if fc.per[d] > 0 {
					sh[d] = math.Round((target-st[d])/fc.per[d]) * fc.per[d]
				}
			}
		} else if len(pts) > 0 {
			prev := pts[len(pts)-1].uv
			st := uvs[i][0].uv
			for d := 0; d < 2; d++ {
				if fc.per[d] > 0 {
					sh[d] = math.Round((prev[d]-st[d])/fc.per[d]) * fc.per[d]
				}
			}
		}
		shifts[i] = sh
		for j, b := range uvs[i] {
			b.uv[0] += sh[0]
			b.uv[1] += sh[1]
			b.seg = segEdge
			if j == 0 && len(pts) > 0 {
				a := pts[len(pts)-1]
				kind, ok := fc.gap(a, b)
				if !ok {
					return loopPoly{}, false
				}
				if kind < 0 {
					continue
				}
				b.seg = uint8(kind)
			}
			pts = append(pts, b)
		}
	}
	if len(pts) < 2 {
		return loopPoly{}, false
	}
	first, last := pts[0], pts[len(pts)-1]
	g := [2]float64{first.uv[0] - last.uv[0], first.uv[1] - last.uv[1]}
	lp := loopPoly{valid: true}
	if fc.small(g, last.uv) {
		pts = pts[:len(pts)-1]
		lp.pts = pts
	} else {
		var w [2]int
		multiple := true
		nonzero := false
		for d := 0; d < 2; d++ {
			if fc.per[d] > 0 {
				k := math.Round(g[d] / fc.per[d])
				if math.Abs(g[d]-k*fc.per[d]) > 1e-6*fc.per[d] {
					multiple = false
				}
				w[d] = int(-k)
				if k != 0 {
					nonzero = true
				}
			} else if math.Abs(g[d]) > 1e-9*(1+math.Abs(last.uv[d])) {
				multiple = false
			}
		}
		deg := fc.degenerate(last.uv, first.uv, last.p) && dist(first.p, last.p) <= fc.tol3
		switch {
		case deg:
			pts[0].seg = segPole
			lp.pts = pts
		case multiple && nonzero && dist(first.p, last.p) <= fc.tol3:
			lp.wind = w
			lp.pts = pts
		default:
			pts[0].seg = segFree
			lp.pts = pts
		}
	}
	if lp.wind == [2]int{} {
		if len(lp.pts) < 3 {
			return loopPoly{}, false
		}
		lp.area = shoelace(lp.pts)
	}
	return lp, true
}

func (fc *faceCtx) small(g, at [2]float64) bool {
	return math.Abs(g[0]) <= 1e-9*(1+math.Abs(at[0])) && math.Abs(g[1]) <= 1e-9*(1+math.Abs(at[1]))
}

func (fc *faceCtx) gap(a, b bpt) (int, bool) {
	g := [2]float64{b.uv[0] - a.uv[0], b.uv[1] - a.uv[1]}
	if fc.small(g, a.uv) {
		return -1, true
	}
	deg := dist(a.p, b.p) <= fc.tol3 && fc.degenerate(a.uv, b.uv, a.p)
	if deg {
		return int(segPole), true
	}
	multiple := true
	nonzero := false
	for d := 0; d < 2; d++ {
		if fc.per[d] > 0 {
			k := math.Round(g[d] / fc.per[d])
			if math.Abs(g[d]-k*fc.per[d]) > 1e-6*fc.per[d] {
				multiple = false
			}
			if k != 0 {
				nonzero = true
			}
		} else if math.Abs(g[d]) > 1e-9*(1+math.Abs(a.uv[d])) {
			multiple = false
		}
	}
	if multiple && nonzero {
		return 0, false
	}
	return int(segFree), true
}

func shiftLoop(pts []bpt, sh [2]float64) {
	for i := range pts {
		pts[i].uv[0] += sh[0]
		pts[i].uv[1] += sh[1]
	}
}

func bounds2(pts []bpt) [4]float64 {
	b := [4]float64{math.Inf(1), math.Inf(-1), math.Inf(1), math.Inf(-1)}
	for _, p := range pts {
		b[0] = math.Min(b[0], p.uv[0])
		b[1] = math.Max(b[1], p.uv[0])
		b[2] = math.Min(b[2], p.uv[1])
		b[3] = math.Max(b[3], p.uv[1])
	}
	return b
}

func (fc *faceCtx) cutValue(holes []loopPoly, d int, fallback float64) float64 {
	per := fc.per[d]
	if per <= 0 || len(holes) == 0 {
		return fallback
	}
	type iv struct{ a, b float64 }
	var ivs []iv
	for _, h := range holes {
		b := bounds2(h.pts)
		lo, hi := b[2*d], b[2*d+1]
		if hi-lo >= per {
			return fallback
		}
		lo = wrapPeriod(lo, 0, per)
		hi = lo + (b[2*d+1] - b[2*d])
		ivs = append(ivs, iv{lo, hi}, iv{lo + per, hi + per})
	}
	f := wrapPeriod(fallback, 0, per)
	cands := []float64{f}
	for _, x := range ivs {
		cands = append(cands, wrapPeriod(x.b, 0, per))
	}
	bestGap, best := -1.0, f
	for _, c := range cands {
		gapv := per
		for _, x := range ivs {
			for _, cc := range []float64{c, c + per} {
				if cc >= x.a && cc <= x.b {
					gapv = -1
				}
			}
		}
		if gapv < 0 {
			continue
		}
		md := per
		for _, x := range ivs {
			for _, cc := range []float64{c, c + per} {
				md = math.Min(md, math.Min(math.Abs(cc-x.a), math.Abs(cc-x.b)))
			}
		}
		if md > bestGap {
			bestGap, best = md, c
		}
	}
	k := math.Round((fallback - best) / per)
	return best + k*per
}

func rotateWinding(pts []bpt, k int, sh [2]float64) []bpt {
	n := len(pts) - 1
	if k <= 0 || k >= n {
		return pts
	}
	out := make([]bpt, 0, len(pts))
	out = append(out, pts[k:]...)
	for i := 1; i <= k; i++ {
		b := pts[i]
		b.uv[0] += sh[0]
		b.uv[1] += sh[1]
		out = append(out, b)
	}
	return out
}

func nearestIndex(pts []bpt, d int, val, per float64, n int) int {
	best, bi := math.Inf(1), 0
	for i := 0; i < n; i++ {
		x := pts[i].uv[d] - val
		if per > 0 {
			x -= math.Round(x/per) * per
		}
		if math.Abs(x) < best {
			best, bi = math.Abs(x), i
		}
	}
	return bi
}

func (fc *faceCtx) placeHoles(holes []loopPoly, ref [4]float64) {
	for _, h := range holes {
		b := bounds2(h.pts)
		var sh [2]float64
		for d := 0; d < 2; d++ {
			if fc.per[d] > 0 {
				c := (b[2*d] + b[2*d+1]) / 2
				rc := (ref[2*d] + ref[2*d+1]) / 2
				sh[d] = math.Round((rc-c)/fc.per[d]) * fc.per[d]
				if b[2*d]+sh[d] < ref[2*d] && b[2*d+1]+sh[d]+fc.per[d] <= ref[2*d+1] {
					sh[d] += fc.per[d]
				}
				if b[2*d+1]+sh[d] > ref[2*d+1] && b[2*d]+sh[d]-fc.per[d] >= ref[2*d] {
					sh[d] -= fc.per[d]
				}
			}
		}
		shiftLoop(h.pts, sh)
	}
}

func (fc *faceCtx) newCut() int32 {
	fc.cuts++
	return fc.cuts - 1
}

func (fc *faceCtx) rings(loops []loopPoly) ([][]bpt, bool) {
	var closed, wind []loopPoly
	for _, l := range loops {
		if l.wind == [2]int{} {
			closed = append(closed, l)
		} else {
			wind = append(wind, l)
		}
	}
	switch len(wind) {
	case 0:
		outer := -1
		for i, l := range closed {
			if l.area*fc.sign > 0 && (outer < 0 || math.Abs(l.area) > math.Abs(closed[outer].area)) {
				outer = i
			}
		}
		if outer < 0 {
			if full, ok := fc.fullDomain(closed); ok {
				out := [][]bpt{full}
				for _, h := range closed {
					out = append(out, h.pts)
				}
				return out, true
			}
			if len(closed) == 0 {
				return nil, false
			}
			for i, l := range closed {
				if outer < 0 || math.Abs(l.area) > math.Abs(closed[outer].area) {
					outer = i
				}
			}
		}
		ref := bounds2(closed[outer].pts)
		var holes []loopPoly
		for i, l := range closed {
			if i != outer {
				holes = append(holes, l)
			}
		}
		fc.placeHoles(holes, ref)
		out := [][]bpt{closed[outer].pts}
		for _, h := range holes {
			out = append(out, h.pts)
		}
		return out, true
	case 1:
		r, ok := fc.poleClose(wind[0], closed)
		if !ok {
			return nil, false
		}
		out := [][]bpt{r}
		fc.placeHoles(closed, bounds2(r))
		for _, h := range closed {
			out = append(out, h.pts)
		}
		return out, true
	case 2:
		r, ok := fc.join(wind[0], wind[1], closed)
		if !ok {
			return nil, false
		}
		out := [][]bpt{r}
		fc.placeHoles(closed, bounds2(r))
		for _, h := range closed {
			out = append(out, h.pts)
		}
		return out, true
	}
	return nil, false
}

func windDir(w [2]int) (int, int, bool) {
	switch {
	case w[0] != 0 && w[1] == 0:
		return 0, w[0], true
	case w[1] != 0 && w[0] == 0:
		return 1, w[1], true
	}
	return 0, 0, false
}

func (fc *faceCtx) poleClose(a loopPoly, holes []loopPoly) ([]bpt, bool) {
	d, s, ok := windDir(a.wind)
	if !ok || (s != 1 && s != -1) {
		return nil, false
	}
	e := 1 - d
	per := fc.per[d]
	n := len(a.pts) - 1
	cut := fc.cutValue(holes, d, a.pts[0].uv[d])
	k := nearestIndex(a.pts, d, cut, per, n)
	var sh [2]float64
	sh[d] = float64(s) * per
	pts := rotateWinding(a.pts, k, sh)
	side := float64(s) * fc.sign
	if d == 1 {
		side = -side
	}
	b := bounds2(pts)
	ext := b[2*e+1]
	if side < 0 {
		ext = b[2*e]
	}
	rows := singularRows(fc.s, d)
	row, found := 0.0, false
	for _, r := range rows {
		if side > 0 && r >= ext-1e-12*(1+math.Abs(ext)) {
			if !found || r < row {
				row, found = r, true
			}
		}
		if side < 0 && r <= ext+1e-12*(1+math.Abs(ext)) {
			if !found || r > row {
				row, found = r, true
			}
		}
	}
	if !found {
		return nil, false
	}
	last := pts[len(pts)-1]
	var q1, q0 bpt
	q1.uv[d], q1.uv[e] = last.uv[d], row
	q0.uv[d], q0.uv[e] = pts[0].uv[d], row
	pp := fc.s.at(q1.uv[0], q1.uv[1])
	if !pp.finite() {
		return nil, false
	}
	q1.p, q0.p = pp, pp
	c := fc.newCut()
	q1.seg, q1.cut = segCut, c
	q0.seg = segPole
	pts[0].seg, pts[0].cut, pts[0].rev = segCut, c, true
	pts = append(pts, q1, q0)
	return pts, true
}

func (fc *faceCtx) join(a, b loopPoly, holes []loopPoly) ([]bpt, bool) {
	da, sa, ok1 := windDir(a.wind)
	db, sb, ok2 := windDir(b.wind)
	if !ok1 || !ok2 || da != db || sa != -sb || (sa != 1 && sa != -1) {
		return nil, false
	}
	d := da
	per := fc.per[d]
	na := len(a.pts) - 1
	nb := len(b.pts) - 1
	cut := fc.cutValue(holes, d, a.pts[0].uv[d])
	ka := nearestIndex(a.pts, d, cut, per, na)
	var sha, shb [2]float64
	sha[d] = float64(sa) * per
	shb[d] = float64(sb) * per
	ap := rotateWinding(a.pts, ka, sha)
	kb := nearestIndex(b.pts, d, ap[0].uv[d], per, nb)
	bp := rotateWinding(b.pts, kb, shb)
	an := ap[len(ap)-1]
	var off [2]float64
	off[d] = math.Round((an.uv[d]-bp[0].uv[d])/per) * per
	e := 1 - d
	if pe := fc.per[e]; pe > 0 {
		side := float64(sa) * fc.sign
		if d == 1 {
			side = -side
		}
		ba := bounds2(ap)
		bb := bounds2(bp)
		ca := (ba[2*e] + ba[2*e+1]) / 2
		cb := (bb[2*e] + bb[2*e+1]) / 2
		k := math.Floor((ca - cb) / pe)
		if side > 0 {
			k++
		}
		off[e] = k * pe
		if side > 0 && cb+off[e] <= ca {
			off[e] += pe
		}
		if side < 0 && cb+off[e] >= ca {
			off[e] -= pe
		}
	}
	shiftLoop(bp, off)
	c := fc.newCut()
	ap[0].seg, ap[0].cut, ap[0].rev = segCut, c, true
	bp[0].seg, bp[0].cut, bp[0].rev = segCut, c, false
	out := make([]bpt, 0, len(ap)+len(bp))
	out = append(out, ap...)
	out = append(out, bp...)
	g0 := [2]float64{out[0].uv[0] - bp[len(bp)-1].uv[0], out[0].uv[1] - bp[len(bp)-1].uv[1]}
	g1 := [2]float64{bp[0].uv[0] - an.uv[0], bp[0].uv[1] - an.uv[1]}
	if math.Abs(math.Abs(g0[d])-math.Abs(g1[d])) > 1e-6*per+1e-9 {
		return nil, false
	}
	return out, true
}

func (fc *faceCtx) fullDomain(holes []loopPoly) ([]bpt, bool) {
	pu, pv := fc.per[0], fc.per[1]
	dom := fc.s.domain()
	switch {
	case pu > 0 && pv > 0:
		u0 := fc.cutValue(holes, 0, dom[0])
		v0 := fc.cutValue(holes, 1, dom[2])
		p := fc.s.at(u0, v0)
		if !p.finite() {
			return nil, false
		}
		ca, cb := fc.newCut(), fc.newCut()
		r := []bpt{
			{uv: [2]float64{u0, v0}, p: p, seg: segCut, cut: cb, rev: true},
			{uv: [2]float64{u0 + pu, v0}, p: p, seg: segCut, cut: ca},
			{uv: [2]float64{u0 + pu, v0 + pv}, p: p, seg: segCut, cut: cb},
			{uv: [2]float64{u0, v0 + pv}, p: p, seg: segCut, cut: ca, rev: true},
		}
		return r, true
	case pu > 0 || pv > 0:
		d := 0
		if pu <= 0 {
			d = 1
		}
		e := 1 - d
		rows := singularRows(fc.s, d)
		if len(rows) < 2 {
			return nil, false
		}
		lo, hi := math.Inf(1), math.Inf(-1)
		for _, r := range rows {
			lo, hi = math.Min(lo, r), math.Max(hi, r)
		}
		if !(hi > lo) {
			return nil, false
		}
		per := fc.per[d]
		c0 := fc.cutValue(holes, d, dom[2*d])
		mk := func(dv, ev float64) [2]float64 {
			var q [2]float64
			q[d], q[e] = dv, ev
			return q
		}
		plo := fc.s.at(mk(c0, lo)[0], mk(c0, lo)[1])
		phi := fc.s.at(mk(c0, hi)[0], mk(c0, hi)[1])
		if !plo.finite() || !phi.finite() {
			return nil, false
		}
		c := fc.newCut()
		r := []bpt{
			{uv: mk(c0, lo), p: plo, seg: segCut, cut: c, rev: true},
			{uv: mk(c0+per, lo), p: plo, seg: segPole},
			{uv: mk(c0+per, hi), p: phi, seg: segCut, cut: c},
			{uv: mk(c0, hi), p: phi, seg: segPole},
		}
		return r, true
	}
	return nil, false
}
