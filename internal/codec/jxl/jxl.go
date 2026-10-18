package jxl

import (
	"bytes"
	"context"
	"errors"
	"image"
	"io"
	"math"

	"github.com/gen2brain/jxl"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/assemble"
	"github.com/oops1/PixGo/internal/codec/internal/guard"
	"github.com/oops1/PixGo/internal/codec/internal/shield"
)

const (
	formatTag           = "jxl"
	serialThreads       = 1
	maxWorkingSetBytes  = 3 << 30
	workingBytesPlane   = 10
	frameBytesPlane     = 4
	frameBytesNRGBA     = 4
	deepFrameExtraBytes = 4
)

type Decoder struct {
	Threads int
}

func (d Decoder) threads() int { return max(d.Threads, serialThreads) }

func (Decoder) ID() codec.ID { return codec.JXL }

func (Decoder) Name() string { return "JPEG XL" }

func (Decoder) Extensions() []string { return []string{".jxl"} }

func (Decoder) Sniff(head []byte) bool {
	return bytes.HasPrefix(head, codestreamSignature) || bytes.HasPrefix(head, containerSignature)
}

type inspection struct {
	header imageHeader
	info   codec.Info
	frames int
	width  int
	height int
}

func readHeader(stream *codestream) (imageHeader, error) {
	window := int64(initialWindow)
	for {
		data, err := stream.read(0, min(window, stream.size))
		if err != nil {
			return imageHeader{}, err
		}
		header, err := parseImageHeader(data)
		if !errors.Is(err, errNeedMoreData) {
			return header, err
		}
		if int64(len(data)) >= stream.size {
			return imageHeader{}, guard.Corrupt("jxl: codestream ends inside the image header")
		}
		if window >= maxWindow {
			return imageHeader{}, guard.Corrupt("jxl: image header larger than %d bytes", maxWindow)
		}
		window *= 4
	}
}

func bitsPerPixel(header imageHeader) int {
	channels := header.colorChannels()
	if header.hasAlpha() {
		channels++
	}
	return channels * int(header.bitsPerSample)
}

func inspectFile(ctx context.Context, r io.ReaderAt, size int64, lim codec.Limits) (inspection, error) {
	if err := guard.CheckSize(size, lim); err != nil {
		return inspection{}, err
	}
	if err := ctx.Err(); err != nil {
		return inspection{}, err
	}
	reader := guard.NewReader(ctx, r, size)
	stream, err := openCodestream(ctx, reader, size)
	if err != nil {
		return inspection{}, err
	}
	if stream.level > supportedLevel {
		return inspection{}, guard.Unsupported("jxl: codestream level %d", stream.level)
	}
	header, err := readHeader(stream)
	if err != nil {
		return inspection{}, err
	}
	width, height := header.displaySize()
	if err := lim.Check(width, height); err != nil {
		return inspection{}, err
	}
	frames := 1
	if header.animated {
		if header.embeddedProfile {
			return inspection{}, guard.Unsupported("jxl: animations with an embedded colour profile are not supported")
		}
		counted, err := stream.countFrames(ctx, header, lim.MaxFrames)
		switch {
		case errors.Is(err, errPermutedTable):
			return inspection{}, guard.Unsupported("jxl: animations with a permuted table of contents are not supported")
		case err != nil:
			return inspection{}, err
		}
		frames = max(counted.visible, 1)
	}
	info := codec.Info{
		Format:      codec.JXL,
		FormatName:  "JPEG XL",
		Width:       width,
		Height:      height,
		Frames:      frames,
		Animated:    frames > 1,
		HasAlpha:    header.hasAlpha(),
		Orientation: codec.OrientNormal,
		BitDepth:    bitsPerPixel(header),
	}
	return inspection{header: header, info: info, frames: frames, width: width, height: height}, nil
}

func inspect(ctx context.Context, r io.ReaderAt, size int64, lim codec.Limits) (inspection, error) {
	var found inspection
	err := shield.Run(formatTag, func() error {
		var err error
		found, err = inspectFile(ctx, r, size, lim)
		return err
	})
	if err != nil {
		return inspection{}, guard.Classify(ctx, err, nil)
	}
	return found, nil
}

func (Decoder) Probe(ctx context.Context, r io.ReaderAt, size int64, lim codec.Limits) (codec.Info, error) {
	found, err := inspect(ctx, r, size, guard.Limits(lim))
	return found.info, err
}

func (h imageHeader) planes() int {
	planes := h.colorChannels()
	if h.xybEncoded {
		planes = 3
	}
	return planes + len(h.extraChannels)
}

func workingSet(found inspection) int64 {
	planes := int64(found.header.planes())
	pixels := int64(found.width) * int64(found.height)
	return pixels * (workingBytesPlane*planes + int64(found.frames)*(frameBytesPlane*planes+frameBytesNRGBA))
}

func frameSizeLimit(lim codec.Limits) int {
	return int(min(lim.MaxPixels, math.MaxInt32))
}

func classify(ctx context.Context, err error) error {
	if errors.Is(err, jxl.ErrFrameTooLarge) {
		return guard.TooLarge("jxl: %v", err)
	}
	return guard.Classify(ctx, err, func(err error) bool { return errors.Is(err, jxl.ErrUnsupported) })
}

func (d Decoder) Decode(ctx context.Context, r io.ReaderAt, size int64, o codec.Options) (*codec.Image, error) {
	lim := guard.Limits(o.Limits)
	found, err := inspect(ctx, r, size, lim)
	if err != nil {
		return nil, err
	}
	if err := guard.CheckFrames(lim, found.frames, found.width, found.height); err != nil {
		return nil, err
	}
	plan, hdr := found.header.hdrPlan()
	hdr = hdr && o.Deep
	need := workingSet(found)
	if hdr {
		need += int64(found.width) * int64(found.height) * deepFrameExtraBytes
	}
	if need > maxWorkingSetBytes {
		return nil, guard.TooLarge("jxl: decoding would need about %d MiB", need>>20)
	}
	source := guard.Stream(ctx, r, size)
	if hdr {
		return d.decodeHDR(ctx, source, found, plan, lim)
	}
	var result *jxl.JXL
	err = shield.Run(formatTag, func() error {
		var err error
		result, err = jxl.DecodeAll(source, jxl.Options{Threads: d.threads(), FrameSizeLimit: frameSizeLimit(lim)})
		return err
	})
	if err != nil {
		return nil, classify(ctx, err)
	}
	images := result.Image
	if o.Frames == codec.FirstFrame && len(images) > 1 {
		images = images[:1]
	}
	color := found.header.color(result.Color.ICCP)
	source2 := assemble.Source{Images: images, Delays: result.Delay}
	var frames []codec.Frame
	var bounds image.Rectangle
	if o.Deep && assemble.WantsDeep(source2, color) {
		frames, err = assemble.DeepFrames(ctx, formatTag, source2, nil)
		if err == nil {
			bounds = frames[0].Deep.Bounds()
		}
	} else {
		frames, err = assemble.Frames(ctx, formatTag, source2, nil)
		if err == nil {
			bounds = frames[0].Pix.Bounds()
		}
	}
	if err != nil {
		return nil, err
	}
	info := found.info
	info.Width, info.Height = bounds.Dx(), bounds.Dy()
	if o.Frames == codec.AllFrames {
		info.Frames = len(frames)
		info.Animated = len(frames) > 1
	}
	return &codec.Image{Info: info, Color: color, Frames: frames, Loop: int(result.Loops), Scale: 1}, nil
}
