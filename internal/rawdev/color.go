package rawdev

import "math"

type Matrix [9]float64

var identity = Matrix{1, 0, 0, 0, 1, 0, 0, 0, 1}

func (a Matrix) Mul(b Matrix) Matrix {
	var o Matrix
	for r := 0; r < 3; r++ {
		for c := 0; c < 3; c++ {
			o[r*3+c] = a[r*3]*b[c] + a[r*3+1]*b[3+c] + a[r*3+2]*b[6+c]
		}
	}
	return o
}

func (a Matrix) Apply(v [3]float64) [3]float64 {
	return [3]float64{
		a[0]*v[0] + a[1]*v[1] + a[2]*v[2],
		a[3]*v[0] + a[4]*v[1] + a[5]*v[2],
		a[6]*v[0] + a[7]*v[1] + a[8]*v[2],
	}
}

func (a Matrix) Inverse() (Matrix, bool) {
	c00 := a[4]*a[8] - a[5]*a[7]
	c01 := a[5]*a[6] - a[3]*a[8]
	c02 := a[3]*a[7] - a[4]*a[6]
	det := a[0]*c00 + a[1]*c01 + a[2]*c02
	if det == 0 || math.IsNaN(det) || math.IsInf(det, 0) || math.Abs(det) < 1e-12 {
		return Matrix{}, false
	}
	inv := 1 / det
	return Matrix{
		c00 * inv, (a[2]*a[7] - a[1]*a[8]) * inv, (a[1]*a[5] - a[2]*a[4]) * inv,
		c01 * inv, (a[0]*a[8] - a[2]*a[6]) * inv, (a[2]*a[3] - a[0]*a[5]) * inv,
		c02 * inv, (a[1]*a[6] - a[0]*a[7]) * inv, (a[0]*a[4] - a[1]*a[3]) * inv,
	}, true
}

func (a Matrix) finite() bool {
	for _, v := range a {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return false
		}
	}
	return true
}

func diag(v [3]float64) Matrix { return Matrix{v[0], 0, 0, 0, v[1], 0, 0, 0, v[2]} }

var bradford = Matrix{0.8951, 0.2664, -0.1614, -0.7502, 1.7135, 0.0367, 0.0389, -0.0685, 1.0296}

var (
	whiteD50 = [3]float64{0.96422, 1, 0.82521}
	xyD50    = [2]float64{0.3457, 0.3585}
)

var xyzD50ToSRGB = Matrix{
	3.1338561, -1.6168667, -0.4906146,
	-0.9787684, 1.9161415, 0.0334540,
	0.0719453, -0.2289914, 1.4052427,
}

func xyToXYZ(xy [2]float64) [3]float64 {
	return [3]float64{xy[0] / xy[1], 1, (1 - xy[0] - xy[1]) / xy[1]}
}

func xyzToXY(v [3]float64) ([2]float64, bool) {
	s := v[0] + v[1] + v[2]
	if s <= 0 || math.IsNaN(s) || math.IsInf(s, 0) {
		return [2]float64{}, false
	}
	return [2]float64{v[0] / s, v[1] / s}, true
}

func adapt(from, to [3]float64) Matrix {
	inv, _ := bradford.Inverse()
	src := bradford.Apply(from)
	dst := bradford.Apply(to)
	var scale [3]float64
	for i := range scale {
		if src[i] == 0 {
			return identity
		}
		scale[i] = dst[i] / src[i]
	}
	return inv.Mul(diag(scale)).Mul(bradford)
}

var robertson = [31][4]float64{
	{0, 0.18006, 0.26352, -0.24341},
	{10, 0.18066, 0.26589, -0.25479},
	{20, 0.18133, 0.26846, -0.26876},
	{30, 0.18208, 0.27119, -0.28539},
	{40, 0.18293, 0.27407, -0.30470},
	{50, 0.18388, 0.27709, -0.32675},
	{60, 0.18494, 0.28021, -0.35156},
	{70, 0.18611, 0.28342, -0.37915},
	{80, 0.18740, 0.28668, -0.40955},
	{90, 0.18880, 0.28997, -0.44278},
	{100, 0.19032, 0.29326, -0.47888},
	{125, 0.19462, 0.30141, -0.58204},
	{150, 0.19962, 0.30921, -0.70471},
	{175, 0.20525, 0.31647, -0.84901},
	{200, 0.21142, 0.32312, -1.0182},
	{225, 0.21807, 0.32909, -1.2168},
	{250, 0.22511, 0.33439, -1.4512},
	{275, 0.23247, 0.33904, -1.7298},
	{300, 0.24010, 0.34308, -2.0637},
	{325, 0.24702, 0.34655, -2.4681},
	{350, 0.25591, 0.34951, -2.9641},
	{375, 0.26400, 0.35200, -3.5814},
	{400, 0.27218, 0.35407, -4.3633},
	{425, 0.28039, 0.35577, -5.3762},
	{450, 0.28863, 0.35714, -6.7262},
	{475, 0.29685, 0.35823, -8.5955},
	{500, 0.30505, 0.35907, -11.324},
	{525, 0.31320, 0.35968, -15.628},
	{550, 0.32129, 0.36011, -23.325},
	{575, 0.32931, 0.36038, -40.770},
	{600, 0.33724, 0.36051, -116.45},
}

