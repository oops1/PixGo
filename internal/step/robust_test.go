package step

import (
	"context"
	"math/rand"
	"regexp"
	"strconv"
	"testing"
	"time"
)

var numRE = regexp.MustCompile(`[-+]?[0-9]+\.[0-9]*(E[-+]?[0-9]+)?`)
var refRE = regexp.MustCompile(`#[0-9]+`)

func TestRandomMutations(t *testing.T) {
	seeds := fuzzSeeds()
	rng := rand.New(rand.NewSource(7))
	vals := []string{"0.", "-0.", "1.E-300", "1.E+300", "-1.", "1.E-12", "1.E+12", "3.14159265358979", "1.5707963267949", "100000.", "-7.5", "2."}
	n := 40
	if testing.Short() {
		n = 20
	}
	before := facePanics.Load()
	worst := time.Duration(0)
	for si, s := range seeds {
		for k := 0; k < n; k++ {
			mut := numRE.ReplaceAllStringFunc(s, func(x string) string {
				if rng.Intn(8) != 0 {
					return x
				}
				if rng.Intn(2) == 0 {
					return vals[rng.Intn(len(vals))]
				}
				f, _ := strconv.ParseFloat(x, 64)
				return rs(f * (1 + (rng.Float64()-0.5)*0.2))
			})
			if k%3 == 0 {
				mut = refRE.ReplaceAllStringFunc(mut, func(x string) string {
					if rng.Intn(30) != 0 {
						return x
					}
					return "#" + strconv.Itoa(1+rng.Intn(300))
				})
			}
			t0 := time.Now()
			Read(context.Background(), rd{mut}, int64(len(mut)), Options{MaxTriangles: 2000000})
			Probe(context.Background(), rd{mut}, int64(len(mut)), Options{})
			if d := time.Since(t0); d > worst {
				worst = d
				if d > 2*time.Second {
					t.Errorf("seed %d variant %d took %v", si, k, d)
				}
			}
		}
	}
	t.Logf("worst %v", worst)
	if facePanics.Load() != before {
		t.Fatalf("%d recovered panics", facePanics.Load()-before)
	}
}
