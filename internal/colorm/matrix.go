package colorm

import "math"

type Mat3 [9]float64

type Vec3 [3]float64

var d50 = Vec3{0.9642, 1.0, 0.8249}

var identity3 = Mat3{1, 0, 0, 0, 1, 0, 0, 0, 1}

func (m Mat3) Mul(n Mat3) Mat3 {
	var r Mat3
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			r[i*3+j] = m[i*3]*n[j] + m[i*3+1]*n[3+j] + m[i*3+2]*n[6+j]
		}
	}
	return r
}

func (m Mat3) Apply(v Vec3) Vec3 {
	return Vec3{
		m[0]*v[0] + m[1]*v[1] + m[2]*v[2],
		m[3]*v[0] + m[4]*v[1] + m[5]*v[2],
		m[6]*v[0] + m[7]*v[1] + m[8]*v[2],
	}
}

func (m Mat3) Det() float64 {
	return m[0]*(m[4]*m[8]-m[5]*m[7]) - m[1]*(m[3]*m[8]-m[5]*m[6]) + m[2]*(m[3]*m[7]-m[4]*m[6])
}

func (m Mat3) Inverse() (Mat3, bool) {
	det := m.Det()
	if det == 0 || math.IsNaN(det) || math.IsInf(det, 0) {
		return Mat3{}, false
	}
	id := 1 / det
	return Mat3{
		(m[4]*m[8] - m[5]*m[7]) * id, (m[2]*m[7] - m[1]*m[8]) * id, (m[1]*m[5] - m[2]*m[4]) * id,
		(m[5]*m[6] - m[3]*m[8]) * id, (m[0]*m[8] - m[2]*m[6]) * id, (m[2]*m[3] - m[0]*m[5]) * id,
		(m[3]*m[7] - m[4]*m[6]) * id, (m[1]*m[6] - m[0]*m[7]) * id, (m[0]*m[4] - m[1]*m[3]) * id,
	}, true
}

func (m Mat3) finite() bool {
	for _, v := range m {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return false
		}
	}
	return true
}

func (m Mat3) near(n Mat3, tol float64) bool {
	for i := range m {
		if math.Abs(m[i]-n[i]) > tol {
			return false
		}
	}
	return true
}

var bradford = Mat3{
	0.8951, 0.2664, -0.1614,
	-0.7502, 1.7135, 0.0367,
	0.0389, -0.0685, 1.0296,
}

func Adaptation(from, to Vec3) Mat3 {
	inv, _ := bradford.Inverse()
	src := bradford.Apply(from)
	dst := bradford.Apply(to)
	scale := Mat3{dst[0] / src[0], 0, 0, 0, dst[1] / src[1], 0, 0, 0, dst[2] / src[2]}
	return inv.Mul(scale).Mul(bradford)
}

func xyFromXYZ(x, y float64) Vec3 {
	if y == 0 {
		return Vec3{}
	}
	return Vec3{x / y, 1, (1 - x - y) / y}
}

func PrimariesMatrix(rx, ry, gx, gy, bx, by, wx, wy float64) (Mat3, bool) {
	r, g, b := xyFromXYZ(rx, ry), xyFromXYZ(gx, gy), xyFromXYZ(bx, by)
	w := xyFromXYZ(wx, wy)
	m := Mat3{r[0], g[0], b[0], r[1], g[1], b[1], r[2], g[2], b[2]}
	inv, ok := m.Inverse()
	if !ok {
		return Mat3{}, false
	}
	s := inv.Apply(w)
	return Mat3{
		m[0] * s[0], m[1] * s[1], m[2] * s[2],
		m[3] * s[0], m[4] * s[1], m[5] * s[2],
		m[6] * s[0], m[7] * s[1], m[8] * s[2],
	}, true
}

func PrimariesToD50(rx, ry, gx, gy, bx, by, wx, wy float64) (Mat3, bool) {
	m, ok := PrimariesMatrix(rx, ry, gx, gy, bx, by, wx, wy)
	if !ok {
		return Mat3{}, false
	}
	return Adaptation(xyFromXYZ(wx, wy), d50).Mul(m), true
}

const (
	labEpsilon = 216.0 / 24389
	labKappa   = 24389.0 / 27
)

func labToXYZ(l, a, b float64) Vec3 {
	fy := (l + 16) / 116
	fx := fy + a/500
	fz := fy - b/200
	inv := func(f float64) float64 {
		if f3 := f * f * f; f3 > labEpsilon {
			return f3
		}
		return (116*f - 16) / labKappa
	}
	var y float64
	if l > labKappa*labEpsilon {
		y = fy * fy * fy
	} else {
		y = l / labKappa
	}
	return Vec3{inv(fx) * d50[0], y * d50[1], inv(fz) * d50[2]}
}

func xyzToLab(v Vec3) (float64, float64, float64) {
	f := func(t float64) float64 {
		if t > labEpsilon {
			return math.Cbrt(t)
		}
		return (labKappa*t + 16) / 116
	}
	fx, fy, fz := f(v[0]/d50[0]), f(v[1]/d50[1]), f(v[2]/d50[2])
	return 116*fy - 16, 500 * (fx - fy), 200 * (fy - fz)
}
