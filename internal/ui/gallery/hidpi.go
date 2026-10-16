package gallery

import (
	"image"
	"math"

	"github.com/oops1/PixGo/internal/thumbs"
)

func thumbSideFor(side int, size image.Point, k float64) int {
	if k <= 1 {
		return side
	}
	return thumbs.Nearest(max(size.X, size.Y)).Px()
}

func (p *Page) prefetchSide() int {
	if p.k <= 1 {
		return p.snap.side
	}
	return thumbs.Nearest(int(math.Ceil(float64(p.tile.photoW) * p.k))).Px()
}
