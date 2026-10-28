package step

import (
	"math"
	"math/bits"
	"sort"
)

const snapRange = 1 << 24

type cdtTri struct {
	v [3]int32
	n [3]int32
	c uint8
}

type cdt struct {
	x, y  []int64
	t     []cdtTri
	vt    []int32
	last  int32
	stack []int32
	steps int
}

func orient(ax, ay, bx, by, cx, cy int64) int {
	d := (bx-ax)*(cy-ay) - (by-ay)*(cx-ax)
	switch {
	case d > 0:
		return 1
	case d < 0:
		return -1
	}
	return 0
}

func (c *cdt) orient(a, b, p int32) int {
	return orient(c.x[a], c.y[a], c.x[b], c.y[b], c.x[p], c.y[p])
}

func mul128(a, b int64) (int64, uint64) {
	neg := (a < 0) != (b < 0)
	ua, ub := uint64(a), uint64(b)
	if a < 0 {
		ua = uint64(-a)
	}
	if b < 0 {
		ub = uint64(-b)
	}
	hi, lo := bits.Mul64(ua, ub)
	if neg {
		lo = ^lo + 1
		hi = ^hi
		if lo == 0 {
			hi++
		}
	}
	return int64(hi), lo
}

func add128(ah int64, al uint64, bh int64, bl uint64) (int64, uint64) {
	lo, carry := bits.Add64(al, bl, 0)
	hi := ah + bh + int64(carry)
	return hi, lo
}

func (c *cdt) incircle(a, b, cc, d int32) int {
	adx, ady := c.x[a]-c.x[d], c.y[a]-c.y[d]
	bdx, bdy := c.x[b]-c.x[d], c.y[b]-c.y[d]
	cdx, cdy := c.x[cc]-c.x[d], c.y[cc]-c.y[d]
	al := adx*adx + ady*ady
	bl := bdx*bdx + bdy*bdy
	cl := cdx*cdx + cdy*cdy
	h1, l1 := mul128(al, bdx*cdy-cdx*bdy)
	h2, l2 := mul128(bl, cdx*ady-adx*cdy)
	h3, l3 := mul128(cl, adx*bdy-bdx*ady)
	h, l := add128(h1, l1, h2, l2)
	h, l = add128(h, l, h3, l3)
	switch {
	case h < 0:
		return -1
	case h > 0 || l > 0:
		return 1
	}
	return 0
}

func newCDT(n int) *cdt {
	c := &cdt{
		x:  make([]int64, 3, n+3),
		y:  make([]int64, 3, n+3),
		vt: make([]int32, 3, n+3),
	}
	const m = snapRange
	c.x[0], c.y[0] = -4*m, -4*m
	c.x[1], c.y[1] = 12*m, -4*m
	c.x[2], c.y[2] = -4*m, 12*m
	c.t = append(c.t, cdtTri{v: [3]int32{0, 1, 2}, n: [3]int32{-1, -1, -1}})
	c.vt[0], c.vt[1], c.vt[2] = 0, 0, 0
	return c
}

func (c *cdt) setNeighbor(t, old, nw int32) {
	if t < 0 {
		return
	}
	tr := &c.t[t]
	for i := 0; i < 3; i++ {
		if tr.n[i] == old {
			tr.n[i] = nw
			return
		}
	}
}

func (c *cdt) locate(px, py int64) (int32, bool) {
	t := c.last
	if t < 0 || int(t) >= len(c.t) {
		t = 0
	}
	limit := 4*int(math.Sqrt(float64(len(c.t)))) + 64
	for step := 0; step < limit; step++ {
		tr := &c.t[t]
		moved := false
		for k := 0; k < 3; k++ {
			i := (k + step) % 3
			a, b := tr.v[(i+1)%3], tr.v[(i+2)%3]
			if orient(c.x[a], c.y[a], c.x[b], c.y[b], px, py) < 0 {
				nt := tr.n[i]
				if nt < 0 {
					return c.scan(px, py)
				}
				t = nt
				moved = true
				break
			}
		}
		if !moved {
			return t, true
		}
	}
	return c.scan(px, py)
}

func (c *cdt) scan(px, py int64) (int32, bool) {
	for t := range c.t {
		tr := &c.t[t]
		ok := true
		for i := 0; i < 3 && ok; i++ {
			a, b := tr.v[(i+1)%3], tr.v[(i+2)%3]
			if orient(c.x[a], c.y[a], c.x[b], c.y[b], px, py) < 0 {
				ok = false
			}
		}
		if ok {
			return int32(t), true
		}
	}
	return -1, false
}

