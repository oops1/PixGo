package webp

import (
	"bytes"
	"context"
	"errors"
	"image"
	"io"
	"math"
	"time"

	vpx "github.com/gen2brain/vpx/webp"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/convert"
	"github.com/oops1/PixGo/internal/codec/internal/guard"
	"github.com/oops1/PixGo/internal/meta"
)

const (
	decodeThreads = 1
	minFrameDelay = 10 * time.Millisecond
	frameDelayFix = 100 * time.Millisecond
)

type Decoder struct{}

func (Decoder) ID() codec.ID { return codec.WebP }

func (Decoder) Name() string { return "WebP" }

func (Decoder) Extensions() []string { return []string{".webp"} }

func (Decoder) Sniff(head []byte) bool {
	return len(head) >= riffHeaderSize && bytes.Equal(head[:4], []byte("RIFF")) && bytes.Equal(head[8:12], []byte("WEBP"))
}

func frameDelay(milliseconds int) time.Duration {
	delay := time.Duration(milliseconds) * time.Millisecond
	if delay <= minFrameDelay {
		return frameDelayFix
	}
	return delay
}

func inspect(ctx context.Context, r io.ReaderAt, size int64, lim codec.Limits) (layout, error) {
	if err := guard.CheckSize(size, lim); err != nil {
		return layout{}, err
	}
	if err := ctx.Err(); err != nil {
		return layout{}, err
	}
	found, err := walkChunks(ctx, r, size)
	if err != nil {
		return found, err
	}
	if err := lim.Check(found.width, found.height); err != nil {
		return found, err
	}
	return found, nil
}

func (Decoder) Probe(ctx context.Context, r io.ReaderAt, size int64, lim codec.Limits) (codec.Info, error) {
	found, err := inspect(ctx, r, size, guard.Limits(lim))
	if err != nil {
		return codec.Info{}, err
	}
	return found.info(), nil
}

func isUnsupported(err error) bool { return errors.Is(err, vpx.ErrUnsupported) }

func pixelLimit(lim codec.Limits) int {
	return int(min(lim.MaxPixels, math.MaxInt32))
}

func (Decoder) Decode(ctx context.Context, r io.ReaderAt, size int64, o codec.Options) (*codec.Image, error) {
	lim := guard.Limits(o.Limits)
	found, err := inspect(ctx, r, size, lim)
	if err != nil {
		return nil, err
	}
	info := found.info()
	every := info.Animated && o.Frames == codec.AllFrames
	if every {
		if err := guard.CheckFrames(lim, found.frames, found.width, found.height); err != nil {
			return nil, err
		}
	}
	color := meta.ReadColor(ctx, r, size, codec.WebP)
	source := io.NewSectionReader(guard.NewReader(ctx, r, size), 0, size)
	options := vpx.Options{ToRGBA: !found.lossless, Threads: decodeThreads, FrameSizeLimit: pixelLimit(lim)}
	var frames []codec.Frame
	if every {
		frames, err = decodeAll(source, options)
	} else {
		frames, err = decodeFirst(source, options, found.firstDelay)
	}
	if err != nil {
		return nil, guard.Classify(ctx, err, isUnsupported)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if every {
		info.Frames = len(frames)
		info.Animated = len(frames) > 1
	}
	return &codec.Image{Info: info, Color: color, Frames: frames, Loop: found.loop, Scale: 1}, nil
}

func decodeFirst(source io.Reader, options vpx.Options, delay time.Duration) ([]codec.Frame, error) {
	decoded, err := vpx.Decode(source, options)
	if err != nil {
		return nil, err
	}
	return []codec.Frame{{Pix: straight(decoded), Delay: delay}}, nil
}

func decodeAll(source io.Reader, options vpx.Options) ([]codec.Frame, error) {
	animation, err := vpx.DecodeAll(source, options)
	if err != nil {
		return nil, err
	}
	frames := make([]codec.Frame, len(animation.Image))
	for i, decoded := range animation.Image {
		frames[i] = codec.Frame{Pix: straight(decoded), Delay: frameDelay(animation.Delay[i])}
	}
	return frames, nil
}

func straight(decoded image.Image) *image.NRGBA {
	if rgba, ok := decoded.(*image.RGBA); ok {
		return straightAlpha(rgba)
	}
	return convert.NRGBA(decoded)
}
