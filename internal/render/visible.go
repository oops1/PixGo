package render

import (
	"image"
	"math"

	"github.com/oops1/PixGo/internal/codec"
)

const maxLevelShift = 40

func LevelShift(scale float64) int {
	if !validScale(scale) {
		return 0
	}
	n := 0
	for n < maxLevelShift && scale*float64(uint(2)<<uint(n)) <= 1 {
		n++
	}
	return n
}

func normalizeMapping(m Mapping) Mapping {
	if math.Abs(m.Scale-1) < unitScaleTolerance {
		m.Scale = 1
		m.OriginX = math.Floor(m.OriginX + 0.5)
		m.OriginY = math.Floor(m.OriginY + 0.5)
	}
	return m
}

func validScale(scale float64) bool { return scale > 0 && !math.IsInf(scale, 0) }

func VisibleRect(m Mapping, w, h int, dstW, dstH int) image.Rectangle {
	return visibleRect(m, w, h, image.Rect(0, 0, dstW, dstH))
}

func visibleRect(m Mapping, w, h int, px image.Rectangle) image.Rectangle {
	if w <= 0 || h <= 0 || px.Empty() || !validScale(m.Scale) {
		return image.Rectangle{}
	}
	m = normalizeMapping(m)
	ow, oh := m.Orient.Size(w, h)
	ua, ub := axisSpan(m.OriginX, m.Scale, px.Min.X, px.Max.X, ow)
	va, vb := axisSpan(m.OriginY, m.Scale, px.Min.Y, px.Max.Y, oh)
	if ua > ub || va > vb {
		return image.Rectangle{}
	}
	fw, fh := float64(w), float64(h)
	x0, y0 := m.Orient.ToSource(ua, va, fw, fh)
	x1, y1 := m.Orient.ToSource(ub, vb, fw, fh)
	r := image.Rect(
		int(math.Floor(math.Min(x0, x1))),
		int(math.Floor(math.Min(y0, y1))),
		int(math.Ceil(math.Max(x0, x1))),
		int(math.Ceil(math.Max(y0, y1))),
	)
	return r.Intersect(image.Rect(0, 0, w, h))
}

func axisSpan(origin, scale float64, from, to, extent int) (lo, hi float64) {
	lo = origin + (float64(from)+0.5)/scale
	hi = origin + (float64(to-1)+0.5)/scale
	lo = math.Max(lo, 0)
	hi = math.Min(hi, math.Nextafter(float64(extent), 0))
	return lo, hi
}

func levelNeed(r image.Rectangle, shift, w, h int) image.Rectangle {
	if r.Empty() {
		return image.Rectangle{}
	}
	lw, lh := codec.LevelSize(w, h, shift)
	round := 1<<uint(shift) - 1
	need := image.Rect(
		(r.Min.X>>uint(shift))-1,
		(r.Min.Y>>uint(shift))-1,
		((r.Max.X+round)>>uint(shift))+1,
		((r.Max.Y+round)>>uint(shift))+1,
	)
	return need.Intersect(image.Rect(0, 0, lw, lh))
}