func (c *cdt) insert(px, py int64) (int32, bool) {
	t, ok := c.locate(px, py)
	if !ok {
		return -1, false
	}
	tr := c.t[t]
	zero := -1
	for i := 0; i < 3; i++ {
		v := tr.v[i]
		if c.x[v] == px && c.y[v] == py {
			return v, true
		}
	}
	for i := 0; i < 3; i++ {
		a, b := tr.v[(i+1)%3], tr.v[(i+2)%3]
		if orient(c.x[a], c.y[a], c.x[b], c.y[b], px, py) == 0 {
			zero = i
		}
	}
	p := int32(len(c.x))
	c.x = append(c.x, px)
	c.y = append(c.y, py)
	c.vt = append(c.vt, t)
	if zero >= 0 && tr.n[zero] >= 0 {
		c.split4(t, zero, p)
	} else {
		c.split3(t, p)
	}
	return p, true
}

func (c *cdt) split3(t int32, p int32) {
	old := c.t[t]
	v0, v1, v2 := old.v[0], old.v[1], old.v[2]
	n0, n1, n2 := old.n[0], old.n[1], old.n[2]
	A := t
	B := int32(len(c.t))
	C := B + 1
	c.t = append(c.t, cdtTri{}, cdtTri{})
	c.t[A] = cdtTri{v: [3]int32{v0, v1, p}, n: [3]int32{B, C, n2}, c: bit(old.c, 2) << 2}
	c.t[B] = cdtTri{v: [3]int32{v1, v2, p}, n: [3]int32{C, A, n0}, c: bit(old.c, 0) << 2}
	c.t[C] = cdtTri{v: [3]int32{v2, v0, p}, n: [3]int32{A, B, n1}, c: bit(old.c, 1) << 2}
	c.setNeighbor(n0, t, B)
	c.setNeighbor(n1, t, C)
	c.vt[v0], c.vt[v1], c.vt[v2], c.vt[p] = A, A, B, A
	c.last = A
	c.legalize(A, 2)
	c.legalize(B, 2)
	c.legalize(C, 2)
}

func bit(c uint8, i int) uint8 { return (c >> uint(i)) & 1 }

func (c *cdt) split4(t int32, i int, p int32) {
	tr := c.t[t]
	u := tr.n[i]
	a, b, cc := tr.v[i], tr.v[(i+1)%3], tr.v[(i+2)%3]
	ur := c.t[u]
	j := 0
	for k := 0; k < 3; k++ {
		if ur.n[k] == t {
			j = k
		}
	}
	d := ur.v[j]
	tAB := tr.n[(i+2)%3]
	tCA := tr.n[(i+1)%3]
	uDC := ur.n[(j+2)%3]
	uBD := ur.n[(j+1)%3]
	cAB := bit(tr.c, (i+2)%3)
	cCA := bit(tr.c, (i+1)%3)
	cDC := bit(ur.c, (j+2)%3)
	cBD := bit(ur.c, (j+1)%3)
	t1, t2, u1, u2 := t, int32(len(c.t)), u, int32(len(c.t)+1)
	c.t = append(c.t, cdtTri{}, cdtTri{})
	c.t[t1] = cdtTri{v: [3]int32{a, b, p}, n: [3]int32{u2, t2, tAB}, c: cAB << 2}
	c.t[t2] = cdtTri{v: [3]int32{a, p, cc}, n: [3]int32{u1, tCA, t1}, c: cCA << 1}
	c.t[u1] = cdtTri{v: [3]int32{d, cc, p}, n: [3]int32{t2, u2, uDC}, c: cDC << 2}
	c.t[u2] = cdtTri{v: [3]int32{d, p, b}, n: [3]int32{t1, uBD, u1}, c: cBD << 1}
	c.setNeighbor(tCA, t, t2)
	c.setNeighbor(uBD, u, u2)
	c.vt[a], c.vt[b], c.vt[cc], c.vt[d], c.vt[p] = t1, t1, t2, u1, t1
	c.last = t1
	c.legalize(t1, 2)
	c.legalize(t2, 1)
	c.legalize(u1, 2)
	c.legalize(u2, 1)
}