func Temperature(xy [2]float64) float64 {
	den := -2*xy[0] + 12*xy[1] + 3
	if den <= 0 {
		return 5000
	}
	u := 4 * xy[0] / den
	v := 6 * xy[1] / den
	last := 0.0
	for i := range robertson {
		row := robertson[i]
		d := ((v - row[2]) - row[3]*(u-row[1])) / math.Sqrt(1+row[3]*row[3])
		if i > 0 && (d <= 0 || i == len(robertson)-1) {
			if i == len(robertson)-1 && d > 0 {
				return 1e6 / row[0]
			}
			f := last / (last - d)
			mired := robertson[i-1][0] + f*(row[0]-robertson[i-1][0])
			if mired < 1 {
				mired = 1
			}
			return 1e6 / mired
		}
		last = d
	}
	return 5000
}

func IlluminantTemperature(code uint16) float64 {
	switch code {
	case 1, 4, 9:
		return 5500
	case 2:
		return 4200
	case 3:
		return 2850
	case 10:
		return 6500
	case 11:
		return 7500
	case 12:
		return 6430
	case 13:
		return 5000
	case 14:
		return 4150
	case 15:
		return 3450
	case 16:
		return 2930
	case 17:
		return 2856
	case 18:
		return 4874
	case 19:
		return 6774
	case 20:
		return 5503
	case 21:
		return 6504
	case 22:
		return 7504
	case 23:
		return 5003
	case 24:
		return 3200
	}
	return 0
}

type Color struct {
	ColorMatrix [2]Matrix
	HasCM       [2]bool
	Forward     [2]Matrix
	HasFM       [2]bool
	Calibration [2]Matrix
	HasCC       [2]bool
	Analog      [3]float64
	Illuminant  [2]uint16
	Neutral     *[3]float64
	WhiteXY     *[2]float64
}

type Rendering struct {
	Matrix Matrix
	Gain   [3]float64
}

func (c Color) weights(t float64) (w [2]float64, use [2]bool) {
	var avail []int
	for i := 0; i < 2; i++ {
		if c.HasCM[i] || c.HasFM[i] {
			avail = append(avail, i)
		}
	}
	switch len(avail) {
	case 0:
		return
	case 1:
		use[avail[0]] = true
		w[avail[0]] = 1
		return
	}
	t0, t1 := IlluminantTemperature(c.Illuminant[0]), IlluminantTemperature(c.Illuminant[1])
	if t0 == 0 || t1 == 0 || t0 == t1 {
		use[0], w[0] = true, 1
		return
	}
	low, high := 0, 1
	if t0 > t1 {
		low, high = 1, 0
		t0, t1 = t1, t0
	}
	wl := (1/t - 1/t1) / (1/t0 - 1/t1)
	wl = math.Min(1, math.Max(0, wl))
	use[low], use[high] = true, true
	w[low], w[high] = wl, 1-wl
	return
}

func (c Color) xyzToCamera(which [2]float64, use [2]bool) (Matrix, bool) {
	var sum Matrix
	found := false
	for i := 0; i < 2; i++ {
		if !use[i] || !c.HasCM[i] {
			continue
		}
		cc := identity
		if c.HasCC[i] {
			cc = c.Calibration[i]
		}
		m := diag(c.Analog).Mul(cc).Mul(c.ColorMatrix[i])
		for k := range sum {
			sum[k] += which[i] * m[k]
		}
		found = true
	}
	if !found {
		return Matrix{}, false
	}
	norm := 0.0
	for i := 0; i < 2; i++ {
		if use[i] && c.HasCM[i] {
			norm += which[i]
		}
	}
	if norm <= 0 {
		return Matrix{}, false
	}
	for k := range sum {
		sum[k] /= norm
	}
	return sum, true
}

