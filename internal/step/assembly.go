package step

import (
	"context"
	"math"
	"strings"
)

const (
	maxAsmDepth     = 32
	maxInstances    = 1 << 20
	maxItemFaces    = 1 << 22
	maxTopoDepth    = 8
	defaultGreyByte = 180
)

type faceRef struct {
	face  int
	flip  bool
	shell int
}

type itemInfo struct {
	ei    int
	faces []faceRef
	tess  bool
}

type instance struct {
	item  int
	x     xform
	scale float64
}

type repInfo struct {
	ei    int
	items []int
	ctx   int
}

type link struct {
	parent, child int
	t             xform
}

type unitInfo struct {
	scale float64
	name  string
	ok    bool
}

type scene struct {
	items     map[int]*itemInfo
	order     []int
	insts     []instance
	unit      string
	faceCol   map[int][3]uint8
	itemScale map[int]float64
}

func isGeomItem(name string) bool {
	switch name {
	case "MANIFOLD_SOLID_BREP", "BREP_WITH_VOIDS", "FACETED_BREP", "SHELL_BASED_SURFACE_MODEL",
		"CLOSED_SHELL", "OPEN_SHELL", "ORIENTED_CLOSED_SHELL", "ORIENTED_OPEN_SHELL",
		"TESSELLATED_SOLID", "TESSELLATED_SHELL", "TRIANGULATED_FACE", "COMPLEX_TRIANGULATED_FACE",
		"TRIANGULATED_SURFACE_SET", "COMPLEX_TRIANGULATED_SURFACE_SET", "ADVANCED_FACE", "FACE_SURFACE":
		return true
	}
	return false
}

func isSolidItem(name string) bool {
	switch name {
	case "MANIFOLD_SOLID_BREP", "BREP_WITH_VOIDS", "FACETED_BREP", "SHELL_BASED_SURFACE_MODEL",
		"TESSELLATED_SOLID", "TESSELLATED_SHELL", "TRIANGULATED_SURFACE_SET", "COMPLEX_TRIANGULATED_SURFACE_SET":
		return true
	}
	return false
}

func isTessName(name string) bool {
	switch name {
	case "TESSELLATED_SOLID", "TESSELLATED_SHELL", "TRIANGULATED_FACE", "COMPLEX_TRIANGULATED_FACE",
		"TRIANGULATED_SURFACE_SET", "COMPLEX_TRIANGULATED_SURFACE_SET":
		return true
	}
	return false
}

func isShapeRep(name string) bool {
	if !strings.HasSuffix(name, "REPRESENTATION") {
		return false
	}
	head := strings.TrimSuffix(name, "REPRESENTATION")
	if strings.Contains(head, "PRESENTATION") || strings.Contains(head, "DRAUGHTING") ||
		head == "DEFINITIONAL_" || strings.HasSuffix(head, "DEFINITION_") ||
		strings.Contains(head, "CONTEXT_DEPENDENT") {
		return false
	}
	return true
}

type unionFind map[int]int

func (u unionFind) find(x int) int {
	r := x
	for i := 0; i < 1<<20; i++ {
		p, ok := u[r]
		if !ok || p == r {
			break
		}
		r = p
	}
	for i := 0; i < 1<<20; i++ {
		p, ok := u[x]
		if !ok || p == x || p == r {
			break
		}
		u[x] = r
		x = p
	}
	return r
}

func (u unionFind) union(a, b int) {
	ra, rb := u.find(a), u.find(b)
	if ra != rb {
		if ra < rb {
			u[rb] = ra
		} else {
			u[ra] = rb
		}
	}
}