func (c *cdt) flip(t int32, i int) (int32, int32) {
	tr := c.t[t]
	u := tr.n[i]
	ur := c.t[u]
	j := 0
	for k := 0; k < 3; k++ {
		if ur.n[k] == t {
			j = k
		}
	}
	p, a, b := tr.v[i], tr.v[(i+1)%3], tr.v[(i+2)%3]
	d := ur.v[j]
	tB := tr.n[(i+1)%3]
	tA := tr.n[(i+2)%3]
	uA := ur.n[(j+1)%3]
	uB := ur.n[(j+2)%3]
	cB := bit(tr.c, (i+1)%3)
	cA := bit(tr.c, (i+2)%3)
	cuA := bit(ur.c, (j+1)%3)
	cuB := bit(ur.c, (j+2)%3)
	c.t[t] = cdtTri{v: [3]int32{p, a, d}, n: [3]int32{uA, u, tA}, c: cuA | cA<<2}
	c.t[u] = cdtTri{v: [3]int32{p, d, b}, n: [3]int32{uB, tB, t}, c: cuB | cB<<1}
	c.setNeighbor(uA, u, t)
	c.setNeighbor(tB, t, u)
	c.vt[p], c.vt[a], c.vt[d], c.vt[b] = t, t, t, u
	return t, u
}

func (c *cdt) legalize(t int32, i int) {
	c.stack = append(c.stack[:0], t, int32(i))
	guard := 0
	for len(c.stack) > 0 {
		guard++
		if guard > 1<<22 {
			return
		}
		i := int(c.stack[len(c.stack)-1])
		t := c.stack[len(c.stack)-2]
		c.stack = c.stack[:len(c.stack)-2]
		tr := c.t[t]
		u := tr.n[i]
		if u < 0 || bit(tr.c, i) != 0 {
			continue
		}
		ur := c.t[u]
		j := -1
		for k := 0; k < 3; k++ {
			if ur.n[k] == t {
				j = k
			}
		}
		if j < 0 {
			continue
		}
		p, a, b := tr.v[i], tr.v[(i+1)%3], tr.v[(i+2)%3]
		d := ur.v[j]
		if c.incircle(p, a, b, d) <= 0 {
			continue
		}
		if c.orient(p, a, d) <= 0 || c.orient(p, d, b) <= 0 {
			continue
		}
		t1, t2 := c.flip(t, i)
		c.stack = append(c.stack, t1, 0, t2, 0)
	}
}

func (c *cdt) findEdge(a, b int32) (int32, int) {
	if a < 3 && b >= 3 {
		a, b = b, a
	}
	t0 := c.vt[a]
	if t0 < 0 {
		return -1, -1
	}
	t := t0
	for n := 0; n < len(c.t)+1; n++ {
		tr := &c.t[t]
		ia := -1
		for k := 0; k < 3; k++ {
			if tr.v[k] == a {
				ia = k
			}
		}
		if ia < 0 {
			return c.findEdgeSlow(a, b)
		}
		if tr.v[(ia+1)%3] == b {
			return t, (ia + 2) % 3
		}
		if tr.v[(ia+2)%3] == b {
			return t, (ia + 1) % 3
		}
		nt := tr.n[(ia+1)%3]
		if nt == t0 {
			return -1, -1
		}
		if nt < 0 {
			break
		}
		t = nt
	}
	return c.findEdgeSlow(a, b)
}

func (c *cdt) findEdgeSlow(a, b int32) (int32, int) {
	for t := range c.t {
		tr := &c.t[t]
		for k := 0; k < 3; k++ {
			if tr.v[(k+1)%3] == a && tr.v[(k+2)%3] == b || tr.v[(k+1)%3] == b && tr.v[(k+2)%3] == a {
				return int32(t), k
			}
		}
	}
	return -1, -1
}

func (c *cdt) markConstraint(t int32, i int) {
	c.t[t].c |= 1 << uint(i)
	u := c.t[t].n[i]
	if u < 0 {
		return
	}
	for k := 0; k < 3; k++ {
		if c.t[u].n[k] == t {
			c.t[u].c |= 1 << uint(k)
		}
	}
}

func (c *cdt) crosses(a, b, p, q int32) bool {
	o1, o2 := c.orient(a, b, p), c.orient(a, b, q)
	if o1*o2 >= 0 {
		return false
	}
	o3, o4 := c.orient(p, q, a), c.orient(p, q, b)
	return o3*o4 < 0
}

