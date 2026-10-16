package jpegxform

type blockMap struct {
	perm [blockSize]uint8
	sign [blockSize]int16
}

type signRule uint8

const (
	signNone signRule = iota
	signOddCol
	signOddRow
	signOddSum
)

func newBlockMap(transpose bool, rule signRule) blockMap {
	var m blockMap
	for r := 0; r < 8; r++ {
		for c := 0; c < 8; c++ {
			n := r*8 + c
			m.perm[n] = uint8(n)
			if transpose {
				m.perm[n] = uint8(c*8 + r)
			}
			m.sign[n] = 1
			odd := false
			switch rule {
			case signOddCol:
				odd = c&1 == 1
			case signOddRow:
				odd = r&1 == 1
			case signOddSum:
				odd = (r+c)&1 == 1
			}
			if odd {
				m.sign[n] = -1
			}
		}
	}
	return m
}

var blockMaps = [opCount]blockMap{
	Identity:   newBlockMap(false, signNone),
	FlipH:      newBlockMap(false, signOddCol),
	FlipV:      newBlockMap(false, signOddRow),
	Rotate90:   newBlockMap(true, signOddCol),
	Rotate180:  newBlockMap(false, signOddSum),
	Rotate270:  newBlockMap(true, signOddRow),
	Transpose:  newBlockMap(true, signNone),
	Transverse: newBlockMap(true, signOddSum),
}

func (m *blockMap) apply(src []int16, dst *[blockSize]int16) {
	for n := 0; n < blockSize; n++ {
		dst[n] = src[m.perm[n]] * m.sign[n]
	}
}

type plan struct {
	op           Op
	srcMX, srcMY int
	outMX, outMY int
	outW, outH   int
	cropW, cropH int
	comps        []planComp
}

type planComp struct {
	srcWide, srcHigh int
	outH, outV       int
}

func edgeCrop(fr *frame, op Op) (cropW, cropH int, perfect bool) {
	needW, needH := op.edgeNeeds()
	cropW, cropH = fr.width, fr.height
	perfect = true
	if needW && fr.width%fr.mcuW != 0 {
		perfect = false
		cropW = fr.width - fr.width%fr.mcuW
	}
	if needH && fr.height%fr.mcuH != 0 {
		perfect = false
		cropH = fr.height - fr.height%fr.mcuH
	}
	return cropW, cropH, perfect
}

func newPlan(fr *frame, op Op) *plan {
	cropW, cropH, _ := edgeCrop(fr, op)
	p := &plan{op: op, cropW: cropW, cropH: cropH}
	p.srcMX = (cropW + fr.mcuW - 1) / fr.mcuW
	p.srcMY = (cropH + fr.mcuH - 1) / fr.mcuH
	p.outMX, p.outMY = p.srcMX, p.srcMY
	p.outW, p.outH = cropW, cropH
	if op.Transposes() {
		p.outMX, p.outMY = p.srcMY, p.srcMX
		p.outW, p.outH = cropH, cropW
	}
	p.comps = make([]planComp, len(fr.comps))
	for i, c := range fr.comps {
		pc := planComp{srcWide: p.srcMX * c.h, srcHigh: p.srcMY * c.v, outH: c.h, outV: c.v}
		if op.Transposes() {
			pc.outH, pc.outV = c.v, c.h
		}
		p.comps[i] = pc
	}
	return p
}

func (p *plan) fetch(g *grid, comp, bx, by int, scratch *[blockSize]int16) []int16 {
	pc := &p.comps[comp]
	var sx, sy int
	switch p.op {
	case Identity:
		return g.block(bx, by)
	case FlipH:
		sx, sy = pc.srcWide-1-bx, by
	case FlipV:
		sx, sy = bx, pc.srcHigh-1-by
	case Rotate180:
		sx, sy = pc.srcWide-1-bx, pc.srcHigh-1-by
	case Transpose:
		sx, sy = by, bx
	case Rotate90:
		sx, sy = by, pc.srcHigh-1-bx
	case Rotate270:
		sx, sy = pc.srcWide-1-by, bx
	default:
		sx, sy = pc.srcWide-1-by, pc.srcHigh-1-bx
	}
	blockMaps[p.op].apply(g.block(sx, sy), scratch)
	return scratch[:]
}