func (m *model) buildScene(ctx context.Context) (*scene, error) {
	f := m.f
	sc := &scene{items: make(map[int]*itemInfo), faceCol: make(map[int][3]uint8), itemScale: make(map[int]float64)}
	reps := map[int]*repInfo{}
	var repOrder []int
	var rrs, mapped, cdsrs, sdrs, styled, overStyled []int
	var solids []int
	units := map[int]unitInfo{}
	var names [][]byte
	for ei := range f.ents {
		if ei&0xffff == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		if f.complex(ei) {
			names = f.partNames(ei, names)
			isRep := false
			for _, n := range names {
				switch string(n) {
				case "REPRESENTATION_RELATIONSHIP":
					rrs = append(rrs, ei)
				case "STYLED_ITEM":
					styled = append(styled, ei)
				case "SHAPE_REPRESENTATION", "ADVANCED_BREP_SHAPE_REPRESENTATION":
					isRep = true
				}
			}
			if isRep {
				if a := f.argsOf(ei, "REPRESENTATION"); a != nil {
					reps[ei] = &repInfo{ei: ei, items: m.refs(a, 1), ctx: m.ref(a, 2)}
					repOrder = append(repOrder, ei)
				}
			}
			continue
		}
		nm := f.name(ei)
		switch nm {
		case "SHAPE_REPRESENTATION_RELATIONSHIP", "REPRESENTATION_RELATIONSHIP":
			rrs = append(rrs, ei)
		case "MAPPED_ITEM":
			mapped = append(mapped, ei)
		case "CONTEXT_DEPENDENT_SHAPE_REPRESENTATION":
			cdsrs = append(cdsrs, ei)
		case "SHAPE_DEFINITION_REPRESENTATION":
			sdrs = append(sdrs, ei)
		case "STYLED_ITEM":
			styled = append(styled, ei)
		case "OVER_RIDING_STYLED_ITEM", "CONTEXT_DEPENDENT_OVER_RIDING_STYLED_ITEM":
			overStyled = append(overStyled, ei)
		default:
			if isSolidItem(nm) {
				solids = append(solids, ei)
			} else if isShapeRep(nm) {
				a := f.args(ei)
				reps[ei] = &repInfo{ei: ei, items: m.refs(a, 1), ctx: m.ref(a, 2)}
				repOrder = append(repOrder, ei)
			}
		}
	}
	repUnit := func(r int) unitInfo {
		ri := reps[r]
		if ri == nil || ri.ctx < 0 {
			return unitInfo{scale: 1}
		}
		if u, ok := units[ri.ctx]; ok {
			return u
		}
		u := m.contextUnit(ri.ctx)
		units[ri.ctx] = u
		return u
	}
	uf := unionFind{}
	seenCtx := map[int]bool{}
	for _, r := range repOrder {
		uf[r] = r
		if c := reps[r].ctx; c >= 0 && !seenCtx[c] {
			seenCtx[c] = true
			if m.degreeContext(c) {
				m.angleDeg = true
			}
		}
	}
	pdReps := map[int][]int{}
	for _, s := range sdrs {
		a := f.args(s)
		pds := m.ref(a, 0)
		rep := m.ref(a, 1)
		if pds < 0 || rep < 0 {
			continue
		}
		pd := m.ref(f.args(pds), 2)
		if pd >= 0 {
			pdReps[pd] = append(pdReps[pd], rep)
		}
	}
	childOfRR := map[int]int{}
	for _, c := range cdsrs {
		a := f.args(c)
		rr := m.ref(a, 0)
		pds := m.ref(a, 1)
		if rr < 0 || pds < 0 {
			continue
		}
		nauo := m.ref(f.args(pds), 2)
		if nauo < 0 {
			continue
		}
		na := f.args(nauo)
		if na == nil {
			na = f.argsOf(nauo, "PRODUCT_DEFINITION_RELATIONSHIP")
		}
		child := m.ref(na, 4)
		if child >= 0 {
			childOfRR[rr] = child
		}
	}
	type rawLink struct {
		r1, r2 int
		op     int
		rr     int
	}
	var tlinks []rawLink
	for _, rr := range rrs {
		var a []value
		op := -1
		if f.complex(rr) {
			a = f.argsOf(rr, "REPRESENTATION_RELATIONSHIP")
			if t := f.argsOf(rr, "REPRESENTATION_RELATIONSHIP_WITH_TRANSFORMATION"); t != nil {
				op = m.ref(t, 0)
			}
		} else {
			a = f.args(rr)
		}
		r1, r2 := m.ref(a, 2), m.ref(a, 3)
		if r1 < 0 || r2 < 0 || reps[r1] == nil || reps[r2] == nil {
			continue
		}
		if op < 0 {
			uf.union(r1, r2)
			continue
		}
		tlinks = append(tlinks, rawLink{r1: r1, r2: r2, op: op, rr: rr})
	}
	var links []link
	for _, l := range tlinks {
		child, parent := l.r1, l.r2
		reversed := false
		if pd, ok := childOfRR[l.rr]; ok {
			g1, g2 := uf.find(l.r1), uf.find(l.r2)
			in1, in2 := false, false
			for _, r := range pdReps[pd] {
				if reps[r] == nil {
					continue
				}
				g := uf.find(r)
				in1 = in1 || g == g1
				in2 = in2 || g == g2
			}
			if in2 && !in1 {
				reversed = true
			}
		}
		u1, u2 := repUnit(l.r1), repUnit(l.r2)
		t, ok := m.relTransform(l.op, u1.scale, u2.scale)
		if !ok {
			continue
		}
		if reversed {
			child, parent = l.r2, l.r1
			inv, ok := t.inverse()
			if !ok {
				continue
			}
			t = inv
		}
		links = append(links, link{parent: uf.find(parent), child: uf.find(child), t: t})
	}
	repOfItem := map[int]int{}
	for _, r := range repOrder {
		for _, it := range reps[r].items {
			if _, ok := repOfItem[it]; !ok {
				repOfItem[it] = r
			}
		}
	}
	for _, mi := range mapped {
		parentRep, ok := repOfItem[mi]
		if !ok {
			continue
		}
		a := f.args(mi)
		rm := m.ref(a, 1)
		target := m.ref(a, 2)
		if rm < 0 || f.name(rm) != "REPRESENTATION_MAP" {
			continue
		}
		ra := f.args(rm)
		origin := m.ref(ra, 0)
		childRep := m.ref(ra, 1)
		if reps[childRep] == nil {
			continue
		}
		uc, up := repUnit(childRep), repUnit(parentRep)
		to, ok1 := m.transformOp(origin)
		tt, ok2 := m.transformOp(target)
		if !ok1 || !ok2 {
			continue
		}
		to.t = to.t.mul(uc.scale)
		tt.t = tt.t.mul(up.scale)
		inv, ok := to.inverse()
		if !ok {
			continue
		}
		links = append(links, link{parent: uf.find(parentRep), child: uf.find(childRep), t: tt.mul(inv)})
	}
	groupItems := map[int][]int{}
	groupUnit := map[int]unitInfo{}
	var groups []int
	seenGroup := map[int]bool{}
	for _, r := range repOrder {
		g := uf.find(r)
		if !seenGroup[g] {
			seenGroup[g] = true
			groups = append(groups, g)
		}
		u := repUnit(r)
		if gu, ok := groupUnit[g]; !ok || (!gu.ok && u.ok) {
			groupUnit[g] = u
		}
		for _, it := range reps[r].items {
			if isGeomItem(f.name(it)) {
				groupItems[g] = append(groupItems[g], it)
			}
		}
	}
	isChild := map[int]bool{}
	children := map[int][]link{}
	for _, l := range links {
		isChild[l.child] = true
		children[l.parent] = append(children[l.parent], l)
	}
	addItem := func(it int, x xform, s float64) bool {
		if len(sc.insts) >= maxInstances {
			return false
		}
		if _, ok := sc.items[it]; !ok {
			info := m.itemFaces(it)
			if info == nil {
				return true
			}
			sc.items[it] = info
			sc.order = append(sc.order, it)
			sc.itemScale[it] = s
		}
		sc.insts = append(sc.insts, instance{item: it, x: x.scaled(s), scale: s})
		return true
	}
	onPath := map[int]bool{}
	var visit func(g int, w xform, depth int)
	visit = func(g int, w xform, depth int) {
		if depth > maxAsmDepth || onPath[g] || len(sc.insts) >= maxInstances {
			return
		}
		onPath[g] = true
		u := groupUnit[g]
		if u.scale == 0 {
			u.scale = 1
		}
		if depth == 0 && sc.unit == "" && u.ok {
			sc.unit = u.name
		}
		seen := map[int]bool{}
		for _, it := range groupItems[g] {
			if seen[it] {
				continue
			}
			seen[it] = true
			if sc.unit == "" && u.ok {
				sc.unit = u.name
			}
			if !addItem(it, w, u.scale) {
				break
			}
		}
		for _, l := range children[g] {
			visit(l.child, w.mul(l.t), depth+1)
		}
		delete(onPath, g)
	}
	for _, g := range groups {
		if isChild[g] {
			continue
		}
		if len(groupItems[g]) == 0 && len(children[g]) == 0 {
			continue
		}
		visit(g, identityXform(), 0)
	}
	if len(sc.insts) == 0 {
		s := 1.0
		for _, g := range groups {
			if u := groupUnit[g]; u.ok {
				s = u.scale
				sc.unit = u.name
				break
			}
		}
		cands := solids
		if len(cands) == 0 {
			for ei := range f.ents {
				switch f.name(ei) {
				case "CLOSED_SHELL", "OPEN_SHELL", "TRIANGULATED_FACE", "COMPLEX_TRIANGULATED_FACE":
					cands = append(cands, ei)
				}
			}
		}
		for _, it := range cands {
			if !addItem(it, identityXform(), s) {
				break
			}
		}
	}
	hasBrep := false
	for _, in := range sc.insts {
		if !sc.items[in.item].tess {
			hasBrep = true
			break
		}
	}
	if hasBrep {
		k := 0
		for _, in := range sc.insts {
			if !sc.items[in.item].tess {
				sc.insts[k] = in
				k++
			}
		}
		sc.insts = sc.insts[:k]
	}
	m.colours(sc, styled, overStyled)
	return sc, nil
}

