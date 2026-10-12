package gallery

import (
	"github.com/oops1/PixGo/internal/thumbs"
	"github.com/oops1/PixGo/internal/viewer"
)

const DefaultThumbSize = int(thumbs.Size160)

type ViewState struct {
	Order     Order
	Format    string
	ThumbSize int
}

func (v ViewState) Normalized() ViewState {
	if v.Order.Mode > viewer.SortSize {
		v.Order.Mode = viewer.SortName
	}
	v.Format = canonicalFormat(v.Format)
	v.ThumbSize = NormalizeThumbSize(v.ThumbSize)
	return v
}

func NormalizeThumbSize(px int) int {
	sizes := thumbs.Sizes()
	if px <= 0 {
		return DefaultThumbSize
	}
	best := sizes[0]
	for _, s := range sizes[1:] {
		if abs(int(s)-px) <= abs(int(best)-px) {
			best = s
		}
	}
	return int(best)
}

func StepThumbSize(px, steps int) int {
	sizes := thumbs.Sizes()
	current := NormalizeThumbSize(px)
	index := 0
	for i, s := range sizes {
		if int(s) == current {
			index = i
		}
	}
	return int(sizes[min(max(index+steps, 0), len(sizes)-1)])
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
