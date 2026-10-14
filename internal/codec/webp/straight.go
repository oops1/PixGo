package webp

import (
	"image"
	"sync"

	"github.com/oops1/PixGo/internal/codec/internal/convert"
)

const (
	premultiplyScale = 32897
	premultiplyShift = 23
	alphaOpaque      = 0xff
)

var (
	straightTable     [256 * 256]uint8
	straightTableOnce sync.Once
)

func premultiplied(value, alpha uint32) uint32 {
	return value * (alpha * premultiplyScale) >> premultiplyShift
}

func buildStraightTable() {
	for alpha := uint32(1); alpha < alphaOpaque; alpha++ {
		var low, high [256]int
		for v := range low {
			low[v], high[v] = -1, -1
		}
		for value := uint32(0); value < 256; value++ {
			p := premultiplied(value, alpha)
			if low[p] < 0 {
				low[p] = int(value)
			}
			high[p] = int(value)
		}
		for p := range low {
			estimate := min(255, (p*255+int(alpha)/2)/int(alpha))
			switch {
			case low[p] == 0:
				estimate = 0
			case high[p] == 255:
				estimate = 255
			case low[p] > 0:
				estimate = (low[p] + high[p] + 1) / 2
			}
			straightTable[alpha<<8|uint32(p)] = uint8(estimate)
		}
	}
}

func straightAlpha(src *image.RGBA) *image.NRGBA {
	dst := &image.NRGBA{Pix: src.Pix, Stride: src.Stride, Rect: src.Rect}
	width, height := dst.Rect.Dx(), dst.Rect.Dy()
	if width <= 0 || height <= 0 {
		return dst
	}
	straightTableOnce.Do(buildStraightTable)
	convert.Bands(width, height, func(y0, y1 int) {
		for row := y0; row < y1; row++ {
			start := dst.PixOffset(dst.Rect.Min.X, dst.Rect.Min.Y+row)
			line := dst.Pix[start : start+width*4]
			for o := 0; o < len(line); o += 4 {
				alpha := line[o+3]
				switch alpha {
				case alphaOpaque:
				case 0:
					line[o], line[o+1], line[o+2] = 0, 0, 0
				default:
					base := int(alpha) << 8
					line[o] = straightTable[base|int(line[o])]
					line[o+1] = straightTable[base|int(line[o+1])]
					line[o+2] = straightTable[base|int(line[o+2])]
				}
			}
		}
	})
	return dst
}
