package colorm

import (
	"math"
	"sort"
)

type Curve struct {
	eval     func(float64) float64
	inv      func(float64) float64
	identity bool
	gamma    float64
}

func (c *Curve) IsIdentity() bool { return c == nil || c.identity }

func (c *Curve) Eval(x float64) float64 {
	if c == nil || c.identity {
		return clamp01(x)
	}
	return c.eval(clamp01(x))
}

func (c *Curve) Inverse(y float64) float64 {
	if c == nil || c.identity {
		return clamp01(y)
	}
	return clamp01(c.inv(y))
}

func clamp01(x float64) float64 {
	if x != x || x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}

func identityCurve() *Curve { return &Curve{identity: true} }

func gammaCurve(g float64) *Curve {
	if g <= 0 || math.IsNaN(g) || math.IsInf(g, 0) {
		g = 1
	}
	if math.Abs(g-1) < 1e-9 {
		return identityCurve()
	}
	return &Curve{
		gamma: g,
		eval:  func(x float64) float64 { return math.Pow(x, g) },
		inv:   func(y float64) float64 { return math.Pow(clamp01(y), 1/g) },
	}
}

func funcCurve(f, finv func(float64) float64) *Curve {
	return &Curve{eval: f, inv: finv}
}

func tableCurve(t []float64) *Curve {
	n := len(t)
	if n < 2 {
		return identityCurve()
	}
	tab := append([]float64(nil), t...)
	last := float64(n - 1)
	eval := func(x float64) float64 {
		p := x * last
		i := int(p)
		if i >= n-1 {
			return tab[n-1]
		}
		f := p - float64(i)
		return tab[i] + (tab[i+1]-tab[i])*f
	}
	inc := tab[n-1] >= tab[0]
	inv := func(y float64) float64 {
		if inc {
			i := sort.Search(n, func(i int) bool { return tab[i] >= y })
			switch {
			case i <= 0:
				return 0
			case i >= n:
				return 1
			}
			lo, hi := tab[i-1], tab[i]
			if hi == lo {
				return float64(i) / last
			}
			return (float64(i-1) + (y-lo)/(hi-lo)) / last
		}
		i := sort.Search(n, func(i int) bool { return tab[i] <= y })
		switch {
		case i <= 0:
			return 0
		case i >= n:
			return 1
		}
		lo, hi := tab[i-1], tab[i]
		if hi == lo {
			return float64(i) / last
		}
		return (float64(i-1) + (y-lo)/(hi-lo)) / last
	}
	identity := true
	for i, v := range tab {
		if math.Abs(v-float64(i)/last) > 1e-4 {
			identity = false
			break
		}
	}
	return &Curve{eval: eval, inv: inv, identity: identity}
}

