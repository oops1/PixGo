package webp

import (
	"errors"
	"fmt"
	"image"
	"io"

	vpx "github.com/gen2brain/vpx/webp"

	"github.com/oops1/PixGo/internal/codec"
)

const (
	DefaultQuality = 80
	MinQuality     = 1
	MaxQuality     = 100
	MaxDimension   = 1<<14 - 1
	encodeMethod   = 4
	losslessEffort = 75
)

type Encoder struct{}

func (Encoder) ID() codec.ID { return codec.WebP }

func (Encoder) Name() string { return "WebP" }

func (Encoder) Extension() string { return ".webp" }

func ClampQuality(quality int) int {
	switch {
	case quality == 0:
		return DefaultQuality
	case quality < MinQuality:
		return MinQuality
	case quality > MaxQuality:
		return MaxQuality
	}
	return quality
}

func (Encoder) Encode(w io.Writer, img *image.NRGBA, o codec.EncodeOptions) error {
	if img == nil || img.Bounds().Empty() {
		return errors.New("webp: nothing to encode")
	}
	bounds := img.Bounds()
	if bounds.Dx() > MaxDimension || bounds.Dy() > MaxDimension {
		return fmt.Errorf("webp: %dx%d exceeds the format limit of %d pixels per side", bounds.Dx(), bounds.Dy(), MaxDimension)
	}
	quality := ClampQuality(o.Quality)
	if o.Lossless {
		quality = losslessEffort
	}
	return vpx.Encode(w, img, vpx.EncodeOptions{
		Quality:  quality,
		Lossless: o.Lossless,
		Method:   encodeMethod,
		Exact:    o.Lossless,
	})
}
