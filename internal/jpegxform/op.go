package jpegxform

import "github.com/oops1/PixGo/internal/codec"

type Op uint8

const (
	Identity Op = iota
	FlipH
	FlipV
	Rotate90
	Rotate180
	Rotate270
	Transpose
	Transverse
	opCount
)

var opNames = [opCount]string{"identity", "flip-h", "flip-v", "rotate-90", "rotate-180", "rotate-270", "transpose", "transverse"}

var opOfOrientation = [9]Op{Identity, Identity, FlipH, Rotate180, FlipV, Transpose, Rotate90, Transverse, Rotate270}

var orientationOfOp = [opCount]codec.Orientation{
	codec.OrientNormal, codec.OrientFlipH, codec.OrientFlipV, codec.OrientRotate90,
	codec.OrientRotate180, codec.OrientRotate270, codec.OrientTranspose, codec.OrientTransverse,
}

func FromOrientation(o codec.Orientation) Op {
	if !o.Valid() {
		return Identity
	}
	return opOfOrientation[o]
}

func (o Op) Valid() bool { return o < opCount }

func (o Op) Orientation() codec.Orientation {
	if !o.Valid() {
		return codec.OrientNormal
	}
	return orientationOfOp[o]
}

func (o Op) String() string {
	if !o.Valid() {
		return "invalid"
	}
	return opNames[o]
}

func (o Op) Transposes() bool {
	switch o {
	case Rotate90, Rotate270, Transpose, Transverse:
		return true
	}
	return false
}

func (o Op) edgeNeeds() (width, height bool) {
	switch o {
	case FlipH, Rotate270:
		return true, false
	case FlipV, Rotate90:
		return false, true
	case Rotate180, Transverse:
		return true, true
	}
	return false, false
}
