package jpegdec

import (
	"math"
	"math/rand"
	"testing"
)

func randomCoefficients(rng *rand.Rand, density float64, magnitude int) []int16 {
	coef := make([]int16, blockSize)
	for i := range coef {
		if rng.Float64() < density {
			coef[i] = int16(rng.Intn(2*magnitude+1) - magnitude)
		}
	}
	return coef
}

func randomQuant(rng *rand.Rand, max int) *[blockSize]int32 {
	var q [blockSize]int32
	for i := range q {
		q[i] = int32(1 + rng.Intn(max))
	}
	return &q
}

func referenceBlock8(coef []int16, q *[blockSize]int32) [64]uint8 {
	var b block
	for i := range b {
		b[i] = int32(coef[i]) * q[i]
	}
	referenceIDCT(&b)
	var out [64]uint8
	for i := range out {
		out[i] = uint8(max(0, min(255, b[i]+128)))
	}
	return out
}

func TestFusedIDCTMatchesStandardLibraryAlgorithm(t *testing.T) {
	rng := rand.New(rand.NewSource(11))
	var sc scratch
	cases := []struct {
		density   float64
		magnitude int
		quant     int
	}{{0.05, 40, 8}, {0.3, 120, 16}, {1, 1024, 4}, {0.1, 32767, 255}, {0.02, 3000, 64}}
	for _, tc := range cases {
		for i := 0; i < 3000; i++ {
			coef := randomCoefficients(rng, tc.density, tc.magnitude)
			q := randomQuant(rng, tc.quant)
			want := referenceBlock8(coef, q)
			var got [64]uint8
			sc.block8(coef, 8, q, got[:], 8)
			if got != want {
				t.Fatalf("density %v magnitude %d: block %d differs\n got %v\nwant %v", tc.density, tc.magnitude, i, got, want)
			}
		}
	}
}

func TestDCOnlyBlockMatchesFullTransform(t *testing.T) {
	var sc scratch
	for _, q0 := range []int32{1, 2, 3, 7, 16, 100, 255} {
		var q [blockSize]int32
		for i := range q {
			q[i] = 5
		}
		q[0] = q0
		for dc := -2100; dc <= 2100; dc++ {
			coef := make([]int16, blockSize)
			coef[0] = int16(dc)
			want := referenceBlock8(coef, &q)
			var got [64]uint8
			sc.block8(coef, 8, &q, got[:], 8)
			if got != want {
				t.Fatalf("q0 %d dc %d: got %d want %d", q0, dc, got[0], want[0])
			}
		}
	}
	var q [blockSize]int32
	for i := range q {
		q[i] = 255
	}
	for _, dc := range []int16{32767, -32768, 20000, -20000, 17000} {
		coef := make([]int16, blockSize)
		coef[0] = dc
		want := referenceBlock8(coef, &q)
		var got [64]uint8
		sc.block8(coef, 8, &q, got[:], 8)
		if got != want {
			t.Fatalf("dc %d overflow case differs", dc)
		}
	}
}

func floatReduced(coef []int16, stride int, q *[blockSize]int32, side int) []float64 {
	out := make([]float64, side*side)
	scale := float64(side) / 8
	alpha := func(u int) float64 {
		if u == 0 {
			return math.Sqrt(1 / float64(side))
		}
		return math.Sqrt(2 / float64(side))
	}
	for y := 0; y < side; y++ {
		for x := 0; x < side; x++ {
			sum := 0.0
			for v := 0; v < side; v++ {
				for u := 0; u < side; u++ {
					f := float64(int32(coef[v*stride+u])*q[v*8+u]) * scale
					sum += alpha(u) * alpha(v) * f * math.Cos(float64(2*x+1)*float64(u)*math.Pi/float64(2*side)) * math.Cos(float64(2*y+1)*float64(v)*math.Pi/float64(2*side))
				}
			}
			out[y*side+x] = sum
		}
	}
	return out
}

func TestReducedTransformsMatchFloatingPoint(t *testing.T) {
	rng := rand.New(rand.NewSource(5))
	var sc scratch
	for _, side := range []int{4, 2, 1} {
		fn := blockFor(side, side)
		for i := 0; i < 2000; i++ {
			stride := side
			if i%2 == 1 {
				stride = 8
			}
			q := randomQuant(rng, 20)
			coef := make([]int16, blockSize)
			for k := range coef {
				if rng.Intn(2) == 0 {
					limit := 1800 / int(q[k])
					coef[k] = int16(rng.Intn(2*limit+1) - limit)
				}
			}
			if stride == side {
				packed := make([]int16, side*side)
				for y := 0; y < side; y++ {
					copy(packed[y*side:(y+1)*side], coef[y*8:y*8+side])
				}
				coef = packed
			}
			want := floatReduced(coef, stride, q, side)
			got := make([]uint8, side*side)
			fn(&sc, coef, stride, q, got, side)
			for k := range got {
				expected := math.Max(0, math.Min(255, math.Floor(want[k]+128.5)))
				if math.Abs(float64(got[k])-expected) > 1 {
					t.Fatalf("side %d stride %d block %d pixel %d: got %d want %.2f", side, stride, i, k, got[k], want[k]+128)
				}
			}
		}
	}
}

func TestReducedTransformsPreserveFlatBlocks(t *testing.T) {
	var sc scratch
	var q [blockSize]int32
	for i := range q {
		q[i] = 1
	}
	for _, side := range []int{4, 2, 1} {
		fn := blockFor(side, side)
		for dc := -1000; dc <= 1000; dc += 8 {
			coef := make([]int16, side*side)
			coef[0] = int16(dc)
			got := make([]uint8, side*side)
			fn(&sc, coef, side, &q, got, side)
			want := uint8(max(0, min(255, (dc+4)>>3+128)))
			for k, v := range got {
				if v != want {
					t.Fatalf("side %d dc %d pixel %d: %d, want %d", side, dc, k, v, want)
				}
			}
		}
	}
}