func (c *cdt) onSegment(a, b, p int32) bool {
	if c.orient(a, b, p) != 0 {
		return false
	}
	dx, dy := c.x[b]-c.x[a], c.y[b]-c.y[a]
	s := (c.x[p]-c.x[a])*dx + (c.y[p]-c.y[a])*dy
	return s > 0 && s < dx*dx+dy*dy
}

func (c *cdt) constrain(a, b int32, depth int) bool {
	if a == b {
		return true
	}
	if depth > 64 {
		return false
	}
	if t, i := c.findEdge(a, b); t >= 0 {
		c.markConstraint(t, i)
		return true
	}
	var queue [][2]int32
	t0 := c.vt[a]
	t := t0
	found := false
	var e1, e2 int32
	var cur int32
	for n := 0; n < len(c.t)+1; n++ {
		tr := c.t[t]
		ia := -1
		for k := 0; k < 3; k++ {
			if tr.v[k] == a {
				ia = k
			}
		}
		if ia < 0 {
			return false
		}
		v1, v2 := tr.v[(ia+1)%3], tr.v[(ia+2)%3]
		if c.onSegment(a, b, v1) {
			return c.constrain(a, v1, depth+1) && c.constrain(v1, b, depth+1)
		}
		if c.onSegment(a, b, v2) {
			return c.constrain(a, v2, depth+1) && c.constrain(v2, b, depth+1)
		}
		if c.orient(a, v1, b) > 0 && c.orient(a, b, v2) > 0 {
			e1, e2 = v2, v1
			cur = t
			if bit(tr.c, ia) != 0 {
				return false
			}
			queue = append(queue, [2]int32{v1, v2})
			found = true
			_ = cur
			t = tr.n[ia]
			break
		}
		nt := tr.n[(ia+1)%3]
		if nt < 0 || nt == t0 {
			return false
		}
		t = nt
	}
	if !found || t < 0 {
		return false
	}
	for n := 0; n < len(c.t)+1; n++ {
		tr := c.t[t]
		w := int32(-1)
		for k := 0; k < 3; k++ {
			if tr.v[k] != e1 && tr.v[k] != e2 {
				w = tr.v[k]
			}
		}
		if w < 0 {
			return false
		}
		if w == b {
			break
		}
		if c.onSegment(a, b, w) {
			return c.constrain(a, w, depth+1) && c.constrain(w, b, depth+1)
		}
		var ne1, ne2 int32
		if c.orient(a, b, w) > 0 {
			ne1, ne2 = w, e2
		} else {
			ne1, ne2 = e1, w
		}
		k := -1
		for q := 0; q < 3; q++ {
			x, y := tr.v[(q+1)%3], tr.v[(q+2)%3]
			if (x == ne1 && y == ne2) || (x == ne2 && y == ne1) {
				k = q
			}
		}
		if k < 0 || bit(tr.c, k) != 0 {
			return false
		}
		queue = append(queue, [2]int32{ne1, ne2})
		e1, e2 = ne1, ne2
		t = tr.n[k]
		if t < 0 {
			return false
		}
	}
	limit := 64*len(queue)*len(queue) + 4096
	for it := 0; len(queue) > 0; it++ {
		if it > limit {
			return false
		}
		e := queue[0]
		queue = queue[1:]
		t, i := c.findEdge(e[0], e[1])
		if t < 0 {
			continue
		}
		if bit(c.t[t].c, i) != 0 {
			return false
		}
		tr := c.t[t]
		u := tr.n[i]
		if u < 0 {
			return false
		}
		p := tr.v[i]
		ur := c.t[u]
		var d int32 = -1
		for k := 0; k < 3; k++ {
			if ur.n[k] == t {
				d = ur.v[k]
			}
		}
		if d < 0 {
			return false
		}
		if !c.crosses(p, d, e[0], e[1]) {
			queue = append(queue, e)
			continue
		}
		c.flip(t, i)
		if c.crosses(a, b, p, d) {
			queue = append(queue, [2]int32{p, d})
		}
	}
	t, i := c.findEdge(a, b)
	if t < 0 {
		return false
	}
	c.markConstraint(t, i)
	return true
}

