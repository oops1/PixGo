package jpegxform

import (
	"context"
	"errors"
	"fmt"
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
	if _, _, perfect := edgeCrop(&h.fr, op); !perfect && !o.Trim {
		return ErrImperfect
	}
	pl := newPlan(&h.fr, op)
	if pl.outW == 0 || pl.outH == 0 {
		return fmt.Errorf("%w: the picture is smaller than one block", ErrImperfect)
	}
	grids, err := decodeScan(ctx, src, h)
	if err != nil {
		return err
	}
	return writeJPEG(ctx, w, h, grids, pl)
}
