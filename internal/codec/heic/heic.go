package heic

import (
	"context"
	"errors"
	"image"
	"io"
	"math"

	"github.com/gen2brain/h265/heic"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/assemble"
	"github.com/oops1/PixGo/internal/codec/internal/guard"
	"github.com/oops1/PixGo/internal/codec/internal/shield"
	"github.com/oops1/PixGo/internal/isobmff"
)

const (
	formatTag     = "heic"
	serialThreads = 1
)

var (
	alphaURNs       = []string{"urn:mpeg:mpegB:cicp:systems:auxiliary:alpha", "urn:mpeg:hevc:2015:auxid:1"}
	knownProperties = []string{"ispe", "hvcC", "pixi", "auxC", "irot", "imir", "clap", "colr"}
)

type Decoder struct {
	Threads int
}

func (d Decoder) threads() int { return max(d.Threads, serialThreads) }

func (Decoder) ID() codec.ID { return codec.HEIC }

func (Decoder) Name() string { return "HEIC" }

func (Decoder) Extensions() []string { return []string{".heic", ".heif", ".hif"} }

func (Decoder) Sniff(head []byte) bool {
	family := isobmff.Classify(head)
	return family == isobmff.FamilyHEIC || family == isobmff.FamilyUndetermined
}

type inspection struct {
	picture isobmff.Picture
	info    codec.Info
	width   int
	height  int
}

func isUnsupported(err error) bool { return errors.Is(err, heic.ErrUnsupported) }

func chooseItem(file *isobmff.File) (*isobmff.Item, error) {
	if item := file.PrimaryItem(); item != nil {
		return item, nil
	}
	if file.Mini && !file.HasMeta {
		return nil, guard.Unsupported("heic: minimized image boxes are not supported")
	}
	if !file.HasMeta {
		if _, ok := file.Sequence(); ok {
			return nil, guard.Unsupported("heic: image sequences without a still image are not supported")
		}
		return nil, guard.Corrupt("heic: no meta box")
	}
	return nil, guard.Corrupt("heic: no primary item")
}

func inspectFile(ctx context.Context, r io.ReaderAt, size int64, lim codec.Limits) (inspection, error) {
	if err := guard.CheckSize(size, lim); err != nil {
		return inspection{}, err
	}
	if err := ctx.Err(); err != nil {
		return inspection{}, err
	}
	file, err := isobmff.Open(ctx, guard.NewReader(ctx, r, size), size)
	if err != nil {
		return inspection{}, err
	}
	if file.Brands.Family() == isobmff.FamilyAVIF {
		return inspection{}, guard.Unsupported("heic: the file is an AVIF file")
	}
	item, err := chooseItem(file)
	if err != nil {
		return inspection{}, err
	}
	if kind := file.UnknownEssential(item, knownProperties); kind != "" {
		return inspection{}, guard.Unsupported("heic: essential property %q is not supported", kind)
	}
	picture, err := file.Describe(item, alphaURNs)
	if err != nil {
		return inspection{}, err
	}
	switch picture.Codec {
	case isobmff.CodecHEVC:
	case isobmff.CodecAV1:
		return inspection{}, guard.Unsupported("heic: the file holds AV1 pictures, it is an AVIF file")
	default:
		return inspection{}, guard.Unsupported("heic: %s items are not supported", picture.ItemType)
	}
	if err := picture.Check(lim); err != nil {
		return inspection{}, err
	}
	width, height, _ := picture.StoredSize()
	info := codec.Info{
		Format:      codec.HEIC,
		FormatName:  "HEIC",
		Width:       width,
		Height:      height,
		Frames:      1,
		HasAlpha:    picture.Alpha,
		Orientation: picture.Transform.Orientation(),
		BitDepth:    picture.BitsPerPixel(),
	}
	if track, ok := file.Sequence(); ok && track.Samples > 1 {
		info.Frames = track.Samples
		info.Animated = true
	}
	return inspection{picture: picture, info: info, width: width, height: height}, nil
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

func frameSizeLimit(lim codec.Limits) int {
	return int(min(lim.MaxPixels, math.MaxInt32))
}

func decodeImages(ctx context.Context, source io.Reader, all bool, lim codec.Limits, threads int) (assemble.Source, int, error) {
	options := heic.Options{FrameSizeLimit: frameSizeLimit(lim), Threads: threads}
	var out assemble.Source
	var loopCount int
	err := shield.Run(formatTag, func() error {
		if all {
			result, err := heic.DecodeAll(source, options)
			if err != nil {
				return err
			}
			out = assemble.Source{Images: result.Image, Delays: result.Delay}
			loopCount = result.LoopCount
			return nil
		}
		img, err := heic.Decode(source, options)
		if err != nil {
			return err
		}
		out = assemble.Source{Images: []image.Image{img}, Delays: []float64{0}}
		return nil
	})
	if err != nil {
		return assemble.Source{}, 0, guard.Classify(ctx, err, isUnsupported)
	}
	return out, loopCount, nil
}

func (d Decoder) Decode(ctx context.Context, r io.ReaderAt, size int64, o codec.Options) (*codec.Image, error) {
	lim := guard.Limits(o.Limits)
	found, err := inspect(ctx, r, size, lim)
	if err != nil {
		return nil, err
	}
	all := o.Frames == codec.AllFrames && found.info.Animated
	if all {
		if err := guard.CheckFrames(found.picture.FrameLimits(lim), found.info.Frames, found.width, found.height); err != nil {
			return nil, err
		}
	}
	source := io.NewSectionReader(guard.NewReader(ctx, r, size), 0, size)
	result, loopCount, err := decodeImages(ctx, source, all, lim, d.threads())
	if err != nil {
		return nil, err
	}
	frames, err := assemble.Frames(ctx, formatTag, result, found.picture.Transform.Crop)
	if err != nil {
		return nil, err
	}
	info := found.info
	info.Width, info.Height = frames[0].Pix.Bounds().Dx(), frames[0].Pix.Bounds().Dy()
	if all {
		info.Frames = len(frames)
		info.Animated = len(frames) > 1
	}
	return &codec.Image{Info: info, Frames: frames, Loop: isobmff.PlayCount(loopCount), Scale: 1}, nil
}
