package guard

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"math"

	"github.com/oops1/PixGo/internal/codec"
)

const streamBufferSize = 64 << 10

func Limits(l codec.Limits) codec.Limits {
	defaults := codec.DefaultLimits()
	if l.MaxFileBytes <= 0 {
		l.MaxFileBytes = defaults.MaxFileBytes
	}
	if l.MaxPixels <= 0 {
		l.MaxPixels = defaults.MaxPixels
	}
	if l.MaxFrames <= 0 {
		l.MaxFrames = defaults.MaxFrames
	}
	if l.MaxFramePixels <= 0 {
		l.MaxFramePixels = defaults.MaxFramePixels
	}
	return l
}

func CheckSize(size int64, lim codec.Limits) error {
	if size < 0 {
		return fmt.Errorf("%w: negative size", codec.ErrCorrupt)
	}
	if size > lim.MaxFileBytes {
		return fmt.Errorf("%w: file is %d bytes", codec.ErrTooLarge, size)
	}
	return nil
}

func Corrupt(format string, args ...any) error {
	return fmt.Errorf("%w: %s", codec.ErrCorrupt, fmt.Sprintf(format, args...))
}

func Unsupported(format string, args ...any) error {
	return fmt.Errorf("%w: %s", codec.ErrUnsupported, fmt.Sprintf(format, args...))
}

func TooLarge(format string, args ...any) error {
	return fmt.Errorf("%w: %s", codec.ErrTooLarge, fmt.Sprintf(format, args...))
}

func IsCodecError(err error) bool {
	return errors.Is(err, codec.ErrCorrupt) ||
		errors.Is(err, codec.ErrUnsupported) ||
		errors.Is(err, codec.ErrTooLarge) ||
		errors.Is(err, codec.ErrUnknownFormat)
}

func Classify(ctx context.Context, err error, isUnsupported func(error) bool) error {
	if err == nil {
		return nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || IsCodecError(err) {
		return err
	}
	if isUnsupported != nil && isUnsupported(err) {
		return fmt.Errorf("%w: %v", codec.ErrUnsupported, err)
	}
	return fmt.Errorf("%w: %v", codec.ErrCorrupt, err)
}

type Reader struct {
	ctx     context.Context
	section *io.SectionReader
}

func NewReader(ctx context.Context, r io.ReaderAt, size int64) *Reader {
	if size < 0 {
		size = 0
	}
	return &Reader{ctx: ctx, section: io.NewSectionReader(r, 0, size)}
}

func (r *Reader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.section.Read(p)
}

func (r *Reader) ReadAt(p []byte, off int64) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.section.ReadAt(p, off)
}

func (r *Reader) Size() int64 { return r.section.Size() }

func Stream(ctx context.Context, r io.ReaderAt, size int64) *bufio.Reader {
	return bufio.NewReaderSize(NewReader(ctx, r, size), streamBufferSize)
}

func ReadAt(r io.ReaderAt, off, n, size int64) ([]byte, error) {
	if off < 0 || n < 0 || off > size || n > size-off {
		return nil, Corrupt("range %d+%d outside %d byte file", off, n, size)
	}
	if n > math.MaxInt32 {
		return nil, TooLarge("read of %d bytes", n)
	}
	buf := make([]byte, n)
	if n == 0 {
		return buf, nil
	}
	got, err := r.ReadAt(buf, off)
	if got == len(buf) {
		return buf, nil
	}
	if err == nil || err == io.EOF {
		err = io.ErrUnexpectedEOF
	}
	return nil, fmt.Errorf("%w: %v", codec.ErrCorrupt, err)
}

func ReadAtInto(r io.ReaderAt, buf []byte, off, size int64) error {
	n := int64(len(buf))
	if off < 0 || off > size || n > size-off {
		return Corrupt("range %d+%d outside %d byte file", off, n, size)
	}
	if n == 0 {
		return nil
	}
	got, err := r.ReadAt(buf, off)
	if got == len(buf) {
		return nil
	}
	if err == nil || err == io.EOF {
		err = io.ErrUnexpectedEOF
	}
	return fmt.Errorf("%w: %v", codec.ErrCorrupt, err)
}

type FrameBudget struct {
	limits codec.Limits
	frames int
	pixels int64
}

func NewFrameBudget(lim codec.Limits) *FrameBudget {
	return &FrameBudget{limits: lim}
}

func (b *FrameBudget) Add(width, height int) error {
	if err := b.limits.Check(width, height); err != nil {
		return err
	}
	b.frames++
	if b.frames > b.limits.MaxFrames {
		return TooLarge("more than %d frames", b.limits.MaxFrames)
	}
	b.pixels += int64(width) * int64(height)
	if b.pixels > b.limits.MaxFramePixels {
		return TooLarge("frames exceed %d pixels in total", b.limits.MaxFramePixels)
	}
	return nil
}

func (b *FrameBudget) Frames() int { return b.frames }

func CheckFrames(lim codec.Limits, frames, width, height int) error {
	if err := lim.Check(width, height); err != nil {
		return err
	}
	if frames > lim.MaxFrames {
		return TooLarge("%d frames exceed limit %d", frames, lim.MaxFrames)
	}
	if int64(frames) > lim.MaxFramePixels/(int64(width)*int64(height)) {
		return TooLarge("%d frames of %dx%d exceed %d pixels in total", frames, width, height, lim.MaxFramePixels)
	}
	return nil
}
