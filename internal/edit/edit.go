package edit

import (
	"context"
	"errors"
	"image"
	"math"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/imaging"
)

var (
	ErrEmptyPicture = errors.New("edit: there is no picture")
	ErrEmptyCrop    = errors.New("edit: the crop rectangle is outside the picture")
	ErrSize         = errors.New("edit: the target size is not valid")
)

type Plan struct {
	Orientation codec.Orientation
	Crop        image.Rectangle
	Size        image.Point
}

func (p Plan) orientation() codec.Orientation {
	if !p.Orientation.Valid() {
		return codec.OrientNormal
	}
	return p.Orientation
}

func (p Plan) OrientedSize(w, h int) (int, int) { return p.orientation().Size(w, h) }

func (p Plan) CropRect(w, h int) image.Rectangle {
	ow, oh := p.OrientedSize(w, h)
	full := image.Rect(0, 0, ow, oh)
	if p.Crop.Empty() {
		return full
	}
	return p.Crop.Intersect(full)
}

func (p Plan) OutputSize(w, h int) (int, int) {
	r := p.CropRect(w, h)
	if p.Size.X > 0 && p.Size.Y > 0 {
		return p.Size.X, p.Size.Y
	}
	return r.Dx(), r.Dy()
}

func (p Plan) Resizes(w, h int) bool {
	r := p.CropRect(w, h)
	ow, oh := p.OutputSize(w, h)
	return ow != r.Dx() || oh != r.Dy()
}

func (p Plan) Changes(w, h int) bool {
	ow, oh := p.OrientedSize(w, h)
	return p.orientation() != codec.OrientNormal || p.CropRect(w, h) != image.Rect(0, 0, ow, oh) || p.Resizes(w, h)
}

func (p Plan) LosslessCapable(w, h int) bool { return !p.Resizes(w, h) }

func storedRect(o codec.Orientation, r image.Rectangle, w, h int) image.Rectangle {
	fw, fh := float64(w), float64(h)
	x0, y0 := o.ToSource(float64(r.Min.X), float64(r.Min.Y), fw, fh)
	x1, y1 := o.ToSource(float64(r.Max.X), float64(r.Max.Y), fw, fh)
	return image.Rect(
		int(math.Round(math.Min(x0, x1))), int(math.Round(math.Min(y0, y1))),
		int(math.Round(math.Max(x0, x1))), int(math.Round(math.Max(y0, y1))),
	)
}

func window(src *image.NRGBA, r image.Rectangle) *image.NRGBA {
	return &image.NRGBA{Pix: src.Pix[src.PixOffset(r.Min.X, r.Min.Y):], Stride: src.Stride, Rect: image.Rect(0, 0, r.Dx(), r.Dy())}
}

func clone(src *image.NRGBA) *image.NRGBA {
	b := src.Bounds()
	dst := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	n := b.Dx() * 4
	for y := 0; y < b.Dy(); y++ {
		so := src.PixOffset(b.Min.X, b.Min.Y+y)
		copy(dst.Pix[y*dst.Stride:y*dst.Stride+n], src.Pix[so:so+n])
	}
	return dst
}

func Apply(ctx context.Context, src *image.NRGBA, p Plan) (*image.NRGBA, error) {
	if src == nil || src.Bounds().Empty() {
		return nil, ErrEmptyPicture
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	o := p.orientation()
	r := p.CropRect(w, h)
	if r.Empty() {
		return nil, ErrEmptyCrop
	}
	view := window(src, storedRect(o, r, w, h).Add(b.Min))
	if !p.Resizes(w, h) {
		if o == codec.OrientNormal {
			return clone(view), nil
		}
		return imaging.Orient(view, o), nil
	}
	ow, oh := p.OutputSize(w, h)
	if o == codec.OrientNormal {
		return Resample(ctx, view, ow, oh)
	}
	if ow*oh <= r.Dx()*r.Dy() {
		tw, th := ow, oh
		if o.Transposes() {
			tw, th = oh, ow
		}
		small, err := Resample(ctx, view, tw, th)
		if err != nil {
			return nil, err
		}
		return imaging.Orient(small, o), nil
	}
	oriented := imaging.Orient(view, o)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return Resample(ctx, oriented, ow, oh)
}
