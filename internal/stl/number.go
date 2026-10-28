package stl

import (
	"errors"
	"math"
	"strconv"
)

var pow10Table = [...]float64{
	1e0, 1e1, 1e2, 1e3, 1e4, 1e5, 1e6, 1e7, 1e8, 1e9, 1e10, 1e11,
	1e12, 1e13, 1e14, 1e15, 1e16, 1e17, 1e18, 1e19, 1e20, 1e21, 1e22,
}

const (
	maxExactMantissa  = 1 << 53
	maxExactExponent  = 22
	halfwayLowBits    = 1<<29 - 1
	halfwayPattern    = 1 << 28
	smallestNormal32  = 1.1754943508222875e-38
	largestFinite32   = 3.4028234663852886e38
	maxMantissaDigits = 19
	maxExponentDigits = 5
)

func parseFloat32(b []byte) float32 {
	if v, ok := fastFloat32(b); ok {
		return v
	}
	f, err := strconv.ParseFloat(string(b), 32)
	if err != nil && !errors.Is(err, strconv.ErrRange) {
		return float32(math.NaN())
	}
	return float32(f)
}

func fastFloat32(b []byte) (float32, bool) {
	n := len(b)
	i := 0
	neg := false
	if i < n && (b[i] == '-' || b[i] == '+') {
		neg = b[i] == '-'
		i++
	}
	var mant uint64
	digits := 0
	exp := 0
	seen := false
	for ; i < n; i++ {
		c := b[i] - '0'
		if c > 9 {
			break
		}
		seen = true
		if digits >= maxMantissaDigits {
			return 0, false
		}
		mant = mant*10 + uint64(c)
		if mant != 0 {
			digits++
		}
	}
	if i < n && b[i] == '.' {
		i++
		for ; i < n; i++ {
			c := b[i] - '0'
			if c > 9 {
				break
			}
			seen = true
			if digits >= maxMantissaDigits {
				return 0, false
			}
			mant = mant*10 + uint64(c)
			if mant != 0 {
				digits++
			}
			exp--
		}
	}
	if !seen {
		return 0, false
	}
	if i < n && (b[i] == 'e' || b[i] == 'E') {
		i++
		eneg := false
		if i < n && (b[i] == '-' || b[i] == '+') {
			eneg = b[i] == '-'
			i++
		}
		e := 0
		count := 0
		for ; i < n; i++ {
			c := b[i] - '0'
			if c > 9 {
				break
			}
			if count >= maxExponentDigits {
				return 0, false
			}
			e = e*10 + int(c)
			count++
		}
		if count == 0 {
			return 0, false
		}
		if eneg {
			e = -e
		}
		exp += e
	}
	if i != n {
		return 0, false
	}
	if mant == 0 {
		if neg {
			return float32(math.Copysign(0, -1)), true
		}
		return 0, true
	}
	if mant >= maxExactMantissa {
		return 0, false
	}
	f := float64(mant)
	switch {
	case exp == 0:
	case exp > 0 && exp <= maxExactExponent:
		f *= pow10Table[exp]
	case exp < 0 && exp >= -maxExactExponent:
		f /= pow10Table[-exp]
	default:
		return 0, false
	}
	if f < smallestNormal32 || f > largestFinite32 {
		return 0, false
	}
	if math.Float64bits(f)&halfwayLowBits == halfwayPattern {
		return 0, false
	}
	if neg {
		f = -f
	}
	return float32(f), true
}