func paraCurve(kind int, p []float64) *Curve {
	var g, a, b, c, d, e, f float64
	g = p[0]
	switch kind {
	case 1:
		a, b = p[1], p[2]
	case 2:
		a, b, c = p[1], p[2], p[3]
	case 3:
		a, b, c, d = p[1], p[2], p[3], p[4]
	case 4:
		a, b, c, d, e, f = p[1], p[2], p[3], p[4], p[5], p[6]
	}
	pw := func(x float64) float64 {
		if x <= 0 {
			return 0
		}
		return math.Pow(x, g)
	}
	var eval func(float64) float64
	switch kind {
	case 0:
		eval = func(x float64) float64 { return pw(x) }
	case 1:
		eval = func(x float64) float64 {
			if a != 0 && x >= -b/a {
				return pw(a*x + b)
			}
			return 0
		}
	case 2:
		eval = func(x float64) float64 {
			if a != 0 && x >= -b/a {
				return pw(a*x+b) + c
			}
			return c
		}
	case 3:
		eval = func(x float64) float64 {
			if x >= d {
				return pw(a*x + b)
			}
			return c * x
		}
	default:
		eval = func(x float64) float64 {
			if x >= d {
				return pw(a*x+b) + e
			}
			return c*x + f
		}
	}
	if g <= 0 || math.IsNaN(g) || math.IsInf(g, 0) {
		return identityCurve()
	}
	cv := &Curve{eval: eval}
	closed := a != 0
	if kind >= 3 && c == 0 {
		closed = false
	}
	if closed {
		root := func(y float64) float64 {
			if y <= 0 {
				return 0
			}
			return math.Pow(y, 1/g)
		}
		switch kind {
		case 0:
			cv.inv = func(y float64) float64 { return root(y) }
		case 1:
			cv.inv = func(y float64) float64 { return (root(y) - b) / a }
		case 2:
			cv.inv = func(y float64) float64 {
				if y <= c {
					return -b / a
				}
				return (root(y-c) - b) / a
			}
		case 3:
			yd := pw(a*d + b)
			cv.inv = func(y float64) float64 {
				if y >= yd {
					return (root(y) - b) / a
				}
				return y / c
			}
		default:
			yd := pw(a*d+b) + e
			cv.inv = func(y float64) float64 {
				if y >= yd {
					return (root(y-e) - b) / a
				}
				return (y - f) / c
			}
		}
	}
	if kind == 0 {
		cv.gamma = g
	}
	if cv.inv == nil {
		const n = 4096
		t := make([]float64, n+1)
		for i := range t {
			t[i] = eval(float64(i) / n)
		}
		cv.inv = tableCurve(t).inv
	}
	if math.IsNaN(cv.Eval(0.5)) {
		return identityCurve()
	}
	return cv
}

func srgbEncode(l float64) float64 {
	if l <= 0.0031308 {
		return 12.92 * l
	}
	return 1.055*math.Pow(l, 1/2.4) - 0.055
}

func srgbDecode(v float64) float64 {
	if v <= 0.04045 {
		return v / 12.92
	}
	return math.Pow((v+0.055)/1.055, 2.4)
}

func srgbCurve() *Curve {
	return funcCurve(srgbDecode, srgbEncode)
}

const (
	bt709Alpha = 1.09929682680944
	bt709Beta  = 0.018053968510807
)

func bt709Decode(v float64) float64 {
	if v < 4.5*bt709Beta {
		return v / 4.5
	}
	return math.Pow((v+bt709Alpha-1)/bt709Alpha, 1/0.45)
}

func bt709Encode(l float64) float64 {
	if l < bt709Beta {
		return 4.5 * l
	}
	return bt709Alpha*math.Pow(l, 0.45) - (bt709Alpha - 1)
}

const (
	pqM1 = 2610.0 / 16384
	pqM2 = 2523.0 / 4096 * 128
	pqC1 = 3424.0 / 4096
	pqC2 = 2413.0 / 4096 * 32
	pqC3 = 2392.0 / 4096 * 32

	pqPeakNits = 10000.0
)

func pqEOTF(e float64) float64 {
	if e <= 0 {
		return 0
	}
	if e > 1 {
		e = 1
	}
	p := math.Pow(e, 1/pqM2)
	num := math.Max(p-pqC1, 0)
	den := pqC2 - pqC3*p
	return math.Pow(num/den, 1/pqM1)
}

func pqInverse(y float64) float64 {
	if y <= 0 {
		return 0
	}
	if y > 1 {
		y = 1
	}
	p := math.Pow(y, pqM1)
	return math.Pow((pqC1+pqC2*p)/(1+pqC3*p), pqM2)
}

const (
	hlgA = 0.17883277
	hlgB = 0.28466892
	hlgC = 0.55991073
)

func hlgOETFInverse(e float64) float64 {
	if e <= 0 {
		return 0
	}
	if e <= 0.5 {
		return e * e / 3
	}
	return (math.Exp((e-hlgC)/hlgA) + hlgB) / 12
}

func hlgOETF(s float64) float64 {
	if s <= 0 {
		return 0
	}
	if s <= 1.0/12 {
		return math.Sqrt(3 * s)
	}
	return hlgA*math.Log(12*s-hlgB) + hlgC
}
