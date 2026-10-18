package assemble

import (
	"context"
	"image"
	"time"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/convert"
	"github.com/oops1/PixGo/internal/codec/internal/guard"
	"github.com/oops1/PixGo/internal/meta"
)

const maxFrameDelay = time.Hour

type CropFunc func(width, height int) (image.Rectangle, bool)

type Source struct {
	Images []image.Image
	Delays []float64
}

func Delay(seconds float64) time.Duration {
	if !(seconds > 0) {
		return 0
	}
	return time.Duration(min(seconds*float64(time.Second), float64(maxFrameDelay)))
}

func Crop(src *image.NRGBA, rect image.Rectangle) *image.NRGBA {
	if rect == src.Bounds() {
		return src
	}
	dst := image.NewNRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
	rowBytes := rect.Dx() * 4
	for row := 0; row < rect.Dy(); row++ {
		from := src.PixOffset(rect.Min.X, rect.Min.Y+row)
		copy(dst.Pix[row*dst.Stride:row*dst.Stride+rowBytes], src.Pix[from:from+rowBytes])
	}
	return dst
}

func Frames(ctx context.Context, format string, src Source, crop CropFunc) ([]codec.Frame, error) {
	if len(src.Images) == 0 {
		return nil, guard.Corrupt("%s: no frames", format)
	}
	frames := make([]codec.Frame, 0, len(src.Images))
	for i, img := range src.Images {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		pix := convert.NRGBA(img)
		if crop != nil {
			rect, ok := crop(pix.Bounds().Dx(), pix.Bounds().Dy())
			if !ok {
				return nil, guard.Corrupt("%s: clean aperture does not fit a %v frame", format, pix.Bounds())
			}
			pix = Crop(pix, rect)
		}
		if i > 0 && pix.Bounds() != frames[0].Pix.Bounds() {
			return nil, guard.Corrupt("%s: frame %d is %v, the first is %v", format, i, pix.Bounds(), frames[0].Pix.Bounds())
		}
		var delay time.Duration
		if i < len(src.Delays) {
			delay = Delay(src.Delays[i])
		}
		frames = append(frames, codec.Frame{Pix: pix, Delay: delay})
	}
	return frames, nil
}

const unspecifiedCode = 2

func Color(icc []byte, primaries, transfer, matrix uint16, fullRange bool) codec.ColorInfo {
	var cicp *codec.CICP
	if primaries != unspecifiedCode || transfer != unspecifiedCode {
		if primaries <= 255 && transfer <= 255 && matrix <= 255 {
			cicp = &codec.CICP{Primaries: uint8(primaries), Transfer: uint8(transfer), Matrix: uint8(matrix), FullRange: fullRange}
		}
	}
	return meta.NewColor(icc, cicp)
}

func CropDeep(src *image.NRGBA64, rect image.Rectangle) *image.NRGBA64 {
	if rect == src.Bounds() {
		return src
	}
	dst := image.NewNRGBA64(image.Rect(0, 0, rect.Dx(), rect.Dy()))
	rowBytes := rect.Dx() * 8
	for row := 0; row < rect.Dy(); row++ {
		from := src.PixOffset(rect.Min.X, rect.Min.Y+row)
		copy(dst.Pix[row*dst.Stride:row*dst.Stride+rowBytes], src.Pix[from:from+rowBytes])
	}
	return dst
}

func WantsDeep(src Source, color codec.ColorInfo) bool {
	if len(src.Images) == 0 {
		return false
	}
	switch src.Images[0].(type) {
	case *image.NRGBA64, *image.Gray16:
		return true
	}
	return color.HDR()
}

func DeepFrames(ctx context.Context, format string, src Source, crop CropFunc) ([]codec.Frame, error) {
	if len(src.Images) == 0 {
		return nil, guard.Corrupt("%s: no frames", format)
	}
	frames := make([]codec.Frame, 0, len(src.Images))
	for i, img := range src.Images {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		pix := convert.Deep(img)
		src.Images[i] = nil
		if crop != nil {
			rect, ok := crop(pix.Bounds().Dx(), pix.Bounds().Dy())
			if !ok {
				return nil, guard.Corrupt("%s: clean aperture does not fit a %v frame", format, pix.Bounds())
			}
			pix = CropDeep(pix, rect)
		}
		if i > 0 && pix.Bounds() != frames[0].Deep.Bounds() {
			return nil, guard.Corrupt("%s: frame %d is %v, the first is %v", format, i, pix.Bounds(), frames[0].Deep.Bounds())
		}
		var delay time.Duration
		if i < len(src.Delays) {
			delay = Delay(src.Delays[i])
		}
		frames = append(frames, codec.Frame{Deep: pix, Delay: delay})
	}
	return frames, nil
}
