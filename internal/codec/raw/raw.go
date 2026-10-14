package raw

import (
	"context"
	"io"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/guard"
	"github.com/oops1/PixGo/internal/codec/jpeg"
	"github.com/oops1/PixGo/internal/meta/exif"
)

const (
	formatName = "RAW"
	sniffBytes = codec.SniffLen
)

type Decoder struct{}

func (Decoder) ID() codec.ID { return codec.RAW }

func (Decoder) Name() string { return "RAW (встроенный просмотр)" }

func (Decoder) Extensions() []string {
	return []string{".cr2", ".cr3", ".nef", ".arw", ".dng", ".orf", ".rw2", ".pef", ".raf"}
}

func (Decoder) Sniff(head []byte) bool {
	switch identify(head) {
	case containerRAF, containerCR3:
		return true
	case containerTIFF:
		return sniffTIFF(head)
	}
	return false
}

type found struct {
	best           candidate
	orientation    codec.Orientation
	hasOrientation bool
}

func discover(ctx context.Context, r io.ReaderAt, size int64, lim codec.Limits) (found, error) {
	lim = guard.Limits(lim)
	if err := guard.CheckSize(size, lim); err != nil {
		return found{}, err
	}
	if err := ctx.Err(); err != nil {
		return found{}, err
	}
	head := make([]byte, min(size, sniffBytes))
	if err := guard.ReadAtInto(r, head, 0, size); err != nil {
		return found{}, err
	}
	f := newFinder(ctx, r, size)
	var out found
	var err error
	switch identify(head) {
	case containerTIFF:
		out.orientation, out.hasOrientation, err = discoverTIFF(ctx, r, size, f)
	case containerCR3:
		out.orientation, out.hasOrientation, err = discoverCR3(ctx, r, size, f)
	case containerRAF:
		err = discoverRAF(r, size, f)
	default:
		return found{}, guard.Corrupt("raw: not a recognised RAW container")
	}
	if err == nil {
		err = f.carveWhenSmall()
	}
	if err != nil {
		return found{}, guard.Classify(ctx, err, nil)
	}
	if out.best, err = f.choose(lim); err != nil {
		return found{}, err
	}
	if !out.hasOrientation {
		out.orientation, out.hasOrientation = previewOrientation(r, size, out.best)
	}
	return out, nil
}

func previewOrientation(r io.ReaderAt, size int64, c candidate) (codec.Orientation, bool) {
	span := c.span(size)
	payload, err := exif.FromJPEG(io.NewSectionReader(r, c.offset, span), span)
	if err != nil {
		return codec.OrientNormal, false
	}
	return exif.Orientation(payload)
}

func (f found) info() codec.Info {
	orientation := codec.OrientNormal
	if f.hasOrientation {
		orientation = f.orientation
	}
	depth := 24
	switch f.best.head.components {
	case 1:
		depth = 8
	case 4:
		depth = 32
	}
	return codec.Info{
		Format:      codec.RAW,
		FormatName:  formatName,
		Width:       f.best.head.width,
		Height:      f.best.head.height,
		Frames:      1,
		Orientation: orientation,
		BitDepth:    depth,
		Preview:     true,
	}
}

func (Decoder) Probe(ctx context.Context, r io.ReaderAt, size int64, lim codec.Limits) (codec.Info, error) {
	found, err := discover(ctx, r, size, lim)
	if err != nil {
		return codec.Info{}, err
	}
	return found.info(), nil
}

func (Decoder) Decode(ctx context.Context, r io.ReaderAt, size int64, o codec.Options) (*codec.Image, error) {
	lim := guard.Limits(o.Limits)
	found, err := discover(ctx, r, size, lim)
	if err != nil {
		return nil, err
	}
	span := found.best.span(size)
	section := io.NewSectionReader(r, found.best.offset, span)
	decoded, err := jpeg.Decoder{}.Decode(ctx, section, span, codec.Options{Limits: lim, Frames: codec.FirstFrame})
	if err != nil {
		return nil, guard.Classify(ctx, err, nil)
	}
	info := found.info()
	info.Width = decoded.First().Bounds().Dx()
	info.Height = decoded.First().Bounds().Dy()
	return &codec.Image{Info: info, Frames: decoded.Frames, Scale: 1}, nil
}
