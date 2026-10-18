package jpegxform

import (
	"context"
	"errors"
	"fmt"
	"image"
	"io"
)

const (
	DefaultMaxPixels = 1 << 28
	maxInputBytes    = 1 << 30
)

var ErrImperfect = errors.New("jpegxform: the picture size does not fit the block grid of this transform")

type Options struct {
	Trim      bool
	MaxPixels int
	Crop      image.Rectangle
	StripGPS  bool
	Applied   *image.Rectangle
}

func Perfect(r io.ReaderAt, size int64, op Op) (bool, error) {
	if !op.Valid() {
		return false, fmt.Errorf("jpegxform: invalid operation %d", op)
	}
	h, err := readHeader(newByteSource(r, size, 0), false)
	if err != nil {
		return false, err
	}
	_, _, perfect := edgeCrop(&h.fr, op)
	return perfect, nil
}

func Transform(w io.Writer, r io.ReaderAt, size int64, op Op, o Options) error {
	return TransformContext(context.Background(), w, r, size, op, o)
}

func TransformContext(ctx context.Context, w io.Writer, r io.ReaderAt, size int64, op Op, o Options) error {
	if !op.Valid() {
		return fmt.Errorf("jpegxform: invalid operation %d", op)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if size > maxInputBytes {
		return tooLarge("%d bytes", size)
	}
	src := newByteSource(r, size, 0)
	src.ctx = ctx
	h, err := readHeader(src, true)
	if err != nil {
		if ioErr := ioFailure(src.err); ioErr != nil {
			return ioErr
		}
		return err
	}
	limit := int64(o.MaxPixels)
	if limit <= 0 {
		limit = DefaultMaxPixels
	}
	if int64(h.fr.width)*int64(h.fr.height) > limit {
		return tooLarge("%dx%d pixels", h.fr.width, h.fr.height)
	}
	pl, err := planFor(&h.fr, op, o)
	if err != nil {
		return err
	}
	grids, err := decodeScan(ctx, src, h)
	if err != nil {
		return err
	}
	return writeJPEG(ctx, w, h, grids, pl, o)
}

func planFor(fr *frame, op Op, o Options) (*plan, error) {
	g := geometryOf(fr, op)
	if o.Crop.Empty() {
		if _, _, perfect := edgeCrop(fr, op); !perfect && !o.Trim {
			return nil, ErrImperfect
		}
		pl := newPlan(fr, op)
		if pl.outW == 0 || pl.outH == 0 {
			return nil, fmt.Errorf("%w: the picture is smaller than one block", ErrImperfect)
		}
		if o.Applied != nil {
			*o.Applied = image.Rect(0, 0, pl.outW, pl.outH).Add(image.Pt(g.padXOrigin(), g.padYOrigin()))
		}
		return pl, nil
	}
	rect, trimmed, err := g.snap(o.Crop)
	if err != nil {
		return nil, err
	}
	if trimmed && !o.Trim {
		return nil, ErrImperfect
	}
	if o.Applied != nil {
		*o.Applied = rect
	}
	return newCropPlan(fr, op, g, rect), nil
}
