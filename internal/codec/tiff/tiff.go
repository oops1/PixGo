package tiff

import (
	"context"
	"errors"
	"image"
	"io"

	xtiff "golang.org/x/image/tiff"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/convert"
	"github.com/oops1/PixGo/internal/codec/internal/guard"
	"github.com/oops1/PixGo/internal/meta/exif"
)

type Decoder struct {
	CacheBytes int64
}

func (Decoder) ID() codec.ID { return codec.TIFF }

func (Decoder) Name() string { return "TIFF" }

func (Decoder) Extensions() []string { return []string{".tif", ".tiff"} }

func (Decoder) Sniff(head []byte) bool {
	if len(head) < 4 {
		return false
	}
	little := head[0] == 'I' && head[1] == 'I' && (head[2] == 42 || head[2] == 43) && head[3] == 0
	big := head[0] == 'M' && head[1] == 'M' && head[2] == 0 && (head[3] == 42 || head[3] == 43)
	return little || big
}

func (d Decoder) cacheBytes() int64 {
	switch {
	case d.CacheBytes < 0:
		return 0
	case d.CacheBytes == 0:
		return defaultCacheBytes
	}
	return d.CacheBytes
}

func isUnsupported(err error) bool {
	var unsupported xtiff.UnsupportedError
	return errors.As(err, &unsupported)
}

func inspect(ctx context.Context, r io.ReaderAt, size int64) (*file, *layout, string, error) {
	f, err := readFile(guard.NewReader(ctx, r, size), size)
	if err != nil {
		return nil, nil, "", guard.Classify(ctx, err, nil)
	}
	lay, why, err := classify(f)
	if err != nil {
		return nil, nil, "", guard.Classify(ctx, err, nil)
	}
	return f, lay, why, nil
}

func pixelCount(lay *layout) int64 {
	return int64(lay.width) * int64(lay.height)
}

func (d Decoder) OpenTiled(ctx context.Context, r io.ReaderAt, size int64, lim codec.Limits) (codec.TileReader, error) {
	lim = lim.WithDefaults()
	if err := guard.CheckSize(size, lim); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f, lay, why, err := inspect(ctx, r, size)
	if err != nil {
		return nil, err
	}
	if why != "" {
		return nil, guard.Unsupported("tiff: %s", why)
	}
	rd, err := newReader(f, lay, r, size, lim, d.cacheBytes())
	if err != nil {
		return nil, guard.Classify(ctx, err, nil)
	}
	return rd, nil
}

func (d Decoder) Probe(ctx context.Context, r io.ReaderAt, size int64, lim codec.Limits) (codec.Info, error) {
	lim = lim.WithDefaults()
	if err := guard.CheckSize(size, lim); err != nil {
		return codec.Info{}, err
	}
	if err := ctx.Err(); err != nil {
		return codec.Info{}, err
	}
	_, lay, why, err := inspect(ctx, r, size)
	switch {
	case err == nil:
		pixels := pixelCount(lay)
		if pixels > lim.MaxTiledPixels {
			return codec.Info{}, guard.TooLarge("tiff: %d pixels", pixels)
		}
		if why == "" {
			if pixels > lim.MaxPixels {
				return lay.info, guard.TooLarge("tiff: %d pixels", pixels)
			}
			return lay.info, nil
		}
		if pixels > lim.MaxPixels {
			return codec.Info{}, guard.Unsupported("tiff: %s", why)
		}
	case errors.Is(err, codec.ErrTooLarge), ctx.Err() != nil:
		return codec.Info{}, err
	}
	return legacyProbe(ctx, r, size, lim)
}

