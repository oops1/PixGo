package rawdev

import (
	"errors"
	"math"
	"testing"
)

func TestTemperatureOfKnownIlluminants(t *testing.T) {
	cases := []struct {
		name string
		xy   [2]float64
		want float64
		tol  float64
	}{
		{"D65", [2]float64{0.31272, 0.32903}, 6504, 60},
		{"D50", [2]float64{0.34567, 0.35850}, 5003, 50},
		{"A", [2]float64{0.44757, 0.40745}, 2856, 40},
		{"D55", [2]float64{0.33242, 0.34743}, 5503, 60},
		{"D75", [2]float64{0.29902, 0.31485}, 7504, 90},
	}
	for _, c := range cases {
		if got := Temperature(c.xy); math.Abs(got-c.want) > c.tol {
			t.Errorf("%s: %.0f K, want %.0f", c.name, got, c.want)
		}
	}
	for _, xy := range [][2]float64{{0, 0}, {0.1, 0}, {1, 1}, {0.9, 0.05}, {-1, 2}} {
		if v := Temperature(xy); math.IsNaN(v) || v <= 0 {
			t.Errorf("%v: %v", xy, v)
		}
	}
}

func TestMatrixInverseAndAdaptation(t *testing.T) {
	inv, ok := testCamera.Inverse()
	if !ok {
		t.Fatal("singular")
	}
	prod := testCamera.Mul(inv)
	for i := range prod {
		if math.Abs(prod[i]-identity[i]) > 1e-12 {
			t.Fatalf("product %v", prod)
		}
	}
	if _, ok := (Matrix{1, 2, 3, 2, 4, 6, 0, 0, 1}).Inverse(); ok {
		t.Fatal("singular matrix inverted")
	}
	white := xyToXYZ(d65)
	got := adapt(white, whiteD50).Apply(white)
	for i := range got {
		if math.Abs(got[i]-whiteD50[i]) > 1e-9 {
			t.Fatalf("adapted white %v", got)
		}
	}
	m := xyzD50ToSRGB.Apply(whiteD50)
	for _, v := range m {
		if math.Abs(v-1) > 1e-3 {
			t.Fatalf("D50 white in sRGB: %v", m)
		}
	}
}

func TestResolveGivesUnitRowsAndGains(t *testing.T) {
	r, err := cameraColor(testCamera).Resolve()
	if err != nil {
		t.Fatal(err)
	}
	for row := 0; row < 3; row++ {
		if sum := r.Matrix[row*3] + r.Matrix[row*3+1] + r.Matrix[row*3+2]; math.Abs(sum-1) > 1e-9 {
			t.Fatalf("row %d sums to %v", row, sum)
		}
	}
	n := cameraNeutral(testCamera)
	for i := range n {
		if math.Abs(r.Gain[i]*n[i]-math.Max(n[0], math.Max(n[1], n[2]))) > 1e-9 {
			t.Fatalf("gain %v for neutral %v", r.Gain, n)
		}
	}
	min := math.Min(r.Gain[0], math.Min(r.Gain[1], r.Gain[2]))
	if math.Abs(min-1) > 1e-12 {
		t.Fatalf("smallest gain %v", min)
	}
}

func TestResolveRecoversTheCameraRegardlessOfMatrixPairs(t *testing.T) {
	base, _ := cameraColor(testCamera).Resolve()
	n := cameraNeutral(testCamera)
	other := diag([3]float64{1.1, 0.9, 1.3}).Mul(testCamera)
	pair := Color{
		ColorMatrix: [2]Matrix{other, testCamera},
		HasCM:       [2]bool{true, true},
		Illuminant:  [2]uint16{17, 21},
		Analog:      [3]float64{1, 1, 1},
		Neutral:     &n,
	}
	got, err := pair.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	for i := range got.Matrix {
		if math.Abs(got.Matrix[i]-base.Matrix[i]) > 0.02 {
			t.Fatalf("pair matrix %v vs %v", got.Matrix, base.Matrix)
		}
	}
	swapped := pair
	swapped.ColorMatrix = [2]Matrix{testCamera, other}
	swapped.Illuminant = [2]uint16{21, 17}
	again, err := swapped.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if again.Matrix != got.Matrix {
		t.Fatal("the order of the two illuminants changes the result")
	}
}