func (m *model) relTransform(op int, s1, s2 float64) (xform, bool) {
	f := m.f
	if f.name(op) == "ITEM_DEFINED_TRANSFORMATION" {
		a := f.args(op)
		p1, ok1 := m.transformOp(m.ref(a, 2))
		p2, ok2 := m.transformOp(m.ref(a, 3))
		if !ok1 || !ok2 {
			return xform{}, false
		}
		p1.t = p1.t.mul(s1)
		p2.t = p2.t.mul(s2)
		inv, ok := p1.inverse()
		if !ok {
			return xform{}, false
		}
		return p2.mul(inv), true
	}
	t, ok := m.transformOp(op)
	if !ok {
		return xform{}, false
	}
	t.t = t.t.mul(s2)
	if s1 != s2 && s1 > 0 {
		t.m = t.scaled(s2 / s1).m
	}
	return t, true
}

func (m *model) contextUnit(ctx int) unitInfo {
	a := m.f.argsOf(ctx, "GLOBAL_UNIT_ASSIGNED_CONTEXT")
	if a == nil {
		return unitInfo{scale: 1}
	}
	for _, u := range m.refs(a, 0) {
		if !m.f.hasPart(u, "LENGTH_UNIT") {
			continue
		}
		if ui := m.lengthUnit(u, 0); ui.ok {
			return ui
		}
	}
	return unitInfo{scale: 1}
}