func (d Decoder) Decode(ctx context.Context, r io.ReaderAt, size int64, o codec.Options) (*codec.Image, error) {
	lim := o.Limits.WithDefaults()
	if err := guard.CheckSize(size, lim); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f, lay, why, err := inspect(ctx, r, size)
	switch {
	case err == nil:
	case errors.Is(err, codec.ErrTooLarge), ctx.Err() != nil:
		return nil, err
	default:
		return legacyDecode(ctx, r, size, lim, legacyRegion(o.Region, o.Scale))
	}
	pixels := pixelCount(lay)
	if pixels > lim.MaxTiledPixels {
		return nil, guard.TooLarge("tiff: %d pixels", pixels)
	}
	if why != "" {
		if pixels > lim.MaxPixels {
			return nil, guard.Unsupported("tiff: %s", why)
		}
		return legacyDecode(ctx, r, size, lim, legacyRegion(o.Region, o.Scale))
	}
	shift := scaleShift(o.Scale)
	rect, err := outputRect(o.Region, lay.width, lay.height, shift)
	if err != nil {
		return nil, err
	}
	if int64(rect.Dx())*int64(rect.Dy()) > lim.MaxPixels {
		return nil, guard.TooLarge("tiff: %d output pixels", int64(rect.Dx())*int64(rect.Dy()))
	}
	pix, err := d.decodeLevel(ctx, f, lay, r, size, lim, shift, rect)
	if err != nil {
		if ctx.Err() == nil && pixels <= lim.MaxPixels && (errors.Is(err, codec.ErrCorrupt) || errors.Is(err, codec.ErrUnsupported)) {
			return legacyDecode(ctx, r, size, lim, legacyRegion(o.Region, o.Scale))
		}
		return nil, err
	}
	return &codec.Image{Info: lay.info, Frames: []codec.Frame{{Pix: pix}}, Scale: 1 << uint(shift)}, nil
}

func (d Decoder) decodeLevel(ctx context.Context, f *file, lay *layout, r io.ReaderAt, size int64, lim codec.Limits, shift int, rect image.Rectangle) (*image.NRGBA, error) {
	rd, err := newReader(f, lay, r, size, lim, 0)
	if err != nil {
		return nil, guard.Classify(ctx, err, nil)
	}
	defer rd.Close()
	return rd.shielded(ctx, shift, rect, false)
}

func scaleShift(scale int) int {
	switch {
	case scale >= 8:
		return 3
	case scale >= 4:
		return 2
	case scale >= 2:
		return 1
	}
	return 0
}

func outputRect(region *image.Rectangle, width, height, shift int) (image.Rectangle, error) {
	levelW, levelH := codec.LevelSize(width, height, shift)
	level := image.Rect(0, 0, levelW, levelH)
	if region == nil {
		return level, nil
	}
	want := region.Canon().Intersect(level)
	if want.Empty() {
		return image.Rectangle{}, guard.Corrupt("tiff: region %v is outside the %dx%d picture", *region, levelW, levelH)
	}
	return want, nil
}

func legacyRegion(region *image.Rectangle, scale int) *image.Rectangle {
	if region == nil {
		return nil
	}
	factor := 1 << uint(scaleShift(scale))
	r := region.Canon()
	full := image.Rect(r.Min.X*factor, r.Min.Y*factor, r.Max.X*factor, r.Max.Y*factor)
	return &full
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

func legacyProbe(ctx context.Context, r io.ReaderAt, size int64, lim codec.Limits) (codec.Info, error) {
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

func legacyDecode(ctx context.Context, r io.ReaderAt, size int64, lim codec.Limits, region *image.Rectangle) (*codec.Image, error) {
	info, err := legacyProbe(ctx, r, size, lim)
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
	if region != nil {
		pix = crop(pix, region.Canon())
		if pix.Rect.Empty() {
			return nil, guard.Corrupt("tiff: region %v is outside the picture", *region)
		}
	}
	return &codec.Image{Info: info, Frames: []codec.Frame{{Pix: pix}}, Scale: 1}, nil
}

func crop(src *image.NRGBA, region image.Rectangle) *image.NRGBA {
	region = region.Intersect(src.Rect)
	if region.Empty() {
		return image.NewNRGBA(image.Rectangle{})
	}
	dst := image.NewNRGBA(image.Rect(0, 0, region.Dx(), region.Dy()))
	for y := 0; y < region.Dy(); y++ {
		from := src.PixOffset(region.Min.X, region.Min.Y+y)
		copy(dst.Pix[y*dst.Stride:y*dst.Stride+region.Dx()*4], src.Pix[from:from+region.Dx()*4])
	}
	return dst
}
