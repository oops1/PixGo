package codec

import "testing"

func apply(o Orientation, w, h int, src [][]int) [][]int {
	dw, dh := o.Size(w, h)
	out := make([][]int, dh)
	for v := 0; v < dh; v++ {
		out[v] = make([]int, dw)
		for u := 0; u < dw; u++ {
			x, y := o.ToSource(float64(u)+0.5, float64(v)+0.5, float64(w), float64(h))
			out[v][u] = src[int(y)][int(x)]
		}
	}
	return out
}

func grid(w, h int) [][]int {
	g := make([][]int, h)
	for y := range g {
		g[y] = make([]int, w)
		for x := range g[y] {
			g[y][x] = y*w + x
		}
	}
	return g
}

func equal(a, b [][]int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if len(a[i]) != len(b[i]) {
			return false
		}
		for j := range a[i] {
			if a[i][j] != b[i][j] {
				return false
			}
		}
	}
	return true
}

func TestExifMeaning(t *testing.T) {
	src := grid(3, 2)
	want := map[Orientation][][]int{
		OrientNormal:     {{0, 1, 2}, {3, 4, 5}},
		OrientFlipH:      {{2, 1, 0}, {5, 4, 3}},
		OrientRotate180:  {{5, 4, 3}, {2, 1, 0}},
		OrientFlipV:      {{3, 4, 5}, {0, 1, 2}},
		OrientTranspose:  {{0, 3}, {1, 4}, {2, 5}},
		OrientRotate90:   {{3, 0}, {4, 1}, {5, 2}},
		OrientTransverse: {{5, 2}, {4, 1}, {3, 0}},
		OrientRotate270:  {{2, 5}, {1, 4}, {0, 3}},
	}
	for o, w := range want {
		if got := apply(o, 3, 2, src); !equal(got, w) {
			t.Errorf("orientation %d: got %v want %v", o, got, w)
		}
	}
}

func TestThenMatchesSequentialApplication(t *testing.T) {
	src := grid(3, 2)
	for a := OrientNormal; a <= OrientRotate270; a++ {
		for b := OrientNormal; b <= OrientRotate270; b++ {
			step := apply(a, 3, 2, src)
			w, h := a.Size(3, 2)
			want := apply(b, w, h, step)
			got := apply(a.Then(b), 3, 2, src)
			if !equal(got, want) {
				t.Errorf("%d then %d = %d: got %v want %v", a, b, a.Then(b), got, want)
			}
		}
	}
}

func TestRotationsCycle(t *testing.T) {
	for o := OrientNormal; o <= OrientRotate270; o++ {
		if got := o.RotateCW().RotateCW().RotateCW().RotateCW(); got != o {
			t.Errorf("four CW rotations of %d gave %d", o, got)
		}
		if got := o.RotateCW().RotateCCW(); got != o {
			t.Errorf("CW then CCW of %d gave %d", o, got)
		}
		if got := o.FlipH().FlipH(); got != o {
			t.Errorf("double flip of %d gave %d", o, got)
		}
	}
}
