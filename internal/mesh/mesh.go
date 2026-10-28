package mesh

import (
	"errors"
	"math"
)

var ErrTooManyTriangles = errors.New("mesh: too many triangles")

const DefaultMaxTriangles = 12_000_000

type Mesh struct {
	V      []float32
	N      []float32
	C      []uint8
	Min    [3]float32
	Max    [3]float32
	Parts  int
	Failed int
}

func (m *Mesh) Triangles() int {
	if m == nil {
		return 0
	}
	return len(m.V) / 9
}

func (m *Mesh) Empty() bool { return m.Triangles() == 0 }

func (m *Mesh) HasNormals() bool { return m != nil && len(m.N) == len(m.V) && len(m.N) > 0 }

func (m *Mesh) HasColors() bool { return m != nil && len(m.C) == m.Triangles()*3 && len(m.C) > 0 }

func (m *Mesh) Bytes() int64 {
	if m == nil {
		return 0
	}
	return int64(len(m.V))*4 + int64(len(m.N))*4 + int64(len(m.C))
}

func (m *Mesh) UpdateBounds() {
	if m.Empty() {
		m.Min, m.Max = [3]float32{}, [3]float32{}
		return
	}
	lo := [3]float32{math.MaxFloat32, math.MaxFloat32, math.MaxFloat32}
	hi := [3]float32{-math.MaxFloat32, -math.MaxFloat32, -math.MaxFloat32}
	for i := 0; i+2 < len(m.V); i += 3 {
		for k := 0; k < 3; k++ {
			v := m.V[i+k]
			if v < lo[k] {
				lo[k] = v
			}
			if v > hi[k] {
				hi[k] = v
			}
		}
	}
	m.Min, m.Max = lo, hi
}

func (m *Mesh) Center() [3]float32 {
	return [3]float32{(m.Min[0] + m.Max[0]) / 2, (m.Min[1] + m.Max[1]) / 2, (m.Min[2] + m.Max[2]) / 2}
}

func (m *Mesh) Radius() float32 {
	dx, dy, dz := m.Max[0]-m.Min[0], m.Max[1]-m.Min[1], m.Max[2]-m.Min[2]
	return float32(math.Sqrt(float64(dx*dx+dy*dy+dz*dz))) / 2
}

func (m *Mesh) Area() float64 {
	var s float64
	for i := 0; i+8 < len(m.V); i += 9 {
		ax, ay, az := float64(m.V[i+3]-m.V[i]), float64(m.V[i+4]-m.V[i+1]), float64(m.V[i+5]-m.V[i+2])
		bx, by, bz := float64(m.V[i+6]-m.V[i]), float64(m.V[i+7]-m.V[i+1]), float64(m.V[i+8]-m.V[i+2])
		cx, cy, cz := ay*bz-az*by, az*bx-ax*bz, ax*by-ay*bx
		s += math.Sqrt(cx*cx+cy*cy+cz*cz) / 2
	}
	return s
}

func (m *Mesh) Volume() float64 {
	var s float64
	for i := 0; i+8 < len(m.V); i += 9 {
		ax, ay, az := float64(m.V[i]), float64(m.V[i+1]), float64(m.V[i+2])
		bx, by, bz := float64(m.V[i+3]), float64(m.V[i+4]), float64(m.V[i+5])
		cx, cy, cz := float64(m.V[i+6]), float64(m.V[i+7]), float64(m.V[i+8])
		s += ax*(by*cz-bz*cy) - ay*(bx*cz-bz*cx) + az*(bx*cy-by*cx)
	}
	return s / 6
}