func TestInterpolationFollowsTheTemperature(t *testing.T) {
	a := Matrix{1, 0, 0, 0, 1, 0, 0, 0, 1}
	b := Matrix{2, 0, 0, 0, 2, 0, 0, 0, 2}
	c := Color{ColorMatrix: [2]Matrix{a, b}, HasCM: [2]bool{true, true}, Illuminant: [2]uint16{17, 21}, Analog: [3]float64{1, 1, 1}}
	w, use := c.weights(2856)
	if !use[0] || !use[1] || math.Abs(w[0]-1) > 1e-9 {
		t.Fatalf("at 2856 K: %v", w)
	}
	w, _ = c.weights(6504)
	if math.Abs(w[1]-1) > 1e-9 {
		t.Fatalf("at 6504 K: %v", w)
	}
	w, _ = c.weights(1500)
	if w[0] != 1 {
		t.Fatalf("below the range: %v", w)
	}
	w, _ = c.weights(20000)
	if w[1] != 1 {
		t.Fatalf("above the range: %v", w)
	}
	mid := 1 / (0.5/2856 + 0.5/6504)
	w, _ = c.weights(mid)
	if math.Abs(w[0]-0.5) > 1e-9 {
		t.Fatalf("halfway in mired: %v", w)
	}
}

func TestForwardMatrixAgreesWithTheColorMatrixPath(t *testing.T) {
	n := cameraNeutral(testCamera)
	inv, _ := testCamera.Inverse()
	forward := adapt(xyToXYZ(d65), whiteD50).Mul(inv).Mul(diag(n))
	c := cameraColor(testCamera)
	c.Forward = [2]Matrix{forward}
	c.HasFM = [2]bool{true}
	withFM, err := c.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	without, _ := cameraColor(testCamera).Resolve()
	for i := range withFM.Matrix {
		if math.Abs(withFM.Matrix[i]-without.Matrix[i]) > 5e-4 {
			t.Fatalf("%v vs %v", withFM.Matrix, without.Matrix)
		}
	}
}

func TestWhitePointFromXYAndDefault(t *testing.T) {
	c := cameraColor(testCamera)
	c.Neutral = nil
	xy := d65
	c.WhiteXY = &xy
	a, err := c.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := cameraColor(testCamera).Resolve()
	for i := range a.Matrix {
		if math.Abs(a.Matrix[i]-b.Matrix[i]) > 1e-3 {
			t.Fatalf("%v vs %v", a.Matrix, b.Matrix)
		}
	}
	c.WhiteXY = nil
	if _, err := c.Resolve(); err != nil {
		t.Fatalf("no white point at all: %v", err)
	}
}

func TestResolveRejectsBrokenInput(t *testing.T) {
	good := cameraColor(testCamera)
	cases := map[string]func(*Color){
		"no matrix":     func(c *Color) { c.HasCM = [2]bool{} },
		"singular":      func(c *Color) { c.ColorMatrix[0] = Matrix{1, 1, 1, 1, 1, 1, 1, 1, 1} },
		"NaN":           func(c *Color) { c.ColorMatrix[0][3] = math.NaN() },
		"Inf":           func(c *Color) { c.ColorMatrix[0][3] = math.Inf(1) },
		"zero neutral":  func(c *Color) { z := [3]float64{0.5, 0, 0.5}; c.Neutral = &z },
		"neg neutral":   func(c *Color) { z := [3]float64{0.5, -1, 0.5}; c.Neutral = &z },
		"analog":        func(c *Color) { c.Analog = [3]float64{1, 0, 1} },
		"bad white":     func(c *Color) { c.Neutral = nil; w := [2]float64{0.3, 0}; c.WhiteXY = &w },
		"NaN forward":   func(c *Color) { c.HasFM[0] = true; c.Forward[0][0] = math.NaN() },
		"huge neutral":  func(c *Color) { z := [3]float64{math.Inf(1), 1, 1}; c.Neutral = &z },
		"NaN neutral":   func(c *Color) { z := [3]float64{math.NaN(), 1, 1}; c.Neutral = &z },
		"zero forward":  func(c *Color) { c.HasFM[0] = true; c.Forward[0] = Matrix{} },
		"neutral far":   func(c *Color) { z := [3]float64{1e-9, 1, 1}; c.Neutral = &z },
		"calibration":   func(c *Color) { c.HasCC[0] = true; c.Calibration[0][1] = math.Inf(-1) },
		"white outside": func(c *Color) { c.Neutral = nil; w := [2]float64{0.9, 0.9}; c.WhiteXY = &w },
	}
	for name, mutate := range cases {
		c := good
		mutate(&c)
		_, err := c.Resolve()
		if err == nil {
			t.Errorf("%s accepted", name)
			continue
		}
		if !errors.Is(err, ErrCorrupt) && !errors.Is(err, ErrUnsupported) {
			t.Errorf("%s: %v", name, err)
		}
	}
}