func (m *model) degreeContext(ctx int) bool {
	a := m.f.argsOf(ctx, "GLOBAL_UNIT_ASSIGNED_CONTEXT")
	for _, u := range m.refs(a, 0) {
		if m.f.hasPart(u, "PLANE_ANGLE_UNIT") && m.f.hasPart(u, "CONVERSION_BASED_UNIT") {
			return true
		}
	}
	return false
}

var siPrefix = map[string]struct {
	f float64
	s string
}{
	"EXA": {1e18, "E"}, "PETA": {1e15, "P"}, "TERA": {1e12, "T"}, "GIGA": {1e9, "G"}, "MEGA": {1e6, "M"},
	"KILO": {1e3, "k"}, "HECTO": {1e2, "h"}, "DECA": {1e1, "da"}, "DECI": {1e-1, "d"}, "CENTI": {1e-2, "c"},
	"MILLI": {1e-3, "m"}, "MICRO": {1e-6, "u"}, "NANO": {1e-9, "n"}, "PICO": {1e-12, "p"},
	"FEMTO": {1e-15, "f"}, "ATTO": {1e-18, "a"},
}

func (m *model) lengthUnit(u int, depth int) unitInfo {
	if depth > 4 || u < 0 {
		return unitInfo{}
	}
	f := m.f
	if a := f.argsOf(u, "SI_UNIT"); a != nil {
		off := 0
		if !f.complex(u) {
			off = 1
		}
		if argEnum(a, 1+off) != "METRE" {
			return unitInfo{}
		}
		p := argEnum(a, off)
		if p == "" {
			return unitInfo{scale: 1000, name: "m", ok: true}
		}
		if px, ok := siPrefix[p]; ok {
			return unitInfo{scale: 1000 * px.f, name: px.s + "m", ok: true}
		}
		return unitInfo{}
	}
	if a := f.argsOf(u, "CONVERSION_BASED_UNIT"); a != nil {
		name := ""
		if len(a) > 0 && a[0].kind == vStr {
			name = strings.ToLower(strings.TrimSpace(decodeString(a[0].text)))
		}
		mw := m.ref(a, 1)
		ma := f.argsOf(mw, "MEASURE_WITH_UNIT")
		if ma == nil {
			ma = f.args(mw)
		}
		x, ok := argNum(ma, 0)
		if !ok || !(x > 0) {
			return unitInfo{}
		}
		base := m.lengthUnit(m.ref(ma, 1), depth+1)
		if !base.ok {
			return unitInfo{}
		}
		s := x * base.scale
		if !(s > 0) || !finite(s) {
			return unitInfo{}
		}
		if name == "" {
			name = "unit"
		}
		return unitInfo{scale: s, name: name, ok: true}
	}
	return unitInfo{}
}

