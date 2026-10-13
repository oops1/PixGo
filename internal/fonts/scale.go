package fonts

import (
	"math"
	"sync/atomic"
)

const (
	ReferenceBodyPt = 7.5
	MinTextScale    = 0.8
	MaxTextScale    = 2.5
)

var scaleBits atomic.Uint64

func init() { scaleBits.Store(math.Float64bits(1)) }

func TextScale() float64 { return math.Float64frombits(scaleBits.Load()) }

func SetTextScale(k float64) {
	if math.IsNaN(k) || math.IsInf(k, 0) || k <= 0 {
		k = 1
	}
	scaleBits.Store(math.Float64bits(min(max(k, MinTextScale), MaxTextScale)))
}

func ScaleForBody(pt float64) float64 {
	if math.IsNaN(pt) || math.IsInf(pt, 0) || pt <= 0 {
		return 1
	}
	return min(max(pt/ReferenceBodyPt, MinTextScale), MaxTextScale)
}