func (c Color) forward(which [2]float64, use [2]bool) (Matrix, bool) {
	var sum Matrix
	norm := 0.0
	for i := 0; i < 2; i++ {
		if !use[i] || !c.HasFM[i] {
			continue
		}
		for k := range sum {
			sum[k] += which[i] * c.Forward[i][k]
		}
		norm += which[i]
	}
	if norm <= 0 {
		return Matrix{}, false
	}
	for k := range sum {
		sum[k] /= norm
	}
	return sum, true
}

func (c Color) Resolve() (Rendering, error) {
	if !c.HasCM[0] && !c.HasCM[1] {
		return Rendering{}, unsupported("no color matrix")
	}
	for i := range c.Analog {
		if !(c.Analog[i] > 0) {
			return Rendering{}, corrupt("analog balance")
		}
	}
	for i := 0; i < 2; i++ {
		if (c.HasCM[i] && !c.ColorMatrix[i].finite()) || (c.HasFM[i] && !c.Forward[i].finite()) || (c.HasCC[i] && !c.Calibration[i].finite()) {
			return Rendering{}, corrupt("matrix values")
		}
	}
	var neutral [3]float64
	var xy [2]float64
	var m Matrix
	var wts [2]float64
	var use [2]bool
	solve := func(xy [2]float64) (Matrix, bool) {
		wts, use = c.weights(Temperature(xy))
		return c.xyzToCamera(wts, use)
	}
	switch {
	case c.Neutral != nil:
		neutral = *c.Neutral
		for _, v := range neutral {
			if !(v > 1e-6) || math.IsInf(v, 0) {
				return Rendering{}, corrupt("camera neutral")
			}
		}
		xy = xyD50
		for i := 0; i < 40; i++ {
			mm, ok := solve(xy)
			if !ok {
				return Rendering{}, unsupported("no color matrix for the illuminant")
			}
			inv, ok := mm.Inverse()
			if !ok {
				return Rendering{}, corrupt("singular color matrix")
			}
			next, ok := xyzToXY(inv.Apply(neutral))
			if !ok {
				return Rendering{}, corrupt("camera neutral")
			}
			done := math.Abs(next[0]-xy[0]) < 1e-7 && math.Abs(next[1]-xy[1]) < 1e-7
			xy = next
			if done {
				break
			}
		}
		var ok bool
		if m, ok = solve(xy); !ok {
			return Rendering{}, unsupported("no color matrix for the illuminant")
		}
	default:
		xy = xyD50
		if c.WhiteXY != nil {
			xy = *c.WhiteXY
			if !(xy[1] > 1e-3) || xy[0] < 0 || xy[0]+xy[1] > 1 {
				return Rendering{}, corrupt("white point")
			}
		}
		var ok bool
		if m, ok = solve(xy); !ok {
			return Rendering{}, unsupported("no color matrix for the illuminant")
		}
		neutral = m.Apply(xyToXYZ(xy))
		for _, v := range neutral {
			if !(v > 1e-6) || math.IsInf(v, 0) {
				return Rendering{}, corrupt("camera neutral")
			}
		}
	}
	white := xyToXYZ(xy)
	var toXYZ Matrix
	if fm, ok := c.forward(wts, use); ok {
		toXYZ = fm
	} else {
		inv, ok := m.Inverse()
		if !ok {
			return Rendering{}, corrupt("singular color matrix")
		}
		toXYZ = adapt(white, whiteD50).Mul(inv).Mul(diag(neutral))
	}
	final := xyzD50ToSRGB.Mul(toXYZ)
	for r := 0; r < 3; r++ {
		sum := final[r*3] + final[r*3+1] + final[r*3+2]
		if !(math.Abs(sum) > 1e-9) || math.IsNaN(sum) || math.IsInf(sum, 0) {
			return Rendering{}, corrupt("color matrix")
		}
		for k := 0; k < 3; k++ {
			final[r*3+k] /= sum
		}
	}
	top := math.Max(neutral[0], math.Max(neutral[1], neutral[2]))
	var gain [3]float64
	for i := range gain {
		gain[i] = top / neutral[i]
	}
	return Rendering{Matrix: final, Gain: gain}, nil
}