func (c *cdt) delaunay() {
	type ed struct {
		t int32
		i int8
	}
	var q []ed
	for t := range c.t {
		for i := 0; i < 3; i++ {
			if n := c.t[t].n[i]; n > int32(t) && bit(c.t[t].c, i) == 0 {
				q = append(q, ed{int32(t), int8(i)})
			}
		}
	}
	limit := 32*len(c.t) + 1024
	for it := 0; len(q) > 0 && it < limit; it++ {
		e := q[len(q)-1]
		q = q[:len(q)-1]
		tr := c.t[e.t]
		i := int(e.i)
		u := tr.n[i]
		if u < 0 || bit(tr.c, i) != 0 {
			continue
		}
		ur := c.t[u]
		var d int32 = -1
		for k := 0; k < 3; k++ {
			if ur.n[k] == e.t {
				d = ur.v[k]
			}
		}
		if d < 0 {
			continue
		}
		p, a, b := tr.v[i], tr.v[(i+1)%3], tr.v[(i+2)%3]
		if c.incircle(p, a, b, d) <= 0 {
			continue
		}
		if c.orient(p, a, d) <= 0 || c.orient(p, d, b) <= 0 {
			continue
		}
		t1, t2 := c.flip(e.t, i)
		q = append(q, ed{t1, 0}, ed{t1, 2}, ed{t2, 0}, ed{t2, 1})
	}
}

func (c *cdt) inside() []bool {
	level := make([]int32, len(c.t))
	for i := range level {
		level[i] = -1
	}
	start := int32(-1)
	for t := range c.t {
		v := c.t[t].v
		if v[0] < 3 || v[1] < 3 || v[2] < 3 {
			start = int32(t)
			break
		}
	}
	if start < 0 {
		return make([]bool, len(c.t))
	}
	cur := []int32{start}
	level[start] = 0
	var next []int32
	for lv := int32(0); len(cur) > 0; lv++ {
		for len(cur) > 0 {
			t := cur[len(cur)-1]
			cur = cur[:len(cur)-1]
			for i := 0; i < 3; i++ {
				n := c.t[t].n[i]
				if n < 0 || level[n] >= 0 {
					continue
				}
				if bit(c.t[t].c, i) != 0 {
					next = append(next, n)
					continue
				}
				level[n] = lv
				cur = append(cur, n)
			}
		}
		var nn []int32
		for _, t := range next {
			if level[t] < 0 {
				level[t] = lv + 1
				nn = append(nn, t)
			}
		}
		cur, next = nn, next[:0]
	}
	out := make([]bool, len(c.t))
	for t := range c.t {
		v := c.t[t].v
		out[t] = level[t]%2 == 1 && v[0] >= 3 && v[1] >= 3 && v[2] >= 3
	}
	return out
}

type cdtInput struct {
	pts   [][2]float64
	rings [][]int32
}

func triangulate(in *cdtInput) ([][3]int32, bool) {
	n := len(in.pts)
	if n < 3 {
		return nil, false
	}
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	for _, p := range in.pts {
		if !finite(p[0]) || !finite(p[1]) {
			return nil, false
		}
		minX, maxX = math.Min(minX, p[0]), math.Max(maxX, p[0])
		minY, maxY = math.Min(minY, p[1]), math.Max(maxY, p[1])
	}
	ext := math.Max(maxX-minX, maxY-minY)
	if !(ext > 0) {
		return nil, false
	}
	k := float64(snapRange) / ext
	c := newCDT(n)
	vmap := make([]int32, n)
	back := make([]int32, 0, n+3)
	back = append(back, -1, -1, -1)
	for i, p := range in.pts {
		x := int64(math.Round((p[0] - minX) * k))
		y := int64(math.Round((p[1] - minY) * k))
		v, ok := c.insert(x, y)
		if !ok {
			return nil, false
		}
		vmap[i] = v
		if int(v) >= len(back) {
			back = append(back, int32(i))
		}
	}
	for _, r := range in.rings {
		for i := range r {
			a, b := vmap[r[i]], vmap[r[(i+1)%len(r)]]
			if !c.constrain(a, b, 0) {
				return nil, false
			}
		}
	}
	c.delaunay()
	ins := c.inside()
	out := make([][3]int32, 0, len(c.t)/2)
	for t, ok := range ins {
		if !ok {
			continue
		}
		v := c.t[t].v
		out = append(out, [3]int32{back[v[0]], back[v[1]], back[v[2]]})
	}
	return out, true
}

func sortedUnique(x []float64) []float64 {
	sort.Float64s(x)
	out := x[:0]
	for i, v := range x {
		if i == 0 || v != out[len(out)-1] {
			out = append(out, v)
		}
	}
	return out
}
