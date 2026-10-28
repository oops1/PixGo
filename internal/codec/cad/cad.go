package cad

import (
	"context"
	"io"

	"github.com/oops1/PixGo/internal/codec"
	"github.com/oops1/PixGo/internal/codec/internal/guard"
	"github.com/oops1/PixGo/internal/model3d"
	"github.com/oops1/PixGo/internal/step"
	"github.com/oops1/PixGo/internal/stl"
)

const thumbnailSide = 512

type STLDecoder struct{}

func (STLDecoder) ID() codec.ID { return codec.STL }

func (STLDecoder) Name() string { return "STL" }

func (STLDecoder) Extensions() []string { return []string{".stl"} }

func (STLDecoder) Sniff(head []byte) bool { return stl.Sniff(head) }

func (STLDecoder) ClaimsByExtension() bool { return true }

func (d STLDecoder) Probe(ctx context.Context, r io.ReaderAt, size int64, lim codec.Limits) (codec.Info, error) {
	lim = guard.Limits(lim)
	if err := guard.CheckSize(size, lim); err != nil {
		return codec.Info{}, err
	}
	_, bounds, err := stl.Probe(ctx, r, size)
	if err != nil {
		return codec.Info{}, guard.Classify(ctx, err, nil)
	}
	return info(d, bounds, lim), nil
}

func (d STLDecoder) Decode(ctx context.Context, r io.ReaderAt, size int64, o codec.Options) (*codec.Image, error) {
	return decode(ctx, d, r, size, o)
}

type STEPDecoder struct{}

func (STEPDecoder) ID() codec.ID { return codec.STEP }

func (STEPDecoder) Name() string { return "STEP" }

func (STEPDecoder) Extensions() []string { return []string{".step", ".stp"} }

func (STEPDecoder) Sniff(head []byte) bool { return step.Sniff(head) }

func (d STEPDecoder) Probe(ctx context.Context, r io.ReaderAt, size int64, lim codec.Limits) (codec.Info, error) {
	lim = guard.Limits(lim)
	if err := guard.CheckSize(size, lim); err != nil {
		return codec.Info{}, err
	}
	_, bounds, err := step.Probe(ctx, r, size, step.Options{})
	if err != nil {
		return codec.Info{}, guard.Classify(ctx, err, nil)
	}
	return info(d, bounds, lim), nil
}

func (d STEPDecoder) Decode(ctx context.Context, r io.ReaderAt, size int64, o codec.Options) (*codec.Image, error) {
	return decode(ctx, d, r, size, o)
}

func info(d codec.Decoder, bounds [6]float32, lim codec.Limits) codec.Info {
	w, h := model3d.PictureSize(model3d.BoundsAspect(bounds), codec.Options{}, lim)
	return codec.Info{
		Format:     d.ID(),
		FormatName: d.Name(),
		Width:      w,
		Height:     h,
		Frames:     1,
		HasAlpha:   true,
		BitDepth:   8,
		Vector:     true,
		Model:      true,
	}
}

func decode(ctx context.Context, d codec.Decoder, r io.ReaderAt, size int64, o codec.Options) (*codec.Image, error) {
	lim := guard.Limits(o.Limits)
	if err := guard.CheckSize(size, lim); err != nil {
		return nil, err
	}
	q := model3d.Fine
	if o.VectorW > 0 && o.VectorH > 0 && max(o.VectorW, o.VectorH) <= thumbnailSide {
		q = model3d.Coarse
	}
	m, err := model3d.Read(ctx, d.ID(), r, size, q)
	if err != nil {
		return nil, guard.Classify(ctx, err, nil)
	}
	o.Limits = lim
	pix, err := model3d.Picture(ctx, m, o)
	if err != nil {
		return nil, guard.Classify(ctx, err, nil)
	}
	in := codec.Info{
		Format:     d.ID(),
		FormatName: d.Name(),
		Width:      pix.Rect.Dx(),
		Height:     pix.Rect.Dy(),
		Frames:     1,
		HasAlpha:   true,
		BitDepth:   8,
		Vector:     true,
		Model:      true,
	}
	return &codec.Image{Info: in, Frames: []codec.Frame{{Pix: pix}}, Scale: 1}, nil
}
