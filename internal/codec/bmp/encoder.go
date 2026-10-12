package bmp

import (
	"errors"
	"image"
	"io"

	xbmp "golang.org/x/image/bmp"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/convert"
)

const maxEncodedBytes = 1<<31 - 1

type Encoder struct{}

func (Encoder) ID() codec.ID { return codec.BMP }

func (Encoder) Name() string { return "BMP" }

func (Encoder) Extension() string { return ".bmp" }

func (Encoder) Encode(w io.Writer, img *image.NRGBA, o codec.EncodeOptions) error {
	if img == nil || img.Bounds().Empty() {
		return errors.New("bmp: nothing to encode")
	}
	bounds := img.Bounds()
	if int64(bounds.Dx())*int64(bounds.Dy())*4 > maxEncodedBytes {
		return errors.New("bmp: image too large for the format")
	}
	return xbmp.Encode(w, convert.NRGBA(img))
}
