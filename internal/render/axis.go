package render

import (
	"math"

	"github.com/oops1/PixGo/internal/codec"
)

type coordMapper struct {
	orient codec.Orientation
	w, h   float64
	fromU  bool
	wantX  bool
}

func (c coordMapper) at(t float64) float64 {
	var x, y float64
	if c.fromU {
		x, y = c.orient.ToSource(t, 0, c.w, c.h)
	} else {
		x, y = c.orient.ToSource(0, t, c.w, c.h)
	}
	if c.wantX {
		return x
	}
	return y
}

type axisSpec struct {
	n       int
	base    int
	origin  float64
	scale   float64
	extent  float64
	mapper  coordMapper
	shift   float64
	limit   int
	mul     int
	nearest bool
	checker bool
	cell    float64
}

type axis struct {
	off0     []int
	off1     []int
	frac     []uint32
	par      []uint8
	lo       int
	hi       int
	copyable bool
}

func (a *axis) resize(n int) {
	if cap(a.off0) < n {
		a.off0 = make([]int, n)
		a.off1 = make([]int, n)
		a.frac = make([]uint32, n)
		a.par = make([]uint8, n)
		return
	}
	a.off0 = a.off0[:n]
	a.off1 = a.off1[:n]
	a.frac = a.frac[:n]
	a.par = a.par[:n]
}

func (a *axis) clear() {
	a.lo, a.hi = 0, 0
	a.copyable = false
	a.off0, a.off1, a.frac, a.par = a.off0[:0], a.off1[:0], a.frac[:0], a.par[:0]
}

func (a *axis) build(s *axisSpec) {
	a.resize(s.n)
	first, last := -1, -1
	for k := 0; k < s.n; k++ {
		d := float64(s.base + k)
		t := s.origin + (d+0.5)/s.scale
		if s.checker {
			a.par[k] = uint8(int64(math.Floor((d+0.5+s.origin*s.scale)/s.cell)) & 1)
		} else {
			a.par[k] = 0
		}
		if !(t >= 0 && t < s.extent) {
			a.off0[k], a.off1[k], a.frac[k] = 0, 0, 0
			continue
		}
		if first < 0 {
			first = k
		}
		last = k
		c := s.mapper.at(t) * s.shift
		if s.nearest {
			i := clampInt(int(math.Floor(c)), 0, s.limit-1)
			a.off0[k], a.off1[k], a.frac[k] = i*s.mul, i*s.mul, 0
			continue
		}
		pos := c - 0.5
		fl := math.Floor(pos)
		i0 := int(fl)
		switch {
		case i0 < 0:
			a.off0[k], a.off1[k], a.frac[k] = 0, 0, 0
		case i0 >= s.limit-1:
			e := (s.limit - 1) * s.mul
			a.off0[k], a.off1[k], a.frac[k] = e, e, 0
		default:
			f := uint32((pos - fl) * 65536)
			if f > 65535 {
				f = 65535
			}
			a.off0[k], a.off1[k], a.frac[k] = i0*s.mul, (i0+1)*s.mul, f
		}
	}
	if first < 0 {
		a.lo, a.hi, a.copyable = 0, 0, false
		return
	}
	a.lo, a.hi = first, last+1
	a.copyable = true
	if s.mul != 4 {
		a.copyable = false
		return
	}
	start := a.off0[first]
	for k := first; k <= last; k++ {
		if a.frac[k] != 0 || a.off0[k] != start+(k-first)*4 {
			a.copyable = false
			return
		}
	}
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
