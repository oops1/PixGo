package step

import (
	"math"
)

const maxEarPoints = 4000

func ringArea(pts [][2]float64, r []int32) float64 {
	a := 0.0
	for i := range r {
		p, q := pts[r[i]], pts[r[(i+1)%len(r)]]
		a += p[0]*q[1] - q[0]*p[1]
	}
	return a / 2
}

func segCross(a, b, c, d [2]float64) bool {
	o1 := orient2(a[0], a[1], b[0], b[1], c[0], c[1])
	o2 := orient2(a[0], a[1], b[0], b[1], d[0], d[1])
	o3 := orient2(c[0], c[1], d[0], d[1], a[0], a[1])
	o4 := orient2(c[0], c[1], d[0], d[1], b[0], b[1])
	return ((o1 > 0 && o2 < 0) || (o1 < 0 && o2 > 0)) && ((o3 > 0 && o4 < 0) || (o3 < 0 && o4 > 0))
}

func bridgeHoles(pts [][2]float64, rings [][]int32) ([]int32, bool) {
	if len(rings) == 0 {
		return nil, false
	}
	total := 0
	for _, r := range rings {
		total += len(r)
	}
	if total > maxEarPoints {
		return nil, false
	}
	outer := append([]int32(nil), rings[0]...)
	if ringArea(pts, outer) < 0 {
		reverseIdx(outer)
	}
	holes := make([][]int32, 0, len(rings)-1)
	for _, h := range rings[1:] {
		h = append([]int32(nil), h...)
		if ringArea(pts, h) > 0 {
			reverseIdx(h)
		}
		holes = append(holes, h)
	}
	maxX := func(h []int32) (int, float64) {
		bi, bx := 0, math.Inf(-1)
		for i, v := range h {
			if pts[v][0] > bx {
				bi, bx = i, pts[v][0]
			}
		}
		return bi, bx
	}
	for len(holes) > 0 {
		best, bx := 0, math.Inf(-1)
		for i, h := range holes {
			if _, x := maxX(h); x > bx {
				best, bx = i, x
			}
		}
		h := holes[best]
		holes = append(holes[:best], holes[best+1:]...)
		mi, _ := maxX(h)
		m := pts[h[mi]]
		bo, bd := -1, math.Inf(1)
		fallback, fd := 0, math.Inf(1)
		for oi, ov := range outer {
			o := pts[ov]
			d := (o[0]-m[0])*(o[0]-m[0]) + (o[1]-m[1])*(o[1]-m[1])
			if d < fd {
				fallback, fd = oi, d
			}
			if d >= bd {
				continue
			}
			ok := true
			check := func(r []int32) {
				for i := 0; i < len(r) && ok; i++ {
					a, b := pts[r[i]], pts[r[(i+1)%len(r)]]
					if segCross(m, o, a, b) {
						ok = false
					}
				}
			}
			check(outer)
			check(h)
			for _, other := range holes {
				check(other)
			}
			if ok {
				bo, bd = oi, d
			}
		}
		if bo < 0 {
			bo = fallback
		}
		merged := make([]int32, 0, len(outer)+len(h)+2)
		merged = append(merged, outer[:bo+1]...)
		for k := 0; k <= len(h); k++ {
			merged = append(merged, h[(mi+k)%len(h)])
		}
		merged = append(merged, outer[bo:]...)
		outer = merged
	}
	return outer, true
}

func reverseIdx(r []int32) {
	for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
		r[i], r[j] = r[j], r[i]
	}
}

func earClip(pts [][2]float64, poly []int32) ([][3]int32, bool) {
	n := len(poly)
	if n < 3 || n > 2*maxEarPoints {
		return nil, false
	}
	prev := make([]int, n)
	next := make([]int, n)
	for i := range poly {
		prev[i] = (i + n - 1) % n
		next[i] = (i + 1) % n
	}
	cross := func(a, b, c int) float64 {
		pa, pb, pc := pts[poly[a]], pts[poly[b]], pts[poly[c]]
		return orient2(pa[0], pa[1], pb[0], pb[1], pc[0], pc[1])
	}
	inside := func(p, a, b, c [2]float64) bool {
		return orient2(a[0], a[1], b[0], b[1], p[0], p[1]) > 0 &&
			orient2(b[0], b[1], c[0], c[1], p[0], p[1]) > 0 &&
			orient2(c[0], c[1], a[0], a[1], p[0], p[1]) > 0
	}
	isEar := func(b int) bool {
		a, c := prev[b], next[b]
		if !(cross(a, b, c) > 0) {
			return false
		}
		pa, pb, pc := pts[poly[a]], pts[poly[b]], pts[poly[c]]
		for v := next[c]; v != a; v = next[v] {
			p := pts[poly[v]]
			if p == pa || p == pb || p == pc {
				continue
			}
			if inside(p, pa, pb, pc) {
				return false
			}
		}
		return true
	}
	out := make([][3]int32, 0, n-2)
	remaining := n
	i := 0
	stall := 0
	for remaining > 3 {
		if isEar(i) {
			a, c := prev[i], next[i]
			out = append(out, [3]int32{poly[a], poly[i], poly[c]})
			next[a], prev[c] = c, a
			remaining--
			i = a
			stall = 0
			continue
		}
		i = next[i]
		stall++
		if stall > remaining {
			best, bc := i, math.Inf(1)
			v := i
			for k := 0; k < remaining; k++ {
				if c := math.Abs(cross(prev[v], v, next[v])); c < bc {
					best, bc = v, c
				}
				v = next[v]
			}
			a, c := prev[best], next[best]
			out = append(out, [3]int32{poly[a], poly[best], poly[c]})
			next[a], prev[c] = c, a
			remaining--
			i = a
			stall = 0
		}
	}
	a := i
	b := next[a]
	c := next[b]
	out = append(out, [3]int32{poly[a], poly[b], poly[c]})
	return out, true
}

func earFallback(pts [][2]float64, rings [][]int32) ([][3]int32, bool) {
	poly, ok := bridgeHoles(pts, rings)
	if !ok {
		return nil, false
	}
	tris, ok := earClip(pts, poly)
	if !ok {
		return nil, false
	}
	want := 0.0
	for _, r := range rings {
		want += math.Abs(ringArea(pts, r))
	}
	if len(rings) > 1 {
		want = 2*math.Abs(ringArea(pts, rings[0])) - want
	}
	got := 0.0
	for _, t := range tris {
		a, b, c := pts[t[0]], pts[t[1]], pts[t[2]]
		got += math.Abs(orient2(a[0], a[1], b[0], b[1], c[0], c[1])) / 2
	}
	if !(got <= 1.02*math.Abs(want)+1e-12) {
		return nil, false
	}
	return tris, true
}
