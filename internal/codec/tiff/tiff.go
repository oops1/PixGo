package tiff

import (
	"context"
	"errors"
	"io"

	xtiff "golang.org/x/image/tiff"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/convert"
	"github.com/oops1/PixGo/internal/codec/internal/guard"
	"github.com/oops1/PixGo/internal/meta/exif"
)

const photometricRGB = 2

type Decoder struct{}

func (Decoder) ID() codec.ID { return codec.TIFF }

func (Decoder) Name() string { return "TIFF" }

func (Decoder) Extensions() []string { return []string{".tif", ".tiff"} }

func (Decoder) Sniff(head []byte) bool {
	if len(head) < 4 {
		return false
	}
	return (head[0] == 'I' && head[1] == 'I' && head[2] == 42 && head[3] == 0) ||
		(head[0] == 'M' && head[1] == 'M' && head[2] == 0 && head[3] == 42)
}

func isUnsupported(err error) bool {
	var unsupported xtiff.UnsupportedError
	return errors.As(err, &unsupported)
}

func describe(r io.ReaderAt, size int64) (bitsPerPixel int, hasAlpha bool) {
	reader, err := exif.NewReaderAt(r, 0, size)
	if err != nil {
		return 0, false
	}
	dir, err := reader.Directory(reader.FirstOffset())
	if err != nil {
		return 0, false
	}
	bitsEntry, ok := dir.Find(exif.TagBitsPerSample)
	if !ok {
		return 1, false
	}
	samples := int(bitsEntry.Count)
	if samples > 16 {
		return 0, false
	}
	for i := 0; i < samples; i++ {
		bits, ok := bitsEntry.Uint(i)
		if !ok {
			return 0, false
		}
		bitsPerPixel += int(bits)
	}
	if photometric, ok := dir.Find(exif.TagPhotometric); ok {
		if value, ok := photometric.Uint(0); ok && value == photometricRGB && samples == 4 {
			hasAlpha = true
		}
	}
	return bitsPerPixel, hasAlpha
}

func (Decoder) Probe(ctx context.Context, r io.ReaderAt, size int64, lim codec.Limits) (codec.Info, error) {
	lim = guard.Limits(lim)
	if err := guard.CheckSize(size, lim); err != nil {
		return codec.Info{}, err
	}
	if err := ctx.Err(); err != nil {
		return codec.Info{}, err
	}
	config, err := xtiff.DecodeConfig(guard.NewReader(ctx, r, size))
	if err != nil {
		return codec.Info{}, guard.Classify(ctx, err, isUnsupported)
	}
	if err := lim.Check(config.Width, config.Height); err != nil {
		return codec.Info{}, err
	}
	orientation := codec.OrientNormal
	if value, ok := exif.OrientationAt(r, 0, size); ok {
		orientation = value
	}
	bitsPerPixel, hasAlpha := describe(r, size)
	return codec.Info{
		Format:      codec.TIFF,
		FormatName:  "TIFF",
		Width:       config.Width,
		Height:      config.Height,
		Frames:      1,
		HasAlpha:    hasAlpha,
		Orientation: orientation,
		BitDepth:    bitsPerPixel,
	}, nil
}

func (d Decoder) Decode(ctx context.Context, r io.ReaderAt, size int64, o codec.Options) (*codec.Image, error) {
	info, err := d.Probe(ctx, r, size, o.Limits)
	if err != nil {
		return nil, err
	}
	decoded, err := xtiff.Decode(guard.NewReader(ctx, r, size))
	if err != nil {
		return nil, guard.Classify(ctx, err, isUnsupported)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	pix := convert.NRGBA(decoded)
	return &codec.Image{Info: info, Frames: []codec.Frame{{Pix: pix}}, Scale: 1}, nil
}
