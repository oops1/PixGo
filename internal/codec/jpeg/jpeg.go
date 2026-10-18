package jpeg

import (
	"context"
	"errors"
	"image"
	"image/color"
	stdjpeg "image/jpeg"
	"io"
	"math/bits"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/convert"
	"github.com/oops1/PixGo/internal/codec/internal/guard"
	"github.com/oops1/PixGo/internal/codec/internal/splice"
	"github.com/oops1/PixGo/internal/jpegdec"
	"github.com/oops1/PixGo/internal/meta"
	"github.com/oops1/PixGo/internal/meta/exif"
)

const (
	DefaultQuality = 90
	MinQuality     = 1
	MaxQuality     = 100
)

type Decoder struct {
	Workers int
}

func (Decoder) ID() codec.ID { return codec.JPEG }

func (Decoder) Name() string { return "JPEG" }

func (Decoder) Extensions() []string {
	return []string{".jpg", ".jpeg", ".jpe", ".jfif"}
}

func (Decoder) Sniff(head []byte) bool {
	return len(head) >= 3 && head[0] == 0xff && head[1] == 0xd8 && head[2] == 0xff
}

func isUnsupported(err error) bool {
	var unsupported jpegdec.UnsupportedError
	return errors.As(err, &unsupported)
}

func classify(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	if errors.Is(err, jpegdec.ErrTooLarge) {
		return guard.TooLarge("%v", err)
	}
	return guard.Classify(ctx, err, isUnsupported)
}

