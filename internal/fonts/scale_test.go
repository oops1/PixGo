package fonts

import (
	"math"
	"testing"
)

func restoreScale(t *testing.T) {
	t.Helper()
	old := TextScale()
	t.Cleanup(func() { SetTextScale(old) })
}

func TestScaleStartsAtOne(t *testing.T) {
	if TextScale() != 1 {
		t.Fatalf("scale = %v", TextScale())
	}
}

func TestScaleForBodyMapsTheSystemSizeOntoTheReferenceBody(t *testing.T) {
	cases := []struct {
		pt   float64
		want float64
	}{
		{9, 1.2},
		{ReferenceBodyPt, 1},
		{10, 10 / ReferenceBodyPt},
		{3, MinTextScale},
		{60, MaxTextScale},
		{0, 1},
		{-4, 1},
		{math.NaN(), 1},
		{math.Inf(1), 1},
	}
	for _, c := range cases {
		if got := ScaleForBody(c.pt); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("ScaleForBody(%v) = %v, want %v", c.pt, got, c.want)
		}
	}
}

func TestSetTextScaleKeepsTheValueInRange(t *testing.T) {
	restoreScale(t)
	for in, want := range map[float64]float64{1.2: 1.2, 0.1: MinTextScale, 9: MaxTextScale, 0: 1, -1: 1, math.NaN(): 1} {
		SetTextScale(in)
		if got := TextScale(); math.Abs(got-want) > 1e-9 {
			t.Errorf("SetTextScale(%v) -> %v, want %v", in, got, want)
		}
	}
}