func (m *model) itemFaces(ei int) *itemInfo {
	info := &itemInfo{ei: ei}
	nm := m.f.name(ei)
	if isTessName(nm) {
		info.tess = true
	}
	m.collect(ei, false, -1, info, 0)
	if len(info.faces) == 0 {
		return nil
	}
	return info
}

func (m *model) collect(ei int, flip bool, shell int, info *itemInfo, depth int) {
	if depth > maxTopoDepth || ei < 0 || len(info.faces) >= maxItemFaces {
		return
	}
	f := m.f
	a := f.args(ei)
	switch f.name(ei) {
	case "MANIFOLD_SOLID_BREP", "FACETED_BREP":
		m.collect(m.ref(a, 1), flip, shell, info, depth+1)
	case "BREP_WITH_VOIDS":
		m.collect(m.ref(a, 1), flip, shell, info, depth+1)
		for _, v := range m.refs(a, 2) {
			m.collect(v, flip, shell, info, depth+1)
		}
	case "SHELL_BASED_SURFACE_MODEL":
		for _, s := range m.refs(a, 1) {
			m.collect(s, flip, shell, info, depth+1)
		}
	case "CLOSED_SHELL", "OPEN_SHELL":
		for _, fc := range m.refs(a, 1) {
			m.collectFace(fc, flip, ei, info, 0)
		}
	case "ORIENTED_CLOSED_SHELL", "ORIENTED_OPEN_SHELL":
		m.collect(m.ref(a, 2), flip != !argBool(a, 3, true), shell, info, depth+1)
	case "TESSELLATED_SOLID", "TESSELLATED_SHELL":
		for _, fc := range m.refs(a, 1) {
			m.collectFace(fc, flip, ei, info, 0)
		}
	case "TRIANGULATED_FACE", "COMPLEX_TRIANGULATED_FACE", "TRIANGULATED_SURFACE_SET",
		"COMPLEX_TRIANGULATED_SURFACE_SET", "ADVANCED_FACE", "FACE_SURFACE", "ORIENTED_FACE":
		m.collectFace(ei, flip, shell, info, 0)
	}
}

