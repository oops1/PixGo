package codec

type Orientation uint8

const (
	OrientNormal Orientation = 1 + iota
	OrientFlipH
	OrientRotate180
	OrientFlipV
	OrientTranspose
	OrientRotate90
	OrientTransverse
	OrientRotate270
)

type d4 struct {
	rot  int
	flip bool
}

var orientToD4 = [9]d4{
	{0, false},
	{0, false},
	{0, true},
	{2, false},
	{2, true},
	{3, true},
	{1, false},
	{1, true},
	{3, false},
}

func fromD4(g d4) Orientation {
	g.rot = ((g.rot % 4) + 4) % 4
	for o := OrientNormal; o <= OrientRotate270; o++ {
		if orientToD4[o] == g {
			return o
		}
	}
	return OrientNormal
}

func (o Orientation) Valid() bool { return o >= OrientNormal && o <= OrientRotate270 }

func (o Orientation) norm() Orientation {
	if !o.Valid() {
		return OrientNormal
	}
	return o
}

func (o Orientation) Then(next Orientation) Orientation {
	a, b := orientToD4[o.norm()], orientToD4[next.norm()]
	k := a.rot
	if b.flip {
		k = -k
	}
	return fromD4(d4{rot: b.rot + k, flip: a.flip != b.flip})
}

func (o Orientation) RotateCW() Orientation  { return o.Then(OrientRotate90) }
func (o Orientation) RotateCCW() Orientation { return o.Then(OrientRotate270) }
func (o Orientation) Rotate180() Orientation { return o.Then(OrientRotate180) }
func (o Orientation) FlipH() Orientation     { return o.Then(OrientFlipH) }
func (o Orientation) FlipV() Orientation     { return o.Then(OrientFlipV) }

func (o Orientation) Transposes() bool {
	return orientToD4[o.norm()].rot%2 == 1
}

func (o Orientation) Size(w, h int) (int, int) {
	if o.Transposes() {
		return h, w
	}
	return w, h
}

func (o Orientation) ToSource(u, v, w, h float64) (x, y float64) {
	switch o.norm() {
	case OrientFlipH:
		return w - u, v
	case OrientRotate180:
		return w - u, h - v
	case OrientFlipV:
		return u, h - v
	case OrientTranspose:
		return v, u
	case OrientRotate90:
		return v, h - u
	case OrientTransverse:
		return w - v, h - u
	case OrientRotate270:
		return w - v, u
	}
	return u, v
}

func (o Orientation) Matrix(w, h float64) (a, b, c, d, e, f float64) {
	c, f = o.ToSource(0, 0, w, h)
	x1, y1 := o.ToSource(1, 0, w, h)
	x2, y2 := o.ToSource(0, 1, w, h)
	return x1 - c, x2 - c, c, y1 - f, y2 - f, f
}
