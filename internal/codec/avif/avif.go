package avif

import (
	"context"
	"errors"
	"image"
	"io"
	"math"

	"github.com/gen2brain/gav1d/avif"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/assemble"
	"github.com/oops1/PixGo/internal/codec/internal/guard"
	"github.com/oops1/PixGo/internal/codec/internal/shield"
	"github.com/oops1/PixGo/internal/isobmff"
	"github.com/oops1/PixGo/internal/meta"
)

const formatTag = "avif"

var (
	alphaURNs       = []string{"urn:mpeg:mpegB:cicp:systems:auxiliary:alpha"}
	knownProperties = []string{"ispe", "av1C", "pixi", "auxC", "a1op", "irot", "imir", "clap", "colr", "lsel", "a1lx"}
	acceptedBrands  = []string{"avif", "avis", "mif1", "msf1", "miaf", "MA1A", "MA1B"}
)

type Decoder struct{}

func (Decoder) ID() codec.ID { return codec.AVIF }

func (Decoder) Name() string { return "AVIF" }

func (Decoder) Extensions() []string { return []string{".avif"} }

func (Decoder) Sniff(head []byte) bool {
	family := isobmff.Classify(head)
	return family == isobmff.FamilyAVIF || family == isobmff.FamilyUndetermined
}

type inspection struct {
	icc     []byte
	picture isobmff.Picture
	info    codec.Info
	width   int
	height  int
}

func isUnsupported(err error) bool { return errors.Is(err, avif.ErrUnsupported) }

func hasAcceptedBrand(brands isobmff.Brands) bool {
	for _, brand := range acceptedBrands {
		if brands.Contains(brand) {
			return true
		}
	}
	return false
}

func chooseItem(file *isobmff.File) (*isobmff.Item, error) {
	if item := file.PrimaryItem(); item != nil && (item.Type == "av01" || item.Type == "grid") {
		return item, nil
	}
	if item := file.FirstItem("av01", "grid"); item != nil {
		return item, nil
	}
	if file.FirstItem("hvc1", "hvc2") != nil {
		return nil, guard.Unsupported("avif: the file holds HEVC pictures, it is a HEIC file")
	}
	if file.Mini {
		return nil, guard.Unsupported("avif: minimized image boxes are not supported")
	}
	return nil, guard.Corrupt("avif: no AV1 image item")
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
	if !hasAcceptedBrand(file.Brands) {
		if file.Brands.Family() == isobmff.FamilyHEIC {
			return inspection{}, guard.Unsupported("avif: the file is a HEIC file")
		}
		return inspection{}, guard.Corrupt("avif: brand %q is not an AVIF brand", file.Brands.Major)
	}
	if !file.HasMeta {
		if _, ok := file.Sequence(); ok {
			return inspection{}, guard.Unsupported("avif: image sequences without a still image item are not supported")
		}
		return inspection{}, guard.Corrupt("avif: no meta box")
	}
	item, err := chooseItem(file)
	if err != nil {
		return inspection{}, err
	}
	if kind := file.UnknownEssential(item, knownProperties); kind != "" {
		return inspection{}, guard.Unsupported("avif: essential property %q is not supported", kind)
	}
	picture, err := file.Describe(item, alphaURNs)
	if err != nil {
		return inspection{}, err
	}
	if picture.Codec != isobmff.CodecAV1 {
		return inspection{}, guard.Unsupported("avif: %s items are not AV1", picture.ItemType)
	}
	if err := picture.Check(lim); err != nil {
		return inspection{}, err
	}
	width, height, _ := picture.StoredSize()
	info := codec.Info{
		Format:      codec.AVIF,
		FormatName:  "AVIF",
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
	return inspection{icc: meta.HEIFProfile(file, item), picture: picture, info: info, width: width, height: height}, nil
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

func decodeImages(ctx context.Context, source io.Reader, all bool, lim codec.Limits, icc []byte) (assemble.Source, int, codec.ColorInfo, error) {
	options := avif.Options{FrameSizeLimit: frameSizeLimit(lim)}
	var out assemble.Source
	var loopCount int
	var color codec.ColorInfo
	err := shield.Run(formatTag, func() error {
		if all {
			result, err := avif.DecodeAll(source, options)
			if err != nil {
				return err
			}
			out = assemble.Source{Images: result.Image, Delays: result.Delay}
			loopCount = result.LoopCount
			color = assemble.Color(icc, result.Color.Primaries, result.Color.Transfer, result.Color.Matrix, result.Color.FullRange)
			return nil
		}
		img, ci, err := avif.DecodeColor(source, options)
		if err != nil {
			return err
		}
		out = assemble.Source{Images: []image.Image{img}, Delays: []float64{0}}
		color = assemble.Color(icc, ci.Primaries, ci.Transfer, ci.Matrix, ci.FullRange)
		return nil
	})
	if err != nil {
		return assemble.Source{}, 0, codec.ColorInfo{}, guard.Classify(ctx, err, isUnsupported)
	}
	return out, loopCount, color, nil
}

func (Decoder) Decode(ctx context.Context, r io.ReaderAt, size int64, o codec.Options) (*codec.Image, error) {
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
	result, loopCount, color, err := decodeImages(ctx, source, all, lim, found.icc)
	if err != nil {
		return nil, err
	}
	var frames []codec.Frame
	var bounds image.Rectangle
	if o.Deep && assemble.WantsDeep(result, color) {
		frames, err = assemble.DeepFrames(ctx, formatTag, result, found.picture.Transform.Crop)
		if err == nil {
			bounds = frames[0].Deep.Bounds()
		}
	} else {
		frames, err = assemble.Frames(ctx, formatTag, result, found.picture.Transform.Crop)
		if err == nil {
			bounds = frames[0].Pix.Bounds()
		}
	}
	if err != nil {
		return nil, err
	}
	info := found.info
	info.Width, info.Height = bounds.Dx(), bounds.Dy()
	if all {
		info.Frames = len(frames)
		info.Animated = len(frames) > 1
	}
	return &codec.Image{Info: info, Color: color, Frames: frames, Loop: isobmff.PlayCount(loopCount), Scale: 1}, nil
}