func bitsPerPixel(components int) int {
	switch components {
	case 1:
		return 8
	case 4:
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

func infoOf(h jpegdec.Header, orientation codec.Orientation) codec.Info {
	return codec.Info{
		Format:      codec.JPEG,
		FormatName:  "JPEG",
		Width:       h.Width,
		Height:      h.Height,
		Frames:      1,
		Orientation: orientation,
		BitDepth:    bitsPerPixel(h.Components),
	}
}

func probe(ctx context.Context, r io.ReaderAt, size int64, lim codec.Limits) (codec.Info, error) {
	if err := guard.CheckSize(size, lim); err != nil {
		return codec.Info{}, err
	}
	if err := ctx.Err(); err != nil {
		return codec.Info{}, err
	}
	header, err := jpegdec.ReadHeader(ctx, r, size)
	if err != nil {
		return codec.Info{}, classify(ctx, err)
	}
	if header.Width <= 0 || header.Height <= 0 {
		return codec.Info{}, guard.Corrupt("image has no pixels")
	}
	pixels := int64(header.Width) * int64(header.Height)
	if pixels > lim.MaxTiledPixels {
		return codec.Info{}, guard.TooLarge("%dx%d pixels exceed the %d pixel limit", header.Width, header.Height, lim.MaxTiledPixels)
	}
	info := infoOf(header, orientationOf(r, size))
	if pixels > lim.MaxPixels {
		return info, guard.TooLarge("%dx%d pixels exceed the %d pixel limit", header.Width, header.Height, lim.MaxPixels)
	}
	return info, nil
}

func (Decoder) Probe(ctx context.Context, r io.ReaderAt, size int64, lim codec.Limits) (codec.Info, error) {
	return probe(ctx, r, size, lim.WithDefaults())
}

func supportedScale(requested int) int {
	switch {
	case requested >= 8:
		return 8
	case requested >= 4:
		return 4
	case requested >= 2:
		return 2
	}
	return 1
}

func (d Decoder) options(lim codec.Limits) jpegdec.Options {
	return jpegdec.Options{Workers: d.Workers, MaxPixels: lim.MaxTiledPixels}
}

func (d Decoder) Decode(ctx context.Context, r io.ReaderAt, size int64, o codec.Options) (*codec.Image, error) {
	lim := o.Limits.WithDefaults()
	info, err := probe(ctx, r, size, lim)
	if err != nil && !(errors.Is(err, codec.ErrTooLarge) && info.Width > 0) {
		return nil, err
	}
	scale := supportedScale(o.Scale)
	shift := bits.TrailingZeros(uint(scale))
	levelW, levelH := codec.LevelSize(info.Width, info.Height, shift)
	level := image.Rect(0, 0, levelW, levelH)
	want := level
	if o.Region != nil {
		want = o.Region.Intersect(level)
		if want.Empty() {
			return nil, guard.Corrupt("region %v is outside the %dx%d picture", *o.Region, levelW, levelH)
		}
	}
	if int64(want.Dx())*int64(want.Dy()) > lim.MaxPixels {
		return nil, guard.TooLarge("%dx%d output pixels exceed the %d pixel limit", want.Dx(), want.Dy(), lim.MaxPixels)
	}
	dec, err := jpegdec.Open(ctx, r, size, d.options(lim))
	if err != nil {
		return nil, classify(ctx, err)
	}
	defer dec.Close()
	if !dec.CanRegion() && int64(levelW)*int64(levelH) > lim.MaxPixels {
		return nil, guard.TooLarge("%dx%d pixels need the whole level in memory, over the %d pixel limit", levelW, levelH, lim.MaxPixels)
	}
	pix, err := decodeLevel(ctx, dec, shift, want, want != level)
	if err != nil {
		return nil, classify(ctx, err)
	}
	color := meta.ReadColor(ctx, r, size, codec.JPEG)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &codec.Image{Info: info, Color: color, Frames: []codec.Frame{{Pix: pix}}, Scale: scale}, nil
}

func decodeLevel(ctx context.Context, dec *jpegdec.Decoder, shift int, want image.Rectangle, partial bool) (*image.NRGBA, error) {
	if !partial {
		return dec.Decode(ctx, shift)
	}
	if !dec.CanRegion() {
		if err := dec.Prepare(ctx, shift); err != nil {
			return nil, err
		}
	}
	return dec.Region(ctx, shift, want)
}

func (d Decoder) OpenTiled(ctx context.Context, r io.ReaderAt, size int64, lim codec.Limits) (codec.TileReader, error) {
	lim = lim.WithDefaults()
	if err := guard.CheckSize(size, lim); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	dec, err := jpegdec.Open(ctx, r, size, d.options(lim))
	if err != nil {
		return nil, classify(ctx, err)
	}
	header := dec.Header()
	if header.Width <= 0 || header.Height <= 0 {
		dec.Close()
		return nil, guard.Corrupt("image has no pixels")
	}
	return &tileReader{dec: dec, info: infoOf(header, orientationOf(r, size)), color: meta.ReadColor(ctx, r, size, codec.JPEG)}, nil
}

type tileReader struct {
	dec   *jpegdec.Decoder
	info  codec.Info
	color codec.ColorInfo
}

func (t *tileReader) Color() codec.ColorInfo { return t.color }

func (t *tileReader) Info() codec.Info { return t.info }

func (t *tileReader) MinShift() int { return t.dec.MinShift() }

func (t *tileReader) Close() error {
	t.dec.Close()
	return nil
}

func (t *tileReader) Overview(ctx context.Context, shift int) (*image.NRGBA, error) {
	if shift < 0 {
		return nil, guard.Unsupported("negative level %d", shift)
	}
	native := min(shift, jpegdec.MaxShift)
	img, err := t.dec.Decode(ctx, native)
	if err != nil {
		return nil, classify(ctx, err)
	}
	for n := native; n < shift; n++ {
		img = halve(img)
	}
	return img, nil
}

func (t *tileReader) Region(ctx context.Context, shift int, r image.Rectangle) (*image.NRGBA, error) {
	if shift < 0 {
		return nil, guard.Unsupported("negative level %d", shift)
	}
	w, h := codec.LevelSize(t.info.Width, t.info.Height, shift)
	r = r.Intersect(image.Rect(0, 0, w, h))
	if r.Empty() {
		return image.NewNRGBA(image.Rectangle{}), nil
	}
	if shift <= jpegdec.MaxShift {
		img, err := t.dec.Region(ctx, shift, r)
		return img, classify(ctx, err)
	}
	extra := shift - jpegdec.MaxShift
	w3, h3 := codec.LevelSize(t.info.Width, t.info.Height, jpegdec.MaxShift)
	source := image.Rect(r.Min.X<<uint(extra), r.Min.Y<<uint(extra), r.Max.X<<uint(extra), r.Max.Y<<uint(extra)).Intersect(image.Rect(0, 0, w3, h3))
	img, err := t.dec.Region(ctx, jpegdec.MaxShift, source)
	if err != nil {
		return nil, classify(ctx, err)
	}
	for n := 0; n < extra; n++ {
		img = halve(img)
	}
	return img, nil
}

func halve(src *image.NRGBA) *image.NRGBA {
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	nw, nh := (w+1)/2, (h+1)/2
	dst := image.NewNRGBA(image.Rect(0, 0, nw, nh))
	for y := 0; y < nh; y++ {
		top := src.Pix[y*2*src.Stride:]
		bottom := src.Pix[min(2*y+1, h-1)*src.Stride:]
		out := dst.Pix[y*dst.Stride : y*dst.Stride+nw*4]
		for x := 0; x < nw; x++ {
			left := x * 8
			right := min(2*x+1, w-1) * 4
			o := x * 4
			for c := 0; c < 3; c++ {
				sum := int(top[left+c]) + int(top[right+c]) + int(bottom[left+c]) + int(bottom[right+c])
				out[o+c] = uint8((sum + 2) >> 2)
			}
			out[o+3] = 0xff
		}
	}
	return dst
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
	out := splice.New(w, soiLength, metadataSegments(o.Metadata))
	if err := stdjpeg.Encode(out, flattened, &stdjpeg.Options{Quality: ClampQuality(o.Quality)}); err != nil {
		return err
	}
	return out.Flush()
}
