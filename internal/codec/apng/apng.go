package apng

import (
	"bytes"
	"context"
	"encoding/binary"
	"image"
	stdpng "image/png"
	"io"
	"math"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/convert"
	"github.com/oops1/PixGo/internal/codec/internal/guard"
)

type Decoder struct{}

func (Decoder) ID() codec.ID { return codec.APNG }

func (Decoder) Name() string { return "APNG" }

func (Decoder) Extensions() []string { return []string{".apng", ".png"} }

func (Decoder) Sniff(head []byte) bool {
	if len(head) < firstChunkOffset || !bytes.Equal(head[:signatureSize], signature) {
		return false
	}
	if binary.BigEndian.Uint32(head[8:12]) != headerChunkSize || string(head[12:16]) != "IHDR" {
		return false
	}
	position := int64(firstChunkOffset)
	for position+chunkHeaderSize <= int64(len(head)) {
		length := int64(binary.BigEndian.Uint32(head[position : position+4]))
		switch string(head[position+4 : position+8]) {
		case "acTL":
			return length == controlSize
		case "IDAT", "IEND":
			return false
		}
		position += chunkHeaderSize + length + chunkTrailerSize
	}
	return false
}

func inspect(ctx context.Context, r io.ReaderAt, size int64, lim codec.Limits, keep bool) (codec.Info, *layout, error) {
	lim = guard.Limits(lim)
	if err := guard.CheckSize(size, lim); err != nil {
		return codec.Info{}, nil, err
	}
	if err := ctx.Err(); err != nil {
		return codec.Info{}, nil, err
	}
	l, err := scan(ctx, r, size, lim, keep)
	if err != nil {
		return codec.Info{}, nil, guard.Classify(ctx, err, nil)
	}
	return codec.Info{
		Format:      codec.APNG,
		FormatName:  "APNG",
		Width:       l.width,
		Height:      l.height,
		Frames:      l.frameCount,
		Animated:    l.frameCount > 1,
		HasAlpha:    l.hasAlpha(),
		Orientation: l.orientation,
		BitDepth:    l.bitsPerPixel(),
	}, l, nil
}

func (Decoder) Probe(ctx context.Context, r io.ReaderAt, size int64, lim codec.Limits) (codec.Info, error) {
	info, _, err := inspect(ctx, r, size, lim, false)
	return info, err
}

func (l *layout) decodeFrame(ctx context.Context, r io.ReaderAt, size int64, f frame) (*image.NRGBA, error) {
	stream, err := l.frameStream(ctx, r, size, f)
	if err != nil {
		return nil, err
	}
	decoded, err := stdpng.Decode(bytes.NewReader(stream))
	if err != nil {
		return nil, guard.Classify(ctx, err, nil)
	}
	return convert.NRGBA(decoded), nil
}

func (Decoder) Decode(ctx context.Context, r io.ReaderAt, size int64, o codec.Options) (*codec.Image, error) {
	lim := guard.Limits(o.Limits)
	info, l, err := inspect(ctx, r, size, lim, true)
	if err != nil {
		return nil, err
	}
	count := 1
	if o.Frames == codec.AllFrames {
		count = len(l.frames)
		if err := guard.CheckFrames(lim, count, l.width, l.height); err != nil {
			return nil, err
		}
	}
	budget := guard.NewFrameBudget(lim)
	canvas := newCompositor(l.width, l.height)
	frames := make([]codec.Frame, 0, count)
	for i := 0; i < count; i++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := budget.Add(l.width, l.height); err != nil {
			return nil, err
		}
		pix, err := l.decodeFrame(ctx, r, size, l.frames[i])
		if err != nil {
			return nil, err
		}
		canvas.apply(l.frames[i], pix)
		shown := canvas.canvas
		if i < count-1 {
			shown = canvas.snapshot()
		}
		frames = append(frames, codec.Frame{Pix: shown, Delay: l.frames[i].delay})
	}
	return &codec.Image{
		Info:   info,
		Frames: frames,
		Loop:   int(min(l.numPlays, math.MaxInt32)),
		Scale:  1,
	}, nil
}
