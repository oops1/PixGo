package imaging

import (
	"github.com/oops1/PixGo/internal/codec"
)

const (
	nrgbaBytesPerPixel = 4
	maxEstimateFrames  = 1 << 12
	defaultScratchBpp  = 4
)

var scratchBytesPerPixel = map[codec.ID]int64{
	codec.JPEG: 2,
	codec.PNG:  1,
	codec.BMP:  1,
	codec.WebP: 12,
	codec.AVIF: 12,
	codec.HEIC: 12,
	codec.JXL:  12,
	codec.TIFF: 8,
	codec.RAW:  12,
}

func scratchFor(id codec.ID) int64 {
	if n, ok := scratchBytesPerPixel[id]; ok {
		return n
	}
	return defaultScratchBpp
}

func pyramidBytesFor(base int64) int64 { return base / 3 }

func overviewBytes(w, h int) int64 {
	shift := overviewShift(w, h, 0)
	lw, lh := codec.LevelSize(w, h, shift)
	base := int64(lw) * int64(lh) * nrgbaBytesPerPixel
	return base + pyramidBytesFor(base)
}

func (l *Loader) tiledEstimate(info codec.Info) int64 {
	return overviewBytes(info.Width, info.Height) + l.tileBudget.Load() + windowReserveBytes
}

func directEstimate(info codec.Info, vw, vh int) int64 {
	px := int64(info.Width) * int64(info.Height)
	if info.Vector && vw > 0 && vh > 0 {
		px = int64(vw) * int64(vh)
	}
	if px < 0 {
		px = 0
	}
	frames := int64(1)
	if info.Animated {
		frames = int64(min(max(info.Frames, 1), maxEstimateFrames))
	}
	base := px * nrgbaBytesPerPixel
	return base*frames + pyramidBytesFor(base) + px*scratchFor(info.Format)
}
