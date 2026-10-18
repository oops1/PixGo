package jpegxform

import (
	"fmt"
	"image"
	"io"
)

var ErrCrop = fmt.Errorf("jpegxform: the crop rectangle is outside the picture")

type cropGeometry struct {
	outW, outH   int
	cellX, cellY int
	padX, padY   int
}

func (o Op) flipsX() bool {
	switch o {
	case FlipH, Rotate180, Rotate90, Transverse:
		return true
	}
	return false
}

func (o Op) flipsY() bool {
	switch o {
	case FlipV, Rotate180, Rotate270, Transverse:
		return true
	}
	return false
}

func geometryOf(fr *frame, op Op) cropGeometry {
	g := cropGeometry{outW: fr.width, outH: fr.height, cellX: fr.mcuW, cellY: fr.mcuH}
	if op.Transposes() {
		g.outW, g.outH = fr.height, fr.width
		g.cellX, g.cellY = fr.mcuH, fr.mcuW
	}
	if op.flipsX() {
		g.padX = (g.cellX - g.outW%g.cellX) % g.cellX
	}
	if op.flipsY() {
		g.padY = (g.cellY - g.outH%g.cellY) % g.cellY
	}
	return g
}

func (g cropGeometry) padXOrigin() int {
	if g.padX == 0 {
		return 0
	}
	return g.cellX - g.padX
}

func (g cropGeometry) padYOrigin() int {
	if g.padY == 0 {
		return 0
	}
	return g.cellY - g.padY
}

func snapAxis(lo, hi, cell, pad int) (start int, trimmed bool, err error) {
	start = (lo+pad)/cell*cell - pad
	if start < 0 {
		start, trimmed = cell-pad, true
	}
	if start >= hi {
		return 0, false, ErrCrop
	}
	return start, trimmed, nil
}

func (g cropGeometry) snap(want image.Rectangle) (image.Rectangle, bool, error) {
	r := want.Canon().Intersect(image.Rect(0, 0, g.outW, g.outH))
	if r.Empty() {
		return image.Rectangle{}, false, ErrCrop
	}
	x0, tx, err := snapAxis(r.Min.X, r.Max.X, g.cellX, g.padX)
	if err != nil {
		return image.Rectangle{}, false, err
	}
	y0, ty, err := snapAxis(r.Min.Y, r.Max.Y, g.cellY, g.padY)
	if err != nil {
		return image.Rectangle{}, false, err
	}
	return image.Rect(x0, y0, r.Max.X, r.Max.Y), tx || ty, nil
}

func CropFor(r io.ReaderAt, size int64, op Op, want image.Rectangle) (image.Rectangle, error) {
	got, _, err := CropForTrim(r, size, op, want)
	return got, err
}

func CropForTrim(r io.ReaderAt, size int64, op Op, want image.Rectangle) (image.Rectangle, bool, error) {
	if !op.Valid() {
		return image.Rectangle{}, false, fmt.Errorf("jpegxform: invalid operation %d", op)
	}
	h, err := readHeader(newByteSource(r, size, 0), false)
	if err != nil {
		return image.Rectangle{}, false, err
	}
	return geometryOf(&h.fr, op).snap(want)
}
