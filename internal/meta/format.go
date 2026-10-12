package meta

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

type Rational struct{ Num, Den int64 }

func (r Rational) Valid() bool { return r.Den != 0 }

func (r Rational) Float() float64 {
	if r.Den == 0 {
		return 0
	}
	return float64(r.Num) / float64(r.Den)
}

func trimDecimal(v float64) string {
	return strings.TrimSuffix(strconv.FormatFloat(v, 'f', 1, 64), ".0")
}

func (r Rational) String() string {
	if r.Den == 0 {
		return ""
	}
	num, den := r.Num, r.Den
	if den < 0 {
		num, den = -num, -den
	}
	if num == 0 {
		return "0"
	}
	if num < 0 {
		return "-" + Rational{Num: -num, Den: den}.String()
	}
	if den == 1 {
		return strconv.FormatInt(num, 10)
	}
	if num == 1 {
		return "1/" + strconv.FormatInt(den, 10)
	}
	v := float64(num) / float64(den)
	if v >= 1 {
		return trimDecimal(v)
	}
	n := math.Round(1 / v)
	if n >= 2 && math.Abs(1/n-v) <= 0.15*v {
		return "1/" + strconv.FormatFloat(n, 'f', 0, 64)
	}
	return trimDecimal(v)
}

func (r Rational) FNumberString() string {
	if r.Den == 0 {
		return ""
	}
	return strconv.FormatFloat(r.Float(), 'f', 1, 64)
}

func dmsPart(v float64, positive, negative string) string {
	hemisphere := positive
	if v < 0 {
		hemisphere = negative
		v = -v
	}
	tenths := int64(math.Round(v * 36000))
	degrees := tenths / 36000
	rest := tenths % 36000
	minutes := rest / 600
	seconds := rest % 600
	return fmt.Sprintf("%d°%02d′%02d.%d″ %s", degrees, minutes, seconds/10, seconds%10, hemisphere)
}

func (g *GPS) valid() bool {
	return g != nil &&
		!math.IsNaN(g.Lat) && !math.IsNaN(g.Lon) &&
		math.Abs(g.Lat) <= 90 && math.Abs(g.Lon) <= 180
}

func (g *GPS) DMS() string {
	if !g.valid() {
		return ""
	}
	return dmsPart(g.Lat, "N", "S") + ", " + dmsPart(g.Lon, "E", "W")
}

func (g *GPS) Decimal() string {
	if !g.valid() {
		return ""
	}
	return fmt.Sprintf("%.6f, %.6f", g.Lat, g.Lon)
}

type commonRatio struct{ w, h int }

var commonRatios = [...]commonRatio{
	{1, 1}, {5, 4}, {4, 3}, {3, 2}, {16, 10}, {16, 9}, {2, 1}, {21, 9},
	{4, 5}, {3, 4}, {2, 3}, {10, 16}, {9, 16}, {1, 2}, {9, 21},
}

func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

func AspectRatio(w, h int) string {
	if w <= 0 || h <= 0 {
		return ""
	}
	value := float64(w) / float64(h)
	best, bestErr := -1, math.Inf(1)
	for i, c := range commonRatios {
		target := float64(c.w) / float64(c.h)
		if diff := math.Abs(value-target) / target; diff < bestErr {
			best, bestErr = i, diff
		}
	}
	if best >= 0 && bestErr <= 0.01 {
		return fmt.Sprintf("%d:%d", commonRatios[best].w, commonRatios[best].h)
	}
	g := gcd(w, h)
	return fmt.Sprintf("%d:%d", w/g, h/g)
}
