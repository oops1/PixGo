package jpeg

import (
	"context"
	"errors"
	"image"
	"image/color"
	stdjpeg "image/jpeg"
	"io"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/convert"
	"github.com/oops1/PixGo/internal/codec/internal/guard"
	"github.com/oops1/PixGo/internal/meta/exif"
)

const (
	DefaultQuality = 90
	MinQuality     = 1
	MaxQuality     = 100
)

type Decoder struct{}

func (Decoder) ID() codec.ID { return codec.JPEG }

func (Decoder) Name() string { return "JPEG" }

func (Decoder) Extensions() []string {
	return []string{".jpg", ".jpeg", ".jpe", ".jfif"}
}

func (Decoder) Sniff(head []byte) bool {
	return len(head) >= 3 && head[0] == 0xff && head[1] == 0xd8 && head[2] == 0xff
}

func isUnsupported(err error) bool {
	var unsupported stdjpeg.UnsupportedError
	return errors.As(err, &unsupported)
}

func bitsPerPixel(model color.Model) int {
	switch model {
	case color.GrayModel:
		return 8
	case color.CMYKModel:
		return 32
	}
	return 24
}

func orientationOf(r io.ReaderAt, size int64) codec.Orientation {
	payload, err := exif.FromJPEG(r, size)
	if err != nil {
		return codec.OrientNormal
	}
	if orientation, ok := exif.Orientation(payload); ok {
		return orientation
	}
	return codec.OrientNormal
}

func (Decoder) Probe(ctx context.Context, r io.ReaderAt, size int64, lim codec.Limits) (codec.Info, error) {
	lim = guard.Limits(lim)
	if err := guard.CheckSize(size, lim); err != nil {
		return codec.Info{}, err
	}
	if err := ctx.Err(); err != nil {
		return codec.Info{}, err
	}
	config, err := stdjpeg.DecodeConfig(guard.Stream(ctx, r, size))
	if err != nil {
		return codec.Info{}, guard.Classify(ctx, err, isUnsupported)
	}
	if err := lim.Check(config.Width, config.Height); err != nil {
		return codec.Info{}, err
	}
	return codec.Info{
		Format:      codec.JPEG,
		FormatName:  "JPEG",
		Width:       config.Width,
		Height:      config.Height,
		Frames:      1,
		Orientation: orientationOf(r, size),
		BitDepth:    bitsPerPixel(config.ColorModel),
	}, nil
}

func (d Decoder) Decode(ctx context.Context, r io.ReaderAt, size int64, o codec.Options) (*codec.Image, error) {
	info, err := d.Probe(ctx, r, size, o.Limits)
	if err != nil {
		return nil, err
	}
	decoded, err := stdjpeg.Decode(guard.Stream(ctx, r, size))
	if err != nil {
		return nil, guard.Classify(ctx, err, isUnsupported)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	pix := convert.NRGBA(decoded)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &codec.Image{Info: info, Frames: []codec.Frame{{Pix: pix}}, Scale: 1}, nil
}

type Encoder struct{}

func (Encoder) ID() codec.ID { return codec.JPEG }

func (Encoder) Name() string { return "JPEG" }

func (Encoder) Extension() string { return ".jpg" }

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
		return errors.New("jpeg: nothing to encode")
	}
	flattened := convert.OverBackground(img, color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff})
	return stdjpeg.Encode(w, flattened, &stdjpeg.Options{Quality: ClampQuality(o.Quality)})
}