func (m *model) collectFace(ei int, flip bool, shell int, info *itemInfo, depth int) {
	if depth > maxTopoDepth || ei < 0 || len(info.faces) >= maxItemFaces {
		return
	}
	switch m.f.name(ei) {
	case "ORIENTED_FACE":
		a := m.f.args(ei)
		m.collectFace(m.ref(a, 2), flip != !argBool(a, 3, true), shell, info, depth+1)
	case "ADVANCED_FACE", "FACE_SURFACE", "FACE", "TRIANGULATED_FACE", "COMPLEX_TRIANGULATED_FACE",
		"TRIANGULATED_SURFACE_SET", "COMPLEX_TRIANGULATED_SURFACE_SET", "TESSELLATED_FACE":
		info.faces = append(info.faces, faceRef{face: ei, flip: flip, shell: shell})
	}
}

func (m *model) colours(sc *scene, styled, over []int) {
	apply := func(list []int) {
		for _, s := range list {
			a := m.f.argsOf(s, "STYLED_ITEM")
			off := 0
			if a == nil {
				a = m.f.args(s)
			}
			if m.f.complex(s) {
				off = -1
			}
			target := m.ref(a, 2+off)
			if target < 0 {
				continue
			}
			if c, ok := m.styleColour(m.refs(a, 1+off)); ok {
				sc.faceCol[target] = c
			}
		}
	}
	apply(styled)
	apply(over)
}

func (m *model) styleColour(styles []int) ([3]uint8, bool) {
	var walk func(ei int, depth int) ([3]uint8, bool)
	walk = func(ei int, depth int) ([3]uint8, bool) {
		if depth > 10 || ei < 0 {
			return [3]uint8{}, false
		}
		f := m.f
		a := f.args(ei)
		switch f.name(ei) {
		case "COLOUR_RGB":
			r, ok1 := argNum(a, 1)
			g, ok2 := argNum(a, 2)
			b, ok3 := argNum(a, 3)
			if !ok1 || !ok2 || !ok3 {
				return [3]uint8{}, false
			}
			return [3]uint8{toByte(r), toByte(g), toByte(b)}, true
		case "DRAUGHTING_PRE_DEFINED_COLOUR":
			if len(a) > 0 && a[0].kind == vStr {
				if c, ok := predefColour[strings.ToLower(strings.TrimSpace(decodeString(a[0].text)))]; ok {
					return c, true
				}
			}
			return [3]uint8{}, false
		case "PRESENTATION_STYLE_ASSIGNMENT":
			for _, s := range m.refs(a, 0) {
				if c, ok := walk(s, depth+1); ok {
					return c, true
				}
			}
		case "PRESENTATION_STYLE_BY_CONTEXT":
			for _, s := range m.refs(a, 0) {
				if c, ok := walk(s, depth+1); ok {
					return c, true
				}
			}
		case "SURFACE_STYLE_USAGE":
			return walk(m.ref(a, 1), depth+1)
		case "SURFACE_SIDE_STYLE":
			for _, s := range m.refs(a, 1) {
				if c, ok := walk(s, depth+1); ok {
					return c, true
				}
			}
		case "SURFACE_STYLE_FILL_AREA":
			return walk(m.ref(a, 0), depth+1)
		case "FILL_AREA_STYLE":
			for _, s := range m.refs(a, 1) {
				if c, ok := walk(s, depth+1); ok {
					return c, true
				}
			}
		case "FILL_AREA_STYLE_COLOUR":
			return walk(m.ref(a, 1), depth+1)
		case "SURFACE_STYLE_RENDERING", "SURFACE_STYLE_RENDERING_WITH_PROPERTIES":
			return walk(m.ref(a, 1), depth+1)
		}
		return [3]uint8{}, false
	}
	for _, s := range styles {
		if c, ok := walk(s, 0); ok {
			return c, true
		}
	}
	return [3]uint8{}, false
}

func toByte(x float64) uint8 {
	if !(x > 0) {
		return 0
	}
	if x >= 1 {
		return 255
	}
	return uint8(math.Round(x * 255))
}

var predefColour = map[string][3]uint8{
	"red": {255, 0, 0}, "green": {0, 255, 0}, "blue": {0, 0, 255}, "yellow": {255, 255, 0},
	"magenta": {255, 0, 255}, "cyan": {0, 255, 255}, "black": {0, 0, 0}, "white": {255, 255, 255},
}
